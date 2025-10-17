// -----------------------------------------------------------------------
// Application Orchestration and Lifecycle
// -----------------------------------------------------------------------
//
// Package app provides core application orchestration for the health check
// service. It coordinates configuration loading, D-Bus connectivity, HTTP
// server setup, background checker lifecycle, and graceful shutdown sequences.
// All major components are initialized and their lifecycles managed here.
//
// -----------------------------------------------------------------------

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
	"github.com/afreidah/health-check-service/internal/metrics"
	"github.com/afreidah/health-check-service/internal/ratelimit"
	"github.com/coreos/go-systemd/v22/dbus"
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

// -----------------------------------------------------------------------
// Configuration & D-Bus Setup
// -----------------------------------------------------------------------

// MustLoadConfig loads application configuration from environment variables
// and performs validation. If configuration is invalid or incomplete, the
// service logs an error and exits with status code 1. The logger is
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
// validates that the target service exists in the current systemd
// configuration. If the connection fails or the service cannot be found, the
// application exits with status code 1 after logging the error condition.
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

// -----------------------------------------------------------------------
// Rate Limited Handler
// -----------------------------------------------------------------------

// RateLimitedHandler wraps an HTTP handler with per-IP rate limiting.
type RateLimitedHandler struct {
	handler  http.Handler
	limiter  *ratelimit.Manager
	endpoint string
}

// ServeHTTP implements the http.Handler interface with rate limiting applied.
func (h *RateLimitedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := ratelimit.GetIP(r)

	if !h.limiter.Allow(ip) {
		// Include rate limit info in response headers
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", h.limiter.GetRate()))
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("Retry-After", "1")

		slog.Warn("rate limit exceeded",
			"ip", ip,
			"endpoint", h.endpoint,
			"method", r.Method,
			"path", r.URL.Path,
		)

		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// Calculate approximate remaining tokens for header
	tokens := h.limiter.GetTokens(ip)
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%.0f", h.limiter.GetRate()))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%.0f", tokens))

	h.handler.ServeHTTP(w, r)
}

// -----------------------------------------------------------------------
// HTTP Server Setup
// -----------------------------------------------------------------------

// SetupHTTPServer initializes the HTTP server with routes for the dashboard,
// health check endpoint, status API, and Prometheus metrics. Rate limiting
// is applied per endpoint with appropriate limits. TLS settings are applied
// based on configuration. The server is not started; this function only
// performs configuration and returns the server instance for later startup.
func SetupHTTPServer(cfg *config.Config, serviceCache *cache.ServiceCache, dashboardHTML []byte) *http.Server {
	// Create rate limiters for different endpoint categories

	// Health endpoint is critical for monitoring - very permissive
	// 100 req/sec, burst 200 = thousands per minute
	// Prometheus, load balancers, multiple monitoring tools won't hit this
	healthLimiter := ratelimit.New(100, 200)

	// Dashboard and API - moderate for human/UI usage
	// 10 req/sec, burst 20
	// Dashboard polls every 2s = 0.5 req/sec, humans max out at 2-5 req/sec
	dashboardLimiter := ratelimit.New(10, 20)

	// Metrics endpoint - Prometheus-specific
	// 2 req/sec, burst 10
	// Prometheus typically scrapes once every 15-30 seconds = 0.033 req/sec
	// Burst handles multiple Prometheus instances
	metricsLimiter := ratelimit.New(2, 10)

	// Create mux for explicit handler registration
	mux := http.NewServeMux()

	// Dashboard route serves the embedded React frontend
	mux.Handle("/", &RateLimitedHandler{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if _, err := w.Write(dashboardHTML); err != nil {
				slog.Error("error writing dashboard", "err", err)
			}
		}),
		limiter:  dashboardLimiter,
		endpoint: "dashboard",
	})

	// Health endpoint returns service status with appropriate HTTP status code
	mux.Handle("/health", &RateLimitedHandler{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlers.HealthHandler(w, r, serviceCache)
		}),
		limiter:  healthLimiter,
		endpoint: "health",
	})

	// Status API returns detailed health information as JSON
	mux.Handle("/api/status", &RateLimitedHandler{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlers.StatusAPIHandler(w, r, serviceCache, cfg.Service)
		}),
		limiter:  dashboardLimiter,
		endpoint: "api_status",
	})

	// Metrics endpoint exports Prometheus-formatted metrics
	mux.Handle("/metrics", &RateLimitedHandler{
		handler:  promhttp.Handler(),
		limiter:  metricsLimiter,
		endpoint: "metrics",
	})

	// Log rate limiting configuration
	loga.Info("rate limiting configured",
		"health_limit", "100 req/sec, burst 200",
		"dashboard_limit", "10 req/sec, burst 20",
		"metrics_limit", "2 req/sec, burst 10",
	)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
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

// -----------------------------------------------------------------------
// Background Checker Setup
// -----------------------------------------------------------------------

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
// fails to update within the expected time window, the watchdog logs an
// alert, sets the checker health metric to 0, and continues monitoring for
// recovery. The watchdog checks every 10 seconds and considers the checker
// unhealthy if its last update exceeds 2x the configured check interval.
//
// Metrics Updated:
//   - health_checker_healthy: Set to 1 when checker is responsive, 0 when stuck
//   - health_checker_last_check_timestamp_seconds: Updated with current cache timestamp
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

			// Update Prometheus metrics to reflect checker health
			if isHealthy {
				metrics.CheckerHealthy.Set(1)
			} else {
				metrics.CheckerHealthy.Set(0)
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
			metrics.CheckerLastCheckTimestamp.Set(float64(serviceCache.GetLastChecked().Unix()))

		case <-ctx.Done():
			loga.Info("stopping checker watchdog")
			metrics.CheckerHealthy.Set(0)
			return
		}
	}
}

// -----------------------------------------------------------------------
// HTTP Server Start
// -----------------------------------------------------------------------

// StartHTTPServer launches the HTTP/HTTPS server in a background goroutine.
// The server mode (HTTP, TLS with manual certs, or TLS with Let's Encrypt)
// is determined by the configuration. Startup information is logged to assist
// with operational debugging. If the server fails to start, an error is
// logged and the process exits with status code 1.
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

// -----------------------------------------------------------------------
// Graceful Shutdown
// -----------------------------------------------------------------------

// WaitForShutdown blocks until receiving a termination signal (SIGTERM or
// SIGINT), then initiates graceful shutdown of the checker and HTTP server.
// Shutdown follows a phased approach: the background checker is stopped first
// (5s timeout), followed by the HTTP server (remaining time from 30s overall
// budget). If shutdown exceeds the overall 30-second deadline, the server is
// forcefully closed. This function logs all shutdown phases for operational
// observability.
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
