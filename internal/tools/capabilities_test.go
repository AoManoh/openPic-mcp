package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestListImageCapabilitiesHandler_ReturnsKnownEnums(t *testing.T) {
	handler := ListImageCapabilitiesHandler()

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("result content must not be empty")
	}

	var caps ImageCapabilities
	if err := json.Unmarshal([]byte(result.Content[0].Text), &caps); err != nil {
		t.Fatalf("failed to decode capabilities JSON: %v", err)
	}

	if len(caps.Sizes) == 0 {
		t.Error("sizes must not be empty")
	}
	for _, want := range []string{"1024x1024", "1024x1536", "1536x1024"} {
		if !containsString(caps.Sizes, want) {
			t.Errorf("sizes missing %q, got %v", want, caps.Sizes)
		}
	}

	for _, want := range []string{aspectRatio1To1, aspectRatio4To3, aspectRatio3To4, aspectRatioAuto} {
		if !containsString(caps.AspectRatios, want) {
			t.Errorf("aspect_ratios missing %q, got %v", want, caps.AspectRatios)
		}
	}

	if caps.AspectRatioToSize[aspectRatio1To1] != "1024x1024" {
		t.Errorf("aspect_ratio mapping mismatch for 1:1: got %q", caps.AspectRatioToSize[aspectRatio1To1])
	}
	if caps.AspectRatioToSize[aspectRatioAuto] != "" {
		t.Errorf("aspect_ratio mapping for auto must be empty, got %q", caps.AspectRatioToSize[aspectRatioAuto])
	}

	for _, want := range []string{"png", "jpeg", "webp"} {
		if !containsString(caps.OutputFormats, want) {
			t.Errorf("output_formats missing %q, got %v", want, caps.OutputFormats)
		}
	}
	for _, want := range []string{"file_path", "url", "b64_json"} {
		if !containsString(caps.ResponseFormats, want) {
			t.Errorf("response_formats missing %q, got %v", want, caps.ResponseFormats)
		}
	}

	if caps.DefaultSize != defaultImageSize {
		t.Errorf("default_size = %q, want %q", caps.DefaultSize, defaultImageSize)
	}
	if caps.DefaultResponseFormat != defaultImageResponseFormat {
		t.Errorf("default_response_format = %q, want %q", caps.DefaultResponseFormat, defaultImageResponseFormat)
	}
	if caps.MaxCompareImages != DefaultMaxImages {
		t.Errorf("max_compare_images = %d, want %d", caps.MaxCompareImages, DefaultMaxImages)
	}
	if caps.MaxImageResultsPerCall != maxImageResults {
		t.Errorf("max_image_results_per_call = %d, want %d", caps.MaxImageResultsPerCall, maxImageResults)
	}
	if !caps.UpstreamResponseFormatOmitted {
		t.Error("upstream_response_format_omitted must be true")
	}
	if caps.UpstreamResponseFormatRationale == "" {
		t.Error("upstream_response_format_rationale must not be empty")
	}
}

func TestListImageCapabilitiesHandler_DefensiveAspectMapCopy(t *testing.T) {
	handler := ListImageCapabilitiesHandler()

	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	var caps ImageCapabilities
	if err := json.Unmarshal([]byte(result.Content[0].Text), &caps); err != nil {
		t.Fatalf("failed to decode capabilities JSON: %v", err)
	}

	// Mutating the returned map must not affect subsequent invocations.
	caps.AspectRatioToSize[aspectRatio1To1] = "tampered"

	result2, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("second handler call returned error: %v", err)
	}
	var caps2 ImageCapabilities
	if err := json.Unmarshal([]byte(result2.Content[0].Text), &caps2); err != nil {
		t.Fatalf("failed to decode second capabilities JSON: %v", err)
	}
	if caps2.AspectRatioToSize[aspectRatio1To1] != "1024x1024" {
		t.Errorf("aspect map was leaked across calls, got %q", caps2.AspectRatioToSize[aspectRatio1To1])
	}
}

func TestListImageCapabilitiesTool_HasNoArgs(t *testing.T) {
	if ListImageCapabilitiesTool.Name != "list_image_capabilities" {
		t.Errorf("tool name = %q, want list_image_capabilities", ListImageCapabilitiesTool.Name)
	}
	if len(ListImageCapabilitiesTool.InputSchema.Required) != 0 {
		t.Errorf("tool must have no required arguments, got %v", ListImageCapabilitiesTool.InputSchema.Required)
	}
	if ListImageCapabilitiesTool.InputSchema.AdditionalProperties {
		t.Error("tool must reject unknown properties to keep the contract tight")
	}
}
