package infrastructure

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

const (
	defaultNatsMaxRetries      = 10
	defaultNatsBackoffFactor   = 2.0
	defaultNatsMinJitter       = 100 * time.Millisecond
	defaultNatsMaxJitter       = 2 * time.Second
	defaultNatsConnectTimeout  = 5 * time.Second
	defaultNatsDrainTimeout   = 10 * time.Second
	defaultNatsPingInterval    = 30 * time.Second
	defaultNatsPingOutstanding = 3
	defaultJetStreamMaxWait    = 5 * time.Second
)

func NewJetstream() (nc *nats.Conn, js nats.JetStreamContext, err error) {
	cfg := config.Env.NatsJetstream
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, nil, errors.New("nats jetstream url is required")
	}

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultNatsMaxRetries
	}

	backoffFactor := cfg.ReconnectFactor
	if backoffFactor < 1 {
		backoffFactor = defaultNatsBackoffFactor
	}

	minJitter := cfg.MinJitter
	if minJitter <= 0 {
		minJitter = defaultNatsMinJitter
	}

	maxJitter := cfg.MaxJitter
	if maxJitter <= 0 {
		maxJitter = defaultNatsMaxJitter
	}
	if maxJitter < minJitter {
		maxJitter = minJitter
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	nc, err = nats.Connect(cfg.URL,
		nats.Name("hft-service"),
		nats.Timeout(defaultNatsConnectTimeout),
		nats.DrainTimeout(defaultNatsDrainTimeout),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(maxRetries),
		nats.PingInterval(defaultNatsPingInterval),
		nats.MaxPingsOutstanding(defaultNatsPingOutstanding),
		nats.CustomReconnectDelay(func(attempts int) time.Duration {
			return reconnectDelayWithJitter(attempts, backoffFactor, minJitter, maxJitter, rng)
		}),
		nats.DisconnectErrHandler(func(conn *nats.Conn, disErr error) {
			if disErr != nil {
				logrus.Warnf("nats disconnected: %v", disErr)
				return
			}
			logrus.Warn("nats disconnected")
		}),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			logrus.Infof("nats reconnected: %s", conn.ConnectedUrl())
		}),
		nats.ClosedHandler(func(conn *nats.Conn) {
			logrus.Warnf("nats connection closed: %v", conn.LastError())
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connect nats: %w", err)
	}

	js, err = nc.JetStream(
		nats.PublishAsyncMaxPending(256),
		nats.MaxWait(defaultJetStreamMaxWait),
	)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("create jetstream context: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"url":         cfg.URL,
		"max_retries": maxRetries,
	}).Info("nats jetstream connection established")

	return nc, js, nil
}

func reconnectDelayWithJitter(attempt int, factor float64, min, max time.Duration, rng *rand.Rand) time.Duration {
	backoff := float64(min) * math.Pow(factor, float64(attempt))
	if backoff > float64(max) {
		backoff = float64(max)
	}

	base := time.Duration(backoff)
	if max <= min {
		return base
	}

	jitterWindow := max - min
	jitter := time.Duration(rng.Int63n(int64(jitterWindow) + 1))
	result := base + jitter
	if result > max {
		return max
	}

	return result
}

func CloseJetstream(nc *nats.Conn) error {
	if nc == nil {
		return nil
	}

	if err := nc.Drain(); err != nil {
		nc.Close()
		return fmt.Errorf("drain nats connection: %w", err)
	}

	nc.Close()
	return nil
}
