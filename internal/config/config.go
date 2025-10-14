// Package config
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
	"github.com/spf13/pflag" // Changed from "flag"
)

// Config holds all application configuration
type Config struct {
	Port     int    `koanf:"port"`
	Service  string `koanf:"service"`
	Interval int    `koanf:"interval"`
}

// Load loads configuration with precedence: flags > env vars > config file > defaults
func Load() (*Config, error) {
	k := koanf.New(".")

	// Define command-line flags using pflag
	f := pflag.NewFlagSet("health-checker", pflag.ExitOnError)
	f.Int("port", 8080, "port to listen on")
	f.String("service", "", "systemd service to monitor")
	f.Int("interval", 10, "check interval in seconds")
	f.String("config", "", "path to config file (optional)")

	if err := f.Parse(os.Args[1:]); err != nil {
		return nil, fmt.Errorf("error parsing flags: %w", err)
	}

	// Load optional config file first (lowest priority)
	configPath, _ := f.GetString("config")
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("error loading config file: %w", err)
			}
		}
	}

	// Load environment variables (middle priority)
	if err := k.Load(env.Provider("HEALTH_", ".", func(s string) string {
		return strings.ToLower(strings.TrimPrefix(s, "HEALTH_"))
	}), nil); err != nil {
		return nil, fmt.Errorf("error loading env vars: %w", err)
	}

	// Load command-line flags (highest priority)
	if err := k.Load(posflag.Provider(f, ".", k), nil); err != nil {
		return nil, fmt.Errorf("error loading flags: %w", err)
	}

	// Unmarshal into Config struct
	cfg := &Config{}
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that configuration values are valid
func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1-65535, got %d", c.Port)
	}

	if c.Service == "" {
		return fmt.Errorf("service name is required (use --service flag, HEALTH_SERVICE env var, or config file)")
	}

	if c.Interval < 1 {
		return fmt.Errorf("interval must be at least 1 second, got %d", c.Interval)
	}

	return nil
}
