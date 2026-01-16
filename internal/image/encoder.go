package image

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DataURIPrefix is the prefix for data URIs.
const DataURIPrefix = "data:"

// Encoder handles image encoding and decoding operations.
type Encoder struct {
	client *http.Client
}

// NewEncoder creates a new Encoder with default settings.
func NewEncoder() *Encoder {
	return &Encoder{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewEncoderWithClient creates a new Encoder with a custom HTTP client.
func NewEncoderWithClient(client *http.Client) *Encoder {
	return &Encoder{
		client: client,
	}
}

// DecodeInput decodes image input from various formats.
// Supports: base64 string, data URI, HTTP/HTTPS URL, local file path.
// Returns the raw image bytes and detected MIME type.
func (e *Encoder) DecodeInput(input string) ([]byte, string, error) {
	input = strings.TrimSpace(input)

	// Check if it's a data URI
	if strings.HasPrefix(input, DataURIPrefix) {
		return e.decodeDataURI(input)
	}

	// Check if it's a URL
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return e.downloadImage(input)
	}

	// Check if it's a local file path
	if isLocalFilePath(input) {
		return e.readLocalFile(input)
	}

	// Assume it's raw base64
	return e.decodeBase64(input)
}

// decodeDataURI decodes a data URI.
// Format: data:[<mediatype>][;base64],<data>
func (e *Encoder) decodeDataURI(uri string) ([]byte, string, error) {
	// Remove the "data:" prefix
	uri = strings.TrimPrefix(uri, DataURIPrefix)

	// Find the comma separator
	commaIdx := strings.Index(uri, ",")
	if commaIdx == -1 {
		return nil, "", fmt.Errorf("invalid data URI: missing comma separator")
	}

	// Parse metadata
	metadata := uri[:commaIdx]
	data := uri[commaIdx+1:]

	// Extract MIME type
	mimeType := "application/octet-stream"
	isBase64 := false

	parts := strings.Split(metadata, ";")
	for i, part := range parts {
		if i == 0 && part != "" {
			mimeType = part
		} else if part == "base64" {
			isBase64 = true
		}
	}

	// Decode the data
	var decoded []byte
	var err error

	if isBase64 {
		decoded, err = base64.StdEncoding.DecodeString(data)
	} else {
		// URL-encoded data (not common for images)
		decoded = []byte(data)
	}

	if err != nil {
		return nil, "", fmt.Errorf("failed to decode data URI: %w", err)
	}

	return decoded, mimeType, nil
}

// decodeBase64 decodes a raw base64 string.
func (e *Encoder) decodeBase64(data string) ([]byte, string, error) {
	// Try standard base64 first
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		// Try URL-safe base64
		decoded, err = base64.URLEncoding.DecodeString(data)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decode base64: %w", err)
		}
	}

	// Detect format from decoded data
	format, err := ValidateFormat(decoded)
	if err != nil {
		return decoded, "", nil // Return data even if format unknown
	}

	return decoded, GetMIMEType(format), nil
}

// downloadImage downloads an image from a URL.
func (e *Encoder) downloadImage(url string) ([]byte, string, error) {
	resp, err := e.client.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	// Read the body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image data: %w", err)
	}

	// Get MIME type from Content-Type header or detect from data
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		format, _ := ValidateFormat(data)
		mimeType = GetMIMEType(format)
	}

	return data, mimeType, nil
}

// EncodeToBase64 encodes image data to base64.
func EncodeToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// EncodeToDataURI encodes image data to a data URI.
func EncodeToDataURI(data []byte, mimeType string) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
}

// IsDataURI checks if a string is a data URI.
func IsDataURI(s string) bool {
	return strings.HasPrefix(s, DataURIPrefix)
}

// IsURL checks if a string is an HTTP/HTTPS URL.
func IsURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// windowsDrivePattern matches Windows drive letter paths (e.g., "C:\", "D:/")
var windowsDrivePattern = regexp.MustCompile(`^[a-zA-Z]:[\\/]`)

// isLocalFilePath checks if the given string is a local file path.
// Supports Unix paths (/, ./, ../) and Windows paths (C:\, D:/, UNC paths).
func isLocalFilePath(filePath string) bool {
	// Unix/Linux absolute paths
	if strings.HasPrefix(filePath, "/") {
		return true
	}

	// Unix/Linux relative paths
	if strings.HasPrefix(filePath, "./") || strings.HasPrefix(filePath, "../") {
		return true
	}

	// Windows drive letter paths (e.g., "C:\", "D:/")
	if windowsDrivePattern.MatchString(filePath) {
		return true
	}

	// Windows UNC paths (e.g., "\\server\share")
	if strings.HasPrefix(filePath, "\\\\") {
		return true
	}

	// Windows relative paths with backslashes
	if strings.HasPrefix(filePath, ".\\") || strings.HasPrefix(filePath, "..\\") {
		return true
	}

	return false
}

// IsLocalFilePath checks if the given string is a local file path (exported version).
func IsLocalFilePath(filePath string) bool {
	return isLocalFilePath(filePath)
}

// readLocalFile reads an image from a local file path.
func (e *Encoder) readLocalFile(filePath string) ([]byte, string, error) {
	// Normalize the file path for cross-platform compatibility
	normalizedPath := filepath.Clean(filePath)

	// Check if file exists
	if _, err := os.Stat(normalizedPath); os.IsNotExist(err) {
		return nil, "", fmt.Errorf("file not found: %s", normalizedPath)
	}

	// Read the file
	data, err := os.ReadFile(normalizedPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read local file: %w", err)
	}

	// Detect MIME type from file content
	format, err := ValidateFormat(data)
	if err != nil {
		// Try to detect from file extension
		mimeType := getMIMETypeFromExtension(normalizedPath)
		if mimeType != "" {
			return data, mimeType, nil
		}
		return data, "", nil // Return data even if format unknown
	}

	return data, GetMIMEType(format), nil
}

// getMIMETypeFromExtension returns the MIME type based on file extension.
func getMIMETypeFromExtension(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	case ".svg":
		return "image/svg+xml"
	default:
		return ""
	}
}
