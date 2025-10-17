// -----------------------------------------------------------------------
// Health Check Service - Main Entry Point
// -----------------------------------------------------------------------
//
// Package main implements the application entry point and lifecycle
// orchestration for the systemd service health checker. It initializes
// configuration, establishes D-Bus connectivity, and coordinates background
// checker and HTTP server components before waiting for shutdown signals.
//
// -----------------------------------------------------------------------

package main

import (
	"context"
	_ "embed"

	"github.com/afreidah/health-check-service/internal/app"
	"github.com/afreidah/health-check-service/internal/cache"
)

//go:embed static/dashboard.html
var dashboardHTML []byte

func main() {
	cfg := app.MustLoadConfig()

	ctx := context.Background()
	conn := app.MustConnectDBus(ctx, cfg)
	defer conn.Close()

	serviceCache := cache.New()
	srv := app.SetupHTTPServer(cfg, serviceCache, dashboardHTML)

	cancelChecker, _ := app.StartBackgroundChecker(conn, cfg, serviceCache)

	app.StartHTTPServer(srv, cfg)

	app.WaitForShutdown(srv, cancelChecker)
}
