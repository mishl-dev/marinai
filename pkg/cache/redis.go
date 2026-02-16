package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	RecentMessagesTTL = 24 * time.Hour
	EmojiCacheTTL     = 6 * time.Hour
	LLMResponseTTL    = 1 * time.Hour
	UserStateTTL      = 7 * 24 * time.Hour
)

type Cache struct {
	client *redis.Client
	prefix string
}

func NewRedisCache(url string, prefix string) (*Cache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Cache{
		client: client,
		prefix: prefix,
	}, nil
}

func (c *Cache) Key(parts ...string) string {
	if c.prefix == "" {
		return strings.Join(parts, ":")
	}
	return c.prefix + ":" + strings.Join(parts, ":")
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

func (c *Cache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *Cache) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *Cache) GetJSON(ctx context.Context, key string, dest any) error {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := c.client.Exists(ctx, key).Result()
	return result > 0, err
}

func (c *Cache) LPush(ctx context.Context, key string, values ...string) error {
	args := make([]interface{}, len(values))
	for i, v := range values {
		args[i] = v
	}
	return c.client.LPush(ctx, key, args...).Err()
}

func (c *Cache) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.LRange(ctx, key, start, stop).Result()
}

func (c *Cache) LTrim(ctx context.Context, key string, start, stop int64) error {
	return c.client.LTrim(ctx, key, start, stop).Err()
}

func (c *Cache) LLen(ctx context.Context, key string) (int64, error) {
	return c.client.LLen(ctx, key).Result()
}

func (c *Cache) HSet(ctx context.Context, key string, field string, value string) error {
	return c.client.HSet(ctx, key, field, value).Err()
}

func (c *Cache) HGet(ctx context.Context, key string, field string) (string, error) {
	return c.client.HGet(ctx, key, field).Result()
}

func (c *Cache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

func (c *Cache) HDel(ctx context.Context, key string, fields ...string) error {
	return c.client.HDel(ctx, key, fields...).Err()
}

func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Cache) Close() error {
	return c.client.Close()
}
