package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AoManoh/openPic-mcp/internal/config"
	"github.com/AoManoh/openPic-mcp/internal/provider"
)

func TestGenerateImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/images/generations" {
			t.Fatalf("path = %s, want /images/generations", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}

		var req ImageGenerationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "gpt-image-1" {
			t.Fatalf("model = %s, want gpt-image-1", req.Model)
		}
		if req.Prompt != "A cat" {
			t.Fatalf("prompt = %s, want A cat", req.Prompt)
		}
		if req.Size != "1024x1024" {
			t.Fatalf("size = %s, want 1024x1024", req.Size)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.com/image.png","revised_prompt":"A cat"}]}`))
	}))
	defer server.Close()

	p := NewProvider(&config.Config{
		APIBaseURL:  server.URL,
		APIKey:      "test-key",
		VisionModel: "gpt-4o",
		ImageModel:  "gpt-image-1",
		Timeout:     5 * time.Second,
	})

	resp, err := p.GenerateImage(context.Background(), &provider.GenerateImageRequest{
		Prompt:         "A cat",
		Size:           "1024x1024",
		ResponseFormat: "url",
		N:              1,
	})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v, want nil", err)
	}
	if resp.Created != 123 {
		t.Fatalf("Created = %d, want 123", resp.Created)
	}
	if len(resp.Images) != 1 {
		t.Fatalf("Images length = %d, want 1", len(resp.Images))
	}
	if resp.Images[0].URL != "https://example.com/image.png" {
		t.Fatalf("URL = %s, want https://example.com/image.png", resp.Images[0].URL)
	}
}

func TestGenerateImageRequiresImageModel(t *testing.T) {
	p := NewProvider(&config.Config{
		APIBaseURL:  "https://api.example.com/v1",
		APIKey:      "test-key",
		VisionModel: "gpt-4o",
		Timeout:     5 * time.Second,
	})

	_, err := p.GenerateImage(context.Background(), &provider.GenerateImageRequest{Prompt: "A cat"})
	if err == nil {
		t.Fatal("GenerateImage() error = nil, want error")
	}
}

func TestEditImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/images/edits" {
			t.Fatalf("path = %s, want /images/edits", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}

		if err := r.ParseMultipartForm(1024); err != nil {
			t.Fatalf("failed to parse multipart request: %v", err)
		}
		if r.FormValue("model") != "gpt-image-1" {
			t.Fatalf("model = %s, want gpt-image-1", r.FormValue("model"))
		}
		if r.FormValue("prompt") != "Add a red hat" {
			t.Fatalf("prompt = %s, want Add a red hat", r.FormValue("prompt"))
		}
		if r.FormValue("size") != "1024x1024" {
			t.Fatalf("size = %s, want 1024x1024", r.FormValue("size"))
		}
		if r.FormValue("response_format") != "url" {
			t.Fatalf("response_format = %s, want url", r.FormValue("response_format"))
		}

		imageFiles := r.MultipartForm.File["image"]
		if len(imageFiles) != 1 {
			t.Fatalf("image files length = %d, want 1", len(imageFiles))
		}
		imageFile, err := imageFiles[0].Open()
		if err != nil {
			t.Fatalf("failed to open image file: %v", err)
		}
		defer imageFile.Close()
		imageData, err := io.ReadAll(imageFile)
		if err != nil {
			t.Fatalf("failed to read image file: %v", err)
		}
		if string(imageData) != "image-data" {
			t.Fatalf("image data = %q, want image-data", string(imageData))
		}

		maskFiles := r.MultipartForm.File["mask"]
		if len(maskFiles) != 1 {
			t.Fatalf("mask files length = %d, want 1", len(maskFiles))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.com/edited.png","revised_prompt":"A cat with a red hat"}]}`))
	}))
	defer server.Close()

	p := NewProvider(&config.Config{
		APIBaseURL:  server.URL,
		APIKey:      "test-key",
		VisionModel: "gpt-4o",
		ImageModel:  "gpt-image-1",
		Timeout:     5 * time.Second,
	})

	resp, err := p.EditImage(context.Background(), &provider.EditImageRequest{
		Image:          []byte("image-data"),
		ImageMediaType: "image/png",
		Mask:           []byte("maskdata"),
		MaskMediaType:  "image/png",
		Prompt:         "Add a red hat",
		Size:           "1024x1024",
		ResponseFormat: "url",
		N:              1,
	})
	if err != nil {
		t.Fatalf("EditImage() error = %v, want nil", err)
	}
	if resp.Created != 123 {
		t.Fatalf("Created = %d, want 123", resp.Created)
	}
	if len(resp.Images) != 1 {
		t.Fatalf("Images length = %d, want 1", len(resp.Images))
	}
	if resp.Images[0].URL != "https://example.com/edited.png" {
		t.Fatalf("URL = %s, want https://example.com/edited.png", resp.Images[0].URL)
	}
}

func TestEditImageRequiresImageModel(t *testing.T) {
	p := NewProvider(&config.Config{
		APIBaseURL:  "https://api.example.com/v1",
		APIKey:      "test-key",
		VisionModel: "gpt-4o",
		Timeout:     5 * time.Second,
	})

	_, err := p.EditImage(context.Background(), &provider.EditImageRequest{
		Image:  []byte("image-data"),
		Prompt: "Add a red hat",
	})
	if err == nil {
		t.Fatal("EditImage() error = nil, want error")
	}
}
