// Package metrics
package metrics

import "github.com/prometheus/client_golang/prometheus"

// Prometheus metrics
var (
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_requests_total",
			Help: "Total number of health check requests by HTTP status code",
		},
		[]string{"status_code"},
	)

	ServiceStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "monitored_service_status",
			Help: "Status of the monitored systemd service (1=active, 0=not active)",
		},
		[]string{"service", "state"},
	)

	RequestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "health_check_request_duration_seconds",
			Help:    "Duration of health check requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
)

// automatically runs before main()
func init() {
	// Register metrics with Prometheus
	prometheus.MustRegister(RequestsTotal)
	prometheus.MustRegister(ServiceStatus)
	prometheus.MustRegister(RequestDuration)
}
