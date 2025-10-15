package app

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

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/checker"
	"github.com/afreidah/health-check-service/internal/config"
	"github.com/afreidah/health-check-service/internal/handlers"
	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
)

// MustLoadConfig loads and validates configuration or exits
func MustLoadConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("==========================================")
	log.Printf("Health Check Service Starting")
	log.Printf("Service: %s | Port: %d | Interval: %ds",
		cfg.Service, cfg.Port, cfg.Interval)
	log.Printf("TLS: %t | Autocert: %t", cfg.TLSEnabled, cfg.TLSAutocert)
	log.Printf("==========================================")

	return cfg
}

// MustConnectDBus establishes D-Bus connection and validates service exists
func MustConnectDBus(ctx context.Context, cfg *config.Config) *dbus.Conn {
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to D-Bus: %v", err)
	}

	// Validate service exists
	_, err = conn.GetUnitPropertyContext(ctx, cfg.Service+".service", "ActiveState")
	if err != nil {
		log.Fatalf("Service '%s' not found in systemd: %v", cfg.Service, err)
	}
	log.Printf("Successfully validated service: %s", cfg.Service)

	return conn
}

// SetupHTTPServer configures routes and TLS
func SetupHTTPServer(cfg *config.Config, serviceCache *cache.ServiceCache, dashboardHTML []byte) *http.Server {
	// Dashboard route
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(dashboardHTML); err != nil {
			log.Printf("Error writing dashboard: %v", err)
		}
	})

	// Health endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.HealthHandler(w, r, serviceCache)
	})

	// Status API
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		handlers.StatusAPIHandler(w, r, serviceCache, cfg.Service)
	})

	// Prometheus metrics
	http.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Configure TLS
	configureTLS(srv, cfg)

	return srv
}

// configureTLS sets up TLS configuration
func configureTLS(srv *http.Server, cfg *config.Config) {
	if cfg.TLSAutocert {
		certManager := &autocert.Manager{
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

		// Start HTTP server for ACME challenges
		go func() {
			log.Println("Starting HTTP server on :80 for ACME challenges")
			if err := http.ListenAndServe(":80", certManager.HTTPHandler(nil)); err != nil {
				log.Printf("ACME challenge server error: %v", err)
			}
		}()

	} else if cfg.TLSEnabled {
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
}

// StartBackgroundChecker starts the service monitoring goroutine
func StartBackgroundChecker(conn *dbus.Conn, cfg *config.Config, serviceCache *cache.ServiceCache) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	interval := time.Duration(cfg.Interval) * time.Second
	go checker.StartServiceChecker(ctx, conn, cfg.Service, serviceCache, interval)

	return cancel
}

// StartHTTPServer starts the HTTP/HTTPS server in a goroutine
func StartHTTPServer(srv *http.Server, cfg *config.Config) {
	go func() {
		var err error

		if cfg.TLSAutocert {
			log.Printf("Monitoring %s on :%d (HTTPS with Let's Encrypt)", cfg.Service, cfg.Port)
			log.Printf("Dashboard: https://%s:%d/", cfg.TLSAutocertDomain, cfg.Port)
			log.Printf("Health: https://%s:%d/health", cfg.TLSAutocertDomain, cfg.Port)
			log.Printf("API: https://%s:%d/api/status", cfg.TLSAutocertDomain, cfg.Port)
			log.Printf("Metrics: https://%s:%d/metrics", cfg.TLSAutocertDomain, cfg.Port)
			err = srv.ListenAndServeTLS("", "")
		} else if cfg.TLSEnabled {
			log.Printf("Monitoring %s on :%d (HTTPS with manual certs)", cfg.Service, cfg.Port)
			log.Printf("Dashboard: https://localhost:%d/", cfg.Port)
			log.Printf("Health: https://localhost:%d/health", cfg.Port)
			log.Printf("API: https://localhost:%d/api/status", cfg.Port)
			log.Printf("Metrics: https://localhost:%d/metrics", cfg.Port)
			err = srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			log.Printf("Monitoring %s on :%d (HTTP)", cfg.Service, cfg.Port)
			log.Printf("Dashboard: http://localhost:%d/", cfg.Port)
			log.Printf("Health: http://localhost:%d/health", cfg.Port)
			log.Printf("API: http://localhost:%d/api/status", cfg.Port)
			log.Printf("Metrics: http://localhost:%d/metrics", cfg.Port)
			err = srv.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()
}

// WaitForShutdown blocks until shutdown signal, then gracefully stops
func WaitForShutdown(srv *http.Server, cancelChecker context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutdown signal received, gracefully shutting down...")

	// Stop background checker first
	cancelChecker()

	// Shutdown HTTP server with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
