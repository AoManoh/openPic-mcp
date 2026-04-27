package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AoManoh/openPic-mcp/internal/config"
	"github.com/AoManoh/openPic-mcp/internal/protocol"
	"github.com/AoManoh/openPic-mcp/internal/provider/openai"
	"github.com/AoManoh/openPic-mcp/internal/service/tool"
	"github.com/AoManoh/openPic-mcp/internal/tools"
	"github.com/AoManoh/openPic-mcp/internal/transport"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// setupTestServer creates a test server with mock configuration.
func setupTestServer() (*protocol.MCPHandler, *tool.Manager) {
	// Create mock config
	cfg := &config.Config{
		APIBaseURL:  "https://api.openai.com/v1",
		APIKey:      "test-key",
		Model:       "gpt-4o",
		VisionModel: "gpt-4o",
		ImageModel:  "gpt-image-1",
	}

	// Create provider
	provider := openai.NewProvider(cfg)

	// Create tool manager and register tools
	toolManager := tool.NewManager()
	toolManager.Register(tools.DescribeImageTool, tools.DescribeImageHandler(provider))
	toolManager.Register(tools.CompareImagesTool, tools.CompareImagesHandler(provider))
	toolManager.Register(tools.GenerateImageTool, tools.GenerateImageHandler(provider))
	toolManager.Register(tools.EditImageTool, tools.EditImageHandler(provider))

	// Create MCP handler
	mcpHandler := protocol.NewMCPHandler()

	// Register tools handlers
	mcpHandler.RegisterToolsHandlers(
		func(req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
			result := types.ToolsListResult{
				Tools: toolManager.List(),
			}
			return protocol.NewSuccessResponse(req.ID, result), nil
		},
		func(req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
			params, err := protocol.ParseToolCallParams(req)
			if err != nil {
				return protocol.NewInvalidParamsError(req.ID, err.Error()), nil
			}
			result, err := toolManager.Execute(nil, params.Name, params.Arguments)
			if err != nil {
				return protocol.NewToolExecutionError(req.ID, err.Error()), nil
			}
			return protocol.NewSuccessResponse(req.ID, result), nil
		},
	)

	return mcpHandler, toolManager
}

func TestInitializeRequest(t *testing.T) {
	handler, _ := setupTestServer()

	// Create initialize request
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`

	resp, err := handler.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("HandleMessage error: %v", err)
	}

	if resp == nil {
		t.Fatal("Response is nil")
	}

	if resp.Error != nil {
		t.Fatalf("Response has error: %v", resp.Error)
	}

	// Verify response structure
	result, ok := resp.Result.(types.InitializeResult)
	if !ok {
		t.Fatalf("Result is not InitializeResult: %T", resp.Result)
	}

	if result.ProtocolVersion != types.MCPProtocolVersion {
		t.Errorf("ProtocolVersion = %v, want %v", result.ProtocolVersion, types.MCPProtocolVersion)
	}

	if result.ServerInfo.Name != protocol.ServerName {
		t.Errorf("ServerInfo.Name = %v, want %v", result.ServerInfo.Name, protocol.ServerName)
	}
}

func TestToolsListRequest(t *testing.T) {
	handler, _ := setupTestServer()

	// First initialize
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	handler.HandleMessage([]byte(initReq))

	// Send initialized notification
	initedReq := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	handler.HandleMessage([]byte(initedReq))

	// Now test tools/list
	req := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	resp, err := handler.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("HandleMessage error: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("Response has error: %v", resp.Error)
	}

	// Encode response to JSON for inspection
	respJSON, _ := json.Marshal(resp)
	t.Logf("tools/list response: %s", string(respJSON))

	// Verify tools are returned
	if resp.Result == nil {
		t.Fatal("Result is nil")
	}

	if !strings.Contains(string(respJSON), "describe_image") {
		t.Fatal("tools/list response missing describe_image")
	}
	if !strings.Contains(string(respJSON), "compare_images") {
		t.Fatal("tools/list response missing compare_images")
	}
	if !strings.Contains(string(respJSON), "generate_image") {
		t.Fatal("tools/list response missing generate_image")
	}
	if !strings.Contains(string(respJSON), "edit_image") {
		t.Fatal("tools/list response missing edit_image")
	}
}

func TestStdioTransportIntegration(t *testing.T) {
	// Create a buffer to simulate stdin/stdout
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n")
	output := &bytes.Buffer{}

	trans := transport.NewStdioTransportWithIO(input, output)

	// Read message
	data, err := trans.Read()
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	// Verify message was read correctly
	var req types.JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if req.Method != "initialize" {
		t.Errorf("Method = %v, want initialize", req.Method)
	}

	// Write response
	resp := protocol.NewSuccessResponse(req.ID, map[string]string{"status": "ok"})
	respData, _ := protocol.EncodeResponse(resp)
	if err := trans.Write(respData); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Verify output
	if !strings.Contains(output.String(), "jsonrpc") {
		t.Errorf("Output doesn't contain jsonrpc: %s", output.String())
	}
}

func TestRunMessageLoopReturnsNilOnEOF(t *testing.T) {
	handler, _ := setupTestServer()
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`)
	output := &bytes.Buffer{}
	trans := transport.NewStdioTransportWithIO(input, output)

	if err := runMessageLoop(context.Background(), trans, handler); err != nil {
		t.Fatalf("runMessageLoop() error = %v, want nil", err)
	}
	if !strings.Contains(output.String(), "protocolVersion") {
		t.Fatalf("output missing initialize response: %s", output.String())
	}
}

func TestMethodNotFoundError(t *testing.T) {
	handler, _ := setupTestServer()

	// Initialize first
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	handler.HandleMessage([]byte(initReq))
	initedReq := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	handler.HandleMessage([]byte(initedReq))

	// Test unknown method
	req := `{"jsonrpc":"2.0","id":2,"method":"unknown/method","params":{}}`
	resp, err := handler.HandleMessage([]byte(req))
	if err != nil {
		t.Fatalf("HandleMessage error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error response")
	}

	if resp.Error.Code != types.ErrCodeMethodNotFound {
		t.Errorf("Error code = %v, want %v", resp.Error.Code, types.ErrCodeMethodNotFound)
	}
}
