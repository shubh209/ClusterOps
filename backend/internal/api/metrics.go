package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics exposed on /metrics.
var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "clusterops",
		Subsystem: "api",
		Name:      "requests_total",
		Help:      "Total HTTP requests by method, path, and status code.",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "clusterops",
		Subsystem: "api",
		Name:      "request_duration_seconds",
		Help:      "HTTP request latency.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path"})

	cacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "clusterops",
		Subsystem: "cache",
		Name:      "hits_total",
		Help:      "Redis cache hits by resource type.",
	}, []string{"resource"})

	cacheMissesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "clusterops",
		Subsystem: "cache",
		Name:      "misses_total",
		Help:      "Redis cache misses by resource type.",
	}, []string{"resource"})

	sseClientsGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "clusterops",
		Subsystem: "sse",
		Name:      "connected_clients",
		Help:      "Number of currently connected SSE clients.",
	})
)

// recordCacheHit increments the hit counter for the given resource.
func recordCacheHit(resource string) {
	cacheHitsTotal.WithLabelValues(resource).Inc()
}

// recordCacheMiss increments the miss counter for the given resource.
func recordCacheMiss(resource string) {
	cacheMissesTotal.WithLabelValues(resource).Inc()
}
