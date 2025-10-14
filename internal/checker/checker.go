// -----------------------------------------------------------------------------
// Background Service Checker
// -----------------------------------------------------------------------------
//
// This package implements the background health checking logic that monitors
// systemd services via D-Bus. It runs in a separate goroutine and periodically
// queries the service status, updating the shared cache with the results.
//
// Architecture:
//   - Runs as a long-lived background goroutine
//   - Uses a ticker for periodic health checks
//   - Communicates with systemd via D-Bus system bus
//   - Updates thread-safe cache with results
//   - Publishes metrics to Prometheus
//
// Systemd Integration:
//   The checker queries the "ActiveState" property of systemd units, which
//   can return values like "active", "inactive", "failed", etc. These states
//   are mapped to appropriate HTTP status codes for the health endpoint.
//
// Graceful Shutdown:
//   The checker respects context cancellation and will stop cleanly when
//   the main application initiates shutdown.
//
// -----------------------------------------------------------------------------

package checker

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/metrics"
	"github.com/coreos/go-systemd/v22/dbus"
)

// -----------------------------------------------------------------------------
// Systemd State Constants
// -----------------------------------------------------------------------------

// Systemd ActiveState values as defined by systemd specification.
// These represent the possible states a systemd service can be in.
const (
	StateActive       = "active"       // Service is running
	StateInactive     = "inactive"     // Service is stopped
	StateFailed       = "failed"       // Service has failed
	StateActivating   = "activating"   // Service is starting up
	StateDeactivating = "deactivating" // Service is shutting down
	StateReloading    = "reloading"    // Service is reloading config
)

// -----------------------------------------------------------------------------
// State Mapping
// -----------------------------------------------------------------------------

// stateToStatusCode maps systemd ActiveState values to HTTP status codes.
// Only "active" returns 200 OK - all other states indicate the service
// is not fully operational and return 503 Service Unavailable.
var stateToStatusCode = map[string]int{
	StateActive:       http.StatusOK,                 // 200 - Service healthy
	StateInactive:     http.StatusServiceUnavailable, // 503 - Service stopped
	StateFailed:       http.StatusServiceUnavailable, // 503 - Service crashed
	StateActivating:   http.StatusServiceUnavailable, // 503 - Service starting
	StateDeactivating: http.StatusServiceUnavailable, // 503 - Service stopping
	StateReloading:    http.StatusServiceUnavailable, // 503 - Service reloading
}

// -----------------------------------------------------------------------------
// Background Checker
// -----------------------------------------------------------------------------

// StartServiceChecker runs a background loop that periodically checks the
// systemd service status and updates the cache. This function blocks until
// the context is cancelled.
//
// The checker performs an immediate check on startup to populate the cache
// before any HTTP requests arrive, then continues checking at the specified
// interval.
//
// Parameters:
//   - ctx: Context for cancellation during graceful shutdown
//   - conn: D-Bus connection to systemd
//   - service: Name of the systemd service to monitor (without .service suffix)
//   - cache: Thread-safe cache to update with status
//   - interval: Time between health checks
func StartServiceChecker(ctx context.Context, conn *dbus.Conn, service string, cache *cache.ServiceCache, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Perform immediate check on startup to populate cache before HTTP requests
	CheckAndUpdateCache(conn, service, cache)

	for {
		select {
		case <-ticker.C:
			// Periodic check triggered by ticker
			CheckAndUpdateCache(conn, service, cache)

		case <-ctx.Done():
			// Graceful shutdown requested
			log.Println("Stopping service checker")
			return
		}
	}
}

// -----------------------------------------------------------------------------
// Health Check Logic
// -----------------------------------------------------------------------------

// CheckAndUpdateCache queries systemd for the service's ActiveState via D-Bus,
// maps it to an HTTP status code, updates the cache, and records Prometheus
// metrics.
//
// Error Handling:
//   - D-Bus communication errors return 500 Internal Server Error
//   - Type assertion failures return 500 with "type_error" state
//   - Unknown systemd states default to 500
//
// Metrics:
//
//	Sets the monitored_service_status gauge to 1 for healthy, 0 otherwise
func CheckAndUpdateCache(conn *dbus.Conn, service string, cache *cache.ServiceCache) {
	// Query systemd for service's ActiveState property via D-Bus
	prop, err := conn.GetUnitPropertyContext(context.Background(), service+".service", "ActiveState")
	if err != nil {
		log.Printf("Error checking service %s: %v", service, err)
		cache.UpdateStatus(http.StatusInternalServerError, "error")
		return
	}

	// Extract the ActiveState value from D-Bus variant type
	activeStatus, ok := prop.Value.Value().(string)
	if !ok {
		log.Printf("Unexpected type for ActiveState")
		cache.UpdateStatus(http.StatusInternalServerError, "type_error")
		return
	}

	// Map systemd state to HTTP status code
	statusCode, found := stateToStatusCode[activeStatus]
	if !found {
		log.Printf("Unknown systemd state: %s", activeStatus)
		statusCode = http.StatusInternalServerError
	}

	// Update cache with new status
	cache.UpdateStatus(statusCode, activeStatus)

	// Update Prometheus gauge: 1 for healthy (active), 0 for any other state
	if stateToStatusCode[activeStatus] == http.StatusOK {
		metrics.ServiceStatus.WithLabelValues(service, activeStatus).Set(1)
	} else {
		metrics.ServiceStatus.WithLabelValues(service, activeStatus).Set(0)
	}
}
