package image

import "fmt"

// VisionInput represents an image input prepared for a vision-style API call.
//
// For HTTP/HTTPS URL inputs, Data is the original URL and MediaType is empty;
// the upstream vision model is expected to fetch the URL on its own.
//
// For local file paths, data URIs, or raw base64 inputs, Data is the base64
// encoded image content and MediaType is the detected MIME type when available.
//
// If a raw base64 input cannot be decoded, the input is passed through
// unchanged with an empty MediaType. This preserves the legacy fallback
// behaviour used by tool handlers before this helper existed and avoids
// rejecting otherwise-valid payloads that happen to fail magic-byte detection.
type VisionInput struct {
	Data      string
	MediaType string
}

// PrepareForVision normalizes a raw image input string into a VisionInput
// suitable for direct use by tool handlers calling a vision-style provider.
//
// Supported input forms (checked in order):
//
//  1. HTTP/HTTPS URL: returned unchanged with an empty MediaType.
//  2. Local file path: read from disk, base64 encoded, MIME detected.
//  3. Data URI (data:<mime>;base64,<payload>): decoded and re-encoded as base64.
//  4. Raw base64: decoded and re-encoded with detected MIME, or passed through
//     unchanged on decode failure.
//
// The function never panics on empty input; callers are expected to validate
// non-empty arguments at the tool layer before calling it.
func (e *Encoder) PrepareForVision(input string) (VisionInput, error) {
	if IsURL(input) {
		return VisionInput{Data: input}, nil
	}

	if IsLocalFilePath(input) {
		data, mediaType, err := e.DecodeInput(input)
		if err != nil {
			return VisionInput{}, fmt.Errorf("read local file: %w", err)
		}
		return VisionInput{Data: EncodeToBase64(data), MediaType: mediaType}, nil
	}

	if IsDataURI(input) {
		data, mediaType, err := e.DecodeInput(input)
		if err != nil {
			return VisionInput{}, fmt.Errorf("decode data URI: %w", err)
		}
		return VisionInput{Data: EncodeToBase64(data), MediaType: mediaType}, nil
	}

	// Raw base64 fallback: keep the legacy permissive behaviour so callers
	// that forward a pre-encoded payload still work even when magic-byte
	// detection fails.
	data, mediaType, err := e.DecodeInput(input)
	if err != nil {
		return VisionInput{Data: input}, nil
	}
	return VisionInput{Data: EncodeToBase64(data), MediaType: mediaType}, nil
}
