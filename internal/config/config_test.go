package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Success(t *testing.T) {
	// Set required environment variables
	os.Setenv("VISION_API_BASE_URL", "https://api.openai.com/v1")
	os.Setenv("VISION_API_KEY", "sk-test-key")
	os.Setenv("VISION_MODEL", "gpt-4o")
	defer func() {
		os.Unsetenv("VISION_API_BASE_URL")
		os.Unsetenv("VISION_API_KEY")
		os.Unsetenv("VISION_MODEL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.APIBaseURL != "https://api.openai.com/v1" {
		t.Errorf("APIBaseURL = %v, want %v", cfg.APIBaseURL, "https://api.openai.com/v1")
	}
	if cfg.APIKey != "sk-test-key" {
		t.Errorf("APIKey = %v, want %v", cfg.APIKey, "sk-test-key")
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("Model = %v, want %v", cfg.Model, "gpt-4o")
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, DefaultLogLevel)
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
				"VISION_API_KEY": "sk-test",
				"VISION_MODEL":   "gpt-4o",
			},
			wantErr: "VISION_API_BASE_URL is required",
		},
		{
			name: "missing API key",
			envVars: map[string]string{
				"VISION_API_BASE_URL": "https://api.openai.com/v1",
				"VISION_MODEL":        "gpt-4o",
			},
			wantErr: "VISION_API_KEY is required",
		},
		{
			name: "missing model",
			envVars: map[string]string{
				"VISION_API_BASE_URL": "https://api.openai.com/v1",
				"VISION_API_KEY":      "sk-test",
			},
			wantErr: "VISION_MODEL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars first
			os.Unsetenv("VISION_API_BASE_URL")
			os.Unsetenv("VISION_API_KEY")
			os.Unsetenv("VISION_MODEL")

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}
			defer func() {
				for k := range tt.envVars {
					os.Unsetenv(k)
				}
			}()

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
	os.Setenv("VISION_API_BASE_URL", "https://api.openai.com/v1")
	os.Setenv("VISION_API_KEY", "sk-test-key")
	os.Setenv("VISION_MODEL", "gpt-4o")
	os.Setenv("VISION_TIMEOUT", "60s")
	defer func() {
		os.Unsetenv("VISION_API_BASE_URL")
		os.Unsetenv("VISION_API_KEY")
		os.Unsetenv("VISION_MODEL")
		os.Unsetenv("VISION_TIMEOUT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 60*time.Second)
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	os.Setenv("VISION_API_BASE_URL", "https://api.openai.com/v1")
	os.Setenv("VISION_API_KEY", "sk-test-key")
	os.Setenv("VISION_MODEL", "gpt-4o")
	os.Setenv("VISION_TIMEOUT", "invalid")
	defer func() {
		os.Unsetenv("VISION_API_BASE_URL")
		os.Unsetenv("VISION_API_KEY")
		os.Unsetenv("VISION_MODEL")
		os.Unsetenv("VISION_TIMEOUT")
	}()

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
				APIKey:     "sk-test",
				Model:      "gpt-4o",
			},
			wantErr: false,
		},
		{
			name: "empty API base URL",
			config: Config{
				APIKey: "sk-test",
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
