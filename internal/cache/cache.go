package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/yourusername/cdn-simulator/internal/config"
	"github.com/yourusername/cdn-simulator/pkg/types"
)

type Cache interface {
	Get(ctx context.Context, key types.CacheKey) (*types.CacheEntry, error)
	Set(ctx context.Context, entry *types.CacheEntry) error
	Delete(ctx context.Context, key types.CacheKey) error
	InvalidateByTags(ctx context.Context, tags []string) error
	InvalidateByPattern(ctx context.Context, pattern string) error
	GetStats(ctx context.Context) (*CacheStats, error)
	Close() error
}

type CacheStats struct {
	Hits       int64
	Misses     int64
	StaleHits  int64
	Errors     int64
	KeysCount  int64
	MemoryUsed int64
}

type RedisCache struct {
	client *redis.Client
	config *config.CacheConfig
	stats  *CacheStats
}

func NewRedisCache(cfg *config.CacheConfig, redisAddr string) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: cfg.RedisPassword,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisCache{
		client: client,
		config: cfg,
		stats:  &CacheStats{},
	}, nil
}

func (c *RedisCache) Get(ctx context.Context, key types.CacheKey) (*types.CacheEntry, error) {
	data, err := c.client.Get(ctx, string(key)).Bytes()
	if err == redis.Nil {
		c.stats.Misses++
		return nil, ErrCacheMiss
	}
	if err != nil {
		c.stats.Errors++
		return nil, err
	}

	var entry types.CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		c.stats.Errors++
		return nil, err
	}

	now := time.Now().Unix()
	if entry.ExpiresAt > now {
		c.stats.Hits++
		entry.HitCount++
		return &entry, nil
	}

	if c.config.EnableStale && entry.StaleUntil > now {
		c.stats.StaleHits++
		entry.HitCount++
		return &entry, nil
	}

	c.stats.Misses++
	c.Delete(ctx, key)
	return nil, ErrCacheMiss
}

func (c *RedisCache) Set(ctx context.Context, entry *types.CacheEntry) error {
	entry.CreatedAt = time.Now().Unix()
	if entry.ExpiresAt == 0 {
		entry.ExpiresAt = entry.CreatedAt + int64(c.config.DefaultTTL)
	}
	if entry.StaleUntil == 0 && c.config.EnableStale {
		entry.StaleUntil = entry.ExpiresAt + int64(c.config.StaleWhileRevalidate)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	ttl := time.Duration(entry.ExpiresAt-time.Now().Unix()) * time.Second
	if ttl < 0 {
		ttl = time.Duration(c.config.DefaultTTL) * time.Second
	}

	if err := c.client.Set(ctx, string(entry.Key), data, ttl).Err(); err != nil {
		return err
	}

	if c.config.EnableTags {
		for _, tag := range entry.Tags {
			tagKey := "tag:" + tag + ":" + string(entry.Key)
			if err := c.client.Set(ctx, tagKey, string(entry.Key), ttl).Err(); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *RedisCache) Delete(ctx context.Context, key types.CacheKey) error {
	return c.client.Del(ctx, string(key)).Err()
}

func (c *RedisCache) InvalidateByTags(ctx context.Context, tags []string) error {
	for _, tag := range tags {
		pattern := "tag:" + tag + ":*"
		keys, err := c.client.Keys(ctx, pattern).Result()
		if err != nil {
			return err
		}
		for _, tagKey := range keys {
			contentKey, err := c.client.Get(ctx, tagKey).Result()
			if err == nil && contentKey != "" {
				if err := c.client.Del(ctx, contentKey).Err(); err != nil {
					return err
				}
			}
			if err := c.client.Del(ctx, tagKey).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *RedisCache) InvalidateByPattern(ctx context.Context, pattern string) error {
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}
	return nil
}

func (c *RedisCache) GetStats(ctx context.Context) (*CacheStats, error) {
	c.client.Info(ctx, "memory", "stats")
	return c.stats, nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

var ErrCacheMiss = &CacheError{Message: "cache miss"}

type CacheError struct {
	Message string
}

func (e *CacheError) Error() string {
	return e.Message
}
