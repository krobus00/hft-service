package util

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"

	"github.com/nats-io/nats.go"
)

func ProcessWithTimeout(timeout time.Duration, msg *nats.Msg, callback func(ctx context.Context, msg *nats.Msg) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- callback(ctx, msg)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("processing timeout for message: %s", string(msg.Data))
	case err := <-done:
		return err
	}
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
