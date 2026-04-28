package tools

import (
	"testing"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

func TestToolArraySchemasDeclareItems(t *testing.T) {
	tools := []types.Tool{
		DescribeImageTool,
		CompareImagesTool,
		GenerateImageTool,
		EditImageTool,
		ListImageCapabilitiesTool,
	}

	for _, tool := range tools {
		for name, property := range tool.InputSchema.Properties {
			assertArrayItems(t, tool.Name+".properties."+name, property)
		}
	}
}

func assertArrayItems(t *testing.T, path string, property types.Property) {
	t.Helper()
	if property.Type == "array" {
		if property.Items == nil {
			t.Fatalf("%s is array but items is nil", path)
		}
		assertArrayItems(t, path+".items", *property.Items)
	}
}

func TestCompareImagesToolImagesSchema(t *testing.T) {
	images := CompareImagesTool.InputSchema.Properties["images"]
	if images.Type != "array" {
		t.Fatalf("images type = %q, want array", images.Type)
	}
	if images.Items == nil {
		t.Fatal("images.items must not be nil")
	}
	if images.Items.Type != "string" {
		t.Fatalf("images.items.type = %q, want string", images.Items.Type)
	}
	if images.MinItems != MinImages {
		t.Fatalf("images.minItems = %d, want %d", images.MinItems, MinImages)
	}
	if images.MaxItems != DefaultMaxImages {
		t.Fatalf("images.maxItems = %d, want %d", images.MaxItems, DefaultMaxImages)
	}
}
