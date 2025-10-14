// -----------------------------------------------------------------------------
// Configuration Management
// -----------------------------------------------------------------------------
//
// This package provides flexible configuration loading with multiple sources
// and a clear precedence order. Configuration can come from command-line flags,
// environment variables, or YAML files, making it suitable for various
// deployment scenarios.
//
// Precedence Order (highest to lowest):
//   1. Command-line flags  - Explicit user overrides
//   2. Environment vars    - Container/orchestration settings
//   3. Config file         - Persistent configuration
//   4. Default values      - Safe fallbacks
//
// Configuration Sources:
//   Flags:       --port 8080 --service nginx --interval 10
//   Environment: HEALTH_PORT=8080 HEALTH_SERVICE=nginx HEALTH_INTERVAL=10
//   YAML file:   port: 8080, service: nginx, interval: 10
//
// Validation:
//   All configuration is validated after loading to fail fast with clear
//   error messages if required values are missing or invalid.
//
// -----------------------------------------------------------------------------

package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

// -----------------------------------------------------------------------------
// Type Definitions
// -----------------------------------------------------------------------------

// Config holds all application configuration values.
// The koanf struct tags enable automatic unmarshaling from various sources.
type Config struct {
	Port     int    `koanf:"port"`     // HTTP port to listen on
	Service  string `koanf:"service"`  // Systemd service name to monitor
	Interval int    `koanf:"interval"` // Health check interval in seconds

	// TLS/HTTPS configuration
	TLSEnabled  bool   `koanf:"tls_enabled"` // Enable HTTPS
	TLSCertFile string `koanf:"tls_cert"`    // Path to cert file
	TLSKeyFile  string `koanf:"tls_key"`     // Path to key file

	// Let's Encrypt / autocert Configuration
	TLSAutocert       bool   `koanf:"tls_autocert"`        // Enable autocert
	TLSAutocertDomain string `koanf:"tls_autocert_domain"` // Domain for cert
	TLSAutocertCache  string `koanf:"tls_autocert_cache"`  // Cache directory
	TLSAutocertEmail  string `koanf:"tls_autocert_email"`  // Optional email
}

// -----------------------------------------------------------------------------
// Configuration Loading
// -----------------------------------------------------------------------------

// Load reads configuration from multiple sources and returns a validated Config.
// Sources are loaded in reverse precedence order (lowest to highest priority)
// so that higher priority sources can override lower priority ones.
//
// Loading Order:
//  1. Config file (if specified)
//  2. Environment variables
//  3. Command-line flags
//
// Returns an error if configuration is invalid or cannot be loaded.
func Load() (*Config, error) {
	k := koanf.New(".")

	// -------------------------------------------------------------------------
	// Command-Line Flags Definition
	// -------------------------------------------------------------------------
	// Define all available flags with defaults and help text
	f := pflag.NewFlagSet("health-checker", pflag.ExitOnError)
	f.Int("port", 8080, "port to listen on")
	f.String("service", "", "systemd service to monitor")
	f.Int("interval", 10, "check interval in seconds")
	f.String("config", "", "path to config file (optional)")
	f.Bool("tls_enabled", false, "enable HTTPS/TLS")
	f.String("tls_cert", "", "path to TLS certificate file")
	f.String("tls_key", "", "path to TLS private key file")
	f.Bool("tls_autocert", false, "enable Let's Encrypt automatic certificates")
	f.String("tls_autocert_domain", "", "domain name for Let's Encrypt certificate")
	f.String("tls_autocert_cache", "/var/cache/health-checker", "directory for certificate cache")
	f.String("tls_autocert_email", "", "email for Let's Encrypt notifications (optional)")

	if err := f.Parse(os.Args[1:]); err != nil {
		return nil, fmt.Errorf("error parsing flags: %w", err)
	}

	// -------------------------------------------------------------------------
	// Config File Loading (Lowest Priority)
	// -------------------------------------------------------------------------
	// Load YAML config file if specified via --config flag
	// File is optional - no error if not provided
	configPath, _ := f.GetString("config")
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("error loading config file: %w", err)
			}
		}
	}

	// -------------------------------------------------------------------------
	// Environment Variables (Middle Priority)
	// -------------------------------------------------------------------------
	// Load environment variables with HEALTH_ prefix
	// Example: HEALTH_PORT=8080 becomes port=8080
	if err := k.Load(env.Provider("HEALTH_", ".", func(s string) string {
		return strings.ToLower(strings.TrimPrefix(s, "HEALTH_"))
	}), nil); err != nil {
		return nil, fmt.Errorf("error loading env vars: %w", err)
	}

	// -------------------------------------------------------------------------
	// Command-Line Flags (Highest Priority)
	// -------------------------------------------------------------------------
	// Load command-line flags last so they override everything else
	if err := k.Load(posflag.Provider(f, ".", k), nil); err != nil {
		return nil, fmt.Errorf("error loading flags: %w", err)
	}

	// -------------------------------------------------------------------------
	// Unmarshal & Validate
	// -------------------------------------------------------------------------
	// Convert koanf's internal map to our Config struct
	cfg := &Config{}
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate all configuration values before returning
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// -----------------------------------------------------------------------------
// Validation
// -----------------------------------------------------------------------------

// Validate checks that all configuration values are within acceptable ranges.
// Returns a descriptive error if any value is invalid, helping users quickly
// identify and fix configuration problems.
func (c *Config) Validate() error {
	// Validate port is in valid TCP port range
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1-65535, got %d", c.Port)
	}

	// Service name is required - can't monitor nothing
	if c.Service == "" {
		return fmt.Errorf("service name is required (use --service flag, HEALTH_SERVICE env var, or config file)")
	}

	// Interval must be at least 1 second to avoid excessive polling
	if c.Interval < 1 {
		return fmt.Errorf("interval must be at least 1 second, got %d", c.Interval)
	}

	if c.TLSEnabled {
		if c.TLSCertFile == "" {
			return fmt.Errorf("tls-cert is required when TLS is enabled")
		}
		if c.TLSKeyFile == "" {
			return fmt.Errorf("tls-key is required when TLS is enabled")
		}
		// Verify files exist
		if _, err := os.Stat(c.TLSCertFile); err != nil {
			return fmt.Errorf("tls-cert file not found: %w", err)
		}
		if _, err := os.Stat(c.TLSKeyFile); err != nil {
			return fmt.Errorf("tls-key file not found: %w", err)
		}
	}

	if c.TLSAutocert {
		if c.TLSAutocertDomain == "" {
			return fmt.Errorf("tls-autocert-domain is required when using autocert")
		}
		// Autocert requires port 443
		if c.Port != 443 {
			return fmt.Errorf("autocert requires port 443, got %d", c.Port)
		}
	}

	// Can't use both manual certs and autocert
	if c.TLSEnabled && c.TLSAutocert {
		return fmt.Errorf("cannot use both manual TLS and autocert")
	}

	return nil
}
