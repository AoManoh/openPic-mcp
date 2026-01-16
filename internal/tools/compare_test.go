package tools

import (
	"context"
	"testing"

	"github.com/anthropic/vision-mcp-server/internal/provider"
)

// mockVisionProvider is a mock implementation of VisionProvider for testing.
type mockVisionProvider struct {
	analyzeResult *provider.AnalyzeResponse
	analyzeErr    error
	compareResult *provider.CompareResponse
	compareErr    error
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
