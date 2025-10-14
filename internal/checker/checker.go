// Packer checker
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

// Systemd ActiveState values
const (
	StateActive       = "active"
	StateInactive     = "inactive"
	StateFailed       = "failed"
	StateActivating   = "activating"
	StateDeactivating = "deactivating"
	StateReloading    = "reloading"
)

// Values map of status => http code
// Map of systemd ActiveState to HTTP status codes
var stateToStatusCode = map[string]int{
	StateActive:       http.StatusOK,
	StateInactive:     http.StatusServiceUnavailable,
	StateFailed:       http.StatusServiceUnavailable,
	StateActivating:   http.StatusServiceUnavailable,
	StateDeactivating: http.StatusServiceUnavailable,
	StateReloading:    http.StatusServiceUnavailable,
}

func StartServiceChecker(ctx context.Context, conn *dbus.Conn, service string, cache *cache.ServiceCache, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Do an immediate check on startup
	CheckAndUpdateCache(conn, service, cache)

	for {
		select {
		case <-ticker.C:
			// Ticker fired - time to check
			CheckAndUpdateCache(conn, service, cache)
		case <-ctx.Done():
			// Context cancelled - time to shut down
			log.Println("Stopping service checker")
			return
		}
	}
}

func CheckAndUpdateCache(conn *dbus.Conn, service string, cache *cache.ServiceCache) {
	prop, err := conn.GetUnitPropertyContext(context.Background(), service+".service", "ActiveState")
	if err != nil {
		log.Printf("Error checking service %s: %v", service, err)
		cache.UpdateStatus(http.StatusInternalServerError, "error")
		return
	}

	activeStatus, ok := prop.Value.Value().(string)
	if !ok {
		log.Printf("Unexpected type for ActiveState")
		cache.UpdateStatus(http.StatusInternalServerError, "type_error")
		return
	}

	statusCode, found := stateToStatusCode[activeStatus]
	if !found {
		log.Printf("Unknown systemd state: %s", activeStatus)
		statusCode = http.StatusInternalServerError
	}
	cache.UpdateStatus(statusCode, activeStatus)

	// Update Prometheus gauge
	if stateToStatusCode[activeStatus] == http.StatusOK {
		metrics.ServiceStatus.WithLabelValues(service, activeStatus).Set(1)
	} else {
		metrics.ServiceStatus.WithLabelValues(service, activeStatus).Set(0)
	}
}
