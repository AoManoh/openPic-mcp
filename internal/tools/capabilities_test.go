package tools

import (
	"context"
	"encoding/json"
	"strings"
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

	// output_format is advisory: openPic-mcp cannot guarantee the upstream
	// honours the requested encoding, so list_image_capabilities must
	// expose the enforcement level explicitly together with a non-empty
	// note that points callers to the magic-byte format/warnings fields.
	if caps.OutputFormatEnforcement != "advisory" {
		t.Errorf("output_format_enforcement = %q, want %q", caps.OutputFormatEnforcement, "advisory")
	}
	if caps.OutputFormatNotes == "" {
		t.Error("output_format_notes must not be empty")
	}
	for _, token := range []string{"webp", "format", "warnings"} {
		if !strings.Contains(caps.OutputFormatNotes, token) {
			t.Errorf("output_format_notes missing token %q: %q", token, caps.OutputFormatNotes)
		}
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

// TestImageCapabilities_SizeReliabilityNotes_NotEmpty pins the
// observation-1 follow-up: list_image_capabilities must surface a
// non-empty advisory string about size reliability under
// OPENPIC_TIMEOUT, so LLM agents auto-selecting a size can see the
// caveat without reading README.
//
// The test also pins the deliberate non-recommendation: openPic-mcp
// supports many upstream providers with different reliability
// envelopes, so the field must not bake a specific number like
// "1024x1536" into the contract. We assert the absence of a
// recommendation pattern alongside the presence of the principle.
func TestImageCapabilities_SizeReliabilityNotes_NotEmpty(t *testing.T) {
	handler := ListImageCapabilitiesHandler()
	result, err := handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	var caps ImageCapabilities
	if err := json.Unmarshal([]byte(result.Content[0].Text), &caps); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if caps.SizeReliabilityNotes == "" {
		t.Fatal("size_reliability_notes must not be empty — LLM agents need this to make timeout-aware size choices")
	}

	// Advisory must reference OPENPIC_TIMEOUT and the async escape
	// hatch (submit_image_task), since those are the actionable
	// remedies the agent can take.
	for _, token := range []string{"OPENPIC_TIMEOUT", "submit_image_task"} {
		if !strings.Contains(caps.SizeReliabilityNotes, token) {
			t.Errorf("size_reliability_notes must reference %q so callers know the remedy: %q",
				token, caps.SizeReliabilityNotes)
		}
	}

	// Anti-recommendation pin: the field must NOT prescribe a
	// specific maximum size with an imperative verb (e.g. "use
	// 1024x1536"). Reliability is upstream-dependent and openPic-mcp
	// must not pretend otherwise. We look for "<imperative> <specific
	// size>" patterns rather than the bare phrase "recommended max
	// size", so the field can still legitimately use that phrase to
	// describe what it does NOT do (which is itself a useful signal).
	imperativePrefixes := []string{"use ", "set size to ", "stick to "}
	specificSizes := []string{"1024x1024", "1024x1536", "1536x1024", "2048x2048"}
	lowerNotes := strings.ToLower(caps.SizeReliabilityNotes)
	for _, prefix := range imperativePrefixes {
		for _, sz := range specificSizes {
			needle := prefix + sz
			if strings.Contains(lowerNotes, needle) {
				t.Errorf("size_reliability_notes must not prescribe a specific size with an imperative; "+
					"openPic-mcp supports multiple upstreams with different reliability envelopes. "+
					"forbidden pattern %q matched: %q", needle, caps.SizeReliabilityNotes)
			}
		}
	}
}

// TestCancelTaskTool_DescriptionDocumentsQuotaSemantics is the
// observation-2 regression. The README has the quota disclosure but
// LLM agents only read tool schemas; this test pins that the cancel
// quota caveat is also part of the schema-level Description so an
// agent considering cancel_task to "save quota" gets the truth.
func TestCancelTaskTool_DescriptionDocumentsQuotaSemantics(t *testing.T) {
	desc := CancelTaskTool.Description
	if desc == "" {
		t.Fatal("cancel_task description must not be empty")
	}
	for _, token := range []string{"upstream", "quota"} {
		if !strings.Contains(strings.ToLower(desc), token) {
			t.Errorf("cancel_task description must mention %q: %q", token, desc)
		}
	}
	// Pin the negative claim so a future doc rewrite can't accidentally
	// over-promise quota savings.
	for _, token := range []string{"do not rely", "implementation-dependent"} {
		if !strings.Contains(strings.ToLower(desc), token) {
			t.Errorf("cancel_task description must include the negative-claim phrase %q: %q", token, desc)
		}
	}
}

// TestSubmitImageTaskTool_DescriptionRecommendsAsyncForLongRequests
// pins the observation-2 follow-up: when describing the async tool,
// the schema must guide LLM agents to use it for any request that
// might exceed the synchronous tools/call timeout, so they don't
// default to synchronous generate_image / edit_image for risky calls.
func TestSubmitImageTaskTool_DescriptionRecommendsAsyncForLongRequests(t *testing.T) {
	desc := SubmitImageTaskTool.Description
	if desc == "" {
		t.Fatal("submit_image_task description must not be empty")
	}
	for _, token := range []string{"OPENPIC_TIMEOUT", "client disconnect", "task_id"} {
		if !strings.Contains(desc, token) {
			t.Errorf("submit_image_task description must mention %q: %q", token, desc)
		}
	}
}
