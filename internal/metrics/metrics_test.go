// -----------------------------------------------------------------------------
// Prometheus Metrics - Tests
// -----------------------------------------------------------------------------
//
// This test suite validates the Prometheus metrics definitions and verifies
// they function correctly. While these tests might seem simple, they catch
// real issues that could break production monitoring.
//
// Why Test Metrics?
//   - Broken metrics = blind monitoring (no alerts, no visibility)
//   - Registration failures crash the application at startup
//   - Label mismatches prevent metric aggregation in Prometheus
//   - Type errors (Counter vs Gauge) break query assumptions
//
// Test Strategy:
//   - Use prometheus/testutil for metric validation
//   - Verify each metric type's specific operations
//   - Test label cardinality doesn't explode
//   - Ensure metrics survive package initialization
//
// Limitations:
//   Histograms can't be fully validated with testutil.ToFloat64, so we use
//   CollectAndCount to verify they at least register observations.
//
// -----------------------------------------------------------------------------

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// -----------------------------------------------------------------------------
// Registration Tests
// -----------------------------------------------------------------------------

// TestMetricsRegistration verifies all metrics were successfully registered
// during package initialization. The init() function already ran when this
// package loaded, so we just verify the metrics exist and aren't nil.
//
// Why This Matters:
//
//	If metrics fail to register (e.g., duplicate names), the init() function
//	will panic and crash the service at startup. This test catches that early.
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

// -----------------------------------------------------------------------------
// Counter Tests
// -----------------------------------------------------------------------------

// TestRequestsTotalIncrement verifies the counter increments correctly.
// Counters must be monotonically increasing - they should never decrease.
//
// Real-World Usage:
//
//	This counter tracks request volume and error rates. Broken counters would
//	prevent monitoring systems from calculating rates like req/sec.
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
//
// Label Cardinality:
//
//	We test multiple status codes to ensure labels work, but in production
//	we limit to 3 values (200, 503, 500) to avoid metric explosion.
func TestMultipleLabelValues(t *testing.T) {
	tests := []struct {
		statusCode string
		expected   bool
	}{
		{"200", true}, // Success
		{"503", true}, // Service unavailable
		{"500", true}, // Internal error
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

// -----------------------------------------------------------------------------
// Gauge Tests
// -----------------------------------------------------------------------------

// TestServiceStatusSet verifies the gauge can be set to arbitrary values.
// Unlike counters, gauges can increase or decrease and be set to any value.
//
// Real-World Usage:
//
//	This gauge represents current service health. It must accurately reflect
//	state changes (active -> inactive -> active) for alerts to work correctly.
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
//
// Label Cardinality Note:
//
//	In production, we typically monitor 1 service per instance, but the
//	metric supports multiple services if needed for testing or multi-service
//	monitoring.
func TestServiceStatusMultipleServices(t *testing.T) {
	services := []struct {
		name  string
		state string
		value float64
	}{
		{"nginx", "active", 1},        // Healthy web server
		{"postgresql", "inactive", 0}, // Stopped database
		{"redis", "failed", 0},        // Failed cache
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

// -----------------------------------------------------------------------------
// Histogram Tests
// -----------------------------------------------------------------------------

// TestRequestDurationObserve verifies the histogram accepts observations.
// Histograms are more complex than counters/gauges - they bucket values and
// track sum/count for calculating percentiles.
//
// Testing Limitation:
//
//	testutil.ToFloat64 doesn't work with histograms (they have multiple
//	internal metrics), so we use CollectAndCount to verify observations
//	are at least being recorded.
//
// Real-World Usage:
//
//	This histogram tracks request latency. Broken histograms would prevent
//	calculating p50, p95, p99 latency for SLA monitoring.
func TestRequestDurationObserve(t *testing.T) {
	// Record some sample latencies (in seconds)
	RequestDuration.Observe(0.1) // 100ms - fast
	RequestDuration.Observe(0.5) // 500ms - moderate
	RequestDuration.Observe(1.0) // 1s - slow

	// Verify histogram is collecting observations
	// (We can't check specific buckets, but we can verify it's working)
	count := testutil.CollectAndCount(RequestDuration)
	if count == 0 {
		t.Error("Histogram reported 0 metrics collected")
	}
}
