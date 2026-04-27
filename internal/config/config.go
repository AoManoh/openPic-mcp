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
	DefaultTimeout  = 5 * time.Minute
	DefaultLogLevel = "info"
)

// Config holds the configuration for the Vision MCP Server.
type Config struct {
	// Required fields
	APIBaseURL  string // OPENPIC_API_BASE_URL or VISION_API_BASE_URL
	APIKey      string // OPENPIC_API_KEY or VISION_API_KEY
	Model       string // OPENPIC_VISION_MODEL or VISION_MODEL
	VisionModel string // OPENPIC_VISION_MODEL or VISION_MODEL

	// Image generation fields
	ImageModel string // OPENPIC_IMAGE_MODEL

	// Optional fields
	Timeout  time.Duration // OPENPIC_TIMEOUT or VISION_TIMEOUT (default: 5m)
	LogLevel string        // VISION_LOG_LEVEL (default: info)
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.APIBaseURL == "" {
		return errors.New("OPENPIC_API_BASE_URL or VISION_API_BASE_URL is required")
	}
	if c.APIKey == "" {
		return errors.New("OPENPIC_API_KEY or VISION_API_KEY is required")
	}
	if c.VisionModel == "" && c.Model == "" {
		return errors.New("OPENPIC_VISION_MODEL or VISION_MODEL is required")
	}
	if c.VisionModel == "" {
		c.VisionModel = c.Model
	}
	if c.Model == "" {
		c.Model = c.VisionModel
	}
	return nil
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	visionModel := firstNonEmptyEnv("OPENPIC_VISION_MODEL", "VISION_MODEL")
	cfg := &Config{
		APIBaseURL:  firstNonEmptyEnv("OPENPIC_API_BASE_URL", "VISION_API_BASE_URL"),
		APIKey:      firstNonEmptyEnv("OPENPIC_API_KEY", "VISION_API_KEY"),
		Model:       visionModel,
		VisionModel: visionModel,
		ImageModel:  os.Getenv("OPENPIC_IMAGE_MODEL"),
		Timeout:     DefaultTimeout,
		LogLevel:    DefaultLogLevel,
	}

	// Parse optional timeout
	timeoutName, timeoutStr := firstNonEmptyNamedEnv("OPENPIC_TIMEOUT", "VISION_TIMEOUT")
	if timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", timeoutName, err)
		}
		cfg.Timeout = timeout
	}

	// Parse optional log level
	if logLevel := firstNonEmptyEnv("OPENPIC_LOG_LEVEL", "VISION_LOG_LEVEL"); logLevel != "" {
		cfg.LogLevel = logLevel
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

func firstNonEmptyEnv(keys ...string) string {
	_, value := firstNonEmptyNamedEnv(keys...)
	return value
}

func firstNonEmptyNamedEnv(keys ...string) (string, string) {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return key, value
		}
	}
	return "", ""
}
