// Package logging
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type Options struct {
	Level     slog.Level
	Format    string            // "json" (default) | "text"
	Tags      map[string]string // static tags (env=prod,service=...,version=...,component=...)
	AddSource bool              // include file:line
}

// Init builds and installs a default slog.Logger with static tags.
func Init(opts Options) *slog.Logger {
	var h slog.Handler
	switch strings.ToLower(opts.Format) {
	case "text":
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: opts.Level, AddSource: opts.AddSource})
	default:
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: opts.Level, AddSource: opts.AddSource})
	}

	attrs := make([]any, 0, len(opts.Tags)*2)
	for k, v := range opts.Tags {
		attrs = append(attrs, k, v)
	}

	logger := slog.New(h).With(attrs...)
	slog.SetDefault(logger)
	return logger
}

// InitFromEnv convenience:
//
//	LOG_LEVEL:  debug|info|warn|error   (default: info)
//	LOG_FORMAT: json|text               (default: json)
//	LOG_TAGS:   "k=v,k2=v2"             (applied to every log)
//	LOG_SOURCE: true|1                  (include file:line)
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
	addSource := strings.EqualFold(os.Getenv("LOG_SOURCE"), "1") || strings.EqualFold(os.Getenv("LOG_SOURCE"), "true")

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

// WithRequest returns a logger enriched with request-scoped tags (e.g., request_id).
func WithRequest(_ context.Context, base *slog.Logger, tags map[string]any) *slog.Logger {
	attrs := make([]any, 0, len(tags)*2)
	for k, v := range tags {
		attrs = append(attrs, k, v)
	}
	return base.With(attrs...)
}
