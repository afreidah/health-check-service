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

// LoadConfig loads and validates configuration or returns error
// Previously MustLoadConfig - now returns error instead of exiting
func LoadConfig(logger *slog.Logger) (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration_failed",
			"error", err,
		)
		return nil, err
	}

	logger.Info("configuration_loaded",
		"service", cfg.Service,
		"port", cfg.Port,
		"interval", cfg.Interval,
		"tls_enabled", cfg.TLSEnabled,
		"tls_autocert", cfg.TLSAutocert,
	)

	return cfg, nil
}

// ConnectDBus establishes D-Bus connection and validates service exists
// Previously MustConnectDBus - now returns error instead of exiting
func ConnectDBus(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*dbus.Conn, error) {
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		logger.Error("dbus_connection_failed",
			"error", err,
		)
		return nil, err
	}

	// Validate service exists
	_, err = conn.GetUnitPropertyContext(ctx, cfg.Service+".service", "ActiveState")
	if err != nil {
		logger.Error("service_not_found",
			"service", cfg.Service,
			"error", err,
		)
		conn.Close()
		return nil, err
	}

	logger.Info("dbus_connected_and_validated",
		"service", cfg.Service,
	)

	return conn, nil
}

// SetupHTTPServer configures routes and TLS
func SetupHTTPServer(cfg *config.Config, serviceCache *cache.ServiceCache, dashboardHTML []byte, logger *slog.Logger) *http.Server {
	// Dashboard route
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(dashboardHTML); err != nil {
			logger.ErrorContext(r.Context(), "dashboard_write_failed",
				"error", err,
			)
		}
	})

	// Liveness probe - always responds 200 if process alive
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Readiness probe - responds 200 only if service is ready
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if serviceCache.IsStale(30 * time.Second) {
			logger.WarnContext(r.Context(), "readiness_check_stale")
			http.Error(w, "not ready - stale data", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Health endpoint - service health status
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.HealthHandler(w, r, serviceCache, logger)
	})

	// Status API
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		handlers.StatusAPIHandler(w, r, serviceCache, cfg.Service, logger)
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
	configureTLS(srv, cfg, logger)

	return srv
}

// configureTLS sets up TLS configuration
func configureTLS(srv *http.Server, cfg *config.Config, logger *slog.Logger) {
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

		logger.Info("autocert_enabled",
			"domain", cfg.TLSAutocertDomain,
			"cache_path", cfg.TLSAutocertCache,
		)

		// Start HTTP server for ACME challenges
		go func() {
			logger.Info("acme_challenge_server_starting", "port", 80)
			if err := http.ListenAndServe(":80", certManager.HTTPHandler(nil)); err != nil {
				logger.ErrorContext(context.Background(), "acme_challenge_server_failed",
					"error", err,
				)
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

		logger.Info("manual_tls_enabled",
			"cert_file", cfg.TLSCertFile,
			"key_file", cfg.TLSKeyFile,
		)
	}
}

// StartBackgroundChecker starts the service monitoring goroutine
func StartBackgroundChecker(conn *dbus.Conn, cfg *config.Config, serviceCache *cache.ServiceCache, logger *slog.Logger) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	interval := time.Duration(cfg.Interval) * time.Second
	go checker.StartServiceChecker(ctx, conn, cfg.Service, serviceCache, interval, logger)

	return cancel
}

// StartHTTPServer starts the HTTP/HTTPS server in a goroutine
func StartHTTPServer(srv *http.Server, cfg *config.Config, logger *slog.Logger) {
	go func() {
		var err error

		if cfg.TLSAutocert {
			logger.Info("http_server_starting",
				"mode", "https_autocert",
				"domain", cfg.TLSAutocertDomain,
				"port", cfg.Port,
				"service", cfg.Service,
			)
			err = srv.ListenAndServeTLS("", "")
		} else if cfg.TLSEnabled {
			logger.Info("http_server_starting",
				"mode", "https_manual",
				"port", cfg.Port,
				"service", cfg.Service,
			)
			err = srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			logger.Info("http_server_starting",
				"mode", "http",
				"port", cfg.Port,
				"service", cfg.Service,
			)
			err = srv.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			logger.ErrorContext(context.Background(), "http_server_failed",
				"error", err,
			)
		}
	}()
}

// WaitForShutdown blocks until shutdown signal, then gracefully stops
func WaitForShutdown(srv *http.Server, cancelChecker context.CancelFunc, logger *slog.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	logger.Info("shutdown_signal_received")

	// Stop background checker first
	cancelChecker()
	logger.Debug("background_checker_stopped")

	// Shutdown HTTP server with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.ErrorContext(context.Background(), "server_shutdown_error",
			"error", err,
		)
	}

	logger.Info("http_server_stopped")
}
