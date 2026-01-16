package image

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsLocalFilePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Unix paths
		{"Unix absolute path", "/home/user/image.png", true},
		{"Unix relative path ./", "./images/test.jpg", true},
		{"Unix relative path ../", "../images/test.jpg", true},

		// Windows paths
		{"Windows drive C:\\", "C:\\Users\\test\\image.png", true},
		{"Windows drive D:/", "D:/images/test.jpg", true},
		{"Windows UNC path", "\\\\server\\share\\image.png", true},
		{"Windows relative .\\", ".\\images\\test.jpg", true},
		{"Windows relative ..\\", "..\\images\\test.jpg", true},

		// Non-local paths
		{"HTTP URL", "http://example.com/image.png", false},
		{"HTTPS URL", "https://example.com/image.png", false},
		{"Data URI", "data:image/png;base64,iVBORw0KGgo=", false},
		{"Raw base64", "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", false},
		{"Empty string", "", false},
		{"Simple filename", "image.png", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLocalFilePath(tt.input)
			if result != tt.expected {
				t.Errorf("isLocalFilePath(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsLocalFilePath_Exported(t *testing.T) {
	// Test the exported version
	if !IsLocalFilePath("/home/user/image.png") {
		t.Error("IsLocalFilePath should return true for Unix absolute path")
	}
	if IsLocalFilePath("http://example.com/image.png") {
		t.Error("IsLocalFilePath should return false for HTTP URL")
	}
}

func TestGetMIMETypeFromExtension(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected string
	}{
		{"JPEG .jpg", "image.jpg", "image/jpeg"},
		{"JPEG .jpeg", "image.jpeg", "image/jpeg"},
		{"PNG", "image.png", "image/png"},
		{"GIF", "image.gif", "image/gif"},
		{"WebP", "image.webp", "image/webp"},
		{"BMP", "image.bmp", "image/bmp"},
		{"TIFF .tiff", "image.tiff", "image/tiff"},
		{"TIFF .tif", "image.tif", "image/tiff"},
		{"SVG", "image.svg", "image/svg+xml"},
		{"Unknown", "image.xyz", ""},
		{"Uppercase", "IMAGE.PNG", "image/png"},
		{"Path with directory", "/path/to/image.jpg", "image/jpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMIMETypeFromExtension(tt.filePath)
			if result != tt.expected {
				t.Errorf("getMIMETypeFromExtension(%q) = %q, want %q", tt.filePath, result, tt.expected)
			}
		})
	}
}

func TestReadLocalFile(t *testing.T) {
	// Create a temporary test image file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.png")

	// PNG file header (minimal valid PNG)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE,
	}

	if err := os.WriteFile(testFile, pngData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	encoder := NewEncoder()

	t.Run("Read existing file", func(t *testing.T) {
		data, mimeType, err := encoder.readLocalFile(testFile)
		if err != nil {
			t.Errorf("readLocalFile() error = %v", err)
			return
		}
		if len(data) != len(pngData) {
			t.Errorf("readLocalFile() data length = %d, want %d", len(data), len(pngData))
		}
		if mimeType != "image/png" {
			t.Errorf("readLocalFile() mimeType = %q, want %q", mimeType, "image/png")
		}
	})

	t.Run("Read non-existent file", func(t *testing.T) {
		_, _, err := encoder.readLocalFile(filepath.Join(tmpDir, "nonexistent.png"))
		if err == nil {
			t.Error("readLocalFile() should return error for non-existent file")
		}
	})
}

func TestDecodeInput_LocalFile(t *testing.T) {
	// Create a temporary test image file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.jpg")

	// JPEG file header (minimal)
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	if err := os.WriteFile(testFile, jpegData, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	encoder := NewEncoder()

	data, mimeType, err := encoder.DecodeInput(testFile)
	if err != nil {
		t.Errorf("DecodeInput() error = %v", err)
		return
	}
	if len(data) != len(jpegData) {
		t.Errorf("DecodeInput() data length = %d, want %d", len(data), len(jpegData))
	}
	if mimeType != "image/jpeg" {
		t.Errorf("DecodeInput() mimeType = %q, want %q", mimeType, "image/jpeg")
	}
}
