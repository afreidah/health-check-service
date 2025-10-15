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
//
// These three metrics enable:
//   - Alerting on service downtime (ServiceStatus)
//   - Detecting health checker problems (RequestsTotal 500s)
//   - Monitoring health check performance (RequestDuration)
//   - Capacity planning (request volume trends)
//
// Metric Types:
//   Counter   - Monotonically increasing value (requests, errors)
//   Gauge     - Value that can go up or down (current status)
//   Histogram - Distribution of values (latency, duration)
//
// -----------------------------------------------------------------------------

package metrics

import "github.com/prometheus/client_golang/prometheus"

// -----------------------------------------------------------------------------
// Metric Definitions
// -----------------------------------------------------------------------------

// Prometheus metrics exported by this service.
// All metrics are registered in init() to be available at /metrics endpoint.
var (
	// RequestsTotal counts all health check requests by status code.
	// Labels: status_code (200, 503, 500)
	//
	// Use Case: Monitor request volume and error rates
	// Example Queries:
	//   - rate(health_check_requests_total[5m])  # Request rate
	//   - sum(health_check_requests_total{status_code="500"})  # Error count
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
	// Example Alert:
	//   - monitored_service_status{service="nginx"} == 0
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
	// Example Queries:
	//   - histogram_quantile(0.99, health_check_request_duration_seconds)  # p99 latency
	//   - rate(health_check_request_duration_seconds_sum[5m])  # Total time spent
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
	// Example Queries:
	//   - rate(health_check_failures_total[5m])  # Failure rate
	//   - sum(health_check_failures_total{error_type="dbus_error"})  # D-Bus issues
	CheckFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_failures_total",
			Help: "Total number of failed health checks by error type",
		},
		[]string{"service", "error_type"},
	)
)

// -----------------------------------------------------------------------------
// Metric Registration
// -----------------------------------------------------------------------------

// init automatically registers all metrics with Prometheus default registry.
// This function runs before main(), ensuring metrics are available immediately
// when the /metrics endpoint starts serving.
//
// Registration:
//   - MustRegister panics if metrics can't be registered (fail fast)
//   - This ensures we catch metric definition errors at startup
//   - Better to crash at startup than serve broken metrics
func init() {
	prometheus.MustRegister(RequestsTotal)
	prometheus.MustRegister(ServiceStatus)
	prometheus.MustRegister(RequestDuration)
	prometheus.MustRegister(CheckFailures)
}
