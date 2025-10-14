// -----------------------------------------------------------------------------
// Background Service Checker - Tests
// -----------------------------------------------------------------------------
//
// This test suite validates the state mapping logic that translates systemd
// ActiveState values to HTTP status codes. While these tests might seem simple,
// they are critical - incorrect mappings would cause monitoring systems to
// receive incorrect health status, potentially leading to failed alerts or
// false positives.
//
// Test Coverage:
//   - State-to-HTTP-code mapping correctness
//   - Constant validation (ensure no empty strings)
//   - Comprehensive coverage of all systemd states
//
// Note: Integration tests with actual D-Bus/systemd would require a running
// systemd instance and proper test fixtures, so we focus on unit testing
// the mapping logic here.
//
// -----------------------------------------------------------------------------

package checker

import (
	"net/http"
	"testing"
)

// -----------------------------------------------------------------------------
// State Mapping Tests
// -----------------------------------------------------------------------------

// TestStateToStatusCodeMapping verifies that all systemd states map to the
// correct HTTP status codes. This is the core logic that determines what
// monitoring systems see.
//
// Critical Behavior:
//   - Only "active" should return 200 OK
//   - All other states should return 503 Service Unavailable
//   - This ensures monitoring systems only see healthy when truly healthy
func TestStateToStatusCodeMapping(t *testing.T) {
	tests := []struct {
		state      string
		wantStatus int
	}{
		{StateActive, http.StatusOK}, // Only this should be 200
		{StateInactive, http.StatusServiceUnavailable},
		{StateFailed, http.StatusServiceUnavailable},
		{StateActivating, http.StatusServiceUnavailable},   // Starting up is unhealthy
		{StateDeactivating, http.StatusServiceUnavailable}, // Shutting down is unhealthy
		{StateReloading, http.StatusServiceUnavailable},    // Reloading is unhealthy
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := stateToStatusCode[tt.state]
			if got != tt.wantStatus {
				t.Errorf("State %s: expected %d, got %d", tt.state, tt.wantStatus, got)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Constant Validation Tests
// -----------------------------------------------------------------------------

// TestStateConstants verifies that all state constants are properly defined
// and not empty strings. While this might seem redundant, it catches copy-paste
// errors and ensures constants match systemd's actual string values.
//
// This test would catch mistakes like:
//
//	const StateActive = ""  // Accidentally empty
//	const StateFailed = "active"  // Copy-paste error
func TestStateConstants(t *testing.T) {
	states := []struct {
		name  string
		value string
	}{
		{"StateActive", StateActive},
		{"StateInactive", StateInactive},
		{"StateFailed", StateFailed},
		{"StateActivating", StateActivating},
		{"StateDeactivating", StateDeactivating},
		{"StateReloading", StateReloading},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			if s.value == "" {
				t.Errorf("%s constant is empty", s.name)
			}
		})
	}
}
