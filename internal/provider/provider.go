// Package provider provides Vision API provider implementations.
package provider

import (
	"context"
)

// VisionProvider defines the interface for Vision API providers.
type VisionProvider interface {
	// Name returns the provider name.
	Name() string

	// AnalyzeImage analyzes an image and returns a description.
	AnalyzeImage(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error)

	// CompareImages compares multiple images and returns a comparison result.
	CompareImages(ctx context.Context, req *CompareRequest) (*CompareResponse, error)
}

type ImageProvider interface {
	Name() string
	GenerateImage(ctx context.Context, req *GenerateImageRequest) (*GenerateImageResponse, error)
	EditImage(ctx context.Context, req *EditImageRequest) (*EditImageResponse, error)
}

// AnalyzeRequest represents a request to analyze an image.
type AnalyzeRequest struct {
	// Image is the image data (base64 encoded) or URL.
	Image string

	// ImageMediaType is the MIME type of the image (e.g., "image/jpeg").
	ImageMediaType string

	// Prompt is an optional prompt to guide the analysis.
	Prompt string

	// DetailLevel controls the detail level of the response.
	// Possible values: "brief", "normal", "detailed"
	DetailLevel string
}

// AnalyzeResponse represents the response from image analysis.
type AnalyzeResponse struct {
	// Description is the text description of the image.
	Description string

	// Usage contains token usage information.
	Usage *Usage
}

// Usage contains token usage information.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// CompareRequest represents a request to compare multiple images.
type CompareRequest struct {
	// Images is a list of image data (base64 encoded) or URLs.
	Images []ImageInput

	// Prompt is the comparison prompt.
	Prompt string

	// DetailLevel controls the detail level of the response.
	DetailLevel string
}

// ImageInput represents a single image input.
type ImageInput struct {
	// Data is the image data (base64 encoded) or URL.
	Data string

	// MediaType is the MIME type of the image (e.g., "image/jpeg").
	MediaType string
}

// CompareResponse represents the response from image comparison.
type CompareResponse struct {
	// Comparison is the text comparison result.
	Comparison string

	// Usage contains token usage information.
	Usage *Usage
}

// GenerateImageRequest is the domain-level request used by tool handlers.
//
// The MCP-facing response_format parameter is intentionally NOT part of this
// struct: it controls how the result is delivered back to the MCP client and
// is handled at the tool layer. Whether the upstream API receives any
// response_format hint is decided inside the provider implementation, so the
// domain stays decoupled from the upstream wire format. See
// docs/refactor/2026-04-28-decoupling-plan.md (Phase 2).
//
// OutputFormat (Phase 6a) selects the encoding of the generated image bytes
// (png / jpeg / webp). It is forwarded to the upstream API when set so the
// model can produce the requested encoding directly; an empty string lets
// the upstream pick its default.
type GenerateImageRequest struct {
	Prompt       string `json:"prompt"`
	Size         string `json:"size,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
	N            int    `json:"n,omitempty"`
}

type GenerateImageResponse struct {
	Images  []GeneratedImage `json:"images"`
	Created int64            `json:"created"`
}

type GeneratedImage struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	FilePath      string `json:"file_path,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// EditImageRequest is the domain-level request used by tool handlers.
//
// As with GenerateImageRequest, the MCP-facing response_format is excluded:
// it is a delivery concern owned by the tool layer. The provider decides what
// (if anything) to forward to the upstream API so the domain stays portable
// across providers. OutputFormat (Phase 6a) follows the same conventions as
// in GenerateImageRequest.
type EditImageRequest struct {
	Image          []byte
	ImageMediaType string
	Mask           []byte
	MaskMediaType  string
	Prompt         string
	Size           string
	Quality        string
	OutputFormat   string
	N              int
}

type EditImageResponse struct {
	Images  []GeneratedImage `json:"images"`
	Created int64            `json:"created"`
}

// DefaultPrompts contains default prompts for different analysis types.
var DefaultPrompts = map[string]string{
	"describe": "Please describe this image in detail. Include information about the main subjects, colors, composition, and any text visible in the image.",
	"brief":    "Briefly describe what you see in this image.",
	"detailed": "Provide a comprehensive and detailed description of this image, including all visible elements, their relationships, colors, textures, and any text or symbols present.",
	"compare":  "Please compare these images in detail. Identify similarities and differences in content, composition, colors, subjects, and any other notable aspects. Provide a structured comparison.",
}

// GetPrompt returns the appropriate prompt based on detail level.
func GetPrompt(detailLevel string, customPrompt string) string {
	if customPrompt != "" {
		return customPrompt
	}

	switch detailLevel {
	case "brief":
		return DefaultPrompts["brief"]
	case "detailed":
		return DefaultPrompts["detailed"]
	default:
		return DefaultPrompts["describe"]
	}
}

// GetComparePrompt returns the appropriate prompt for image comparison.
func GetComparePrompt(customPrompt string) string {
	if customPrompt != "" {
		return customPrompt
	}
	return DefaultPrompts["compare"]
}
