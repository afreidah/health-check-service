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
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "embed"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/checker"
	"github.com/afreidah/health-check-service/internal/config"
	"github.com/afreidah/health-check-service/internal/handlers"
	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
)

//go:embed static/dashboard.html
var dashboardHTML []byte

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

	// Print banner
	log.Printf("==========================================")
	log.Printf("Health Check Service Starting")
	log.Printf("Service: %s | Port: %d | Interval: %ds",
		cfg.Service, cfg.Port, cfg.Interval)
	log.Printf("TLS: %t | Autocert: %t", cfg.TLSEnabled, cfg.TLSAutocert)
	log.Printf("==========================================")

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
	serviceCache := cache.New()

	// Dashboard route - serves embedded HTML
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(dashboardHTML)
	})

	// Health endpoint - returns service status with appropriate HTTP code
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.HealthHandler(w, r, serviceCache)
	})

	// Status API - returns JSON status for dashboard
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		handlers.StatusAPIHandler(w, r, serviceCache, cfg.Service)
	})

	// Prometheus metrics endpoint
	http.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// -------------------------------------------------------------------------
	// TLS Configuration
	// -------------------------------------------------------------------------
	var certManager *autocert.Manager

	if cfg.TLSAutocert {
		// Let's Encrypt with autocert
		certManager = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.TLSAutocertDomain),
			Cache:      autocert.DirCache(cfg.TLSAutocertCache),
			Email:      cfg.TLSAutocertEmail,
		}

		srv.TLSConfig = &tls.Config{
			GetCertificate: certManager.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		}

		log.Printf("Let's Encrypt autocert enabled for domain: %s", cfg.TLSAutocertDomain)

		// Start HTTP server for ACME challenges on port 80
		go func() {
			log.Println("Starting HTTP server on :80 for ACME challenges")
			if err := http.ListenAndServe(":80", certManager.HTTPHandler(nil)); err != nil {
				log.Printf("ACME challenge server error: %v", err)
			}
		}()

	} else if cfg.TLSEnabled {
		// Manual certificate files
		srv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			},
		}
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
	// HTTP/HTTPS Server Startup
	// -------------------------------------------------------------------------
	go func() {
		if cfg.TLSAutocert {
			log.Printf("Monitoring %s on :%d (HTTPS with Let's Encrypt)", cfg.Service, cfg.Port)
			log.Printf("Metrics available at https://%s:%d/metrics", cfg.TLSAutocertDomain, cfg.Port)
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTPS server failed: %v", err)
			}
		} else if cfg.TLSEnabled {
			log.Printf("Monitoring %s on :%d (HTTPS with manual certs)", cfg.Service, cfg.Port)
			log.Printf("Metrics available at https://localhost:%d/metrics", cfg.Port)
			if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTPS server failed: %v", err)
			}
		} else {
			log.Printf("Monitoring %s on :%d (HTTP)", cfg.Service, cfg.Port)
			log.Printf("Metrics available at http://localhost:%d/metrics", cfg.Port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server failed: %v", err)
			}
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
