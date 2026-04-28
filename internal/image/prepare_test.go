package image

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestPNG(t *testing.T, dir string, name string) (string, []byte) {
	t.Helper()
	data := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test png: %v", err)
	}
	return path, data
}

func TestEncoder_PrepareForVision_URL(t *testing.T) {
	encoder := NewEncoder()
	got, err := encoder.PrepareForVision("https://example.com/cat.png")
	if err != nil {
		t.Fatalf("PrepareForVision(URL) returned error: %v", err)
	}
	if got.Data != "https://example.com/cat.png" {
		t.Errorf("URL data not preserved: got %q", got.Data)
	}
	if got.MediaType != "" {
		t.Errorf("URL MediaType should be empty, got %q", got.MediaType)
	}
}

func TestEncoder_PrepareForVision_LocalFile(t *testing.T) {
	dir := t.TempDir()
	path, raw := writeTestPNG(t, dir, "image.png")

	encoder := NewEncoder()
	got, err := encoder.PrepareForVision(path)
	if err != nil {
		t.Fatalf("PrepareForVision(local) returned error: %v", err)
	}
	if got.MediaType != "image/png" {
		t.Errorf("expected image/png, got %q", got.MediaType)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.Data)
	if err != nil {
		t.Fatalf("returned data not base64: %v", err)
	}
	if len(decoded) != len(raw) {
		t.Errorf("decoded length mismatch: got %d, want %d", len(decoded), len(raw))
	}
}

func TestEncoder_PrepareForVision_LocalFileMissing(t *testing.T) {
	encoder := NewEncoder()
	_, err := encoder.PrepareForVision("/this/path/should/not/exist.png")
	if err == nil {
		t.Fatal("expected error for missing local file, got nil")
	}
	if !strings.Contains(err.Error(), "read local file") {
		t.Errorf("error should mention read local file: %v", err)
	}
}

func TestEncoder_PrepareForVision_DataURI(t *testing.T) {
	raw := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	dataURI := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)

	encoder := NewEncoder()
	got, err := encoder.PrepareForVision(dataURI)
	if err != nil {
		t.Fatalf("PrepareForVision(dataURI) returned error: %v", err)
	}
	if got.MediaType != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %q", got.MediaType)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.Data)
	if err != nil {
		t.Fatalf("returned data not base64: %v", err)
	}
	if len(decoded) != len(raw) {
		t.Errorf("decoded length mismatch: got %d, want %d", len(decoded), len(raw))
	}
}

func TestEncoder_PrepareForVision_DataURIInvalid(t *testing.T) {
	encoder := NewEncoder()
	_, err := encoder.PrepareForVision("data:image/png;base64")
	if err == nil {
		t.Fatal("expected error for malformed data URI, got nil")
	}
	if !strings.Contains(err.Error(), "decode data URI") {
		t.Errorf("error should mention decode data URI: %v", err)
	}
}

func TestEncoder_PrepareForVision_RawBase64WithMagic(t *testing.T) {
	raw := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}
	encoded := base64.StdEncoding.EncodeToString(raw)

	encoder := NewEncoder()
	got, err := encoder.PrepareForVision(encoded)
	if err != nil {
		t.Fatalf("PrepareForVision(raw base64) returned error: %v", err)
	}
	if got.MediaType != "image/png" {
		t.Errorf("expected image/png, got %q", got.MediaType)
	}
	if got.Data == "" {
		t.Fatal("data must not be empty")
	}
	if _, err := base64.StdEncoding.DecodeString(got.Data); err != nil {
		t.Errorf("returned data should be base64: %v", err)
	}
}

func TestEncoder_PrepareForVision_RawBase64Fallback(t *testing.T) {
	// "@@@@" cannot be decoded by any base64 alphabet, simulating callers
	// that pass a payload our detector cannot understand.
	encoder := NewEncoder()
	got, err := encoder.PrepareForVision("@@@@")
	if err != nil {
		t.Fatalf("PrepareForVision should fall back, got error: %v", err)
	}
	if got.Data != "@@@@" {
		t.Errorf("fallback should preserve input: got %q", got.Data)
	}
	if got.MediaType != "" {
		t.Errorf("fallback MediaType should be empty, got %q", got.MediaType)
	}
}

func TestEncoder_PrepareForVision_DownloadedURLNotInvoked(t *testing.T) {
	// PrepareForVision must NOT fetch URLs; ensure the test server is never hit.
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	encoder := NewEncoder()
	got, err := encoder.PrepareForVision(server.URL + "/foo.png")
	if err != nil {
		t.Fatalf("PrepareForVision(URL) returned error: %v", err)
	}
	if got.Data != server.URL+"/foo.png" {
		t.Errorf("URL data not preserved: got %q", got.Data)
	}
	if hits != 0 {
		t.Errorf("URL must not be fetched during PrepareForVision, got %d hits", hits)
	}
}
