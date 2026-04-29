package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// ServerName is the name of the MCP server.
const ServerName = "vision-mcp-server"

// ServerVersion is the version of the MCP server.
const ServerVersion = "1.0.0"

// MethodCancelled is the MCP-defined notification name for client-driven
// request cancellation. The handler resolves the requestId in
// [CancellationRegistry] and lets the registered cancel func interrupt the
// matching in-flight tool call.
const MethodCancelled = "notifications/cancelled"

// MCPHandler handles MCP protocol messages.
//
// MCPHandler is safe for concurrent calls by design: lifecycle flags are
// stored in [atomic.Bool], the router has its own RWMutex, and the
// cancellation registry serializes its own state. The server engine drives
// HandleMessage from multiple worker goroutines once concurrent dispatch is
// enabled in the upcoming server commit.
type MCPHandler struct {
	initialized atomic.Bool
	router      *Router
	cancels     *CancellationRegistry
}

// NewMCPHandler creates a new MCP handler.
func NewMCPHandler() *MCPHandler {
	h := &MCPHandler{
		router:  NewRouter(),
		cancels: NewCancellationRegistry(),
	}
	// Register built-in handlers
	h.router.Register("initialize", h.handleInitialize)
	h.router.Register("notifications/initialized", h.handleInitialized)
	h.router.Register("shutdown", h.handleShutdown)
	h.router.Register(MethodCancelled, h.handleCancelled)
	return h
}

// Router returns the message router.
func (h *MCPHandler) Router() *Router {
	return h.router
}

// Cancellations exposes the cancellation registry so the server engine can
// register and clear per-request cancel funcs around tools/call execution.
func (h *MCPHandler) Cancellations() *CancellationRegistry {
	return h.cancels
}

// IsInitialized returns whether the handler has been initialized.
func (h *MCPHandler) IsInitialized() bool {
	return h.initialized.Load()
}

// HandleMessage processes an incoming JSON-RPC message.
//
// ctx propagates to the dispatched handler. The server engine derives
// per-request ctx values from its receive-loop ctx so engine shutdown and
// MCP cancellation notifications both reach long-running tool calls.
func (h *MCPHandler) HandleMessage(ctx context.Context, data []byte) (*types.JSONRPCResponse, error) {
	// Decode the request
	req, err := DecodeRequest(data)
	if err != nil {
		return NewParseError(err.Error()), nil
	}

	// Check if initialized for non-initialize methods. Allow initialize and
	// notifications/initialized before initialization. Cancellation
	// notifications are also allowed pre-initialization so a client racing
	// the handshake cannot leak in-flight work.
	if !h.initialized.Load() && req.Method != "initialize" && req.Method != "notifications/initialized" && req.Method != MethodCancelled {
		return NewErrorResponse(req.ID, types.ErrCodeInvalidRequest,
			"Server not initialized", nil), nil
	}

	// Route the request
	return h.router.Route(ctx, req)
}

// handleInitialize handles the initialize request.
func (h *MCPHandler) handleInitialize(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
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
func (h *MCPHandler) handleInitialized(_ context.Context, _ *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
	h.initialized.Store(true)
	// Notifications don't return a response
	return nil, nil
}

// handleShutdown handles the shutdown request.
func (h *MCPHandler) handleShutdown(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
	h.initialized.Store(false)
	return NewSuccessResponse(req.ID, nil), nil
}

// handleCancelled handles the MCP `notifications/cancelled` notification by
// triggering the cancel func registered for the requestId, if any. It
// always returns a nil response since notifications do not carry a reply.
func (h *MCPHandler) handleCancelled(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
	params, err := ParseCancellationParams(req)
	if err != nil {
		// Per JSON-RPC, a malformed notification has no response. Surface
		// the failure through the registry no-op rather than a wire error.
		return nil, nil
	}
	h.cancels.Cancel(params.RequestID)
	return nil, nil
}

// RegisterToolsHandlers registers the tools/list and tools/call handlers.
//
// Both handlers receive the per-request ctx the server engine derived for
// the call. tools/call handlers are expected to forward ctx all the way to
// the tool implementation so cancellation reaches the upstream HTTP call.
func (h *MCPHandler) RegisterToolsHandlers(listHandler Handler, callHandler Handler) {
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
