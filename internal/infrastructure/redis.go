package infrastructure

import (
	"context"
	"strings"

	"github.com/krobus00/hft-service/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

func NewRedisClient(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	if strings.TrimSpace(cfg.CacheDSN) == "" {
		return nil, nil
	}

	opt, err := redis.ParseURL(cfg.CacheDSN)
	if err != nil {
		return nil, err
	}
	if cfg.MaxRetry > 0 {
		opt.MaxRetries = cfg.MaxRetry
	}
	if cfg.MaxIdleConns > 0 {
		opt.MaxIdleConns = cfg.MaxIdleConns
	}
	if cfg.MaxActiveConns > 0 {
		opt.PoolSize = cfg.MaxActiveConns
	}
	if cfg.MaxConnLifetime > 0 {
		opt.ConnMaxLifetime = cfg.MaxConnLifetime
	}

	client := redis.NewClient(opt)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	logrus.Info("redis connection established")
	return client, nil
}
