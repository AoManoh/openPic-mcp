// Package tools provides concrete tool implementations for the Vision MCP Server.
package tools

import (
	"context"
	"fmt"

	"github.com/AoManoh/openPic-mcp/internal/image"
	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// DescribeImageTool is the tool definition for describe_image.
var DescribeImageTool = types.Tool{
	Name:        "describe_image",
	Description: "Analyze and describe the content of an image. Returns a detailed text description of what is visible in the image.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"image": {
				Type:        "string",
				Description: "The image to analyze. Can be a base64-encoded image data or a URL pointing to an image.",
			},
			"prompt": {
				Type:        "string",
				Description: "Optional custom prompt to guide the image analysis. If not provided, a default prompt will be used.",
			},
			"detail_level": {
				Type:        "string",
				Description: "Level of detail for the description.",
				Enum:        []string{"brief", "normal", "detailed"},
				Default:     "normal",
			},
		},
		Required: []string{"image"},
	},
}

// DescribeImageHandler creates a handler for the describe_image tool.
func DescribeImageHandler(visionProvider provider.VisionProvider) types.ToolHandler {
	encoder := image.NewEncoder()

	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		// Extract and validate arguments
		imageInput, ok := args["image"].(string)
		if !ok || imageInput == "" {
			return errorResult("image parameter is required and must be a string"), nil
		}

		// Optional parameters
		prompt, _ := args["prompt"].(string)
		detailLevel, _ := args["detail_level"].(string)
		if detailLevel == "" {
			detailLevel = "normal"
		}

		// Normalize the image input (local file / data URI / URL / raw base64)
		// using the shared image-preparation helper so the same edge-case
		// handling is applied across every vision-style tool.
		prepared, err := encoder.PrepareForVision(imageInput)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to prepare image input: %v", err)), nil
		}

		// Build analyze request
		req := &provider.AnalyzeRequest{
			Image:          prepared.Data,
			ImageMediaType: prepared.MediaType,
			Prompt:         prompt,
			DetailLevel:    detailLevel,
		}

		// Call the vision provider
		resp, err := visionProvider.AnalyzeImage(ctx, req)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to analyze image: %v", err)), nil
		}

		// Return success result
		return &types.ToolCallResult{
			Content: []types.ContentItem{
				{
					Type: "text",
					Text: resp.Description,
				},
			},
			IsError: false,
		}, nil
	}
}

// errorResult creates an error result for tool execution.
func errorResult(message string) *types.ToolCallResult {
	return &types.ToolCallResult{
		Content: []types.ContentItem{
			{
				Type: "text",
				Text: message,
			},
		},
		IsError: true,
	}
}
