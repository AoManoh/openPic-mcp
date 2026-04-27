package tools

import (
	"context"
	"fmt"

	imageutil "github.com/AoManoh/openPic-mcp/internal/image"
	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

var EditImageTool = types.Tool{
	Name:        "edit_image",
	Description: "Edit an existing image using a text prompt and optional mask with an OpenAI-compatible image editing model. Defaults to size 1024x1024 and response_format file_path to avoid large inline base64 responses.",
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
				Description: "Optional output image size. Defaults to 1024x1024.",
				Enum:        supportedImageSizes,
			},
			"quality": {
				Type:        "string",
				Description: "Optional output quality supported by the configured provider.",
			},
			"response_format": {
				Type:        "string",
				Description: "Optional response format. Defaults to file_path. Use b64_json only when inline base64 is explicitly required. If url returns a data URI, the result is saved as file_path.",
				Enum:        []string{"file_path", "url", "b64_json"},
				Default:     defaultImageResponseFormat,
			},
			"n": {
				Type:        "integer",
				Description: "Optional number of edited images to return. Currently only n=1 is supported.",
				Default:     "1",
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

		size := stringArg(args, "size")
		if size == "" {
			size = defaultImageSize
		}
		if !containsString(supportedImageSizes, size) {
			return errorResult(fmt.Sprintf("unsupported size %q: expected one of %v", size, supportedImageSizes)), nil
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

		req := &provider.EditImageRequest{
			Image:          imageData,
			ImageMediaType: imageMediaType,
			Prompt:         prompt,
			Size:           size,
			Quality:        stringArg(args, "quality"),
			ResponseFormat: providerResponseFormat(responseFormat),
			N:              n,
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

		result, err := imageToolResult(resp.Images, resp.Created, responseFormat, "edit")
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to encode image editing result: %v", err)), nil
		}

		return result, nil
	}
}

func decodeImageEditInput(input string) ([]byte, string, error) {
	encoder := imageutil.NewEncoder()
	return encoder.DecodeInput(input)
}
