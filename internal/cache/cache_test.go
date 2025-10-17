// =============================================================================
// Service Status Cache - Tests
// =============================================================================
//
// This test suite verifies the thread-safe behavior and state machine logic
// of the ServiceCache. Since the cache is accessed concurrently by multiple
// goroutines in production, these tests focus heavily on race conditions and
// concurrent access patterns.
//
// Test Coverage:
//   - Constructor initialization and default values
//   - Cache lifecycle state transitions
//   - Basic read/write operations
//   - Timestamp tracking on updates
//   - Staleness detection
//   - Concurrent access (race detection)
//   - New state query methods
//
// Run with race detector: go test -race ./internal/cache
//
// =============================================================================

package cache

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Constructor Tests
// =============================================================================

// TestNew verifies the constructor initializes cache in uninitialized state.
// The cache should start in an "uninitialized" state until the first real
// health check completes.
func TestNew(t *testing.T) {
	c := New()

	if c == nil {
		t.Fatal("New() returned nil")
	}

	// Check initial status
	statusCode, state := c.GetStatus()

	if statusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected initial status %d, got %d", http.StatusServiceUnavailable, statusCode)
	}

	if state != "uninitialized" {
		t.Errorf("Expected initial state 'uninitialized', got '%s'", state)
	}

	// Check cache state
	if c.GetCacheState() != StateUninitialized {
		t.Errorf("Expected StateUninitialized, got %s", c.GetCacheState())
	}

	// Check that it's uninitialized (never checked)
	if !c.IsUninitialized() {
		t.Error("New cache should report IsUninitialized() = true")
	}

	// Check last checked is zero time
	if !c.GetLastChecked().IsZero() {
		t.Error("New cache should have zero lastChecked time")
	}
}

// =============================================================================
// State Transition Tests
// =============================================================================

// TestStateTransitionUninitialized tests that cache starts uninitialized.
func TestStateTransitionUninitialized(t *testing.T) {
	c := New()

	if c.GetCacheState() != StateUninitialized {
		t.Errorf("New cache should be StateUninitialized, got %s", c.GetCacheState())
	}

	if !c.IsUninitialized() {
		t.Error("IsUninitialized() should be true for new cache")
	}
}

// TestStateTransitionFirstUpdate tests transition from uninitialized to running.
func TestStateTransitionFirstUpdate(t *testing.T) {
	c := New()

	// First update: service is active
	c.UpdateStatus(http.StatusOK, "active")

	// Should transition to StateRunning
	if c.GetCacheState() != StateRunning {
		t.Errorf("After first update, cache should be StateRunning, got %s", c.GetCacheState())
	}

	// Should no longer report uninitialized
	if c.IsUninitialized() {
		t.Error("After UpdateStatus, IsUninitialized() should be false")
	}

	// Should have recent lastChecked
	if c.GetLastChecked().IsZero() {
		t.Error("After UpdateStatus, lastChecked should not be zero")
	}

	// Status should be updated
	statusCode, state := c.GetStatus()
	if statusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, statusCode)
	}
	if state != "active" {
		t.Errorf("Expected state 'active', got '%s'", state)
	}
}

// TestStateTransitionToError tests transition to error state.
func TestStateTransitionToError(t *testing.T) {
	c := New()

	// Normal operation first
	c.UpdateStatus(http.StatusOK, "active")
	if c.GetCacheState() != StateRunning {
		t.Fatal("Expected StateRunning")
	}

	// Now encounter error
	c.UpdateStatus(http.StatusInternalServerError, "error")

	// Should transition to StateError
	if c.GetCacheState() != StateError {
		t.Errorf("After error update, should be StateError, got %s", c.GetCacheState())
	}

	if !c.IsError() {
		t.Error("IsError() should be true after error status update")
	}

	// Status should reflect error
	statusCode, state := c.GetStatus()
	if statusCode != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, statusCode)
	}
	if state != "error" {
		t.Errorf("Expected state 'error', got '%s'", state)
	}
}

// =============================================================================
// Basic Operations Tests
// =============================================================================

// TestUpdateAndGetStatus verifies the basic update/read cycle.
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

// =============================================================================
// Timestamp Validation Tests
// =============================================================================

// TestUpdateStatusSetsLastChecked verifies lastChecked is updated on change.
func TestUpdateStatusSetsLastChecked(t *testing.T) {
	c := New()

	before := time.Now()
	time.Sleep(10 * time.Millisecond)

	c.UpdateStatus(http.StatusOK, "active")

	lastChecked := c.GetLastChecked()

	if lastChecked.Before(before) {
		t.Errorf("lastChecked should be updated to recent time")
	}
}

// =============================================================================
// Staleness Tests
// =============================================================================

// TestIsStale verifies staleness detection works correctly.
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
func TestIsStaleWithNewCache(t *testing.T) {
	c := New()

	// Brand new cache should be stale (lastChecked is zero time)
	if !c.IsStale(1 * time.Millisecond) {
		t.Error("New cache with zero lastChecked should be stale")
	}
}

// TestGetStaleness verifies staleness duration is calculated correctly.
func TestGetStaleness(t *testing.T) {
	c := New()
	c.UpdateStatus(http.StatusOK, "active")

	// Set lastChecked to 5 seconds ago
	c.SetLastChecked(time.Now().Add(-5 * time.Second))

	staleness := c.GetStaleness()

	// Should be approximately 5 seconds
	// Allow 100ms tolerance for test execution time
	if staleness < 4900*time.Millisecond || staleness > 5100*time.Millisecond {
		t.Errorf("Expected staleness ~5s, got %v", staleness)
	}
}

// =============================================================================
// Concurrency Tests
// =============================================================================

// TestConcurrentAccess verifies thread safety under high concurrent load.
// Simulates production with many HTTP handlers reading and one checker writing.
//
// Run with: go test -race to detect data races
func TestConcurrentAccess(t *testing.T) {
	c := New()

	numReaders := 50 // HTTP request handlers
	numWriters := 10 // Background checker (simulated)

	var wg sync.WaitGroup

	// Start reader goroutines
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.GetStatus()
				c.GetLastChecked()
				c.IsStale(1 * time.Second)
				c.GetStaleness()
			}
		}()
	}

	// Start writer goroutines
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					c.UpdateStatus(http.StatusOK, "active")
				} else {
					c.UpdateStatus(http.StatusServiceUnavailable, "inactive")
				}
			}
		}(i)
	}

	// Wait for all goroutines
	wg.Wait()

	// Verify cache is still valid
	statusCode, state := c.GetStatus()
	validStates := map[string]bool{"active": true, "inactive": true}
	if !validStates[state] {
		t.Errorf("After concurrent access, got unexpected state: %s", state)
	}

	validCodes := map[int]bool{http.StatusOK: true, http.StatusServiceUnavailable: true}
	if !validCodes[statusCode] {
		t.Errorf("After concurrent access, got unexpected code: %d", statusCode)
	}
}

// TestIsStaleConcurrentAccess verifies IsStale is thread-safe.
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
}

// =============================================================================
// New Query Method Tests
// =============================================================================

// TestIsUninitialized verifies uninitialized detection.
func TestIsUninitialized(t *testing.T) {
	c := New()

	if !c.IsUninitialized() {
		t.Error("New cache should report IsUninitialized() = true")
	}

	c.UpdateStatus(http.StatusOK, "active")

	if c.IsUninitialized() {
		t.Error("After UpdateStatus, IsUninitialized() should be false")
	}
}

// TestIsError verifies error state detection.
func TestIsError(t *testing.T) {
	c := New()

	if c.IsError() {
		t.Error("New cache should not report IsError() = true")
	}

	// Normal update
	c.UpdateStatus(http.StatusOK, "active")
	if c.IsError() {
		t.Error("After normal update, IsError() should be false")
	}

	// Error update
	c.UpdateStatus(http.StatusInternalServerError, "error")
	if !c.IsError() {
		t.Error("After error update, IsError() should be true")
	}
}

// TestGetCacheState verifies cache state reporting.
func TestGetCacheState(t *testing.T) {
	c := New()

	if c.GetCacheState() != StateUninitialized {
		t.Errorf("New cache should have StateUninitialized, got %s", c.GetCacheState())
	}

	c.UpdateStatus(http.StatusOK, "active")
	if c.GetCacheState() != StateRunning {
		t.Errorf("After update, should have StateRunning, got %s", c.GetCacheState())
	}

	c.UpdateStatus(http.StatusInternalServerError, "error")
	if c.GetCacheState() != StateError {
		t.Errorf("After error, should have StateError, got %s", c.GetCacheState())
	}
}

// =============================================================================
// String Representation Tests
// =============================================================================

// TestString verifies String() provides useful debug output.
func TestString(t *testing.T) {
	c := New()
	s := c.String()

	if len(s) == 0 {
		t.Error("String() returned empty string")
	}

	// Should contain key information
	if !contains(s, "uninitialized") {
		t.Errorf("String() should contain 'uninitialized', got: %s", s)
	}

	c.UpdateStatus(http.StatusOK, "active")
	s = c.String()

	if !contains(s, "active") {
		t.Errorf("String() should contain 'active', got: %s", s)
	}
}

// Helper function for string tests
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
