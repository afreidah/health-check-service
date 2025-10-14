// Package handlers
package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/metrics"
)

func HealthHandler(w http.ResponseWriter, r *http.Request, cache *cache.ServiceCache) {
	start := time.Now()
	var statusCode int

	defer func() {
		// Record metrics after handler completes
		duration := time.Since(start).Seconds()
		metrics.RequestDuration.Observe(duration)
		metrics.RequestsTotal.WithLabelValues(fmt.Sprintf("%d", statusCode)).Inc()
	}()

	statusCode, status := cache.GetStatus()
	log.Printf("Current status: %s", status)
	w.WriteHeader(statusCode)
}
