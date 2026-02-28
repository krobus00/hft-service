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
	ErrExchangeNotFound            = errors.New("exchange not found")
	ErrPlaceOrderFailed            = errors.New("failed to place order")
	ErrFailToGetOrderHistoryFailed = errors.New("failed to get order history")
	ErrCreateOrderHistoryFailed    = errors.New("failed to create order history")
	ErrDuplicateOrder              = errors.New("duplicate order")
	ErrPublishOrderEventFailed     = errors.New("failed to publish order event")
	ErrInvalidAPIKey               = errors.New("invalid API key")
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
			err := util.ProcessWithTimeout(config.Env.NatsJetstream.TimeoutHandler["place_order"], msg, e.handlePlaceOrderEvent)
			if err != nil {
				logrus.Errorf("error processing message: %v", err)
				return
			}

			err = msg.Ack()
			if err != nil {
				logrus.Errorf("failed to acknowledge message: %v", err)
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

	defer func() {
		if err != nil {
			logger.Error(err)
			req.RetryCount++
			if req.RetryCount >= config.Env.NatsJetstream.MaxRetries {
				return
			}

			err := util.PublishEvent(e.js, constant.OrderEngineStreamSubjectPlaceOrder, req)
			if err != nil {
				logger.Error(err)
				return
			}
		}
	}()

	if req.Data.ExpiredAt != nil && *req.Data.ExpiredAt < time.Now().UTC().Unix() {
		req.RetryCount = config.Env.NatsJetstream.MaxRetries // set retry count to max to prevent further retry
		if req.Data.OrderID != nil {
			return fmt.Errorf("order has expired: %s, order ID: %s", req.Data.Exchange, *req.Data.OrderID)
		}
		return fmt.Errorf("order has expired: %s", req.Data.Exchange)
	}

	_, err = e.PlaceOrder(ctx, req.Data)
	if err != nil {
		if err == ErrExchangeNotFound || err == ErrCreateOrderHistoryFailed || err == ErrDuplicateOrder {
			req.RetryCount = config.Env.NatsJetstream.MaxRetries // set retry count to max to prevent further retry
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

		return orderHistory, nil
	}

	orderHistory, err := exchange.PlaceOrder(ctx, order)
	if err != nil {
		logrus.Error(err)
		return nil, ErrPlaceOrderFailed
	}

	err = s.orderHistoryRepo.Create(ctx, orderHistory)
	if err != nil {
		logrus.Error(err)
		return nil, ErrCreateOrderHistoryFailed
	}

	return orderHistory, nil
}

func (s *OrderEngineService) PlaceOrderAsync(ctx context.Context, order entity.OrderRequest) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	event := entity.OrderRequestEvent{
		RetryCount: 0,
		Data:       order,
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
		Symbol:            order.Symbol,
		OrderID:           fmt.Sprintf("paper-%s", order.RequestID),
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
		ErrorMessage:      sql.NullString{},
		CreatedAt:         now,
		UpdatedAt:         now,
		IsPaperTrading:    true,
	}
}
