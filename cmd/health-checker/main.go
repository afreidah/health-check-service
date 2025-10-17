// -----------------------------------------------------------------------------
// Health Check Service
// -----------------------------------------------------------------------------
//
// A lightweight systemd service health checker with Prometheus metrics and
// real-time web dashboard.
//
// This service connects to the system D-Bus to monitor systemd services and
// exposes their health status via HTTP endpoints. It continuously polls the
// configured service and updates an in-memory cache with the current state.
//
// Features:
//   - Systemd service monitoring via D-Bus
//   - Real-time React dashboard with status visualization
//   - HTTP health endpoint with appropriate status codes
//   - JSON API for programmatic access
//   - Prometheus metrics for monitoring and alerting
//   - Thread-safe status caching
//   - Graceful shutdown with configurable timeout
//   - Automatic D-Bus reconnection with exponential backoff
//
// Endpoints:
//   GET /              - Real-time dashboard (React)
//   GET /health        - Health check endpoint (200/503/500)
//   GET /api/status    - JSON status API
//   GET /metrics       - Prometheus metrics
//
// Usage:
//   health-checker --service nginx --port 8080 --interval 10
//
// Environment Variables:
//   HEALTH_SERVICE  - Service name to monitor
//   HEALTH_PORT     - HTTP port to listen on
//   HEALTH_INTERVAL - Check interval in seconds
//
// Author: Alex Freidah <alex.freidah@gmail.com>
// License: Apache 2.0
// -----------------------------------------------------------------------------

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

	// cancel function here
	cancelChecker, _ := app.StartBackgroundChecker(conn, cfg, serviceCache)

	app.StartHTTPServer(srv, cfg)

	// WaitForShutdown now handles all the shutdown orchestration
	app.WaitForShutdown(srv, cancelChecker)
}
