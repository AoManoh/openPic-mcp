// Package protocol provides MCP protocol handling for the Vision MCP Server.
package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/anthropic/vision-mcp-server/pkg/types"
)

// DecodeRequest decodes a JSON-RPC request from raw bytes.
func DecodeRequest(data []byte) (*types.JSONRPCRequest, error) {
	var req types.JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to decode JSON-RPC request: %w", err)
	}

	// Validate JSON-RPC version
	if req.JSONRPC != types.JSONRPCVersion {
		return nil, fmt.Errorf("invalid JSON-RPC version: %s", req.JSONRPC)
	}

	return &req, nil
}

// EncodeResponse encodes a JSON-RPC response to bytes.
func EncodeResponse(resp *types.JSONRPCResponse) ([]byte, error) {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to encode JSON-RPC response: %w", err)
	}
	return data, nil
}

// NewSuccessResponse creates a successful JSON-RPC response.
func NewSuccessResponse(id any, result any) *types.JSONRPCResponse {
	return &types.JSONRPCResponse{
		JSONRPC: types.JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates an error JSON-RPC response.
func NewErrorResponse(id any, code int, message string, data any) *types.JSONRPCResponse {
	return &types.JSONRPCResponse{
		JSONRPC: types.JSONRPCVersion,
		ID:      id,
		Error: &types.JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// NewParseError creates a parse error response.
func NewParseError(data any) *types.JSONRPCResponse {
	return NewErrorResponse(nil, types.ErrCodeParseError, "Parse error", data)
}

// NewInvalidRequestError creates an invalid request error response.
func NewInvalidRequestError(id any, data any) *types.JSONRPCResponse {
	return NewErrorResponse(id, types.ErrCodeInvalidRequest, "Invalid Request", data)
}

// NewMethodNotFoundError creates a method not found error response.
func NewMethodNotFoundError(id any, method string) *types.JSONRPCResponse {
	return NewErrorResponse(id, types.ErrCodeMethodNotFound, "Method not found", method)
}

// NewInvalidParamsError creates an invalid params error response.
func NewInvalidParamsError(id any, data any) *types.JSONRPCResponse {
	return NewErrorResponse(id, types.ErrCodeInvalidParams, "Invalid params", data)
}

// NewInternalError creates an internal error response.
func NewInternalError(id any, data any) *types.JSONRPCResponse {
	return NewErrorResponse(id, types.ErrCodeInternalError, "Internal error", data)
}

// NewToolExecutionError creates a tool execution error response.
func NewToolExecutionError(id any, data any) *types.JSONRPCResponse {
	return NewErrorResponse(id, types.ErrCodeToolExecution, "Tool execution error", data)
}

// NewProviderError creates a provider error response.
func NewProviderError(id any, data any) *types.JSONRPCResponse {
	return NewErrorResponse(id, types.ErrCodeProviderError, "Provider error", data)
}

// IsNotification checks if a request is a notification (no ID).
func IsNotification(req *types.JSONRPCRequest) bool {
	return req.ID == nil
}
