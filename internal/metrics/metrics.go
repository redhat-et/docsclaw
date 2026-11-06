// Package metrics provides Prometheus metrics for the DocsClaw services.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestDuration tracks HTTP request latency
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "docsclaw",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"service", "method", "path", "status"},
	)

	// RequestsTotal counts total HTTP requests
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "docsclaw",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"service", "method", "path", "status"},
	)
)
