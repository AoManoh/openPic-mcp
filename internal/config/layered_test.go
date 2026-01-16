package config

import (
	"os"
	"testing"
	"time"
)

func TestLayeredConfig_Priority(t *testing.T) {
	lc := NewLayeredConfig()

	// Add sources with different priorities
	defaultSource := NewMapSource("default", map[string]string{
		"KEY1": "default_value",
		"KEY2": "default_value",
	})
	envSource := NewMapSource("env", map[string]string{
		"KEY1": "env_value",
	})
	runtimeSource := NewMapSource("runtime", map[string]string{
		"KEY1": "runtime_value",
	})

	lc.AddSource(PriorityDefault, defaultSource)
	lc.AddSource(PriorityEnv, envSource)
	lc.AddSource(PriorityRuntime, runtimeSource)

	// KEY1 should come from runtime (highest priority)
	if val := lc.Get("KEY1"); val != "runtime_value" {
		t.Errorf("Get(KEY1) = %q, want %q", val, "runtime_value")
	}

	// KEY2 should come from default (only source that has it)
	if val := lc.Get("KEY2"); val != "default_value" {
		t.Errorf("Get(KEY2) = %q, want %q", val, "default_value")
	}
}

func TestLayeredConfig_GetWithSource(t *testing.T) {
	lc := NewLayeredConfig()

	lc.AddSource(PriorityDefault, NewMapSource("default", map[string]string{
		"KEY1": "value1",
	}))
	lc.AddSource(PriorityEnv, NewMapSource("env", map[string]string{
		"KEY1": "value2",
	}))

	val, source := lc.GetWithSource("KEY1")
	if val != "value2" {
		t.Errorf("value = %q, want %q", val, "value2")
	}
	if source != "env" {
		t.Errorf("source = %q, want %q", source, "env")
	}
}

func TestLayeredConfig_TypedGetters(t *testing.T) {
	lc := NewLayeredConfig()
	lc.AddSource(PriorityDefault, NewMapSource("test", map[string]string{
		"STRING_KEY":   "hello",
		"INT_KEY":      "42",
		"INT64_KEY":    "1000000",
		"BOOL_KEY":     "true",
		"DURATION_KEY": "30s",
		"SLICE_KEY":    "a,b,c",
	}))

	if val := lc.GetString("STRING_KEY", "default"); val != "hello" {
		t.Errorf("GetString() = %q, want %q", val, "hello")
	}

	if val := lc.GetInt("INT_KEY", 0); val != 42 {
		t.Errorf("GetInt() = %d, want %d", val, 42)
	}

	if val := lc.GetInt64("INT64_KEY", 0); val != 1000000 {
		t.Errorf("GetInt64() = %d, want %d", val, 1000000)
	}

	if val := lc.GetBool("BOOL_KEY", false); val != true {
		t.Errorf("GetBool() = %v, want %v", val, true)
	}

	if val := lc.GetDuration("DURATION_KEY", 0); val != 30*time.Second {
		t.Errorf("GetDuration() = %v, want %v", val, 30*time.Second)
	}

	slice := lc.GetStringSlice("SLICE_KEY", nil)
	if len(slice) != 3 || slice[0] != "a" {
		t.Errorf("GetStringSlice() = %v, want [a b c]", slice)
	}
}

func TestLayeredConfig_DefaultFallback(t *testing.T) {
	lc := NewLayeredConfig()

	if val := lc.GetString("NON_EXISTENT", "default"); val != "default" {
		t.Errorf("GetString() = %q, want %q", val, "default")
	}

	if val := lc.GetInt("NON_EXISTENT", 99); val != 99 {
		t.Errorf("GetInt() = %d, want %d", val, 99)
	}
}

func TestLayeredConfig_Sources(t *testing.T) {
	lc := NewLayeredConfig()
	lc.AddSource(PriorityDefault, NewMapSource("default", nil))
	lc.AddSource(PriorityEnv, NewMapSource("env", nil))
	lc.AddSource(PriorityRuntime, NewMapSource("runtime", nil))

	sources := lc.Sources()
	if len(sources) != 3 {
		t.Errorf("Sources() length = %d, want 3", len(sources))
	}

	// Should be in priority order (highest first)
	if sources[0] != "runtime" {
		t.Errorf("sources[0] = %q, want %q", sources[0], "runtime")
	}
}

func TestLoadLayered(t *testing.T) {
	// Set required env vars
	os.Setenv("VISION_API_BASE_URL", "https://api.test.com")
	os.Setenv("VISION_API_KEY", "test-key")
	os.Setenv("VISION_MODEL", "test-model")
	defer func() {
		os.Unsetenv("VISION_API_BASE_URL")
		os.Unsetenv("VISION_API_KEY")
		os.Unsetenv("VISION_MODEL")
	}()

	lc, err := LoadLayered(nil)
	if err != nil {
		t.Fatalf("LoadLayered() error = %v", err)
	}

	if val := lc.Get("VISION_API_BASE_URL"); val != "https://api.test.com" {
		t.Errorf("Get(VISION_API_BASE_URL) = %q, want %q", val, "https://api.test.com")
	}
}

func TestLoadLayered_WithRuntimeOpts(t *testing.T) {
	os.Setenv("VISION_API_BASE_URL", "https://env.test.com")
	os.Setenv("VISION_API_KEY", "env-key")
	os.Setenv("VISION_MODEL", "env-model")
	defer func() {
		os.Unsetenv("VISION_API_BASE_URL")
		os.Unsetenv("VISION_API_KEY")
		os.Unsetenv("VISION_MODEL")
	}()

	runtimeOpts := map[string]string{
		"VISION_API_BASE_URL": "https://runtime.test.com",
	}

	lc, err := LoadLayered(runtimeOpts)
	if err != nil {
		t.Fatalf("LoadLayered() error = %v", err)
	}

	// Runtime should override env
	if val := lc.Get("VISION_API_BASE_URL"); val != "https://runtime.test.com" {
		t.Errorf("Get(VISION_API_BASE_URL) = %q, want %q", val, "https://runtime.test.com")
	}

	// Env should still work for non-overridden keys
	if val := lc.Get("VISION_API_KEY"); val != "env-key" {
		t.Errorf("Get(VISION_API_KEY) = %q, want %q", val, "env-key")
	}
}
