// -----------------------------------------------------------------------------
// Structured Logging Setup
// -----------------------------------------------------------------------------
//
// This package initializes the structured logger (slog) for the application.
// Logs are output as JSON to stderr, making them easy to parse and search
// in production environments (Kubernetes, Docker, ELK, etc.)
//
// Log Levels:
//   DEBUG - Detailed internal information (only when debugging)
//   INFO  - Normal operation events
//   WARN  - Recoverable issues that should be noted
//   ERROR - Problems that need immediate attention
//
// Debug Mode:
//   Set DEBUG=1 environment variable to enable DEBUG level logging.
//   Without it, only INFO and above are logged.
//
// Usage:
//   logger := logging.New()
//   logger.Info("event_name", "key", "value", "key2", "value2")
//   logger.ErrorContext(ctx, "error_event", "error", err)
//
// Author: Alex Freidah <alex.freidah@gmail.com>
// License: Apache 2.0
// -----------------------------------------------------------------------------

package logging

import (
	"log/slog"
	"os"
)

// New creates and returns a new structured logger instance.
// Uses JSON output to stderr for production compatibility.
// Log level is determined by DEBUG environment variable.
//
// Returns:
//
//	A configured *slog.Logger ready to use throughout the application.
//
// Environment Variables:
//
//	DEBUG - If set to any non-empty value, enables DEBUG level logging
//	        Default: not set (INFO level)
func New() *slog.Logger {
	// Determine log level from environment
	logLevel := slog.LevelInfo
	if os.Getenv("DEBUG") != "" {
		logLevel = slog.LevelDebug
	}

	// Create JSON handler for stderr output
	// This is ideal for containerized environments
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
		// AddSource: true,  // Uncomment to include file:line in output
	})

	// Return configured logger
	return slog.New(handler)
}
