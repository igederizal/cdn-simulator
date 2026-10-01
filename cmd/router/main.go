package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/yourusername/cdn-simulator/internal/config"
	"github.com/yourusername/cdn-simulator/internal/metrics"
	"github.com/yourusername/cdn-simulator/internal/routing"
	"github.com/yourusername/cdn-simulator/pkg/types"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	metrics.Init(&cfg.Metrics)

	router := routing.NewLatencyRouter(&cfg.Router)

	for _, spec := range cfg.Router.Edges {
		region, addr := spec, spec
		if i := strings.Index(spec, "="); i >= 0 {
			region, addr = spec[:i], spec[i+1:]
		}
		router.RegisterEdge(&types.EdgeNode{
			ID:       addr,
			Region:   region,
			Address:  addr,
			Healthy:  true,
			Capacity: 10000,
		})
	}

	for _, backend := range cfg.Origin.Backends {
		u, _ := url.Parse(backend)
		router.RegisterOrigin(&types.OriginServer{
			ID:      u.Host,
			Address: u.Host,
			Healthy: true,
			Weight:  1,
		})
	}

	router.StartHealthChecks(time.Duration(cfg.Router.HealthCheckInterval) * time.Second)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggingMiddleware(logger))
	r.Use(metricsMiddleware)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.NoRoute(func(c *gin.Context) {
		req := &types.Request{
			Method:  c.Request.Method,
			Path:    c.Request.URL.Path,
			Headers: headersToMap(c.Request.Header),
		}

		edge, err := router.SelectEdge(req)
		if err != nil {
			logger.Error("No healthy edge", zap.Error(err))
			c.Status(http.StatusServiceUnavailable)
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(&url.URL{
			Scheme: "http",
			Host:   edge.Address,
		})

		proxy.Director = func(req *http.Request) {
			req.Header = c.Request.Header.Clone()
			req.Host = edge.Address
			req.URL.Scheme = "http"
			req.URL.Host = edge.Address
			req.URL.Path = c.Request.URL.Path
			req.URL.RawQuery = c.Request.URL.RawQuery
		}

		proxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Set("X-CDN-Edge", edge.ID)
			resp.Header.Set("X-CDN-Region", edge.Region)
			return nil
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("Proxy error", zap.Error(err))
			w.WriteHeader(http.StatusBadGateway)
		}

		start := time.Now()
		proxy.ServeHTTP(c.Writer, c.Request)
		duration := time.Since(start).Seconds()

		metrics.RecordRequest(edge.Region, "proxied", c.Request.Method)
		metrics.RecordRequestDuration(edge.Region, "proxied", duration)
	})

	addr := ":" + strconv.Itoa(cfg.Router.Port)
	logger.Info("Starting router", zap.String("addr", addr))

	if err := r.Run(addr); err != nil {
		logger.Fatal("Router failed", zap.Error(err))
	}
}

func loggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("Router request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}

func metricsMiddleware(c *gin.Context) {
	c.Next()
}

func headersToMap(h http.Header) map[string]string {
	m := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			m[k] = v[0]
		}
	}
	return m
}