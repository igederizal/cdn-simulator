package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/yourusername/cdn-simulator/internal/config"
	"github.com/yourusername/cdn-simulator/internal/metrics"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	metrics.Init(&cfg.Metrics)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(loggingMiddleware(logger))
	router.Use(metricsMiddleware)

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "origin_id": cfg.Origin.ID})
	})

	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	router.NoRoute(func(c *gin.Context) {
		c.Header("Content-Type", "text/plain")
		c.Header("Cache-Control", "public, max-age=300")
		c.String(http.StatusOK, "Origin response for: "+c.Request.URL.Path+"\n")
	})

	addr := ":" + strconv.Itoa(cfg.Origin.Port)
	logger.Info("Starting origin server", zap.String("addr", addr))

	if err := router.Run(addr); err != nil {
		logger.Fatal("Server failed", zap.Error(err))
	}
}

func loggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("Origin request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}

func metricsMiddleware(c *gin.Context) {
	start := time.Now()
	c.Next()
	duration := time.Since(start).Seconds()
	status := "success"
	if c.Writer.Status() >= 400 {
		status = "error"
	}
	metrics.RecordOriginRequest("origin", status)
	metrics.RecordOriginLatency("origin", duration)
}