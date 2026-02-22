package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/krobus00/hft-service/internal/service/ordermanager"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

type LazyGridConfig struct {
	Symbol         string
	Exchange       ordermanager.ExchangeName
	OrderType      ordermanager.OrderType
	GridPercent    decimal.Decimal
	BaseQuantity   decimal.Decimal
	TotalBudgetIDR decimal.Decimal
	BuyFeeRate     decimal.Decimal
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
		BuyFeeRate:     decimal.NewFromFloat(0.001222), // 0.1222% fee for limit orders, 0.3322% for market orders
		InitialPrice:   decimal.NewFromFloat(0),
		MaxLongLevels:  0,
		StrategySource: "lazy-grid",
		StateKey:       "",
	}
}

type LazyGridStrategy struct {
	mu            sync.Mutex
	config        LazyGridConfig
	orderManager  ordermanager.OrderManager
	stateStore    LazyGridStateStore
	anchorPrice   decimal.Decimal
	lastGridLevel int
	filledLevels  map[int]decimal.Decimal
	pendingBuys   map[int]decimal.Decimal
	pendingSells  map[int]decimal.Decimal
}

func NewLazyGridStrategy(ctx context.Context, config LazyGridConfig, orderManager ordermanager.OrderManager, stateStore LazyGridStateStore) (*LazyGridStrategy, error) {
	if config.Symbol == "" {
		config.Symbol = "tkoidr"
	}
	if config.Exchange == "" {
		config.Exchange = ordermanager.ExchangeTokoCrypto
	}
	if config.OrderType == "" {
		config.OrderType = ordermanager.OrderTypeLimit
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
	if config.BuyFeeRate.Equal(decimal.Zero) {
		if config.OrderType == ordermanager.OrderTypeMarket {
			config.BuyFeeRate = decimal.NewFromFloat(0.002222) // 0.2222% fee for market orders
		} else {
			config.BuyFeeRate = decimal.NewFromFloat(0.001222) // 0.1222% fee for limit orders
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
		config:        config,
		orderManager:  orderManager,
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

func (s *LazyGridStrategy) OnPrice(ctx context.Context, klineData ordermanager.KlineData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !klineData.IsClosed {
		return nil
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

				if err := s.orderManager.PlaceOrder(ctx, ordermanager.OrderRequest{
					Exchange: string(s.config.Exchange),
					Symbol:   s.config.Symbol,
					Type:     s.config.OrderType,
					Side:     ordermanager.OrderSideBuy,
					Price:    orderPrice,
					Quantity: orderQuantity,
					Source:   s.config.StrategySource,
				}); err != nil {
					logrus.Error(err)
					return err
				}

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
				orderPrice := s.takeProfitPrice(level)

				if err := s.orderManager.PlaceOrder(ctx, ordermanager.OrderRequest{
					Exchange: string(s.config.Exchange),
					Symbol:   s.config.Symbol,
					Type:     s.config.OrderType,
					Side:     ordermanager.OrderSideSell,
					Price:    orderPrice,
					Quantity: orderQuantity,
					Source:   s.config.StrategySource,
				}); err != nil {
					logrus.Error(err)
					return err
				}

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

func (s *LazyGridStrategy) takeProfitPrice(level int) decimal.Decimal {
	return s.levelPrice(level + 1)
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
