package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// maxAPIErrorBodySnippet caps how much of an unstructured error body the
// formatter will surface in messages. The intent is to keep MCP tool results
// readable when an upstream proxy returns large HTML bodies for 5xx errors.
const maxAPIErrorBodySnippet = 2048

// formatAPIError builds a human-readable error from an upstream non-2xx
// response. When the body matches the OpenAI error envelope the message,
// type, and code are surfaced; otherwise a truncated raw body snippet is
// included. Temporary upstream statuses additionally carry a hint so MCP
// clients can decide whether to retry or fall back without parsing the
// status code separately.
func formatAPIError(statusCode int, respBody []byte) error {
	var errResp ErrorResponse
	if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
		parts := []string{errResp.Error.Message}
		if errResp.Error.Type != "" {
			parts = append(parts, "type="+errResp.Error.Type)
		}
		if errResp.Error.Code != "" {
			parts = append(parts, "code="+errResp.Error.Code)
		}
		if isTemporaryUpstreamStatus(statusCode) {
			parts = append(parts, "hint=upstream service is temporarily unavailable; retry later or check provider logs")
		}
		return fmt.Errorf("API error: status %d: %s", statusCode, strings.Join(parts, ", "))
	}

	parts := []string{"body: " + truncateAPIErrorBody(string(respBody))}
	if isTemporaryUpstreamStatus(statusCode) {
		parts = append(parts, "hint=upstream service is temporarily unavailable; retry later or check provider logs")
	}
	return fmt.Errorf("API error: status %d, %s", statusCode, strings.Join(parts, ", "))
}

func truncateAPIErrorBody(body string) string {
	body = strings.TrimSpace(body)
	if len(body) <= maxAPIErrorBodySnippet {
		return body
	}
	return body[:maxAPIErrorBodySnippet] + "...(truncated)"
}

// isTemporaryUpstreamStatus reports whether the given HTTP status indicates a
// transient upstream condition (502/503/504). MCP clients typically treat
// these as retry-eligible without a code change.
func isTemporaryUpstreamStatus(statusCode int) bool {
	return statusCode == http.StatusBadGateway ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout
}
