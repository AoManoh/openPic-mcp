package openai

import (
	"context"
	"fmt"

	"github.com/AoManoh/openPic-mcp/internal/provider"
)

// GenerateImage delegates a text-to-image request to /v1/images/generations.
//
// The upstream payload deliberately omits response_format; see types.go and
// docs/refactor/2026-04-28-decoupling-plan.md (Phase 2) for the rationale.
// Callers receive base64-encoded image data when the upstream returns it; URL
// return values are forwarded as-is so the tool layer can decide on local
// persistence.
func (p *Provider) GenerateImage(ctx context.Context, req *provider.GenerateImageRequest) (*provider.GenerateImageResponse, error) {
	if p.imageModel == "" {
		return nil, fmt.Errorf("OPENPIC_IMAGE_MODEL is required for image generation")
	}

	imageReq := &ImageGenerationRequest{
		Model:        p.imageModel,
		Prompt:       req.Prompt,
		N:            req.N,
		Size:         req.Size,
		Quality:      req.Quality,
		OutputFormat: req.OutputFormat,
	}

	var imageResp ImageGenerationResponse
	if err := p.doJSON(ctx, "/images/generations", imageReq, &imageResp); err != nil {
		return nil, err
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
