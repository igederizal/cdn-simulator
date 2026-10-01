package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/yourusername/cdn-simulator/internal/config"
	"github.com/yourusername/cdn-simulator/pkg/types"
)

func newTestCache(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)

	cfg := &config.CacheConfig{
		DefaultTTL:           300,
		MaxTTL:               86400,
		StaleWhileRevalidate: 60,
		EnableTags:           true,
		EnableStale:          true,
	}

	c, err := NewRedisCache(cfg, mr.Addr())
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	return c, mr
}

func TestSetAndGet(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	entry := &types.CacheEntry{
		Key:        "/api/v1/logo.png",
		Value:      []byte("hello"),
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "image/png"},
	}

	if err := c.Set(ctx, entry); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := c.Get(ctx, entry.Key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(got.Value) != "hello" {
		t.Fatalf("expected value 'hello', got %q", got.Value)
	}
	if got.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", got.StatusCode)
	}
	if got.HitCount != 1 {
		t.Fatalf("expected HitCount 1, got %d", got.HitCount)
	}
}

func TestGetMiss(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	_, err := c.Get(ctx, "/not/cached")
	if err != ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss, got %v", err)
	}
}

func TestExpiredKeyIsMiss(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	entry := &types.CacheEntry{Key: "/api/v1/x", Value: []byte("data"), StatusCode: 200}
	if err := c.Set(ctx, entry); err != nil {
		t.Fatal(err)
	}

	mr.FastForward(301 * time.Second)

	_, err := c.Get(ctx, "/api/v1/x")
	if err != ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss after expiry, got %v", err)
	}
}

func TestStaleWhileRevalidate(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	entry := &types.CacheEntry{
		Key:        "/api/v1/stale.png",
		Value:      []byte("stale-content"),
		StatusCode: 200,
		ExpiresAt:  time.Now().Unix() - 10,
	}
	if err := c.Set(ctx, entry); err != nil {
		t.Fatal(err)
	}

	got, err := c.Get(ctx, "/api/v1/stale.png")
	if err != nil {
		t.Fatalf("expected stale hit, got %v", err)
	}
	if string(got.Value) != "stale-content" {
		t.Fatalf("unexpected value %q", got.Value)
	}
}

func TestDelete(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	entry := &types.CacheEntry{Key: "/api/v1/y", Value: []byte("v"), StatusCode: 200}
	if err := c.Set(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, entry.Key); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Get(ctx, entry.Key); err != ErrCacheMiss {
		t.Fatalf("expected miss after delete, got %v", err)
	}
}

func TestInvalidateByTags(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	a := &types.CacheEntry{Key: "/p/1", Value: []byte("a"), StatusCode: 200, Tags: []string{"product:1", "home"}}
	b := &types.CacheEntry{Key: "/p/2", Value: []byte("b"), StatusCode: 200, Tags: []string{"product:2", "home"}}
	keep := &types.CacheEntry{Key: "/other", Value: []byte("k"), StatusCode: 200, Tags: []string{"about"}}

	for _, e := range []*types.CacheEntry{a, b, keep} {
		if err := c.Set(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.InvalidateByTags(ctx, []string{"product:1"}); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Get(ctx, "/p/1"); err != ErrCacheMiss {
		t.Fatalf("/p/1 should be purged, got %v", err)
	}
	if _, err := c.Get(ctx, "/p/2"); err != nil {
		t.Fatalf("/p/2 should still be cached, got %v", err)
	}

	if err := c.InvalidateByTags(ctx, []string{"home"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "/p/2"); err != ErrCacheMiss {
		t.Fatalf("/p/2 should be purged by 'home' tag, got %v", err)
	}
	if _, err := c.Get(ctx, "/other"); err != nil {
		t.Fatalf("/other should still be cached, got %v", err)
	}
}

func TestInvalidateByPattern(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	for _, k := range []string{"/assets/a.png", "/assets/b.png", "/api/v1/data"} {
		e := &types.CacheEntry{Key: types.CacheKey(k), Value: []byte("v"), StatusCode: 200}
		if err := c.Set(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.InvalidateByPattern(ctx, "/assets/*"); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Get(ctx, "/assets/a.png"); err != ErrCacheMiss {
		t.Fatalf("/assets/a.png should be purged, got %v", err)
	}
	if _, err := c.Get(ctx, "/assets/b.png"); err != ErrCacheMiss {
		t.Fatalf("/assets/b.png should be purged, got %v", err)
	}
	if _, err := c.Get(ctx, "/api/v1/data"); err != nil {
		t.Fatalf("/api/v1/data should still be cached, got %v", err)
	}
}

func TestHitStats(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	entry := &types.CacheEntry{Key: "/stats", Value: []byte("v"), StatusCode: 200}
	if err := c.Set(ctx, entry); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if _, err := c.Get(ctx, "/stats"); err != nil {
			t.Fatal(err)
		}
	}
	c.Get(ctx, "/missing")

	stats, err := c.GetStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Hits != 3 {
		t.Fatalf("expected 3 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
}