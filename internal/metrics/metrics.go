// Package metrics provides Prometheus metrics for the SPIFFE demo services.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SVID metrics

	// SVIDExpirationSeconds tracks the time remaining until SVID expires
	SVIDExpirationSeconds = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "spiffe_demo",
			Name:      "svid_expiration_seconds",
			Help:      "Seconds until SVID expires",
		},
		[]string{"service", "spiffe_id"},
	)

	// SVIDRotations counts the number of SVID rotations
	SVIDRotations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "spiffe_demo",
			Name:      "svid_rotations_total",
			Help:      "Total number of SVID rotations",
		},
		[]string{"service", "spiffe_id"},
	)

	// Request metrics

	// RequestDuration tracks HTTP request latency
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "spiffe_demo",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"service", "method", "path", "status"},
	)

	// RequestsTotal counts total HTTP requests
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "spiffe_demo",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"service", "method", "path", "status"},
	)

	// Authorization metrics

	// AuthorizationDecisions counts OPA authorization decisions
	AuthorizationDecisions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "spiffe_demo",
			Name:      "authorization_decisions_total",
			Help:      "Total number of OPA authorization decisions",
		},
		[]string{"service", "decision", "caller_type"},
	)

	// AuthorizationDuration tracks OPA policy evaluation latency
	AuthorizationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "spiffe_demo",
			Name:      "authorization_duration_seconds",
			Help:      "OPA authorization evaluation duration in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
		[]string{"service"},
	)

	// Delegation metrics

	// DelegationsTotal counts delegation attempts
	DelegationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "spiffe_demo",
			Name:      "delegations_total",
			Help:      "Total number of delegation attempts",
		},
		[]string{"user", "agent", "result"},
	)
)
