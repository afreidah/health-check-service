package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestMetricsRegistration verifies metrics are registered without panicking
func TestMetricsRegistration(t *testing.T) {
	// The init() function already ran when the package loaded
	// Just verify the metrics exist and aren't nil
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

// TestRequestsTotalIncrement verifies counter can be incremented
func TestRequestsTotalIncrement(t *testing.T) {
	// Get initial count
	before := testutil.ToFloat64(RequestsTotal.WithLabelValues("200"))

	// Increment the counter
	RequestsTotal.WithLabelValues("200").Inc()

	// Verify it increased
	after := testutil.ToFloat64(RequestsTotal.WithLabelValues("200"))

	if after <= before {
		t.Errorf("Counter did not increment: before=%f, after=%f", before, after)
	}
}

// TestServiceStatusSet verifies gauge can be set
func TestServiceStatusSet(t *testing.T) {
	// Set the gauge
	ServiceStatus.WithLabelValues("nginx", "active").Set(1)

	// Read it back
	value := testutil.ToFloat64(ServiceStatus.WithLabelValues("nginx", "active"))

	if value != 1 {
		t.Errorf("Expected gauge value 1, got %f", value)
	}

	// Change it
	ServiceStatus.WithLabelValues("nginx", "active").Set(0)
	value = testutil.ToFloat64(ServiceStatus.WithLabelValues("nginx", "active"))

	if value != 0 {
		t.Errorf("Expected gauge value 0, got %f", value)
	}
}

// TestRequestDurationObserve verifies histogram accepts observations
func TestRequestDurationObserve(t *testing.T) {
	// Get count before
	before := testutil.ToFloat64(RequestDuration)

	// Record some observations
	RequestDuration.Observe(0.1)
	RequestDuration.Observe(0.5)
	RequestDuration.Observe(1.0)

	// Verify count increased (histogram stores count internally)
	after := testutil.ToFloat64(RequestDuration)

	// The sum should have increased
	if after <= before {
		t.Errorf("Histogram did not record observations: before=%f, after=%f", before, after)
	}
}

// TestMultipleLabelValues verifies metrics work with different labels
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
			// Should not panic with different label values
			RequestsTotal.WithLabelValues(tt.statusCode).Inc()

			value := testutil.ToFloat64(RequestsTotal.WithLabelValues(tt.statusCode))
			if value <= 0 {
				t.Errorf("Counter for status %s should be > 0", tt.statusCode)
			}
		})
	}
}

// TestServiceStatusMultipleServices verifies gauge works for different services
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
			ServiceStatus.WithLabelValues(svc.name, svc.state).Set(svc.value)

			got := testutil.ToFloat64(ServiceStatus.WithLabelValues(svc.name, svc.state))
			if got != svc.value {
				t.Errorf("Service %s: expected %f, got %f", svc.name, svc.value, got)
			}
		})
	}
}
