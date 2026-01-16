// Package retry provides error message mapping for user-friendly error display.
package retry

import (
	"github.com/AoManoh/openPic-mcp/internal/errors"
)

// Language represents supported languages for error messages.
type Language string

const (
	LangEnglish Language = "en"
	LangChinese Language = "zh"
)

// ErrorMessage contains user-friendly error information.
type ErrorMessage struct {
	Title       string // Short error title
	Description string // Detailed description
	Suggestion  string // Suggested action to resolve
	Retryable   bool   // Whether the error is retryable
}

// ErrorMessageMap maps error codes to user-friendly messages.
type ErrorMessageMap map[string]map[Language]ErrorMessage

// DefaultErrorMessages returns the default error message mappings.
func DefaultErrorMessages() ErrorMessageMap {
	return ErrorMessageMap{
		errors.CodeConfigError: {
			LangEnglish: ErrorMessage{
				Title:       "Configuration Error",
				Description: "The server configuration is invalid or incomplete.",
				Suggestion:  "Please check your environment variables and configuration files.",
				Retryable:   false,
			},
			LangChinese: ErrorMessage{
				Title:       "配置错误",
				Description: "服务器配置无效或不完整。",
				Suggestion:  "请检查您的环境变量和配置文件。",
				Retryable:   false,
			},
		},
		errors.CodeProviderError: {
			LangEnglish: ErrorMessage{
				Title:       "Provider Error",
				Description: "The vision provider encountered an error while processing your request.",
				Suggestion:  "Please try again later or contact support if the issue persists.",
				Retryable:   true,
			},
			LangChinese: ErrorMessage{
				Title:       "提供者错误",
				Description: "视觉服务提供者在处理您的请求时遇到错误。",
				Suggestion:  "请稍后重试，如果问题持续存在，请联系技术支持。",
				Retryable:   true,
			},
		},
		errors.CodeFileUploadError: {
			LangEnglish: ErrorMessage{
				Title:       "File Upload Error",
				Description: "Failed to upload the file to the server.",
				Suggestion:  "Please check your file and network connection, then try again.",
				Retryable:   true,
			},
			LangChinese: ErrorMessage{
				Title:       "文件上传错误",
				Description: "无法将文件上传到服务器。",
				Suggestion:  "请检查您的文件和网络连接，然后重试。",
				Retryable:   true,
			},
		},
		errors.CodeFileNotFound: {
			LangEnglish: ErrorMessage{
				Title:       "File Not Found",
				Description: "The requested file could not be found.",
				Suggestion:  "Please verify the file path or URL is correct.",
				Retryable:   false,
			},
			LangChinese: ErrorMessage{
				Title:       "文件未找到",
				Description: "无法找到请求的文件。",
				Suggestion:  "请确认文件路径或URL是否正确。",
				Retryable:   false,
			},
		},
		errors.CodeUnsupportedFileType: {
			LangEnglish: ErrorMessage{
				Title:       "Unsupported File Type",
				Description: "The file type is not supported for image analysis.",
				Suggestion:  "Please use a supported format: JPEG, PNG, GIF, or WebP.",
				Retryable:   false,
			},
			LangChinese: ErrorMessage{
				Title:       "不支持的文件类型",
				Description: "该文件类型不支持图像分析。",
				Suggestion:  "请使用支持的格式：JPEG、PNG、GIF 或 WebP。",
				Retryable:   false,
			},
		},
		errors.CodeFileSizeExceeded: {
			LangEnglish: ErrorMessage{
				Title:       "File Size Exceeded",
				Description: "The file size exceeds the maximum allowed limit.",
				Suggestion:  "Please compress or resize your image and try again.",
				Retryable:   false,
			},
			LangChinese: ErrorMessage{
				Title:       "文件大小超限",
				Description: "文件大小超过了允许的最大限制。",
				Suggestion:  "请压缩或调整图像大小后重试。",
				Retryable:   false,
			},
		},
		errors.CodeRateLimitExceeded: {
			LangEnglish: ErrorMessage{
				Title:       "Rate Limit Exceeded",
				Description: "Too many requests have been made in a short period.",
				Suggestion:  "Please wait a moment before trying again.",
				Retryable:   true,
			},
			LangChinese: ErrorMessage{
				Title:       "请求频率超限",
				Description: "短时间内发送了过多请求。",
				Suggestion:  "请稍等片刻后再试。",
				Retryable:   true,
			},
		},
		errors.CodeAuthenticationError: {
			LangEnglish: ErrorMessage{
				Title:       "Authentication Failed",
				Description: "The API key or credentials are invalid.",
				Suggestion:  "Please check your API key and ensure it is correctly configured.",
				Retryable:   false,
			},
			LangChinese: ErrorMessage{
				Title:       "认证失败",
				Description: "API密钥或凭证无效。",
				Suggestion:  "请检查您的API密钥并确保配置正确。",
				Retryable:   false,
			},
		},
	}
}

// Additional error messages (continued)
func additionalErrorMessages() ErrorMessageMap {
	return ErrorMessageMap{
		errors.CodeAuthorizationError: {
			LangEnglish: ErrorMessage{
				Title:       "Authorization Failed",
				Description: "You do not have permission to perform this action.",
				Suggestion:  "Please check your account permissions or contact your administrator.",
				Retryable:   false,
			},
			LangChinese: ErrorMessage{
				Title:       "授权失败",
				Description: "您没有执行此操作的权限。",
				Suggestion:  "请检查您的账户权限或联系管理员。",
				Retryable:   false,
			},
		},
		errors.CodeNetworkError: {
			LangEnglish: ErrorMessage{
				Title:       "Network Error",
				Description: "A network error occurred while connecting to the server.",
				Suggestion:  "Please check your internet connection and try again.",
				Retryable:   true,
			},
			LangChinese: ErrorMessage{
				Title:       "网络错误",
				Description: "连接服务器时发生网络错误。",
				Suggestion:  "请检查您的网络连接后重试。",
				Retryable:   true,
			},
		},
		errors.CodeValidationError: {
			LangEnglish: ErrorMessage{
				Title:       "Validation Error",
				Description: "The input data is invalid or malformed.",
				Suggestion:  "Please check your input and ensure all required fields are provided.",
				Retryable:   false,
			},
			LangChinese: ErrorMessage{
				Title:       "验证错误",
				Description: "输入数据无效或格式错误。",
				Suggestion:  "请检查您的输入并确保提供了所有必填字段。",
				Retryable:   false,
			},
		},
		errors.CodeAnalysisError: {
			LangEnglish: ErrorMessage{
				Title:       "Analysis Error",
				Description: "Failed to analyze the image.",
				Suggestion:  "Please ensure the image is clear and try again with a different image if needed.",
				Retryable:   true,
			},
			LangChinese: ErrorMessage{
				Title:       "分析错误",
				Description: "无法分析图像。",
				Suggestion:  "请确保图像清晰，如有需要可尝试使用其他图像。",
				Retryable:   true,
			},
		},
	}
}

// MessageResolver resolves error messages for display.
type MessageResolver struct {
	messages ErrorMessageMap
	lang     Language
}

// NewMessageResolver creates a new message resolver with the specified language.
func NewMessageResolver(lang Language) *MessageResolver {
	messages := DefaultErrorMessages()
	// Merge additional messages
	for code, langMap := range additionalErrorMessages() {
		messages[code] = langMap
	}
	return &MessageResolver{
		messages: messages,
		lang:     lang,
	}
}

// Resolve returns the user-friendly message for an error.
func (r *MessageResolver) Resolve(err error) *ErrorMessage {
	code := r.getErrorCode(err)
	if code == "" {
		return r.defaultMessage()
	}

	langMap, ok := r.messages[code]
	if !ok {
		return r.defaultMessage()
	}

	msg, ok := langMap[r.lang]
	if !ok {
		// Fallback to English
		msg, ok = langMap[LangEnglish]
		if !ok {
			return r.defaultMessage()
		}
	}

	return &msg
}

// getErrorCode extracts the error code from an error.
func (r *MessageResolver) getErrorCode(err error) string {
	if ve, ok := err.(*errors.VisionError); ok {
		return ve.Code
	}
	// Check specific error types
	switch e := err.(type) {
	case *errors.ConfigurationError:
		return e.Code
	case *errors.ProviderError:
		return e.Code
	case *errors.FileNotFoundError:
		return e.Code
	case *errors.RateLimitExceededError:
		return e.Code
	case *errors.NetworkError:
		return e.Code
	case *errors.ValidationError:
		return e.Code
	case *errors.AnalysisError:
		return e.Code
	case *errors.AuthenticationError:
		return e.Code
	case *errors.AuthorizationError:
		return e.Code
	case *errors.UnsupportedFileTypeError:
		return e.Code
	case *errors.FileSizeExceededError:
		return e.Code
	case *errors.FileUploadError:
		return e.Code
	}
	return ""
}

// defaultMessage returns a generic error message.
func (r *MessageResolver) defaultMessage() *ErrorMessage {
	if r.lang == LangChinese {
		return &ErrorMessage{
			Title:       "未知错误",
			Description: "发生了意外错误。",
			Suggestion:  "请稍后重试，如果问题持续存在，请联系技术支持。",
			Retryable:   false,
		}
	}
	return &ErrorMessage{
		Title:       "Unknown Error",
		Description: "An unexpected error occurred.",
		Suggestion:  "Please try again later or contact support if the issue persists.",
		Retryable:   false,
	}
}

// FormatError formats an error with user-friendly message.
func (r *MessageResolver) FormatError(err error) string {
	msg := r.Resolve(err)
	return msg.Title + ": " + msg.Description + "\n" + msg.Suggestion
}
