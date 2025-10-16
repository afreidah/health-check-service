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
	"log/slog"
	"net/http"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/metrics"
	"github.com/coreos/go-systemd/v22/dbus"
)

// Systemd ActiveState Constants
const (
	StateActive       = "active"
	StateInactive     = "inactive"
	StateFailed       = "failed"
	StateActivating   = "activating"
	StateDeactivating = "deactivating"
	StateReloading    = "reloading"
)

// Reconnection Constants
const (
	initialRetryDelay = 1 * time.Second
	maxRetryDelay     = 30 * time.Second
	backoffMultiplier = 2
)

// State to Status Code Mapping
var stateToStatusCode = map[string]int{
	StateActive:       http.StatusOK,
	StateInactive:     http.StatusServiceUnavailable,
	StateFailed:       http.StatusServiceUnavailable,
	StateActivating:   http.StatusServiceUnavailable,
	StateDeactivating: http.StatusServiceUnavailable,
	StateReloading:    http.StatusServiceUnavailable,
}

// StartServiceChecker runs background health check loop with structured logging
func StartServiceChecker(ctx context.Context, conn *dbus.Conn, service string,
	cache *cache.ServiceCache, interval time.Duration, logger *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	currentConn := conn
	defer func() {
		if currentConn != nil {
			currentConn.Close()
		}
	}()

	logger.DebugContext(ctx, "checker_started",
		"service", service,
		"interval_seconds", interval.Seconds(),
	)

	// Perform immediate check on startup
	currentConn = CheckAndUpdateCacheWithReconnect(ctx, currentConn, service, cache, logger)

	for {
		select {
		case <-ticker.C:
			currentConn = CheckAndUpdateCacheWithReconnect(ctx, currentConn, service, cache, logger)

		case <-ctx.Done():
			logger.DebugContext(ctx, "checker_stopping", "service", service)
			return
		}
	}
}

// CheckAndUpdateCacheWithReconnect wraps health check with automatic D-Bus reconnection
func CheckAndUpdateCacheWithReconnect(ctx context.Context, conn *dbus.Conn,
	service string, cache *cache.ServiceCache, logger *slog.Logger,
) *dbus.Conn {
	// Try the check with current connection
	err := CheckAndUpdateCache(conn, service, cache, logger)
	if err == nil {
		return conn // Success
	}

	// Connection might be dead - attempt reconnection
	logger.ErrorContext(ctx, "dbus_connection_failed",
		"service", service,
		"error", err,
	)

	// Close old connection
	if conn != nil {
		conn.Close()
	}

	// Reconnection loop with exponential backoff
	attemptNum := 1
	retryDelay := initialRetryDelay
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			// Attempt to establish new connection
			newConn, err := dbus.NewSystemConnectionContext(ctx)
			if err == nil {
				// Successfully connected, try a check
				if checkErr := CheckAndUpdateCache(newConn, service, cache, logger); checkErr == nil {
					logger.InfoContext(ctx, "dbus_reconnected",
						"service", service,
						"attempts_needed", attemptNum,
						"recovery_time_ms", time.Since(startTime).Milliseconds(),
					)
					return newConn
				}
				// Check failed on new connection, close and retry
				newConn.Close()
			}

			// Reconnection failed
			logger.WarnContext(ctx, "dbus_reconnection_attempt_failed",
				"service", service,
				"attempt", attemptNum,
				"retry_delay_ms", retryDelay.Milliseconds(),
				"error", err,
			)

			// Wait before retry (with early termination check)
			select {
			case <-time.After(retryDelay):
				// Exponential backoff with cap
				retryDelay *= backoffMultiplier
				if retryDelay > maxRetryDelay {
					retryDelay = maxRetryDelay
				}
				attemptNum++

			case <-ctx.Done():
				return nil
			}
		}
	}
}

// CheckAndUpdateCache queries systemd service status and updates cache
func CheckAndUpdateCache(conn *dbus.Conn, service string, cache *cache.ServiceCache,
	logger *slog.Logger,
) error {
	// Query systemd for ActiveState property
	prop, err := conn.GetUnitPropertyContext(context.Background(), service+".service", "ActiveState")
	if err != nil {
		logger.ErrorContext(context.Background(), "service_check_failed",
			"service", service,
			"error", err,
			"error_type", "dbus_error",
		)
		cache.UpdateStatus(http.StatusInternalServerError, "error")
		metrics.CheckFailures.WithLabelValues(service, "dbus_error").Inc()
		return err
	}

	// Extract ActiveState value
	activeStatus, ok := prop.Value.Value().(string)
	if !ok {
		logger.ErrorContext(context.Background(), "service_check_failed",
			"service", service,
			"error_type", "type_error",
			"details", "failed to parse ActiveState",
		)
		cache.UpdateStatus(http.StatusInternalServerError, "type_error")
		metrics.CheckFailures.WithLabelValues(service, "type_error").Inc()
		return err
	}

	// Map state to HTTP status code
	statusCode, found := stateToStatusCode[activeStatus]
	if !found {
		logger.WarnContext(context.Background(), "unknown_service_state",
			"service", service,
			"state", activeStatus,
		)
		statusCode = http.StatusInternalServerError
	}

	// Update cache
	cache.UpdateStatus(statusCode, activeStatus)

	// Update Prometheus metric
	if stateToStatusCode[activeStatus] == http.StatusOK {
		metrics.ServiceStatus.WithLabelValues(service, activeStatus).Set(1)
	} else {
		metrics.ServiceStatus.WithLabelValues(service, activeStatus).Set(0)
	}

	logger.DebugContext(context.Background(), "service_checked",
		"service", service,
		"state", activeStatus,
		"status_code", statusCode,
	)

	return nil // Success
}
