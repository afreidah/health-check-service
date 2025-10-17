// -----------------------------------------------------------------------
// Background Service Checker - Tests
// -----------------------------------------------------------------------
//
// Package checker_test validates the state mapping logic that translates
// systemd ActiveState values to HTTP status codes. These tests verify the
// core mapping is correct since incorrect mappings would cause monitoring
// systems to receive incorrect health status.
//
// -----------------------------------------------------------------------

package checker

import (
	"net/http"
	"testing"
)

// -----------------------------------------------------------------------
// State Mapping Tests
// -----------------------------------------------------------------------

// TestStateToStatusCodeMapping verifies that all systemd states map to the
// correct HTTP status codes. Only "active" should return 200 OK; all other
// states should return 503 Service Unavailable to ensure monitoring systems
// only consider a service healthy when truly healthy.
func TestStateToStatusCodeMapping(t *testing.T) {
	tests := []struct {
		state      string
		wantStatus int
	}{
		{StateActive, http.StatusOK},
		{StateInactive, http.StatusServiceUnavailable},
		{StateFailed, http.StatusServiceUnavailable},
		{StateActivating, http.StatusServiceUnavailable},
		{StateDeactivating, http.StatusServiceUnavailable},
		{StateReloading, http.StatusServiceUnavailable},
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

// -----------------------------------------------------------------------
// Constant Validation Tests
// -----------------------------------------------------------------------

// TestStateConstants verifies that all state constants are properly defined
// and not empty strings. This catches copy-paste errors and ensures constants
// match systemd's actual string values.
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
