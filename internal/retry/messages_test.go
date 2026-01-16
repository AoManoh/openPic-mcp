package retry

import (
	"testing"

	"github.com/AoManoh/openPic-mcp/internal/errors"
)

func TestNewMessageResolver(t *testing.T) {
	resolver := NewMessageResolver(LangEnglish)
	if resolver == nil {
		t.Fatal("NewMessageResolver returned nil")
	}
	if resolver.lang != LangEnglish {
		t.Errorf("lang = %q, want %q", resolver.lang, LangEnglish)
	}
}

func TestMessageResolver_Resolve_English(t *testing.T) {
	resolver := NewMessageResolver(LangEnglish)

	tests := []struct {
		name          string
		err           error
		expectedTitle string
	}{
		{
			name:          "config error",
			err:           errors.NewConfigurationError("missing key", "API_KEY"),
			expectedTitle: "Configuration Error",
		},
		{
			name:          "network error",
			err:           errors.NewNetworkError("connection failed", nil),
			expectedTitle: "Network Error",
		},
		{
			name:          "rate limit error",
			err:           errors.NewRateLimitExceededError("too many requests", "openai", 60),
			expectedTitle: "Rate Limit Exceeded",
		},
		{
			name:          "auth error",
			err:           errors.NewAuthenticationError("invalid key", "openai"),
			expectedTitle: "Authentication Failed",
		},
		{
			name:          "file not found",
			err:           errors.NewFileNotFoundError("file123", "gemini"),
			expectedTitle: "File Not Found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := resolver.Resolve(tt.err)
			if msg.Title != tt.expectedTitle {
				t.Errorf("Title = %q, want %q", msg.Title, tt.expectedTitle)
			}
		})
	}
}

func TestMessageResolver_Resolve_Chinese(t *testing.T) {
	resolver := NewMessageResolver(LangChinese)

	tests := []struct {
		name          string
		err           error
		expectedTitle string
	}{
		{
			name:          "config error",
			err:           errors.NewConfigurationError("missing key", "API_KEY"),
			expectedTitle: "配置错误",
		},
		{
			name:          "network error",
			err:           errors.NewNetworkError("connection failed", nil),
			expectedTitle: "网络错误",
		},
		{
			name:          "rate limit error",
			err:           errors.NewRateLimitExceededError("too many requests", "openai", 60),
			expectedTitle: "请求频率超限",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := resolver.Resolve(tt.err)
			if msg.Title != tt.expectedTitle {
				t.Errorf("Title = %q, want %q", msg.Title, tt.expectedTitle)
			}
		})
	}
}

func TestMessageResolver_DefaultMessage(t *testing.T) {
	t.Run("English default", func(t *testing.T) {
		resolver := NewMessageResolver(LangEnglish)
		msg := resolver.Resolve(nil)
		if msg.Title != "Unknown Error" {
			t.Errorf("Title = %q, want %q", msg.Title, "Unknown Error")
		}
	})

	t.Run("Chinese default", func(t *testing.T) {
		resolver := NewMessageResolver(LangChinese)
		msg := resolver.Resolve(nil)
		if msg.Title != "未知错误" {
			t.Errorf("Title = %q, want %q", msg.Title, "未知错误")
		}
	})
}

func TestMessageResolver_FormatError(t *testing.T) {
	resolver := NewMessageResolver(LangEnglish)
	err := errors.NewNetworkError("connection failed", nil)

	formatted := resolver.FormatError(err)
	if formatted == "" {
		t.Error("FormatError returned empty string")
	}

	// Should contain title, description, and suggestion
	if len(formatted) < 50 {
		t.Errorf("FormatError output too short: %q", formatted)
	}
}

func TestErrorMessage_Retryable(t *testing.T) {
	resolver := NewMessageResolver(LangEnglish)

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "network error is retryable",
			err:       errors.NewNetworkError("failed", nil),
			retryable: true,
		},
		{
			name:      "rate limit is retryable",
			err:       errors.NewRateLimitExceededError("limit", "", 0),
			retryable: true,
		},
		{
			name:      "config error is not retryable",
			err:       errors.NewConfigurationError("invalid", "KEY"),
			retryable: false,
		},
		{
			name:      "auth error is not retryable",
			err:       errors.NewAuthenticationError("invalid", ""),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := resolver.Resolve(tt.err)
			if msg.Retryable != tt.retryable {
				t.Errorf("Retryable = %v, want %v", msg.Retryable, tt.retryable)
			}
		})
	}
}
