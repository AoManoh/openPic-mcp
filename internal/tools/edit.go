package tools

import (
	"context"
	"encoding/json"
	"fmt"

	imageutil "github.com/AoManoh/openPic-mcp/internal/image"
	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

var EditImageTool = types.Tool{
	Name:        "edit_image",
	Description: "Edit an existing image using a text prompt and optional mask with an OpenAI-compatible image editing model.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"image": {
				Type:        "string",
				Description: "Image to edit. Supports local file path, HTTP/HTTPS URL, data URI, or raw base64.",
			},
			"prompt": {
				Type:        "string",
				Description: "Text prompt describing the desired edit.",
			},
			"mask": {
				Type:        "string",
				Description: "Optional mask image. Supports local file path, HTTP/HTTPS URL, data URI, or raw base64.",
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
				Description: "Optional number of edited images to return.",
			},
		},
		Required: []string{"image", "prompt"},
	},
}

func EditImageHandler(imageProvider provider.ImageProvider) types.ToolHandler {
	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		imageInput, ok := args["image"].(string)
		if !ok || imageInput == "" {
			return errorResult("image parameter is required and must be a string"), nil
		}
		prompt, ok := args["prompt"].(string)
		if !ok || prompt == "" {
			return errorResult("prompt parameter is required and must be a string"), nil
		}

		imageData, imageMediaType, err := decodeImageEditInput(imageInput)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to decode image: %v", err)), nil
		}

		req := &provider.EditImageRequest{
			Image:          imageData,
			ImageMediaType: imageMediaType,
			Prompt:         prompt,
			Size:           stringArg(args, "size"),
			Quality:        stringArg(args, "quality"),
			ResponseFormat: stringArg(args, "response_format"),
			N:              intArg(args, "n"),
		}

		if maskInput := stringArg(args, "mask"); maskInput != "" {
			maskData, maskMediaType, err := decodeImageEditInput(maskInput)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to decode mask: %v", err)), nil
			}
			req.Mask = maskData
			req.MaskMediaType = maskMediaType
		}

		resp, err := imageProvider.EditImage(ctx, req)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to edit image: %v", err)), nil
		}
		if len(resp.Images) == 0 {
			return errorResult("Failed to edit image: response contained no images"), nil
		}

		resultJSON, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to encode image editing result: %v", err)), nil
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

func decodeImageEditInput(input string) ([]byte, string, error) {
	encoder := imageutil.NewEncoder()
	return encoder.DecodeInput(input)
}
