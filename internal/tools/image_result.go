package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
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
//
// Requested, Applied, Files and Usage are added by the P1 structured
// result contract and are all optional. Existing fields (Images,
// Created, Warnings) keep their previous semantics so older clients that
// only inspect those keep working unchanged.
type imageToolResponse struct {
	Images    []ImageResult    `json:"images"`
	Created   int64            `json:"created"`
	Requested *RequestedParams `json:"requested,omitempty"`
	Applied   *AppliedParams   `json:"applied,omitempty"`
	Files     []ImageFileInfo  `json:"files,omitempty"`
	Usage     *UsageInfo       `json:"usage,omitempty"`
	Warnings  []string         `json:"warnings,omitempty"`
}

// imageToolResult turns a slice of ImageResult values into a serialized
// MCP tool-call result. Depending on responseFormat it either keeps inline
// base64 (b64_json) or persists the bytes via writeImageBytes and clears
// the inline payload to avoid bloating MCP transcripts. URL fields that
// are actually data URIs are cleared along with B64JSON whenever a file
// path is produced, since both reproduce the same bytes.
//
// requestedOutputFormat is the upstream output_format value the caller
// asked for ("png" / "jpeg" / "webp"). It is used purely to detect
// silent contract violations: when it is non-empty and disagrees with
// the format actually present in the payload, the function attaches an
// advisory warning. Pass an empty string when the caller did not request
// a specific format.
//
// policy is the resolved output-path policy (Dir / Prefix / Overwrite)
// produced by resolveOutputPolicy. The Prefix field must be non-empty
// when responseFormat is anything other than b64_json so the writer has
// a deterministic name to emit. Tests that exercise the b64_json branch
// can pass a zero policy.
//
// requested and applied are optional structured echoes of the tool call
// parameters. When non-nil they are surfaced verbatim in the response so
// MCP clients can distinguish "user supplied" from "tool layer applied".
// usage is forwarded only when the upstream provider actually returned
// token usage figures — the function never fabricates them.
//
// maxInlinePayloadBytes (added by P1 / T6) caps the size of inline base64
// payloads that may flow through the MCP transcript. When the limit is
// exceeded the behaviour depends on responseFormat:
//   - b64_json: the function returns an isError ToolCallResult so the
//     caller has to opt in to a different delivery shape rather than
//     silently switching to file_path. This preserves the project rule
//     that the tool layer never rewrites caller-visible semantics.
//   - file_path / url: the bytes are still written to disk but a warning
//     is attached so callers know the inline payload was unusually
//     large. A non-positive limit disables the guard.
func imageToolResult(
	images []ImageResult,
	created int64,
	responseFormat string,
	requestedOutputFormat string,
	policy outputPathPolicy,
	requested *RequestedParams,
	applied *AppliedParams,
	usage *UsageInfo,
	maxInlinePayloadBytes int64,
) (*types.ToolCallResult, error) {
	resultImages := make([]ImageResult, len(images))
	copy(resultImages, images)

	var warnings []string
	var files []ImageFileInfo
	for i := range resultImages {
		inlineBytes := inlinePayloadByteCount(resultImages[i])
		if maxInlinePayloadBytes > 0 && responseFormat == "b64_json" && inlineBytes > maxInlinePayloadBytes {
			return errorResult(fmt.Sprintf(
				"images[%d]: inline payload %d bytes exceeds OPENPIC_MAX_INLINE_PAYLOAD_BYTES=%d; "+
					"set response_format=file_path or reduce quality/size",
				i, inlineBytes, maxInlinePayloadBytes,
			)), nil
		}

		var detectedFormat string
		var savedExtension string
		if responseFormat != "b64_json" {
			path, format, err := saveInlineImageResult(resultImages[i], policy)
			if err != nil {
				return nil, err
			}
			if path != "" {
				resultImages[i].FilePath = path
				if imageutil.IsDataURI(resultImages[i].URL) {
					resultImages[i].URL = ""
				}
				resultImages[i].B64JSON = ""
				savedExtension = extensionForFormat(format)
				if info, err := buildImageFileInfo(i, path, format); err != nil {
					return nil, err
				} else {
					files = append(files, info)
				}
			}
			detectedFormat = format
			if maxInlinePayloadBytes > 0 && inlineBytes > maxInlinePayloadBytes {
				warnings = append(warnings, fmt.Sprintf(
					"images[%d]: inline payload %d bytes exceeded OPENPIC_MAX_INLINE_PAYLOAD_BYTES=%d; "+
						"file written to disk but consider lowering quality/size to avoid future inline overruns",
					i, inlineBytes, maxInlinePayloadBytes,
				))
			}
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

		if msg := outputFormatMismatchWarning(requestedOutputFormat, detectedFormat, savedExtension, i); msg != "" {
			warnings = append(warnings, msg)
		}
	}

	payload := imageToolResponse{
		Images:    resultImages,
		Created:   created,
		Requested: requested,
		Applied:   applied,
		Files:     files,
		Usage:     usage,
		Warnings:  warnings,
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

// buildImageFileInfo collects the metadata fields that go into the
// top-level files[] slice for a single saved image. It stat()s the
// resulting file once so callers receive a concrete byte count rather
// than relying on the upstream Content-Length, which is sometimes
// unavailable for data URIs and base64 responses.
func buildImageFileInfo(index int, path, format string) (ImageFileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ImageFileInfo{}, fmt.Errorf("failed to stat output file %q: %w", path, err)
	}
	return ImageFileInfo{
		Index:     index,
		Path:      path,
		SizeBytes: info.Size(),
		Format:    format,
	}, nil
}

// saveInlineImageResult persists any inline payload referenced by the
// given ImageResult according to the resolved policy and returns the
// resulting absolute path together with the canonical format name
// detected from the bytes. Returns an empty path and empty format when
// the image only carries a non-data URL, signalling to the caller that
// no file was produced for that record.
func saveInlineImageResult(image ImageResult, policy outputPathPolicy) (string, string, error) {
	if image.B64JSON != "" {
		return saveBase64Image(image.B64JSON, policy)
	}
	if imageutil.IsDataURI(image.URL) {
		return saveBase64Image(image.URL, policy)
	}
	return "", "", nil
}

// inlinePayloadByteCount returns the decoded size in bytes of any inline
// payload referenced by an ImageResult (b64_json or data URI). Returns
// 0 when the image only carries a non-data URL or when decoding fails;
// non-data URLs are treated as zero because their bytes never travel
// through the MCP transcript and therefore cannot violate the inline
// payload guard.
func inlinePayloadByteCount(image ImageResult) int64 {
	encoded := image.B64JSON
	if encoded == "" && imageutil.IsDataURI(image.URL) {
		encoded = image.URL
	}
	if encoded == "" {
		return 0
	}
	data, _, err := decodeGeneratedImage(encoded)
	if err != nil {
		return 0
	}
	return int64(len(data))
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

// saveBase64Image decodes the given inline payload (raw base64 or data
// URI) and writes the bytes to the location dictated by policy. The
// returned file path is absolute and the format string is the canonical
// magic-byte name (png / jpeg / webp ...) detected from the decoded
// bytes; both are surfaced verbatim in the response payload.
//
// All filesystem layout, name composition and overwrite handling lives
// inside writeImageBytes so this function stays focused on decoding and
// format detection.
func saveBase64Image(encoded string, policy outputPathPolicy) (string, string, error) {
	data, mimeType, err := decodeGeneratedImage(encoded)
	if err != nil {
		return "", "", err
	}

	format := canonicalFormat(data, mimeType)
	path, err := writeImageBytes(policy, data, format)
	if err != nil {
		return "", "", err
	}
	return path, format, nil
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
func outputFormatMismatchWarning(requested string, detected string, savedExtension string, index int) string {
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
	delivery := "inline payload retained."
	if savedExtension != "" {
		delivery = fmt.Sprintf("saved as .%s.", savedExtension)
	}
	return fmt.Sprintf(
		"images[%d]: requested output_format=%q but upstream returned %q; "+
			"%s Some OpenAI-compatible providers (e.g. gpt-image-1 /v1/images/edits) "+
			"silently ignore unsupported output formats and fall back to PNG.",
		index, requested, detected, delivery,
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
