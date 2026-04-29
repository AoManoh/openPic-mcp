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

// pngImageProvider is a tiny convenience builder for a mockVisionProvider
// pre-configured to return a single 1x1 PNG image. It keeps each P1
// handler test focused on the behaviour under inspection rather than the
// boilerplate of constructing the upstream stub.
func pngImageProvider() *mockVisionProvider {
	return &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 123,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64, RevisedPrompt: "A cat"},
			},
		},
		editResult: &provider.EditImageResponse{
			Created: 123,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64, RevisedPrompt: "A cat with a hat"},
			},
		},
	}
}

// decodePayload unwraps the JSON envelope used by the image-producing
// tools so test cases can inspect the structured P1 fields directly.
func decodePayload(t *testing.T, text string) imageToolResponse {
	t.Helper()
	var payload imageToolResponse
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("failed to decode tool result: %v", err)
	}
	return payload
}

func TestGenerateImageHandler_PerCallOutputDirOverride(t *testing.T) {
	dir := t.TempDir()
	handler := GenerateImageHandler(pngImageProvider())

	result, err := handler(context.Background(), map[string]any{
		"prompt":     "A cat",
		"output_dir": dir,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	payload := decodePayload(t, result.Content[0].Text)
	if len(payload.Files) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(payload.Files))
	}
	got := payload.Files[0].Path
	if filepath.Dir(got) != dir {
		t.Fatalf("file %q not under override dir %q", got, dir)
	}
	if !strings.HasPrefix(filepath.Base(got), "generate-") {
		t.Errorf("expected default 'generate-' prefix, got %q", filepath.Base(got))
	}
	if payload.Files[0].SizeBytes <= 0 {
		t.Errorf("expected positive SizeBytes, got %d", payload.Files[0].SizeBytes)
	}
	if payload.Files[0].Format != "png" {
		t.Errorf("expected detected format png, got %q", payload.Files[0].Format)
	}
	defer os.Remove(got)
}

func TestGenerateImageHandler_DeploymentDefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	handler := GenerateImageHandler(
		pngImageProvider(),
		WithDefaultOutputDir(dir),
		WithDefaultFilenamePrefix("custom"),
	)

	result, err := handler(context.Background(), map[string]any{"prompt": "A cat"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	payload := decodePayload(t, result.Content[0].Text)
	if len(payload.Files) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(payload.Files))
	}
	got := payload.Files[0].Path
	if filepath.Dir(got) != dir {
		t.Fatalf("file %q not under deployment-default dir %q", got, dir)
	}
	if !strings.HasPrefix(filepath.Base(got), "custom-") {
		t.Errorf("expected deployment-default 'custom-' prefix, got %q", filepath.Base(got))
	}
	defer os.Remove(got)
}

func TestGenerateImageHandler_PerCallFilenamePrefixWinsOverDeploymentDefault(t *testing.T) {
	dir := t.TempDir()
	handler := GenerateImageHandler(
		pngImageProvider(),
		WithDefaultOutputDir(dir),
		WithDefaultFilenamePrefix("deploy"),
	)

	result, err := handler(context.Background(), map[string]any{
		"prompt":          "A cat",
		"filename_prefix": "callscope",
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	payload := decodePayload(t, result.Content[0].Text)
	got := payload.Files[0].Path
	if !strings.HasPrefix(filepath.Base(got), "callscope-") {
		t.Errorf("expected per-call 'callscope-' prefix to win, got %q", filepath.Base(got))
	}
	defer os.Remove(got)
}

func TestGenerateImageHandler_RejectsRelativeOutputDir(t *testing.T) {
	handler := GenerateImageHandler(pngImageProvider())

	result, err := handler(context.Background(), map[string]any{
		"prompt":     "A cat",
		"output_dir": "relative/path",
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for relative output_dir")
	}
	if !strings.Contains(result.Content[0].Text, "absolute path") {
		t.Errorf("error must mention absolute path requirement: %q", result.Content[0].Text)
	}
}

func TestGenerateImageHandler_RejectsInvalidFilenamePrefix(t *testing.T) {
	handler := GenerateImageHandler(pngImageProvider())

	result, err := handler(context.Background(), map[string]any{
		"prompt":          "A cat",
		"filename_prefix": "../escape",
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for invalid filename_prefix")
	}
	if !strings.Contains(result.Content[0].Text, "filename_prefix") {
		t.Errorf("error must mention filename_prefix: %q", result.Content[0].Text)
	}
}

func TestGenerateImageHandler_RejectsInvalidOverwriteString(t *testing.T) {
	handler := GenerateImageHandler(pngImageProvider())

	result, err := handler(context.Background(), map[string]any{
		"prompt":    "A cat",
		"overwrite": "maybe",
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for invalid overwrite value")
	}
	if !strings.Contains(result.Content[0].Text, "overwrite") {
		t.Errorf("error must mention overwrite: %q", result.Content[0].Text)
	}
}

func TestGenerateImageHandler_StructuredFieldsPopulated(t *testing.T) {
	dir := t.TempDir()
	handler := GenerateImageHandler(pngImageProvider())

	result, err := handler(context.Background(), map[string]any{
		"prompt":     "A cat",
		"output_dir": dir,
		"size":       "1024x1024",
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	payload := decodePayload(t, result.Content[0].Text)
	if payload.Requested == nil {
		t.Fatal("expected requested to be populated")
	}
	if payload.Requested.Prompt != "A cat" {
		t.Errorf("Requested.Prompt = %q, want A cat", payload.Requested.Prompt)
	}
	if payload.Requested.Size != "1024x1024" {
		t.Errorf("Requested.Size = %q, want 1024x1024", payload.Requested.Size)
	}
	if payload.Requested.OutputDir != dir {
		t.Errorf("Requested.OutputDir = %q, want %q", payload.Requested.OutputDir, dir)
	}
	if payload.Applied == nil {
		t.Fatal("expected applied to be populated")
	}
	if payload.Applied.Size != "1024x1024" {
		t.Errorf("Applied.Size = %q, want 1024x1024", payload.Applied.Size)
	}
	if payload.Applied.ResponseFormat != "file_path" {
		t.Errorf("Applied.ResponseFormat = %q, want file_path", payload.Applied.ResponseFormat)
	}
	if payload.Applied.OutputDir != dir {
		t.Errorf("Applied.OutputDir = %q, want %q", payload.Applied.OutputDir, dir)
	}
	if payload.Applied.FilenamePrefix != "generate" {
		t.Errorf("Applied.FilenamePrefix = %q, want generate", payload.Applied.FilenamePrefix)
	}
	if payload.Files == nil || len(payload.Files) != 1 {
		t.Fatalf("expected 1 file entry, got %v", payload.Files)
	}
	defer os.Remove(payload.Files[0].Path)
}

func TestGenerateImageHandler_NoOverwriteProducesDistinctFiles(t *testing.T) {
	dir := t.TempDir()
	provider := pngImageProvider()
	handler := GenerateImageHandler(provider)

	first, err := handler(context.Background(), map[string]any{
		"prompt":     "A cat",
		"output_dir": dir,
	})
	if err != nil || first.IsError {
		t.Fatalf("first call failed: err=%v isErr=%v", err, first.IsError)
	}
	second, err := handler(context.Background(), map[string]any{
		"prompt":     "A cat",
		"output_dir": dir,
	})
	if err != nil || second.IsError {
		t.Fatalf("second call failed: err=%v isErr=%v", err, second.IsError)
	}

	p1 := decodePayload(t, first.Content[0].Text).Files[0].Path
	p2 := decodePayload(t, second.Content[0].Text).Files[0].Path
	defer os.Remove(p1)
	defer os.Remove(p2)
	if p1 == p2 {
		t.Fatalf("expected two distinct file paths, got %q twice", p1)
	}
}

func TestGenerateImageHandler_UsageForwardedFromProvider(t *testing.T) {
	in := int64(10)
	out := int64(7)
	total := int64(17)
	mock := pngImageProvider()
	mock.generateResult.Usage = &provider.ImageUsage{
		InputTokens:  &in,
		OutputTokens: &out,
		TotalTokens:  &total,
	}
	handler := GenerateImageHandler(mock)

	result, err := handler(context.Background(), map[string]any{"prompt": "A cat"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	payload := decodePayload(t, result.Content[0].Text)
	if payload.Usage == nil {
		t.Fatal("expected usage to be populated when upstream returned it")
	}
	if payload.Usage.InputTokens == nil || *payload.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %v, want 10", payload.Usage.InputTokens)
	}
	if payload.Usage.OutputTokens == nil || *payload.Usage.OutputTokens != 7 {
		t.Errorf("Usage.OutputTokens = %v, want 7", payload.Usage.OutputTokens)
	}
	if payload.Usage.TotalTokens == nil || *payload.Usage.TotalTokens != 17 {
		t.Errorf("Usage.TotalTokens = %v, want 17", payload.Usage.TotalTokens)
	}
	if len(payload.Files) == 1 {
		defer os.Remove(payload.Files[0].Path)
	}
}

func TestGenerateImageHandler_UsageAbsentWhenUpstreamSilent(t *testing.T) {
	handler := GenerateImageHandler(pngImageProvider())

	result, err := handler(context.Background(), map[string]any{"prompt": "A cat"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	payload := decodePayload(t, result.Content[0].Text)
	if payload.Usage != nil {
		t.Errorf("expected usage to be omitted, got %+v", payload.Usage)
	}
	if len(payload.Files) == 1 {
		defer os.Remove(payload.Files[0].Path)
	}
}

func TestGenerateImageHandler_PayloadThresholdRejectsB64JSON(t *testing.T) {
	// The 1x1 PNG decodes to ~70 bytes. A budget of 10 bytes is well
	// below that and triggers the b64_json refusal path.
	handler := GenerateImageHandler(pngImageProvider(), WithMaxInlinePayloadBytes(10))

	result, err := handler(context.Background(), map[string]any{
		"prompt":          "A cat",
		"response_format": "b64_json",
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when inline payload exceeds budget")
	}
	if !strings.Contains(result.Content[0].Text, "OPENPIC_MAX_INLINE_PAYLOAD_BYTES") {
		t.Errorf("error must mention OPENPIC_MAX_INLINE_PAYLOAD_BYTES: %q", result.Content[0].Text)
	}
}

func TestGenerateImageHandler_PayloadThresholdWarnsOnFilePath(t *testing.T) {
	dir := t.TempDir()
	handler := GenerateImageHandler(pngImageProvider(), WithMaxInlinePayloadBytes(10))

	result, err := handler(context.Background(), map[string]any{
		"prompt":     "A cat",
		"output_dir": dir,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("file_path delivery must succeed even when over budget; got error: %v", result.Content)
	}

	payload := decodePayload(t, result.Content[0].Text)
	if len(payload.Files) != 1 {
		t.Fatalf("expected file to still be written, got %d files", len(payload.Files))
	}
	defer os.Remove(payload.Files[0].Path)
	if len(payload.Warnings) == 0 {
		t.Fatal("expected at least one warning about the inline payload size")
	}
	found := false
	for _, w := range payload.Warnings {
		if strings.Contains(w, "OPENPIC_MAX_INLINE_PAYLOAD_BYTES") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warnings did not mention OPENPIC_MAX_INLINE_PAYLOAD_BYTES: %v", payload.Warnings)
	}
}

func TestEditImageHandler_PerCallOutputDirOverride(t *testing.T) {
	dir := t.TempDir()
	handler := EditImageHandler(pngImageProvider())

	result, err := handler(context.Background(), map[string]any{
		"image":      onePixelPNGBase64,
		"prompt":     "Add a hat",
		"output_dir": dir,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	payload := decodePayload(t, result.Content[0].Text)
	if len(payload.Files) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(payload.Files))
	}
	got := payload.Files[0].Path
	if filepath.Dir(got) != dir {
		t.Fatalf("file %q not under override dir %q", got, dir)
	}
	if !strings.HasPrefix(filepath.Base(got), "edit-") {
		t.Errorf("expected default 'edit-' prefix, got %q", filepath.Base(got))
	}
	defer os.Remove(got)
}
