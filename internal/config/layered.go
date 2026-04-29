// Package config provides layered configuration management.
package config

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// LayeredConfig manages multiple configuration sources with priority.
type LayeredConfig struct {
	mu      sync.RWMutex
	sources []sourceEntry
}

type sourceEntry struct {
	priority int
	source   Source
}

// NewLayeredConfig creates a new layered configuration manager.
func NewLayeredConfig() *LayeredConfig {
	return &LayeredConfig{
		sources: make([]sourceEntry, 0),
	}
}

// AddSource adds a configuration source with the given priority.
// Higher priority sources override lower priority ones.
func (c *LayeredConfig) AddSource(priority int, source Source) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sources = append(c.sources, sourceEntry{
		priority: priority,
		source:   source,
	})

	// Sort by priority (highest first)
	sort.Slice(c.sources, func(i, j int) bool {
		return c.sources[i].priority > c.sources[j].priority
	})
}

// Get returns the value for a key from the highest priority source that has it.
func (c *LayeredConfig) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, entry := range c.sources {
		if entry.source.Has(key) {
			return entry.source.Get(key)
		}
	}
	return ""
}

// GetWithSource returns the value and the source name it came from.
func (c *LayeredConfig) GetWithSource(key string) (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, entry := range c.sources {
		if entry.source.Has(key) {
			return entry.source.Get(key), entry.source.Name()
		}
	}
	return "", ""
}

// Has returns true if any source has the key.
func (c *LayeredConfig) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, entry := range c.sources {
		if entry.source.Has(key) {
			return true
		}
	}
	return false
}

// GetString returns a string value with a default fallback.
func (c *LayeredConfig) GetString(key string, defaultVal string) string {
	val := c.Get(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func (c *LayeredConfig) getFirst(keys ...string) string {
	for _, key := range keys {
		if val := c.Get(key); val != "" {
			return val
		}
	}
	return ""
}

// GetDuration returns a duration value with a default fallback.
func (c *LayeredConfig) GetDuration(key string, defaultVal time.Duration) time.Duration {
	return ParseDuration(c.Get(key), defaultVal)
}

// GetInt returns an integer value with a default fallback.
func (c *LayeredConfig) GetInt(key string, defaultVal int) int {
	return ParseInt(c.Get(key), defaultVal)
}

// GetInt64 returns an int64 value with a default fallback.
func (c *LayeredConfig) GetInt64(key string, defaultVal int64) int64 {
	return ParseInt64(c.Get(key), defaultVal)
}

// GetBool returns a boolean value with a default fallback.
func (c *LayeredConfig) GetBool(key string, defaultVal bool) bool {
	return ParseBool(c.Get(key), defaultVal)
}

// GetStringSlice returns a string slice value with a default fallback.
func (c *LayeredConfig) GetStringSlice(key string, defaultVal []string) []string {
	return ParseStringSlice(c.Get(key), defaultVal)
}

// Sources returns the list of source names in priority order.
func (c *LayeredConfig) Sources() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, len(c.sources))
	for i, entry := range c.sources {
		names[i] = entry.source.Name()
	}
	return names
}

// BuildConfig builds a Config struct from the layered configuration.
func (c *LayeredConfig) BuildConfig() (*Config, error) {
	visionModel := c.getFirst("OPENPIC_VISION_MODEL", "VISION_MODEL")
	cfg := &Config{
		APIBaseURL:            c.getFirst("OPENPIC_API_BASE_URL", "VISION_API_BASE_URL"),
		APIKey:                c.getFirst("OPENPIC_API_KEY", "VISION_API_KEY"),
		Model:                 visionModel,
		VisionModel:           visionModel,
		ImageModel:            c.Get("OPENPIC_IMAGE_MODEL"),
		OutputDir:             c.Get("OPENPIC_OUTPUT_DIR"),
		FilenamePrefix:        c.Get("OPENPIC_FILENAME_PREFIX"),
		MaxInlinePayloadBytes: c.GetInt64("OPENPIC_MAX_INLINE_PAYLOAD_BYTES", DefaultMaxInlinePayloadBytes),
		Overwrite:             c.GetBool("OPENPIC_OVERWRITE", DefaultOverwrite),
		Timeout:               ParseDuration(c.getFirst("OPENPIC_TIMEOUT", "VISION_TIMEOUT"), DefaultTimeout),
		LogLevel:              c.GetString("OPENPIC_LOG_LEVEL", c.GetString("VISION_LOG_LEVEL", DefaultLogLevel)),
	}

	// Treat non-positive overrides as a request for the default. This
	// keeps the inline payload guard always on; configurations that try
	// to disable it via 0 or a negative value still get a safe budget.
	if cfg.MaxInlinePayloadBytes <= 0 {
		cfg.MaxInlinePayloadBytes = DefaultMaxInlinePayloadBytes
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// ExtendedConfig holds extended configuration options.
type ExtendedConfig struct {
	*Config

	// Retry configuration
	MaxRetries     int           `json:"max_retries"`
	RetryBaseDelay time.Duration `json:"retry_base_delay"`
	RetryMaxDelay  time.Duration `json:"retry_max_delay"`

	// File processing configuration
	MaxImageSize     int64    `json:"max_image_size"`
	AllowedFormats   []string `json:"allowed_formats"`
	MaxImagesCompare int      `json:"max_images_compare"`
}

// BuildExtendedConfig builds an ExtendedConfig from the layered configuration.
func (c *LayeredConfig) BuildExtendedConfig() (*ExtendedConfig, error) {
	baseCfg, err := c.BuildConfig()
	if err != nil {
		return nil, err
	}

	return &ExtendedConfig{
		Config:           baseCfg,
		MaxRetries:       c.GetInt("VISION_MAX_RETRIES", 3),
		RetryBaseDelay:   c.GetDuration("VISION_RETRY_BASE_DELAY", 1*time.Second),
		RetryMaxDelay:    c.GetDuration("VISION_RETRY_MAX_DELAY", 30*time.Second),
		MaxImageSize:     c.GetInt64("VISION_MAX_IMAGE_SIZE", 20*1024*1024),
		AllowedFormats:   c.GetStringSlice("VISION_ALLOWED_FORMATS", []string{"jpg", "jpeg", "png", "gif", "webp"}),
		MaxImagesCompare: c.GetInt("VISION_MAX_IMAGES_COMPARE", 4),
	}, nil
}

// LoadLayered creates a layered configuration with standard sources.
// Priority order (highest to lowest):
// 1. Runtime options (if provided)
// 2. Environment variables
// 3. Configuration file (if found)
// 4. Default values
func LoadLayered(runtimeOpts map[string]string) (*LayeredConfig, error) {
	lc := NewLayeredConfig()

	// Add default source (lowest priority)
	lc.AddSource(PriorityDefault, NewDefaultSource())

	// Add file source if config file exists
	if configFile := FindConfigFile(); configFile != "" {
		fileSource, err := NewFileSource(configFile)
		if err == nil {
			lc.AddSource(PriorityFile, fileSource)
		}
		// Ignore file errors - file config is optional
	}

	// Add environment source
	lc.AddSource(PriorityEnv, NewEnvSource(""))

	// Add runtime options if provided (highest priority)
	if len(runtimeOpts) > 0 {
		lc.AddSource(PriorityRuntime, NewMapSource("runtime", runtimeOpts))
	}

	return lc, nil
}

// LoadWithOptions loads configuration with runtime options override.
func LoadWithOptions(opts map[string]string) (*Config, error) {
	lc, err := LoadLayered(opts)
	if err != nil {
		return nil, err
	}
	return lc.BuildConfig()
}

// LoadExtended loads extended configuration with all options.
func LoadExtended(opts map[string]string) (*ExtendedConfig, error) {
	lc, err := LoadLayered(opts)
	if err != nil {
		return nil, err
	}
	return lc.BuildExtendedConfig()
}
