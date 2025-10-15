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
// Resilience:
//   - Automatically reconnects to D-Bus if connection drops
//   - Uses exponential backoff for reconnection attempts
//   - Continues monitoring without manual intervention
//   - Logs reconnection attempts for observability
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
// Reconnection Constants
// -----------------------------------------------------------------------------

const (
	// Initial retry delay when D-Bus connection fails
	initialRetryDelay = 1 * time.Second

	// Maximum retry delay (caps exponential backoff)
	maxRetryDelay = 30 * time.Second

	// Backoff multiplier for exponential backoff
	backoffMultiplier = 2
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
// D-Bus Connection Management:
//
//	The checker maintains its own D-Bus connection and will automatically
//	reconnect if the connection drops. This ensures the service remains
//	operational even if D-Bus restarts or has issues.
//
// Parameters:
//   - ctx: Context for cancellation during graceful shutdown
//   - conn: Initial D-Bus connection (may be replaced on reconnect)
//   - service: Name of the systemd service to monitor (without .service suffix)
//   - cache: Thread-safe cache to update with status
//   - interval: Time between health checks
func StartServiceChecker(ctx context.Context, conn *dbus.Conn, service string, cache *cache.ServiceCache, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Current connection (may be replaced on reconnection)
	currentConn := conn
	defer func() {
		if currentConn != nil {
			currentConn.Close()
		}
	}()

	// Perform immediate check on startup to populate cache before HTTP requests
	currentConn = CheckAndUpdateCacheWithReconnect(ctx, currentConn, service, cache)

	for {
		select {
		case <-ticker.C:
			// Periodic check triggered by ticker
			// Connection may be replaced if reconnection is needed
			currentConn = CheckAndUpdateCacheWithReconnect(ctx, currentConn, service, cache)

		case <-ctx.Done():
			// Graceful shutdown requested
			log.Println("Stopping service checker")
			return
		}
	}
}

// -----------------------------------------------------------------------------
// Health Check with Reconnection Logic
// -----------------------------------------------------------------------------

// CheckAndUpdateCacheWithReconnect wraps CheckAndUpdateCache with automatic
// D-Bus reconnection logic. If the connection fails, it will attempt to
// reconnect with exponential backoff.
//
// Returns:
//   - The current D-Bus connection (may be a new connection if reconnected)
//
// Reconnection Strategy:
//   - Exponential backoff starting at 1s, maxing at 30s
//   - Continues retrying until successful or context cancelled
//   - Logs all reconnection attempts for debugging
func CheckAndUpdateCacheWithReconnect(ctx context.Context, conn *dbus.Conn, service string, cache *cache.ServiceCache) *dbus.Conn {
	// Try the check with current connection
	err := CheckAndUpdateCache(conn, service, cache)
	if err == nil {
		return conn // Success - return existing connection
	}

	// Connection might be dead - attempt reconnection
	log.Printf("D-Bus connection error, attempting reconnection: %v", err)

	// Close old connection if it exists
	if conn != nil {
		conn.Close()
	}

	// Reconnection loop with exponential backoff
	attemptNum := 1
	retryDelay := initialRetryDelay
	for {
		select {
		case <-ctx.Done():
			// Shutdown requested during reconnection
			return nil
		default:
			// Attempt to establish new connection
			newConn, err := dbus.NewSystemConnectionContext(ctx)
			if err == nil {
				log.Println("Successfully reconnected to D-Bus")

				// Immediately check with new connection
				if checkErr := CheckAndUpdateCache(newConn, service, cache); checkErr == nil {
					return newConn
				}
				// New connection failed check, close it and retry
				newConn.Close()
			}

			// Reconnection failed, wait before retry
			log.Printf("[Attempt %d] D-Bus reconnection failed, retrying in %v: %v",
				attemptNum, retryDelay, err)

			select {
			case <-time.After(retryDelay):
				// Exponential backoff with cap
				retryDelay *= backoffMultiplier
				if retryDelay > maxRetryDelay {
					retryDelay = maxRetryDelay
					attemptNum++
				}
			case <-ctx.Done():
				// Shutdown during backoff wait
				return nil
			}
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
// Returns an error if the D-Bus connection fails, allowing the caller to
// attempt reconnection.
//
// Error Handling:
//   - D-Bus communication errors return error (triggers reconnection)
//   - Type assertion failures return error and update cache with error state
//   - Unknown systemd states default to 500 but don't return error
//
// Metrics:
//
//	Sets the monitored_service_status gauge to 1 for healthy, 0 otherwise
func CheckAndUpdateCache(conn *dbus.Conn, service string, cache *cache.ServiceCache) error {
	// Query systemd for service's ActiveState property via D-Bus
	prop, err := conn.GetUnitPropertyContext(context.Background(), service+".service", "ActiveState")
	if err != nil {
		log.Printf("Error checking service %s: %v", service, err)
		cache.UpdateStatus(http.StatusInternalServerError, "error")
		metrics.CheckFailures.WithLabelValues(service, "dbus_error").Inc()
		return err
	}

	// Extract the ActiveState value from D-Bus variant type
	activeStatus, ok := prop.Value.Value().(string)
	if !ok {
		log.Printf("Unexpected type for ActiveState")
		cache.UpdateStatus(http.StatusInternalServerError, "type_error")
		metrics.CheckFailures.WithLabelValues(service, "type_error").Inc()
		return err // Return error to trigger reconnection
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

	return nil // Success
}
