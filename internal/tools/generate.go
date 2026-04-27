package tools

import (
	"context"
	"fmt"
	"regexp"

	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

const (
	defaultImageResponseFormat = "file_path"
	defaultImageSize           = "1024x1024"
	maxImageResults            = 1
)

var imageSizePattern = regexp.MustCompile(`^[1-9][0-9]*x[1-9][0-9]*$`)

var GenerateImageTool = types.Tool{
	Name:        "generate_image",
	Description: "Generate one image from a text prompt using an OpenAI-compatible image generation model. Defaults to size 1024x1024 and response_format file_path to avoid large inline base64 responses.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"prompt": {
				Type:        "string",
				Description: "Text prompt describing the image to generate.",
			},
			"size": {
				Type:        "string",
				Description: "Optional output image size in WIDTHxHEIGHT format. Defaults to 1024x1024.",
			},
			"quality": {
				Type:        "string",
				Description: "Optional output quality supported by the configured provider.",
			},
			"response_format": {
				Type:        "string",
				Description: "Optional response format. Defaults to file_path. Use b64_json only when inline base64 is explicitly required.",
				Enum:        []string{"file_path", "url", "b64_json"},
				Default:     defaultImageResponseFormat,
			},
			"n": {
				Type:        "integer",
				Description: "Optional number of images to generate. Currently only n=1 is supported.",
				Default:     "1",
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

		size := stringArg(args, "size")
		if size == "" {
			size = defaultImageSize
		}
		if !imageSizePattern.MatchString(size) {
			return errorResult(fmt.Sprintf("size must match WIDTHxHEIGHT using positive integers, got %q", size)), nil
		}

		responseFormat := stringArg(args, "response_format")
		if responseFormat == "" {
			responseFormat = defaultImageResponseFormat
		}

		n := 1
		if _, ok := args["n"]; ok {
			n = intArg(args, "n")
		}
		if n != maxImageResults {
			return errorResult(fmt.Sprintf("n=%d is not supported: this tool currently supports only n=1", n)), nil
		}

		req := &provider.GenerateImageRequest{
			Prompt:         prompt,
			Size:           size,
			Quality:        stringArg(args, "quality"),
			ResponseFormat: providerResponseFormat(responseFormat),
			N:              n,
		}

		resp, err := imageProvider.GenerateImage(ctx, req)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to generate image: %v", err)), nil
		}
		if len(resp.Images) == 0 {
			return errorResult("Failed to generate image: response contained no images"), nil
		}

		result, err := imageToolResult(resp.Images, resp.Created, responseFormat, "generate")
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to encode image generation result: %v", err)), nil
		}

		return result, nil
	}
}

func providerResponseFormat(responseFormat string) string {
	if responseFormat == defaultImageResponseFormat {
		return "b64_json"
	}
	return responseFormat
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
