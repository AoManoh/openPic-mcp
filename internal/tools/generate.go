package tools

import (
	"context"
	"fmt"

	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

const (
	defaultImageResponseFormat = "file_path"
	defaultImageSize           = "1024x1024"
	maxImageResults            = 1
)

var supportedImageSizes = []string{"1024x1024", "1024x1536", "1536x1024", "2048x2048"}

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
				Description: "Optional number of images to generate. Currently only n=1 is supported.",
				Default:     "1",
			},
			"output_dir": {
				Type:        "string",
				Description: "Optional absolute directory where saved images should be written. Overrides OPENPIC_OUTPUT_DIR for this call. Must be an absolute path with no '..' segments; ignored when response_format=b64_json.",
			},
			"filename_prefix": {
				Type:        "string",
				Description: "Optional filename prefix used when saving images. Limited to [A-Za-z0-9._-] and 32 characters; cannot start with '.'. Falls back to OPENPIC_FILENAME_PREFIX or 'generate' when omitted.",
			},
			"overwrite": {
				Type:        "boolean",
				Description: "Optional overwrite policy for saved files. When false (the default) collisions cause a numeric suffix to be appended; when true existing files are replaced.",
			},
		},
		Required: []string{"prompt"},
	},
}

// GenerateImageHandler returns the MCP tool handler for generate_image.
// The variadic HandlerOption arguments let main.go thread the
// deployment-level output-path policy (derived from config.Config) into
// the closure without changing existing call sites that don't care; the
// handler defaults to the legacy temp directory when no options are
// supplied.
func GenerateImageHandler(imageProvider provider.ImageProvider, opts ...HandlerOption) types.ToolHandler {
	handlerOpts := applyImageHandlerOptions(opts)
	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		prompt, ok := args["prompt"].(string)
		if !ok || prompt == "" {
			return errorResult("prompt parameter is required and must be a string"), nil
		}

		// Phase 6b: size and aspect_ratio share the same downstream slot.
		// resolveImageSize centralises the precedence rules so generate_image
		// and edit_image stay in sync.
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
			"generate",
			handlerOpts.fallbackPolicy(),
		)
		if err != nil {
			return errorResult(err.Error()), nil
		}

		// response_format is purely an MCP-side delivery hint after Phase 2.
		// The provider decides whether to forward anything to the upstream
		// API; the tool layer below decides how to wrap the response (b64
		// inline vs. file path) using responseFormat.
		req := &provider.GenerateImageRequest{
			Prompt:       prompt,
			Size:         size,
			Quality:      stringArg(args, "quality"),
			OutputFormat: outputFormat,
			N:            n,
		}

		resp, err := imageProvider.GenerateImage(ctx, req)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to generate image: %v", err)), nil
		}
		if len(resp.Images) == 0 {
			return errorResult("Failed to generate image: response contained no images"), nil
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
			return errorResult(fmt.Sprintf("Failed to encode image generation result: %v", err)), nil
		}

		return result, nil
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
