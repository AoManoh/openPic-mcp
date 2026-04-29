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

	"github.com/AoManoh/openPic-mcp/internal/provider"
)

// EditImage delegates an image edit (with optional mask) to
// /v1/images/edits using a multipart payload.
//
// The upstream multipart deliberately omits response_format; GPT image models
// always return b64_json by default and several OpenAI-compatible proxies
// reject the field outright. See docs/refactor/2026-04-28-decoupling-plan.md
// (Phase 2) for the full rationale.
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
	if req.OutputFormat != "" {
		// Phase 6a: forward the requested output encoding so the upstream
		// emits the desired image bytes directly (png/jpeg/webp).
		if err := writer.WriteField("output_format", req.OutputFormat); err != nil {
			return nil, fmt.Errorf("failed to write output_format field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize multipart request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/images/edits", &body)
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
		Usage:   convertImageUsage(imageResp.Usage),
	}, nil
}

// writeImagePart attaches a binary image (or mask) field to a multipart
// writer using the given MIME type. When mediaType is empty the helper falls
// back to image/png to keep parity with prior behaviour.
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
