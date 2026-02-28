package kline

import (
	"context"
	"errors"
	"time"

	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/repository"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type klineService struct {
	marketKlineRepo       *repository.MarketKlineRepository
	symbolMappingRepo     *repository.SymbolMappingRepository
	klineSubscriptionRepo *repository.KlineSubscriptionRepository
	js                    nats.JetStreamContext
}

func NewKlineService(js nats.JetStreamContext, marketKlineRepo *repository.MarketKlineRepository, symbolMappingRepo *repository.SymbolMappingRepository, klineSubscriptionRepo *repository.KlineSubscriptionRepository) *klineService {
	return &klineService{
		js:                    js,
		marketKlineRepo:       marketKlineRepo,
		symbolMappingRepo:     symbolMappingRepo,
		klineSubscriptionRepo: klineSubscriptionRepo,
	}
}

func (s *klineService) JetstreamEventInit(ctx context.Context) error {
	streamConfig := &nats.StreamConfig{
		Name:      constant.KlineStreamName,
		Subjects:  []string{constant.KlineStreamSubjectAll},
		Storage:   nats.FileStorage, // use MemoryStorage for ultra-low latency
		Retention: nats.LimitsPolicy,
		MaxAge:    5 * time.Minute,
		Replicas:  1,
	}

	stream, err := s.js.StreamInfo(constant.KlineStreamName)
	if err != nil && !errors.Is(err, nats.ErrStreamNotFound) {
		logrus.Error(err)
		return err
	}

	if stream == nil {
		logrus.Infof("creating stream: %s", constant.KlineStreamName)
		_, err = s.js.AddStream(streamConfig, nats.Context(ctx))
		return err
	}

	logrus.Infof("updating stream: %s", constant.KlineStreamName)
	_, err = s.js.UpdateStream(streamConfig, nats.Context(ctx))
	if err != nil {
		logrus.Error(err)
		return err
	}

	logrus.Infof("stream %s is ready", constant.KlineStreamName)

	return nil
}
