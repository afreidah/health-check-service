// Package app
package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
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

// initLogger configures slog to emit structured JSON to stdout (captured by systemd/journald)
func initLogger() {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo, // set to slog.LevelDebug locally for more verbosity
	})
	slog.SetDefault(slog.New(h))
}

// MustLoadConfig loads and validates configuration or exits
func MustLoadConfig() *config.Config {
	// initialize structured logging first so any errors are emitted via slog
	initLogger()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}

	slog.Info("==========================================")
	slog.Info("Health Check Service Starting",
		"service", cfg.Service,
		"port", cfg.Port,
		"interval_sec", cfg.Interval,
	)
	slog.Info("TLS/Autocert settings",
		"tls_enabled", cfg.TLSEnabled,
		"autocert", cfg.TLSAutocert,
	)
	slog.Info("==========================================")

	return cfg
}

// MustConnectDBus establishes D-Bus connection and validates service exists
func MustConnectDBus(ctx context.Context, cfg *config.Config) *dbus.Conn {
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		slog.Error("failed to connect to D-Bus", "err", err)
		os.Exit(1)
	}

	// Validate service exists
	if _, err := conn.GetUnitPropertyContext(ctx, cfg.Service+".service", "ActiveState"); err != nil {
		slog.Error("service not found in systemd", "service", cfg.Service, "err", err)
		os.Exit(1)
	}
	slog.Info("successfully validated service", "service", cfg.Service)

	return conn
}

// SetupHTTPServer configures routes and TLS
func SetupHTTPServer(cfg *config.Config, serviceCache *cache.ServiceCache, dashboardHTML []byte) *http.Server {
	// Dashboard route
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(dashboardHTML); err != nil {
			slog.Error("error writing dashboard", "err", err)
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

		slog.Info("Let's Encrypt autocert enabled", "domain", cfg.TLSAutocertDomain)

		// Start HTTP server for ACME challenges
		go func() {
			slog.Info("starting HTTP server for ACME challenges", "addr", ":80")
			if err := http.ListenAndServe(":80", certManager.HTTPHandler(nil)); err != nil {
				slog.Error("ACME challenge server error", "err", err)
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
			slog.Info("monitoring (HTTPS with Let's Encrypt)",
				"service", cfg.Service, "port", cfg.Port, "domain", cfg.TLSAutocertDomain)
			slog.Info("endpoints",
				"dashboard", fmt.Sprintf("https://%s:%d/", cfg.TLSAutocertDomain, cfg.Port),
				"health", fmt.Sprintf("https://%s:%d/health", cfg.TLSAutocertDomain, cfg.Port),
				"api", fmt.Sprintf("https://%s:%d/api/status", cfg.TLSAutocertDomain, cfg.Port),
				"metrics", fmt.Sprintf("https://%s:%d/metrics", cfg.TLSAutocertDomain, cfg.Port),
			)
			err = srv.ListenAndServeTLS("", "")
		} else if cfg.TLSEnabled {
			slog.Info("monitoring (HTTPS with manual certs)",
				"service", cfg.Service, "port", cfg.Port)
			slog.Info("endpoints",
				"dashboard", fmt.Sprintf("https://localhost:%d/", cfg.Port),
				"health", fmt.Sprintf("https://localhost:%d/health", cfg.Port),
				"api", fmt.Sprintf("https://localhost:%d/api/status", cfg.Port),
				"metrics", fmt.Sprintf("https://localhost:%d/metrics", cfg.Port),
			)
			err = srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			slog.Info("monitoring (HTTP)", "service", cfg.Service, "port", cfg.Port)
			slog.Info("endpoints",
				"dashboard", fmt.Sprintf("http://localhost:%d/", cfg.Port),
				"health", fmt.Sprintf("http://localhost:%d/health", cfg.Port),
				"api", fmt.Sprintf("http://localhost:%d/api/status", cfg.Port),
				"metrics", fmt.Sprintf("http://localhost:%d/metrics", cfg.Port),
			)
			err = srv.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			slog.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()
}

// WaitForShutdown blocks until shutdown signal, then gracefully stops
func WaitForShutdown(srv *http.Server, cancelChecker context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	slog.Info("shutdown signal received; gracefully shutting down...")

	// Stop background checker first
	cancelChecker()

	// Shutdown HTTP server with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "err", err)
	}

	slog.Info("server stopped")
}
