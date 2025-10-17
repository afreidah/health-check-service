// -----------------------------------------------------------------------
// Rate Limiting - Per-IP Token Bucket
// -----------------------------------------------------------------------
//
// Package ratelimit provides per-IP rate limiting using token bucket algorithm.
// Different rate limits are applied to different endpoints. Stale IP entries are
// cleaned up periodically to prevent memory leaks.
//
// -----------------------------------------------------------------------

package ratelimit

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var logr = slog.Default().With("component", "ratelimit")

// -----------------------------------------------------------------------
// Types
// -----------------------------------------------------------------------

// Limiter holds a rate limiter for a single IP address and tracks last access.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Manager coordinates per-IP rate limiters and cleanup.
type Manager struct {
	mu               sync.RWMutex
	limiters         map[string]*ipLimiter
	requestsPerSec   float64 // tokens/second
	burstSize        int     // max burst tokens
	cleanupInterval  time.Duration
	cleanupIdleAfter time.Duration
}

// -----------------------------------------------------------------------
// Constructor
// -----------------------------------------------------------------------

// New creates a new rate limit manager with the given configuration.
//
// Parameters:
//   - requestsPerSec: Refill rate (tokens per second)
//   - burstSize: Maximum burst capacity
//
// Example: New(50, 100) = 50 requests/sec, burst of 100
func New(requestsPerSec float64, burstSize int) *Manager {
	m := &Manager{
		limiters:         make(map[string]*ipLimiter),
		requestsPerSec:   requestsPerSec,
		burstSize:        burstSize,
		cleanupInterval:  5 * time.Minute,
		cleanupIdleAfter: 10 * time.Minute,
	}

	// Start background cleanup goroutine
	go m.cleanupLoop()

	return m
}

// -----------------------------------------------------------------------
// Rate Limit Check
// -----------------------------------------------------------------------

// Allow checks if a request from the given IP is allowed.
// Returns true if the request is within rate limit, false otherwise.
func (m *Manager) Allow(ip string) bool {
	limiter := m.getLimiter(ip)
	return limiter.Allow()
}

// Reserve attempts to reserve a token and returns how long to wait.
// Returns 0 if allowed immediately, otherwise the wait duration.
func (m *Manager) Reserve(ip string) time.Duration {
	limiter := m.getLimiter(ip)
	reservation := limiter.Reserve()
	if !reservation.OK() {
		return time.Duration(time.Second)
	}
	return reservation.Delay()
}

// -----------------------------------------------------------------------
// Helper Methods
// -----------------------------------------------------------------------

// getLimiter returns the limiter for the given IP, creating one if it doesn't exist.
// Also updates the lastSeen timestamp for cleanup tracking.
func (m *Manager) getLimiter(ip string) *rate.Limiter {
	m.mu.RLock()
	if limiter, exists := m.limiters[ip]; exists {
		limiter.lastSeen = time.Now()
		m.mu.RUnlock()
		return limiter.limiter
	}
	m.mu.RUnlock()

	// Create new limiter
	newLimiter := rate.NewLimiter(
		rate.Limit(m.requestsPerSec),
		m.burstSize,
	)

	// Store it
	m.mu.Lock()
	m.limiters[ip] = &ipLimiter{
		limiter:  newLimiter,
		lastSeen: time.Now(),
	}
	m.mu.Unlock()

	logr.Debug("created limiter for IP",
		"ip", ip,
		"rate", fmt.Sprintf("%.0f/sec", m.requestsPerSec),
		"burst", m.burstSize,
	)

	return newLimiter
}

// GetTokens returns the approximate number of available tokens for debugging.
func (m *Manager) GetTokens(ip string) float64 {
	limiter := m.getLimiter(ip)
	return limiter.Tokens()
}

// GetRate returns the configured requests per second rate.
func (m *Manager) GetRate() float64 {
	return m.requestsPerSec
}

// -----------------------------------------------------------------------
// Cleanup
// -----------------------------------------------------------------------

// cleanupLoop periodically removes stale IP entries to prevent memory leaks.
// IPs that haven't been seen in cleanupIdleAfter are removed.
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanup()
	}
}

// cleanup removes stale IP entries.
func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0

	for ip, entry := range m.limiters {
		if now.Sub(entry.lastSeen) > m.cleanupIdleAfter {
			delete(m.limiters, ip)
			removed++
		}
	}

	if removed > 0 {
		logr.Debug("cleanup: removed stale IP entries",
			"count", removed,
			"remaining", len(m.limiters),
		)
	}
}

// Stats returns diagnostic information about the limiter state.
func (m *Manager) Stats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"active_ips": len(m.limiters),
		"rate":       m.requestsPerSec,
		"burst":      m.burstSize,
	}
}

// -----------------------------------------------------------------------
// HTTP Middleware
// -----------------------------------------------------------------------

// Middleware returns an HTTP middleware that applies rate limiting per IP.
// Returns 429 Too Many Requests if limit exceeded.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := GetIP(r)

		if !m.Allow(ip) {
			// Include rate limit info in response headers
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", m.requestsPerSec))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "1")

			logr.Warn("rate limit exceeded",
				"ip", ip,
				"method", r.Method,
				"path", r.URL.Path,
			)

			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		// Calculate approximate remaining tokens for header
		tokens := m.GetTokens(ip)
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", m.requestsPerSec))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%.0f", tokens))

		next.ServeHTTP(w, r)
	})
}

// EndpointMiddleware creates endpoint-specific rate limit middleware.
// Useful when different endpoints need different limits.
func (m *Manager) EndpointMiddleware(endpoint string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := GetIP(r)

		if !m.Allow(ip) {
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", m.requestsPerSec))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "1")

			logr.Warn("rate limit exceeded",
				"ip", ip,
				"endpoint", endpoint,
				"method", r.Method,
				"path", r.URL.Path,
			)

			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		tokens := m.GetTokens(ip)
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", m.requestsPerSec))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%.0f", tokens))

		next.ServeHTTP(w, r)
	})
}

// -----------------------------------------------------------------------
// IP Extraction
// -----------------------------------------------------------------------

// GetIP extracts the real client IP from the request.
// Checks X-Forwarded-For header (when behind proxy) first, then X-Real-IP,
// then falls back to RemoteAddr.
func GetIP(r *http.Request) string {
	// X-Forwarded-For can contain multiple IPs (client, proxy1, proxy2, ...)
	// We want the first one (client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// X-Real-IP set by some proxies
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Direct connection
	return r.RemoteAddr
}
