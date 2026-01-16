// Package image provides enhanced MIME type detection for images.
package image

// FormatInfo contains information about an image format.
type FormatInfo struct {
	Name       string   // Format name (e.g., "jpeg", "png")
	MIMEType   string   // MIME type (e.g., "image/jpeg")
	Extensions []string // File extensions (e.g., ["jpg", "jpeg"])
	MagicBytes [][]byte // Magic byte signatures
	MinBytes   int      // Minimum bytes needed for detection
}

// Supported image formats with full information.
var formats = []FormatInfo{
	{
		Name:       FormatJPEG,
		MIMEType:   "image/jpeg",
		Extensions: []string{"jpg", "jpeg", "jpe", "jfif"},
		MagicBytes: [][]byte{
			{0xFF, 0xD8, 0xFF},
		},
		MinBytes: 3,
	},
	{
		Name:       FormatPNG,
		MIMEType:   "image/png",
		Extensions: []string{"png"},
		MagicBytes: [][]byte{
			{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
		},
		MinBytes: 8,
	},
	{
		Name:       FormatGIF,
		MIMEType:   "image/gif",
		Extensions: []string{"gif"},
		MagicBytes: [][]byte{
			{0x47, 0x49, 0x46, 0x38, 0x37, 0x61}, // GIF87a
			{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}, // GIF89a
		},
		MinBytes: 6,
	},
	{
		Name:       FormatWebP,
		MIMEType:   "image/webp",
		Extensions: []string{"webp"},
		MagicBytes: [][]byte{
			{0x52, 0x49, 0x46, 0x46}, // RIFF (needs additional check for WEBP)
		},
		MinBytes: 12,
	},
	{
		Name:       FormatBMP,
		MIMEType:   "image/bmp",
		Extensions: []string{"bmp", "dib"},
		MagicBytes: [][]byte{
			{0x42, 0x4D}, // BM
		},
		MinBytes: 2,
	},
	{
		Name:       FormatTIFF,
		MIMEType:   "image/tiff",
		Extensions: []string{"tiff", "tif"},
		MagicBytes: [][]byte{
			{0x49, 0x49, 0x2A, 0x00}, // Little-endian
			{0x4D, 0x4D, 0x00, 0x2A}, // Big-endian
		},
		MinBytes: 4,
	},
	{
		Name:       FormatICO,
		MIMEType:   "image/x-icon",
		Extensions: []string{"ico"},
		MagicBytes: [][]byte{
			{0x00, 0x00, 0x01, 0x00}, // ICO
		},
		MinBytes: 4,
	},
	{
		Name:       FormatHEIC,
		MIMEType:   "image/heic",
		Extensions: []string{"heic", "heif"},
		MagicBytes: [][]byte{
			// ftyp box with heic/heif brand (checked specially)
		},
		MinBytes: 12,
	},
	{
		Name:       FormatAVIF,
		MIMEType:   "image/avif",
		Extensions: []string{"avif"},
		MagicBytes: [][]byte{
			// ftyp box with avif brand (checked specially)
		},
		MinBytes: 12,
	},
	{
		Name:       FormatSVG,
		MIMEType:   "image/svg+xml",
		Extensions: []string{"svg", "svgz"},
		MagicBytes: [][]byte{
			// SVG is XML-based, checked specially
		},
		MinBytes: 5,
	},
}

// formatInfoMap provides quick lookup by format name.
var formatInfoMap = make(map[string]*FormatInfo)

// mimeToFormat provides quick lookup from MIME type to format.
var mimeToFormat = make(map[string]string)

// extToFormat provides quick lookup from extension to format.
var extToFormat = make(map[string]string)

func init() {
	for i := range formats {
		f := &formats[i]
		formatInfoMap[f.Name] = f
		mimeToFormat[f.MIMEType] = f.Name
		for _, ext := range f.Extensions {
			extToFormat[ext] = f.Name
		}
	}
	// Add common MIME type variations
	mimeToFormat["image/jpg"] = FormatJPEG
	mimeToFormat["image/x-icon"] = FormatICO
	mimeToFormat["image/vnd.microsoft.icon"] = FormatICO
	mimeToFormat["image/heif"] = FormatHEIC
}

// GetFormatInfo returns detailed information about a format.
func GetFormatInfo(format string) *FormatInfo {
	return formatInfoMap[format]
}

// GetAllFormats returns all supported format information.
func GetAllFormats() []FormatInfo {
	result := make([]FormatInfo, len(formats))
	copy(result, formats)
	return result
}

// GetSupportedMIMETypes returns all supported MIME types.
func GetSupportedMIMETypes() []string {
	types := make([]string, 0, len(formats))
	for _, f := range formats {
		types = append(types, f.MIMEType)
	}
	return types
}

// GetSupportedExtensions returns all supported file extensions.
func GetSupportedExtensions() []string {
	exts := make([]string, 0)
	for _, f := range formats {
		exts = append(exts, f.Extensions...)
	}
	return exts
}
