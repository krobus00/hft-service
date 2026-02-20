package service

import (
	"context"
	"fmt"
	"math"
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
	MaxLongLevels  int
	StrategySource string
}

func DefaultLazyGridConfig() LazyGridConfig {
	return LazyGridConfig{
		Symbol:         "tkoidr",
		GridPercent:    decimal.NewFromFloat(0.005), // 0.5% grid
		BaseQuantity:   decimal.NewFromFloat(1),
		TotalBudgetIDR: decimal.NewFromInt(1_000_000),
		BuyFeeRate:     decimal.NewFromFloat(0.001222), // 0.1222% fee for limit orders
		InitialPrice:   decimal.NewFromFloat(0),
		MaxLongLevels:  5,
		StrategySource: "lazy-grid",
	}
}

type LazyGridStrategy struct {
	mu                 sync.Mutex
	config             LazyGridConfig
	orderManager       ordermanager.OrderManager
	reference          decimal.Decimal
	openLevelQuantites []decimal.Decimal
}

func NewLazyGridStrategy(config LazyGridConfig, orderManager ordermanager.OrderManager) *LazyGridStrategy {
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
	if config.MaxLongLevels <= 0 {
		config.MaxLongLevels = 1
	}
	if config.StrategySource == "" {
		config.StrategySource = "lazy-grid"
	}

	return &LazyGridStrategy{
		config:       config,
		orderManager: orderManager,
		reference:    config.InitialPrice,
	}
}

func (s *LazyGridStrategy) OnPrice(ctx context.Context, klineData ordermanager.KlineData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	price := klineData.Close
	if price.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("price must be greater than zero")
	}

	if s.reference.Equal(decimal.Zero) {
		s.reference = price
		logrus.WithField("reference", s.reference).Info("lazy-grid initialized")
		return nil
	}

	lowerTrigger := s.reference.Mul(decimal.NewFromFloat(1).Sub(s.config.GridPercent))
	upperTrigger := s.reference.Mul(decimal.NewFromFloat(1).Add(s.config.GridPercent))
	gapGrids := gridDistance(s.reference, price, s.config.GridPercent)
	logrus.WithFields(logrus.Fields{
		"reference":      s.reference,
		"currentPrice":   price,
		"lowerGridPrice": lowerTrigger,
		"upperGridPrice": upperTrigger,
		"grid":           s.config.GridPercent,
		"gapGrids":       gapGrids,
	}).Debug("lazy-grid price band")

	if price.LessThanOrEqual(lowerTrigger) {
		logrus.WithFields(logrus.Fields{
			"reference":    s.reference,
			"currentPrice": price,
			"lowerPrice":   lowerTrigger,
			"upperPrice":   upperTrigger,
			"grid":         s.config.GridPercent,
			"gapGrids":     gapGrids,
			"openLevels":   len(s.openLevelQuantites),
			"maxLevels":    s.config.MaxLongLevels,
		}).Debug("lazy-grid lower trigger hit")

		openLevels := len(s.openLevelQuantites)
		if openLevels >= s.config.MaxLongLevels {
			s.reference = price
			return nil
		}

		levelsToBuy := max(1, gapGrids)
		remainingLevels := s.config.MaxLongLevels - openLevels
		if levelsToBuy > remainingLevels {
			levelsToBuy = remainingLevels
		}

		orderPrice := price
		perLevelQuantity := s.resolvePerLevelQuantity(price)
		orderQuantity := perLevelQuantity.Mul(decimal.NewFromInt(int64(levelsToBuy)))
		if gapGrids > 1 {
			logrus.WithFields(logrus.Fields{
				"reference": s.reference,
				"price":     price,
				"grid":      s.config.GridPercent,
				"gapGrids":  gapGrids,
				"quantity":  orderQuantity,
				"levels":    levelsToBuy,
			}).Info("price moved multiple grids down, placing single optimized buy order")
		}

		err := s.orderManager.PlaceOrder(ctx, ordermanager.OrderRequest{
			Exchange: string(s.config.Exchange),
			Symbol:   s.config.Symbol,
			Type:     s.config.OrderType,
			Side:     ordermanager.OrderSideBuy,
			Price:    orderPrice,
			Quantity: orderQuantity,
			Source:   s.config.StrategySource,
		})
		if err != nil {
			return err
		}

		for i := 0; i < levelsToBuy; i++ {
			s.openLevelQuantites = append(s.openLevelQuantites, perLevelQuantity)
		}
		s.reference = price
		return nil
	}

	if price.GreaterThanOrEqual(upperTrigger) {
		logrus.WithFields(logrus.Fields{
			"reference":    s.reference,
			"currentPrice": price,
			"lowerPrice":   lowerTrigger,
			"upperPrice":   upperTrigger,
			"grid":         s.config.GridPercent,
			"gapGrids":     gapGrids,
			"openLevels":   len(s.openLevelQuantites),
		}).Debug("lazy-grid upper trigger hit")

		openLevels := len(s.openLevelQuantites)
		if openLevels == 0 {
			s.reference = price
			return nil
		}

		levelsToSell := max(1, gapGrids)
		if levelsToSell > openLevels {
			levelsToSell = openLevels
		}

		orderPrice := price
		orderQuantity := decimal.Zero
		startIdx := openLevels - levelsToSell
		for i := startIdx; i < openLevels; i++ {
			orderQuantity = orderQuantity.Add(s.openLevelQuantites[i])
		}
		if gapGrids > 1 {
			logrus.WithFields(logrus.Fields{
				"reference": s.reference,
				"price":     price,
				"grid":      s.config.GridPercent,
				"gapGrids":  gapGrids,
				"quantity":  orderQuantity,
				"levels":    levelsToSell,
			}).Info("price moved multiple grids up, placing single optimized sell order")
		}

		err := s.orderManager.PlaceOrder(ctx, ordermanager.OrderRequest{
			Exchange: string(s.config.Exchange),
			Symbol:   s.config.Symbol,
			Type:     s.config.OrderType,
			Side:     ordermanager.OrderSideSell,
			Price:    orderPrice,
			Quantity: orderQuantity,
			Source:   s.config.StrategySource,
		})
		if err != nil {
			return err
		}

		s.openLevelQuantites = s.openLevelQuantites[:startIdx]
		s.reference = price
	}

	return nil
}

func (s *LazyGridStrategy) resolvePerLevelQuantity(price decimal.Decimal) decimal.Decimal {
	if s.config.TotalBudgetIDR.LessThanOrEqual(decimal.Zero) {
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

func gridDistance(reference, price, gridPercent decimal.Decimal) int {
	if reference.LessThanOrEqual(decimal.Zero) || gridPercent.LessThanOrEqual(decimal.Zero) {
		return 0
	}

	deltaRatio, _ := price.Sub(reference).Abs().Div(reference).Float64()
	gridSize, _ := gridPercent.Float64()
	if gridSize <= 0 {
		return 0
	}

	return int(math.Floor(deltaRatio / gridSize))
}
