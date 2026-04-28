package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// ListImageCapabilitiesTool exposes a static description of what the
// image-producing tools currently accept. It performs no upstream calls and
// is safe to invoke on any deployment, even when OPENPIC_IMAGE_MODEL has not
// been configured. The intent is to give MCP clients a single place to look
// up enums (size / aspect_ratio / output_format / response_format) and the
// tool layer's contract with the upstream API.
var ListImageCapabilitiesTool = types.Tool{
	Name:        "list_image_capabilities",
	Description: "List the size, aspect_ratio, output_format and response_format values currently accepted by generate_image and edit_image, plus a note on upstream forwarding behaviour.",
	InputSchema: types.InputSchema{
		Type:                 "object",
		Properties:           map[string]types.Property{},
		AdditionalProperties: false,
	},
}

// ImageCapabilities is the JSON payload returned by list_image_capabilities.
// Field names are stable so callers can depend on the keys; new fields may be
// added in future versions but existing ones will not be renamed.
type ImageCapabilities struct {
	Sizes                           []string          `json:"sizes"`
	AspectRatios                    []string          `json:"aspect_ratios"`
	AspectRatioToSize               map[string]string `json:"aspect_ratio_to_size"`
	OutputFormats                   []string          `json:"output_formats"`
	OutputFormatEnforcement         string            `json:"output_format_enforcement"`
	OutputFormatNotes               string            `json:"output_format_notes"`
	ResponseFormats                 []string          `json:"response_formats"`
	DefaultSize                     string            `json:"default_size"`
	DefaultResponseFormat           string            `json:"default_response_format"`
	MaxCompareImages                int               `json:"max_compare_images"`
	MaxImageResultsPerCall          int               `json:"max_image_results_per_call"`
	UpstreamResponseFormatOmitted   bool              `json:"upstream_response_format_omitted"`
	UpstreamResponseFormatRationale string            `json:"upstream_response_format_rationale"`
}

// ListImageCapabilitiesHandler returns a handler that emits the static
// capability snapshot described above. It is a pure function — no provider
// dependency — so registering it never requires an upstream provider to be
// available, which keeps the MCP server discoverable even before the user
// has finished configuring image credentials.
func ListImageCapabilitiesHandler() types.ToolHandler {
	return func(_ context.Context, _ map[string]any) (*types.ToolCallResult, error) {
		caps := ImageCapabilities{
			Sizes:                           append([]string(nil), supportedImageSizes...),
			AspectRatios:                    append([]string(nil), supportedAspectRatios...),
			AspectRatioToSize:               copyAspectRatioMap(),
			OutputFormats:                   append([]string(nil), supportedOutputFormats...),
			OutputFormatEnforcement:         "advisory",
			OutputFormatNotes:               "openPic-mcp forwards output_format verbatim to the upstream API but cannot enforce it. Some OpenAI-compatible providers (notably gpt-image-1 /v1/images/edits and several proxies such as sub2api) silently ignore output_format=webp and return PNG instead. Each ImageResult therefore carries a magic-byte-detected format field, and imageToolResponse.warnings reports any mismatch so callers can react.",
			ResponseFormats:                 []string{"file_path", "url", "b64_json"},
			DefaultSize:                     defaultImageSize,
			DefaultResponseFormat:           defaultImageResponseFormat,
			MaxCompareImages:                DefaultMaxImages,
			MaxImageResultsPerCall:          maxImageResults,
			UpstreamResponseFormatOmitted:   true,
			UpstreamResponseFormatRationale: "openPic-mcp never forwards response_format to /v1/images/generations or /v1/images/edits; GPT image models always return b64_json and several OpenAI-compatible proxies reject the field.",
		}

		body, err := json.MarshalIndent(caps, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal capabilities: %w", err)
		}

		return &types.ToolCallResult{
			Content: []types.ContentItem{
				{Type: "text", Text: string(body)},
			},
			IsError: false,
		}, nil
	}
}

// copyAspectRatioMap returns a defensive copy of aspectRatioToSize so the
// internal mapping cannot be mutated by clients of the result struct.
func copyAspectRatioMap() map[string]string {
	out := make(map[string]string, len(aspectRatioToSize))
	for k, v := range aspectRatioToSize {
		out[k] = v
	}
	return out
}
