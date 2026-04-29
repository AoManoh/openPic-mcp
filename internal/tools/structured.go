package tools

import "fmt"

// RequestedParams is a structured echo of the parameters the caller
// supplied to a generate_image / edit_image tool call. Only fields the
// caller actually provided are populated; absent fields are omitted via
// json:",omitempty" so consumers can distinguish "not provided" from
// "default applied". The struct intentionally lives in the tools layer:
// it is part of the MCP-facing response contract, not the provider
// abstraction.
type RequestedParams struct {
	Prompt         string `json:"prompt,omitempty"`
	Size           string `json:"size,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	Quality        string `json:"quality,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	N              int    `json:"n,omitempty"`
	OutputDir      string `json:"output_dir,omitempty"`
	FilenamePrefix string `json:"filename_prefix,omitempty"`
	Overwrite      *bool  `json:"overwrite,omitempty"`
}

// AppliedParams describes the parameters that the tool layer actually
// produced during a call: the values forwarded upstream to the provider
// and the delivery shape the tool layer used for the response. It lets
// MCP clients reconstruct exactly how the call was executed without
// having to second-guess defaults or fallbacks.
type AppliedParams struct {
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	N              int    `json:"n,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	OutputDir      string `json:"output_dir,omitempty"`
	FilenamePrefix string `json:"filename_prefix,omitempty"`
	Overwrite      bool   `json:"overwrite"`
}

// ImageFileInfo summarises a single file produced by the tool layer
// during a generate_image / edit_image call. It is included in the
// top-level response so generic MCP clients can locate generated assets
// without having to parse the per-image envelope. Fields are absolute
// paths and concrete sizes; format is the canonical magic-byte name
// detected from the bytes (png / jpeg / webp ...) when known.
type ImageFileInfo struct {
	Index     int    `json:"index"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Format    string `json:"format,omitempty"`
}

// UsageInfo carries any token-usage figures the upstream provider
// returned for an image generation / edit. Fields are pointers so we can
// distinguish "missing" from "zero" — some OpenAI-compatible deployments
// only return a subset of the usage breakdown, and we never want to
// fabricate values that were not present upstream.
type UsageInfo struct {
	InputTokens  *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
	TotalTokens  *int64 `json:"total_tokens,omitempty"`
}

// parseBoolArg extracts an optional boolean argument from a tool call
// argument map. Returns (nil, nil) when the key is absent so callers can
// distinguish "not provided" from "explicitly false". JSON-RPC clients
// may serialise booleans either as native bool or as the strings
// "true"/"false"; both are accepted. An invalid value is reported back
// to the caller rather than silently coerced so the contract stays
// transparent.
func parseBoolArg(args map[string]any, key string) (*bool, error) {
	raw, ok := args[key]
	if !ok {
		return nil, nil
	}
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case bool:
		out := v
		return &out, nil
	case string:
		if v == "" {
			return nil, nil
		}
		switch v {
		case "true", "TRUE", "True":
			out := true
			return &out, nil
		case "false", "FALSE", "False":
			out := false
			return &out, nil
		default:
			return nil, fmt.Errorf("%s must be a boolean (true/false), got %q", key, v)
		}
	default:
		return nil, fmt.Errorf("%s must be a boolean, got %T", key, raw)
	}
}

// buildRequestedFromArgs assembles a RequestedParams snapshot from the
// raw MCP tool arguments. Only fields the caller actually supplied are
// populated so JSON consumers can distinguish "not provided" from
// "default applied". Numeric n is always emitted because tool callers
// effectively always rely on the default of 1 — surfacing it makes the
// contract explicit without forcing every test to omit the field.
func buildRequestedFromArgs(args map[string]any, prompt string, n int, responseFormat string, overwriteOverride *bool) *RequestedParams {
	out := &RequestedParams{
		Prompt:         prompt,
		Size:           stringArg(args, "size"),
		AspectRatio:    stringArg(args, "aspect_ratio"),
		Quality:        stringArg(args, "quality"),
		OutputFormat:   stringArg(args, "output_format"),
		ResponseFormat: responseFormat,
		N:              n,
		OutputDir:      stringArg(args, "output_dir"),
		FilenamePrefix: stringArg(args, "filename_prefix"),
		Overwrite:      overwriteOverride,
	}
	return out
}

// appliedRequestView captures the subset of provider request fields that
// the AppliedParams contract surfaces back to MCP clients. Using a value
// struct (rather than an interface implemented by provider types) keeps
// AppliedParams free of any dependency on the provider package and makes
// it trivial for tests to assert against.
type appliedRequestView struct {
	Size         string
	Quality      string
	OutputFormat string
	N            int
}

// buildAppliedFromRequest projects an appliedRequestView and the
// resolved output policy back into an AppliedParams snapshot. It is the
// canonical "what the tool layer actually did" view that pairs with
// RequestedParams in the response.
func buildAppliedFromRequest(req appliedRequestView, responseFormat string, policy outputPathPolicy) *AppliedParams {
	return &AppliedParams{
		Size:           req.Size,
		Quality:        req.Quality,
		OutputFormat:   req.OutputFormat,
		N:              req.N,
		ResponseFormat: responseFormat,
		OutputDir:      policy.Dir,
		FilenamePrefix: policy.Prefix,
		Overwrite:      policy.Overwrite,
	}
}
