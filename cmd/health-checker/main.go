// -----------------------------------------------------------------------------
// Health Check Service
// -----------------------------------------------------------------------------
//
// A lightweight systemd service health checker with Prometheus metrics.
//
// This service connects to the system D-Bus to monitor systemd services and
// exposes their health status via HTTP endpoints. It continuously polls the
// configured service and updates an in-memory cache with the current state.
//
// Features:
//   - Systemd service monitoring via D-Bus
//   - HTTP health endpoint with appropriate status codes
//   - Prometheus metrics for monitoring and alerting
//   - Thread-safe status caching
//   - Graceful shutdown with configurable timeout
//
// Usage:
//   health-checker --service nginx --port 8080 --interval 10
//
// Environment Variables:
//   HEALTH_SERVICE  - Service name to monitor
//   HEALTH_PORT     - HTTP port to listen on
//   HEALTH_INTERVAL - Check interval in seconds
//
// Author: Alex Freidah <alex.freidah@gmail.com>
// License: Apache 2.0
// -----------------------------------------------------------------------------

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/checker"
	"github.com/afreidah/health-check-service/internal/config"
	"github.com/afreidah/health-check-service/internal/handlers"
	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// -------------------------------------------------------------------------
	// Configuration & D-Bus Connection
	// -------------------------------------------------------------------------
	// Load configuration from flags, environment variables, or YAML file
	// with precedence: flags > env > file > defaults
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Establish connection to system D-Bus for communicating with systemd
	ctx := context.Background()
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to D-Bus: %v", err)
	}
	defer conn.Close()

	// -------------------------------------------------------------------------
	// Service Validation
	// -------------------------------------------------------------------------
	// Validate service exists early to fail fast before starting HTTP server
	_, err = conn.GetUnitPropertyContext(ctx, cfg.Service+".service", "ActiveState")
	if err != nil {
		log.Fatalf("Service '%s' not found in systemd: %v", cfg.Service, err)
	}
	log.Printf("Successfully validated service: %s", cfg.Service)

	// -------------------------------------------------------------------------
	// HTTP Server Setup
	// -------------------------------------------------------------------------
	// Initialize thread-safe cache for service status
	serviceCache := cache.New()

	// Register HTTP handlers for health checks and metrics
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.HealthHandler(w, r, serviceCache)
	})
	http.Handle("/metrics", promhttp.Handler())

	// Configure HTTP server with production timeouts
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// -------------------------------------------------------------------------
	// Background Service Checker
	// -------------------------------------------------------------------------
	// Create cancellable context for background checker lifecycle management
	checkerCtx, checkerCancel := context.WithCancel(context.Background())
	defer checkerCancel()

	// Start background goroutine to periodically check service status
	interval := time.Duration(cfg.Interval) * time.Second
	go checker.StartServiceChecker(checkerCtx, conn, cfg.Service, serviceCache, interval)

	// -------------------------------------------------------------------------
	// HTTP Server Startup
	// -------------------------------------------------------------------------
	// Run HTTP server in background goroutine to allow graceful shutdown
	go func() {
		log.Printf("Monitoring %s on :%d", cfg.Service, cfg.Port)
		log.Printf("Metrics available at :%d/metrics", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// -------------------------------------------------------------------------
	// Graceful Shutdown
	// -------------------------------------------------------------------------
	// Use buffered channel to prevent signal loss during handler execution
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Block until shutdown signal received
	<-sigChan
	log.Println("Shutdown signal received, gracefully shutting down...")

	// Stop background checker first to prevent new cache updates
	checkerCancel()

	// Shutdown HTTP server with timeout, allowing in-flight requests to complete
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
