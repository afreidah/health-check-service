// -----------------------------------------------------------------------------
// Service Status Cache - Tests
// -----------------------------------------------------------------------------
//
// This test suite verifies the thread-safe behavior of the ServiceCache.
// Since the cache is accessed concurrently by multiple goroutines in
// production, these tests focus heavily on race conditions and concurrent
// access patterns.
//
// Test Coverage:
//   - Constructor initialization and default values
//   - Basic read/write operations
//   - Timestamp tracking on updates
//   - Concurrent access (50 readers + 10 writers)
//   - Read immutability (GetStatus doesn't modify state)
//
// Run with race detector: go test -race
//
// -----------------------------------------------------------------------------

package cache

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// Constructor Tests
// -----------------------------------------------------------------------------

// TestNew verifies the constructor initializes cache with safe defaults.
// The cache should start in an "unavailable" state until the first real
// health check completes.
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

// -----------------------------------------------------------------------------
// Basic Operations Tests
// -----------------------------------------------------------------------------

// TestUpdateAndGetStatus verifies the basic update/read cycle.
// This is the fundamental operation: checker writes, handler reads.
func TestUpdateAndGetStatus(t *testing.T) {
	c := New()

	// Simulate background checker updating status
	c.UpdateStatus(http.StatusOK, "active")

	// Simulate HTTP handler reading status
	statusCode, state := c.GetStatus()

	if statusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, statusCode)
	}

	if state != "active" {
		t.Errorf("Expected state 'active', got '%s'", state)
	}
}

// TestMultipleUpdates verifies cache handles repeated updates correctly.
// In production, the background checker updates the cache every N seconds,
// so this simulates that continuous update pattern.
func TestMultipleUpdates(t *testing.T) {
	c := New()

	// Table of test cases simulating different service states over time
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

// -----------------------------------------------------------------------------
// Timestamp Validation Tests
// -----------------------------------------------------------------------------

// TestUpdateStatusSetsLastChecked verifies that lastChecked timestamp
// is properly updated on each status change. This is important for
// future features like stale data detection.
func TestUpdateStatusSetsLastChecked(t *testing.T) {
	c := New()

	before := time.Now()
	time.Sleep(10 * time.Millisecond) // Small delay to ensure time difference

	c.UpdateStatus(http.StatusOK, "active")

	// Access lastChecked directly since we're in the same package
	c.mu.RLock()
	lastChecked := c.lastChecked
	c.mu.RUnlock()

	if lastChecked.Before(before) {
		t.Errorf("lastChecked should be updated to recent time")
	}
}

// -----------------------------------------------------------------------------
// Concurrency Tests
// -----------------------------------------------------------------------------

// TestConcurrentAccess is the most important test - it verifies thread safety
// under high concurrent load. This simulates production where:
//   - 1 background checker writes periodically
//   - Many HTTP handlers read simultaneously
//
// Run with: go test -race to detect race conditions
func TestConcurrentAccess(t *testing.T) {
	c := New()

	// Simulate realistic production load
	numReaders := 50 // Multiple HTTP request handlers
	numWriters := 10 // Background checker (simulated multiple times)

	var wg sync.WaitGroup

	// Start reader goroutines (simulating concurrent HTTP requests)
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.GetStatus() // Just read repeatedly
			}
		}()
	}

	// Start writer goroutines (simulating background checker updates)
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

	// Verify cache is still in a valid state after concurrent access
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

// TestIsStale verifies the staleness detection works correctly.
// The cache should report stale data when lastChecked is older than maxAge.
func TestIsStale(t *testing.T) {
	c := New()

	// Update status to set lastChecked to now
	c.UpdateStatus(http.StatusOK, "active")

	// Immediately check - should NOT be stale
	if c.IsStale(1 * time.Second) {
		t.Error("Freshly updated cache should not be stale")
	}

	// Wait 100ms, check with 50ms threshold - should be stale
	time.Sleep(100 * time.Millisecond)
	if !c.IsStale(50 * time.Millisecond) {
		t.Error("Cache should be stale after 100ms with 50ms threshold")
	}

	// Check with 200ms threshold - should NOT be stale
	if c.IsStale(200 * time.Millisecond) {
		t.Error("Cache should not be stale after 100ms with 200ms threshold")
	}
}

// TestIsStaleWithNewCache verifies a brand new cache reports as stale.
// A cache that has never been updated should be considered stale.
func TestIsStaleWithNewCache(t *testing.T) {
	c := New()

	// Brand new cache should be stale (lastChecked is zero time)
	if !c.IsStale(1 * time.Millisecond) {
		t.Error("New cache with zero lastChecked should be stale")
	}
}

// TestIsStaleConcurrentAccess verifies IsStale is thread-safe.
// Multiple goroutines should be able to check staleness simultaneously
// without race conditions.
func TestIsStaleConcurrentAccess(t *testing.T) {
	c := New()
	c.UpdateStatus(http.StatusOK, "active")

	done := make(chan bool, 10)

	// 10 concurrent staleness checks
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.IsStale(1 * time.Second)
			}
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we got here without panicking, thread safety works
}

// -----------------------------------------------------------------------------
// Immutability Tests
// -----------------------------------------------------------------------------

// TestGetStatusDoesNotModifyCache verifies that reading the cache doesn't
// change its state. This might seem obvious, but it's important to verify
// that RLock/RUnlock is used correctly and doesn't have side effects.
func TestGetStatusDoesNotModifyCache(t *testing.T) {
	c := New()
	c.UpdateStatus(http.StatusOK, "active")

	// Read status multiple times
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
