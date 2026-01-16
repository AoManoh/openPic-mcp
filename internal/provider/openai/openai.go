// Package openai provides OpenAI-compatible Vision API implementation.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/AoManoh/openPic-mcp/internal/config"
	"github.com/AoManoh/openPic-mcp/internal/provider"
)

// Provider implements the VisionProvider interface for OpenAI-compatible APIs.
type Provider struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
}

// NewProvider creates a new OpenAI-compatible provider.
func NewProvider(cfg *config.Config) *Provider {
	return &Provider{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		baseURL: strings.TrimSuffix(cfg.APIBaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "openai-compatible"
}

// AnalyzeImage analyzes an image using the OpenAI Vision API.
func (p *Provider) AnalyzeImage(ctx context.Context, req *provider.AnalyzeRequest) (*provider.AnalyzeResponse, error) {
	// Build the request
	chatReq := p.buildChatRequest(req)

	// Marshal request body
	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Send request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract description
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &provider.AnalyzeResponse{
		Description: chatResp.Choices[0].Message.Content,
		Usage: &provider.Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}, nil
}

// buildChatRequest builds the chat completion request.
func (p *Provider) buildChatRequest(req *provider.AnalyzeRequest) *ChatCompletionRequest {
	prompt := provider.GetPrompt(req.DetailLevel, req.Prompt)

	// Build image URL (data URI or regular URL)
	imageURL := req.Image
	if !strings.HasPrefix(req.Image, "http://") && !strings.HasPrefix(req.Image, "https://") {
		// Assume base64 encoded image
		mediaType := req.ImageMediaType
		if mediaType == "" {
			mediaType = "image/jpeg"
		}
		imageURL = fmt.Sprintf("data:%s;base64,%s", mediaType, req.Image)
	}

	return &ChatCompletionRequest{
		Model: p.model,
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentPart{
					{
						Type: "text",
						Text: prompt,
					},
					{
						Type: "image_url",
						ImageURL: &ImageURL{
							URL:    imageURL,
							Detail: getDetailLevel(req.DetailLevel),
						},
					},
				},
			},
		},
		MaxTokens: 4096,
	}
}

// getDetailLevel converts detail level to OpenAI format.
func getDetailLevel(level string) string {
	switch level {
	case "brief":
		return "low"
	case "detailed":
		return "high"
	default:
		return "auto"
	}
}

// CompareImages compares multiple images using the OpenAI Vision API.
func (p *Provider) CompareImages(ctx context.Context, req *provider.CompareRequest) (*provider.CompareResponse, error) {
	// Build the request
	chatReq := p.buildCompareRequest(req)

	// Marshal request body
	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Send request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract comparison result
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &provider.CompareResponse{
		Comparison: chatResp.Choices[0].Message.Content,
		Usage: &provider.Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}, nil
}

// buildCompareRequest builds the chat completion request for image comparison.
func (p *Provider) buildCompareRequest(req *provider.CompareRequest) *ChatCompletionRequest {
	prompt := provider.GetComparePrompt(req.Prompt)

	// Build content parts: text prompt + all images
	contentParts := []ContentPart{
		{
			Type: "text",
			Text: prompt,
		},
	}

	// Add each image
	for i, img := range req.Images {
		imageURL := img.Data
		if !strings.HasPrefix(img.Data, "http://") && !strings.HasPrefix(img.Data, "https://") && !strings.HasPrefix(img.Data, "data:") {
			// Assume base64 encoded image
			mediaType := img.MediaType
			if mediaType == "" {
				mediaType = "image/jpeg"
			}
			imageURL = fmt.Sprintf("data:%s;base64,%s", mediaType, img.Data)
		}

		contentParts = append(contentParts, ContentPart{
			Type: "image_url",
			ImageURL: &ImageURL{
				URL:    imageURL,
				Detail: getDetailLevel(req.DetailLevel),
			},
		})

		// Add image label for clarity
		contentParts = append(contentParts, ContentPart{
			Type: "text",
			Text: fmt.Sprintf("[Image %d above]", i+1),
		})
	}

	return &ChatCompletionRequest{
		Model: p.model,
		Messages: []Message{
			{
				Role:    "user",
				Content: contentParts,
			},
		},
		MaxTokens: 4096,
	}
}
