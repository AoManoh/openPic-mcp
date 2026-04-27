// Package types provides common type definitions for the Vision MCP Server.
package types

import (
	"context"
	"encoding/json"
)

// =============================================================================
// JSON-RPC 2.0 Types
// =============================================================================

// JSONRPCVersion is the JSON-RPC protocol version.
const JSONRPCVersion = "2.0"

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id,omitempty"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParseError     = -32700 // Invalid JSON
	ErrCodeInvalidRequest = -32600 // Invalid Request
	ErrCodeMethodNotFound = -32601 // Method not found
	ErrCodeInvalidParams  = -32602 // Invalid params
	ErrCodeInternalError  = -32603 // Internal error

	// Custom error codes for Vision MCP Server
	ErrCodeToolExecution = -32000 // Tool execution error
	ErrCodeProviderError = -32001 // Vision API provider error
	ErrCodeConfigError   = -32002 // Configuration error
)

// =============================================================================
// MCP Protocol Types
// =============================================================================

// MCPProtocolVersion is the MCP protocol version supported by this server.
const MCPProtocolVersion = "2024-11-05"

// ServerInfo contains information about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientInfo contains information about the MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities defines the capabilities of the MCP server.
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability defines the tools capability.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ClientCapabilities defines the capabilities of the MCP client.
type ClientCapabilities struct {
	Roots *RootsCapability `json:"roots,omitempty"`
}

// RootsCapability defines the roots capability.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// InitializeParams represents the parameters for the initialize request.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

// InitializeResult represents the result of the initialize request.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// =============================================================================
// Tool Types
// =============================================================================

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema defines the JSON Schema for tool input.
type InputSchema struct {
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties,omitempty"`
	Required             []string            `json:"required,omitempty"`
	AdditionalProperties bool                `json:"additionalProperties"`
}

// Property defines a property in the input schema.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     string   `json:"default,omitempty"`
}

// ToolsListResult represents the result of tools/list request.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams represents the parameters for tools/call request.
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolCallResult represents the result of tools/call request.
type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem represents a content item in tool result.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolHandler is the function signature for tool handlers.
type ToolHandler func(ctx context.Context, args map[string]any) (*ToolCallResult, error)
