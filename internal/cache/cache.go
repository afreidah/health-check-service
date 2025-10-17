// -----------------------------------------------------------------------
// Service Status Cache
// -----------------------------------------------------------------------
//
// Package cache provides a thread-safe in-memory cache for storing systemd
// service health status. The cache is accessed concurrently by the background
// checker (writer) and HTTP handlers (readers).
//
// Cache Lifecycle:
// The cache progresses through states as the service operates. Initially
// uninitialized before the first health check, the cache transitions to
// running state when the checker successfully queries systemd. If the
// checker stops responding, the cache becomes stale (old data still served).
// If the checker encounters persistent errors such as D-Bus unavailability
// or permission issues, it transitions to error state.
//
// When the checker fails, the cache continues serving the last known good
// state rather than returning unknown. This allows load balancers to make
// informed decisions. Staleness is tracked separately via IsStale() to warn
// consumers that data is old.
//
// -----------------------------------------------------------------------

package cache

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// -----------------------------------------------------------------------
// State Type Definition
// -----------------------------------------------------------------------

// StateType represents the lifecycle state of the cache.
type StateType int

const (
	// StateUninitialized indicates the service just started and the checker
	// has not yet completed its first run.
	StateUninitialized StateType = iota

	// StateRunning indicates the checker is operating normally and cache data
	// is current.
	StateRunning

	// StateStale indicates the checker has not updated the cache recently.
	// The cache continues serving the last known state but with staleness
	// warnings to indicate data age.
	StateStale

	// StateError indicates the checker encountered a persistent error and is
	// unable to retrieve status from systemd.
	StateError
)

// String returns the human-readable name of the state.
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

// -----------------------------------------------------------------------
// Service Cache Type
// -----------------------------------------------------------------------

// ServiceCache holds the current health status of a monitored systemd
// service. All fields are protected by RWMutex to enable concurrent access
// by the background checker and HTTP handlers.
type ServiceCache struct {
	mu sync.RWMutex

	// systemdState is the ActiveState from systemd.
	// Examples: "active", "inactive", "failed", "activating"
	systemdState string

	// statusCode is the HTTP status code to return.
	// 200 = service is active, 503 = service is down, 500 = error
	statusCode int

	// lastChecked is the timestamp of the most recent cache update.
	lastChecked time.Time

	// cacheState represents the lifecycle state of the cache.
	cacheState StateType
}

// -----------------------------------------------------------------------
// Constructor
// -----------------------------------------------------------------------

// New creates a new ServiceCache initialized in the uninitialized state
// with 503 Service Unavailable until the first successful health check.
func New() *ServiceCache {
	return &ServiceCache{
		cacheState:   StateUninitialized,
		systemdState: "uninitialized",
		statusCode:   http.StatusServiceUnavailable,
		lastChecked:  time.Time{},
	}
}

// -----------------------------------------------------------------------
// Query Methods
// -----------------------------------------------------------------------

// GetStatus returns the current cached HTTP status code and systemd state.
// Multiple goroutines may call this concurrently (HTTP handlers reading
// during checker updates).
func (c *ServiceCache) GetStatus() (int, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statusCode, c.systemdState
}

// GetLastChecked returns the timestamp of the most recent status update.
func (c *ServiceCache) GetLastChecked() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastChecked
}

// GetCacheState returns the current cache lifecycle state.
func (c *ServiceCache) GetCacheState() StateType {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cacheState
}

// IsUninitialized returns whether the cache has never been updated.
func (c *ServiceCache) IsUninitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastChecked.IsZero()
}

// IsError returns whether the cache is in error state.
func (c *ServiceCache) IsError() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cacheState == StateError
}

// IsStale checks if the last update is older than the specified duration.
// Returns true if data age exceeds maxAge, false if data is recent.
func (c *ServiceCache) IsStale(maxAge time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.lastChecked) > maxAge
}

// GetStaleness returns the duration since the last cache update.
func (c *ServiceCache) GetStaleness() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.lastChecked)
}

// -----------------------------------------------------------------------
// Update Methods
// -----------------------------------------------------------------------

// UpdateStatus atomically updates the cached status and transitions the
// cache state. Called by the background checker when it successfully queries
// systemd.
//
// Parameters:
//   - code: HTTP status code (200, 503, 500)
//   - state: systemd ActiveState (active, inactive, failed, etc.)
func (c *ServiceCache) UpdateStatus(code int, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.statusCode = code
	c.systemdState = state
	c.lastChecked = time.Now()

	// Transition state machine based on state
	if state == "error" {
		c.cacheState = StateError
	} else {
		c.cacheState = StateRunning
	}
}

// SetLastChecked sets the lastChecked timestamp manually. This method is
// exported for testing staleness detection. Production code should use
// UpdateStatus which sets it automatically.
func (c *ServiceCache) SetLastChecked(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastChecked = t
}

// -----------------------------------------------------------------------
// String Representation
// -----------------------------------------------------------------------

// String returns a debug representation of the cache state including
// lifecycle state, systemd state, HTTP code, and data age.
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
