package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/krobus00/hft-service/internal/entity"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

type LazyGridConfig struct {
	Symbol         string
	Exchange       entity.ExchangeName
	OrderType      entity.OrderType
	GridPercent    decimal.Decimal
	BaseQuantity   decimal.Decimal
	TotalBudgetIDR decimal.Decimal
	BuyFeeRate     decimal.Decimal
	SellFeeRate    decimal.Decimal
	InitialPrice   decimal.Decimal
	// MaxLongLevels controls maximum concurrent long levels.
	// 0 means unlimited.
	MaxLongLevels  int
	StrategySource string
	StateKey       string
}

func DefaultLazyGridConfig() LazyGridConfig {
	return LazyGridConfig{
		Symbol:         "tkoidr",
		GridPercent:    decimal.NewFromFloat(0.005), // 0.5% grid
		BaseQuantity:   decimal.NewFromFloat(50),
		TotalBudgetIDR: decimal.NewFromInt(1_000_000),
		BuyFeeRate:     decimal.NewFromFloat(0.002222), // 0.2222% fee for limit orders
		SellFeeRate:    decimal.NewFromFloat(0.003322), // 0.3322% fee for limit orders
		InitialPrice:   decimal.NewFromFloat(0),
		MaxLongLevels:  0,
		StrategySource: "lazy-grid",
		StateKey:       "",
	}
}

type LazyGridStrategy struct {
	mu     sync.Mutex
	config LazyGridConfig
	// orderManager  entity.OrderManager
	stateStore    LazyGridStateStore
	anchorPrice   decimal.Decimal
	lastGridLevel int
	filledLevels  map[int]decimal.Decimal
	pendingBuys   map[int]decimal.Decimal
	pendingSells  map[int]decimal.Decimal
}

const lazyGridProcessingLockTTL = 15 * time.Second

func NewLazyGridStrategy(ctx context.Context, config LazyGridConfig, stateStore LazyGridStateStore) (*LazyGridStrategy, error) {
	if config.Symbol == "" {
		config.Symbol = "tkoidr"
	}
	if config.Exchange == "" {
		config.Exchange = entity.ExchangeTokoCrypto
	}
	if config.OrderType == "" {
		config.OrderType = entity.OrderTypeLimit
	}
	if config.GridPercent.LessThanOrEqual(decimal.Zero) {
		config.GridPercent = decimal.NewFromFloat(0.005)
	}
	if config.BaseQuantity.LessThanOrEqual(decimal.Zero) {
		config.BaseQuantity = decimal.NewFromFloat(1)
	}
	if config.BuyFeeRate.LessThan(decimal.Zero) || config.BuyFeeRate.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		config.BuyFeeRate = decimal.Zero
	}
	if config.SellFeeRate.LessThan(decimal.Zero) || config.SellFeeRate.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		config.SellFeeRate = decimal.Zero
	}
	if config.BuyFeeRate.Equal(decimal.Zero) {
		if config.OrderType == entity.OrderTypeMarket {
			config.BuyFeeRate = decimal.NewFromFloat(0.002222) // 0.2222% fee for market orders
		} else {
			config.BuyFeeRate = decimal.NewFromFloat(0.002222) // 0.2222% fee for limit orders
		}
	}
	if config.SellFeeRate.Equal(decimal.Zero) {
		if config.OrderType == entity.OrderTypeMarket {
			config.SellFeeRate = decimal.NewFromFloat(0.004322) // 0.4322% fee for market orders
		} else {
			config.SellFeeRate = decimal.NewFromFloat(0.003322) // 0.3322% fee for limit orders
		}
	}
	if config.MaxLongLevels < 0 {
		config.MaxLongLevels = 0
	}
	if config.StrategySource == "" {
		config.StrategySource = "lazy-grid"
	}
	if config.StateKey == "" {
		config.StateKey = fmt.Sprintf("lazy-grid:%s:%s:%s", config.Exchange, config.Symbol, config.StrategySource)
	}

	strategy := &LazyGridStrategy{
		config: config,
		// orderManager:  orderManager,
		stateStore:    stateStore,
		anchorPrice:   config.InitialPrice,
		lastGridLevel: 0,
		filledLevels:  make(map[int]decimal.Decimal),
		pendingBuys:   make(map[int]decimal.Decimal),
		pendingSells:  make(map[int]decimal.Decimal),
	}

	if strategy.stateStore != nil {
		persistedState, found, err := strategy.stateStore.Load(ctx, config.StateKey)
		if err != nil {
			return nil, err
		}
		if found {
			strategy.anchorPrice = persistedState.AnchorPrice
			strategy.lastGridLevel = persistedState.LastLevel
			strategy.filledLevels = make(map[int]decimal.Decimal, len(persistedState.Positions))
			strategy.pendingBuys = make(map[int]decimal.Decimal, len(persistedState.PendingBuys))
			strategy.pendingSells = make(map[int]decimal.Decimal, len(persistedState.PendingSells))
			for _, position := range persistedState.Positions {
				if position.Quantity.LessThanOrEqual(decimal.Zero) {
					continue
				}
				strategy.filledLevels[position.Level] = position.Quantity
			}
			for _, pendingBuy := range persistedState.PendingBuys {
				if pendingBuy.Quantity.LessThanOrEqual(decimal.Zero) {
					continue
				}
				strategy.pendingBuys[pendingBuy.Level] = pendingBuy.Quantity
			}
			for _, pendingSell := range persistedState.PendingSells {
				if pendingSell.Quantity.LessThanOrEqual(decimal.Zero) {
					continue
				}
				strategy.pendingSells[pendingSell.Level] = pendingSell.Quantity
			}
			if len(strategy.filledLevels) == 0 && len(persistedState.FilledLevels) > 0 {
				for _, level := range persistedState.FilledLevels {
					levelPrice := strategy.levelPrice(level)
					strategy.filledLevels[level] = strategy.resolvePerLevelQuantity(levelPrice)
				}
			}

			logrus.WithFields(logrus.Fields{
				"stateKey":      config.StateKey,
				"anchorPrice":   strategy.anchorPrice,
				"lastGridLevel": strategy.lastGridLevel,
				"positionCount": len(strategy.filledLevels),
				"pendingBuys":   len(strategy.pendingBuys),
				"pendingSells":  len(strategy.pendingSells),
			}).Info("lazy-grid state restored from redis")
		}
	}

	return strategy, nil
}

func (s *LazyGridStrategy) OnPrice(ctx context.Context, klineData entity.KlineData) error {
	if !klineData.IsClosed {
		return nil
	}

	lockOwner := fmt.Sprintf("%d", time.Now().UnixNano())
	if s.stateStore != nil {
		acquired, err := s.stateStore.AcquireProcessingLock(ctx, s.config.StateKey, lazyGridProcessingLockTTL, lockOwner)
		if err != nil {
			return err
		}
		if !acquired {
			logrus.WithField("stateKey", s.config.StateKey).Debug("lazy-grid processing lock not acquired, skipping tick")
			return nil
		}

		defer func() {
			if err := s.stateStore.ReleaseProcessingLock(context.Background(), s.config.StateKey, lockOwner); err != nil {
				logrus.WithError(err).WithField("stateKey", s.config.StateKey).Warn("lazy-grid release processing lock failed")
			}
		}()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.refreshFromStore(ctx); err != nil {
		return err
	}

	price := klineData.Close
	if price.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("price must be greater than zero")
	}
	if s.anchorPrice.Equal(decimal.Zero) {
		s.anchorPrice = price
		s.lastGridLevel = 0
		if err := s.persistState(ctx); err != nil {
			return err
		}
		logrus.WithFields(logrus.Fields{
			"anchorPrice": s.anchorPrice,
			"stateKey":    s.config.StateKey,
		}).Info("lazy-grid initialized")
		return nil
	}

	s.assumePendingOrderFills(price)

	currentLevel := gridLevel(s.anchorPrice, price, s.config.GridPercent)
	if currentLevel == s.lastGridLevel {
		if err := s.persistState(ctx); err != nil {
			return err
		}
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"anchorPrice":   s.anchorPrice,
		"currentPrice":  price,
		"lastGridLevel": s.lastGridLevel,
		"currentLevel":  currentLevel,
		"positionCount": len(s.filledLevels),
		"pendingBuys":   len(s.pendingBuys),
		"pendingSells":  len(s.pendingSells),
		"grid":          s.config.GridPercent,
	}).Debug("lazy-grid state transition")

	if currentLevel < s.lastGridLevel {
		levelsToBuy := s.collectBuyLevels(currentLevel, s.lastGridLevel)
		if len(levelsToBuy) > 0 {
			totalQuantity := decimal.Zero
			for _, level := range levelsToBuy {
				orderPrice := s.levelPrice(level)
				orderQuantity := s.resolvePerLevelQuantity(orderPrice)
				if orderQuantity.LessThanOrEqual(decimal.Zero) {
					continue
				}

				// if err := s.orderManager.PlaceOrder(ctx, entity.OrderRequest{
				// 	Exchange: string(s.config.Exchange),
				// 	Symbol:   s.config.Symbol,
				// 	Type:     s.config.OrderType,
				// 	Side:     entity.OrderSideBuy,
				// 	Price:    orderPrice,
				// 	Quantity: orderQuantity,
				// 	Source:   s.config.StrategySource,
				// }); err != nil {
				// 	logrus.Error(err)
				// 	return err
				// }

				s.pendingBuys[level] = orderQuantity
				totalQuantity = totalQuantity.Add(orderQuantity)
			}
			logrus.WithFields(logrus.Fields{
				"levels":        levelsToBuy,
				"orderQuantity": totalQuantity,
			}).Info("lazy-grid buy levels submitted")
		}
	}

	if currentLevel > s.lastGridLevel {
		levelsToSell := s.collectSellLevels(currentLevel)
		if len(levelsToSell) > 0 {
			totalQuantity := decimal.Zero
			for _, level := range levelsToSell {
				orderQuantity, exists := s.filledLevels[level]
				if !exists || orderQuantity.LessThanOrEqual(decimal.Zero) {
					continue
				}
				// orderPrice := s.takeProfitPrice(level)

				// if err := s.orderManager.PlaceOrder(ctx, entity.OrderRequest{
				// 	Exchange: string(s.config.Exchange),
				// 	Symbol:   s.config.Symbol,
				// 	Type:     s.config.OrderType,
				// 	Side:     entity.OrderSideSell,
				// 	Price:    orderPrice,
				// 	Quantity: orderQuantity,
				// 	Source:   s.config.StrategySource,
				// }); err != nil {
				// 	logrus.Error(err)
				// 	return err
				// }

				s.pendingSells[level] = orderQuantity
				totalQuantity = totalQuantity.Add(orderQuantity)
			}
			logrus.WithFields(logrus.Fields{
				"levels":        levelsToSell,
				"orderQuantity": totalQuantity,
			}).Info("lazy-grid sell levels submitted")
		}
	}

	s.lastGridLevel = currentLevel
	if err := s.persistState(ctx); err != nil {
		return err
	}

	return nil
}

func (s *LazyGridStrategy) refreshFromStore(ctx context.Context) error {
	if s.stateStore == nil {
		return nil
	}

	persistedState, found, err := s.stateStore.Load(ctx, s.config.StateKey)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	s.applyStateSnapshot(persistedState)
	return nil
}

func (s *LazyGridStrategy) applyStateSnapshot(state LazyGridState) {
	s.anchorPrice = state.AnchorPrice
	s.lastGridLevel = state.LastLevel

	s.filledLevels = make(map[int]decimal.Decimal, len(state.Positions))
	s.pendingBuys = make(map[int]decimal.Decimal, len(state.PendingBuys))
	s.pendingSells = make(map[int]decimal.Decimal, len(state.PendingSells))

	for _, position := range state.Positions {
		if position.Quantity.LessThanOrEqual(decimal.Zero) {
			continue
		}
		s.filledLevels[position.Level] = position.Quantity
	}

	for _, pendingBuy := range state.PendingBuys {
		if pendingBuy.Quantity.LessThanOrEqual(decimal.Zero) {
			continue
		}
		s.pendingBuys[pendingBuy.Level] = pendingBuy.Quantity
	}

	for _, pendingSell := range state.PendingSells {
		if pendingSell.Quantity.LessThanOrEqual(decimal.Zero) {
			continue
		}
		s.pendingSells[pendingSell.Level] = pendingSell.Quantity
	}

	if len(s.filledLevels) == 0 && len(state.FilledLevels) > 0 {
		for _, level := range state.FilledLevels {
			levelPrice := s.levelPrice(level)
			s.filledLevels[level] = s.resolvePerLevelQuantity(levelPrice)
		}
	}
}

func (s *LazyGridStrategy) takeProfitPrice(level int) decimal.Decimal {
	grossTarget := s.levelPrice(level + 1)
	if s.config.SellFeeRate.LessThanOrEqual(decimal.Zero) {
		return grossTarget
	}

	netRatio := decimal.NewFromInt(1).Sub(s.config.SellFeeRate)
	if netRatio.LessThanOrEqual(decimal.Zero) {
		return grossTarget
	}

	return grossTarget.Div(netRatio)
}

func (s *LazyGridStrategy) collectBuyLevels(currentLevel int, previousLevel int) []int {
	levels := make([]int, 0)
	for level := previousLevel - 1; level >= currentLevel; level-- {
		if level >= 0 {
			continue
		}
		if _, exists := s.filledLevels[level]; exists {
			continue
		}
		if _, exists := s.pendingBuys[level]; exists {
			continue
		}
		if s.config.MaxLongLevels > 0 && len(s.filledLevels)+len(levels) >= s.config.MaxLongLevels {
			break
		}
		levels = append(levels, level)
	}

	return levels
}

func (s *LazyGridStrategy) collectSellLevels(currentLevel int) []int {
	levels := make([]int, 0)
	for level := range s.filledLevels {
		if level < currentLevel {
			if _, exists := s.pendingSells[level]; exists {
				continue
			}
			levels = append(levels, level)
		}
	}
	sort.Ints(levels)
	return levels
}

func (s *LazyGridStrategy) sortedFilledLevels() []int {
	levels := make([]int, 0, len(s.filledLevels))
	for level := range s.filledLevels {
		levels = append(levels, level)
	}
	sort.Ints(levels)
	return levels
}

func (s *LazyGridStrategy) sortedPositions() []LazyGridPosition {
	levels := s.sortedFilledLevels()
	positions := make([]LazyGridPosition, 0, len(levels))
	for _, level := range levels {
		positions = append(positions, LazyGridPosition{
			Level:    level,
			Quantity: s.filledLevels[level],
		})
	}

	return positions
}

func sortedLevelQuantities(levels map[int]decimal.Decimal) []LazyGridPosition {
	keys := make([]int, 0, len(levels))
	for level := range levels {
		keys = append(keys, level)
	}
	sort.Ints(keys)

	positions := make([]LazyGridPosition, 0, len(keys))
	for _, level := range keys {
		quantity := levels[level]
		if quantity.LessThanOrEqual(decimal.Zero) {
			continue
		}
		positions = append(positions, LazyGridPosition{
			Level:    level,
			Quantity: quantity,
		})
	}

	return positions
}

func (s *LazyGridStrategy) levelPrice(level int) decimal.Decimal {
	step, _ := decimal.NewFromInt(1).Add(s.config.GridPercent).Float64()
	if step <= 1 {
		return s.anchorPrice
	}

	return s.anchorPrice.Mul(decimal.NewFromFloat(math.Pow(step, float64(level))))
}

func (s *LazyGridStrategy) persistState(ctx context.Context) error {
	if s.stateStore == nil {
		return nil
	}

	state := LazyGridState{
		AnchorPrice:  s.anchorPrice,
		LastLevel:    s.lastGridLevel,
		Positions:    s.sortedPositions(),
		PendingBuys:  sortedLevelQuantities(s.pendingBuys),
		PendingSells: sortedLevelQuantities(s.pendingSells),
	}
	if err := s.stateStore.Save(ctx, s.config.StateKey, state); err != nil {
		return err
	}

	return nil
}

func (s *LazyGridStrategy) resolvePerLevelQuantity(price decimal.Decimal) decimal.Decimal {
	if s.config.TotalBudgetIDR.LessThanOrEqual(decimal.Zero) {
		return s.config.BaseQuantity
	}
	if s.config.MaxLongLevels <= 0 {
		return s.config.BaseQuantity
	}

	perLevelBudget := s.config.TotalBudgetIDR.Div(decimal.NewFromInt(int64(s.config.MaxLongLevels)))
	netPerLevelBudget := perLevelBudget.Mul(decimal.NewFromInt(1).Sub(s.config.BuyFeeRate))
	if netPerLevelBudget.LessThanOrEqual(decimal.Zero) || price.LessThanOrEqual(decimal.Zero) {
		return s.config.BaseQuantity
	}

	quantity := netPerLevelBudget.Div(price)
	if quantity.LessThanOrEqual(decimal.Zero) {
		return s.config.BaseQuantity
	}

	return quantity
}

func (s *LazyGridStrategy) assumePendingOrderFills(price decimal.Decimal) {
	for level, quantity := range s.pendingBuys {
		if quantity.LessThanOrEqual(decimal.Zero) {
			delete(s.pendingBuys, level)
			continue
		}

		buyPrice := s.levelPrice(level)
		if price.LessThanOrEqual(buyPrice) {
			s.filledLevels[level] = quantity
			delete(s.pendingBuys, level)

			logrus.WithFields(logrus.Fields{
				"level":    level,
				"buyPrice": buyPrice,
				"quantity": quantity,
			}).Info("lazy-grid buy level assumed filled")
		}
	}

	for level := range s.pendingSells {
		sellPrice := s.takeProfitPrice(level)
		if price.GreaterThanOrEqual(sellPrice) {
			delete(s.pendingSells, level)
			delete(s.filledLevels, level)

			logrus.WithFields(logrus.Fields{
				"level":     level,
				"sellPrice": sellPrice,
			}).Info("lazy-grid sell level assumed filled")
		}
	}
}

func gridLevel(anchorPrice, price, gridPercent decimal.Decimal) int {
	if anchorPrice.LessThanOrEqual(decimal.Zero) || price.LessThanOrEqual(decimal.Zero) || gridPercent.LessThanOrEqual(decimal.Zero) {
		return 0
	}

	ratio, _ := price.Div(anchorPrice).Float64()
	gridSize, _ := gridPercent.Float64()
	if ratio <= 0 || gridSize <= 0 {
		return 0
	}

	step := 1 + gridSize
	if step <= 1 {
		return 0
	}

	return int(math.Floor(math.Log(ratio) / math.Log(step)))
}
