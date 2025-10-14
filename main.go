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

	"github.com/coreos/go-systemd/v22/dbus"
)

// Systemd ActiveState values
const (
	StateActive       = "active"
	StateInactive     = "inactive"
	StateFailed       = "failed"
	StateActivating   = "activating"
	StateDeactivating = "deactivating"
	StateReloading    = "reloading"
)

func healthHandler(w http.ResponseWriter, r *http.Request, conn *dbus.Conn, service string) {
	prop, err := conn.GetUnitPropertyContext(r.Context(), service+".service", "ActiveState")
	if err != nil {
		log.Printf("Error checking service %s: %v", service, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	activeStatus, ok := prop.Value.Value().(string)
	if !ok {
		log.Printf("Unexpected type for ActiveState")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch activeStatus {
	case StateActive:
		w.WriteHeader(http.StatusOK)
	case StateInactive, StateFailed, StateActivating, StateDeactivating, StateReloading:
		w.WriteHeader(http.StatusServiceUnavailable)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func main() {
	// Handle flags
	portFlag := flag.Int("port", 8080, "port to listen on")
	serviceFlag := flag.String("service", "", "service to monitor")
	flag.Parse()

	port := *portFlag
	service := *serviceFlag

	// D-Bus connection
	ctx := context.Background()
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Setup HTTP handler
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthHandler(w, r, conn, service)
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Monitoring %s on :%d", service, port)
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

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
