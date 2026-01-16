// Package retry provides retry logic with exponential backoff for the Vision MCP Server.
package retry

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/anthropic/vision-mcp-server/internal/errors"
)

// Options configures retry behavior.
type Options struct {
	MaxRetries        int           // Maximum number of retry attempts (default: 3)
	BaseDelay         time.Duration // Initial delay between retries (default: 1s)
	MaxDelay          time.Duration // Maximum delay between retries (default: 30s)
	BackoffMultiplier float64       // Multiplier for exponential backoff (default: 2.0)
	Jitter            bool          // Add randomness to delay (default: true)
	RetryableErrors   []string      // Error codes that are retryable
	OnRetry           func(attempt int, err error, delay time.Duration)
}

// DefaultOptions returns the default retry options.
func DefaultOptions() *Options {
	return &Options{
		MaxRetries:        3,
		BaseDelay:         1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            true,
		RetryableErrors: []string{
			errors.CodeRateLimitExceeded,
			errors.CodeNetworkError,
		},
		OnRetry: nil,
	}
}

// Result contains the result of a retried operation.
type Result[T any] struct {
	Value      T
	Attempts   int
	TotalDelay time.Duration
}

// Handler provides retry functionality.
type Handler struct {
	opts *Options
}

// NewHandler creates a new retry handler with the given options.
func NewHandler(opts *Options) *Handler {
	if opts == nil {
		opts = DefaultOptions()
	}
	return &Handler{opts: opts}
}

// Do executes the operation with retry logic.
func (h *Handler) Do(ctx context.Context, operation func() error) error {
	_, err := DoWithResult(ctx, h.opts, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

// DoWithResult executes the operation with retry logic and returns the result.
func DoWithResult[T any](ctx context.Context, opts *Options, operation func() (T, error)) (*Result[T], error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	var lastErr error
	var totalDelay time.Duration
	var zero T

	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := operation()
		if err == nil {
			return &Result[T]{
				Value:      result,
				Attempts:   attempt + 1,
				TotalDelay: totalDelay,
			}, nil
		}

		lastErr = err

		// Don't retry on the last attempt or if error is not retryable
		if attempt == opts.MaxRetries || !shouldRetry(err, opts) {
			break
		}

		delay := calculateDelay(attempt, opts)
		totalDelay += delay

		if opts.OnRetry != nil {
			opts.OnRetry(attempt+1, err, delay)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	return &Result[T]{
		Value:      zero,
		Attempts:   opts.MaxRetries + 1,
		TotalDelay: totalDelay,
	}, lastErr
}

// WithExponentialBackoff executes the operation with exponential backoff.
func WithExponentialBackoff[T any](ctx context.Context, operation func() (T, error), opts *Options) (T, error) {
	if opts == nil {
		opts = DefaultOptions()
	}
	opts.BackoffMultiplier = 2.0
	opts.Jitter = true

	result, err := DoWithResult(ctx, opts, operation)
	if err != nil {
		var zero T
		return zero, err
	}
	return result.Value, nil
}

// WithLinearBackoff executes the operation with linear backoff.
func WithLinearBackoff[T any](ctx context.Context, operation func() (T, error), opts *Options) (T, error) {
	if opts == nil {
		opts = DefaultOptions()
	}
	opts.BackoffMultiplier = 1.0
	opts.Jitter = false

	result, err := DoWithResult(ctx, opts, operation)
	if err != nil {
		var zero T
		return zero, err
	}
	return result.Value, nil
}

// shouldRetry determines if an error should be retried.
func shouldRetry(err error, opts *Options) bool {
	// Check if it's a VisionError with a retryable code
	if ve, ok := err.(*errors.VisionError); ok {
		for _, code := range opts.RetryableErrors {
			if ve.Code == code {
				return true
			}
		}
	}

	// Check specific error types
	if errors.IsRetryable(err) {
		return true
	}

	// Check for rate limit error
	if _, ok := err.(*errors.RateLimitExceededError); ok {
		return true
	}

	// Check for network error
	if _, ok := err.(*errors.NetworkError); ok {
		return true
	}

	return false
}

// calculateDelay calculates the delay before the next retry.
func calculateDelay(attempt int, opts *Options) time.Duration {
	delay := float64(opts.BaseDelay) * math.Pow(opts.BackoffMultiplier, float64(attempt))

	// Apply jitter if enabled (±25% randomness)
	if opts.Jitter {
		jitterFactor := 0.75 + rand.Float64()*0.5 // 0.75 to 1.25
		delay = delay * jitterFactor
	}

	// Ensure delay doesn't exceed maximum
	if time.Duration(delay) > opts.MaxDelay {
		return opts.MaxDelay
	}

	return time.Duration(delay)
}

// Wrap creates a retryable version of a function.
func Wrap[T any](fn func() (T, error), opts *Options) func(context.Context) (T, error) {
	return func(ctx context.Context) (T, error) {
		result, err := DoWithResult(ctx, opts, fn)
		if err != nil {
			var zero T
			return zero, err
		}
		return result.Value, nil
	}
}
