// Package metrics provides Prometheus metrics for the Health Check Service.
//
// This package defines and registers all Prometheus metrics exported by the service.
// Metrics are exposed at the /metrics endpoint and can be scraped by Prometheus servers
// for monitoring, alerting, and dashboards.
//
// Metrics are centrally defined here to ensure single registration and prevent
// conflicts. All components throughout the application import this package to
// record observations and maintain consistency.
//
// Metric Philosophy:
// The selected metrics focus on operational observability for the monitoring system:
// request volume and error rates for service visibility, actual service health as
// the primary business metric, request latency for performance monitoring, cache
// staleness to detect when the checker stops responding, and explicit checker
// health signals for watchdog alerting.
//
// These metrics enable:
//   - Alerting on service downtime
//   - Detecting health checker problems
//   - Monitoring health check performance
//   - Capacity planning and trend analysis
//   - Watchdog detection of stuck goroutines
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Request and Service Health Metrics
//
// These metrics track the primary concerns of a monitoring service: how many
// requests are being served, what is the health of the monitored service, and
// how quickly is the service responding to requests.

var (
	// RequestsTotal counts all health check requests by status code.
	// This metric enables tracking of request volume, error rates, and success rates.
	//
	// Labels:
	//   - status_code: HTTP status code returned (200, 503, 500)
	//
	// Use Cases:
	//   - Monitor request volume and trends over time
	//   - Calculate error rate: errors / total requests
	//   - Detect anomalies in request patterns
	//
	// Example Prometheus Queries:
	//   - rate(health_check_requests_total[5m]) -- Request rate per second
	//   - sum(health_check_requests_total) -- Total requests since startup
	//   - sum(health_check_requests_total{status_code="500"}) -- Total errors
	//   - rate(health_check_requests_total{status_code="500"}[5m]) -- Error rate
	//
	// Example Alert:
	//   - alert: HighHealthCheckErrorRate
	//     expr: rate(health_check_requests_total{status_code="500"}[5m]) > 1
	//     annotations:
	//       summary: "High error rate on health check endpoint"
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_requests_total",
			Help: "Total number of health check requests by HTTP status code",
		},
		[]string{"status_code"},
	)

	// ServiceStatus tracks the current health of the monitored systemd service.
	// This is the primary metric that operators care about for alerting.
	//
	// Labels:
	//   - service: Name of the monitored systemd service (e.g., nginx, postgresql)
	//   - state: The current systemd ActiveState (active, inactive, failed, etc.)
	//
	// Values:
	//   - 1: Service is active and healthy (running normally)
	//   - 0: Service is in any other state (down, transitioning, failed)
	//
	// Use Cases:
	//   - Primary alerting metric for service availability
	//   - Dashboard visualization of service health over time
	//   - SLA calculations and uptime percentage tracking
	//
	// Example Prometheus Queries:
	//   - monitored_service_status{service="nginx",state="active"} -- Is nginx up?
	//   - avg_over_time(monitored_service_status[5m]) -- 5-minute average
	//   - 100 * avg_over_time(...) -- Uptime percentage
	//
	// Example Alert:
	//   - alert: ServiceDown
	//     expr: monitored_service_status{service="nginx",state="active"} == 0 for 2m
	//     annotations:
	//       summary: "{{ $labels.service }} service is down"
	//
	// Example Dashboard:
	//   - Stacked area chart showing service status over time
	//   - Green periods = running, red periods = down
	ServiceStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "monitored_service_status",
			Help: "Status of the monitored systemd service (1=active, 0=not active)",
		},
		[]string{"service", "state"},
	)

	// RequestDuration measures the latency of health check requests.
	// Uses a histogram to track the distribution of response times.
	//
	// Buckets:
	//   - Uses Prometheus default buckets for general-purpose latency tracking
	//   - Ranges from microseconds to several seconds
	//
	// Use Cases:
	//   - Detect performance degradation or D-Bus unresponsiveness
	//   - Calculate percentiles (p50, p95, p99) for SLA monitoring
	//   - Identify when system is under load
	//
	// Example Prometheus Queries:
	//   - histogram_quantile(0.99, rate(health_check_request_duration_seconds_bucket[5m]))
	//     -- 99th percentile latency
	//   - histogram_quantile(0.95, rate(...))
	//     -- 95th percentile latency
	//   - rate(health_check_request_duration_seconds_sum[5m]) /
	//     rate(health_check_request_duration_seconds_count[5m])
	//     -- Average latency
	//
	// Example Alert:
	//   - alert: SlowHealthChecks
	//     expr: histogram_quantile(0.99, rate(...)) > 1
	//     annotations:
	//       summary: "Health checks are slow (p99 > 1 second)"
	RequestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "health_check_request_duration_seconds",
			Help:    "Duration of health check requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
)

// Checker Health and Failure Metrics
//
// These metrics provide visibility into the health of the background checker
// goroutine and detect when it encounters problems querying systemd.

var (
	// CheckFailures counts failed health check attempts by error category.
	// Distinguishes infrastructure failures from code issues.
	//
	// Labels:
	//   - service: Name of the monitored systemd service
	//   - error_type: Category of failure
	//       - dbus_error: D-Bus connection or communication failure (infrastructure)
	//       - type_error: Unexpected data type from D-Bus (code issue)
	//
	// Use Cases:
	//   - Determine root cause of health check problems
	//   - Diagnose infrastructure (D-Bus) vs. code issues
	//   - Track error frequency and trends
	//
	// Example Prometheus Queries:
	//   - rate(health_check_failures_total[5m])
	//     -- Overall failure rate
	//   - sum(health_check_failures_total{error_type="dbus_error"})
	//     -- Total D-Bus failures
	//   - sum(health_check_failures_total{error_type="type_error"})
	//     -- Total code/type errors
	//
	// Example Alert:
	//   - alert: HighHealthCheckFailureRate
	//     expr: rate(health_check_failures_total{error_type="dbus_error"}[5m]) > 0.1
	//     annotations:
	//       summary: "High D-Bus communication failure rate"
	CheckFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_failures_total",
			Help: "Total number of failed health checks by error type",
		},
		[]string{"service", "error_type"},
	)

	// CacheStaleness measures how old the cached health check data is.
	// Provides visibility into whether the background checker is still running.
	//
	// Labels:
	//   - service: Name of the monitored systemd service
	//
	// Values:
	//   - Seconds since the last successful health check update
	//   - Increases over time until checker updates cache
	//
	// Why Separate from CheckFailures:
	//   - CheckFailures tracks explicit errors when querying D-Bus
	//   - CacheStaleness detects when checker stops running entirely
	//   - A stuck goroutine won't produce CheckFailures!
	//   - CacheStaleness catches silent failures and deadlocks
	//
	// Use Cases:
	//   - Detect if background checker has stopped responding
	//   - Identify when checker is stuck or deadlocked
	//   - Measure degradation of monitoring responsiveness
	//   - Dashboard indicator of data freshness
	//
	// Example Prometheus Queries:
	//   - health_check_cache_staleness_seconds{service="nginx"}
	//     -- How old is nginx data?
	//   - max(health_check_cache_staleness_seconds) > 60
	//     -- Any service with data older than 60 seconds?
	//
	// Example Alert:
	//   - alert: StaleHealthCheckData
	//     expr: health_check_cache_staleness_seconds > 60 for 2m
	//     annotations:
	//       summary: "Health check data is stale (>60s old) for {{ $labels.service }}"
	//       description: "The background checker may have stopped responding"
	//
	// Thresholds:
	//   - Green: < 15 seconds (checker is actively updating)
	//   - Yellow: 15-60 seconds (checker is running but delayed)
	//   - Red: > 60 seconds (checker is stuck or crashed)
	CacheStaleness = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "health_check_cache_staleness_seconds",
			Help: "Age of cached health data in seconds (how long since last check)",
		},
		[]string{"service"},
	)

	// CheckerHealthy provides a simple boolean signal: is the checker responding?
	// Set by the watchdog goroutine that monitors checker responsiveness.
	//
	// Values:
	//   - 1: Checker has updated health information within the expected interval
	//   - 0: Checker has not updated recently (stuck, deadlocked, or crashed)
	//
	// Use Cases:
	//   - Primary watchdog signal for detecting checker problems
	//   - Boolean metric suitable for simple alerting logic
	//   - Direct indicator of monitoring system health
	//   - Higher-level signal than CacheStaleness for alerting
	//
	// Example Prometheus Queries:
	//   - health_checker_healthy
	//     -- Is the checker alive? (1 = yes, 0 = no)
	//
	// Example Alert:
	//   - alert: HealthCheckerNotResponding
	//     expr: health_checker_healthy == 0 for 1m
	//     severity: critical
	//     annotations:
	//       summary: "Health checker is not responding"
	//       description: "The background monitoring goroutine has stopped updating"
	//
	// Comparison to CacheStaleness:
	//   - health_checker_healthy: Set by watchdog, boolean signal, intent-clear
	//   - health_check_cache_staleness_seconds: Measured in seconds, more granular
	//   - Use both: CheckerHealthy for alerting, Staleness for dashboards
	CheckerHealthy = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_checker_healthy",
			Help: "Whether the background checker goroutine is responding (1=yes, 0=stuck)",
		},
	)

	// CheckerLastCheckTimestamp records the Unix timestamp of the most recent
	// successful health check. Useful for debugging and manual investigation.
	//
	// Values:
	//   - Unix seconds since epoch of the last successful check
	//   - Advances when checker updates cache with new service status
	//
	// Use Cases:
	//   - Manual investigation and troubleshooting
	//   - Calculating staleness: current_time - last_check_timestamp
	//   - Debugging timing issues and checker behavior
	//   - Integration with alerting systems for custom calculations
	//
	// Example Prometheus Queries:
	//   - time() - health_checker_last_check_timestamp_seconds
	//     -- Staleness in seconds (equivalent to CacheStaleness)
	//   - health_checker_last_check_timestamp_seconds
	//     -- When was the last check? (as Unix timestamp)
	//   - timestamp(health_checker_last_check_timestamp_seconds)
	//     -- When was this metric last updated?
	//
	// Example Alert:
	//   - alert: StaleHealthChecks
	//     expr: (time() - health_checker_last_check_timestamp_seconds) > 60
	//     annotations:
	//       summary: "Last health check was {{ $value | humanizeDuration }} ago"
	CheckerLastCheckTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_checker_last_check_timestamp_seconds",
			Help: "Unix timestamp of the last successful health check",
		},
	)
)

// Metric Registration
//
// All metrics are registered with the Prometheus default registry during
// package initialization. Registration occurs automatically when the metrics
// package is imported, before main() runs.

func init() {
	// Register all metrics with the Prometheus default registry.
	// MustRegister panics if a metric cannot be registered, which is the
	// desired behavior - we want to catch metric registration errors at
	// startup rather than silently failing to export metrics.
	//
	// Duplicate metric names or other registration problems will cause a panic
	// at startup, which is appropriate since it indicates a code error that
	// must be fixed before proceeding.

	prometheus.MustRegister(RequestsTotal)
	prometheus.MustRegister(ServiceStatus)
	prometheus.MustRegister(RequestDuration)
	prometheus.MustRegister(CheckFailures)
	prometheus.MustRegister(CacheStaleness)
	prometheus.MustRegister(CheckerHealthy)
	prometheus.MustRegister(CheckerLastCheckTimestamp)
}
