package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

var GenerateImageTool = types.Tool{
	Name:        "generate_image",
	Description: "Generate images from a text prompt using an OpenAI-compatible image generation model.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"prompt": {
				Type:        "string",
				Description: "Text prompt describing the image to generate.",
			},
			"size": {
				Type:        "string",
				Description: "Optional output image size supported by the configured provider, such as 1024x1024.",
			},
			"quality": {
				Type:        "string",
				Description: "Optional output quality supported by the configured provider.",
			},
			"response_format": {
				Type:        "string",
				Description: "Optional response format supported by the configured provider.",
				Enum:        []string{"url", "b64_json"},
			},
			"n": {
				Type:        "integer",
				Description: "Optional number of images to generate.",
			},
		},
		Required: []string{"prompt"},
	},
}

func GenerateImageHandler(imageProvider provider.ImageProvider) types.ToolHandler {
	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		prompt, ok := args["prompt"].(string)
		if !ok || prompt == "" {
			return errorResult("prompt parameter is required and must be a string"), nil
		}

		req := &provider.GenerateImageRequest{
			Prompt:         prompt,
			Size:           stringArg(args, "size"),
			Quality:        stringArg(args, "quality"),
			ResponseFormat: stringArg(args, "response_format"),
			N:              intArg(args, "n"),
		}

		resp, err := imageProvider.GenerateImage(ctx, req)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to generate image: %v", err)), nil
		}
		if len(resp.Images) == 0 {
			return errorResult("Failed to generate image: response contained no images"), nil
		}

		resultJSON, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to encode image generation result: %v", err)), nil
		}

		return &types.ToolCallResult{
			Content: []types.ContentItem{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
			IsError: false,
		}, nil
	}
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func intArg(args map[string]any, key string) int {
	switch value := args[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		return 0
	}
}
