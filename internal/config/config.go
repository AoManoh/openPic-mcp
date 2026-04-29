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
	DefaultLogFormat             = "text"
	DefaultMaxInlinePayloadBytes = 1 << 20 // 1 MiB; matches MCP client soft limits.
	DefaultOverwrite             = false

	// Server engine defaults. These mirror the constants in
	// internal/server but live here so callers can tune them via
	// environment variables without importing the server package.
	DefaultMaxConcurrentRequests = 16
	DefaultRequestQueueSize      = 64
	DefaultRequestTimeout        = 0 // 0 = no timeout; image generation can legitimately take 90s+.
	DefaultShutdownTimeout       = 30 * time.Second

	// Async task layer defaults. The store and dispatcher are enabled by
	// default with disk persistence on; deployments that want pure-memory
	// behaviour (e.g. ephemeral CI runs) flip OPENPIC_TASK_DISK_PERSIST=false.
	DefaultTaskStoreEnabled = true
	DefaultTaskDiskPersist  = true
	DefaultTaskMaxQueued    = 256
	DefaultTaskMaxRetained  = 1024
	DefaultTaskTTL          = 24 * time.Hour
)

// Hard caps for engine tunables. Values above these are silently clamped
// during Load to avoid pathological configurations.
const (
	MaxConcurrentRequestsCap = 100
	RequestQueueSizeCap      = 10000
	TaskMaxQueuedCap         = 10000
	TaskMaxRetainedCap       = 100000
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

	// Server engine tuning. Zero values fall back to the Default* constants
	// above, ensuring an environment with no engine-related variables set
	// still gets sensible production defaults.
	MaxConcurrentRequests int           // OPENPIC_MAX_CONCURRENT_REQUESTS
	RequestQueueSize      int           // OPENPIC_REQUEST_QUEUE_SIZE
	RequestTimeout        time.Duration // OPENPIC_REQUEST_TIMEOUT (0 = no timeout)
	ShutdownTimeout       time.Duration // OPENPIC_SHUTDOWN_TIMEOUT

	// Async task layer. TaskStoreEnabled is the master switch: when false
	// the four async tools are not registered and no taskstore/dispatcher
	// is constructed. TaskDiskPersist toggles disk-backed manifests under
	// $OPENPIC_OUTPUT_DIR/tasks; when false the store is purely in-memory
	// and tasks vanish on restart. The two int caps bound store population
	// (queued + retained terminal); TaskTTL is the GC retention horizon
	// for terminal tasks.
	TaskStoreEnabled bool          // OPENPIC_TASK_STORE_ENABLED (default true)
	TaskDiskPersist  bool          // OPENPIC_TASK_DISK_PERSIST  (default true)
	TaskMaxQueued    int           // OPENPIC_TASK_MAX_QUEUED    (default 256)
	TaskMaxRetained  int           // OPENPIC_TASK_MAX_RETAINED  (default 1024)
	TaskTTL          time.Duration // OPENPIC_TASK_TTL           (default 24h)

	// Optional fields
	Timeout   time.Duration // OPENPIC_TIMEOUT or VISION_TIMEOUT (default: 5m)
	LogLevel  string        // VISION_LOG_LEVEL (default: info)
	LogFormat string        // OPENPIC_LOG_FORMAT (text|json, default: text)
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
		MaxConcurrentRequests: DefaultMaxConcurrentRequests,
		RequestQueueSize:      DefaultRequestQueueSize,
		RequestTimeout:        DefaultRequestTimeout,
		ShutdownTimeout:       DefaultShutdownTimeout,
		TaskStoreEnabled:      DefaultTaskStoreEnabled,
		TaskDiskPersist:       DefaultTaskDiskPersist,
		TaskMaxQueued:         DefaultTaskMaxQueued,
		TaskMaxRetained:       DefaultTaskMaxRetained,
		TaskTTL:               DefaultTaskTTL,
		Timeout:               DefaultTimeout,
		LogLevel:              DefaultLogLevel,
		LogFormat:             DefaultLogFormat,
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

	// Parse server engine tunables. Each follows the same pattern: empty
	// → keep the default; invalid → surface the error so misconfigured
	// deployments fail loudly at startup; out-of-range → clamp to the
	// hard caps so a typo cannot accidentally provision 100 000 workers.
	if raw := os.Getenv("OPENPIC_MAX_CONCURRENT_REQUESTS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_MAX_CONCURRENT_REQUESTS: %w", err)
		}
		cfg.MaxConcurrentRequests = clampPositiveInt(parsed, DefaultMaxConcurrentRequests, MaxConcurrentRequestsCap)
	}
	if raw := os.Getenv("OPENPIC_REQUEST_QUEUE_SIZE"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_REQUEST_QUEUE_SIZE: %w", err)
		}
		cfg.RequestQueueSize = clampPositiveInt(parsed, DefaultRequestQueueSize, RequestQueueSizeCap)
	}
	if raw := os.Getenv("OPENPIC_REQUEST_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_REQUEST_TIMEOUT: %w", err)
		}
		if parsed < 0 {
			return nil, fmt.Errorf("invalid OPENPIC_REQUEST_TIMEOUT: must be >= 0")
		}
		cfg.RequestTimeout = parsed
	}
	if raw := os.Getenv("OPENPIC_SHUTDOWN_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_SHUTDOWN_TIMEOUT: %w", err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("invalid OPENPIC_SHUTDOWN_TIMEOUT: must be > 0")
		}
		cfg.ShutdownTimeout = parsed
	}
	if raw := os.Getenv("OPENPIC_LOG_FORMAT"); raw != "" {
		normalized, err := normalizeLogFormat(raw)
		if err != nil {
			return nil, err
		}
		cfg.LogFormat = normalized
	}

	// Async task layer tunables. Same fail-loud-on-bad-input pattern as
	// the engine knobs above; out-of-range values are clamped to the hard
	// caps so a typo cannot accidentally provision a 10 million-slot heap.
	if raw := os.Getenv("OPENPIC_TASK_STORE_ENABLED"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_TASK_STORE_ENABLED: %w", err)
		}
		cfg.TaskStoreEnabled = parsed
	}
	if raw := os.Getenv("OPENPIC_TASK_DISK_PERSIST"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_TASK_DISK_PERSIST: %w", err)
		}
		cfg.TaskDiskPersist = parsed
	}
	if raw := os.Getenv("OPENPIC_TASK_MAX_QUEUED"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_TASK_MAX_QUEUED: %w", err)
		}
		cfg.TaskMaxQueued = clampPositiveInt(parsed, DefaultTaskMaxQueued, TaskMaxQueuedCap)
	}
	if raw := os.Getenv("OPENPIC_TASK_MAX_RETAINED"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_TASK_MAX_RETAINED: %w", err)
		}
		cfg.TaskMaxRetained = clampPositiveInt(parsed, DefaultTaskMaxRetained, TaskMaxRetainedCap)
	}
	if raw := os.Getenv("OPENPIC_TASK_TTL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid OPENPIC_TASK_TTL: %w", err)
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("invalid OPENPIC_TASK_TTL: must be > 0")
		}
		cfg.TaskTTL = parsed
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// clampPositiveInt returns value when it lies within (0, cap]; otherwise
// returns fallback. Zero/negative inputs always fall back to the default
// so a misconfigured "0" cannot disable the engine entirely.
func clampPositiveInt(value, fallback, cap int) int {
	if value <= 0 {
		return fallback
	}
	if cap > 0 && value > cap {
		return cap
	}
	return value
}

// normalizeLogFormat returns the canonical form of a user-supplied log
// format string. Only `text` and `json` are accepted; anything else is a
// configuration error so deployments can't drift onto an unsupported
// handler silently.
func normalizeLogFormat(raw string) (string, error) {
	switch raw {
	case "text", "json":
		return raw, nil
	default:
		return "", fmt.Errorf("invalid OPENPIC_LOG_FORMAT %q: expected 'text' or 'json'", raw)
	}
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
