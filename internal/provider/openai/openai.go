// Package openai provides an OpenAI-compatible image and vision provider.
//
// The package is split across several files to keep each concern small and
// independently testable:
//
//   - openai.go         : Provider type, constructor, shared HTTP helpers.
//   - vision.go         : AnalyzeImage and CompareImages (chat-completions).
//   - image_generate.go : GenerateImage (/v1/images/generations).
//   - image_edit.go     : EditImage and writeImagePart (/v1/images/edits).
//   - errors.go         : Error formatting and transient-status detection.
//   - types.go          : Wire types shared by the above files.
//
// See docs/refactor/2026-04-28-decoupling-plan.md (Phase 3) for the rationale.
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
)

// Provider implements both VisionProvider and ImageProvider for any
// OpenAI-compatible upstream that exposes /v1/chat/completions, /v1/images/generations
// and /v1/images/edits. Per-endpoint logic lives in vision.go, image_generate.go
// and image_edit.go respectively; this file only carries shared plumbing.
type Provider struct {
	client     *http.Client
	baseURL    string
	apiKey     string
	model      string
	imageModel string
}

// NewProvider creates a new OpenAI-compatible provider configured from the
// shared application Config. The HTTP client honours cfg.Timeout so that
// long-running upstream calls cannot stall the MCP server indefinitely.
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

// Name returns the provider name surfaced to clients and logs.
func (p *Provider) Name() string {
	return "openai-compatible"
}

// doJSON sends a JSON POST request to the given path and decodes the response
// body into target. It centralises payload marshalling, header injection,
// status-code handling and consistent error formatting so the per-endpoint
// helpers in vision.go and image_generate.go can stay focused on payload
// shaping. EditImage uses its own multipart code path because it needs
// streaming uploads of binary data (see image_edit.go).
func (p *Provider) doJSON(ctx context.Context, path string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return formatAPIError(resp.StatusCode, respBody)
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, target); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	return nil
}
