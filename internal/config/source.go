// Package config provides layered configuration management for the Vision MCP Server.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Source represents a configuration source.
type Source interface {
	// Name returns the name of the source for debugging.
	Name() string
	// Get returns the value for a key, or empty string if not found.
	Get(key string) string
	// Has returns true if the key exists in this source.
	Has(key string) bool
}

// Priority levels for configuration sources.
const (
	PriorityDefault = 0  // Lowest priority - default values
	PriorityFile    = 10 // Configuration file
	PriorityEnv     = 20 // Environment variables
	PriorityRuntime = 30 // Runtime/function parameters (highest)
)

// DefaultSource provides default configuration values.
type DefaultSource struct {
	defaults map[string]string
}

// NewDefaultSource creates a new default source with predefined defaults.
func NewDefaultSource() *DefaultSource {
	return &DefaultSource{
		defaults: map[string]string{
			"VISION_TIMEOUT":          "5m",
			"VISION_LOG_LEVEL":        "info",
			"VISION_MAX_RETRIES":      "3",
			"VISION_RETRY_BASE_DELAY": "1s",
			"VISION_RETRY_MAX_DELAY":  "30s",
			"VISION_MAX_IMAGE_SIZE":   "20971520", // 20MB
			"VISION_ALLOWED_FORMATS":  "jpg,jpeg,png,gif,webp",
		},
	}
}

func (s *DefaultSource) Name() string { return "default" }

func (s *DefaultSource) Get(key string) string {
	return s.defaults[key]
}

func (s *DefaultSource) Has(key string) bool {
	_, ok := s.defaults[key]
	return ok
}

// SetDefault sets a default value.
func (s *DefaultSource) SetDefault(key, value string) {
	s.defaults[key] = value
}

// EnvSource reads configuration from environment variables.
type EnvSource struct {
	prefix string
}

// NewEnvSource creates a new environment source.
// If prefix is provided, it will be prepended to all keys (e.g., "VISION_").
func NewEnvSource(prefix string) *EnvSource {
	return &EnvSource{prefix: prefix}
}

func (s *EnvSource) Name() string { return "environment" }

func (s *EnvSource) Get(key string) string {
	envKey := key
	if s.prefix != "" && !strings.HasPrefix(key, s.prefix) {
		envKey = s.prefix + key
	}
	return os.Getenv(envKey)
}

func (s *EnvSource) Has(key string) bool {
	envKey := key
	if s.prefix != "" && !strings.HasPrefix(key, s.prefix) {
		envKey = s.prefix + key
	}
	_, ok := os.LookupEnv(envKey)
	return ok
}

// FileSource reads configuration from a JSON file.
type FileSource struct {
	path   string
	values map[string]string
}

// NewFileSource creates a new file source from a JSON configuration file.
func NewFileSource(path string) (*FileSource, error) {
	s := &FileSource{
		path:   path,
		values: make(map[string]string),
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *FileSource) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Flatten the JSON into string key-value pairs
	s.flatten("", raw)
	return nil
}

func (s *FileSource) flatten(prefix string, data map[string]interface{}) {
	for k, v := range data {
		key := k
		if prefix != "" {
			key = prefix + "_" + k
		}
		// Convert to uppercase for consistency with env vars
		key = strings.ToUpper(key)

		switch val := v.(type) {
		case string:
			s.values[key] = val
		case float64:
			s.values[key] = strconv.FormatFloat(val, 'f', -1, 64)
		case bool:
			s.values[key] = strconv.FormatBool(val)
		case map[string]interface{}:
			s.flatten(key, val)
		}
	}
}

func (s *FileSource) Name() string { return "file:" + s.path }

func (s *FileSource) Get(key string) string {
	return s.values[key]
}

func (s *FileSource) Has(key string) bool {
	_, ok := s.values[key]
	return ok
}

// MapSource reads configuration from a map (for runtime/function parameters).
type MapSource struct {
	name   string
	values map[string]string
}

// NewMapSource creates a new map source.
func NewMapSource(name string, values map[string]string) *MapSource {
	if values == nil {
		values = make(map[string]string)
	}
	return &MapSource{
		name:   name,
		values: values,
	}
}

func (s *MapSource) Name() string { return s.name }

func (s *MapSource) Get(key string) string {
	return s.values[key]
}

func (s *MapSource) Has(key string) bool {
	_, ok := s.values[key]
	return ok
}

// Set sets a value in the map source.
func (s *MapSource) Set(key, value string) {
	s.values[key] = value
}

// Delete removes a value from the map source.
func (s *MapSource) Delete(key string) {
	delete(s.values, key)
}

// FindConfigFile searches for a configuration file in common locations.
func FindConfigFile() string {
	// Search order:
	// 1. Current directory
	// 2. User config directory
	// 3. /etc/vision-mcp/ (Unix only)

	candidates := []string{
		"vision-mcp.json",
		".vision-mcp.json",
		"config.json",
	}

	// Check current directory
	for _, name := range candidates {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}

	// Check user config directory
	if configDir, err := os.UserConfigDir(); err == nil {
		for _, name := range candidates {
			path := filepath.Join(configDir, "vision-mcp", name)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	// Check home directory
	if homeDir, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(homeDir, ".vision-mcp.json")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// ParseDuration parses a duration string with fallback to default.
func ParseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

// ParseInt parses an integer string with fallback to default.
func ParseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return i
}

// ParseInt64 parses an int64 string with fallback to default.
func ParseInt64(s string, defaultVal int64) int64 {
	if s == "" {
		return defaultVal
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return defaultVal
	}
	return i
}

// ParseBool parses a boolean string with fallback to default.
func ParseBool(s string, defaultVal bool) bool {
	if s == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return defaultVal
	}
	return b
}

// ParseStringSlice parses a comma-separated string into a slice.
func ParseStringSlice(s string, defaultVal []string) []string {
	if s == "" {
		return defaultVal
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return defaultVal
	}
	return result
}
