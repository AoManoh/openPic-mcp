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
