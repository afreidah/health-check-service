package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/coreos/go-systemd/v22/dbus"
)

func healthHandler(w http.ResponseWriter, r *http.Request, conn *dbus.Conn, service string) {
	prop, err := conn.GetUnitPropertyContext(r.Context(), service+".service", "ActiveState")
	if err != nil {
		log.Fatal(err)
	}

	activeStatus := prop.Value.Value().(string)

	if activeStatus == "active" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	log.Printf("Status %s", activeStatus)
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
