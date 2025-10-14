package checker

import (
	"net/http"
	"testing"
)

// TestStateToStatusCodeMapping verifies our state mapping is correct
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

// TestStateConstants verifies constants exist and aren't empty
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
