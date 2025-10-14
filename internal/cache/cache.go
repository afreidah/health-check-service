// Package cache
package cache

import (
	"net/http"
	"sync"
	"time"
)

type ServiceCache struct {
	mu          sync.RWMutex
	statusCode  int
	state       string
	lastChecked time.Time
}

func (c *ServiceCache) GetStatus() (int, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statusCode, c.state
}

func (c *ServiceCache) UpdateStatus(code int, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statusCode = code
	c.state = state
	c.lastChecked = time.Now()
}

func New() *ServiceCache {
	return &ServiceCache{
		statusCode: http.StatusServiceUnavailable, // start as unavailable
		state:      "unknown",
	}
}
