// -----------------------------------------------------------------------
// Background Service Checker
// -----------------------------------------------------------------------
//
// Package checker provides periodic healthchecking of systemd services via
// D-Bus with automatic reconnection and exponential backoff. It updates the
// shared cache and exports Prometheus metrics. Checker health is tracked
// separately to detect stuck or unresponsive goroutines.
//
// -----------------------------------------------------------------------

package checker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/metrics"
	"github.com/coreos/go-systemd/v22/dbus"
)

// -----------------------------------------------------------------------
// Systemd State Constants
// -----------------------------------------------------------------------

const (
	StateActive       = "active"
	StateInactive     = "inactive"
	StateFailed       = "failed"
	StateActivating   = "activating"
	StateDeactivating = "deactivating"
	StateReloading    = "reloading"
)

// -----------------------------------------------------------------------
// Reconnection Configuration
// -----------------------------------------------------------------------

const (
	initialRetryDelay = 1 * time.Second
	maxRetryDelay     = 30 * time.Second
	backoffMultiplier = 2
	checkTimeout      = 5 * time.Second
)

// -----------------------------------------------------------------------
// State Mapping
// -----------------------------------------------------------------------

var stateToStatusCode = map[string]int{
	StateActive:       http.StatusOK,
	StateInactive:     http.StatusServiceUnavailable,
	StateFailed:       http.StatusServiceUnavailable,
	StateActivating:   http.StatusServiceUnavailable,
	StateDeactivating: http.StatusServiceUnavailable,
	StateReloading:    http.StatusServiceUnavailable,
}

var logc = slog.Default().With("component", "checker")

// -----------------------------------------------------------------------
// Checker Health Tracking
// -----------------------------------------------------------------------

// CheckerHealth tracks whether the background checker goroutine is actively
// responding and updating the cache. This allows the watchdog to detect
// stuck or deadlocked checker goroutines that have stopped making progress.
type CheckerHealth struct {
	lastSuccessfulCheck time.Time
	mu                  sync.RWMutex
}

// NewCheckerHealth creates a new CheckerHealth tracker initialized to the
// current time (checker just started).
func NewCheckerHealth() *CheckerHealth {
	return &CheckerHealth{
		lastSuccessfulCheck: time.Now(),
	}
}

// RecordSuccess marks that a check completed successfully and updates the
// timestamp. Called after each successful cache update.
func (ch *CheckerHealth) RecordSuccess() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.lastSuccessfulCheck = time.Now()
}

// IsHealthy returns whether the checker has responded within maxAge of the
// current time. If the last successful check exceeds maxAge, the checker is
// considered stuck or unresponsive.
func (ch *CheckerHealth) IsHealthy(maxAge time.Duration) bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return time.Since(ch.lastSuccessfulCheck) < maxAge
}

// -----------------------------------------------------------------------
// Periodic Checker Loop
// -----------------------------------------------------------------------

// StartServiceChecker runs a periodic loop that polls the systemd service
// status and updates the shared cache. The loop respects context cancellation
// for graceful shutdown and automatically reconnects to D-Bus with exponential
// backoff on connection failures.
//
// Parameters:
//   - ctx: cancellation context; loop exits when done
//   - conn: initial D-Bus connection
//   - service: systemd unit name (without .service suffix)
//   - cache: shared cache for status updates
//   - interval: time between checks
//   - checkerHealth: health tracker updated on successful checks
func StartServiceChecker(
	ctx context.Context,
	conn *dbus.Conn,
	service string,
	cache *cache.ServiceCache,
	interval time.Duration,
	checkerHealth *CheckerHealth,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	currentConn := conn
	defer func() {
		if currentConn != nil {
			currentConn.Close()
		}
	}()

	// Perform immediate check on startup to ensure cache is populated quickly
	currentConn = CheckAndUpdateCacheWithReconnect(ctx, currentConn, service, cache)
	if currentConn != nil {
		checkerHealth.RecordSuccess()
	}

	for {
		select {
		case <-ticker.C:
			// Use a timeout context for the check to prevent D-Bus hangs
			// from blocking indefinitely
			checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
			currentConn = CheckAndUpdateCacheWithReconnect(checkCtx, currentConn, service, cache)
			cancel()

			if currentConn != nil {
				checkerHealth.RecordSuccess()
			}

		case <-ctx.Done():
			logc.Info("stopping service checker")
			return
		}
	}
}

// -----------------------------------------------------------------------
// D-Bus Connection Management
// -----------------------------------------------------------------------

// CheckAndUpdateCacheWithReconnect attempts a cache update with the current
// connection. On failure, it closes the connection and enters a reconnection
// loop with exponential backoff. The context is checked before each backoff
// wait to allow graceful shutdown during reconnection attempts.
//
// Returns the active D-Bus connection (or nil if ctx is cancelled).
func CheckAndUpdateCacheWithReconnect(
	ctx context.Context,
	conn *dbus.Conn,
	service string,
	cache *cache.ServiceCache,
) *dbus.Conn {
	// Try the check with current connection
	if err := CheckAndUpdateCache(ctx, conn, service, cache); err == nil {
		return conn
	}

	logc.Warn("D-Bus connection error; attempting reconnection")

	// Close old connection
	if conn != nil {
		conn.Close()
	}

	// Reconnection loop with exponential backoff
	attemptNum := 1
	retryDelay := initialRetryDelay

	for {
		// Check context before any wait operation to allow graceful shutdown
		select {
		case <-ctx.Done():
			logc.Info("shutdown requested during D-Bus reconnection",
				"attempt", attemptNum,
				"reason", ctx.Err().Error())
			return nil
		default:
		}

		// Attempt to establish new connection
		newConn, err := dbus.NewSystemConnectionContext(ctx)
		if err == nil {
			logc.Info("successfully reconnected to D-Bus",
				"attempt", attemptNum,
				"service", service)

			// Verify connection works with immediate check
			if checkErr := CheckAndUpdateCache(ctx, newConn, service, cache); checkErr == nil {
				return newConn
			}

			// Check failed, close this connection and retry
			logc.Warn("check failed on new connection; retrying",
				"attempt", attemptNum,
				"service", service)
			newConn.Close()
		} else {
			logc.Warn("failed to connect to D-Bus",
				"attempt", attemptNum,
				"error", err.Error())
		}

		// Wait before retry with context awareness for shutdown
		select {
		case <-ctx.Done():
			logc.Info("shutdown requested during reconnection backoff",
				"attempt", attemptNum,
				"reason", ctx.Err().Error())
			return nil

		case <-time.After(retryDelay):
			logc.Debug("reconnection backoff completed",
				"attempt", attemptNum,
				"next_delay", (retryDelay * backoffMultiplier).String())

			// Exponential backoff
			retryDelay *= backoffMultiplier
			if retryDelay > maxRetryDelay {
				retryDelay = maxRetryDelay
			}
			attemptNum++
		}
	}
}

// -----------------------------------------------------------------------
// Cache Update
// -----------------------------------------------------------------------

// CheckAndUpdateCache queries the systemd service status via D-Bus and
// updates the cache with the current state and HTTP status code. The
// provided context is used for the D-Bus call to respect timeouts and
// cancellation.
//
// Returns an error if the D-Bus query fails or produces unexpected data.
func CheckAndUpdateCache(
	ctx context.Context,
	conn *dbus.Conn,
	service string,
	cache *cache.ServiceCache,
) error {
	// Query service ActiveState from systemd via D-Bus
	prop, err := conn.GetUnitPropertyContext(ctx, service+".service", "ActiveState")
	if err != nil {
		logc.Error("error checking service via D-Bus",
			"service", service,
			"error", err.Error(),
			"context_err", ctx.Err())

		cache.UpdateStatus(http.StatusInternalServerError, "error")
		metrics.CheckFailures.WithLabelValues(service, "dbus_error").Inc()
		return err
	}

	// Extract the ActiveState value from D-Bus variant type
	activeStatus, ok := prop.Value.Value().(string)
	if !ok {
		logc.Error("unexpected type for ActiveState",
			"service", service,
			"type", fmt.Sprintf("%T", prop.Value.Value()))

		cache.UpdateStatus(http.StatusInternalServerError, "type_error")
		metrics.CheckFailures.WithLabelValues(service, "type_error").Inc()
		return fmt.Errorf("unexpected ActiveState type: %T", prop.Value.Value())
	}

	// Map systemd state to HTTP status code
	statusCode, found := stateToStatusCode[activeStatus]
	if !found {
		logc.Warn("unknown systemd state",
			"state", activeStatus,
			"service", service)
		statusCode = http.StatusInternalServerError
	}

	// Update cache with new status
	cache.UpdateStatus(statusCode, activeStatus)

	// Update Prometheus gauge
	if stateToStatusCode[activeStatus] == http.StatusOK {
		metrics.ServiceStatus.WithLabelValues(service, activeStatus).Set(1)
	} else {
		metrics.ServiceStatus.WithLabelValues(service, activeStatus).Set(0)
	}

	return nil
}
