// -----------------------------------------------------------------------
// Prometheus Metrics
// -----------------------------------------------------------------------
//
// Package metrics provides Prometheus metrics for the health check service.
// All metrics are centrally defined and registered during package init to
// ensure single registration and prevent conflicts. Components throughout
// the application import this package to record observations.
//
// -----------------------------------------------------------------------

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// -----------------------------------------------------------------------
// Request and Service Health Metrics
// -----------------------------------------------------------------------

var (
	// RequestsTotal counts all health check requests by status code.
	// Enables tracking of request volume, error rates, and success rates.
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_requests_total",
			Help: "Total number of health check requests by HTTP status code",
		},
		[]string{"status_code"},
	)

	// ServiceStatus tracks the current health of the monitored systemd service.
	// Set to 1 when active, 0 for any other state. Primary metric for service
	// availability alerting and SLA calculations.
	//
	// Labels:
	//   - service: Name of the monitored systemd service (e.g., nginx, postgresql)
	//   - state: The current systemd ActiveState (active, inactive, failed, etc.)
	ServiceStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "monitored_service_status",
			Help: "Status of the monitored systemd service (1=active, 0=not active)",
		},
		[]string{"service", "state"},
	)

	// RequestDuration measures the latency of health check requests using a
	// histogram with Prometheus default buckets. Enables percentile calculations
	// (p50, p95, p99) for SLA monitoring and detects performance degradation.
	RequestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "health_check_request_duration_seconds",
			Help:    "Duration of health check requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
)

// -----------------------------------------------------------------------
// Checker Health and Failure Metrics
// -----------------------------------------------------------------------

var (
	// CheckFailures counts failed health check attempts by error category.
	// Distinguishes infrastructure failures (dbus_error) from code issues
	// (type_error).
	//
	// Labels:
	//   - service: Name of the monitored systemd service
	//   - error_type: Category of failure (dbus_error, type_error)
	CheckFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_failures_total",
			Help: "Total number of failed health checks by error type",
		},
		[]string{"service", "error_type"},
	)

	// CacheStaleness measures how old the cached health check data is in seconds.
	// Provides visibility into whether the background checker is still running.
	// Increases over time until checker updates cache; detects stuck goroutines
	// and deadlocks.
	//
	// Labels:
	//   - service: Name of the monitored systemd service
	CacheStaleness = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "health_check_cache_staleness_seconds",
			Help: "Age of cached health data in seconds (how long since last check)",
		},
		[]string{"service"},
	)

	// CheckerHealthy provides a simple boolean signal: is the checker responding?
	// Set by the watchdog goroutine that monitors checker responsiveness.
	// Set to 1 when checker has updated health information within the expected
	// interval, 0 when stuck, deadlocked, or crashed.
	CheckerHealthy = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_checker_healthy",
			Help: "Whether the background checker goroutine is responding (1=yes, 0=stuck)",
		},
	)

	// CheckerLastCheckTimestamp records the Unix timestamp of the most recent
	// successful health check. Useful for manual investigation, calculating
	// staleness, and detecting timing issues.
	CheckerLastCheckTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_checker_last_check_timestamp_seconds",
			Help: "Unix timestamp of the last successful health check",
		},
	)
)

// -----------------------------------------------------------------------
// Metric Registration
// -----------------------------------------------------------------------

// init registers all metrics with the Prometheus default registry during
// package initialization. MustRegister panics if a metric cannot be registered,
// which ensures metric registration errors are caught at startup rather than
// silently failing to export metrics.
func init() {
	prometheus.MustRegister(RequestsTotal)
	prometheus.MustRegister(ServiceStatus)
	prometheus.MustRegister(RequestDuration)
	prometheus.MustRegister(CheckFailures)
	prometheus.MustRegister(CacheStaleness)
	prometheus.MustRegister(CheckerHealthy)
	prometheus.MustRegister(CheckerLastCheckTimestamp)
}
