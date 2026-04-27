package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	imageutil "github.com/AoManoh/openPic-mcp/internal/image"
	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

type imageToolResponse struct {
	Images  []provider.GeneratedImage `json:"images"`
	Created int64                     `json:"created"`
}

func imageToolResult(images []provider.GeneratedImage, created int64, outputFormat string, filePrefix string) (*types.ToolCallResult, error) {
	resultImages := make([]provider.GeneratedImage, len(images))
	copy(resultImages, images)

	if outputFormat == "file_path" {
		for i := range resultImages {
			if resultImages[i].B64JSON == "" {
				continue
			}
			path, err := saveBase64Image(resultImages[i].B64JSON, filePrefix)
			if err != nil {
				return nil, err
			}
			resultImages[i].FilePath = path
			resultImages[i].B64JSON = ""
		}
	}

	payload := imageToolResponse{
		Images:  resultImages,
		Created: created,
	}
	resultJSON, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}

	return &types.ToolCallResult{
		Content: []types.ContentItem{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
		IsError: false,
	}, nil
}

func saveBase64Image(encoded string, filePrefix string) (string, error) {
	data, mimeType, err := decodeGeneratedImage(encoded)
	if err != nil {
		return "", err
	}

	ext := extensionForImageData(data, mimeType)
	dir := filepath.Join(os.TempDir(), "openpic-mcp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create image output directory: %w", err)
	}

	file, err := os.CreateTemp(dir, filePrefix+"-*."+ext)
	if err != nil {
		return "", fmt.Errorf("failed to create image output file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("failed to write image output file: %w", err)
	}

	return file.Name(), nil
}

func decodeGeneratedImage(encoded string) ([]byte, string, error) {
	encoded = strings.TrimSpace(encoded)
	if imageutil.IsDataURI(encoded) {
		data, mimeType, err := imageutil.NewEncoder().DecodeInput(encoded)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decode generated image data URI: %w", err)
		}
		return data, mimeType, nil
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(encoded)
	}
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode generated image b64_json: %w", err)
	}

	mimeType := ""
	if format, err := imageutil.ValidateFormat(data); err == nil {
		mimeType = imageutil.GetMIMEType(format)
	}
	return data, mimeType, nil
}

func extensionForImageData(data []byte, mimeType string) string {
	if format, err := imageutil.FormatFromMIME(mimeType); err == nil {
		return extensionForFormat(format)
	}
	if format, err := imageutil.ValidateFormat(data); err == nil {
		return extensionForFormat(format)
	}
	return "png"
}

func extensionForFormat(format string) string {
	switch format {
	case imageutil.FormatJPEG:
		return "jpg"
	case imageutil.FormatTIFF:
		return "tiff"
	case imageutil.FormatICO:
		return "ico"
	case imageutil.FormatPNG, imageutil.FormatWebP, imageutil.FormatGIF, imageutil.FormatBMP, imageutil.FormatHEIC, imageutil.FormatAVIF, imageutil.FormatSVG:
		return format
	default:
		return "png"
	}
}
