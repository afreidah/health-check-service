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
// Observability:
//   - All requests are logged with current status
//   - Request duration and count metrics published to Prometheus
//   - Metrics recorded via defer to ensure they're captured even on errors
//
// -----------------------------------------------------------------------------

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

// component logger
var logh = slog.Default().With("component", "http")

func requestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}

// -----------------------------------------------------------------------------
// Health Check Handler
// -----------------------------------------------------------------------------

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
// Metrics:
//
//	Records request duration and increments total request counter, labeled
//	by status code for monitoring and alerting.
func HealthHandler(w http.ResponseWriter, r *http.Request, cache *cache.ServiceCache) {
	start := time.Now()
	var statusCode int

	// -------------------------------------------------------------------------
	// Deferred Metrics Recording
	// -------------------------------------------------------------------------
	// Use defer to ensure metrics are recorded even if the handler panics
	// or returns early. This guarantees complete observability.
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RequestDuration.Observe(duration)
		metrics.RequestsTotal.
			WithLabelValues(fmt.Sprintf("%d", statusCode)).
			Inc()
	}()

	// -------------------------------------------------------------------------
	// Read Status from Cache
	// -------------------------------------------------------------------------
	// Fetch current status from cache (updated by background checker)
	// This is a fast, non-blocking read operation
	statusCode, status := cache.GetStatus()

	// in HealthHandler:
	logh.Info("health request",
		"request_id", requestID(r),
		"client_ip", clientIP(r),
		"state", status,
		"status", statusCode,
		"path", r.URL.Path,
		"method", r.Method,
	)

	// Add staleness warning
	if cache.IsStale(30 * time.Second) {
		w.Header().Set("Warning", "199 - Stale health check data")
	}

	// -------------------------------------------------------------------------
	// Send HTTP Response
	// -------------------------------------------------------------------------
	// Return only the status code with no body
	// Monitoring systems typically only check the HTTP status
	w.WriteHeader(statusCode)
}

// -----------------------------------------------------------------------------
// Status API Response Types
// -----------------------------------------------------------------------------

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
}

// -----------------------------------------------------------------------------
// Status API Handler
// -----------------------------------------------------------------------------

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
//	  "healthy": true
//	}
func StatusAPIHandler(w http.ResponseWriter, r *http.Request, cache *cache.ServiceCache, serviceName string) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get current status from cache
	statusCode, state := cache.GetStatus()

	// Build response
	response := StatusResponse{
		Service:    serviceName,
		State:      state,
		StatusCode: statusCode,
		Healthy:    statusCode == http.StatusOK,
		Uptime:     99.9, // TODO: Calculate actual uptime from metrics
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

	// Get last checked time
	response.LastChecked = cache.GetLastChecked()

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow CORS for development

	// Encode and send response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// in StatusAPIHandler error path:
		logh.Error("error encoding status response",
			"request_id", requestID(r),
			"client_ip", clientIP(r),
			"err", err,
		)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
