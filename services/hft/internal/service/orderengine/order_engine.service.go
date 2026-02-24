package orderengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

var (
	ErrExchangeNotFound            = errors.New("exchange not found")
	ErrPlaceOrderFailed            = errors.New("failed to place order")
	ErrFailToGetOrderHistoryFailed = errors.New("failed to get order history")
	ErrCreateOrderHistoryFailed    = errors.New("failed to create order history")
	ErrDuplicateOrder              = errors.New("duplicate order")
	ErrPublishOrderEventFailed     = errors.New("failed to publish order event")
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

func (e *OrderEngineService) JetstreamEventInit() error {
	streamConfig := &nats.StreamConfig{
		Name:      constant.OrderEngineStreamName,
		Subjects:  []string{constant.OrderEngineStreamSubjectAll},
		Retention: nats.WorkQueuePolicy,
		Storage:   nats.FileStorage,
		MaxAge:    24 * time.Hour,
	}

	stream, err := e.js.StreamInfo(constant.OrderEngineStreamName)
	if err != nil && !errors.Is(err, nats.ErrStreamNotFound) {
		logrus.Error(err)
		return err
	}

	if stream == nil {
		logrus.Infof("creating stream: %s", constant.OrderEngineStreamName)
		_, err = e.js.AddStream(streamConfig)
		return err
	}

	logrus.Infof("updating stream: %s", constant.OrderEngineStreamName)
	_, err = e.js.UpdateStream(streamConfig)
	if err != nil {
		logrus.Error(err)
		return err
	}

	return nil
}

func (e *OrderEngineService) JetstreamEventSubscribe() error {
	err := e.JetstreamEventInit()
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
