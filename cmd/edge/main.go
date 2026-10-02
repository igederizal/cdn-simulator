package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/yourusername/cdn-simulator/internal/cache"
	"github.com/yourusername/cdn-simulator/internal/config"
	"github.com/yourusername/cdn-simulator/internal/metrics"
	"github.com/yourusername/cdn-simulator/pkg/types"
)

// Version information (set by goreleaser)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type EdgeServer struct {
	cache  cache.Cache
	config *config.Config
	logger *zap.Logger
	router *gin.Engine
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	metrics.Init(&cfg.Metrics)

	c, err := cache.NewRedisCache(&cfg.Cache, cfg.Edge.RedisAddr)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer c.Close()

	server := &EdgeServer{
		cache:  c,
		config: cfg,
		logger: logger,
	}

	server.setupRoutes()

	addr := ":" + strconv.Itoa(cfg.Edge.Port)
	logger.Info("Starting edge server",
		zap.String("addr", addr),
		zap.String("region", cfg.Edge.Region),
		zap.String("version", version),
		zap.String("commit", commit),
	)

	srv := &http.Server{
		Addr:    addr,
		Handler: server.router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	go server.reportMetrics()

	select {}
}

func (s *EdgeServer) setupRoutes() {
	s.router = gin.New()
	s.router.Use(gin.Recovery())
	s.router.Use(s.loggingMiddleware())
	s.router.Use(s.metricsMiddleware())

	s.router.GET("/health", s.healthCheck)
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	api := s.router.Group("/api/v1")
	{
		api.GET("/*path", s.handleRequest)
		api.HEAD("/*path", s.handleRequest)

		admin := api.Group("")
		admin.Use(s.requireAPIKey())
		{
			admin.POST("/purge", s.purgeCache)
			admin.POST("/purge/tags", s.purgeByTags)
			admin.POST("/warm", s.warmCache)
		}
	}
}

func (s *EdgeServer) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		s.logger.Info("Request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("region", s.config.Edge.Region),
		)
	}
}

func (s *EdgeServer) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()
		status := "success"
		if c.Writer.Status() >= 400 {
			status = "error"
		}
		metrics.RecordRequest(s.config.Edge.Region, status, c.Request.Method)
		metrics.RecordRequestDuration(s.config.Edge.Region, status, duration)
	}
}

func (s *EdgeServer) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"region":  s.config.Edge.Region,
		"edge_id": s.config.Edge.ID,
		"version": version,
	})
}

func (s *EdgeServer) handleRequest(c *gin.Context) {
	ctx := c.Request.Context()
	key := types.CacheKey(c.Request.URL.Path)

	entry, err := s.cache.Get(ctx, key)
	if err == nil {
		metrics.RecordCacheHit(s.config.Edge.Region, types.CacheStatusHit)
		s.serveFromCache(c, entry)
		return
	}

	if err == cache.ErrCacheMiss {
		metrics.RecordCacheMiss(s.config.Edge.Region)
		s.fetchFromOrigin(c, key)
		return
	}

	metrics.RecordCacheHit(s.config.Edge.Region, types.CacheStatusError)
	s.logger.Error("Cache error", zap.Error(err))
	c.Status(http.StatusInternalServerError)
}

func (s *EdgeServer) serveFromCache(c *gin.Context, entry *types.CacheEntry) {
	for k, v := range entry.Headers {
		c.Header(k, v)
	}
	c.Header("X-Cache", "HIT")
	c.Header("X-Cache-Region", s.config.Edge.Region)
	c.Data(entry.StatusCode, entry.Headers["Content-Type"], entry.Value)

	metrics.RecordBandwidth(s.config.Edge.Region, "egress", float64(len(entry.Value)))
}

func (s *EdgeServer) fetchFromOrigin(c *gin.Context, key types.CacheKey) {
	ctx := c.Request.Context()
	originURL, err := s.originURL(c.Request.URL.Path)
	if err != nil {
		s.logger.Warn("Rejected origin fetch", zap.Error(err))
		c.Status(http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(ctx, c.Request.Method, originURL, c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	req.Header = c.Request.Header.Clone()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.Error("Origin request failed", zap.Error(err))
		c.Status(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	entry := &types.CacheEntry{
		Key:        key,
		Value:      body,
		Headers:    headers,
		StatusCode: resp.StatusCode,
		Region:     s.config.Edge.Region,
	}

	if cacheable(resp.StatusCode, headers) {
		if err := s.cache.Set(ctx, entry); err != nil {
			s.logger.Warn("Failed to cache response", zap.Error(err))
		}
	}

	for k, v := range headers {
		c.Header(k, v)
	}
	c.Header("X-Cache", "MISS")
	c.Header("X-Cache-Region", s.config.Edge.Region)
	c.Data(resp.StatusCode, headers["Content-Type"], body)

	metrics.RecordBandwidth(s.config.Edge.Region, "egress", float64(len(body)))
	metrics.RecordOriginRequest("origin", "success")
}

func (s *EdgeServer) purgeCache(c *gin.Context) {
	var req struct {
		Keys []string `json:"keys" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	for _, k := range req.Keys {
		if err := s.cache.Delete(ctx, types.CacheKey(k)); err != nil {
			s.logger.Error("Purge failed", zap.String("key", k), zap.Error(err))
		}
	}

	metrics.RecordInvalidation(s.config.Edge.Region, "keys")
	c.JSON(http.StatusOK, gin.H{"purged": len(req.Keys)})
}

func (s *EdgeServer) purgeByTags(c *gin.Context) {
	var req struct {
		Tags []string `json:"tags" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	if err := s.cache.InvalidateByTags(ctx, req.Tags); err != nil {
		s.logger.Error("Tag purge failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	metrics.RecordInvalidation(s.config.Edge.Region, "tags")
	c.JSON(http.StatusOK, gin.H{"purged_tags": len(req.Tags)})
}

func (s *EdgeServer) warmCache(c *gin.Context) {
	var req struct {
		URLs []string `json:"urls" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	invalid := make([]string, 0)
	valid := make([]string, 0, len(req.URLs))
	for _, u := range req.URLs {
		if _, err := s.originURL(u); err != nil {
			invalid = append(invalid, u)
			continue
		}
		valid = append(valid, u)
	}
	if len(invalid) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid urls", "rejected": invalid})
		return
	}

	warmed := 0
	for _, u := range valid {
		key := types.CacheKey(u)
		if _, err := s.cache.Get(ctx, key); err == cache.ErrCacheMiss {
			go s.prefetch(u)
			warmed++
		}
	}

	c.JSON(http.StatusOK, gin.H{"warming": warmed})
}

func (s *EdgeServer) prefetch(url string) {
	ctx := context.Background()
	originURL, err := s.originURL(url)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", originURL, nil)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	entry := &types.CacheEntry{
		Key:        types.CacheKey(url),
		Value:      body,
		Headers:    headers,
		StatusCode: resp.StatusCode,
		Region:     s.config.Edge.Region,
	}

	if cacheable(resp.StatusCode, headers) {
		s.cache.Set(ctx, entry)
	}
}

func (s *EdgeServer) reportMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats, err := s.cache.GetStats(context.Background())
		if err == nil {
			metrics.SetCacheSize(s.config.Edge.Region, float64(stats.MemoryUsed))
		}
	}
}

func (s *EdgeServer) requireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		configured := s.config.Edge.AdminAPIKey
		if configured == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "admin API disabled: set EDGE_ADMIN_API_KEY",
			})
			return
		}
		provided := c.GetHeader("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(configured)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			return
		}
		c.Next()
	}
}

func (s *EdgeServer) originURL(path string) (string, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("path must start with a single /")
	}
	if strings.Contains(path, "@") || strings.Contains(path, "://") {
		return "", fmt.Errorf("path contains forbidden characters")
	}
	base, err := url.Parse(s.config.Origin.Backends[0])
	if err != nil {
		return "", fmt.Errorf("invalid origin backend: %w", err)
	}
	u, err := url.Parse(s.config.Origin.Backends[0] + path)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != base.Scheme || u.Host != base.Host {
		return "", fmt.Errorf("resolved host %q does not match origin", u.Host)
	}
	return u.String(), nil
}

func cacheable(statusCode int, headers map[string]string) bool {
	if statusCode < 200 || statusCode >= 400 {
		return false
	}
	if cc, ok := headers["Cache-Control"]; ok {
		return cc != "no-store" && cc != "private"
	}
	return statusCode == 200 || statusCode == 301 || statusCode == 302 || statusCode == 404
}