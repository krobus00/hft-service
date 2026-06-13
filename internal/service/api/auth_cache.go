package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var errAuthCacheMiss = errors.New("auth cache miss")

type AuthCache struct {
	client *redis.Client
	ttl    time.Duration
}

type RefreshSession struct {
	User AuthUser `json:"user"`
}

func NewAuthCache(client *redis.Client, ttl time.Duration) *AuthCache {
	if client == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &AuthCache{client: client, ttl: ttl}
}

func (c *AuthCache) GetUser(ctx context.Context, userID string) (*AuthUser, error) {
	if c == nil || c.client == nil {
		return nil, errAuthCacheMiss
	}
	raw, err := c.client.Get(ctx, c.userKey(userID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errAuthCacheMiss
		}
		return nil, err
	}

	var user AuthUser
	if err := json.Unmarshal([]byte(raw), &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *AuthCache) SetUser(ctx context.Context, user *AuthUser) error {
	if c == nil || c.client == nil || user == nil {
		return nil
	}
	payload, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.userKey(user.ID), payload, c.ttl).Err()
}

func (c *AuthCache) DeleteUser(ctx context.Context, userID string) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Del(ctx, c.userKey(userID)).Err()
}

func (c *AuthCache) GetRefreshSession(ctx context.Context, tokenHash string) (*RefreshSession, error) {
	if c == nil || c.client == nil {
		return nil, errAuthCacheMiss
	}
	raw, err := c.client.Get(ctx, c.refreshKey(tokenHash)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errAuthCacheMiss
		}
		return nil, err
	}

	var session RefreshSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *AuthCache) SetRefreshSession(ctx context.Context, tokenHash string, session *RefreshSession, ttl time.Duration) error {
	if c == nil || c.client == nil || session == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = c.ttl
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.refreshKey(tokenHash), payload, ttl).Err()
}

func (c *AuthCache) DeleteRefreshSession(ctx context.Context, tokenHash string) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Del(ctx, c.refreshKey(tokenHash)).Err()
}

func (c *AuthCache) userKey(userID string) string {
	return "hft:api:auth:user:" + userID
}

func (c *AuthCache) refreshKey(tokenHash string) string {
	return "hft:api:auth:refresh:" + tokenHash
}
