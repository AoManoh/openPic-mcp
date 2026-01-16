package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	visionerrors "github.com/AoManoh/openPic-mcp/internal/errors"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", opts.MaxRetries)
	}
	if opts.BaseDelay != 1*time.Second {
		t.Errorf("BaseDelay = %v, want 1s", opts.BaseDelay)
	}
	if opts.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", opts.MaxDelay)
	}
	if opts.BackoffMultiplier != 2.0 {
		t.Errorf("BackoffMultiplier = %f, want 2.0", opts.BackoffMultiplier)
	}
	if !opts.Jitter {
		t.Error("Jitter should be true by default")
	}
}

func TestDoWithResult_Success(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	result, err := DoWithResult(ctx, nil, func() (string, error) {
		callCount++
		return "success", nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Value != "success" {
		t.Errorf("Value = %q, want %q", result.Value, "success")
	}
	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestDoWithResult_RetryOnNetworkError(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	opts := &Options{
		MaxRetries:        3,
		BaseDelay:         10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableErrors:   []string{visionerrors.CodeNetworkError},
	}

	result, err := DoWithResult(ctx, opts, func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", visionerrors.NewNetworkError("connection failed", nil)
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Value != "success" {
		t.Errorf("Value = %q, want %q", result.Value, "success")
	}
	if result.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", result.Attempts)
	}
}

func TestDoWithResult_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	opts := &Options{
		MaxRetries:        3,
		BaseDelay:         10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableErrors:   []string{visionerrors.CodeNetworkError},
	}

	_, err := DoWithResult(ctx, opts, func() (string, error) {
		callCount++
		return "", visionerrors.NewConfigurationError("invalid config", "API_KEY")
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (should not retry)", callCount)
	}
}

func TestDoWithResult_MaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	opts := &Options{
		MaxRetries:        2,
		BaseDelay:         10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableErrors:   []string{visionerrors.CodeNetworkError},
	}

	_, err := DoWithResult(ctx, opts, func() (string, error) {
		callCount++
		return "", visionerrors.NewNetworkError("connection failed", nil)
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if callCount != 3 { // 1 initial + 2 retries
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestDoWithResult_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	opts := &Options{
		MaxRetries:        5,
		BaseDelay:         100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableErrors:   []string{visionerrors.CodeNetworkError},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := DoWithResult(ctx, opts, func() (string, error) {
		callCount++
		return "", visionerrors.NewNetworkError("connection failed", nil)
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCalculateDelay(t *testing.T) {
	opts := &Options{
		BaseDelay:         1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            false,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
	}

	for _, tt := range tests {
		delay := calculateDelay(tt.attempt, opts)
		if delay != tt.expected {
			t.Errorf("calculateDelay(%d) = %v, want %v", tt.attempt, delay, tt.expected)
		}
	}
}

func TestCalculateDelay_MaxDelay(t *testing.T) {
	opts := &Options{
		BaseDelay:         1 * time.Second,
		MaxDelay:          5 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            false,
	}

	// Attempt 10 would be 1024 seconds without max cap
	delay := calculateDelay(10, opts)
	if delay != 5*time.Second {
		t.Errorf("calculateDelay(10) = %v, want %v (max delay)", delay, 5*time.Second)
	}
}

func TestShouldRetry(t *testing.T) {
	opts := &Options{
		RetryableErrors: []string{
			visionerrors.CodeNetworkError,
			visionerrors.CodeRateLimitExceeded,
		},
	}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "network error",
			err:      visionerrors.NewNetworkError("connection failed", nil),
			expected: true,
		},
		{
			name:     "rate limit error",
			err:      visionerrors.NewRateLimitExceededError("rate limit", "openai", 60),
			expected: true,
		},
		{
			name:     "config error",
			err:      visionerrors.NewConfigurationError("invalid", "KEY"),
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetry(tt.err, opts); got != tt.expected {
				t.Errorf("shouldRetry() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandler_Do(t *testing.T) {
	handler := NewHandler(&Options{
		MaxRetries:        2,
		BaseDelay:         10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableErrors:   []string{visionerrors.CodeNetworkError},
	})

	callCount := 0
	err := handler.Do(context.Background(), func() error {
		callCount++
		if callCount < 2 {
			return visionerrors.NewNetworkError("failed", nil)
		}
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestOnRetryCallback(t *testing.T) {
	retryAttempts := []int{}
	opts := &Options{
		MaxRetries:        3,
		BaseDelay:         10 * time.Millisecond,
		MaxDelay:          100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
		RetryableErrors:   []string{visionerrors.CodeNetworkError},
		OnRetry: func(attempt int, err error, delay time.Duration) {
			retryAttempts = append(retryAttempts, attempt)
		},
	}

	callCount := 0
	DoWithResult(context.Background(), opts, func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", visionerrors.NewNetworkError("failed", nil)
		}
		return "success", nil
	})

	if len(retryAttempts) != 2 {
		t.Errorf("retryAttempts length = %d, want 2", len(retryAttempts))
	}
	if retryAttempts[0] != 1 || retryAttempts[1] != 2 {
		t.Errorf("retryAttempts = %v, want [1, 2]", retryAttempts)
	}
}
