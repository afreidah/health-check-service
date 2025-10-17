// Package app provides the core application orchestration for the Health Check Service.
//
// This package is responsible for coordinating all major service components:
// structured logging initialization, HTTP server configuration with TLS support,
// D-Bus connectivity for systemd service monitoring, and graceful shutdown sequences.
// It also implements watchdog functionality to detect and alert on checker goroutine
// health degradation.
//
// The service follows a clean separation of concerns: HTTP routing is delegated to
// handlers, systemd queries to the checker package, and data persistence to the cache.
// The app module acts as the composition root, wiring these components together and
// managing their lifecycle.
package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/checker"
	"github.com/afreidah/health-check-service/internal/config"
	"github.com/afreidah/health-check-service/internal/handlers"
	"github.com/afreidah/health-check-service/internal/logging"
	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/crypto/acme/autocert"
)

// Build-time metadata injected via linker flags during compilation.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var loga = slog.Default().With("component", "app")

// Checker Health Metrics
//
// These metrics track the health and responsiveness of the background checker
// goroutine. They provide visibility into whether the service is actively
// monitoring the target systemd unit or if the checker has become unresponsive.

var (
	// checkerHealthy indicates whether the background checker goroutine
	// is responsive and updating health information. Set to 1 when healthy,
	// 0 when the checker has failed to update within the expected interval.
	checkerHealthy = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_checker_healthy",
			Help: "Whether the background checker goroutine is responding (1=yes, 0=no)",
		},
	)

	// checkerLastCheckTimestamp records the Unix timestamp of the most recent
	// successful health check. This metric enables detection of stale data
	// and provides evidence that the checker is actively running.
	checkerLastCheckTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_checker_last_check_timestamp_seconds",
			Help: "Unix timestamp of the last successful health check",
		},
	)
)

func init() {
	prometheus.MustRegister(checkerHealthy)
	prometheus.MustRegister(checkerLastCheckTimestamp)
}

// Configuration & D-Bus Setup
//
// This section handles initial application configuration loading and validation,
// followed by establishing the D-Bus system connection required for systemd queries.

// MustLoadConfig loads application configuration from environment variables
// and performs validation. If configuration is invalid or incomplete, the
// service will log an error and exit with status code 1. The logger is
// initialized twice: first with generic metadata, then re-initialized with
// the monitored service name as a permanent log context field.
func MustLoadConfig() *config.Config {
	// Initialize structured logging first with generic metadata
	logging.InitFromEnv(map[string]string{
		"service":    "health-check-service",
		"version":    version,
		"commit":     commit,
		"build_date": date,
	})

	cfg, err := config.Load()
	if err != nil {
		loga.Error("configuration error", "err", err)
		os.Exit(1)
	}

	// Re-initialize logging with the monitored service name as static context
	logging.InitFromEnv(map[string]string{
		"service":    "health-check-service",
		"unit":       cfg.Service,
		"version":    version,
		"commit":     commit,
		"build_date": date,
	})

	loga = slog.Default().With("component", "app")

	loga.Info("Health Check Service Starting",
		"service", cfg.Service,
		"port", cfg.Port,
		"interval_sec", cfg.Interval,
	)
	loga.Info("TLS/Autocert settings",
		"tls_enabled", cfg.TLSEnabled,
		"autocert", cfg.TLSAutocert,
	)

	return cfg
}

// MustConnectDBus establishes a connection to the systemd D-Bus service and
// validates that the target service exists in the current systemd configuration.
// If the connection fails or the service cannot be found, the application exits
// with status code 1 after logging the error condition.
func MustConnectDBus(ctx context.Context, cfg *config.Config) *dbus.Conn {
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		loga.Error("failed to connect to D-Bus", "err", err)
		os.Exit(1)
	}

	// Validate that the target service exists in systemd before proceeding
	if _, err := conn.GetUnitPropertyContext(ctx, cfg.Service+".service", "ActiveState"); err != nil {
		loga.Error("service not found in systemd", "service", cfg.Service, "err", err)
		os.Exit(1)
	}
	loga.Info("successfully validated service", "service", cfg.Service)

	return conn
}

// HTTP Server Setup
//
// This section configures the HTTP server with all required routes and TLS settings.
// Route handlers are registered for the dashboard, health checks, status API, and
// Prometheus metrics export. TLS configuration supports both manual certificate
// files and automated Let's Encrypt provisioning via ACME.

// SetupHTTPServer initializes the HTTP server with routes for the dashboard,
// health check endpoint, status API, and Prometheus metrics. TLS settings are
// applied based on configuration. The server is not started; this function only
// performs configuration and returns the server instance for later startup.
func SetupHTTPServer(cfg *config.Config, serviceCache *cache.ServiceCache, dashboardHTML []byte) *http.Server {
	// Dashboard route serves the embedded React frontend
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(dashboardHTML); err != nil {
			slog.Error("error writing dashboard", "err", err)
		}
	})

	// Health endpoint returns service status with appropriate HTTP status code
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.HealthHandler(w, r, serviceCache)
	})

	// Status API returns detailed health information as JSON
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		handlers.StatusAPIHandler(w, r, serviceCache, cfg.Service)
	})

	// Metrics endpoint exports Prometheus-formatted metrics
	http.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Apply TLS configuration if enabled
	configureTLS(srv, cfg)

	return srv
}

// configureTLS sets up TLS configuration for the server based on the provided
// configuration. Three modes are supported: Let's Encrypt ACME with autocert,
// manual certificate files, and plain HTTP (no TLS). In autocert mode, a
// background goroutine is started to handle ACME challenges on port 80.
func configureTLS(srv *http.Server, cfg *config.Config) {
	if cfg.TLSAutocert {
		// Let's Encrypt ACME mode with automatic certificate renewal
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

		loga.Info("Let's Encrypt autocert enabled", "domain", cfg.TLSAutocertDomain)

		// Start HTTP server on port 80 to handle ACME challenges (required by Let's Encrypt)
		go func() {
			loga.Info("starting HTTP server for ACME challenges", "addr", ":80")
			if err := http.ListenAndServe(":80", certManager.HTTPHandler(nil)); err != nil {
				loga.Error("ACME challenge server error", "err", err)
			}
		}()

	} else if cfg.TLSEnabled {
		// Manual TLS mode using provided certificate and key files
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

// Background Checker Setup
//
// This section initializes and manages the background goroutine responsible for
// periodically querying systemd service status. A separate watchdog goroutine
// monitors checker health and updates Prometheus metrics.

// StartBackgroundChecker launches the background service monitoring goroutine
// and the checker health watchdog. It returns a context cancellation function
// for clean shutdown and a CheckerHealth handle for health monitoring. The
// background checker runs at the configured interval and updates the shared
// service cache with results.
func StartBackgroundChecker(
	conn *dbus.Conn,
	cfg *config.Config,
	serviceCache *cache.ServiceCache,
) (context.CancelFunc, *checker.CheckerHealth) {
	ctx, cancel := context.WithCancel(context.Background())

	checkerHealth := checker.NewCheckerHealth()
	interval := time.Duration(cfg.Interval) * time.Second

	go checker.StartServiceChecker(ctx, conn, cfg.Service, serviceCache, interval, checkerHealth)

	// Start watchdog goroutine to monitor checker responsiveness
	go startCheckerWatchdog(ctx, cfg, serviceCache, checkerHealth)

	return cancel, checkerHealth
}

// startCheckerWatchdog periodically checks whether the background checker
// goroutine is responding and updating health information. If the checker
// fails to update within the expected time window, the watchdog logs an alert,
// sets the checker health metric to 0, and continues monitoring for recovery.
// The watchdog checks every 10 seconds and considers the checker unhealthy if
// its last update exceeds 2x the configured check interval.
func startCheckerWatchdog(
	ctx context.Context,
	cfg *config.Config,
	serviceCache *cache.ServiceCache,
	checkerHealth *checker.CheckerHealth,
) {
	// Watchdog check interval and health threshold
	maxCheckerAge := time.Duration(cfg.Interval*2) * time.Second

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var isHealthy bool

	for {
		select {
		case <-ticker.C:
			// Evaluate checker health by comparing last update timestamp
			wasHealthy := isHealthy
			isHealthy = checkerHealth.IsHealthy(maxCheckerAge)

			// Update Prometheus metrics
			if isHealthy {
				checkerHealthy.Set(1)
			} else {
				checkerHealthy.Set(0)
			}

			// Log state transitions for operational visibility
			if isHealthy != wasHealthy {
				if isHealthy {
					loga.Info("checker watchdog: checker recovered")
				} else {
					loga.Error("checker watchdog: checker is not responding",
						"max_age", maxCheckerAge.String(),
						"service", cfg.Service)
				}
			}

			// Update gauge with timestamp of the most recent health check
			checkerLastCheckTimestamp.Set(float64(serviceCache.GetLastChecked().Unix()))

		case <-ctx.Done():
			loga.Info("stopping checker watchdog")
			checkerHealthy.Set(0)
			return
		}
	}
}

// HTTP Server Start
//
// This section launches the HTTP/HTTPS server in a background goroutine.
// The server operates in one of three modes: Let's Encrypt ACME with TLS,
// manual certificate TLS, or plain HTTP.

// StartHTTPServer launches the HTTP/HTTPS server in a background goroutine.
// The server mode (HTTP, TLS with manual certs, or TLS with Let's Encrypt)
// is determined by the configuration. Startup information is logged to assist
// with operational debugging. If the server fails to start, an error is logged
// and the process exits with status code 1.
func StartHTTPServer(srv *http.Server, cfg *config.Config) {
	go func() {
		var err error

		if cfg.TLSAutocert {
			loga.Info("monitoring (HTTPS with Let's Encrypt)",
				"service", cfg.Service, "port", cfg.Port, "domain", cfg.TLSAutocertDomain)
			loga.Info("endpoints",
				"dashboard", fmt.Sprintf("https://%s:%d/", cfg.TLSAutocertDomain, cfg.Port),
				"health", fmt.Sprintf("https://%s:%d/health", cfg.TLSAutocertDomain, cfg.Port),
				"api", fmt.Sprintf("https://%s:%d/api/status", cfg.TLSAutocertDomain, cfg.Port),
				"metrics", fmt.Sprintf("https://%s:%d/metrics", cfg.TLSAutocertDomain, cfg.Port),
			)
			err = srv.ListenAndServeTLS("", "")
		} else if cfg.TLSEnabled {
			loga.Info("monitoring (HTTPS with manual certs)",
				"service", cfg.Service, "port", cfg.Port)
			loga.Info("endpoints",
				"dashboard", fmt.Sprintf("https://localhost:%d/", cfg.Port),
				"health", fmt.Sprintf("https://localhost:%d/health", cfg.Port),
				"api", fmt.Sprintf("https://localhost:%d/api/status", cfg.Port),
				"metrics", fmt.Sprintf("https://localhost:%d/metrics", cfg.Port),
			)
			err = srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			loga.Info("monitoring (HTTP)", "service", cfg.Service, "port", cfg.Port)
			loga.Info("endpoints",
				"dashboard", fmt.Sprintf("http://localhost:%d/", cfg.Port),
				"health", fmt.Sprintf("http://localhost:%d/health", cfg.Port),
				"api", fmt.Sprintf("http://localhost:%d/api/status", cfg.Port),
				"metrics", fmt.Sprintf("http://localhost:%d/metrics", cfg.Port),
			)
			err = srv.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			loga.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()
}

// Graceful Shutdown
//
// This section implements a coordinated shutdown sequence that stops the
// background checker and HTTP server in phases, with timeout enforcement
// and proper resource cleanup. The shutdown is triggered by SIGTERM or SIGINT.

// WaitForShutdown blocks until receiving a termination signal (SIGTERM or SIGINT),
// then initiates graceful shutdown of the checker and HTTP server. Shutdown
// follows a phased approach: the background checker is stopped first (5s timeout),
// followed by the HTTP server (remaining time from 30s overall budget). If shutdown
// exceeds the overall 30-second deadline, the server is forcefully closed. This
// function logs all shutdown phases for operational observability.
func WaitForShutdown(srv *http.Server, cancelChecker context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	loga.Info("shutdown signal received; starting graceful shutdown")

	// Overall shutdown context with timeout
	shutdownTimeout := 30 * time.Second
	shutdownStart := time.Now()
	shutdownDeadline := shutdownStart.Add(shutdownTimeout)

	// Use WaitGroup to track shutdown phases
	var wg sync.WaitGroup

	// Phase 1: Stop background checker (should complete within 5 seconds)
	checkerDeadline := time.Now().Add(5 * time.Second)
	wg.Add(1)
	go func() {
		defer wg.Done()
		loga.Info("stopping background checker...")
		cancelChecker()

		// Brief delay to allow goroutine to respect cancellation
		time.Sleep(100 * time.Millisecond)
		loga.Info("background checker stopped")

		if time.Now().After(checkerDeadline) {
			loga.Warn("checker shutdown exceeded deadline")
		}
	}()

	// Wait for checker to stop with respect to overall timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		loga.Debug("checker shutdown completed on time")
	case <-time.After(time.Until(shutdownDeadline)):
		loga.Error("checker shutdown exceeded overall timeout",
			"timeout", shutdownTimeout.String())
	}

	// Phase 2: Shutdown HTTP server with remaining time budget
	remainingTime := time.Until(shutdownDeadline)
	if remainingTime <= 0 {
		remainingTime = 5 * time.Second // Minimum grace period for HTTP shutdown
	}

	httpShutdownCtx, cancel := context.WithTimeout(context.Background(), remainingTime)
	defer cancel()

	loga.Info("shutting down HTTP server",
		"timeout", remainingTime.String())

	if err := srv.Shutdown(httpShutdownCtx); err != nil {
		loga.Error("HTTP server shutdown error", "err", err)
	}

	// Phase 3: Force close if shutdown deadline is exceeded
	if time.Now().After(shutdownDeadline) {
		loga.Warn("shutdown exceeded total timeout; force closing")
		if err := srv.Close(); err != nil {
			loga.Error("Failed to force close connection", "err", err)
		}
	}

	elapsed := time.Since(shutdownStart)
	loga.Info("graceful shutdown complete", "elapsed", elapsed.String())
}
