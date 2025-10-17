// =============================================================================
// Service Status Cache
// =============================================================================
//
// This package provides a thread-safe in-memory cache for storing systemd
// service health status. The cache is designed to be accessed concurrently
// by multiple goroutines - the background checker writes to it periodically,
// while HTTP handlers read from it on every request.
//
// Concurrency Model:
//   - Uses sync.RWMutex for efficient concurrent reads
//   - Single writer (background checker) per service
//   - Multiple readers (HTTP request handlers)
//   - All fields protected by mutex
//
// Cache Lifecycle:
//   1. Uninitialized: Service started, no check yet
//      - State: "uninitialized"
//      - Status: 503 (not ready)
//      - Used for: First few seconds of startup
//
//   2. Checking: Checker is running, data is current
//      - State: "active", "inactive", "failed", etc. (from systemd)
//      - Status: 200, 503, or 500
//      - Used for: Normal operation
//
//   3. Stale: Checker stopped responding
//      - State: Still shows last known state
//      - Status: Still shows last known status (but old!)
//      - Used for: Degraded operation (checker stuck/crashed)
//      - Detected by: Staleness check via IsStale()
//
//   4. Error: Checker encountered persistent error
//      - State: "error"
//      - Status: 500
//      - Used for: D-Bus not available, permission issues, etc.
//
// Key Design Decision:
// When checker fails, cache keeps serving last known good state rather than
// returning "unknown". This allows load balancers to continue routing traffic
// based on last known state (better than abrupt failure). Staleness detection
// warns about stale data separately.
//
// =============================================================================

// Package cache
package cache

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// =============================================================================
// State Type Definition
// =============================================================================

// StateType represents the lifecycle state of the cache
type StateType int

const (
	// StateUninitialized: Service just started, checker hasn't run yet
	// This is the initial state before first check completes
	StateUninitialized StateType = iota

	// StateRunning: Checker is running normally, data is current
	// This is the normal operational state
	StateRunning

	// StateStale: Checker hasn't updated cache recently
	// Data is old but still being served (better than failure)
	StateStale

	// StateError: Checker encountered an error
	// Unable to get status from systemd (D-Bus down, permissions, etc.)
	StateError
)

// String returns human-readable state name
func (s StateType) String() string {
	switch s {
	case StateUninitialized:
		return "uninitialized"
	case StateRunning:
		return "running"
	case StateStale:
		return "stale"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// =============================================================================
// Service Cache Type
// =============================================================================

// ServiceCache holds the current health status of a monitored systemd service.
// All fields are protected by a read-write mutex to ensure thread-safe access.
//
// Example lifecycle:
//
//	cache := New()
//	// cache.cacheState == StateUninitialized
//	// cache.systemdState == "uninitialized"
//	// cache.statusCode == 503
//	// HTTP requests return 503 "Service Unavailable"
//
//	// Checker runs first check...
//	cache.UpdateStatus(200, "active")
//	// cache.cacheState == StateRunning
//	// cache.systemdState == "active"
//	// cache.statusCode == 200
//	// HTTP requests return 200 OK
//
//	// Service goes down...
//	cache.UpdateStatus(503, "inactive")
//	// cache.cacheState == StateRunning (still)
//	// cache.systemdState == "inactive"
//	// cache.statusCode == 503
//	// HTTP requests return 503 Service Unavailable
//
//	// Checker gets stuck (5 minutes without update)...
//	// cache.cacheState == StateStale (detected by IsStale check)
//	// cache.systemdState == "inactive" (still old value)
//	// cache.statusCode == 503 (still old value)
//	// HTTP requests return 503 with Warning header
//	// Metrics show cache_staleness_seconds = 300
//
//	// D-Bus completely fails...
//	cache.UpdateStatus(500, "error")
//	// cache.cacheState == StateError
//	// cache.systemdState == "error"
//	// cache.statusCode == 500
//	// HTTP requests return 500 Internal Server Error
type ServiceCache struct {
	mu sync.RWMutex

	// systemdState is the ActiveState from systemd
	// Examples: "active", "inactive", "failed", "activating", "error"
	systemdState string

	// statusCode is the HTTP status code to return
	// 200 = service is active/healthy
	// 503 = service is down/transitioning
	// 500 = error checking service
	statusCode int

	// lastChecked is when the cache was last updated
	// Used to detect staleness (no update for too long)
	lastChecked time.Time

	// cacheState represents the lifecycle state of the cache itself
	// This is separate from the systemd state
	// Can be: uninitialized, running, stale, error
	cacheState StateType
}

// =============================================================================
// Constructor
// =============================================================================

// New creates a new ServiceCache initialized in the uninitialized state.
// The service starts as unavailable until the first successful health check.
//
// Initial state:
//   - cacheState: StateUninitialized (checker hasn't run yet)
//   - systemdState: "uninitialized" (not a real systemd state)
//   - statusCode: 503 (not ready yet)
//   - lastChecked: zero time (never checked)
//
// This communicates clearly: "Not ready yet, wait for first check"
func New() *ServiceCache {
	return &ServiceCache{
		cacheState:   StateUninitialized,
		systemdState: "uninitialized",
		statusCode:   http.StatusServiceUnavailable,
		lastChecked:  time.Time{}, // Zero time = never checked
	}
}

// =============================================================================
// Public Query Methods
// =============================================================================

// GetStatus returns the current cached status code and systemd state.
// Uses RLock to allow multiple simultaneous readers (concurrent HTTP requests).
//
// Returns:
//   - statusCode: HTTP status code (200, 503, 500)
//   - state: Systemd state (active, inactive, failed, error, etc.)
//
// Example:
//
//	code, state := cache.GetStatus()
//	if code == 200 {
//	    // Service is healthy
//	} else if code == 503 {
//	    // Service is down
//	} else if code == 500 {
//	    // Error checking service
//	}
func (c *ServiceCache) GetStatus() (int, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statusCode, c.systemdState
}

// GetLastChecked returns the timestamp of the last status update.
// Used by the dashboard to show when the service was last checked.
func (c *ServiceCache) GetLastChecked() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastChecked
}

// =============================================================================
// CHANGED: New query methods for cache state
// =============================================================================

// GetCacheState returns the current cache lifecycle state.
// This is separate from the systemd state and describes the cache itself.
//
// Possible values:
//   - StateUninitialized: Checker hasn't run yet (first few seconds)
//   - StateRunning: Checker is working normally
//   - StateStale: Checker hasn't updated in a while
//   - StateError: Checker encountered an error
func (c *ServiceCache) GetCacheState() StateType {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cacheState
}

// IsUninitialized returns true if the cache has never been updated.
// Useful to detect if the service just started and hasn't gotten first check yet.
//
// Example use case:
//   - Dashboard can show "Initializing..." while this is true
//   - Load balancer can skip health check during initialization window
func (c *ServiceCache) IsUninitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastChecked.IsZero()
}

// IsError returns true if the cache is in error state.
// This means the checker encountered a permanent error (D-Bus down, etc.)
//
// Example use case:
//   - Return 500 immediately instead of waiting for staleness
//   - Alert operators that something is fundamentally wrong
func (c *ServiceCache) IsError() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cacheState == StateError
}

// IsStale checks if the last update is older than the specified duration.
// Used to detect if the background checker has stopped responding.
//
// Parameters:
//   - maxAge: How old is too old? (typically 2× check interval)
//
// Example:
//
//	if cache.IsStale(30 * time.Second) {
//	    // Data is older than 30 seconds
//	    // Add warning header, alert operators
//	}
//
// Returns:
//   - true: Last check was more than maxAge ago
//   - false: Last check was recent (within maxAge)
func (c *ServiceCache) IsStale(maxAge time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.lastChecked) > maxAge
}

// GetStaleness returns how old the cached data is in seconds.
// Useful for metrics and logging.
//
// Returns:
//   - Duration since last update
//   - 0 if never updated (but this is unusual)
//
// Example:
//
//	staleness := cache.GetStaleness()
//	slog.Warn("stale data", "age", staleness)
//	metrics.CacheStaleness.Set(staleness.Seconds())
func (c *ServiceCache) GetStaleness() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.lastChecked)
}

// =============================================================================
// Public Update Methods
// =============================================================================

// UpdateStatus atomically updates the cached status and transitions to StateRunning.
// Called by the background checker when it successfully queries systemd.
//
// This method:
//  1. Updates statusCode and systemdState with new values
//  2. Sets lastChecked to current time
//  3. Transitions cacheState to StateRunning (unless already in error)
//  4. Uses Lock for exclusive write access
//
// Parameters:
//   - code: HTTP status code (200, 503, 500)
//   - state: Systemd state (active, inactive, failed, etc.)
//
// Example:
//
//	// Service is active and healthy
//	cache.UpdateStatus(200, "active")
//	// cache.cacheState becomes StateRunning
//	// cache.lastChecked becomes now
//
//	// Service is down
//	cache.UpdateStatus(503, "inactive")
//	// cache.cacheState becomes StateRunning
//	// cache.lastChecked becomes now
//
//	// Checker encountered error
//	cache.UpdateStatus(500, "error")
//	// cache.cacheState becomes StateError
//	// cache.lastChecked becomes now
func (c *ServiceCache) UpdateStatus(code int, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.statusCode = code
	c.systemdState = state
	c.lastChecked = time.Now()

	// Transition state machine
	// If explicitly error, stay in error state
	if state == "error" {
		c.cacheState = StateError
	} else {
		// Otherwise, transition to running
		c.cacheState = StateRunning
	}
}

// =============================================================================
// CHANGED: New helper for testing - set last checked time
// =============================================================================

// SetLastChecked manually sets the lastChecked time.
// This is exported specifically to allow testing of staleness detection.
//
// WARNING: Only use this for testing!
// In production, UpdateStatus() sets lastChecked automatically.
//
// Example (test only):
//
//	cache := New()
//	cache.UpdateStatus(200, "active")
//	cache.SetLastChecked(time.Now().Add(-35 * time.Second))
//	if !cache.IsStale(30 * time.Second) {
//	    t.Error("Expected stale cache")
//	}
func (c *ServiceCache) SetLastChecked(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastChecked = t
}

// =============================================================================
// Helper Methods
// =============================================================================

// String returns a debug string representation of the cache state.
// Useful for logging and debugging.
//
// Example output:
//
//	"ServiceCache{state=running, systemd=active, code=200, age=5s}"
func (c *ServiceCache) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	age := time.Since(c.lastChecked)
	if c.lastChecked.IsZero() {
		age = time.Duration(0)
	}

	return fmt.Sprintf(
		"ServiceCache{state=%s, systemd=%s, code=%d, age=%v}",
		c.cacheState.String(),
		c.systemdState,
		c.statusCode,
		age,
	)
}
