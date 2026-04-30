package tools

import "fmt"

// supportedOutputFormats lists the image encodings the MCP image-producing
// tools accept and forward verbatim to the upstream image API. The list is
// kept narrow to mirror what GPT image and the major OpenAI-compatible
// proxies (notably Cloudflare AI's gpt-image-2) document as supported.
var supportedOutputFormats = []string{"png", "jpeg", "webp"}

// Aspect ratio keys exposed in the MCP schema. They are intentionally
// declared as named constants so they can be referenced from both the
// schema declarations and the resolver below without typos.
const (
	aspectRatio1To1  = "1:1"
	aspectRatio4To3  = "4:3"
	aspectRatio3To4  = "3:4"
	aspectRatio16To9 = "16:9"
	aspectRatio9To16 = "9:16"
	aspectRatioAuto  = "auto"
)

// supportedAspectRatios mirrors the aspect ratio enum advertised in the
// MCP schema. The slice ordering doubles as the documentation order shown
// to clients.
var supportedAspectRatios = []string{
	aspectRatio1To1,
	aspectRatio4To3,
	aspectRatio3To4,
	aspectRatio16To9,
	aspectRatio9To16,
	aspectRatioAuto,
}

// aspectRatioToSize maps each supported aspect ratio onto a value in the
// official size enum. 16:9 and 9:16 map to the closest landscape/portrait
// preset (1536x1024 / 1024x1536) since the upstream image API does not
// expose a true 16:9 size. The "auto" alias intentionally maps to an empty
// string so the upstream picks its own default; callers must check for the
// empty result and skip forwarding the size field in that case.
var aspectRatioToSize = map[string]string{
	aspectRatio1To1:  "1024x1024",
	aspectRatio4To3:  "1536x1024",
	aspectRatio3To4:  "1024x1536",
	aspectRatio16To9: "1536x1024",
	aspectRatio9To16: "1024x1536",
	aspectRatioAuto:  "",
}

// resolveImageSize chooses the size that should be forwarded to the image
// provider for a single MCP call. The selection rules are deliberate:
//
//  1. An explicit size argument always wins; an unsupported value yields a
//     descriptive error so callers see the same enum guidance regardless of
//     which tool they invoke.
//  2. If size is omitted but aspect_ratio is set, the latter is mapped via
//     aspectRatioToSize.
//  3. With neither value present the function returns defaultImageSize so
//     existing behaviour is preserved.
//
// The returned string is empty when the resolved aspect ratio is "auto";
// callers should treat an empty result as "leave the size field unset" and
// rely on the upstream default.
func resolveImageSize(size, aspectRatio string) (string, error) {
	if size != "" {
		if !containsString(supportedImageSizes, size) {
			return "", fmt.Errorf("unsupported size %q: expected one of %v", size, supportedImageSizes)
		}
		return size, nil
	}
	if aspectRatio == "" {
		return defaultImageSize, nil
	}
	mapped, ok := aspectRatioToSize[aspectRatio]
	if !ok {
		return "", fmt.Errorf("unsupported aspect_ratio %q: expected one of %v", aspectRatio, supportedAspectRatios)
	}
	return mapped, nil
}

// validateOutputFormat validates a non-empty output_format argument against
// the supported list. An empty value is valid (means "use upstream default").
func validateOutputFormat(value string) error {
	if value == "" {
		return nil
	}
	if !containsString(supportedOutputFormats, value) {
		return fmt.Errorf("unsupported output_format %q: expected one of %v", value, supportedOutputFormats)
	}
	return nil
}

// supportedResponseFormats lists the MCP-side delivery shapes the image
// tools understand. This is purely an MCP contract — openPic-mcp never
// forwards response_format to the upstream image API (see capabilities.go
// for the rationale). The values are duplicated in generate.go and
// edit.go schemas; keeping a single source of truth here lets the
// runtime validator below stay in lockstep with the advertised enum.
var supportedResponseFormats = []string{"file_path", "url", "b64_json"}

// validateResponseFormat is the defense-in-depth check for the
// response_format enum. The schema already advertises the allowed
// values, but JSON-RPC clients vary in how strictly they enforce
// schemas — IDE 5th-round stress test report (R5 follow-up) found
// that an unknown response_format would silently fall through to the
// file_path branch, masking the typo. Catching it here gives callers
// a fielded error symmetric with the unsupported size / aspect_ratio
// / output_format messages.
func validateResponseFormat(value string) error {
	if value == "" {
		return nil
	}
	if !containsString(supportedResponseFormats, value) {
		return fmt.Errorf("unsupported response_format %q: expected one of %v", value, supportedResponseFormats)
	}
	return nil
}
