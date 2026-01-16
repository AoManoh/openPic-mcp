package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// ServerName is the name of the MCP server.
const ServerName = "vision-mcp-server"

// ServerVersion is the version of the MCP server.
const ServerVersion = "1.0.0"

// MCPHandler handles MCP protocol messages.
type MCPHandler struct {
	initialized bool
	router      *Router
}

// NewMCPHandler creates a new MCP handler.
func NewMCPHandler() *MCPHandler {
	h := &MCPHandler{
		router: NewRouter(),
	}
	// Register built-in handlers
	h.router.Register("initialize", h.handleInitialize)
	h.router.Register("notifications/initialized", h.handleInitialized)
	h.router.Register("shutdown", h.handleShutdown)
	return h
}

// Router returns the message router.
func (h *MCPHandler) Router() *Router {
	return h.router
}

// IsInitialized returns whether the handler has been initialized.
func (h *MCPHandler) IsInitialized() bool {
	return h.initialized
}

// HandleMessage processes an incoming JSON-RPC message.
func (h *MCPHandler) HandleMessage(data []byte) (*types.JSONRPCResponse, error) {
	// Decode the request
	req, err := DecodeRequest(data)
	if err != nil {
		return NewParseError(err.Error()), nil
	}

	// Check if initialized for non-initialize methods
	// Allow initialize and notifications/initialized before initialization
	if !h.initialized && req.Method != "initialize" && req.Method != "notifications/initialized" {
		return NewErrorResponse(req.ID, types.ErrCodeInvalidRequest,
			"Server not initialized", nil), nil
	}

	// Route the request
	return h.router.Route(req)
}

// handleInitialize handles the initialize request.
func (h *MCPHandler) handleInitialize(req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
	// Parse params
	var params types.InitializeParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return NewInvalidParamsError(req.ID, err.Error()), nil
		}
	}

	// Build response
	result := types.InitializeResult{
		ProtocolVersion: types.MCPProtocolVersion,
		Capabilities: types.ServerCapabilities{
			Tools: &types.ToolsCapability{
				ListChanged: true,
			},
		},
		ServerInfo: types.ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
	}

	return NewSuccessResponse(req.ID, result), nil
}

// handleInitialized handles the initialized notification.
func (h *MCPHandler) handleInitialized(req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
	h.initialized = true
	// Notifications don't return a response
	return nil, nil
}

// handleShutdown handles the shutdown request.
func (h *MCPHandler) handleShutdown(req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
	h.initialized = false
	return NewSuccessResponse(req.ID, nil), nil
}

// RegisterToolsHandlers registers the tools/list and tools/call handlers.
func (h *MCPHandler) RegisterToolsHandlers(
	listHandler func(*types.JSONRPCRequest) (*types.JSONRPCResponse, error),
	callHandler func(*types.JSONRPCRequest) (*types.JSONRPCResponse, error),
) {
	h.router.Register("tools/list", listHandler)
	h.router.Register("tools/call", callHandler)
}

// ParseToolCallParams parses the parameters for a tools/call request.
func ParseToolCallParams(req *types.JSONRPCRequest) (*types.ToolCallParams, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("missing params")
	}

	var params types.ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, fmt.Errorf("failed to parse tool call params: %w", err)
	}

	if params.Name == "" {
		return nil, fmt.Errorf("missing tool name")
	}

	return &params, nil
}
