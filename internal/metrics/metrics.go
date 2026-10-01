package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/yourusername/cdn-simulator/internal/config"
)

var (
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cdn_requests_total",
		Help: "Total number of requests processed",
	}, []string{"region", "status", "method"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "cdn_request_duration_seconds",
		Help:    "Request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"region", "status"})

	CacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cdn_cache_hits_total",
		Help: "Total number of cache hits",
	}, []string{"region", "type"})

	CacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cdn_cache_misses_total",
		Help: "Total number of cache misses",
	}, []string{"region"})

	OriginRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cdn_origin_requests_total",
		Help: "Total number of requests to origin",
	}, []string{"origin", "status"})

	OriginLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "cdn_origin_latency_seconds",
		Help:    "Origin request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"origin"})

	ActiveConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cdn_active_connections",
		Help: "Number of active connections",
	}, []string{"region"})

	EdgeLoad = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cdn_edge_load",
		Help: "Current load on edge node",
	}, []string{"edge_id", "region"})

	CacheSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cdn_cache_size_bytes",
		Help: "Current cache size in bytes",
	}, []string{"region"})

	InvalidationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cdn_invalidations_total",
		Help: "Total number of cache invalidations",
	}, []string{"region", "type"})

	BandwidthBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cdn_bandwidth_bytes_total",
		Help: "Total bandwidth served in bytes",
	}, []string{"region", "direction"})
)

func Init(cfg *config.MetricsConfig) {
	_ = cfg
}

func RecordRequest(region, status, method string) {
	RequestsTotal.WithLabelValues(region, status, method).Inc()
}

func RecordRequestDuration(region, status string, duration float64) {
	RequestDuration.WithLabelValues(region, status).Observe(duration)
}

func RecordCacheHit(region, hitType string) {
	CacheHits.WithLabelValues(region, hitType).Inc()
}

func RecordCacheMiss(region string) {
	CacheMisses.WithLabelValues(region).Inc()
}

func RecordOriginRequest(origin, status string) {
	OriginRequests.WithLabelValues(origin, status).Inc()
}

func RecordOriginLatency(origin string, duration float64) {
	OriginLatency.WithLabelValues(origin).Observe(duration)
}

func SetActiveConnections(region string, count float64) {
	ActiveConnections.WithLabelValues(region).Set(count)
}

func SetEdgeLoad(edgeID, region string, load float64) {
	EdgeLoad.WithLabelValues(edgeID, region).Set(load)
}

func SetCacheSize(region string, size float64) {
	CacheSize.WithLabelValues(region).Set(size)
}

func RecordInvalidation(region, invType string) {
	InvalidationsTotal.WithLabelValues(region, invType).Inc()
}

func RecordBandwidth(region, direction string, bytes float64) {
	BandwidthBytes.WithLabelValues(region, direction).Add(bytes)
}