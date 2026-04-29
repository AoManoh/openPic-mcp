package config

import (
	"testing"
	"time"
)

func resetConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OPENPIC_API_BASE_URL",
		"OPENPIC_API_KEY",
		"OPENPIC_VISION_MODEL",
		"OPENPIC_IMAGE_MODEL",
		"OPENPIC_TIMEOUT",
		"OPENPIC_LOG_LEVEL",
		"OPENPIC_LOG_FORMAT",
		"OPENPIC_OUTPUT_DIR",
		"OPENPIC_FILENAME_PREFIX",
		"OPENPIC_MAX_INLINE_PAYLOAD_BYTES",
		"OPENPIC_OVERWRITE",
		"OPENPIC_MAX_CONCURRENT_REQUESTS",
		"OPENPIC_REQUEST_QUEUE_SIZE",
		"OPENPIC_REQUEST_TIMEOUT",
		"OPENPIC_SHUTDOWN_TIMEOUT",
		"OPENPIC_TASK_STORE_ENABLED",
		"OPENPIC_TASK_DISK_PERSIST",
		"OPENPIC_TASK_MAX_QUEUED",
		"OPENPIC_TASK_MAX_RETAINED",
		"OPENPIC_TASK_TTL",
		"VISION_API_BASE_URL",
		"VISION_API_KEY",
		"VISION_MODEL",
		"VISION_TIMEOUT",
		"VISION_LOG_LEVEL",
	} {
		t.Setenv(key, "")
	}
}

func setRequiredImageEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OPENPIC_API_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENPIC_API_KEY", "test-key")
	t.Setenv("OPENPIC_VISION_MODEL", "gpt-4o")
}

func TestLoad_Success(t *testing.T) {
	resetConfigEnv(t)
	// Set required environment variables
	t.Setenv("OPENPIC_API_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENPIC_API_KEY", "test-key")
	t.Setenv("OPENPIC_VISION_MODEL", "gpt-4o")
	t.Setenv("OPENPIC_IMAGE_MODEL", "gpt-image-1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.APIBaseURL != "https://api.openai.com/v1" {
		t.Errorf("APIBaseURL = %v, want %v", cfg.APIBaseURL, "https://api.openai.com/v1")
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %v, want %v", cfg.APIKey, "test-key")
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("Model = %v, want %v", cfg.Model, "gpt-4o")
	}
	if cfg.VisionModel != "gpt-4o" {
		t.Errorf("VisionModel = %v, want %v", cfg.VisionModel, "gpt-4o")
	}
	if cfg.ImageModel != "gpt-image-1" {
		t.Errorf("ImageModel = %v, want %v", cfg.ImageModel, "gpt-image-1")
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, DefaultLogLevel)
	}
}

func TestLoad_LegacyVisionEnv(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("VISION_API_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("VISION_API_KEY", "legacy-key")
	t.Setenv("VISION_MODEL", "gpt-4o")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.APIKey != "legacy-key" {
		t.Errorf("APIKey = %v, want %v", cfg.APIKey, "legacy-key")
	}
	if cfg.VisionModel != "gpt-4o" {
		t.Errorf("VisionModel = %v, want %v", cfg.VisionModel, "gpt-4o")
	}
}

func TestLoad_OpenPicOverridesLegacyVisionEnv(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("OPENPIC_API_BASE_URL", "https://openpic.example.com/v1")
	t.Setenv("OPENPIC_API_KEY", "openpic-key")
	t.Setenv("OPENPIC_VISION_MODEL", "openpic-vision-model")
	t.Setenv("VISION_API_BASE_URL", "https://legacy.example.com/v1")
	t.Setenv("VISION_API_KEY", "legacy-key")
	t.Setenv("VISION_MODEL", "legacy-model")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.APIBaseURL != "https://openpic.example.com/v1" {
		t.Errorf("APIBaseURL = %v, want %v", cfg.APIBaseURL, "https://openpic.example.com/v1")
	}
	if cfg.APIKey != "openpic-key" {
		t.Errorf("APIKey = %v, want %v", cfg.APIKey, "openpic-key")
	}
	if cfg.VisionModel != "openpic-vision-model" {
		t.Errorf("VisionModel = %v, want %v", cfg.VisionModel, "openpic-vision-model")
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr string
	}{
		{
			name: "missing API base URL",
			envVars: map[string]string{
				"VISION_API_KEY": "test-key",
				"VISION_MODEL":   "gpt-4o",
			},
			wantErr: "OPENPIC_API_BASE_URL or VISION_API_BASE_URL is required",
		},
		{
			name: "missing API key",
			envVars: map[string]string{
				"VISION_API_BASE_URL": "https://api.openai.com/v1",
				"VISION_MODEL":        "gpt-4o",
			},
			wantErr: "OPENPIC_API_KEY or VISION_API_KEY is required",
		},
		{
			name: "missing model",
			envVars: map[string]string{
				"VISION_API_BASE_URL": "https://api.openai.com/v1",
				"VISION_API_KEY":      "test-key",
			},
			wantErr: "OPENPIC_VISION_MODEL or VISION_MODEL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetConfigEnv(t)

			// Set test env vars
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			if err.Error() != "configuration validation failed: "+tt.wantErr {
				t.Errorf("Load() error = %v, want containing %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoad_CustomTimeout(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("VISION_API_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("VISION_API_KEY", "test-key")
	t.Setenv("VISION_MODEL", "gpt-4o")
	t.Setenv("VISION_TIMEOUT", "60s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 60*time.Second)
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("VISION_API_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("VISION_API_KEY", "test-key")
	t.Setenv("VISION_MODEL", "gpt-4o")
	t.Setenv("VISION_TIMEOUT", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoad_ImageOutputDefaults(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.OutputDir != "" {
		t.Errorf("OutputDir default = %q, want empty", cfg.OutputDir)
	}
	if cfg.FilenamePrefix != "" {
		t.Errorf("FilenamePrefix default = %q, want empty", cfg.FilenamePrefix)
	}
	if cfg.MaxInlinePayloadBytes != DefaultMaxInlinePayloadBytes {
		t.Errorf("MaxInlinePayloadBytes default = %d, want %d", cfg.MaxInlinePayloadBytes, DefaultMaxInlinePayloadBytes)
	}
	if cfg.Overwrite != DefaultOverwrite {
		t.Errorf("Overwrite default = %v, want %v", cfg.Overwrite, DefaultOverwrite)
	}
}

func TestLoad_ImageOutputOverrides(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_OUTPUT_DIR", "/tmp/openpic-test-out")
	t.Setenv("OPENPIC_FILENAME_PREFIX", "demo")
	t.Setenv("OPENPIC_MAX_INLINE_PAYLOAD_BYTES", "262144")
	t.Setenv("OPENPIC_OVERWRITE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.OutputDir != "/tmp/openpic-test-out" {
		t.Errorf("OutputDir = %q, want /tmp/openpic-test-out", cfg.OutputDir)
	}
	if cfg.FilenamePrefix != "demo" {
		t.Errorf("FilenamePrefix = %q, want demo", cfg.FilenamePrefix)
	}
	if cfg.MaxInlinePayloadBytes != 262144 {
		t.Errorf("MaxInlinePayloadBytes = %d, want 262144", cfg.MaxInlinePayloadBytes)
	}
	if !cfg.Overwrite {
		t.Errorf("Overwrite = false, want true")
	}
}

func TestLoad_NonPositiveMaxInlinePayloadBytesFallsBack(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_MAX_INLINE_PAYLOAD_BYTES", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.MaxInlinePayloadBytes != DefaultMaxInlinePayloadBytes {
		t.Errorf("MaxInlinePayloadBytes = %d, want default %d", cfg.MaxInlinePayloadBytes, DefaultMaxInlinePayloadBytes)
	}
}

func TestLoad_InvalidMaxInlinePayloadBytes(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_MAX_INLINE_PAYLOAD_BYTES", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for invalid OPENPIC_MAX_INLINE_PAYLOAD_BYTES")
	}
}

func TestLoad_InvalidOverwrite(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_OVERWRITE", "maybe")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for invalid OPENPIC_OVERWRITE")
	}
}

func TestLoad_ServerEngineDefaults(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.MaxConcurrentRequests != DefaultMaxConcurrentRequests {
		t.Errorf("MaxConcurrentRequests = %d, want %d", cfg.MaxConcurrentRequests, DefaultMaxConcurrentRequests)
	}
	if cfg.RequestQueueSize != DefaultRequestQueueSize {
		t.Errorf("RequestQueueSize = %d, want %d", cfg.RequestQueueSize, DefaultRequestQueueSize)
	}
	if cfg.RequestTimeout != DefaultRequestTimeout {
		t.Errorf("RequestTimeout = %v, want %v", cfg.RequestTimeout, DefaultRequestTimeout)
	}
	if cfg.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, DefaultShutdownTimeout)
	}
	if cfg.LogFormat != DefaultLogFormat {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, DefaultLogFormat)
	}
}

func TestLoad_ServerEngineOverrides(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_MAX_CONCURRENT_REQUESTS", "32")
	t.Setenv("OPENPIC_REQUEST_QUEUE_SIZE", "128")
	t.Setenv("OPENPIC_REQUEST_TIMEOUT", "90s")
	t.Setenv("OPENPIC_SHUTDOWN_TIMEOUT", "10s")
	t.Setenv("OPENPIC_LOG_FORMAT", "json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.MaxConcurrentRequests != 32 {
		t.Errorf("MaxConcurrentRequests = %d, want 32", cfg.MaxConcurrentRequests)
	}
	if cfg.RequestQueueSize != 128 {
		t.Errorf("RequestQueueSize = %d, want 128", cfg.RequestQueueSize)
	}
	if cfg.RequestTimeout != 90*time.Second {
		t.Errorf("RequestTimeout = %v, want 90s", cfg.RequestTimeout)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 10s", cfg.ShutdownTimeout)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
}

func TestLoad_ServerEngineClampsCaps(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_MAX_CONCURRENT_REQUESTS", "1000000")
	t.Setenv("OPENPIC_REQUEST_QUEUE_SIZE", "1000000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.MaxConcurrentRequests != MaxConcurrentRequestsCap {
		t.Errorf("MaxConcurrentRequests = %d, want clamp %d", cfg.MaxConcurrentRequests, MaxConcurrentRequestsCap)
	}
	if cfg.RequestQueueSize != RequestQueueSizeCap {
		t.Errorf("RequestQueueSize = %d, want clamp %d", cfg.RequestQueueSize, RequestQueueSizeCap)
	}
}

func TestLoad_ServerEngineRejectsInvalid(t *testing.T) {
	cases := []struct {
		name   string
		envVar string
		value  string
	}{
		{name: "non-numeric concurrent", envVar: "OPENPIC_MAX_CONCURRENT_REQUESTS", value: "many"},
		{name: "non-numeric queue", envVar: "OPENPIC_REQUEST_QUEUE_SIZE", value: "lots"},
		{name: "bad request timeout", envVar: "OPENPIC_REQUEST_TIMEOUT", value: "soon"},
		{name: "negative request timeout", envVar: "OPENPIC_REQUEST_TIMEOUT", value: "-1s"},
		{name: "bad shutdown timeout", envVar: "OPENPIC_SHUTDOWN_TIMEOUT", value: "later"},
		{name: "zero shutdown timeout", envVar: "OPENPIC_SHUTDOWN_TIMEOUT", value: "0s"},
		{name: "unknown log format", envVar: "OPENPIC_LOG_FORMAT", value: "yaml"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resetConfigEnv(t)
			setRequiredImageEnv(t)
			t.Setenv(tt.envVar, tt.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() error = nil, want error for %s=%q", tt.envVar, tt.value)
			}
		})
	}
}

func TestLoad_ServerEngineZeroFallsBackToDefaults(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	// "0" disables nothing — Load must still produce a runnable engine.
	t.Setenv("OPENPIC_MAX_CONCURRENT_REQUESTS", "0")
	t.Setenv("OPENPIC_REQUEST_QUEUE_SIZE", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.MaxConcurrentRequests != DefaultMaxConcurrentRequests {
		t.Errorf("MaxConcurrentRequests = %d, want default %d", cfg.MaxConcurrentRequests, DefaultMaxConcurrentRequests)
	}
	if cfg.RequestQueueSize != DefaultRequestQueueSize {
		t.Errorf("RequestQueueSize = %d, want default %d", cfg.RequestQueueSize, DefaultRequestQueueSize)
	}
}

func TestLoad_TaskStoreDefaults(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TaskStoreEnabled != DefaultTaskStoreEnabled {
		t.Errorf("TaskStoreEnabled = %v, want %v", cfg.TaskStoreEnabled, DefaultTaskStoreEnabled)
	}
	if cfg.TaskDiskPersist != DefaultTaskDiskPersist {
		t.Errorf("TaskDiskPersist = %v, want %v", cfg.TaskDiskPersist, DefaultTaskDiskPersist)
	}
	if cfg.TaskMaxQueued != DefaultTaskMaxQueued {
		t.Errorf("TaskMaxQueued = %d, want %d", cfg.TaskMaxQueued, DefaultTaskMaxQueued)
	}
	if cfg.TaskMaxRetained != DefaultTaskMaxRetained {
		t.Errorf("TaskMaxRetained = %d, want %d", cfg.TaskMaxRetained, DefaultTaskMaxRetained)
	}
	if cfg.TaskTTL != DefaultTaskTTL {
		t.Errorf("TaskTTL = %s, want %s", cfg.TaskTTL, DefaultTaskTTL)
	}
}

func TestLoad_TaskStoreOverrides(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_TASK_STORE_ENABLED", "false")
	t.Setenv("OPENPIC_TASK_DISK_PERSIST", "false")
	t.Setenv("OPENPIC_TASK_MAX_QUEUED", "512")
	t.Setenv("OPENPIC_TASK_MAX_RETAINED", "2048")
	t.Setenv("OPENPIC_TASK_TTL", "12h")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TaskStoreEnabled {
		t.Error("TaskStoreEnabled should be false")
	}
	if cfg.TaskDiskPersist {
		t.Error("TaskDiskPersist should be false")
	}
	if cfg.TaskMaxQueued != 512 {
		t.Errorf("TaskMaxQueued = %d, want 512", cfg.TaskMaxQueued)
	}
	if cfg.TaskMaxRetained != 2048 {
		t.Errorf("TaskMaxRetained = %d, want 2048", cfg.TaskMaxRetained)
	}
	if cfg.TaskTTL != 12*time.Hour {
		t.Errorf("TaskTTL = %s, want 12h", cfg.TaskTTL)
	}
}

func TestLoad_TaskStoreClampsCaps(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_TASK_MAX_QUEUED", "999999")
	t.Setenv("OPENPIC_TASK_MAX_RETAINED", "9999999")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TaskMaxQueued != TaskMaxQueuedCap {
		t.Errorf("TaskMaxQueued = %d, want cap %d", cfg.TaskMaxQueued, TaskMaxQueuedCap)
	}
	if cfg.TaskMaxRetained != TaskMaxRetainedCap {
		t.Errorf("TaskMaxRetained = %d, want cap %d", cfg.TaskMaxRetained, TaskMaxRetainedCap)
	}
}

func TestLoad_TaskStoreInvalidValues(t *testing.T) {
	cases := []struct {
		name   string
		envVar string
		value  string
	}{
		{"bool enabled", "OPENPIC_TASK_STORE_ENABLED", "maybe"},
		{"bool persist", "OPENPIC_TASK_DISK_PERSIST", "yesplease"},
		{"int queued", "OPENPIC_TASK_MAX_QUEUED", "abc"},
		{"int retained", "OPENPIC_TASK_MAX_RETAINED", "xyz"},
		{"duration ttl bad", "OPENPIC_TASK_TTL", "5banana"},
		{"duration ttl zero", "OPENPIC_TASK_TTL", "0s"},
		{"duration ttl negative", "OPENPIC_TASK_TTL", "-1h"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resetConfigEnv(t)
			setRequiredImageEnv(t)
			t.Setenv(tt.envVar, tt.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() expected error for %s=%q, got nil", tt.envVar, tt.value)
			}
		})
	}
}

func TestLoad_TaskStoreZeroFallsBackToDefaults(t *testing.T) {
	resetConfigEnv(t)
	setRequiredImageEnv(t)
	t.Setenv("OPENPIC_TASK_MAX_QUEUED", "0")
	t.Setenv("OPENPIC_TASK_MAX_RETAINED", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TaskMaxQueued != DefaultTaskMaxQueued {
		t.Errorf("TaskMaxQueued = %d, want default %d", cfg.TaskMaxQueued, DefaultTaskMaxQueued)
	}
	if cfg.TaskMaxRetained != DefaultTaskMaxRetained {
		t.Errorf("TaskMaxRetained = %d, want default %d", cfg.TaskMaxRetained, DefaultTaskMaxRetained)
	}
}

func TestBuildConfig_TaskStoreFallbacks(t *testing.T) {
	// LayeredConfig path: BuildConfig is forgiving, so non-positive TTL
	// from a file source (where Load's strict check doesn't run) should
	// fall back to the default rather than fail validation.
	lc := NewLayeredConfig()
	lc.AddSource(PriorityRuntime, NewMapSource("test", map[string]string{
		"OPENPIC_API_BASE_URL": "https://api.example.com/v1",
		"OPENPIC_API_KEY":      "k",
		"OPENPIC_VISION_MODEL": "m",
		"OPENPIC_TASK_TTL":     "0s", // zero → fallback
	}))
	cfg, err := lc.BuildConfig()
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if cfg.TaskTTL != DefaultTaskTTL {
		t.Errorf("TaskTTL = %s, want default %s", cfg.TaskTTL, DefaultTaskTTL)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				APIBaseURL: "https://api.openai.com/v1",
				APIKey:     "test-key",
				Model:      "gpt-4o",
			},
			wantErr: false,
		},
		{
			name: "empty API base URL",
			config: Config{
				APIKey: "test-key",
				Model:  "gpt-4o",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
