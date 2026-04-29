// Package config provides configuration management for the Vision MCP Server.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Default configuration values.
const (
	DefaultTimeout               = 5 * time.Minute
	DefaultLogLevel              = "info"
	DefaultMaxInlinePayloadBytes = 1 << 20 // 1 MiB; matches MCP client soft limits.
	DefaultOverwrite             = false
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

	// Image output policy. All optional. Empty/zero values preserve the
	// pre-P1 behaviour (write to ``os.TempDir()/openpic-mcp/``, no
	// overwrite, 1 MiB inline payload guard) so existing deployments do
	// not need to change anything to upgrade.
	OutputDir             string // OPENPIC_OUTPUT_DIR
	FilenamePrefix        string // OPENPIC_FILENAME_PREFIX
	MaxInlinePayloadBytes int64  // OPENPIC_MAX_INLINE_PAYLOAD_BYTES
	Overwrite             bool   // OPENPIC_OVERWRITE

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
		APIBaseURL:            firstNonEmptyEnv("OPENPIC_API_BASE_URL", "VISION_API_BASE_URL"),
		APIKey:                firstNonEmptyEnv("OPENPIC_API_KEY", "VISION_API_KEY"),
		Model:                 visionModel,
		VisionModel:           visionModel,
		ImageModel:            os.Getenv("OPENPIC_IMAGE_MODEL"),
		OutputDir:             os.Getenv("OPENPIC_OUTPUT_DIR"),
		FilenamePrefix:        os.Getenv("OPENPIC_FILENAME_PREFIX"),
		MaxInlinePayloadBytes: DefaultMaxInlinePayloadBytes,
		Overwrite:             DefaultOverwrite,
		Timeout:               DefaultTimeout,
		LogLevel:              DefaultLogLevel,
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

	// Parse optional inline payload byte budget. Negative or zero values
	// fall back to the default rather than disabling the guard, since
	// disabling it would silently re-enable unbounded inline payloads.
	if raw := os.Getenv("OPENPIC_MAX_INLINE_PAYLOAD_BYTES"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_MAX_INLINE_PAYLOAD_BYTES: %w", err)
		}
		if parsed > 0 {
			cfg.MaxInlinePayloadBytes = parsed
		}
	}

	// Parse optional overwrite flag. Anything strconv.ParseBool accepts
	// is honoured; invalid values are reported instead of silently falling
	// back so deployment misconfigurations are surfaced at startup.
	if raw := os.Getenv("OPENPIC_OVERWRITE"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_OVERWRITE: %w", err)
		}
		cfg.Overwrite = parsed
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
