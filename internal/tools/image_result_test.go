package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AoManoh/openPic-mcp/internal/provider"
)

// formatMismatchPNG mirrors the upstream behaviour observed against
// gpt-image-1 /v1/images/edits and several OpenAI-compatible proxies:
// the caller asks for output_format=webp but the bytes returned are PNG.
func TestImageToolResult_FormatMismatchAddsWarningAndKeepsRealFormat(t *testing.T) {
	images := []ImageResult{
		{B64JSON: onePixelPNGBase64},
	}

	result, err := imageToolResult(images, 100, defaultImageResponseFormat, "webp", outputPathPolicy{Prefix: "test-mismatch"}, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("imageToolResult returned error: %v", err)
	}

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if len(payload.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(payload.Images))
	}
	if payload.Images[0].Format != "png" {
		t.Errorf("Format = %q, want %q", payload.Images[0].Format, "png")
	}
	if payload.Images[0].FilePath == "" {
		t.Fatal("expected file_path to be filled in")
	}
	defer os.Remove(payload.Images[0].FilePath)

	// Magic-byte-driven extension must be the real one, not the requested one.
	if got := strings.ToLower(filepath.Ext(payload.Images[0].FilePath)); got != ".png" {
		t.Errorf("file extension = %q, want %q", got, ".png")
	}
	if len(payload.Warnings) == 0 {
		t.Fatal("expected at least one warning when requested format mismatches")
	}
	w := payload.Warnings[0]
	if !strings.Contains(w, `output_format="webp"`) {
		t.Errorf("warning does not mention requested format: %q", w)
	}
	if !strings.Contains(w, `"png"`) {
		t.Errorf("warning does not mention detected format: %q", w)
	}
	if !strings.Contains(w, "saved as .png") {
		t.Errorf("warning does not mention final extension: %q", w)
	}
}

// When the upstream honours output_format the warnings array stays empty
// and Format still reflects the bytes that hit disk.
func TestImageToolResult_FormatMatch_NoWarning(t *testing.T) {
	images := []ImageResult{
		{B64JSON: onePixelPNGBase64},
	}

	result, err := imageToolResult(images, 100, defaultImageResponseFormat, "png", outputPathPolicy{Prefix: "test-match"}, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("imageToolResult returned error: %v", err)
	}

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if len(payload.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", payload.Warnings)
	}
	if payload.Images[0].Format != "png" {
		t.Errorf("Format = %q, want %q", payload.Images[0].Format, "png")
	}
	defer os.Remove(payload.Images[0].FilePath)
}

// When the caller does not specify output_format we must never produce a
// mismatch warning; the field is purely advisory.
func TestImageToolResult_NoRequestedOutputFormat_NoWarning(t *testing.T) {
	images := []ImageResult{
		{B64JSON: onePixelPNGBase64},
	}

	result, err := imageToolResult(images, 100, defaultImageResponseFormat, "", outputPathPolicy{Prefix: "test-blank"}, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("imageToolResult returned error: %v", err)
	}

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if len(payload.Warnings) != 0 {
		t.Errorf("expected no warnings when output_format is unset, got %v", payload.Warnings)
	}
	if payload.Images[0].Format != "png" {
		t.Errorf("Format must still reflect bytes, got %q", payload.Images[0].Format)
	}
	defer os.Remove(payload.Images[0].FilePath)
}

// b64_json delivery mode keeps the inline payload but should still expose
// the magic-byte-detected Format so callers do not have to re-detect it.
func TestImageToolResult_B64JSON_PopulatesFormatWithoutPersisting(t *testing.T) {
	images := []ImageResult{
		{B64JSON: onePixelPNGBase64},
	}

	result, err := imageToolResult(images, 100, "b64_json", "", outputPathPolicy{Prefix: "test-b64"}, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("imageToolResult returned error: %v", err)
	}

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if payload.Images[0].FilePath != "" {
		t.Errorf("FilePath must remain empty for b64_json delivery, got %q", payload.Images[0].FilePath)
	}
	if payload.Images[0].B64JSON == "" {
		t.Error("B64JSON must be preserved for b64_json delivery")
	}
	if payload.Images[0].Format != "png" {
		t.Errorf("Format = %q, want %q", payload.Images[0].Format, "png")
	}
}

func TestImageToolResult_B64JSONMismatchWarningDoesNotClaimFileSaved(t *testing.T) {
	images := []ImageResult{
		{B64JSON: onePixelPNGBase64},
	}

	result, err := imageToolResult(images, 100, "b64_json", "webp", outputPathPolicy{Prefix: "test-b64-mismatch"}, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("imageToolResult returned error: %v", err)
	}

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if payload.Images[0].FilePath != "" {
		t.Errorf("FilePath must remain empty for b64_json delivery, got %q", payload.Images[0].FilePath)
	}
	if payload.Images[0].Format != "png" {
		t.Errorf("Format = %q, want %q", payload.Images[0].Format, "png")
	}
	if len(payload.Warnings) != 1 {
		t.Fatalf("warnings length = %d, want 1", len(payload.Warnings))
	}
	if strings.Contains(payload.Warnings[0], "saved as") {
		t.Fatalf("b64_json mismatch warning must not claim a file was saved: %q", payload.Warnings[0])
	}
	if !strings.Contains(payload.Warnings[0], "inline payload retained") {
		t.Fatalf("b64_json mismatch warning must mention inline payload: %q", payload.Warnings[0])
	}
}

// End-to-end through GenerateImageHandler: requesting webp from a provider
// that returns PNG should still succeed but surface the mismatch warning so
// the user (or upstream agent) knows the contract was downgraded.
func TestGenerateImageHandler_WebpRequestedPngReturnedSurfacesWarning(t *testing.T) {
	mockProvider := &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 123,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64, RevisedPrompt: "A cat"},
			},
		},
	}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"prompt":        "A cat",
		"output_format": "webp",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if mockProvider.generateReq.OutputFormat != "webp" {
		t.Errorf("provider OutputFormat = %q, want %q", mockProvider.generateReq.OutputFormat, "webp")
	}

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if payload.Images[0].Format != "png" {
		t.Errorf("Format = %q, want %q", payload.Images[0].Format, "png")
	}
	if len(payload.Warnings) == 0 {
		t.Fatal("expected mismatch warning, got none")
	}
	if !strings.Contains(payload.Warnings[0], "webp") || !strings.Contains(payload.Warnings[0], "png") {
		t.Errorf("warning lacks key tokens: %q", payload.Warnings[0])
	}
	defer os.Remove(payload.Images[0].FilePath)
}
