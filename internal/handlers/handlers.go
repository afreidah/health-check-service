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

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/metrics"
)

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
		metrics.RequestsTotal.WithLabelValues(fmt.Sprintf("%d", statusCode)).Inc()
	}()

	// -------------------------------------------------------------------------
	// Read Status from Cache
	// -------------------------------------------------------------------------
	// Fetch current status from cache (updated by background checker)
	// This is a fast, non-blocking read operation
	statusCode, status := cache.GetStatus()
	log.Printf("Current status: %s", status)

	// -------------------------------------------------------------------------
	// Send HTTP Response
	// -------------------------------------------------------------------------
	// Return only the status code with no body
	// Monitoring systems typically only check the HTTP status
	w.WriteHeader(statusCode)
}
