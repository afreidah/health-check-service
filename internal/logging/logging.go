// -----------------------------------------------------------------------
// Structured Logging Configuration
// -----------------------------------------------------------------------
//
// Package logging provides centralized structured logging configuration via
// slog. All components use a single logger instance configured at startup and
// set as the default. Supports JSON output for log aggregation systems or text
// for local development.
//
// Configuration via environment variables:
//   - LOG_LEVEL: debug|info|warn|error (default: info)
//   - LOG_FORMAT: json|text (default: json)
//   - LOG_SOURCE: true|1 to include file:line (default: false)
//   - LOG_TAGS: comma-separated key=value pairs added to all logs
//
// -----------------------------------------------------------------------

package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// -----------------------------------------------------------------------
// Type Definitions
// -----------------------------------------------------------------------

// Options encapsulates logging configuration.
type Options struct {
	Level     slog.Level        // Log level threshold (debug, info, warn, error)
	Format    string            // "json" (default) or "text"
	Tags      map[string]string // Static tags added to all log entries
	AddSource bool              // Include file:line in each log entry
}

// -----------------------------------------------------------------------
// Logger Initialization
// -----------------------------------------------------------------------

// Init creates and installs a logger with the given configuration.
// The logger is set as the default via slog.SetDefault.
func Init(opts Options) *slog.Logger {
	var h slog.Handler
	switch strings.ToLower(opts.Format) {
	case "text":
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     opts.Level,
			AddSource: opts.AddSource,
		})
	default:
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:     opts.Level,
			AddSource: opts.AddSource,
		})
	}

	attrs := make([]any, 0, len(opts.Tags)*2)
	for k, v := range opts.Tags {
		attrs = append(attrs, k, v)
	}

	logger := slog.New(h).With(attrs...)
	slog.SetDefault(logger)
	return logger
}

// InitFromEnv creates a logger from environment variables. Additional tags
// passed in extraTags are merged with environment tags (extraTags take
// precedence on conflict).
//
// Environment Variables:
//   - LOG_LEVEL: debug|info|warn|error (default: info)
//   - LOG_FORMAT: json|text (default: json)
//   - LOG_SOURCE: true|1 to include file:line (default: false)
//   - LOG_TAGS: comma-separated key=value pairs (example: "env=prod,team=platform")
func InitFromEnv(extraTags map[string]string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}

	format := os.Getenv("LOG_FORMAT")
	addSource := strings.EqualFold(os.Getenv("LOG_SOURCE"), "1") ||
		strings.EqualFold(os.Getenv("LOG_SOURCE"), "true")

	tags := parseTags(os.Getenv("LOG_TAGS"))
	for k, v := range extraTags {
		tags[k] = v
	}

	return Init(Options{
		Level:     lvl,
		Format:    format,
		Tags:      tags,
		AddSource: addSource,
	})
}

// -----------------------------------------------------------------------
// Helper Functions
// -----------------------------------------------------------------------

// parseTags converts comma-separated key=value string to a map. Whitespace
// around keys and values is trimmed. Empty pairs are skipped.
//
// Example: "env=prod,team=platform" returns map[env:prod team:platform]
func parseTags(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		if pair == "" {
			continue
		}
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

// WithRequest returns a logger enriched with request-specific tags. Use this
// in request handlers to add correlation fields like request_id.
func WithRequest(_ context.Context, base *slog.Logger, tags map[string]any) *slog.Logger {
	attrs := make([]any, 0, len(tags)*2)
	for k, v := range tags {
		attrs = append(attrs, k, v)
	}
	return base.With(attrs...)
}
