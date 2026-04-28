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
				Items:       &types.Property{Type: "string"},
				MinItems:    MinImages,
				MaxItems:    DefaultMaxImages,
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

			// Normalize via the shared helper so each entry receives the same
			// treatment as describe_image and other vision-style tools.
			prepared, err := encoder.PrepareForVision(imgStr)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to prepare image at index %d: %v", i, err)), nil
			}

			images = append(images, provider.ImageInput{
				Data:      prepared.Data,
				MediaType: prepared.MediaType,
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
