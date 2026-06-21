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
	meta, err := msg.Metadata()
	if err != nil {
		_ = msg.Term()
		return fmt.Errorf("read message metadata: %w", err)
	}
	attempt := meta.NumDelivered
	if attempt > 1 {
		logrus.WithFields(logrus.Fields{"subject": msg.Subject, "attempt": attempt}).Info("redelivered message")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- callback(ctx, msg)
	}()

	var processingErr error
	select {
	case <-ctx.Done():
		processingErr = fmt.Errorf("processing timeout: %w", ctx.Err())
	case processingErr = <-done:
	}
	if processingErr == nil {
		if err := msg.Ack(); err != nil {
			return fmt.Errorf("ack message: %w", err)
		}
		return nil
	}

	delay, terminate := messageRetryDecision(attempt, maxRetry)
	if terminate {
		if err := msg.Term(); err != nil {
			return fmt.Errorf("terminate message after processing error %v: %w", processingErr, err)
		}
		logrus.WithFields(logrus.Fields{"subject": msg.Subject, "attempt": attempt}).WithError(processingErr).Warn("message retries exhausted")
		return processingErr
	}

	if err := msg.NakWithDelay(delay); err != nil {
		return fmt.Errorf("schedule message retry after processing error %v: %w", processingErr, err)
	}
	return processingErr
}

func messageRetryDecision(attempt, maxRetry uint64) (time.Duration, bool) {
	if maxRetry == 0 || attempt >= maxRetry {
		return 0, true
	}
	return time.Duration(attempt*2) * time.Second, false
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
