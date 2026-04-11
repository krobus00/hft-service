package util

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/sirupsen/logrus"

	"github.com/nats-io/nats.go"
)

func ProcessWithTimeout(timeout time.Duration, maxRetry uint64, msg *nats.Msg, callback func(ctx context.Context, msg *nats.Msg) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- processMessageWithRetry(ctx, maxRetry, msg, callback)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("processing timeout for message: %s", string(msg.Data))
	case err := <-done:
		return err
	}
}

func processMessageWithRetry(ctx context.Context, maxRetry uint64, msg *nats.Msg, callback func(ctx context.Context, msg *nats.Msg) error) error {
	meta, err := msg.Metadata()
	if err != nil {
		logrus.WithError(err).Error("metadata error")
		msg.Nak()
		return err
	}

	attempt := meta.NumDelivered
	logrus.Infof("processing msg (attempt %d)", attempt)

	err = callback(ctx, msg)
	if err != nil {
		logrus.WithError(err).Error("failed to process message")

		if attempt >= maxRetry {
			logrus.Errorf("max retry attempts reached for message: %s", string(msg.Data))
			msg.Term()
			return err
		}

		delay := time.Duration(attempt*2) * time.Second
		msg.NakWithDelay(delay)
		return err
	}

	msg.Ack()
	return nil
}

func PublishEvent(js nats.JetStreamContext, subject string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = js.Publish(subject, payload)
	if err != nil {
		return err
	}

	return nil
}

func EnsureConsumer(ctx context.Context, js nats.JetStreamContext, streamName string, consumerConfig *nats.ConsumerConfig) error {
	if consumerConfig == nil {
		return fmt.Errorf("consumer config is required")
	}

	durable := strings.TrimSpace(consumerConfig.Durable)
	if durable == "" {
		return fmt.Errorf("consumer durable name is required")
	}

	_, err := js.ConsumerInfo(streamName, durable, nats.Context(ctx))
	if err == nil {
		return nil
	}

	if !errors.Is(err, nats.ErrConsumerNotFound) {
		return err
	}

	_, err = js.AddConsumer(streamName, consumerConfig, nats.Context(ctx))
	if err != nil {
		return err
	}

	return nil
}
