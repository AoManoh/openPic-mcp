package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/AoManoh/openPic-mcp/internal/provider"
)

func TestResolveImageSize_ExplicitSizeWins(t *testing.T) {
	got, err := resolveImageSize("1536x1024", aspectRatio1To1)
	if err != nil {
		t.Fatalf("resolveImageSize returned error: %v", err)
	}
	if got != "1536x1024" {
		t.Errorf("explicit size should win, got %q", got)
	}
}

func TestResolveImageSize_AspectRatioMapping(t *testing.T) {
	cases := []struct {
		aspect string
		want   string
	}{
		{aspectRatio1To1, "1024x1024"},
		{aspectRatio4To3, "1536x1024"},
		{aspectRatio3To4, "1024x1536"},
		{aspectRatio16To9, "1536x1024"},
		{aspectRatio9To16, "1024x1536"},
		{aspectRatioAuto, ""},
	}
	for _, tc := range cases {
		t.Run(tc.aspect, func(t *testing.T) {
			got, err := resolveImageSize("", tc.aspect)
			if err != nil {
				t.Fatalf("resolveImageSize(%q) returned error: %v", tc.aspect, err)
			}
			if got != tc.want {
				t.Errorf("aspect %q -> size %q, want %q", tc.aspect, got, tc.want)
			}
		})
	}
}

func TestResolveImageSize_DefaultsToPreset(t *testing.T) {
	got, err := resolveImageSize("", "")
	if err != nil {
		t.Fatalf("resolveImageSize returned error: %v", err)
	}
	if got != defaultImageSize {
		t.Errorf("default fallback = %q, want %q", got, defaultImageSize)
	}
}

func TestResolveImageSize_RejectsUnsupportedSize(t *testing.T) {
	_, err := resolveImageSize("512x512", "")
	if err == nil {
		t.Fatal("expected error for unsupported size")
	}
	if !strings.Contains(err.Error(), "unsupported size") {
		t.Errorf("error should mention unsupported size: %v", err)
	}
}

func TestResolveImageSize_RejectsUnknownAspectRatio(t *testing.T) {
	_, err := resolveImageSize("", "8:9")
	if err == nil {
		t.Fatal("expected error for unknown aspect_ratio")
	}
	if !strings.Contains(err.Error(), "unsupported aspect_ratio") {
		t.Errorf("error should mention unsupported aspect_ratio: %v", err)
	}
}

func TestValidateOutputFormat(t *testing.T) {
	if err := validateOutputFormat(""); err != nil {
		t.Errorf("empty output_format must be valid: %v", err)
	}
	for _, value := range supportedOutputFormats {
		if err := validateOutputFormat(value); err != nil {
			t.Errorf("%q should be valid: %v", value, err)
		}
	}
	if err := validateOutputFormat("avif"); err == nil {
		t.Error("avif should be rejected by validateOutputFormat")
	}
}

// TestValidateResponseFormat is the R5 follow-up regression: every
// value the schema advertises in the response_format enum must be
// accepted, every other value must be rejected with a fielded error.
// Empty stays valid so existing callers that omit the field continue
// to get the default (file_path).
func TestValidateResponseFormat(t *testing.T) {
	if err := validateResponseFormat(""); err != nil {
		t.Errorf("empty response_format must be valid: %v", err)
	}
	for _, value := range supportedResponseFormats {
		if err := validateResponseFormat(value); err != nil {
			t.Errorf("%q should be valid: %v", value, err)
		}
	}
	for _, value := range []string{"banana", "stream", "json", "binary", "FILE_PATH"} {
		err := validateResponseFormat(value)
		if err == nil {
			t.Errorf("%q must be rejected", value)
			continue
		}
		if !strings.Contains(err.Error(), "unsupported response_format") {
			t.Errorf("error for %q must mention 'unsupported response_format', got %v", value, err)
		}
	}
}

func TestGenerateImageHandler_AspectRatioMapsToSize(t *testing.T) {
	mockProvider := &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 100,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64},
			},
		},
	}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"prompt":       "A cat",
		"aspect_ratio": "4:3",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if mockProvider.generateReq == nil {
		t.Fatal("provider was not called")
	}
	if mockProvider.generateReq.Size != "1536x1024" {
		t.Errorf("size = %q, want 1536x1024", mockProvider.generateReq.Size)
	}
}

func TestGenerateImageHandler_AutoAspectRatioLeavesSizeEmpty(t *testing.T) {
	mockProvider := &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 100,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64},
			},
		},
	}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"prompt":       "A cat",
		"aspect_ratio": aspectRatioAuto,
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if mockProvider.generateReq.Size != "" {
		t.Errorf("auto aspect_ratio should leave size empty, got %q", mockProvider.generateReq.Size)
	}
}

func TestGenerateImageHandler_ExplicitSizeOverridesAspectRatio(t *testing.T) {
	mockProvider := &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 100,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64},
			},
		},
	}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"prompt":       "A cat",
		"size":         "1024x1536",
		"aspect_ratio": "4:3",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if mockProvider.generateReq.Size != "1024x1536" {
		t.Errorf("explicit size must win, got %q", mockProvider.generateReq.Size)
	}
}

func TestGenerateImageHandler_ForwardsOutputFormat(t *testing.T) {
	mockProvider := &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 100,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64},
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
		t.Errorf("output_format = %q, want webp", mockProvider.generateReq.OutputFormat)
	}
}

func TestGenerateImageHandler_RejectsUnsupportedOutputFormat(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"prompt":        "A cat",
		"output_format": "tiff",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected validation error for unsupported output_format")
	}
	if !strings.Contains(result.Content[0].Text, "unsupported output_format") {
		t.Errorf("error should mention unsupported output_format: %s", result.Content[0].Text)
	}
	if mockProvider.generateReq != nil {
		t.Fatal("provider must not be called for invalid output_format")
	}
}

func TestEditImageHandler_AspectRatioAndOutputFormat(t *testing.T) {
	mockProvider := &mockVisionProvider{
		editResult: &provider.EditImageResponse{
			Created: 100,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64},
			},
		},
	}
	handler := EditImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"image":         onePixelPNGBase64,
		"prompt":        "Add a hat",
		"aspect_ratio":  "3:4",
		"output_format": "jpeg",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if mockProvider.editReq == nil {
		t.Fatal("provider was not called")
	}
	if mockProvider.editReq.Size != "1024x1536" {
		t.Errorf("size = %q, want 1024x1536", mockProvider.editReq.Size)
	}
	if mockProvider.editReq.OutputFormat != "jpeg" {
		t.Errorf("output_format = %q, want jpeg", mockProvider.editReq.OutputFormat)
	}
}
