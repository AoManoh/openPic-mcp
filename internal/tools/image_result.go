package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	imageutil "github.com/AoManoh/openPic-mcp/internal/image"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// imageToolResponse is the JSON payload the tool layer hands back to MCP
// clients. It uses the tool-local ImageResult DTO (see result.go) so the
// serialization shape is independent from the provider abstraction.
//
// Warnings carries advisory messages produced by the tool layer itself,
// for example when the user-requested output_format does not match the
// format actually returned by the upstream API. Warnings never replace
// errors; they are surfaced alongside successful payloads so callers can
// decide whether to retry, fall back, or accept the discrepancy.
type imageToolResponse struct {
	Images   []ImageResult `json:"images"`
	Created  int64         `json:"created"`
	Warnings []string      `json:"warnings,omitempty"`
}

// imageToolResult turns a slice of ImageResult values into a serialized
// MCP tool-call result. Depending on responseFormat it either keeps inline
// base64 (b64_json) or persists the bytes to a temp file and clears the
// inline payload to avoid bloating MCP transcripts. URL fields that are
// actually data URIs are cleared along with B64JSON whenever a file path
// is produced, since both reproduce the same bytes.
//
// requestedOutputFormat is the upstream output_format value the caller
// asked for ("png" / "jpeg" / "webp"). It is used purely to detect
// silent contract violations: when it is non-empty and disagrees with
// the format actually present in the payload, the function attaches an
// advisory warning. Pass an empty string when the caller did not request
// a specific format.
func imageToolResult(
	images []ImageResult,
	created int64,
	responseFormat string,
	requestedOutputFormat string,
	filePrefix string,
) (*types.ToolCallResult, error) {
	resultImages := make([]ImageResult, len(images))
	copy(resultImages, images)

	var warnings []string
	for i := range resultImages {
		var detectedFormat string
		if responseFormat != "b64_json" {
			path, format, err := saveInlineImageResult(resultImages[i], filePrefix)
			if err != nil {
				return nil, err
			}
			if path != "" {
				resultImages[i].FilePath = path
				if imageutil.IsDataURI(resultImages[i].URL) {
					resultImages[i].URL = ""
				}
				resultImages[i].B64JSON = ""
			}
			detectedFormat = format
		} else {
			// b64_json response_format: do not persist anything but still
			// detect the real format from the inline payload so callers
			// receive a single authoritative format value regardless of
			// delivery shape.
			detectedFormat = detectFormatFromInline(resultImages[i])
		}

		if detectedFormat != "" {
			resultImages[i].Format = detectedFormat
		}

		if msg := outputFormatMismatchWarning(requestedOutputFormat, detectedFormat, i); msg != "" {
			warnings = append(warnings, msg)
		}
	}

	payload := imageToolResponse{
		Images:   resultImages,
		Created:  created,
		Warnings: warnings,
	}
	resultJSON, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
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

// saveInlineImageResult persists any inline payload referenced by the given
// ImageResult to a temp file and returns the resulting absolute path together
// with the canonical format name detected from the bytes. Returns an empty
// path and empty format when the image only carries a non-data URL.
func saveInlineImageResult(image ImageResult, filePrefix string) (string, string, error) {
	if image.B64JSON != "" {
		return saveBase64Image(image.B64JSON, filePrefix)
	}
	if imageutil.IsDataURI(image.URL) {
		return saveBase64Image(image.URL, filePrefix)
	}
	return "", "", nil
}

// detectFormatFromInline runs magic-byte detection over inline payloads
// without writing them to disk. Used by the b64_json response_format
// branch so callers still see a Format field even when no file is saved.
func detectFormatFromInline(image ImageResult) string {
	encoded := image.B64JSON
	if encoded == "" && imageutil.IsDataURI(image.URL) {
		encoded = image.URL
	}
	if encoded == "" {
		return ""
	}
	data, mimeType, err := decodeGeneratedImage(encoded)
	if err != nil {
		return ""
	}
	return canonicalFormat(data, mimeType)
}

func saveBase64Image(encoded string, filePrefix string) (string, string, error) {
	data, mimeType, err := decodeGeneratedImage(encoded)
	if err != nil {
		return "", "", err
	}

	format := canonicalFormat(data, mimeType)
	ext := extensionForFormat(format)
	if ext == "" {
		ext = "png"
	}
	dir := filepath.Join(os.TempDir(), "openpic-mcp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("failed to create image output directory: %w", err)
	}

	file, err := os.CreateTemp(dir, filePrefix+"-*."+ext)
	if err != nil {
		return "", "", fmt.Errorf("failed to create image output file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return "", "", fmt.Errorf("failed to write image output file: %w", err)
	}

	return file.Name(), format, nil
}

// canonicalFormat returns the canonical format name (image.FormatPNG, etc.)
// detected from the payload, preferring magic bytes over the upstream MIME
// type when both are available. Empty string when neither source yields a
// known format.
func canonicalFormat(data []byte, mimeType string) string {
	if format, err := imageutil.ValidateFormat(data); err == nil {
		return format
	}
	if format, err := imageutil.FormatFromMIME(mimeType); err == nil {
		return format
	}
	return ""
}

// outputFormatMismatchWarning returns a warning string when the upstream
// response disagrees with the user-requested output_format. Empty when the
// caller did not specify a format, when detection failed, or when the two
// values agree after canonicalisation. The index is included verbatim so
// multi-image responses remain decipherable.
func outputFormatMismatchWarning(requested string, detected string, index int) string {
	if requested == "" || detected == "" {
		return ""
	}
	want, err := imageutil.FormatFromExtension(requested)
	if err != nil {
		return ""
	}
	if want == detected {
		return ""
	}
	return fmt.Sprintf(
		"images[%d]: requested output_format=%q but upstream returned %q; "+
			"saved as .%s. Some OpenAI-compatible providers (e.g. gpt-image-1 /v1/images/edits) "+
			"silently ignore unsupported output formats and fall back to PNG.",
		index, requested, detected, extensionForFormat(detected),
	)
}

func decodeGeneratedImage(encoded string) ([]byte, string, error) {
	encoded = strings.TrimSpace(encoded)
	if imageutil.IsDataURI(encoded) {
		data, mimeType, err := imageutil.NewEncoder().DecodeInput(encoded)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decode generated image data URI: %w", err)
		}
		return data, mimeType, nil
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(encoded)
	}
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode generated image b64_json: %w", err)
	}

	mimeType := ""
	if format, err := imageutil.ValidateFormat(data); err == nil {
		mimeType = imageutil.GetMIMEType(format)
	}
	return data, mimeType, nil
}

func extensionForFormat(format string) string {
	switch format {
	case imageutil.FormatJPEG:
		return "jpg"
	case imageutil.FormatTIFF:
		return "tiff"
	case imageutil.FormatICO:
		return "ico"
	case imageutil.FormatPNG, imageutil.FormatWebP, imageutil.FormatGIF, imageutil.FormatBMP, imageutil.FormatHEIC, imageutil.FormatAVIF, imageutil.FormatSVG:
		return format
	default:
		return "png"
	}
}
