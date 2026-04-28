package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/AoManoh/openPic-mcp/internal/provider"
)

// mockVisionProvider is a mock implementation of VisionProvider for testing.
const onePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

type mockVisionProvider struct {
	analyzeResult  *provider.AnalyzeResponse
	analyzeErr     error
	compareResult  *provider.CompareResponse
	compareErr     error
	generateResult *provider.GenerateImageResponse
	generateErr    error
	generateReq    *provider.GenerateImageRequest
	editResult     *provider.EditImageResponse
	editErr        error
	editReq        *provider.EditImageRequest
}

func (m *mockVisionProvider) Name() string {
	return "mock"
}

func (m *mockVisionProvider) AnalyzeImage(ctx context.Context, req *provider.AnalyzeRequest) (*provider.AnalyzeResponse, error) {
	if m.analyzeErr != nil {
		return nil, m.analyzeErr
	}
	return m.analyzeResult, nil
}

func (m *mockVisionProvider) CompareImages(ctx context.Context, req *provider.CompareRequest) (*provider.CompareResponse, error) {
	if m.compareErr != nil {
		return nil, m.compareErr
	}
	return m.compareResult, nil
}

func (m *mockVisionProvider) GenerateImage(ctx context.Context, req *provider.GenerateImageRequest) (*provider.GenerateImageResponse, error) {
	m.generateReq = req
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	return m.generateResult, nil
}

func (m *mockVisionProvider) EditImage(ctx context.Context, req *provider.EditImageRequest) (*provider.EditImageResponse, error) {
	m.editReq = req
	if m.editErr != nil {
		return nil, m.editErr
	}
	return m.editResult, nil
}

func TestCompareImagesHandler_Success(t *testing.T) {
	mockProvider := &mockVisionProvider{
		compareResult: &provider.CompareResponse{
			Comparison: "Image 1 shows a cat, Image 2 shows a dog. Both are pets.",
			Usage: &provider.Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		},
	}

	handler := CompareImagesHandler(mockProvider)

	args := map[string]any{
		"images": []interface{}{
			"base64_image_1",
			"base64_image_2",
		},
		"prompt": "Compare these two images",
	}

	result, err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	if len(result.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(result.Content))
	}

	if result.Content[0].Text != "Image 1 shows a cat, Image 2 shows a dog. Both are pets." {
		t.Errorf("unexpected comparison result: %s", result.Content[0].Text)
	}
}

func TestCompareImagesHandler_MissingImages(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := CompareImagesHandler(mockProvider)

	args := map[string]any{}

	result, err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for missing images")
	}
}

func TestCompareImagesHandler_TooFewImages(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := CompareImagesHandler(mockProvider)

	args := map[string]any{
		"images": []interface{}{"only_one_image"},
	}

	result, err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for too few images")
	}
}

func TestCompareImagesHandler_TooManyImages(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := CompareImagesHandler(mockProvider)

	args := map[string]any{
		"images": []interface{}{"img1", "img2", "img3", "img4", "img5"},
	}

	result, err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for too many images")
	}
}

func TestCompareImagesHandler_InvalidImageType(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := CompareImagesHandler(mockProvider)

	args := map[string]any{
		"images": []interface{}{123, 456}, // Not strings
	}

	result, err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for invalid image type")
	}
}

func TestCompareImagesHandler_WithDetailLevel(t *testing.T) {
	mockProvider := &mockVisionProvider{
		compareResult: &provider.CompareResponse{
			Comparison: "Detailed comparison result",
		},
	}

	handler := CompareImagesHandler(mockProvider)

	args := map[string]any{
		"images":       []interface{}{"img1", "img2"},
		"detail_level": "detailed",
	}

	result, err := handler(context.Background(), args)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}
}

func TestGenerateImageHandler_Success(t *testing.T) {
	mockProvider := &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 123,
			Images: []provider.GeneratedImage{
				{
					URL:           "https://example.com/image.png",
					RevisedPrompt: "A cat",
				},
			},
		},
	}

	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"prompt":          "A cat",
		"size":            "1024x1024",
		"response_format": "url",
		"n":               float64(1),
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Text == "" {
		t.Fatal("expected non-empty generation result")
	}
}

func TestGenerateImageHandler_MissingPrompt(t *testing.T) {
	handler := GenerateImageHandler(&mockVisionProvider{})

	result, err := handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error for missing prompt")
	}
}

func TestEditImageHandler_Success(t *testing.T) {
	mockProvider := &mockVisionProvider{
		editResult: &provider.EditImageResponse{
			Created: 123,
			Images: []provider.GeneratedImage{
				{
					URL:           "https://example.com/edited.png",
					RevisedPrompt: "A cat with a red hat",
				},
			},
		},
	}

	handler := EditImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"image":           "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		"prompt":          "Add a red hat",
		"size":            "1024x1024",
		"response_format": "url",
		"n":               float64(1),
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Text == "" {
		t.Fatal("expected non-empty editing result")
	}
}

func TestEditImageHandler_MissingImage(t *testing.T) {
	handler := EditImageHandler(&mockVisionProvider{})

	result, err := handler(context.Background(), map[string]any{
		"prompt": "Add a red hat",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error for missing image")
	}
}

func TestEditImageHandler_MissingPrompt(t *testing.T) {
	handler := EditImageHandler(&mockVisionProvider{})

	result, err := handler(context.Background(), map[string]any{
		"image": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error for missing prompt")
	}
}

func TestGenerateImageHandler_DefaultsToFilePath(t *testing.T) {
	mockProvider := &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 123,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64, RevisedPrompt: "A cat"},
			},
		},
	}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{"prompt": "A cat"})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if mockProvider.generateReq == nil {
		t.Fatal("provider was not called")
	}
	if mockProvider.generateReq.Size != defaultImageSize {
		t.Fatalf("size = %q, want %q", mockProvider.generateReq.Size, defaultImageSize)
	}
	// Phase 2 contract: response_format is owned by the tool layer and must
	// no longer leak into provider requests. The compile-time absence of
	// ResponseFormat on GenerateImageRequest enforces this; the surrounding
	// payload assertions confirm the tool-side delivery semantics.
	if mockProvider.generateReq.N != 1 {
		t.Fatalf("n = %d, want 1", mockProvider.generateReq.N)
	}

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if len(payload.Images) != 1 {
		t.Fatalf("images length = %d, want 1", len(payload.Images))
	}
	if payload.Images[0].B64JSON != "" {
		t.Fatal("expected b64_json to be omitted from file_path response")
	}
	if payload.Images[0].FilePath == "" {
		t.Fatal("expected file_path in response")
	}
	defer os.Remove(payload.Images[0].FilePath)
	data, err := os.ReadFile(payload.Images[0].FilePath)
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("generated file is empty")
	}
	if strings.Contains(result.Content[0].Text, onePixelPNGBase64) {
		t.Fatal("result still contains inline base64")
	}
}

func TestGenerateImageHandler_InvalidSize(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{"prompt": "A cat", "size": "16:9"})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid size")
	}
	if !strings.Contains(result.Content[0].Text, "unsupported size") {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if mockProvider.generateReq != nil {
		t.Fatal("provider should not be called for invalid size")
	}
}

func TestGenerateImageHandler_RejectsUnsupportedSizeBeforeProvider(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{"prompt": "A cat", "size": "512x512"})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for unsupported size")
	}
	if !strings.Contains(result.Content[0].Text, "unsupported size") {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if mockProvider.generateReq != nil {
		t.Fatal("provider should not be called for unsupported size")
	}
}

func TestGenerateImageHandler_RejectsMultipleResults(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{"prompt": "A cat", "n": float64(2)})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for n > 1")
	}
	if !strings.Contains(result.Content[0].Text, "supports only n=1") {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if mockProvider.generateReq != nil {
		t.Fatal("provider should not be called for unsupported n")
	}
}

func TestEditImageHandler_DefaultsToFilePath(t *testing.T) {
	mockProvider := &mockVisionProvider{
		editResult: &provider.EditImageResponse{
			Created: 123,
			Images: []provider.GeneratedImage{
				{B64JSON: onePixelPNGBase64, RevisedPrompt: "A cat with a red hat"},
			},
		},
	}
	handler := EditImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{"image": onePixelPNGBase64, "prompt": "Add a red hat"})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if mockProvider.editReq == nil {
		t.Fatal("provider was not called")
	}
	if mockProvider.editReq.Size != defaultImageSize {
		t.Fatalf("size = %q, want %q", mockProvider.editReq.Size, defaultImageSize)
	}
	// Phase 2 contract: response_format is owned by the tool layer.
	if mockProvider.editReq.N != 1 {
		t.Fatalf("n = %d, want 1", mockProvider.editReq.N)
	}

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if len(payload.Images) != 1 {
		t.Fatalf("images length = %d, want 1", len(payload.Images))
	}
	if payload.Images[0].B64JSON != "" {
		t.Fatal("expected b64_json to be omitted from file_path response")
	}
	if payload.Images[0].FilePath == "" {
		t.Fatal("expected file_path in response")
	}
	defer os.Remove(payload.Images[0].FilePath)
}

func TestGenerateImageHandler_URLDataURISavedAsFilePath(t *testing.T) {
	mockProvider := &mockVisionProvider{
		generateResult: &provider.GenerateImageResponse{
			Created: 123,
			Images: []provider.GeneratedImage{
				{URL: "data:image/png;base64," + onePixelPNGBase64},
			},
		},
	}
	handler := GenerateImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{"prompt": "A cat", "response_format": "url"})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if mockProvider.generateReq == nil {
		t.Fatal("provider was not called")
	}
	// Phase 2 contract: requesting response_format=url at the MCP layer must
	// still be transparent to the provider; see the payload assertions below
	// for the user-visible delivery shape.

	var payload imageToolResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &payload); err != nil {
		t.Fatalf("failed to decode result JSON: %v", err)
	}
	if len(payload.Images) != 1 {
		t.Fatalf("images length = %d, want 1", len(payload.Images))
	}
	if payload.Images[0].URL != "" {
		t.Fatalf("expected data URI url to be omitted, got %q", payload.Images[0].URL)
	}
	if payload.Images[0].FilePath == "" {
		t.Fatal("expected file_path in response")
	}
	defer os.Remove(payload.Images[0].FilePath)
	if strings.Contains(result.Content[0].Text, onePixelPNGBase64) {
		t.Fatal("result still contains inline base64")
	}
}

func TestEditImageHandler_RejectsUnsupportedSizeBeforeProvider(t *testing.T) {
	mockProvider := &mockVisionProvider{}
	handler := EditImageHandler(mockProvider)

	result, err := handler(context.Background(), map[string]any{
		"image":  onePixelPNGBase64,
		"prompt": "Edit the image",
		"size":   "512x512",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for unsupported size")
	}
	if !strings.Contains(result.Content[0].Text, "unsupported size") {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if mockProvider.editReq != nil {
		t.Fatal("provider should not be called for unsupported size")
	}
}
