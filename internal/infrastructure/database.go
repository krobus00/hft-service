package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/krobus00/hft-service/internal/config"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

const (
	defaultConnectTimeout = 5 * time.Second
	defaultBackoffFactor  = 2.0
	defaultMinJitter      = 100 * time.Millisecond
	defaultMaxJitter      = 1 * time.Second
	defaultMaxIdleConns   = 10
	defaultMaxOpenConns   = 100
	defaultConnLifetime   = 1 * time.Hour
)

func NewPostgresConnection(ctx context.Context, cfg config.DatabaseConfig) (*sqlx.DB, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errors.New("database dsn is required")
	}

	connectTimeout := cfg.PingInterval
	if connectTimeout <= 0 {
		connectTimeout = defaultConnectTimeout
	}

	maxRetry := cfg.MaxRetry
	if maxRetry < 0 {
		maxRetry = 0
	}

	backoffFactor := cfg.ReconnectFactor
	if backoffFactor < 1 {
		backoffFactor = defaultBackoffFactor
	}

	minJitter := cfg.MinJitter
	if minJitter <= 0 {
		minJitter = defaultMinJitter
	}

	maxJitter := cfg.MaxJitter
	if maxJitter <= 0 {
		maxJitter = defaultMaxJitter
	}
	if maxJitter < minJitter {
		maxJitter = minJitter
	}

	maxIdleConns := cfg.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = defaultMaxIdleConns
	}

	maxOpenConns := cfg.MaxActiveConns
	if maxOpenConns <= 0 {
		maxOpenConns = defaultMaxOpenConns
	}

	maxConnLifetime := cfg.MaxConnLifetime
	if maxConnLifetime <= 0 {
		maxConnLifetime = defaultConnLifetime
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var lastErr error

	for attempt := 0; attempt <= maxRetry; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		attemptCtx, cancel := context.WithTimeout(ctx, connectTimeout)
		db, err := sqlx.ConnectContext(attemptCtx, "postgres", cfg.DSN)
		cancel()
		if err == nil {
			db.SetMaxIdleConns(maxIdleConns)
			db.SetMaxOpenConns(maxOpenConns)
			db.SetConnMaxLifetime(maxConnLifetime)
			if cfg.PingInterval > 0 {
				db.SetConnMaxIdleTime(cfg.PingInterval)
			}

			logrus.WithFields(logrus.Fields{
				"max_retry":         maxRetry,
				"max_idle_conns":    maxIdleConns,
				"max_active_conns":  maxOpenConns,
				"max_conn_lifetime": maxConnLifetime,
			}).Info("postgres connection established")

			return db, nil
		}

		lastErr = err
		if attempt == maxRetry {
			break
		}

		waitDuration := backoffWithJitter(attempt, backoffFactor, minJitter, maxJitter, rng)
		logrus.WithFields(logrus.Fields{
			"attempt":      attempt + 1,
			"max_retry":    maxRetry,
			"retry_in":     waitDuration.String(),
			"postgres_dsn": maskDSN(cfg.DSN),
		}).Warnf("postgres connection failed: %v", err)

		select {
		case <-time.After(waitDuration):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("connect postgres after %d attempts: %w", maxRetry+1, lastErr)
}

func StartPostgresHealthCheck(ctx context.Context, db *sqlx.DB, interval time.Duration) {
	if db == nil || interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, cancel := context.WithTimeout(ctx, interval)
				err := db.PingContext(pingCtx)
				cancel()
				if err != nil {
					logrus.Errorf("postgres health check failed: %v", err)
				}
			}
		}
	}()
}

func backoffWithJitter(attempt int, factor float64, min, max time.Duration, rng *rand.Rand) time.Duration {
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

func maskDSN(dsn string) string {
	idx := strings.Index(dsn, "@")
	if idx == -1 {
		return dsn
	}

	prefix := dsn[:idx]
	credsIdx := strings.LastIndex(prefix, "://")
	if credsIdx == -1 {
		return "***" + dsn[idx:]
	}

	return prefix[:credsIdx+3] + "***" + dsn[idx:]
}
