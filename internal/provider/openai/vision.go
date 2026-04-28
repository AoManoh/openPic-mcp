package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/AoManoh/openPic-mcp/internal/provider"
)

// AnalyzeImage analyzes a single image and returns a textual description.
// The image data may be a remote URL or a base64 string; in the latter case
// the helper materializes a data URI before forwarding it to the upstream
// chat-completions endpoint.
func (p *Provider) AnalyzeImage(ctx context.Context, req *provider.AnalyzeRequest) (*provider.AnalyzeResponse, error) {
	chatReq := p.buildChatRequest(req)

	var chatResp ChatCompletionResponse
	if err := p.doJSON(ctx, "/chat/completions", chatReq, &chatResp); err != nil {
		return nil, err
	}
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

// CompareImages submits a single chat-completions call carrying multiple
// images plus an instruction prompt and returns a comparison narrative.
func (p *Provider) CompareImages(ctx context.Context, req *provider.CompareRequest) (*provider.CompareResponse, error) {
	chatReq := p.buildCompareRequest(req)

	var chatResp ChatCompletionResponse
	if err := p.doJSON(ctx, "/chat/completions", chatReq, &chatResp); err != nil {
		return nil, err
	}
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

// buildChatRequest assembles a chat-completions request for AnalyzeImage.
func (p *Provider) buildChatRequest(req *provider.AnalyzeRequest) *ChatCompletionRequest {
	prompt := provider.GetPrompt(req.DetailLevel, req.Prompt)

	imageURL := req.Image
	if !strings.HasPrefix(req.Image, "http://") && !strings.HasPrefix(req.Image, "https://") {
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
					{Type: "text", Text: prompt},
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

// buildCompareRequest assembles a chat-completions request that contains
// multiple image parts plus textual labels so the model can correlate each
// image with its position in the response.
func (p *Provider) buildCompareRequest(req *provider.CompareRequest) *ChatCompletionRequest {
	prompt := provider.GetComparePrompt(req.Prompt)

	contentParts := []ContentPart{
		{Type: "text", Text: prompt},
	}

	for i, img := range req.Images {
		imageURL := img.Data
		if !strings.HasPrefix(img.Data, "http://") &&
			!strings.HasPrefix(img.Data, "https://") &&
			!strings.HasPrefix(img.Data, "data:") {
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
		contentParts = append(contentParts, ContentPart{
			Type: "text",
			Text: fmt.Sprintf("[Image %d above]", i+1),
		})
	}

	return &ChatCompletionRequest{
		Model: p.model,
		Messages: []Message{
			{Role: "user", Content: contentParts},
		},
		MaxTokens: 4096,
	}
}

// getDetailLevel maps the domain-level detail label onto the values accepted
// by the OpenAI vision API's image_url.detail field.
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
