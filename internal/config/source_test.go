package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSource(t *testing.T) {
	s := NewDefaultSource()

	if s.Name() != "default" {
		t.Errorf("Name() = %q, want %q", s.Name(), "default")
	}

	// Check default values exist
	if !s.Has("VISION_TIMEOUT") {
		t.Error("expected VISION_TIMEOUT to exist")
	}

	if s.Get("VISION_TIMEOUT") != "30s" {
		t.Errorf("Get(VISION_TIMEOUT) = %q, want %q", s.Get("VISION_TIMEOUT"), "30s")
	}

	// Test SetDefault
	s.SetDefault("CUSTOM_KEY", "custom_value")
	if s.Get("CUSTOM_KEY") != "custom_value" {
		t.Errorf("Get(CUSTOM_KEY) = %q, want %q", s.Get("CUSTOM_KEY"), "custom_value")
	}
}

func TestEnvSource(t *testing.T) {
	s := NewEnvSource("")

	// Set test env var
	os.Setenv("TEST_CONFIG_KEY", "test_value")
	defer os.Unsetenv("TEST_CONFIG_KEY")

	if s.Name() != "environment" {
		t.Errorf("Name() = %q, want %q", s.Name(), "environment")
	}

	if !s.Has("TEST_CONFIG_KEY") {
		t.Error("expected TEST_CONFIG_KEY to exist")
	}

	if s.Get("TEST_CONFIG_KEY") != "test_value" {
		t.Errorf("Get(TEST_CONFIG_KEY) = %q, want %q", s.Get("TEST_CONFIG_KEY"), "test_value")
	}

	// Test non-existent key
	if s.Has("NON_EXISTENT_KEY_12345") {
		t.Error("expected NON_EXISTENT_KEY_12345 to not exist")
	}
}

func TestEnvSource_WithPrefix(t *testing.T) {
	s := NewEnvSource("MYAPP_")

	os.Setenv("MYAPP_DATABASE_URL", "postgres://localhost")
	defer os.Unsetenv("MYAPP_DATABASE_URL")

	// Should find with prefix
	if !s.Has("DATABASE_URL") {
		t.Error("expected DATABASE_URL to exist with prefix")
	}

	if s.Get("DATABASE_URL") != "postgres://localhost" {
		t.Errorf("Get(DATABASE_URL) = %q, want %q", s.Get("DATABASE_URL"), "postgres://localhost")
	}
}

func TestMapSource(t *testing.T) {
	values := map[string]string{
		"KEY1": "value1",
		"KEY2": "value2",
	}
	s := NewMapSource("test", values)

	if s.Name() != "test" {
		t.Errorf("Name() = %q, want %q", s.Name(), "test")
	}

	if !s.Has("KEY1") {
		t.Error("expected KEY1 to exist")
	}

	if s.Get("KEY1") != "value1" {
		t.Errorf("Get(KEY1) = %q, want %q", s.Get("KEY1"), "value1")
	}

	// Test Set
	s.Set("KEY3", "value3")
	if s.Get("KEY3") != "value3" {
		t.Errorf("Get(KEY3) = %q, want %q", s.Get("KEY3"), "value3")
	}

	// Test Delete
	s.Delete("KEY1")
	if s.Has("KEY1") {
		t.Error("expected KEY1 to be deleted")
	}
}

func TestMapSource_NilValues(t *testing.T) {
	s := NewMapSource("test", nil)
	if s.Has("ANY_KEY") {
		t.Error("expected empty map source to have no keys")
	}
}

func TestFileSource(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
		"vision": {
			"api_base_url": "https://api.example.com",
			"timeout": "60s"
		},
		"log_level": "debug",
		"max_retries": 5
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	s, err := NewFileSource(configPath)
	if err != nil {
		t.Fatalf("NewFileSource() error = %v", err)
	}

	if !s.Has("LOG_LEVEL") {
		t.Error("expected LOG_LEVEL to exist")
	}

	if s.Get("LOG_LEVEL") != "debug" {
		t.Errorf("Get(LOG_LEVEL) = %q, want %q", s.Get("LOG_LEVEL"), "debug")
	}

	// Check nested values are flattened
	if s.Get("VISION_API_BASE_URL") != "https://api.example.com" {
		t.Errorf("Get(VISION_API_BASE_URL) = %q, want %q", s.Get("VISION_API_BASE_URL"), "https://api.example.com")
	}

	if s.Get("MAX_RETRIES") != "5" {
		t.Errorf("Get(MAX_RETRIES) = %q, want %q", s.Get("MAX_RETRIES"), "5")
	}
}

func TestFileSource_InvalidFile(t *testing.T) {
	_, err := NewFileSource("/non/existent/path.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		defVal   time.Duration
		expected time.Duration
	}{
		{"30s", 0, 30 * time.Second},
		{"1m", 0, 1 * time.Minute},
		{"", 10 * time.Second, 10 * time.Second},
		{"invalid", 5 * time.Second, 5 * time.Second},
	}

	for _, tt := range tests {
		result := ParseDuration(tt.input, tt.defVal)
		if result != tt.expected {
			t.Errorf("ParseDuration(%q, %v) = %v, want %v", tt.input, tt.defVal, result, tt.expected)
		}
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input    string
		defVal   int
		expected int
	}{
		{"42", 0, 42},
		{"", 10, 10},
		{"invalid", 5, 5},
	}

	for _, tt := range tests {
		result := ParseInt(tt.input, tt.defVal)
		if result != tt.expected {
			t.Errorf("ParseInt(%q, %d) = %d, want %d", tt.input, tt.defVal, result, tt.expected)
		}
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		input    string
		defVal   int64
		expected int64
	}{
		{"1000000", 0, 1000000},
		{"", 100, 100},
		{"invalid", 50, 50},
	}

	for _, tt := range tests {
		result := ParseInt64(tt.input, tt.defVal)
		if result != tt.expected {
			t.Errorf("ParseInt64(%q, %d) = %d, want %d", tt.input, tt.defVal, result, tt.expected)
		}
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		input    string
		defVal   bool
		expected bool
	}{
		{"true", false, true},
		{"false", true, false},
		{"1", false, true},
		{"0", true, false},
		{"", true, true},
		{"invalid", false, false},
	}

	for _, tt := range tests {
		result := ParseBool(tt.input, tt.defVal)
		if result != tt.expected {
			t.Errorf("ParseBool(%q, %v) = %v, want %v", tt.input, tt.defVal, result, tt.expected)
		}
	}
}

func TestParseStringSlice(t *testing.T) {
	tests := []struct {
		input    string
		defVal   []string
		expected []string
	}{
		{"a,b,c", nil, []string{"a", "b", "c"}},
		{"a, b, c", nil, []string{"a", "b", "c"}},
		{"", []string{"default"}, []string{"default"}},
		{"single", nil, []string{"single"}},
	}

	for _, tt := range tests {
		result := ParseStringSlice(tt.input, tt.defVal)
		if len(result) != len(tt.expected) {
			t.Errorf("ParseStringSlice(%q) length = %d, want %d", tt.input, len(result), len(tt.expected))
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("ParseStringSlice(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
			}
		}
	}
}
