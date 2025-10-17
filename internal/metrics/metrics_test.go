// -----------------------------------------------------------------------
// Prometheus Metrics - Tests
// -----------------------------------------------------------------------
//
// Package metrics_test validates Prometheus metrics definitions and verifies
// they function correctly. Broken metrics prevent monitoring and alerting,
// making these tests critical for production reliability.
//
// -----------------------------------------------------------------------

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// -----------------------------------------------------------------------
// Registration Tests
// -----------------------------------------------------------------------

// TestMetricsRegistration verifies all metrics were successfully registered
// during package initialization. If metrics fail to register (duplicate names,
// invalid configuration), the init() function panics at startup, preventing
// the service from starting.
func TestMetricsRegistration(t *testing.T) {
	// Verify metrics exist after init() completed
	if RequestsTotal == nil {
		t.Error("RequestsTotal metric is nil")
	}
	if ServiceStatus == nil {
		t.Error("ServiceStatus metric is nil")
	}
	if RequestDuration == nil {
		t.Error("RequestDuration metric is nil")
	}
}

// -----------------------------------------------------------------------
// Counter Tests
// -----------------------------------------------------------------------

// TestRequestsTotalIncrement verifies the counter increments correctly.
// Counters must be monotonically increasing; broken counters would prevent
// monitoring systems from calculating rates like requests per second.
func TestRequestsTotalIncrement(t *testing.T) {
	// Get baseline value (may not be zero due to other tests)
	before := testutil.ToFloat64(RequestsTotal.WithLabelValues("200"))

	// Increment the counter
	RequestsTotal.WithLabelValues("200").Inc()

	// Verify it increased by at least 1
	after := testutil.ToFloat64(RequestsTotal.WithLabelValues("200"))

	if after <= before {
		t.Errorf("Counter did not increment: before=%f, after=%f", before, after)
	}
}

// TestMultipleLabelValues verifies counters work with different labels.
// Each unique label combination creates a separate metric series in Prometheus.
func TestMultipleLabelValues(t *testing.T) {
	tests := []struct {
		statusCode string
		expected   bool
	}{
		{"200", true},
		{"503", true},
		{"500", true},
	}

	for _, tt := range tests {
		t.Run("status_"+tt.statusCode, func(t *testing.T) {
			// Increment counter with this label
			RequestsTotal.WithLabelValues(tt.statusCode).Inc()

			// Verify it's tracked separately
			value := testutil.ToFloat64(RequestsTotal.WithLabelValues(tt.statusCode))
			if value <= 0 {
				t.Errorf("Counter for status %s should be > 0", tt.statusCode)
			}
		})
	}
}

// -----------------------------------------------------------------------
// Gauge Tests
// -----------------------------------------------------------------------

// TestServiceStatusSet verifies the gauge can be set to arbitrary values.
// Unlike counters, gauges can increase or decrease. Broken gauges would fail
// to accurately reflect state changes (active -> inactive -> active) needed
// for alerting.
func TestServiceStatusSet(t *testing.T) {
	// Set gauge to 1 (service active)
	ServiceStatus.WithLabelValues("nginx", "active").Set(1)

	// Read it back
	value := testutil.ToFloat64(ServiceStatus.WithLabelValues("nginx", "active"))

	if value != 1 {
		t.Errorf("Expected gauge value 1, got %f", value)
	}

	// Change to 0 (service inactive)
	ServiceStatus.WithLabelValues("nginx", "active").Set(0)
	value = testutil.ToFloat64(ServiceStatus.WithLabelValues("nginx", "active"))

	if value != 0 {
		t.Errorf("Expected gauge value 0, got %f", value)
	}
}

// TestServiceStatusMultipleServices verifies the gauge tracks multiple
// services independently. Each service gets its own metric series.
func TestServiceStatusMultipleServices(t *testing.T) {
	services := []struct {
		name  string
		state string
		value float64
	}{
		{"nginx", "active", 1},
		{"postgresql", "inactive", 0},
		{"redis", "failed", 0},
	}

	for _, svc := range services {
		t.Run(svc.name, func(t *testing.T) {
			// Set gauge for this service
			ServiceStatus.WithLabelValues(svc.name, svc.state).Set(svc.value)

			// Verify it's tracked independently
			got := testutil.ToFloat64(ServiceStatus.WithLabelValues(svc.name, svc.state))
			if got != svc.value {
				t.Errorf("Service %s: expected %f, got %f", svc.name, svc.value, got)
			}
		})
	}
}

// -----------------------------------------------------------------------
// Histogram Tests
// -----------------------------------------------------------------------

// TestRequestDurationObserve verifies the histogram accepts observations.
// Histograms track the distribution of response times, enabling percentile
// calculations (p50, p95, p99) for SLA monitoring. Broken histograms would
// prevent latency analysis and performance detection.
func TestRequestDurationObserve(t *testing.T) {
	// Record some sample latencies (in seconds)
	RequestDuration.Observe(0.1) // 100ms - fast
	RequestDuration.Observe(0.5) // 500ms - moderate
	RequestDuration.Observe(1.0) // 1s - slow

	// Verify histogram is collecting observations
	count := testutil.CollectAndCount(RequestDuration)
	if count == 0 {
		t.Error("Histogram reported 0 metrics collected")
	}
}
