package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func main() {
	// handle flags
	portFlag := flag.Int("port", 8080, "port to listen on")
	serviceFlag := flag.String("service", "", "service to monitor")
	flag.Parse()

	port := *portFlag
	service := *serviceFlag

	http.HandleFunc("/health", healthHandler)

	log.Printf("Monitoring %s on :%d", service, port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatal(err)
	}
}
