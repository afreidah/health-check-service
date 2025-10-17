// -----------------------------------------------------------------------
// Configuration Management
// -----------------------------------------------------------------------
//
// Package config provides flexible configuration loading with multiple
// sources and clear precedence. Configuration is validated at startup for
// fail-fast behavior rather than runtime crashes.
//
// Precedence (highest to lowest): command-line flags, environment variables,
// config file, default values. All configuration is validated before
// returning to ensure correct operation.
//
// -----------------------------------------------------------------------

package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
)

// -----------------------------------------------------------------------
// Type Definitions
// -----------------------------------------------------------------------

// Config holds all application configuration values.
type Config struct {
	Port     int    `koanf:"port"`
	Service  string `koanf:"service"`
	Interval int    `koanf:"interval"`

	TLSEnabled  bool   `koanf:"tls_enabled"`
	TLSCertFile string `koanf:"tls_cert"`
	TLSKeyFile  string `koanf:"tls_key"`

	TLSAutocert       bool   `koanf:"tls_autocert"`
	TLSAutocertDomain string `koanf:"tls_autocert_domain"`
	TLSAutocertCache  string `koanf:"tls_autocert_cache"`
	TLSAutocertEmail  string `koanf:"tls_autocert_email"`
}

// -----------------------------------------------------------------------
// Configuration Loading
// -----------------------------------------------------------------------

// Load reads configuration from multiple sources and returns a validated
// Config. Sources are loaded in reverse precedence order (lowest to highest
// priority). Returns a detailed error if any value is invalid.
func Load() (*Config, error) {
	k := koanf.New(".")

	f := pflag.NewFlagSet("health-checker", pflag.ExitOnError)
	f.Int("port", 8080, "port to listen on (1-65535)")
	f.String("service", "", "systemd service to monitor (required)")
	f.Int("interval", 10, "check interval in seconds (minimum 1)")
	f.String("config", "", "path to YAML config file (optional)")
	f.Bool("tls_enabled", false, "enable HTTPS/TLS with manual certificates")
	f.String("tls_cert", "", "path to TLS certificate file (PEM format)")
	f.String("tls_key", "", "path to TLS private key file (PEM format)")
	f.Bool("tls_autocert", false, "enable Let's Encrypt automatic certificates")
	f.String("tls_autocert_domain", "", "domain name for Let's Encrypt certificate")
	f.String("tls_autocert_cache", "/var/cache/health-checker", "directory for certificate cache")
	f.String("tls_autocert_email", "", "email for Let's Encrypt notifications (optional)")

	if err := f.Parse(os.Args[1:]); err != nil {
		return nil, fmt.Errorf("error parsing command-line flags: %w", err)
	}

	// Load config file if specified
	configPath, _ := f.GetString("config")
	if configPath != "" {
		if _, err := os.Stat(configPath); err != nil {
			return nil, fmt.Errorf("config file not found: %s (error: %w)", configPath, err)
		}

		slog.Info("loading configuration from file", "path", configPath)
		if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("error parsing config file (%s): %w", configPath, err)
		}
	} else {
		slog.Debug("no config file specified, using defaults and environment")
	}

	// Load environment variables with HEALTH_ prefix
	if err := k.Load(env.Provider("HEALTH_", ".", func(s string) string {
		return strings.ToLower(strings.TrimPrefix(s, "HEALTH_"))
	}), nil); err != nil {
		return nil, fmt.Errorf("error loading environment variables: %w", err)
	}

	// Load command-line flags (highest priority)
	if err := k.Load(posflag.Provider(f, ".", k), nil); err != nil {
		return nil, fmt.Errorf("error loading command-line flags: %w", err)
	}

	cfg := &Config{}
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	slog.Info("configuration loaded successfully",
		"service", cfg.Service,
		"port", cfg.Port,
		"interval_sec", cfg.Interval,
		"tls_enabled", cfg.TLSEnabled,
		"tls_autocert", cfg.TLSAutocert,
	)

	return cfg, nil
}

// -----------------------------------------------------------------------
// Validation
// -----------------------------------------------------------------------

// Validate checks that all configuration values are within acceptable ranges
// and that referenced files are valid. Returns a descriptive error if any
// value is invalid to help users quickly identify and fix configuration
// issues.
func (c *Config) Validate() error {
	// Port validation
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf(
			"invalid port: must be between 1-65535, got %d\n"+
				"use: --port 8080 or HEALTH_PORT=8080",
			c.Port)
	}

	// Service name validation
	if c.Service == "" {
		return fmt.Errorf(
			"service name is required\n" +
				"specify with: --service nginx or HEALTH_SERVICE=nginx or config file")
	}

	if strings.Contains(c.Service, " ") || strings.Contains(c.Service, "\t") {
		return fmt.Errorf("service name cannot contain whitespace: %q", c.Service)
	}

	// Interval validation
	if c.Interval < 1 {
		return fmt.Errorf(
			"check interval must be at least 1 second, got %d\n"+
				"use: --interval 10 or HEALTH_INTERVAL=10",
			c.Interval)
	}

	if c.Interval > 3600 {
		slog.Warn("unusually long check interval", "interval_sec", c.Interval)
	}

	// TLS configuration validation
	if c.TLSEnabled && c.TLSAutocert {
		return fmt.Errorf(
			"cannot use both manual TLS and autocert together\n" +
				"either:\n" +
				"  1) Use manual TLS: --tls-enabled --tls-cert /path/to/cert --tls-key /path/to/key\n" +
				"  2) Use autocert: --tls-autocert --tls-autocert-domain example.com")
	}

	if c.TLSEnabled {
		if err := c.validateManualTLS(); err != nil {
			return err
		}
	}

	if c.TLSAutocert {
		if err := c.validateAutocert(); err != nil {
			return err
		}
	}

	return nil
}

// -----------------------------------------------------------------------
// TLS Validation Helpers
// -----------------------------------------------------------------------

// validateManualTLS validates manual TLS configuration including file paths
// and certificate validity.
func (c *Config) validateManualTLS() error {
	if c.TLSCertFile == "" {
		return fmt.Errorf(
			"TLS certificate file path is required when using manual TLS\n" +
				"specify with: --tls-cert /path/to/cert.pem")
	}

	if c.TLSKeyFile == "" {
		return fmt.Errorf(
			"TLS key file path is required when using manual TLS\n" +
				"specify with: --tls-key /path/to/key.pem")
	}

	if _, err := os.Stat(c.TLSCertFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"TLS certificate file not found: %s\n"+
					"ensure the file exists and is readable",
				c.TLSCertFile)
		}
		return fmt.Errorf("cannot access TLS certificate file (%s): %w", c.TLSCertFile, err)
	}

	if _, err := os.Stat(c.TLSKeyFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"TLS key file not found: %s\n"+
					"ensure the file exists and is readable",
				c.TLSKeyFile)
		}
		return fmt.Errorf("cannot access TLS key file (%s): %w", c.TLSKeyFile, err)
	}

	if err := validateTLSCertificatePair(c.TLSCertFile, c.TLSKeyFile); err != nil {
		return fmt.Errorf("TLS certificate validation failed: %w\n"+
			"cert: %s\n"+
			"key:  %s\n"+
			"ensure both files are in valid PEM format and the key matches the cert",
			err, c.TLSCertFile, c.TLSKeyFile)
	}

	slog.Info("TLS certificate pair validated successfully",
		"cert_file", c.TLSCertFile,
		"key_file", c.TLSKeyFile)

	return nil
}

// validateAutocert validates Let's Encrypt autocert configuration.
func (c *Config) validateAutocert() error {
	if c.TLSAutocertDomain == "" {
		return fmt.Errorf(
			"domain is required when using Let's Encrypt autocert\n" +
				"specify with: --tls-autocert-domain example.com")
	}

	if !strings.Contains(c.TLSAutocertDomain, ".") {
		return fmt.Errorf(
			"invalid domain for autocert: %q (must contain at least one dot)\n"+
				"example: --tls-autocert-domain example.com",
			c.TLSAutocertDomain)
	}

	if c.Port != 443 {
		slog.Warn("autocert may require port 443 for ACME challenges",
			"current_port", c.Port,
			"note", "this may work behind a reverse proxy that forwards :443")
	}

	cacheDir := c.TLSAutocertCache
	if cacheDir != "" {
		testFile := cacheDir + "/.acme-test"
		if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
			return fmt.Errorf(
				"autocert cache directory is not writable: %s\n"+
					"ensure directory exists and is writable by the service user",
				cacheDir)
		}
		if err := os.Remove(testFile); err != nil {
			slog.Warn("failed to remove testfile", "testfile", testFile)
		}
	}

	slog.Info("Let's Encrypt autocert configuration validated",
		"domain", c.TLSAutocertDomain,
		"cache_dir", c.TLSAutocertCache,
		"email", c.TLSAutocertEmail)

	return nil
}

// -----------------------------------------------------------------------
// Certificate Validation
// -----------------------------------------------------------------------

// validateTLSCertificatePair verifies that certificate and key files are
// valid PEM format, the certificate is parseable, and the key matches the
// certificate.
func validateTLSCertificatePair(certFile, keyFile string) error {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("failed to read certificate file: %w", err)
	}

	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	_, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "tls: failed to parse private key") {
			return fmt.Errorf("invalid private key format (must be PEM): %w", err)
		}
		if strings.Contains(errMsg, "tls: failed to find certificate") {
			return fmt.Errorf("certificate file does not contain a valid certificate (must be PEM): %w", err)
		}
		if strings.Contains(errMsg, "public key in certificate doesn't match") {
			return fmt.Errorf("certificate and private key do not match: %w", err)
		}
		return fmt.Errorf("certificate pair validation failed: %w", err)
	}

	err = validateCertificateExpiration(certPEM)
	if err != nil {
		slog.Warn("certificate expiration check warning", "err", err)
	}

	return nil
}

// validateCertificateExpiration checks whether a certificate has expired or
// is expiring soon. Returns an error with expiration details if concerning.
func validateCertificateExpiration(certPEM []byte) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	now := time.Now()
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired (valid until %s, now is %s)",
			cert.NotAfter.Format("2006-01-02"), now.Format("2006-01-02"))
	}

	daysUntilExpiry := int(time.Until(cert.NotAfter).Hours() / 24)
	if daysUntilExpiry < 7 {
		return fmt.Errorf("certificate expires in %d days (until %s)",
			daysUntilExpiry, cert.NotAfter.Format("2006-01-02"))
	}

	return nil
}
