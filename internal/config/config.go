// Package config provides configuration management for the Vision MCP Server.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Default configuration values.
const (
	DefaultTimeout  = 30 * time.Second
	DefaultLogLevel = "info"
)

// Config holds the configuration for the Vision MCP Server.
type Config struct {
	// Required fields
	APIBaseURL string // VISION_API_BASE_URL
	APIKey     string // VISION_API_KEY
	Model      string // VISION_MODEL

	// Optional fields
	Timeout  time.Duration // VISION_TIMEOUT (default: 30s)
	LogLevel string        // VISION_LOG_LEVEL (default: info)
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.APIBaseURL == "" {
		return errors.New("VISION_API_BASE_URL is required")
	}
	if c.APIKey == "" {
		return errors.New("VISION_API_KEY is required")
	}
	if c.Model == "" {
		return errors.New("VISION_MODEL is required")
	}
	return nil
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		APIBaseURL: os.Getenv("VISION_API_BASE_URL"),
		APIKey:     os.Getenv("VISION_API_KEY"),
		Model:      os.Getenv("VISION_MODEL"),
		Timeout:    DefaultTimeout,
		LogLevel:   DefaultLogLevel,
	}

	// Parse optional timeout
	if timeoutStr := os.Getenv("VISION_TIMEOUT"); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid VISION_TIMEOUT: %w", err)
		}
		cfg.Timeout = timeout
	}

	// Parse optional log level
	if logLevel := os.Getenv("VISION_LOG_LEVEL"); logLevel != "" {
		cfg.LogLevel = logLevel
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}
