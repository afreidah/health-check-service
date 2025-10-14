// -----------------------------------------------------------------------------
// Service Status Cache
// -----------------------------------------------------------------------------
//
// This package provides a thread-safe in-memory cache for storing systemd
// service health status. The cache is designed to be accessed concurrently
// by multiple goroutines - the background checker writes to it periodically,
// while HTTP handlers read from it on every request.
//
// Concurrency Model:
//   - Uses sync.RWMutex for efficient concurrent reads
//   - Single writer (background checker)
//   - Multiple readers (HTTP request handlers)
//
// The cache stores three key pieces of information:
//   1. HTTP status code (200, 503, 500)
//   2. Systemd service state (active, inactive, failed, etc.)
//   3. Timestamp of last health check
//
// -----------------------------------------------------------------------------

package cache

import (
	"net/http"
	"sync"
	"time"
)

// -----------------------------------------------------------------------------
// Type Definitions
// -----------------------------------------------------------------------------

// ServiceCache holds the current health status of a monitored systemd service.
// All fields are protected by a read-write mutex to ensure thread-safe access.
type ServiceCache struct {
	mu          sync.RWMutex
	statusCode  int       // HTTP status code (200, 503, 500)
	state       string    // Systemd ActiveState value
	lastChecked time.Time // Timestamp of last status update
}

// -----------------------------------------------------------------------------
// Public Methods
// -----------------------------------------------------------------------------

// GetStatus returns the current cached status code and state.
// This method is safe for concurrent access by multiple HTTP handlers.
// Uses RLock to allow multiple simultaneous readers.
func (c *ServiceCache) GetStatus() (int, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statusCode, c.state
}

// UpdateStatus atomically updates the cached status and sets lastChecked to now.
// This method is called by the background checker goroutine.
// Uses Lock for exclusive write access.
func (c *ServiceCache) UpdateStatus(code int, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statusCode = code
	c.state = state
	c.lastChecked = time.Now()
}

// -----------------------------------------------------------------------------
// Constructor
// -----------------------------------------------------------------------------

// New creates a new ServiceCache initialized with "unavailable" status.
// The service starts as unavailable until the first successful health check.
func New() *ServiceCache {
	return &ServiceCache{
		statusCode: http.StatusServiceUnavailable,
		state:      "unknown",
	}
}
