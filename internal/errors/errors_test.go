package errors

import (
	"errors"
	"testing"
)

func TestVisionError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *VisionError
		expected string
	}{
		{
			name: "with provider",
			err: &VisionError{
				Message:  "test error",
				Code:     CodeProviderError,
				Provider: "openai",
			},
			expected: "[PROVIDER_ERROR] openai: test error",
		},
		{
			name: "without provider",
			err: &VisionError{
				Message: "test error",
				Code:    CodeConfigError,
			},
			expected: "[CONFIG_ERROR] test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("VisionError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestVisionError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	err := &VisionError{
		Message: "wrapped error",
		Code:    CodeNetworkError,
		Err:     originalErr,
	}

	if unwrapped := err.Unwrap(); unwrapped != originalErr {
		t.Errorf("VisionError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}
}

func TestNewVisionError(t *testing.T) {
	originalErr := errors.New("original")
	err := NewVisionError("test message", CodeProviderError,
		WithProvider("gemini"),
		WithStatusCode(500),
		WithError(originalErr),
	)

	if err.Message != "test message" {
		t.Errorf("Message = %q, want %q", err.Message, "test message")
	}
	if err.Code != CodeProviderError {
		t.Errorf("Code = %q, want %q", err.Code, CodeProviderError)
	}
	if err.Provider != "gemini" {
		t.Errorf("Provider = %q, want %q", err.Provider, "gemini")
	}
	if err.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 500)
	}
	if err.Err != originalErr {
		t.Errorf("Err = %v, want %v", err.Err, originalErr)
	}
}

func TestNewConfigurationError(t *testing.T) {
	err := NewConfigurationError("missing API key", "VISION_API_KEY")

	if err.Variable != "VISION_API_KEY" {
		t.Errorf("Variable = %q, want %q", err.Variable, "VISION_API_KEY")
	}
	if err.Code != CodeConfigError {
		t.Errorf("Code = %q, want %q", err.Code, CodeConfigError)
	}
	if err.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 400)
	}
}

func TestNewProviderError(t *testing.T) {
	err := NewProviderError("API call failed", "openai", WithStatusCode(503))

	if err.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", err.Provider, "openai")
	}
	if err.Code != CodeProviderError {
		t.Errorf("Code = %q, want %q", err.Code, CodeProviderError)
	}
	if err.StatusCode != 503 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 503)
	}
}

func TestNewFileNotFoundError(t *testing.T) {
	err := NewFileNotFoundError("file123", "gemini")

	if err.FileID != "file123" {
		t.Errorf("FileID = %q, want %q", err.FileID, "file123")
	}
	if err.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 404)
	}
}

func TestNewUnsupportedFileTypeError(t *testing.T) {
	t.Run("with supported types", func(t *testing.T) {
		err := NewUnsupportedFileTypeError("image/bmp", []string{"image/jpeg", "image/png"})
		if err.MIMEType != "image/bmp" {
			t.Errorf("MIMEType = %q, want %q", err.MIMEType, "image/bmp")
		}
		if len(err.SupportedTypes) != 2 {
			t.Errorf("SupportedTypes length = %d, want %d", len(err.SupportedTypes), 2)
		}
	})

	t.Run("without supported types", func(t *testing.T) {
		err := NewUnsupportedFileTypeError("image/bmp", nil)
		if err.MIMEType != "image/bmp" {
			t.Errorf("MIMEType = %q, want %q", err.MIMEType, "image/bmp")
		}
	})
}

func TestNewFileSizeExceededError(t *testing.T) {
	err := NewFileSizeExceededError(1024*1024*10, 1024*1024*5)

	if err.FileSize != 1024*1024*10 {
		t.Errorf("FileSize = %d, want %d", err.FileSize, 1024*1024*10)
	}
	if err.MaxSize != 1024*1024*5 {
		t.Errorf("MaxSize = %d, want %d", err.MaxSize, 1024*1024*5)
	}
	if err.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 400)
	}
}

func TestNewAuthenticationError(t *testing.T) {
	err := NewAuthenticationError("invalid API key", "openai")

	if err.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 401)
	}
	if err.Code != CodeAuthenticationError {
		t.Errorf("Code = %q, want %q", err.Code, CodeAuthenticationError)
	}
}

func TestNewAuthorizationError(t *testing.T) {
	err := NewAuthorizationError("access denied", "gemini")

	if err.StatusCode != 403 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 403)
	}
	if err.Code != CodeAuthorizationError {
		t.Errorf("Code = %q, want %q", err.Code, CodeAuthorizationError)
	}
}

func TestNewNetworkError(t *testing.T) {
	originalErr := errors.New("connection refused")
	err := NewNetworkError("failed to connect", originalErr)

	if err.Code != CodeNetworkError {
		t.Errorf("Code = %q, want %q", err.Code, CodeNetworkError)
	}
	if err.Err != originalErr {
		t.Errorf("Err = %v, want %v", err.Err, originalErr)
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("invalid image format", "image")

	if err.Field != "image" {
		t.Errorf("Field = %q, want %q", err.Field, "image")
	}
	if err.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 400)
	}
}

func TestNewAnalysisError(t *testing.T) {
	originalErr := errors.New("model error")
	err := NewAnalysisError("failed to analyze image", "openai", originalErr)

	if err.Code != CodeAnalysisError {
		t.Errorf("Code = %q, want %q", err.Code, CodeAnalysisError)
	}
	if err.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", err.Provider, "openai")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "rate limit error",
			err:      NewRateLimitExceededError("rate limit", "openai", 60),
			expected: true,
		},
		{
			name:     "network error",
			err:      NewNetworkError("connection failed", nil),
			expected: true,
		},
		{
			name:     "config error",
			err:      NewConfigurationError("missing key", "API_KEY"),
			expected: false,
		},
		{
			name:     "provider error",
			err:      NewProviderError("API error", "openai"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.expected {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"config error", NewConfigurationError("error", "var"), 400},
		{"file not found", NewFileNotFoundError("file", ""), 404},
		{"rate limit", NewRateLimitExceededError("limit", "", 0), 429},
		{"auth error", NewAuthenticationError("auth", ""), 401},
		{"authz error", NewAuthorizationError("authz", ""), 403},
		{"validation error", NewValidationError("invalid", "field"), 400},
		{"unknown error", errors.New("unknown"), 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStatusCode(tt.err); got != tt.expected {
				t.Errorf("GetStatusCode() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestNewRateLimitExceededError(t *testing.T) {
	err := NewRateLimitExceededError("rate limit exceeded", "openai", 60)

	if err.RetryAfter != 60 {
		t.Errorf("RetryAfter = %d, want %d", err.RetryAfter, 60)
	}
	if err.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, 429)
	}
}
