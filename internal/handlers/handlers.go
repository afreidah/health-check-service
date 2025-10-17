// -----------------------------------------------------------------------
// HTTP Handlers
// -----------------------------------------------------------------------
//
// Package handlers implements HTTP endpoint handlers for the health check
// service. Handlers read from a pre-populated cache to avoid D-Bus calls per
// request, preventing connection exhaustion under high load.
//
// Endpoints:
//   GET /health - Returns service health with appropriate HTTP status codes
//                 (200, 503, or 500)
//   GET /api/status - Returns JSON status for dashboard and programmatic access
//
// -----------------------------------------------------------------------

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

// -----------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------

const (
	// staleThreshold defines when cached data is considered stale
	staleThreshold = 30 * time.Second

	// allowedMethods lists HTTP methods accepted by health endpoints
	allowedMethods = "GET, HEAD"
)

var logh = slog.Default().With("component", "http")

// -----------------------------------------------------------------------
// Request Helpers
// -----------------------------------------------------------------------

// requestID returns or generates a request ID for tracing. Checks
// X-Request-ID header first, then generates a random ID if not present.
func requestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// clientIP extracts the client IP from the request, respecting X-Forwarded-For
// header when behind a proxy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}

// -----------------------------------------------------------------------
// Response Helpers
// -----------------------------------------------------------------------

// setSecurityHeaders sets common security headers on the response.
func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
}

// validateMethod checks if the request method is allowed and returns false
// if not, with appropriate error response already written.
func validateMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", allowedMethods)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// -----------------------------------------------------------------------
// Health Check Handler
// -----------------------------------------------------------------------

// HealthHandler serves the /health endpoint by returning the cached service
// status. Returns 200 if active, 503 if unavailable, 500 if error checking.
//
// The handler reads from cache rather than querying systemd directly to
// prevent D-Bus connection exhaustion under high request volume. Metrics are
// recorded regardless of outcome via defer.
func HealthHandler(w http.ResponseWriter, r *http.Request, serviceCache *cache.ServiceCache) {
	reqID := requestID(r)
	start := time.Now()
	var statusCode int

	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RequestDuration.Observe(duration)
		metrics.RequestsTotal.
			WithLabelValues(fmt.Sprintf("%d", statusCode)).
			Inc()

		logh.Debug("health request completed",
			"request_id", reqID,
			"method", r.Method,
			"status", statusCode,
			"duration_ms", int(duration*1000),
		)
	}()

	if !validateMethod(w, r) {
		statusCode = http.StatusMethodNotAllowed
		return
	}

	setSecurityHeaders(w)

	statusCode, state := serviceCache.GetStatus()

	logh.Info("health request",
		"request_id", reqID,
		"client_ip", clientIP(r),
		"state", state,
		"status_code", statusCode,
		"method", r.Method,
	)

	// Add warning header if cached data is stale
	if serviceCache.IsStale(staleThreshold) {
		staleness := time.Since(serviceCache.GetLastChecked())
		w.Header().Set("Warning", fmt.Sprintf("199 - Stale health check data (age: %ds)",
			int(staleness.Seconds())))

		logh.Warn("serving stale health data",
			"request_id", reqID,
			"staleness_seconds", int(staleness.Seconds()),
			"state", state)

		metrics.CacheStaleness.WithLabelValues("").Set(staleness.Seconds())
	}

	w.WriteHeader(statusCode)
}

// -----------------------------------------------------------------------
// Status API Response
// -----------------------------------------------------------------------

// StatusResponse represents the JSON response for the status API endpoint.
// This structure provides all information needed by the dashboard frontend
// and programmatic clients.
type StatusResponse struct {
	Service     string    `json:"service"`
	Status      string    `json:"status"`
	State       string    `json:"state"`
	StatusCode  int       `json:"status_code"`
	LastChecked time.Time `json:"last_checked"`
	Uptime      float64   `json:"uptime"`
	Healthy     bool      `json:"healthy"`
	Stale       bool      `json:"stale"`
	StalenessS  int       `json:"staleness_s"`
}

// -----------------------------------------------------------------------
// Status API Handler
// -----------------------------------------------------------------------

// StatusAPIHandler serves the /api/status endpoint, returning detailed health
// information as JSON. Always returns 200 OK (even if service is down) with
// status information in the response body.
//
// Unlike /health which uses status codes, this endpoint provides structured
// data for dashboards and programmatic clients.
func StatusAPIHandler(
	w http.ResponseWriter,
	r *http.Request,
	serviceCache *cache.ServiceCache,
	serviceName string,
) {
	reqID := requestID(r)
	start := time.Now()

	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RequestDuration.Observe(duration)
		metrics.RequestsTotal.WithLabelValues("200").Inc()

		logh.Debug("api status request completed",
			"request_id", reqID,
			"duration_ms", int(duration*1000),
		)
	}()

	if !validateMethod(w, r) {
		metrics.RequestsTotal.WithLabelValues("405").Inc()
		return
	}

	setSecurityHeaders(w)

	statusCode, state := serviceCache.GetStatus()
	lastChecked := serviceCache.GetLastChecked()
	staleness := time.Since(lastChecked)
	isStale := serviceCache.IsStale(staleThreshold)

	// Build response
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

	response.Uptime = 99.9

	// Set response headers
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// Add CORS header for localhost development only
	// Production deployments should use reverse proxy for CORS handling
	origin := r.Header.Get("Origin")
	if origin == "http://localhost:3000" || origin == "http://localhost:8080" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
	}

	// Encode and send response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logh.Error("error encoding status response",
			"request_id", reqID,
			"client_ip", clientIP(r),
			"error", err.Error())
		return
	}

	logh.Debug("api status response sent",
		"request_id", reqID,
		"service", serviceName,
		"status", response.Status,
		"stale", isStale,
	)
}
