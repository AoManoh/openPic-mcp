package tools

import "github.com/AoManoh/openPic-mcp/internal/provider"

// ImageResult is the tool-layer DTO returned to MCP clients for image
// generation and image editing. It mirrors the shape of upstream
// responses but lives in the tools package so persistence and serialization
// concerns can evolve independently from the provider abstraction.
//
// Each MCP image-producing tool maps its provider response into a slice of
// ImageResult before delegating to imageToolResult, which decides whether to
// embed inline base64, expose a URL, or save the bytes to a temp file
// according to the user-requested response_format.
//
// Format is the format actually detected from the response payload via
// magic bytes (e.g. "png" / "jpeg" / "webp"). It is filled in by the tool
// layer rather than the provider so that callers always receive a single
// authoritative source of truth, even when the upstream API silently
// ignores the requested output_format. Empty when the payload could not
// be decoded (for example a real HTTP URL the upstream returns directly).
type ImageResult struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	FilePath      string `json:"file_path,omitempty"`
	Format        string `json:"format,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// imageResultsFromProvider converts a slice of provider-level GeneratedImage
// values into the tool-layer ImageResult DTO. It is the single seam between
// the provider abstraction and the tool layer's serialization code so the
// rest of the package never needs to import provider types.
func imageResultsFromProvider(items []provider.GeneratedImage) []ImageResult {
	out := make([]ImageResult, len(items))
	for i, item := range items {
		out[i] = ImageResult{
			URL:           item.URL,
			B64JSON:       item.B64JSON,
			FilePath:      item.FilePath,
			RevisedPrompt: item.RevisedPrompt,
		}
	}
	return out
}
