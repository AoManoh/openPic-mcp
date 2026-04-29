package openai

// ChatCompletionRequest represents an OpenAI chat completion request.
type ChatCompletionRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

// ImageGenerationRequest is the wire payload for /v1/images/generations.
//
// response_format is intentionally omitted: GPT image models always return
// b64_json and the parameter is rejected on some compatible proxies. The
// tool layer is the sole source of truth for how results are delivered to
// the MCP client. See docs/refactor/2026-04-28-decoupling-plan.md (Phase 2).
//
// OutputFormat is included as an optional field (Phase 6a) so callers can
// request a specific encoding (png / jpeg / webp). When empty the upstream
// chooses its default and the field is omitted from the marshalled JSON.
type ImageGenerationRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	N            int    `json:"n,omitempty"`
	Size         string `json:"size,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string        `json:"role"`
	Content []ContentPart `json:"content"`
}

// ContentPart represents a part of the message content.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL represents an image URL in the content.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// ChatCompletionResponse represents an OpenAI chat completion response.
type ChatCompletionResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created int64     `json:"created"`
	Model   string    `json:"model"`
	Choices []Choice  `json:"choices"`
	Usage   UsageInfo `json:"usage"`
}

// Choice represents a completion choice.
type Choice struct {
	Index        int             `json:"index"`
	Message      ResponseMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

// ResponseMessage represents the assistant's response message.
type ResponseMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// UsageInfo represents token usage information.
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ImageGenerationResponse struct {
	Created int64                `json:"created"`
	Data    []GeneratedImageData `json:"data"`
	Usage   *ImageUsageInfo      `json:"usage,omitempty"`
}

// ImageUsageInfo mirrors the optional usage object documented on
// /v1/images/generations and /v1/images/edits for gpt-image-1 (and a
// growing list of OpenAI-compatible deployments). Pointer-typed fields
// let the marshaller omit figures the upstream did not actually return,
// which the tool layer can then forward to clients faithfully without
// fabricating zero counters.
type ImageUsageInfo struct {
	InputTokens  *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
	TotalTokens  *int64 `json:"total_tokens,omitempty"`
}

type GeneratedImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// APIError represents the error details.
type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}
