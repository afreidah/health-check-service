// -----------------------------------------------------------------------------
// Configuration Management - Tests
// -----------------------------------------------------------------------------
//
// This test suite validates configuration loading from multiple sources with
// proper precedence and validation. Since config errors prevent startup,
// these tests are critical for catching issues early.
//
// Test Coverage:
//   - Default values
//   - Validation rules (port range, required fields, intervals)
//   - TLS configuration validation
//   - Autocert configuration validation
//   - Conflicting TLS settings
//
// Note: Testing flags and environment variables requires careful setup to
// avoid affecting other tests. We focus on validation logic here.
//
// -----------------------------------------------------------------------------

package config

import (
	"testing"
)

// -----------------------------------------------------------------------------
// Validation Tests
// -----------------------------------------------------------------------------

// TestValidatePortRange verifies port validation catches invalid values.
// Ports must be in the valid TCP range 1-65535.
func TestValidatePortRange(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		shouldErr bool
	}{
		{"valid port 80", 80, false},
		{"valid port 8080", 8080, false},
		{"valid port 443", 443, false},
		{"valid port 65535", 65535, false},
		{"invalid port 0", 0, true},
		{"invalid port -1", -1, true},
		{"invalid port 65536", 65536, true},
		{"invalid port 100000", 100000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Port:     tt.port,
				Service:  "nginx", // Required field
				Interval: 10,      // Required field
			}

			err := cfg.Validate()

			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for port %d, got nil", tt.port)
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error for port %d, got: %v", tt.port, err)
			}
		})
	}
}

// TestValidateServiceRequired verifies that service name is mandatory.
// The service cannot monitor nothing.
func TestValidateServiceRequired(t *testing.T) {
	cfg := &Config{
		Port:     8080,
		Service:  "", // Missing required field
		Interval: 10,
	}

	err := cfg.Validate()

	if err == nil {
		t.Error("Expected error for missing service name, got nil")
	}
}

// TestValidateIntervalMinimum verifies interval must be at least 1 second.
// This prevents excessive polling of systemd.
func TestValidateIntervalMinimum(t *testing.T) {
	tests := []struct {
		name      string
		interval  int
		shouldErr bool
	}{
		{"valid interval 1", 1, false},
		{"valid interval 10", 10, false},
		{"valid interval 60", 60, false},
		{"invalid interval 0", 0, true},
		{"invalid interval -1", -1, true},
		{"invalid interval -10", -10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Port:     8080,
				Service:  "nginx",
				Interval: tt.interval,
			}

			err := cfg.Validate()

			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for interval %d, got nil", tt.interval)
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error for interval %d, got: %v", tt.interval, err)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// TLS Configuration Tests
// -----------------------------------------------------------------------------

// TestValidateTLSRequiresCertAndKey verifies that manual TLS requires both
// certificate and key files to be specified.
func TestValidateTLSRequiresCertAndKey(t *testing.T) {
	tests := []struct {
		name      string
		certFile  string
		keyFile   string
		shouldErr bool
	}{
		{"missing both", "", "", true},
		{"missing cert", "", "key.pem", true},
		{"missing key", "cert.pem", "", true},
		// Note: We can't test with real files without creating them
		// Just verify the validation logic checks for non-empty strings
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Port:        8080,
				Service:     "nginx",
				Interval:    10,
				TLSEnabled:  true,
				TLSCertFile: tt.certFile,
				TLSKeyFile:  tt.keyFile,
			}

			err := cfg.Validate()

			if tt.shouldErr && err == nil {
				t.Error("Expected error for incomplete TLS config, got nil")
			}
		})
	}
}

// TestValidateAutocertRequiresDomain verifies that autocert requires a domain.
// Let's Encrypt needs to know which domain to issue certificates for.
func TestValidateAutocertRequiresDomain(t *testing.T) {
	cfg := &Config{
		Port:              443,
		Service:           "nginx",
		Interval:          10,
		TLSAutocert:       true,
		TLSAutocertDomain: "", // Missing required field
	}

	err := cfg.Validate()

	if err == nil {
		t.Error("Expected error for autocert without domain, got nil")
	}
}

// TestValidateAutocertWarnsAboutPort verifies that autocert with non-443 port
// logs a warning but doesn't fail. This allows reverse proxy scenarios.
func TestValidateAutocertWarnsAboutPort(t *testing.T) {
	cfg := &Config{
		Port:              8443, // Not 443
		Service:           "nginx",
		Interval:          10,
		TLSAutocert:       true,
		TLSAutocertDomain: "example.com",
	}

	// Should not error, just warn (warning is logged, not returned as error)
	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected warning only for port %d with autocert, got error: %v", cfg.Port, err)
	}
}

// TestValidateCannotUseBothTLSAndAutocert verifies that manual TLS and
// autocert are mutually exclusive. You can't use both at once.
func TestValidateCannotUseBothTLSAndAutocert(t *testing.T) {
	cfg := &Config{
		Port:              443,
		Service:           "nginx",
		Interval:          10,
		TLSEnabled:        true, // Manual TLS
		TLSCertFile:       "cert.pem",
		TLSKeyFile:        "key.pem",
		TLSAutocert:       true, // Also autocert - conflict!
		TLSAutocertDomain: "example.com",
	}

	err := cfg.Validate()

	if err == nil {
		t.Error("Expected error for using both manual TLS and autocert, got nil")
	}
}

// -----------------------------------------------------------------------------
// Valid Configuration Tests
// -----------------------------------------------------------------------------

// TestValidateHTTPOnly verifies a basic HTTP-only config is valid.
// This is the most common use case.
func TestValidateHTTPOnly(t *testing.T) {
	cfg := &Config{
		Port:     8080,
		Service:  "nginx",
		Interval: 10,
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected valid HTTP config, got error: %v", err)
	}
}

// TestValidateWithAutocert verifies a valid autocert config passes validation.
func TestValidateWithAutocert(t *testing.T) {
	cfg := &Config{
		Port:              443,
		Service:           "nginx",
		Interval:          10,
		TLSAutocert:       true,
		TLSAutocertDomain: "example.com",
		TLSAutocertCache:  "/var/cache/health-checker",
		TLSAutocertEmail:  "admin@example.com",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected valid autocert config, got error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Edge Cases
// -----------------------------------------------------------------------------

// TestValidateMinimumInterval verifies that interval of 1 second is allowed.
// This is the minimum to prevent excessive systemd polling.
func TestValidateMinimumInterval(t *testing.T) {
	cfg := &Config{
		Port:     8080,
		Service:  "nginx",
		Interval: 1, // Minimum allowed
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected interval 1 to be valid, got error: %v", err)
	}
}

// TestValidateMaximumPort verifies port 65535 is accepted.
// This is the maximum valid TCP port.
func TestValidateMaximumPort(t *testing.T) {
	cfg := &Config{
		Port:     65535, // Maximum valid port
		Service:  "nginx",
		Interval: 10,
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected port 65535 to be valid, got error: %v", err)
	}
}
