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
				Description: "Optional output image size. Defaults to 1024x1024. Mutually exclusive with aspect_ratio: when both are provided, size wins.",
				Enum:        supportedImageSizes,
			},
			"aspect_ratio": {
				Type:        "string",
				Description: "Optional aspect ratio. Mapped onto a supported size: 1:1=1024x1024, 4:3=1536x1024, 3:4=1024x1536, 16:9 and 9:16 use the nearest landscape/portrait preset, auto leaves the upstream default.",
				Enum:        supportedAspectRatios,
			},
			"quality": {
				Type:        "string",
				Description: "Optional output quality supported by the configured provider.",
			},
			"output_format": {
				Type:        "string",
				Description: "Optional output image encoding forwarded to the upstream image API.",
				Enum:        supportedOutputFormats,
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
			"output_dir": {
				Type:        "string",
				Description: "Optional absolute directory where saved images should be written. Overrides OPENPIC_OUTPUT_DIR for this call. Must be an absolute path with no '..' segments; ignored when response_format=b64_json.",
			},
			"filename_prefix": {
				Type:        "string",
				Description: "Optional filename prefix used when saving images. Limited to [A-Za-z0-9._-] and 32 characters; cannot start with '.'. Falls back to OPENPIC_FILENAME_PREFIX or 'edit' when omitted.",
			},
			"overwrite": {
				Type:        "boolean",
				Description: "Optional overwrite policy for saved files. When false (the default) collisions cause a numeric suffix to be appended; when true existing files are replaced.",
			},
		},
		Required: []string{"image", "prompt"},
	},
}

// EditImageHandler returns the MCP tool handler for edit_image. It
// mirrors GenerateImageHandler's variadic options pattern so the same
// deployment-level output-path policy is honoured by both code paths.
func EditImageHandler(imageProvider provider.ImageProvider, opts ...HandlerOption) types.ToolHandler {
	handlerOpts := applyImageHandlerOptions(opts)
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

		// Phase 6b: share the size/aspect_ratio precedence resolver with
		// generate_image so both tools surface identical validation errors.
		size, err := resolveImageSize(stringArg(args, "size"), stringArg(args, "aspect_ratio"))
		if err != nil {
			return errorResult(err.Error()), nil
		}

		outputFormat := stringArg(args, "output_format")
		if err := validateOutputFormat(outputFormat); err != nil {
			return errorResult(err.Error()), nil
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

		overwriteOverride, err := parseBoolArg(args, "overwrite")
		if err != nil {
			return errorResult(err.Error()), nil
		}
		policy, err := resolveOutputPolicy(
			stringArg(args, "output_dir"),
			stringArg(args, "filename_prefix"),
			overwriteOverride,
			"edit",
			handlerOpts.fallbackPolicy(),
		)
		if err != nil {
			return errorResult(err.Error()), nil
		}

		// response_format is owned by the tool layer after Phase 2; provider
		// requests carry only the domain-relevant fields.
		req := &provider.EditImageRequest{
			Image:          imageData,
			ImageMediaType: imageMediaType,
			Prompt:         prompt,
			Size:           size,
			Quality:        stringArg(args, "quality"),
			OutputFormat:   outputFormat,
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

		requested := buildRequestedFromArgs(args, prompt, n, responseFormat, overwriteOverride)
		applied := buildAppliedFromRequest(appliedRequestView{
			Size:         req.Size,
			Quality:      req.Quality,
			OutputFormat: req.OutputFormat,
			N:            req.N,
		}, responseFormat, policy)
		result, err := imageToolResult(
			imageResultsFromProvider(resp.Images),
			resp.Created,
			responseFormat,
			outputFormat,
			policy,
			requested,
			applied,
			usageFromProvider(resp.Usage),
			handlerOpts.MaxInlinePayloadBytes,
		)
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
