package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

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
		// Unknown state from systemd
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func main() {
	// handle flags
	portFlag := flag.Int("port", 8080, "port to listen on")
	serviceFlag := flag.String("service", "", "service to monitor")
	flag.Parse()

	port := *portFlag
	service := *serviceFlag

	// dbus connection
	ctx := context.Background()
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthHandler(w, r, conn, service)
	})

	log.Printf("Monitoring %s on :%d", service, port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatal(err)
	}
}
