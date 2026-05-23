package util

import (
	"context"
	"fmt"
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
	logrus.Infof("subject: %s, subscribed message: %s, attempt: %d", msg.Subject, string(msg.Data), attempt)

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
