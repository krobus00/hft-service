package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/krobus00/hft-service/internal/constant"
	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/util"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type Service struct {
	js             nats.JetStreamContext
	httpClient     *http.Client
	discordWebhook string
}

func NewService(js nats.JetStreamContext) *Service {
	return &Service{
		js: js,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		discordWebhook: strings.TrimSpace(config.Env.Notification.DiscordWebhookURL),
	}
}

func (s *Service) JetstreamEventInit(ctx context.Context) error {
	streamConfig := &nats.StreamConfig{
		Name:      constant.OrderEngineStreamName,
		Subjects:  []string{constant.OrderEngineStreamSubjectAll},
		Retention: nats.WorkQueuePolicy,
		Storage:   nats.FileStorage,
		MaxAge:    24 * time.Hour,
	}

	stream, err := s.js.StreamInfo(constant.OrderEngineStreamName, nats.Context(ctx))
	if err != nil && !errors.Is(err, nats.ErrStreamNotFound) {
		return err
	}

	if stream == nil {
		_, err = s.js.AddStream(streamConfig, nats.Context(ctx))
		return err
	}

	_, err = s.js.UpdateStream(streamConfig, nats.Context(ctx))
	return err
}

func (s *Service) JetstreamEventSubscribe(ctx context.Context) error {
	err := s.JetstreamEventInit(ctx)
	if err != nil {
		return err
	}

	_, err = s.js.QueueSubscribe(
		constant.OrderEngineStreamSubjectNotificationAlert,
		constant.NotificationQueueName,
		func(msg *nats.Msg) {
			timeout := config.Env.NatsJetstream.TimeoutHandler["notification_alert"]
			if timeout <= 0 {
				timeout = 5 * time.Second
			}

			callbackErr := util.ProcessWithTimeout(
				timeout,
				config.Env.NatsJetstream.MaxRetries,
				msg,
				s.handleOrderNotificationEvent,
			)
			if callbackErr != nil {
				logrus.WithError(callbackErr).Error("error processing notification message")
			}
		},
		nats.ManualAck(),
		nats.Durable(constant.NotificationQueueGroup),
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) handleOrderNotificationEvent(ctx context.Context, msg *nats.Msg) error {
	if strings.TrimSpace(s.discordWebhook) == "" {
		return errors.New("notification.discord_webhook_url is required")
	}

	var req entity.OrderNotificationEvent
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		return err
	}

	return s.sendDiscordAlert(ctx, req.Data)
}

func (s *Service) sendDiscordAlert(ctx context.Context, payload entity.OrderNotification) error {
	body := map[string]string{
		"content": fmt.Sprintf(
			"order alert | strategy=%s | symbol=%s | internal=%s | interval=%s | side=%s | price=%s",
			fallback(payload.StrategyName, payload.StrategyID),
			fallback(payload.Symbol, "-"),
			fallback(payload.Internal, payload.Symbol),
			fallback(payload.Interval, "-"),
			payload.Side,
			fallback(payload.Price, "-"),
		),
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.discordWebhook, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("discord webhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func fallback(primary, secondary string) string {
	primary = strings.TrimSpace(primary)
	if primary != "" {
		return primary
	}

	return strings.TrimSpace(secondary)
}
