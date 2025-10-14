package cache

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestNew verifies the constructor initializes cache correctly
func TestNew(t *testing.T) {
	c := New()

	if c == nil {
		t.Fatal("New() returned nil")
	}

	statusCode, state := c.GetStatus()

	if statusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected initial status %d, got %d", http.StatusServiceUnavailable, statusCode)
	}

	if state != "unknown" {
		t.Errorf("Expected initial state 'unknown', got '%s'", state)
	}
}

// TestUpdateAndGetStatus verifies basic update/get operations
func TestUpdateAndGetStatus(t *testing.T) {
	c := New()

	// Update the status
	c.UpdateStatus(http.StatusOK, "active")

	// Get it back
	statusCode, state := c.GetStatus()

	if statusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, statusCode)
	}

	if state != "active" {
		t.Errorf("Expected state 'active', got '%s'", state)
	}
}

// TestUpdateStatusSetsLastChecked verifies lastChecked is updated
func TestUpdateStatusSetsLastChecked(t *testing.T) {
	c := New()

	before := time.Now()
	time.Sleep(10 * time.Millisecond) // Small delay to ensure time difference

	c.UpdateStatus(http.StatusOK, "active")

	// Access lastChecked directly since it's in the same package
	c.mu.RLock()
	lastChecked := c.lastChecked
	c.mu.RUnlock()

	if lastChecked.Before(before) {
		t.Errorf("lastChecked should be updated to recent time")
	}
}

// TestMultipleUpdates verifies cache can be updated multiple times
func TestMultipleUpdates(t *testing.T) {
	c := New()

	// Table of test cases
	tests := []struct {
		name       string
		statusCode int
		state      string
	}{
		{"first update", http.StatusOK, "active"},
		{"second update", http.StatusServiceUnavailable, "inactive"},
		{"third update", http.StatusInternalServerError, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.UpdateStatus(tt.statusCode, tt.state)

			gotCode, gotState := c.GetStatus()

			if gotCode != tt.statusCode {
				t.Errorf("Expected status %d, got %d", tt.statusCode, gotCode)
			}

			if gotState != tt.state {
				t.Errorf("Expected state '%s', got '%s'", tt.state, gotState)
			}
		})
	}
}

// TestConcurrentAccess verifies thread safety with concurrent reads/writes
func TestConcurrentAccess(t *testing.T) {
	c := New()

	// Number of concurrent operations
	numReaders := 50
	numWriters := 10

	var wg sync.WaitGroup

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.GetStatus() // Just read repeatedly
			}
		}()
	}

	// Start writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// Alternate between two states
				if j%2 == 0 {
					c.UpdateStatus(http.StatusOK, "active")
				} else {
					c.UpdateStatus(http.StatusServiceUnavailable, "inactive")
				}
			}
		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Verify cache is still in a valid state
	statusCode, state := c.GetStatus()

	// Should be one of our written states
	validStates := map[string]bool{"active": true, "inactive": true}
	if !validStates[state] {
		t.Errorf("After concurrent access, got unexpected state: %s", state)
	}

	validCodes := map[int]bool{http.StatusOK: true, http.StatusServiceUnavailable: true}
	if !validCodes[statusCode] {
		t.Errorf("After concurrent access, got unexpected code: %d", statusCode)
	}
}

// TestGetStatusDoesNotModifyCache verifies reads don't change state
func TestGetStatusDoesNotModifyCache(t *testing.T) {
	c := New()
	c.UpdateStatus(http.StatusOK, "active")

	// Get status multiple times
	for i := 0; i < 10; i++ {
		statusCode, state := c.GetStatus()

		if statusCode != http.StatusOK {
			t.Errorf("GetStatus modified statusCode on call %d", i)
		}

		if state != "active" {
			t.Errorf("GetStatus modified state on call %d", i)
		}
	}
}
