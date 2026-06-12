package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
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

const (
	defaultExchangeRecvWindow = 5000
	maxExchangeRecvWindow     = 60000
)

type exchangeCredentials struct {
	APIKey    string
	APISecret string
}

func loadExchangeAccounts(accounts map[string]config.ExchangeAccountConfig) map[string]exchangeCredentials {
	credentials := make(map[string]exchangeCredentials, len(accounts))
	for rawUserID, account := range accounts {
		userID := strings.TrimSpace(rawUserID)
		if userID == "" {
			continue
		}

		credentials[userID] = exchangeCredentials{
			APIKey:    strings.TrimSpace(account.APIKey),
			APISecret: strings.TrimSpace(account.APISecret),
		}
	}

	return credentials
}

func credentialsForUser(exchangeName entity.ExchangeName, defaultCredentials exchangeCredentials, userCredentials map[string]exchangeCredentials, userID string) (string, string, error) {
	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID != "" {
		if account, ok := userCredentials[trimmedUserID]; ok {
			if account.APIKey != "" && account.APISecret != "" {
				return account.APIKey, account.APISecret, nil
			}
			return "", "", fmt.Errorf("%s credentials are missing for user_id=%s", exchangeName, trimmedUserID)
		}
	}

	if defaultCredentials.APIKey == "" || defaultCredentials.APISecret == "" {
		if trimmedUserID != "" {
			return "", "", fmt.Errorf("%s credentials are missing for user_id=%s and no default credentials configured", exchangeName, trimmedUserID)
		}
		return "", "", fmt.Errorf("%s credentials are missing in config", exchangeName)
	}

	return defaultCredentials.APIKey, defaultCredentials.APISecret, nil
}

func recvWindowFromEnv(envKey string) int64 {
	raw := envString(envKey)
	if raw == "" {
		return defaultExchangeRecvWindow
	}

	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 || parsed > maxExchangeRecvWindow {
		return defaultExchangeRecvWindow
	}

	return parsed
}

func ensureKlineStream(ctx context.Context, js nats.JetStreamContext) error {
	streamConfig := &nats.StreamConfig{
		Name:      constant.KlineStreamName,
		Subjects:  []string{constant.KlineStreamSubjectAll},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    5 * time.Minute,
		Replicas:  1,
	}

	stream, err := js.StreamInfo(constant.KlineStreamName, nats.Context(ctx))
	if err != nil && !errors.Is(err, nats.ErrStreamNotFound) {
		logrus.Error(err)
		return err
	}

	if stream == nil {
		logrus.Infof("creating stream: %s", constant.KlineStreamName)
		_, err = js.AddStream(streamConfig, nats.Context(ctx))
		return err
	}

	logrus.Infof("updating stream: %s", constant.KlineStreamName)
	if _, err = js.UpdateStream(streamConfig, nats.Context(ctx)); err != nil {
		logrus.Error(err)
		return err
	}

	logrus.Infof("stream %s is ready", constant.KlineStreamName)
	return nil
}

func subscribeKlineInsertEvents(ctx context.Context, js nats.JetStreamContext, exchangeName entity.ExchangeName, handler func(context.Context, *nats.Msg) error) error {
	if err := ensureKlineStream(ctx, js); err != nil {
		logrus.Error(err)
		return err
	}

	consumerName := constant.GetKlineInsertQueueGroup(string(exchangeName))
	logrus.WithFields(logrus.Fields{
		"exchange":      exchangeName,
		"subject":       constant.GetKlineExchangeStreamSubject(string(exchangeName)),
		"consumer_name": consumerName,
	}).Info("subscribing kline insert events")

	_, err := js.QueueSubscribe(
		constant.GetKlineExchangeStreamSubject(string(exchangeName)),
		consumerName,
		func(msg *nats.Msg) {
			err := util.ProcessWithTimeout(config.Env.NatsJetstream.TimeoutHandler["insert_kline"], config.Env.NatsJetstream.MaxRetries, msg, handler)
			if err != nil {
				logrus.Errorf("error processing message: %v", err)
				return
			}
		},
		nats.ManualAck(),
		nats.Durable(consumerName),
		nats.DeliverNew(),
	)
	util.ContinueOrFatal(err)

	logrus.WithFields(logrus.Fields{
		"exchange":      exchangeName,
		"consumer_name": consumerName,
	}).Info("kline insert event subscription ready")

	return nil
}

func insertKlineEvent(ctx context.Context, marketKlineRepo *repository.MarketKlineRepository, msg *nats.Msg) error {
	logger := logrus.WithFields(logrus.Fields{
		"req": string(msg.Data),
	})

	var req *entity.MarketKlineEvent
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		logger.Error(err)
		return err
	}

	if req.Data.EventTime.UTC().Add(1 * time.Minute).Before(time.Now().UTC()) {
		logger.Info("skipping kline data event that is too old")
		return nil
	}

	if err := marketKlineRepo.Create(ctx, &req.Data); err != nil {
		logger.Error(err)
		return err
	}

	logrus.WithFields(logrus.Fields{
		"exchange":    req.Data.Exchange,
		"market_type": req.Data.MarketType,
		"symbol":      req.Data.Symbol,
		"interval":    req.Data.Interval,
		"open_time":   req.Data.OpenTime.UTC().Format(time.RFC3339),
		"is_closed":   req.Data.IsClosed,
	}).Debug("kline event inserted")

	return nil
}

func signPayload(secret, payload string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(payload))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func decimalOrZero(raw string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return decimal.Zero, nil
	}
	return decimal.NewFromString(trimmed)
}

func orderStrategyID(raw *string) sql.NullString {
	if raw == nil {
		return sql.NullString{}
	}

	trimmed := strings.TrimSpace(*raw)
	return sql.NullString{String: trimmed, Valid: trimmed != ""}
}

func orderSentAt(requestedAt int64) sql.NullTime {
	if requestedAt > 0 {
		return sql.NullTime{Time: time.UnixMilli(requestedAt).UTC(), Valid: true}
	}

	return sql.NullTime{Time: time.Now(), Valid: true}
}

func envString(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
