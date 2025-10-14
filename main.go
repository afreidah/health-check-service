package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
	"github.com/afreidah/health-check-service/internal/checker"
	"github.com/afreidah/health-check-service/internal/handlers"
	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Handle flags
	cache := cache.New()
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
		handlers.HealthHandler(w, r, cache)
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
	go checker.StartServiceChecker(checkerCtx, conn, service, cache, interval*time.Second)

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
