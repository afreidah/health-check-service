// -----------------------------------------------------------------------------
// Background Service Checker
// -----------------------------------------------------------------------------
// Background health-check loop for systemd-managed services:
// - Periodic ActiveState polling via D-Bus
// - Thread-safe cache updates + Prometheus metrics
// - Exponential backoff reconnection to D-Bus with proper shutdown handling
// - Context-aware graceful shutdown
// - Checker health monitoring
// -----------------------------------------------------------------------------

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

// -----------------------------------------------------------------------------
// Systemd State Constants
// -----------------------------------------------------------------------------

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
	initialRetryDelay = 1 * time.Second
	maxRetryDelay     = 30 * time.Second
	backoffMultiplier = 2

	// CHANGED: Add timeout for individual D-Bus checks
	// If a check takes longer than this, something is wrong
	checkTimeout = 5 * time.Second
)

// -----------------------------------------------------------------------------
// State Mapping
// -----------------------------------------------------------------------------

var stateToStatusCode = map[string]int{
	StateActive:       http.StatusOK,
	StateInactive:     http.StatusServiceUnavailable,
	StateFailed:       http.StatusServiceUnavailable,
	StateActivating:   http.StatusServiceUnavailable,
	StateDeactivating: http.StatusServiceUnavailable,
	StateReloading:    http.StatusServiceUnavailable,
}

var logc = slog.Default().With("component", "checker")

// =============================================================================
// CHANGED: Add CheckerHealth type to track checker health
// =============================================================================
// This allows the main app to detect if the checker is stuck/deadlocked
type CheckerHealth struct {
	lastSuccessfulCheck time.Time
	mu                  sync.RWMutex
}

func NewCheckerHealth() *CheckerHealth {
	return &CheckerHealth{
		lastSuccessfulCheck: time.Now(),
	}
}

// RecordSuccess marks that a check completed successfully
func (ch *CheckerHealth) RecordSuccess() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.lastSuccessfulCheck = time.Now()
}

// IsHealthy returns true if a check completed within the expected interval
func (ch *CheckerHealth) IsHealthy(maxAge time.Duration) bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return time.Since(ch.lastSuccessfulCheck) < maxAge
}

// =============================================================================

// StartServiceChecker launches a goroutine that periodically polls the given
// systemd unit and updates the shared cache until ctx is cancelled.
//
// CHANGED: Added checkerHealth parameter to track checker health
// This allows detecting if the checker goroutine is stuck
//
// Params:
//   - ctx: cancellation context (NOW PROPERLY RESPECTED)
//   - conn: initial D-Bus connection
//   - service: systemd unit name
//   - cache: shared status cache
//   - interval: polling period
//   - checkerHealth: tracks if checker is responding
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

	// Perform immediate check on startup
	currentConn = CheckAndUpdateCacheWithReconnect(ctx, currentConn, service, cache)
	if currentConn != nil {
		checkerHealth.RecordSuccess()
	}

	for {
		select {
		case <-ticker.C:
			// CHANGED: Use a timeout context for the check itself
			// This ensures that even if D-Bus hangs, we don't block forever
			checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
			currentConn = CheckAndUpdateCacheWithReconnect(checkCtx, currentConn, service, cache)
			cancel()

			// CHANGED: Record successful check
			if currentConn != nil {
				checkerHealth.RecordSuccess()
			}

		case <-ctx.Done():
			logc.Info("stopping service checker")
			return
		}
	}
}

// =============================================================================
// CheckAndUpdateCacheWithReconnect - FIXED ISSUES
// =============================================================================
// Changes:
// 1. Properly respects context cancellation (checked BEFORE time.After)
// 2. Context parameter is now used instead of hardcoded Background()
// 3. Better logging for reconnection attempts
func CheckAndUpdateCacheWithReconnect(
	ctx context.Context,
	conn *dbus.Conn,
	service string,
	cache *cache.ServiceCache,
) *dbus.Conn {
	// Try the check with current connection
	if err := CheckAndUpdateCache(ctx, conn, service, cache); err == nil {
		return conn // Success
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
		// CHANGED: Check context FIRST before any operation
		// This ensures graceful shutdown isn't blocked by time.After
		select {
		case <-ctx.Done():
			logc.Info("shutdown requested during D-Bus reconnection",
				"attempt", attemptNum,
				"reason", ctx.Err().Error())
			return nil
		default:
			// Continue with reconnection attempt
		}

		// Attempt to establish new connection
		newConn, err := dbus.NewSystemConnectionContext(ctx)
		if err == nil {
			logc.Info("successfully reconnected to D-Bus",
				"attempt", attemptNum,
				"service", service)

			// Verify connection works with immediate check
			if checkErr := CheckAndUpdateCache(ctx, newConn, service, cache); checkErr == nil {
				return newConn // Success!
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

		// CHANGED: Use select to check context BEFORE waiting
		// This is the critical fix for graceful shutdown
		select {
		case <-ctx.Done():
			// Shutdown during backoff - stop immediately
			logc.Info("shutdown requested during reconnection backoff",
				"attempt", attemptNum,
				"reason", ctx.Err().Error())
			return nil

		case <-time.After(retryDelay):
			// Backoff wait completed, try again
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

// =============================================================================
// CheckAndUpdateCache - FIXED CONTEXT HANDLING
// =============================================================================
// Changes:
// 1. Now accepts context parameter instead of using context.Background()
// 2. Context is passed to D-Bus call so it can be cancelled
// 3. Respects timeouts and shutdown signals
func CheckAndUpdateCache(
	ctx context.Context,
	conn *dbus.Conn,
	service string,
	cache *cache.ServiceCache,
) error {
	// CHANGED: Use provided context instead of Background()
	// This respects cancellation during shutdown and respects timeouts
	prop, err := conn.GetUnitPropertyContext(ctx, service+".service", "ActiveState")
	if err != nil {
		// CHANGED: Better error logging with context info
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

	return nil // Success
}
