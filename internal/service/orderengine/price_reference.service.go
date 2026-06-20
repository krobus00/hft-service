package orderengine

import (
	"context"
	"encoding/json"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

const priceReferenceConsumer = "PRICE_REFERENCE"

type PriceReferenceService struct {
	repo *repository.PriceReferenceRepository
	js   nats.JetStreamContext
}

func NewPriceReferenceService(repo *repository.PriceReferenceRepository, js nats.JetStreamContext) *PriceReferenceService {
	return &PriceReferenceService{repo: repo, js: js}
}

func (s *PriceReferenceService) Subscribe(ctx context.Context) error {
	_, err := s.js.QueueSubscribe(
		constant.KlineStreamSubjectAll,
		priceReferenceConsumer,
		func(msg *nats.Msg) {
			if err := util.ProcessWithTimeout(config.Env.NatsJetstream.TimeoutHandler["insert_kline"], config.Env.NatsJetstream.MaxRetries, msg, s.handleKline); err != nil {
				logrus.WithError(err).Error("failed to update price reference")
			}
		},
		nats.ManualAck(),
		nats.Durable(priceReferenceConsumer),
		nats.DeliverNew(),
	)
	return err
}

func (s *PriceReferenceService) handleKline(ctx context.Context, msg *nats.Msg) error {
	var event entity.MarketKlineEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		return err
	}
	return s.repo.UpsertKline(ctx, event.Data)
}
