// Package main
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
	// Load and validate configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Connect to D-Bus
	ctx := context.Background()
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to D-Bus: %v", err)
	}
	defer conn.Close()

	// Verify service exists in systemd (fail fast)
	_, err = conn.GetUnitPropertyContext(ctx, cfg.Service+".service", "ActiveState")
	if err != nil {
		log.Fatalf("Service '%s' not found in systemd: %v", cfg.Service, err)
	}
	log.Printf("Successfully validated service: %s", cfg.Service)

	// Initialize cache
	serviceCache := cache.New()

	// Setup HTTP handlers
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.HealthHandler(w, r, serviceCache)
	})
	http.Handle("/metrics", promhttp.Handler())

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Create cancellable context for background checker
	checkerCtx, checkerCancel := context.WithCancel(context.Background())
	defer checkerCancel()

	// Start background service checker
	interval := time.Duration(cfg.Interval) * time.Second
	go checker.StartServiceChecker(checkerCtx, conn, cfg.Service, serviceCache, interval)

	// Start HTTP server in goroutine
	go func() {
		log.Printf("Monitoring %s on :%d", cfg.Service, cfg.Port)
		log.Printf("Metrics available at :%d/metrics", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutdown signal received, gracefully shutting down...")

	// Stop background checker first
	checkerCancel()

	// Shutdown HTTP server with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
