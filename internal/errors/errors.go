// Package errors provides custom error types for the Vision MCP Server.
package errors

import (
	"fmt"
)

// Error codes for the Vision MCP Server.
const (
	CodeConfigError         = "CONFIG_ERROR"
	CodeProviderError       = "PROVIDER_ERROR"
	CodeFileUploadError     = "FILE_UPLOAD_ERROR"
	CodeFileNotFound        = "FILE_NOT_FOUND"
	CodeUnsupportedFileType = "UNSUPPORTED_FILE_TYPE"
	CodeFileSizeExceeded    = "FILE_SIZE_EXCEEDED"
	CodeRateLimitExceeded   = "RATE_LIMIT_EXCEEDED"
	CodeAuthenticationError = "AUTHENTICATION_ERROR"
	CodeAuthorizationError  = "AUTHORIZATION_ERROR"
	CodeNetworkError        = "NETWORK_ERROR"
	CodeValidationError     = "VALIDATION_ERROR"
	CodeAnalysisError       = "ANALYSIS_ERROR"
)

// VisionError is the base error type for all Vision MCP Server errors.
type VisionError struct {
	Message    string // Human-readable error message
	Code       string // Error code for programmatic handling
	Provider   string // Provider name (e.g., "openai", "gemini")
	StatusCode int    // HTTP status code (if applicable)
	Err        error  // Original error (for error wrapping)
}

// Error implements the error interface.
func (e *VisionError) Error() string {
	if e.Provider != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Provider, e.Message)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *VisionError) Unwrap() error {
	return e.Err
}

// NewVisionError creates a new VisionError.
func NewVisionError(message, code string, opts ...ErrorOption) *VisionError {
	e := &VisionError{
		Message: message,
		Code:    code,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ErrorOption is a function that configures a VisionError.
type ErrorOption func(*VisionError)

// WithProvider sets the provider name.
func WithProvider(provider string) ErrorOption {
	return func(e *VisionError) {
		e.Provider = provider
	}
}

// WithStatusCode sets the HTTP status code.
func WithStatusCode(code int) ErrorOption {
	return func(e *VisionError) {
		e.StatusCode = code
	}
}

// WithError wraps an underlying error.
func WithError(err error) ErrorOption {
	return func(e *VisionError) {
		e.Err = err
	}
}

// ConfigurationError represents a configuration error.
type ConfigurationError struct {
	VisionError
	Variable string // The configuration variable that caused the error
}

// NewConfigurationError creates a new ConfigurationError.
func NewConfigurationError(message string, variable string) *ConfigurationError {
	return &ConfigurationError{
		VisionError: VisionError{
			Message:    message,
			Code:       CodeConfigError,
			StatusCode: 400,
		},
		Variable: variable,
	}
}

// ProviderError represents an error from a vision provider.
type ProviderError struct {
	VisionError
}

// NewProviderError creates a new ProviderError.
func NewProviderError(message, provider string, opts ...ErrorOption) *ProviderError {
	e := &ProviderError{
		VisionError: VisionError{
			Message:  message,
			Code:     CodeProviderError,
			Provider: provider,
		},
	}
	for _, opt := range opts {
		opt(&e.VisionError)
	}
	return e
}

// FileUploadError represents a file upload error.
type FileUploadError struct {
	VisionError
}

// NewFileUploadError creates a new FileUploadError.
func NewFileUploadError(message string, opts ...ErrorOption) *FileUploadError {
	e := &FileUploadError{
		VisionError: VisionError{
			Message: message,
			Code:    CodeFileUploadError,
		},
	}
	for _, opt := range opts {
		opt(&e.VisionError)
	}
	return e
}

// FileNotFoundError represents a file not found error.
type FileNotFoundError struct {
	VisionError
	FileID string
}

// NewFileNotFoundError creates a new FileNotFoundError.
func NewFileNotFoundError(fileID string, provider string) *FileNotFoundError {
	return &FileNotFoundError{
		VisionError: VisionError{
			Message:    fmt.Sprintf("file not found: %s", fileID),
			Code:       CodeFileNotFound,
			Provider:   provider,
			StatusCode: 404,
		},
		FileID: fileID,
	}
}

// UnsupportedFileTypeError represents an unsupported file type error.
type UnsupportedFileTypeError struct {
	VisionError
	MIMEType       string
	SupportedTypes []string
}

// NewUnsupportedFileTypeError creates a new UnsupportedFileTypeError.
func NewUnsupportedFileTypeError(mimeType string, supportedTypes []string) *UnsupportedFileTypeError {
	var message string
	if len(supportedTypes) > 0 {
		message = fmt.Sprintf("unsupported file type: %s. Supported types: %v", mimeType, supportedTypes)
	} else {
		message = fmt.Sprintf("unsupported file type: %s", mimeType)
	}
	return &UnsupportedFileTypeError{
		VisionError: VisionError{
			Message:    message,
			Code:       CodeUnsupportedFileType,
			StatusCode: 400,
		},
		MIMEType:       mimeType,
		SupportedTypes: supportedTypes,
	}
}

// FileSizeExceededError represents a file size exceeded error.
type FileSizeExceededError struct {
	VisionError
	FileSize int64
	MaxSize  int64
}

// NewFileSizeExceededError creates a new FileSizeExceededError.
func NewFileSizeExceededError(fileSize, maxSize int64) *FileSizeExceededError {
	return &FileSizeExceededError{
		VisionError: VisionError{
			Message:    fmt.Sprintf("file size %d bytes exceeds maximum allowed size %d bytes", fileSize, maxSize),
			Code:       CodeFileSizeExceeded,
			StatusCode: 400,
		},
		FileSize: fileSize,
		MaxSize:  maxSize,
	}
}

// RateLimitExceededError represents a rate limit exceeded error.
type RateLimitExceededError struct {
	VisionError
	RetryAfter int // Seconds to wait before retrying
}

// NewRateLimitExceededError creates a new RateLimitExceededError.
func NewRateLimitExceededError(message, provider string, retryAfter int) *RateLimitExceededError {
	return &RateLimitExceededError{
		VisionError: VisionError{
			Message:    message,
			Code:       CodeRateLimitExceeded,
			Provider:   provider,
			StatusCode: 429,
		},
		RetryAfter: retryAfter,
	}
}

// AuthenticationError represents an authentication error.
type AuthenticationError struct {
	VisionError
}

// NewAuthenticationError creates a new AuthenticationError.
func NewAuthenticationError(message, provider string) *AuthenticationError {
	return &AuthenticationError{
		VisionError: VisionError{
			Message:    message,
			Code:       CodeAuthenticationError,
			Provider:   provider,
			StatusCode: 401,
		},
	}
}

// AuthorizationError represents an authorization error.
type AuthorizationError struct {
	VisionError
}

// NewAuthorizationError creates a new AuthorizationError.
func NewAuthorizationError(message, provider string) *AuthorizationError {
	return &AuthorizationError{
		VisionError: VisionError{
			Message:    message,
			Code:       CodeAuthorizationError,
			Provider:   provider,
			StatusCode: 403,
		},
	}
}

// NetworkError represents a network error.
type NetworkError struct {
	VisionError
}

// NewNetworkError creates a new NetworkError.
func NewNetworkError(message string, err error) *NetworkError {
	return &NetworkError{
		VisionError: VisionError{
			Message: message,
			Code:    CodeNetworkError,
			Err:     err,
		},
	}
}

// ValidationError represents a validation error.
type ValidationError struct {
	VisionError
	Field string // The field that failed validation
}

// NewValidationError creates a new ValidationError.
func NewValidationError(message, field string) *ValidationError {
	return &ValidationError{
		VisionError: VisionError{
			Message:    message,
			Code:       CodeValidationError,
			StatusCode: 400,
		},
		Field: field,
	}
}

// AnalysisError represents an image analysis error.
type AnalysisError struct {
	VisionError
}

// NewAnalysisError creates a new AnalysisError.
func NewAnalysisError(message, provider string, err error) *AnalysisError {
	return &AnalysisError{
		VisionError: VisionError{
			Message:  message,
			Code:     CodeAnalysisError,
			Provider: provider,
			Err:      err,
		},
	}
}

// IsRetryable returns true if the error is retryable.
func IsRetryable(err error) bool {
	if ve, ok := err.(*VisionError); ok {
		switch ve.Code {
		case CodeRateLimitExceeded, CodeNetworkError:
			return true
		}
	}
	if _, ok := err.(*RateLimitExceededError); ok {
		return true
	}
	if _, ok := err.(*NetworkError); ok {
		return true
	}
	return false
}

// GetStatusCode returns the HTTP status code for an error.
func GetStatusCode(err error) int {
	if ve, ok := err.(*VisionError); ok {
		if ve.StatusCode != 0 {
			return ve.StatusCode
		}
	}
	// Check specific error types
	switch e := err.(type) {
	case *ConfigurationError:
		return e.StatusCode
	case *ProviderError:
		return e.StatusCode
	case *FileUploadError:
		return e.StatusCode
	case *FileNotFoundError:
		return e.StatusCode
	case *UnsupportedFileTypeError:
		return e.StatusCode
	case *FileSizeExceededError:
		return e.StatusCode
	case *RateLimitExceededError:
		return e.StatusCode
	case *AuthenticationError:
		return e.StatusCode
	case *AuthorizationError:
		return e.StatusCode
	case *ValidationError:
		return e.StatusCode
	}
	return 500 // Default to internal server error
}
