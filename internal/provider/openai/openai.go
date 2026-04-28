// Package openai provides OpenAI-compatible Vision API implementation.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/AoManoh/openPic-mcp/internal/config"
	"github.com/AoManoh/openPic-mcp/internal/provider"
)

const maxAPIErrorBodySnippet = 2048

// Provider implements the VisionProvider interface for OpenAI-compatible APIs.
type Provider struct {
	client     *http.Client
	baseURL    string
	apiKey     string
	model      string
	imageModel string
}

// NewProvider creates a new OpenAI-compatible provider.
func NewProvider(cfg *config.Config) *Provider {
	return &Provider{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		baseURL:    strings.TrimSuffix(cfg.APIBaseURL, "/"),
		apiKey:     cfg.APIKey,
		model:      cfg.VisionModel,
		imageModel: cfg.ImageModel,
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
		return nil, formatAPIError(resp.StatusCode, respBody)
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
		return nil, formatAPIError(resp.StatusCode, respBody)
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

func (p *Provider) GenerateImage(ctx context.Context, req *provider.GenerateImageRequest) (*provider.GenerateImageResponse, error) {
	if p.imageModel == "" {
		return nil, fmt.Errorf("OPENPIC_IMAGE_MODEL is required for image generation")
	}

	imageReq := &ImageGenerationRequest{
		Model:          p.imageModel,
		Prompt:         req.Prompt,
		N:              req.N,
		Size:           req.Size,
		Quality:        req.Quality,
		ResponseFormat: req.ResponseFormat,
	}

	body, err := json.Marshal(imageReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, formatAPIError(resp.StatusCode, respBody)
	}

	var imageResp ImageGenerationResponse
	if err := json.Unmarshal(respBody, &imageResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	images := make([]provider.GeneratedImage, 0, len(imageResp.Data))
	for _, item := range imageResp.Data {
		images = append(images, provider.GeneratedImage{
			URL:           item.URL,
			B64JSON:       item.B64JSON,
			RevisedPrompt: item.RevisedPrompt,
		})
	}

	return &provider.GenerateImageResponse{
		Images:  images,
		Created: imageResp.Created,
	}, nil
}

func (p *Provider) EditImage(ctx context.Context, req *provider.EditImageRequest) (*provider.EditImageResponse, error) {
	if p.imageModel == "" {
		return nil, fmt.Errorf("OPENPIC_IMAGE_MODEL is required for image editing")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("model", p.imageModel); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}
	if err := writer.WriteField("prompt", req.Prompt); err != nil {
		return nil, fmt.Errorf("failed to write prompt field: %w", err)
	}
	if err := writeImagePart(writer, "image", "image", req.ImageMediaType, req.Image); err != nil {
		return nil, err
	}
	if len(req.Mask) > 0 {
		if err := writeImagePart(writer, "mask", "mask", req.MaskMediaType, req.Mask); err != nil {
			return nil, err
		}
	}
	if req.N > 0 {
		if err := writer.WriteField("n", fmt.Sprintf("%d", req.N)); err != nil {
			return nil, fmt.Errorf("failed to write n field: %w", err)
		}
	}
	if req.Size != "" {
		if err := writer.WriteField("size", req.Size); err != nil {
			return nil, fmt.Errorf("failed to write size field: %w", err)
		}
	}
	if req.Quality != "" {
		if err := writer.WriteField("quality", req.Quality); err != nil {
			return nil, fmt.Errorf("failed to write quality field: %w", err)
		}
	}
	if req.ResponseFormat != "" {
		if err := writer.WriteField("response_format", req.ResponseFormat); err != nil {
			return nil, fmt.Errorf("failed to write response_format field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize multipart request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/images/edits", &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, formatAPIError(resp.StatusCode, respBody)
	}

	var imageResp ImageGenerationResponse
	if err := json.Unmarshal(respBody, &imageResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	images := make([]provider.GeneratedImage, 0, len(imageResp.Data))
	for _, item := range imageResp.Data {
		images = append(images, provider.GeneratedImage{
			URL:           item.URL,
			B64JSON:       item.B64JSON,
			RevisedPrompt: item.RevisedPrompt,
		})
	}

	return &provider.EditImageResponse{
		Images:  images,
		Created: imageResp.Created,
	}, nil
}

func writeImagePart(writer *multipart.Writer, fieldName string, fileName string, mediaType string, data []byte) error {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileName))
	if mediaType == "" {
		mediaType = "image/png"
	}
	header.Set("Content-Type", mediaType)

	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("failed to create %s part: %w", fieldName, err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("failed to write %s part: %w", fieldName, err)
	}
	return nil
}

func formatAPIError(statusCode int, respBody []byte) error {
	var errResp ErrorResponse
	if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
		parts := []string{errResp.Error.Message}
		if errResp.Error.Type != "" {
			parts = append(parts, "type="+errResp.Error.Type)
		}
		if errResp.Error.Code != "" {
			parts = append(parts, "code="+errResp.Error.Code)
		}
		if isTemporaryUpstreamStatus(statusCode) {
			parts = append(parts, "hint=upstream service is temporarily unavailable; retry later or check provider logs")
		}
		return fmt.Errorf("API error: status %d: %s", statusCode, strings.Join(parts, ", "))
	}
	parts := []string{"body: " + truncateAPIErrorBody(string(respBody))}
	if isTemporaryUpstreamStatus(statusCode) {
		parts = append(parts, "hint=upstream service is temporarily unavailable; retry later or check provider logs")
	}
	return fmt.Errorf("API error: status %d, %s", statusCode, strings.Join(parts, ", "))
}

func truncateAPIErrorBody(body string) string {
	body = strings.TrimSpace(body)
	if len(body) <= maxAPIErrorBodySnippet {
		return body
	}
	return body[:maxAPIErrorBodySnippet] + "...(truncated)"
}

func isTemporaryUpstreamStatus(statusCode int) bool {
	return statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout
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
