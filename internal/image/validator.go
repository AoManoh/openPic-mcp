// Package image provides image processing utilities for the Vision MCP Server.
package image

import (
	"errors"
	"strings"
)

// Supported image formats.
const (
	FormatJPEG = "jpeg"
	FormatPNG  = "png"
	FormatWebP = "webp"
	FormatGIF  = "gif"
	FormatBMP  = "bmp"
	FormatTIFF = "tiff"
	FormatICO  = "ico"
	FormatHEIC = "heic"
	FormatAVIF = "avif"
	FormatSVG  = "svg"
)

// MIME types for supported formats.
var MIMETypes = map[string]string{
	FormatJPEG: "image/jpeg",
	FormatPNG:  "image/png",
	FormatWebP: "image/webp",
	FormatGIF:  "image/gif",
	FormatBMP:  "image/bmp",
	FormatTIFF: "image/tiff",
	FormatICO:  "image/x-icon",
	FormatHEIC: "image/heic",
	FormatAVIF: "image/avif",
	FormatSVG:  "image/svg+xml",
}

// Magic bytes for format detection.
var magicBytes = map[string][]byte{
	FormatJPEG: {0xFF, 0xD8, 0xFF},
	FormatPNG:  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
	FormatWebP: {0x52, 0x49, 0x46, 0x46}, // "RIFF" - WebP starts with RIFF
	FormatGIF:  {0x47, 0x49, 0x46, 0x38}, // "GIF8" - GIF87a or GIF89a
	FormatBMP:  {0x42, 0x4D},             // "BM"
	FormatICO:  {0x00, 0x00, 0x01, 0x00}, // ICO signature
}

// TIFF magic bytes (can be little-endian or big-endian)
var tiffMagicLE = []byte{0x49, 0x49, 0x2A, 0x00} // "II*\0" - Little-endian
var tiffMagicBE = []byte{0x4D, 0x4D, 0x00, 0x2A} // "MM\0*" - Big-endian

// ErrUnsupportedFormat is returned when the image format is not supported.
var ErrUnsupportedFormat = errors.New("unsupported image format")

// ValidateFormat validates the image format from raw bytes.
// Returns the format name (jpeg, png, webp, gif, etc.) or an error if unsupported.
func ValidateFormat(data []byte) (string, error) {
	if len(data) < 4 {
		return "", ErrUnsupportedFormat
	}

	// Check JPEG (FFD8FF)
	if hasPrefix(data, magicBytes[FormatJPEG]) {
		return FormatJPEG, nil
	}

	// Check PNG (89504E47...)
	if len(data) >= 8 && hasPrefix(data, magicBytes[FormatPNG]) {
		return FormatPNG, nil
	}

	// Check GIF (GIF87a or GIF89a)
	if len(data) >= 6 && hasPrefix(data, magicBytes[FormatGIF]) {
		// Verify it's GIF87a or GIF89a
		if data[4] == 0x37 || data[4] == 0x39 { // '7' or '9'
			if data[5] == 0x61 { // 'a'
				return FormatGIF, nil
			}
		}
	}

	// Check WebP (RIFF....WEBP)
	if len(data) >= 12 && hasPrefix(data, magicBytes[FormatWebP]) {
		if string(data[8:12]) == "WEBP" {
			return FormatWebP, nil
		}
	}

	// Check BMP (BM)
	if hasPrefix(data, magicBytes[FormatBMP]) {
		return FormatBMP, nil
	}

	// Check TIFF (II*\0 or MM\0*)
	if len(data) >= 4 {
		if hasPrefix(data, tiffMagicLE) || hasPrefix(data, tiffMagicBE) {
			return FormatTIFF, nil
		}
	}

	// Check ICO
	if hasPrefix(data, magicBytes[FormatICO]) {
		return FormatICO, nil
	}

	// Check HEIC/HEIF and AVIF (ftyp box)
	if len(data) >= 12 {
		format := detectFtypFormat(data)
		if format != "" {
			return format, nil
		}
	}

	// Check SVG (XML-based)
	if detectSVG(data) {
		return FormatSVG, nil
	}

	return "", ErrUnsupportedFormat
}

// detectFtypFormat detects HEIC/HEIF and AVIF formats from ftyp box.
func detectFtypFormat(data []byte) string {
	// ftyp box: [size:4][ftyp:4][brand:4]...
	if len(data) < 12 {
		return ""
	}

	// Check for ftyp marker at offset 4
	if string(data[4:8]) != "ftyp" {
		return ""
	}

	// Get the brand (major brand at offset 8)
	brand := string(data[8:12])

	// HEIC/HEIF brands
	heicBrands := []string{"heic", "heix", "hevc", "hevx", "mif1", "msf1"}
	for _, b := range heicBrands {
		if brand == b {
			return FormatHEIC
		}
	}

	// AVIF brands
	avifBrands := []string{"avif", "avis", "mif1"}
	for _, b := range avifBrands {
		if brand == b {
			// mif1 could be HEIC or AVIF, check compatible brands
			if brand == "mif1" {
				// Look for avif in compatible brands
				if len(data) >= 16 && containsAVIFBrand(data[12:]) {
					return FormatAVIF
				}
				return FormatHEIC // Default to HEIC for mif1
			}
			return FormatAVIF
		}
	}

	return ""
}

// containsAVIFBrand checks if AVIF brand exists in compatible brands.
func containsAVIFBrand(data []byte) bool {
	// Search for "avif" in the data
	for i := 0; i <= len(data)-4; i++ {
		if string(data[i:i+4]) == "avif" {
			return true
		}
	}
	return false
}

// detectSVG detects SVG format from content.
func detectSVG(data []byte) bool {
	// SVG files typically start with <?xml or <svg or have svg namespace
	// Check first 1KB for SVG indicators
	checkLen := len(data)
	if checkLen > 1024 {
		checkLen = 1024
	}

	content := strings.ToLower(string(data[:checkLen]))

	// Check for SVG indicators
	return strings.Contains(content, "<svg") ||
		strings.Contains(content, "<!doctype svg") ||
		strings.Contains(content, "xmlns=\"http://www.w3.org/2000/svg\"")
}

// GetMIMEType returns the MIME type for a format.
func GetMIMEType(format string) string {
	if mime, ok := MIMETypes[format]; ok {
		return mime
	}
	return "application/octet-stream"
}

// FormatFromMIME returns the format from a MIME type.
func FormatFromMIME(mime string) (string, error) {
	mime = strings.ToLower(strings.TrimSpace(mime))

	// Handle common variations
	switch {
	case strings.Contains(mime, "jpeg") || strings.Contains(mime, "jpg"):
		return FormatJPEG, nil
	case strings.Contains(mime, "png"):
		return FormatPNG, nil
	case strings.Contains(mime, "webp"):
		return FormatWebP, nil
	case strings.Contains(mime, "gif"):
		return FormatGIF, nil
	case strings.Contains(mime, "bmp"):
		return FormatBMP, nil
	case strings.Contains(mime, "tiff") || strings.Contains(mime, "tif"):
		return FormatTIFF, nil
	case strings.Contains(mime, "icon") || strings.Contains(mime, "ico"):
		return FormatICO, nil
	case strings.Contains(mime, "heic") || strings.Contains(mime, "heif"):
		return FormatHEIC, nil
	case strings.Contains(mime, "avif"):
		return FormatAVIF, nil
	case strings.Contains(mime, "svg"):
		return FormatSVG, nil
	default:
		return "", ErrUnsupportedFormat
	}
}

// FormatFromExtension returns the format from a file extension.
func FormatFromExtension(ext string) (string, error) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	switch ext {
	case "jpg", "jpeg", "jpe", "jfif":
		return FormatJPEG, nil
	case "png":
		return FormatPNG, nil
	case "webp":
		return FormatWebP, nil
	case "gif":
		return FormatGIF, nil
	case "bmp", "dib":
		return FormatBMP, nil
	case "tiff", "tif":
		return FormatTIFF, nil
	case "ico":
		return FormatICO, nil
	case "heic", "heif":
		return FormatHEIC, nil
	case "avif":
		return FormatAVIF, nil
	case "svg", "svgz":
		return FormatSVG, nil
	default:
		return "", ErrUnsupportedFormat
	}
}

// IsSupportedFormat checks if a format is supported.
func IsSupportedFormat(format string) bool {
	_, ok := MIMETypes[format]
	return ok
}

// IsSupportedMIMEType checks if a MIME type is supported.
func IsSupportedMIMEType(mimeType string) bool {
	_, err := FormatFromMIME(mimeType)
	return err == nil
}

// IsSupportedExtension checks if a file extension is supported.
func IsSupportedExtension(ext string) bool {
	_, err := FormatFromExtension(ext)
	return err == nil
}

// hasPrefix checks if data starts with prefix.
func hasPrefix(data, prefix []byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i, b := range prefix {
		if data[i] != b {
			return false
		}
	}
	return true
}
