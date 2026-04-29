// Package main is the entry point for the Vision MCP Server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/AoManoh/openPic-mcp/internal/config"
	"github.com/AoManoh/openPic-mcp/internal/protocol"
	"github.com/AoManoh/openPic-mcp/internal/provider/openai"
	"github.com/AoManoh/openPic-mcp/internal/service/tool"
	"github.com/AoManoh/openPic-mcp/internal/tools"
	"github.com/AoManoh/openPic-mcp/internal/transport"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

func main() {
	// Set up logging to stderr (stdout is used for MCP communication)
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create transport. Connect synchronously while we still own the
	// initialization context so a misbehaving stdio peer surfaces here
	// instead of inside the receive loop.
	conn, err := transport.NewStdio().Connect(context.Background())
	if err != nil {
		log.Fatalf("Failed to connect transport: %v", err)
	}
	defer conn.Close()

	// Create provider. The same struct currently satisfies both the vision
	// and image-generation interfaces; passing it twice keeps the seams
	// explicit so a future deployment can plug different providers in for
	// each capability without touching this entry point.
	openaiProvider := openai.NewProvider(cfg)

	// Create tool manager and register every tool exported by the tools
	// package in a single call. See internal/tools/registry.go for the
	// authoritative tool list.
	//
	// imageOpts derives deployment-level defaults (output dir, filename
	// prefix, overwrite, inline payload guard) from config so the image
	// generation/edit tools persist files where the operator asked, with
	// the names they configured. Per-call MCP arguments still override
	// these defaults via tools.resolveOutputPolicy.
	imageOpts := []tools.HandlerOption{
		tools.WithDefaultOutputDir(cfg.OutputDir),
		tools.WithDefaultFilenamePrefix(cfg.FilenamePrefix),
		tools.WithDefaultOverwrite(cfg.Overwrite),
		tools.WithMaxInlinePayloadBytes(cfg.MaxInlinePayloadBytes),
	}
	toolManager := tool.NewManager()
	if err := tools.RegisterAll(toolManager, openaiProvider, openaiProvider, imageOpts...); err != nil {
		log.Fatalf("Failed to register MCP tools: %v", err)
	}

	// Create MCP handler
	mcpHandler := protocol.NewMCPHandler()

	// Register tools handlers
	mcpHandler.RegisterToolsHandlers(
		createToolsListHandler(toolManager),
		createToolsCallHandler(toolManager),
	)

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		cancel()
	}()

	// Main message loop. The dispatch model is still serial in this commit;
	// commit C4 introduces the dedicated server engine that turns this into
	// a worker-pool driven loop.
	log.Println("Vision MCP Server started")
	if err := runMessageLoop(ctx, conn, mcpHandler); err != nil {
		log.Fatalf("Message loop error: %v", err)
	}
	log.Println("Vision MCP Server stopped")
}

// runMessageLoop runs the main message processing loop.
//
// It is intentionally minimal at this stage; concurrent dispatch and
// graceful in-flight draining live in [server.Server] (introduced later in
// the refactor series). Keeping this loop single-goroutine for now means
// each commit in the series independently builds and ships a working
// binary.
func runMessageLoop(ctx context.Context, conn transport.Connection, handler *protocol.MCPHandler) error {
	for {
		// Read message; ctx cancellation propagates through the connection.
		data, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("failed to read message: %w", err)
		}

		// Skip empty messages (defensive; the transport already filters
		// pure-newline frames).
		if len(data) == 0 {
			continue
		}

		// Handle message.
		resp, err := handler.HandleMessage(data)
		if err != nil {
			log.Printf("Error handling message: %v", err)
			continue
		}

		// Skip if no response (e.g., for notifications).
		if resp == nil {
			continue
		}

		// Encode and send response.
		respData, err := protocol.EncodeResponse(resp)
		if err != nil {
			log.Printf("Error encoding response: %v", err)
			continue
		}

		if err := conn.Write(ctx, respData); err != nil {
			log.Printf("Error writing response: %v", err)
		}
	}
}

// createToolsListHandler creates a handler for tools/list requests.
func createToolsListHandler(tm *tool.Manager) protocol.Handler {
	return func(req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		result := types.ToolsListResult{
			Tools: tm.List(),
		}
		return protocol.NewSuccessResponse(req.ID, result), nil
	}
}

// createToolsCallHandler creates a handler for tools/call requests.
func createToolsCallHandler(tm *tool.Manager) protocol.Handler {
	return func(req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		// Parse parameters
		params, err := protocol.ParseToolCallParams(req)
		if err != nil {
			return protocol.NewInvalidParamsError(req.ID, err.Error()), nil
		}

		// Execute tool
		result, err := tm.Execute(context.Background(), params.Name, params.Arguments)
		if err != nil {
			return protocol.NewToolExecutionError(req.ID, err.Error()), nil
		}

		return protocol.NewSuccessResponse(req.ID, result), nil
	}
}
