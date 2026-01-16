package image

import (
	"testing"
)

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{
			name:    "JPEG",
			data:    append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 8)...),
			want:    FormatJPEG,
			wantErr: false,
		},
		{
			name:    "PNG",
			data:    append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 4)...),
			want:    FormatPNG,
			wantErr: false,
		},
		{
			name:    "WebP",
			data:    []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50},
			want:    FormatWebP,
			wantErr: false,
		},
		{
			name:    "GIF87a",
			data:    []byte{0x47, 0x49, 0x46, 0x38, 0x37, 0x61, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:    FormatGIF,
			wantErr: false,
		},
		{
			name:    "GIF89a",
			data:    []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:    FormatGIF,
			wantErr: false,
		},
		{
			name:    "BMP",
			data:    []byte{0x42, 0x4D, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:    FormatBMP,
			wantErr: false,
		},
		{
			name:    "TIFF Little-endian",
			data:    []byte{0x49, 0x49, 0x2A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:    FormatTIFF,
			wantErr: false,
		},
		{
			name:    "TIFF Big-endian",
			data:    []byte{0x4D, 0x4D, 0x00, 0x2A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:    FormatTIFF,
			wantErr: false,
		},
		{
			name:    "ICO",
			data:    []byte{0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:    FormatICO,
			wantErr: false,
		},
		{
			name:    "unsupported format",
			data:    []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B},
			want:    "",
			wantErr: true,
		},
		{
			name:    "too short",
			data:    []byte{0xFF, 0xD8},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateFormat(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFormat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetMIMEType(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{FormatJPEG, "image/jpeg"},
		{FormatPNG, "image/png"},
		{FormatWebP, "image/webp"},
		{"unknown", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			if got := GetMIMEType(tt.format); got != tt.want {
				t.Errorf("GetMIMEType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatFromMIME(t *testing.T) {
	tests := []struct {
		mime    string
		want    string
		wantErr bool
	}{
		{"image/jpeg", FormatJPEG, false},
		{"image/jpg", FormatJPEG, false},
		{"image/png", FormatPNG, false},
		{"image/webp", FormatWebP, false},
		{"IMAGE/JPEG", FormatJPEG, false},
		{"image/gif", FormatGIF, false},
		{"image/bmp", FormatBMP, false},
		{"image/tiff", FormatTIFF, false},
		{"image/x-icon", FormatICO, false},
		{"image/heic", FormatHEIC, false},
		{"image/avif", FormatAVIF, false},
		{"image/svg+xml", FormatSVG, false},
		{"text/plain", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			got, err := FormatFromMIME(tt.mime)
			if (err != nil) != tt.wantErr {
				t.Errorf("FormatFromMIME() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FormatFromMIME() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatFromExtension(t *testing.T) {
	tests := []struct {
		ext     string
		want    string
		wantErr bool
	}{
		{"jpg", FormatJPEG, false},
		{"jpeg", FormatJPEG, false},
		{".jpg", FormatJPEG, false},
		{"png", FormatPNG, false},
		{".PNG", FormatPNG, false},
		{"webp", FormatWebP, false},
		{"gif", FormatGIF, false},
		{"bmp", FormatBMP, false},
		{"tiff", FormatTIFF, false},
		{"tif", FormatTIFF, false},
		{"ico", FormatICO, false},
		{"heic", FormatHEIC, false},
		{"avif", FormatAVIF, false},
		{"svg", FormatSVG, false},
		{"xyz", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got, err := FormatFromExtension(tt.ext)
			if (err != nil) != tt.wantErr {
				t.Errorf("FormatFromExtension() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FormatFromExtension() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSupportedFormat(t *testing.T) {
	tests := []struct {
		format string
		want   bool
	}{
		{FormatJPEG, true},
		{FormatPNG, true},
		{FormatWebP, true},
		{FormatGIF, true},
		{FormatBMP, true},
		{FormatTIFF, true},
		{FormatICO, true},
		{FormatHEIC, true},
		{FormatAVIF, true},
		{FormatSVG, true},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			if got := IsSupportedFormat(tt.format); got != tt.want {
				t.Errorf("IsSupportedFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectSVG(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "SVG with xml declaration",
			data: []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`),
			want: FormatSVG,
		},
		{
			name: "SVG without xml declaration",
			data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"></svg>`),
			want: FormatSVG,
		},
		{
			name: "SVG doctype",
			data: []byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN"><svg></svg>`),
			want: FormatSVG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateFormat(tt.data)
			if err != nil {
				t.Errorf("ValidateFormat() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSupportedMIMEType(t *testing.T) {
	tests := []struct {
		mimeType string
		want     bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"image/webp", true},
		{"image/bmp", true},
		{"image/tiff", true},
		{"image/heic", true},
		{"image/avif", true},
		{"image/svg+xml", true},
		{"text/plain", false},
		{"application/json", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			if got := IsSupportedMIMEType(tt.mimeType); got != tt.want {
				t.Errorf("IsSupportedMIMEType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSupportedExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{"jpg", true},
		{"jpeg", true},
		{"png", true},
		{"gif", true},
		{"webp", true},
		{"bmp", true},
		{"tiff", true},
		{"heic", true},
		{"avif", true},
		{"svg", true},
		{".jpg", true},
		{".PNG", true},
		{"xyz", false},
		{"doc", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if got := IsSupportedExtension(tt.ext); got != tt.want {
				t.Errorf("IsSupportedExtension() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetFormatInfo(t *testing.T) {
	info := GetFormatInfo(FormatJPEG)
	if info == nil {
		t.Fatal("GetFormatInfo(FormatJPEG) returned nil")
	}
	if info.Name != FormatJPEG {
		t.Errorf("info.Name = %q, want %q", info.Name, FormatJPEG)
	}
	if info.MIMEType != "image/jpeg" {
		t.Errorf("info.MIMEType = %q, want %q", info.MIMEType, "image/jpeg")
	}

	// Test unknown format
	info = GetFormatInfo("unknown")
	if info != nil {
		t.Errorf("GetFormatInfo(unknown) = %v, want nil", info)
	}
}

func TestGetAllFormats(t *testing.T) {
	formats := GetAllFormats()
	if len(formats) == 0 {
		t.Error("GetAllFormats() returned empty slice")
	}

	// Check that JPEG is in the list
	found := false
	for _, f := range formats {
		if f.Name == FormatJPEG {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetAllFormats() does not contain JPEG")
	}
}

func TestGetSupportedMIMETypes(t *testing.T) {
	types := GetSupportedMIMETypes()
	if len(types) == 0 {
		t.Error("GetSupportedMIMETypes() returned empty slice")
	}

	// Check that image/jpeg is in the list
	found := false
	for _, m := range types {
		if m == "image/jpeg" {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetSupportedMIMETypes() does not contain image/jpeg")
	}
}

func TestGetSupportedExtensions(t *testing.T) {
	exts := GetSupportedExtensions()
	if len(exts) == 0 {
		t.Error("GetSupportedExtensions() returned empty slice")
	}

	// Check that jpg is in the list
	found := false
	for _, e := range exts {
		if e == "jpg" {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetSupportedExtensions() does not contain jpg")
	}
}
