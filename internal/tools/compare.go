// Package tools provides concrete tool implementations for the Vision MCP Server.
package tools

import (
	"context"
	"fmt"

	"github.com/AoManoh/openPic-mcp/internal/image"
	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// Default and maximum number of images for comparison.
const (
	DefaultMaxImages = 4
	MinImages        = 2
)

// CompareImagesTool is the tool definition for compare_images.
var CompareImagesTool = types.Tool{
	Name:        "compare_images",
	Description: "Compare multiple images using AI vision models. Analyzes similarities and differences between 2-4 images. Supports base64-encoded image data, URLs, and local file paths.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"images": {
				Type:        "array",
				Description: "Array of images to compare. Each image can be a base64-encoded image data, a URL pointing to an image, or a local file path. Minimum 2 images, maximum 4 images.",
			},
			"prompt": {
				Type:        "string",
				Description: "Optional custom prompt to guide the comparison. If not provided, a default comparison prompt will be used.",
			},
			"detail_level": {
				Type:        "string",
				Description: "Level of detail for the comparison.",
				Enum:        []string{"brief", "normal", "detailed"},
				Default:     "normal",
			},
		},
		Required: []string{"images"},
	},
}

// CompareImagesHandler creates a handler for the compare_images tool.
func CompareImagesHandler(visionProvider provider.VisionProvider) types.ToolHandler {
	encoder := image.NewEncoder()

	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		// Extract and validate images argument
		imagesRaw, ok := args["images"]
		if !ok {
			return errorResult("images parameter is required"), nil
		}

		// Convert to string slice
		imagesSlice, ok := imagesRaw.([]interface{})
		if !ok {
			return errorResult("images must be an array of strings"), nil
		}

		// Validate image count
		if len(imagesSlice) < MinImages {
			return errorResult(fmt.Sprintf("at least %d images are required for comparison", MinImages)), nil
		}
		if len(imagesSlice) > DefaultMaxImages {
			return errorResult(fmt.Sprintf("maximum %d images allowed for comparison", DefaultMaxImages)), nil
		}

		// Convert to ImageInput slice with preprocessing
		images := make([]provider.ImageInput, 0, len(imagesSlice))
		for i, img := range imagesSlice {
			imgStr, ok := img.(string)
			if !ok || imgStr == "" {
				return errorResult(fmt.Sprintf("image at index %d must be a non-empty string", i)), nil
			}

			// Preprocess image input (handles local files, URLs, base64, data URIs)
			var imageData string
			var mediaType string

			if image.IsLocalFilePath(imgStr) {
				// Local file path - read and convert to base64
				data, mimeType, err := encoder.DecodeInput(imgStr)
				if err != nil {
					return errorResult(fmt.Sprintf("Failed to read local file at index %d: %v", i, err)), nil
				}
				imageData = image.EncodeToBase64(data)
				mediaType = mimeType
			} else if image.IsDataURI(imgStr) {
				// Data URI - extract base64 and media type
				data, mimeType, err := encoder.DecodeInput(imgStr)
				if err != nil {
					return errorResult(fmt.Sprintf("Failed to decode data URI at index %d: %v", i, err)), nil
				}
				imageData = image.EncodeToBase64(data)
				mediaType = mimeType
			} else if image.IsURL(imgStr) {
				// URL - pass directly to provider
				imageData = imgStr
				mediaType = ""
			} else {
				// Assume raw base64 - validate and detect format
				data, mimeType, err := encoder.DecodeInput(imgStr)
				if err != nil {
					// If decoding fails, pass as-is
					imageData = imgStr
					mediaType = ""
				} else {
					imageData = image.EncodeToBase64(data)
					mediaType = mimeType
				}
			}

			images = append(images, provider.ImageInput{
				Data:      imageData,
				MediaType: mediaType,
			})
		}

		// Optional parameters
		prompt, _ := args["prompt"].(string)
		detailLevel, _ := args["detail_level"].(string)
		if detailLevel == "" {
			detailLevel = "normal"
		}

		// Build compare request
		req := &provider.CompareRequest{
			Images:      images,
			Prompt:      prompt,
			DetailLevel: detailLevel,
		}

		// Call the vision provider
		resp, err := visionProvider.CompareImages(ctx, req)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to compare images: %v", err)), nil
		}

		// Return success result
		return &types.ToolCallResult{
			Content: []types.ContentItem{
				{
					Type: "text",
					Text: resp.Comparison,
				},
			},
			IsError: false,
		}, nil
	}
}
