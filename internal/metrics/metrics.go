// -----------------------------------------------------------------------------
// Prometheus Metrics
// -----------------------------------------------------------------------------
//
// This package defines and registers Prometheus metrics for monitoring the
// health check service. Metrics are exposed at the /metrics endpoint and can
// be scraped by Prometheus servers for monitoring, alerting, and dashboards.
//
// Metrics Philosophy:
//   - RequestsTotal: Track request volume and error rates
//   - ServiceStatus: Monitor actual service health (the core business metric)
//   - RequestDuration: Identify performance issues and latency spikes
//   - CacheStaleness: Detect if checker is stuck/unresponsive
//   - CheckerHealthy: Direct signal if background checker is alive
//
// These metrics enable:
//   - Alerting on service downtime (ServiceStatus)
//   - Detecting health checker problems (RequestsTotal 500s, CacheStaleness)
//   - Monitoring health check performance (RequestDuration)
//   - Capacity planning (request volume trends)
//   - Watchdog detection (CheckerHealthy, CacheStaleness)
//
// Metric Types:
//   Counter   - Monotonically increasing value (requests, errors)
//   Gauge     - Value that can go up or down (current status, staleness)
//   Histogram - Distribution of values (latency, duration)
//
// Alert Examples:
//   - health_checker_healthy == 0 (checker not responding)
//   - health_check_cache_staleness_seconds > 60 (cache old)
//   - monitored_service_status == 0 (service down)
//   - rate(health_check_failures_total[5m]) > 0.1 (high error rate)
//
// =============================================================================

// Package metrics - Prometheus metrics exporting
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// =============================================================================
// Metric Definitions
// =============================================================================

// Prometheus metrics exported by this service.
// All metrics are registered in init() to be available at /metrics endpoint.
var (
	// RequestsTotal counts all health check requests by status code.
	// Labels: status_code (200, 503, 500)
	//
	// Use Case: Monitor request volume and error rates
	// Example Queries:
	//   - rate(health_check_requests_total[5m])           # Request rate
	//   - sum(health_check_requests_total)                # Total requests
	//   - sum(health_check_requests_total{status_code="500"})  # Error count
	//
	// Example Alert:
	//   - rate(health_check_requests_total{status_code="500"}[5m]) > 1
	//     "High error rate on health check endpoint"
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_requests_total",
			Help: "Total number of health check requests by HTTP status code",
		},
		[]string{"status_code"},
	)

	// ServiceStatus tracks the current health of the monitored service.
	// Labels: service (service name), state (systemd ActiveState)
	// Values: 1 = active/healthy, 0 = any other state
	//
	// Use Case: Primary alerting metric for service health
	// This is THE metric that operators care about.
	//
	// Example Queries:
	//   - monitored_service_status{service="nginx",state="active"}  # Is nginx healthy?
	//
	// Example Alert:
	//   - monitored_service_status{service="nginx",state="active"} == 0 for 2m
	//     "Nginx service is down"
	//
	// Example Dashboard:
	//   - Graph of monitored_service_status over time shows uptime visually
	ServiceStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "monitored_service_status",
			Help: "Status of the monitored systemd service (1=active, 0=not active)",
		},
		[]string{"service", "state"},
	)

	// RequestDuration measures how long each health check request takes.
	// Uses default Prometheus buckets for general-purpose latency tracking.
	//
	// Use Case: Identify slow health checks and performance degradation
	// Helps detect if D-Bus is responsive or if system is under load.
	//
	// Example Queries:
	//   - histogram_quantile(0.99, rate(health_check_request_duration_seconds_bucket[5m]))  # p99 latency
	//   - histogram_quantile(0.95, rate(health_check_request_duration_seconds_bucket[5m]))  # p95 latency
	//   - rate(health_check_request_duration_seconds_sum[5m]) / rate(health_check_request_duration_seconds_count[5m])  # avg latency
	//
	// Example Alert:
	//   - histogram_quantile(0.99, ...) > 1
	//     "Health checks are slow (p99 > 1s)"
	RequestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "health_check_request_duration_seconds",
			Help:    "Duration of health check requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	// CheckFailures counts failed health check attempts by error type.
	// Labels: service (service name), error_type (dbus_error, type_error)
	//
	// Use Case: Distinguish infrastructure failures from code issues
	//   - dbus_error: D-Bus not responding (infrastructure problem)
	//   - type_error: Unexpected data type from D-Bus (code problem)
	//
	// Example Queries:
	//   - rate(health_check_failures_total[5m])  # Failure rate
	//   - sum(health_check_failures_total{error_type="dbus_error"})  # D-Bus issues
	//   - sum(health_check_failures_total{error_type="type_error"})  # Code issues
	//
	// Example Alert:
	//   - rate(health_check_failures_total{error_type="dbus_error"}[5m]) > 0.1
	//     "D-Bus communication failing"
	CheckFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_failures_total",
			Help: "Total number of failed health checks by error type",
		},
		[]string{"service", "error_type"},
	)

	// =============================================================================
	// CHANGED: New metrics for cache staleness and checker health
	// =============================================================================

	// CacheStaleness measures how old the cached health data is.
	// Labels: service (service name)
	// Values: seconds since last successful check
	//
	// Use Case: Detect if the background checker is stuck/unresponsive
	// A large value means the checker hasn't updated the cache recently.
	//
	// Why separate from CheckFailures?
	//   - CheckFailures = explicit error when querying D-Bus
	//   - CacheStaleness = indirect signal that checker isn't running
	//   - A stuck goroutine won't produce CheckFailures!
	//
	// Example Queries:
	//   - health_check_cache_staleness_seconds{service="nginx"}  # How old is the data?
	//   - max(health_check_cache_staleness_seconds) > 60        # Any cache older than 60s?
	//
	// Example Alerts:
	//   - health_check_cache_staleness_seconds{service="nginx"} > 30 for 2m
	//     "Health check data is stale (> 30s old) for 2 minutes"
	//   - This catches when the checker goroutine is stuck/deadlocked
	//
	// Thresholds:
	//   - Green: < 15 seconds (one check interval)
	//   - Yellow: 15-60 seconds (checker fell behind)
	//   - Red: > 60 seconds (checker is stuck)
	CacheStaleness = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "health_check_cache_staleness_seconds",
			Help: "Age of cached health data in seconds (how long since last check)",
		},
		[]string{"service"},
	)

	// CheckerHealthy is a simple boolean metric: is the background checker running?
	// Labels: none
	// Values: 1 = checker responded recently, 0 = checker is stuck/unresponsive
	//
	// Use Case: Direct watchdog signal
	// This is set by the watchdog goroutine in app.go based on whether the
	// checker has called RecordSuccess() within the expected time window.
	//
	// This is the HIGHEST PRIORITY metric for detecting checker problems.
	// If this is 0, something is definitely wrong.
	//
	// Example Queries:
	//   - health_checker_healthy  # Is the checker alive?
	//
	// Example Alert:
	//   - health_checker_healthy == 0 for 1m
	//     "Background health checker is not responding"
	//   - This is a CRITICAL alert - the monitoring system itself is broken
	//
	// Comparison to CacheStaleness:
	//   - health_checker_healthy: set by watchdog, clearer intent
	//   - health_check_cache_staleness_seconds: measured in seconds, more granular
	//   - Use both: CheckerHealthy for alerting, Staleness for dashboards
	CheckerHealthy = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_checker_healthy",
			Help: "Whether the background checker goroutine is responding (1=yes, 0=stuck)",
		},
	)

	// CheckerLastCheckTimestamp is the Unix timestamp of the last successful check.
	// Labels: none
	// Values: Unix seconds since epoch
	//
	// Use Case: Manual investigation and debugging
	// Useful for:
	//   - Seeing WHEN the last check happened
	//   - Calculating staleness manually
	//   - Debugging timing issues
	//
	// Example Queries:
	//   - time() - health_checker_last_check_timestamp_seconds  # Staleness in seconds
	//   - health_checker_last_check_timestamp_seconds            # When was it?
	//
	// Example Alert:
	//   - time() - health_checker_last_check_timestamp_seconds > 60
	//     "Last health check was more than 60 seconds ago"
	CheckerLastCheckTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_checker_last_check_timestamp_seconds",
			Help: "Unix timestamp of the last successful health check",
		},
	)

	// =============================================================================
	// End new metrics
	// =============================================================================
)

// =============================================================================
// Metric Registration
// =============================================================================

// init automatically registers all metrics with Prometheus default registry.
// This function runs before main(), ensuring metrics are available immediately
// when the /metrics endpoint starts serving.
//
// Registration:
//   - MustRegister panics if metrics can't be registered (fail fast)
//   - This ensures we catch metric definition errors at startup
//   - Better to crash at startup than serve broken metrics
//
// If we have duplicate metric names or other registration problems,
// the service won't start - which is the right behavior.
func init() {
	prometheus.MustRegister(RequestsTotal)
	prometheus.MustRegister(ServiceStatus)
	prometheus.MustRegister(RequestDuration)
	prometheus.MustRegister(CheckFailures)

	// CHANGED: Register new metrics
	prometheus.MustRegister(CacheStaleness)
	prometheus.MustRegister(CheckerHealthy)
	prometheus.MustRegister(CheckerLastCheckTimestamp)
}
