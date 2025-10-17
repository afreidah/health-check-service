// -----------------------------------------------------------------------------
// HTTP Handlers
// -----------------------------------------------------------------------------
//
// This package implements the HTTP endpoint handlers for the health check
// service. Handlers are designed to be lightweight and fast, reading from
// the pre-populated cache rather than checking systemd directly on each
// request.
//
// Architecture:
//   - Handlers read from cache (no D-Bus calls per request)
//   - Background checker updates cache periodically
//   - This design prevents D-Bus overload under high HTTP traffic
//
// Endpoints:
//   GET /health - Returns service health status with appropriate HTTP codes
//                 200 OK: Service is active and healthy
//                 503 Service Unavailable: Service is down or transitioning
//                 500 Internal Server Error: Error checking service
//
//   GET /api/status - Returns JSON status for dashboard
//                     Always returns 200 OK with status info in JSON
//
// Observability:
//   - All requests are logged with current status and timing
//   - Request duration and count metrics published to Prometheus
//   - Cache staleness is tracked and warnings issued
//   - Metrics recorded via defer to ensure they're captured even on errors
//
// Security:
//   - CORS headers only added for localhost (development)
//   - HTTP method validation (GET/HEAD only)
//   - Request timeout handled by HTTP server
//
// =============================================================================

// Package handlers
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/metrics"
)

// =============================================================================
// Constants
// =============================================================================

const (
	// stalensThreshold defines when cache data is considered stale
	// If last check was more than this duration ago, warn the client
	staleThreshold = 30 * time.Second

	// allowedMethods defines which HTTP methods are allowed
	// GET: Standard for health checks
	// HEAD: Also standard (like GET but without body)
	allowedMethods = "GET, HEAD"
)

// component logger
var logh = slog.Default().With("component", "http")

// =============================================================================
// Request Context Helpers
// =============================================================================

// requestID generates or retrieves a request ID for tracing
func requestID(r *http.Request) string {
	// Check if request already has an ID (from load balancer)
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}

	// Generate random request ID if not present
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// clientIP extracts client IP from request
// Respects X-Forwarded-For header (when behind proxy)
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}

// =============================================================================
// Common Response Helpers
// =============================================================================

// CHANGED: Add helper to set common security headers
func setSecurityHeaders(w http.ResponseWriter) {
	// Prevent browser from caching
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Prevent MIME type sniffing
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Prevent clickjacking
	w.Header().Set("X-Frame-Options", "DENY")

	// Enable XSS protection
	w.Header().Set("X-XSS-Protection", "1; mode=block")
}

// CHANGED: Helper to validate HTTP method
func validateMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", allowedMethods)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// =============================================================================
// Health Check Handler
// =============================================================================

// HealthHandler serves the /health endpoint by reading the cached service
// status and returning the appropriate HTTP status code.
//
// Design Decision: Read from Cache
//
//	This handler reads from the in-memory cache instead of querying systemd
//	directly. This prevents D-Bus connection exhaustion under high load and
//	ensures consistent response times regardless of systemd responsiveness.
//
// Response Codes:
//   - 200 OK: Service is active
//   - 503 Service Unavailable: Service is inactive/failed/transitioning
//   - 500 Internal Server Error: Error communicating with systemd
//
// Headers:
//   - Warning: 199 - Stale health check data (if cache is old)
//   - Allow: GET, HEAD (indicates allowed methods)
//
// Metrics:
//
//	Records request duration and increments total request counter, labeled
//	by status code for monitoring and alerting.
func HealthHandler(w http.ResponseWriter, r *http.Request, serviceCache *cache.ServiceCache) {
	reqID := requestID(r)
	start := time.Now()
	var statusCode int

	// -------------------------------------------------------------------------
	// Deferred Metrics Recording
	// -------------------------------------------------------------------------
	// Use defer to ensure metrics are recorded even if handler panics or returns early
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RequestDuration.Observe(duration)
		metrics.RequestsTotal.
			WithLabelValues(fmt.Sprintf("%d", statusCode)).
			Inc()

		// Log request completion
		logh.Debug("health request completed",
			"request_id", reqID,
			"method", r.Method,
			"status", statusCode,
			"duration_ms", int(duration*1000),
		)
	}()

	// -------------------------------------------------------------------------
	// CHANGED: Validate HTTP method first
	// -------------------------------------------------------------------------
	if !validateMethod(w, r) {
		statusCode = http.StatusMethodNotAllowed
		return
	}

	// -------------------------------------------------------------------------
	// Set Security Headers
	// -------------------------------------------------------------------------
	setSecurityHeaders(w)

	// -------------------------------------------------------------------------
	// Read Status from Cache
	// -------------------------------------------------------------------------
	statusCode, state := serviceCache.GetStatus()

	// Log with full context
	logh.Info("health request",
		"request_id", reqID,
		"client_ip", clientIP(r),
		"state", state,
		"status_code", statusCode,
		"method", r.Method,
	)

	// -------------------------------------------------------------------------
	// CHANGED: Check for stale data and add warning header
	// -------------------------------------------------------------------------
	if serviceCache.IsStale(staleThreshold) {
		staleness := time.Since(serviceCache.GetLastChecked())

		// Add warning header per RFC 7234
		// 199 is an "Miscellaneous Persistent Warning"
		w.Header().Set("Warning", fmt.Sprintf("199 - Stale health check data (age: %ds)",
			int(staleness.Seconds())))

		logh.Warn("serving stale health data",
			"request_id", reqID,
			"staleness_seconds", int(staleness.Seconds()),
			"state", state)

		// CHANGED: Update staleness metric so watchdog can detect it
		metrics.CacheStaleness.WithLabelValues("").Set(staleness.Seconds())
	}

	// -------------------------------------------------------------------------
	// Send HTTP Response
	// -------------------------------------------------------------------------
	// For HEAD requests, don't send body (client only wants headers)
	w.WriteHeader(statusCode)
}

// =============================================================================
// Status API Response Types
// =============================================================================

// StatusResponse represents the JSON response for the dashboard API.
// This provides all the information the React dashboard needs to display
// the current service health status.
type StatusResponse struct {
	Service     string    `json:"service"`      // Name of the monitored service
	Status      string    `json:"status"`       // Human-readable status (healthy/unhealthy/error)
	State       string    `json:"state"`        // Systemd state (active/inactive/failed)
	StatusCode  int       `json:"status_code"`  // HTTP status code (200/503/500)
	LastChecked time.Time `json:"last_checked"` // When the status was last updated
	Uptime      float64   `json:"uptime"`       // Uptime percentage (placeholder for now)
	Healthy     bool      `json:"healthy"`      // Simple boolean for UI
	Stale       bool      `json:"stale"`        // CHANGED: Is the data stale?
	StalenessS  int       `json:"staleness_s"`  // CHANGED: How old is it in seconds?
}

// =============================================================================
// Status API Handler
// =============================================================================

// StatusAPIHandler serves the /api/status endpoint for the dashboard.
// Returns JSON with current service health status.
//
// This is different from /health which returns only HTTP status codes.
// This endpoint provides detailed information for the dashboard UI.
//
// Response format:
//
//	{
//	  "service": "nginx",
//	  "status": "healthy",
//	  "state": "active",
//	  "status_code": 200,
//	  "last_checked": "2025-10-15T12:34:56Z",
//	  "uptime": 99.9,
//	  "healthy": true,
//	  "stale": false,
//	  "staleness_s": 5
//	}
//
// Always returns 200 OK (even if service is down, we return the status info)
func StatusAPIHandler(
	w http.ResponseWriter,
	r *http.Request,
	serviceCache *cache.ServiceCache,
	serviceName string,
) {
	reqID := requestID(r)
	start := time.Now()

	// -------------------------------------------------------------------------
	// Deferred Metrics Recording
	// -------------------------------------------------------------------------
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RequestDuration.Observe(duration)
		// API requests are always successful (200), so just count them
		metrics.RequestsTotal.WithLabelValues("200").Inc()

		logh.Debug("api status request completed",
			"request_id", reqID,
			"duration_ms", int(duration*1000),
		)
	}()

	// -------------------------------------------------------------------------
	// CHANGED: Validate HTTP method
	// -------------------------------------------------------------------------
	if !validateMethod(w, r) {
		// ValidationMethod already set status 405, but we need to record it
		metrics.RequestsTotal.WithLabelValues("405").Inc()
		return
	}

	// -------------------------------------------------------------------------
	// Set Security Headers
	// -------------------------------------------------------------------------
	setSecurityHeaders(w)

	// -------------------------------------------------------------------------
	// Get current status from cache
	// -------------------------------------------------------------------------
	statusCode, state := serviceCache.GetStatus()
	lastChecked := serviceCache.GetLastChecked()
	staleness := time.Since(lastChecked)
	isStale := serviceCache.IsStale(staleThreshold)

	// -------------------------------------------------------------------------
	// Build response
	// -------------------------------------------------------------------------
	response := StatusResponse{
		Service:     serviceName,
		State:       state,
		StatusCode:  statusCode,
		LastChecked: lastChecked,
		Healthy:     statusCode == http.StatusOK,
		Stale:       isStale,
		StalenessS:  int(staleness.Seconds()),
	}

	// Map status code to human-readable status
	switch statusCode {
	case http.StatusOK:
		response.Status = "healthy"
	case http.StatusServiceUnavailable:
		response.Status = "unhealthy"
	case http.StatusInternalServerError:
		response.Status = "error"
	default:
		response.Status = "unknown"
	}

	// Placeholder for actual uptime calculation
	response.Uptime = 99.9

	// -------------------------------------------------------------------------
	// CHANGED: Set response headers (more conservative CORS)
	// -------------------------------------------------------------------------
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// CHANGED: Only add CORS header for localhost (development)
	// In production, use a proper reverse proxy (nginx, Traefik, etc.)
	// to handle CORS instead of exposing it from the app
	origin := r.Header.Get("Origin")
	if origin == "http://localhost:3000" || origin == "http://localhost:8080" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
	}

	// -------------------------------------------------------------------------
	// Encode and send response
	// -------------------------------------------------------------------------
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logh.Error("error encoding status response",
			"request_id", reqID,
			"client_ip", clientIP(r),
			"error", err.Error())

		// Response already started, can't send error
		// Just log it
		return
	}

	logh.Debug("api status response sent",
		"request_id", reqID,
		"service", serviceName,
		"status", response.Status,
		"stale", isStale,
	)
}
