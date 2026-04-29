package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AoManoh/openPic-mcp/internal/config"
	"github.com/AoManoh/openPic-mcp/internal/provider"
)

// slowProviderServer returns an httptest server that holds the response open
// until release is closed (or ctx is cancelled by the client). It lets the
// test verify that ctx cancellation propagates from the caller through to
// http.Client.Do, rather than waiting for the http.Client.Timeout to expire.
func slowProviderServer(t *testing.T, release <-chan struct{}) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			// Client disconnected. Returning here lets net/http close the
			// connection; on the client side this surfaces as a context error.
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func freshProvider(baseURL string) *Provider {
	return NewProvider(&config.Config{
		APIBaseURL:  baseURL,
		APIKey:      "test-key",
		VisionModel: "gpt-4o",
		ImageModel:  "gpt-image-1",
		// Use a generous client timeout so the test can only fail if ctx
		// cancellation is actually wired through; a tight timeout would mask
		// regressions by aborting the call for the wrong reason.
		Timeout: 30 * time.Second,
	})
}

// TestProvider_CancellationPropagation verifies that every ctx-bearing
// provider entry point returns context.Canceled promptly when the caller's
// context is cancelled mid-request. This is the regression net for the
// concurrent-dispatch refactor: once the server engine starts handing
// per-request ctx down through the tool layer, an in-flight upstream call
// must be abortable without waiting for the (possibly minute-long) HTTP
// timeout to expire.
func TestProvider_CancellationPropagation(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, p *Provider) error
	}{
		{
			name: "AnalyzeImage",
			call: func(ctx context.Context, p *Provider) error {
				_, err := p.AnalyzeImage(ctx, &provider.AnalyzeRequest{
					Image:       "https://example.com/cat.png",
					DetailLevel: "normal",
				})
				return err
			},
		},
		{
			name: "CompareImages",
			call: func(ctx context.Context, p *Provider) error {
				_, err := p.CompareImages(ctx, &provider.CompareRequest{
					Images: []provider.ImageInput{
						{Data: "https://example.com/a.png"},
						{Data: "https://example.com/b.png"},
					},
				})
				return err
			},
		},
		{
			name: "GenerateImage",
			call: func(ctx context.Context, p *Provider) error {
				_, err := p.GenerateImage(ctx, &provider.GenerateImageRequest{
					Prompt: "A cat",
					Size:   "1024x1024",
					N:      1,
				})
				return err
			},
		},
		{
			name: "EditImage",
			call: func(ctx context.Context, p *Provider) error {
				_, err := p.EditImage(ctx, &provider.EditImageRequest{
					Image:          []byte("data"),
					ImageMediaType: "image/png",
					Prompt:         "Add a hat",
					Size:           "1024x1024",
					N:              1,
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release := make(chan struct{})
			defer close(release)
			srv := slowProviderServer(t, release)
			p := freshProvider(srv.URL)

			ctx, cancel := context.WithCancel(context.Background())

			done := make(chan error, 1)
			go func() {
				done <- tt.call(ctx, p)
			}()

			// Give the request time to land at the slow server before we
			// cancel; otherwise we'd be racing against connection setup
			// and the test would not actually exercise the inflight path.
			time.Sleep(50 * time.Millisecond)
			cancel()

			select {
			case err := <-done:
				if err == nil {
					t.Fatal("expected error after ctx cancellation, got nil")
				}
				// net/http wraps ctx errors inside *url.Error; errors.Is
				// unwraps that automatically.
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("err = %v, want context.Canceled in chain", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("call did not return within 2s after ctx cancellation; ctx is not propagated to the underlying http.Request")
			}
		})
	}
}
