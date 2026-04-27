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
		"VISION_API_BASE_URL",
		"VISION_API_KEY",
		"VISION_MODEL",
		"VISION_TIMEOUT",
		"VISION_LOG_LEVEL",
	} {
		t.Setenv(key, "")
	}
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
