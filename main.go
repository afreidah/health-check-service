package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ServiceCache struct {
	mu          sync.RWMutex
	statusCode  int
	state       string
	lastChecked time.Time
}

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

func startServiceChecker(ctx context.Context, conn *dbus.Conn, service string, cache *ServiceCache, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Do an immediate check on startup
	checkAndUpdateCache(conn, service, cache)

	for {
		select {
		case <-ticker.C:
			// Ticker fired - time to check
			checkAndUpdateCache(conn, service, cache)
		case <-ctx.Done():
			// Context cancelled - time to shut down
			log.Println("Stopping service checker")
			return
		}
	}
}

func checkAndUpdateCache(conn *dbus.Conn, service string, cache *ServiceCache) {
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
		serviceStatus.WithLabelValues(service, activeStatus).Set(1)
	} else {
		serviceStatus.WithLabelValues(service, activeStatus).Set(0)
	}
}

func (c *ServiceCache) GetStatus() (int, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statusCode, c.state
}

func (c *ServiceCache) UpdateStatus(code int, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statusCode = code
	c.state = state
	c.lastChecked = time.Now()
}

func NewServiceCache() *ServiceCache {
	return &ServiceCache{
		statusCode: http.StatusServiceUnavailable, // start as unavailable
		state:      "unknown",
	}
}

// Prometheus metrics
var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "health_check_requests_total",
			Help: "Total number of health check requests by HTTP status code",
		},
		[]string{"status_code"},
	)

	serviceStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "monitored_service_status",
			Help: "Status of the monitored systemd service (1=active, 0=not active)",
		},
		[]string{"service", "state"},
	)

	requestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "health_check_request_duration_seconds",
			Help:    "Duration of health check requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
)

// automatically runs before main()
func init() {
	// Register metrics with Prometheus
	prometheus.MustRegister(requestsTotal)
	prometheus.MustRegister(serviceStatus)
	prometheus.MustRegister(requestDuration)
}

func healthHandler(w http.ResponseWriter, r *http.Request, cache *ServiceCache, conn *dbus.Conn, service string) {
	start := time.Now()
	var statusCode int

	defer func() {
		// Record metrics after handler completes
		duration := time.Since(start).Seconds()
		requestDuration.Observe(duration)
		requestsTotal.WithLabelValues(fmt.Sprintf("%d", statusCode)).Inc()
	}()

	statusCode, status := cache.GetStatus()
	log.Printf("Current status of %s: %s", service, status)

	w.WriteHeader(statusCode)
}

func main() {
	// Handle flags
	cache := NewServiceCache()
	portFlag := flag.Int("port", 8080, "port to listen on")
	intervalFlag := flag.Int("interval", 10, "interval between service health checks in seconds")
	serviceFlag := flag.String("service", "", "service to monitor")
	flag.Parse()

	port := *portFlag
	interval := time.Duration(*intervalFlag)
	service := *serviceFlag

	// D-Bus connection
	ctx := context.Background()
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Setup HTTP handlers
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthHandler(w, r, cache, conn, service)
	})
	http.Handle("/metrics", promhttp.Handler())

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Create a separate cancellable context for the background checker
	checkerCtx, checkerCancel := context.WithCancel(context.Background())
	defer checkerCancel()

	// start background status worker
	go startServiceChecker(checkerCtx, conn, service, cache, interval*time.Second)

	// Start server in a goroutine
	go func() {
		log.Printf("Monitoring %s on :%d/health", service, port)
		log.Printf("Metrics available at :%d/metrics", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for signal
	<-sigChan
	log.Println("Shutdown signal received, gracefully shutting down...")

	// shut down the background worker first
	checkerCancel()

	// shut down the http server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
