package orderengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

var (
	ErrExchangeNotFound               = errors.New("exchange not found")
	ErrPlaceOrderFailed               = errors.New("failed to place order")
	ErrFailToGetOrderHistoryFailed    = errors.New("failed to get order history")
	ErrCreateOrderHistoryFailed       = errors.New("failed to create order history")
	ErrDuplicateOrder                 = errors.New("duplicate order")
	ErrInvalidEntryOrderID            = errors.New("entry_order_id is required")
	ErrPublishOrderEventFailed        = errors.New("failed to publish order event")
	ErrPublishOrderNotificationFailed = errors.New("failed to publish order notification event")
	ErrInvalidAPIKey                  = errors.New("invalid API key")
)

type OrderEngineService struct {
	exchanges        map[entity.ExchangeName]entity.Exchange
	orderHistoryRepo *repository.OrderHistoryRepository
	js               nats.JetStreamContext
}

func NewOrderEngineService(exchanges map[entity.ExchangeName]entity.Exchange, orderHistoryRepo *repository.OrderHistoryRepository, js nats.JetStreamContext) *OrderEngineService {
	return &OrderEngineService{
		exchanges:        exchanges,
		orderHistoryRepo: orderHistoryRepo,
		js:               js,
	}
}

func (e *OrderEngineService) JetstreamEventInit(ctx context.Context) error {
	streamConfig := &nats.StreamConfig{
		Name:      constant.OrderEngineStreamName,
		Subjects:  []string{constant.OrderEngineStreamSubjectAll},
		Retention: nats.WorkQueuePolicy,
		Storage:   nats.FileStorage,
		MaxAge:    24 * time.Hour,
	}

	stream, err := e.js.StreamInfo(constant.OrderEngineStreamName, nats.Context(ctx))
	if err != nil && !errors.Is(err, nats.ErrStreamNotFound) {
		logrus.Error(err)
		return err
	}

	if stream == nil {
		logrus.Infof("creating stream: %s", constant.OrderEngineStreamName)
		_, err = e.js.AddStream(streamConfig, nats.Context(ctx))
		return err
	}

	logrus.Infof("updating stream: %s", constant.OrderEngineStreamName)
	_, err = e.js.UpdateStream(streamConfig, nats.Context(ctx))
	if err != nil {
		logrus.Error(err)
		return err
	}

	return nil
}

func (e *OrderEngineService) JetstreamEventSubscribe(ctx context.Context) error {
	err := e.JetstreamEventInit(ctx)
	if err != nil {
		logrus.Error(err)
		return err
	}

	_, err = e.js.QueueSubscribe(
		constant.OrderEngineStreamSubjectPlaceOrder,
		constant.OrderEngineQueueName,
		func(msg *nats.Msg) {
			err := util.ProcessWithTimeout(config.Env.NatsJetstream.TimeoutHandler["place_order"], config.Env.NatsJetstream.MaxRetries, msg, e.handlePlaceOrderEvent)
			if err != nil {
				logrus.Errorf("error processing message: %v", err)
				return
			}
		},
		nats.ManualAck(),
		nats.Durable(constant.OrderEngineQueueGroup),
	)
	util.ContinueOrFatal(err)

	return nil
}

func (e *OrderEngineService) handlePlaceOrderEvent(ctx context.Context, msg *nats.Msg) (err error) {
	logger := logrus.WithFields(logrus.Fields{
		"req": string(msg.Data),
	})

	var req *entity.OrderRequestEvent
	err = json.Unmarshal(msg.Data, &req)
	if err != nil {
		logger.Error(err)
		return err
	}

	if req.Data.ExpiredAt != nil && *req.Data.ExpiredAt < time.Now().UTC().Unix() {
		if req.Data.OrderID != nil {
			return fmt.Errorf("order has expired: %s, order ID: %s", req.Data.Exchange, *req.Data.OrderID)
		}
		return fmt.Errorf("order has expired: %s", req.Data.Exchange)
	}

	_, err = e.PlaceOrder(ctx, req.Data)
	if err != nil {
		if err == ErrExchangeNotFound || err == ErrCreateOrderHistoryFailed || err == ErrDuplicateOrder {
			logger.WithError(err).Warn("place_order event dropped")
			return nil
		}
		logger.Error(err)
		return err
	}

	return nil
}

func (s *OrderEngineService) PlaceOrder(ctx context.Context, order entity.OrderRequest) (*entity.OrderHistory, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	order.Exchange = strings.ToLower(strings.TrimSpace(order.Exchange))

	marketType := entity.NormalizeMarketType(order.MarketType)
	order.MarketType = string(marketType)

	normalizedSide := entity.NormalizeOrderSideByMarket(string(order.Side), marketType)
	if normalizedSide == "" {
		return nil, fmt.Errorf("invalid order side for market type %s: %s", marketType, order.Side)
	}
	order.Side = normalizedSide

	if marketType == entity.MarketTypeFutures {
		normalizedPosition := entity.NormalizePositionSide(order.PositionSide)
		if normalizedPosition == entity.PositionSideBoth {
			if order.Side == entity.OrderSideShort {
				normalizedPosition = entity.PositionSideShort
			} else {
				normalizedPosition = entity.PositionSideLong
			}
		}
		order.PositionSide = string(normalizedPosition)
	} else {
		order.PositionSide = string(entity.PositionSideBoth)
	}

	order.TradeCondition = string(entity.NormalizeTradeCondition(order.TradeCondition))
	order.OrderReason = strings.TrimSpace(order.OrderReason)
	order.ExitType = normalizeExitType(order.ExitType, order.TradeCondition)
	if err := ensureEntryOrderID(&order); err != nil {
		return nil, err
	}

	exchange, ok := s.exchanges[entity.ExchangeName(order.Exchange)]
	if !ok {
		logrus.Errorf("exchange not found: %s", order.Exchange)
		return nil, ErrExchangeNotFound
	}

	existingOrderHistory, err := s.orderHistoryRepo.GetByRequestID(ctx, order.RequestID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logrus.Error(err)
		return nil, ErrFailToGetOrderHistoryFailed
	}

	if existingOrderHistory != nil {
		logrus.Warnf("duplicate order request: %s, request ID: %s", order.Exchange, order.RequestID)
		return nil, ErrDuplicateOrder
	}

	if order.IsPaperTrading {
		orderHistory := buildPaperOrderHistory(order)

		err := s.orderHistoryRepo.Create(ctx, orderHistory)
		if err != nil {
			logrus.Error(err)
			return nil, ErrCreateOrderHistoryFailed
		}

		err = s.publishOrderNotificationEvent(order, orderHistory)
		if err != nil {
			logrus.Error(err)
			return nil, err
		}

		return orderHistory, nil
	}

	orderHistory, err := exchange.PlaceOrder(ctx, order)
	if err != nil {
		logrus.Error(err)
		return nil, ErrPlaceOrderFailed
	}

	orderHistory = applyOrderMetadataToHistory(orderHistory, order)

	err = s.orderHistoryRepo.Create(ctx, orderHistory)
	if err != nil {
		logrus.Error(err)
		return nil, ErrCreateOrderHistoryFailed
	}

	err = s.publishOrderNotificationEvent(order, orderHistory)
	if err != nil {
		logrus.Error(err)
		return nil, err
	}

	return orderHistory, nil
}

func (s *OrderEngineService) PlaceOrderAsync(ctx context.Context, order entity.OrderRequest) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	order.Exchange = strings.ToLower(strings.TrimSpace(order.Exchange))

	marketType := entity.NormalizeMarketType(order.MarketType)
	order.MarketType = string(marketType)

	normalizedSide := entity.NormalizeOrderSideByMarket(string(order.Side), marketType)
	if normalizedSide == "" {
		return fmt.Errorf("invalid order side for market type %s: %s", marketType, order.Side)
	}
	order.Side = normalizedSide

	if marketType == entity.MarketTypeFutures {
		normalizedPosition := entity.NormalizePositionSide(order.PositionSide)
		if normalizedPosition == entity.PositionSideBoth {
			if order.Side == entity.OrderSideShort {
				normalizedPosition = entity.PositionSideShort
			} else {
				normalizedPosition = entity.PositionSideLong
			}
		}
		order.PositionSide = string(normalizedPosition)
	} else {
		order.PositionSide = string(entity.PositionSideBoth)
	}

	order.TradeCondition = string(entity.NormalizeTradeCondition(order.TradeCondition))
	order.OrderReason = strings.TrimSpace(order.OrderReason)
	order.ExitType = normalizeExitType(order.ExitType, order.TradeCondition)
	if err := ensureEntryOrderID(&order); err != nil {
		return err
	}

	event := entity.OrderRequestEvent{
		Data: order,
	}

	err := util.PublishEvent(s.js, constant.OrderEngineStreamSubjectPlaceOrder, event)
	if err != nil {
		logrus.Error(err)
		return ErrPublishOrderEventFailed
	}

	return nil
}

func buildPaperOrderHistory(order entity.OrderRequest) *entity.OrderHistory {
	now := time.Now().UTC()

	sentAt := sql.NullTime{Time: now, Valid: true}
	if order.RequestedAt > 0 {
		sentAt = sql.NullTime{Time: time.UnixMilli(order.RequestedAt).UTC(), Valid: true}
	}

	acknowledgedAt := sql.NullTime{Time: now, Valid: true}
	filledAt := sql.NullTime{Time: now, Valid: true}

	strategyID := sql.NullString{}
	if order.StrategyID != nil {
		trimmed := strings.TrimSpace(*order.StrategyID)
		if trimmed != "" {
			strategyID = sql.NullString{String: trimmed, Valid: true}
		}
	}

	clientOrderID := sql.NullString{}
	if order.OrderID != nil {
		trimmed := strings.TrimSpace(*order.OrderID)
		if trimmed != "" {
			clientOrderID = sql.NullString{String: trimmed, Valid: true}
		}
	}

	price := order.Price
	var avgFillPrice *decimal.Decimal
	if price.GreaterThan(decimal.Zero) {
		avgFillPrice = &price
	}

	return &entity.OrderHistory{
		RequestID:         order.RequestID,
		UserID:            order.UserID,
		Exchange:          order.Exchange,
		MarketType:        order.MarketType,
		PositionSide:      order.PositionSide,
		Symbol:            order.Symbol,
		OrderID:           fmt.Sprintf("paper-%s", order.RequestID),
		EntryOrderID:      strings.TrimSpace(order.EntryOrderID),
		ClientOrderID:     clientOrderID,
		Side:              order.Side,
		Type:              order.Type,
		Price:             &price,
		Quantity:          order.Quantity,
		FilledQuantity:    order.Quantity,
		AvgFillPrice:      avgFillPrice,
		Status:            "FILLED",
		Leverage:          nil,
		Fee:               nil,
		RealizedPnl:       nil,
		CreatedAtExchange: sql.NullTime{},
		SentAt:            sentAt,
		AcknowledgedAt:    acknowledgedAt,
		FilledAt:          filledAt,
		StrategyID:        strategyID,
		TradeCondition:    order.TradeCondition,
		OrderReason:       order.OrderReason,
		ExitType:          order.ExitType,
		ErrorMessage:      sql.NullString{},
		CreatedAt:         now,
		UpdatedAt:         now,
		IsPaperTrading:    true,
	}
}

func (s *OrderEngineService) publishOrderNotificationEvent(order entity.OrderRequest, orderHistory *entity.OrderHistory) error {
	if !order.NeedNotification {
		return nil
	}

	strategyID := ""
	if order.StrategyID != nil {
		strategyID = strings.TrimSpace(*order.StrategyID)
	}

	strategyName := strings.TrimSpace(order.StrategyName)
	if strategyName == "" {
		strategyName = strategyID
	}

	price := resolveNotificationPrice(order, orderHistory)

	entryPrice := strings.TrimSpace(order.EntryPrice)
	exitPrice := strings.TrimSpace(order.ExitPrice)
	pnlPercentage := strings.TrimSpace(order.PnlPercentage)
	tradeCondition := strings.ToUpper(strings.TrimSpace(order.TradeCondition))

	if exitPrice == "" && tradeCondition != string(entity.TradeConditionEntry) {
		exitPrice = price
	}

	event := entity.OrderNotificationEvent{
		Data: entity.OrderNotification{
			RequestID:      order.RequestID,
			EntryOrderID:   strings.TrimSpace(order.EntryOrderID),
			Exchange:       order.Exchange,
			MarketType:     order.MarketType,
			StrategyID:     strategyID,
			StrategyName:   strategyName,
			Symbol:         order.Symbol,
			Interval:       strings.TrimSpace(order.Interval),
			Side:           strings.ToUpper(strings.TrimSpace(string(order.Side))),
			Price:          price,
			EntryPrice:     entryPrice,
			ExitPrice:      exitPrice,
			PnlPercentage:  pnlPercentage,
			TradeCondition: tradeCondition,
			OrderReason:    strings.TrimSpace(order.OrderReason),
			ExitType:       normalizeExitType(order.ExitType, order.TradeCondition),
		},
	}

	err := util.PublishEvent(s.js, constant.OrderEngineStreamSubjectNotificationAlert, event)
	if err != nil {
		return ErrPublishOrderNotificationFailed
	}

	return nil
}

func resolveNotificationPrice(order entity.OrderRequest, orderHistory *entity.OrderHistory) string {
	if orderHistory != nil {
		if orderHistory.AvgFillPrice != nil && orderHistory.AvgFillPrice.GreaterThan(decimal.Zero) {
			return orderHistory.AvgFillPrice.String()
		}

		if orderHistory.Price != nil && orderHistory.Price.GreaterThan(decimal.Zero) {
			return orderHistory.Price.String()
		}
	}

	if order.Price.GreaterThan(decimal.Zero) {
		return order.Price.String()
	}

	if fallback := firstPositiveDecimalString(order.ExitPrice, order.EntryPrice); fallback != "" {
		return fallback
	}

	return ""
}

func firstPositiveDecimalString(values ...string) string {
	for _, raw := range values {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}

		v, err := decimal.NewFromString(candidate)
		if err != nil {
			continue
		}

		if v.GreaterThan(decimal.Zero) {
			return v.String()
		}
	}

	return ""
}

func normalizeExitType(rawExitType, tradeCondition string) string {
	normalized := entity.NormalizeExitType(rawExitType)
	if normalized != "" {
		return string(normalized)
	}

	switch entity.NormalizeTradeCondition(tradeCondition) {
	case entity.TradeConditionTakeProfit:
		return string(entity.ExitTypeTakeProfit)
	case entity.TradeConditionStopLoss:
		return string(entity.ExitTypeStopLoss)
	case entity.TradeConditionTrailingStop:
		return string(entity.ExitTypeTrailingStop)
	default:
		return ""
	}
}

func ensureEntryOrderID(order *entity.OrderRequest) error {
	if order == nil {
		return ErrInvalidEntryOrderID
	}

	entryOrderID := strings.TrimSpace(order.EntryOrderID)
	if entryOrderID == "" {
		entryOrderID = extractEntryOrderIDFromInternal(order.Internal)
	}

	if entryOrderID == "" {
		return ErrInvalidEntryOrderID
	}

	order.EntryOrderID = entryOrderID
	return nil
}

func extractEntryOrderIDFromInternal(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return ""
	}

	value, ok := payload["entry_order_id"]
	if !ok || value == nil {
		return ""
	}

	return strings.TrimSpace(fmt.Sprint(value))
}

func applyOrderMetadataToHistory(orderHistory *entity.OrderHistory, order entity.OrderRequest) *entity.OrderHistory {
	if orderHistory == nil {
		return nil
	}

	if strings.TrimSpace(orderHistory.TradeCondition) == "" || strings.EqualFold(strings.TrimSpace(orderHistory.TradeCondition), string(entity.TradeConditionUnknown)) {
		orderHistory.TradeCondition = order.TradeCondition
	}

	if strings.TrimSpace(orderHistory.OrderReason) == "" {
		orderHistory.OrderReason = order.OrderReason
	}

	if strings.TrimSpace(orderHistory.ExitType) == "" {
		orderHistory.ExitType = order.ExitType
	}

	if strings.TrimSpace(orderHistory.EntryOrderID) == "" {
		orderHistory.EntryOrderID = strings.TrimSpace(order.EntryOrderID)
	}

	return orderHistory
}
