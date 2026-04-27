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

	// Create transport
	trans := transport.NewStdioTransport()
	defer trans.Close()

	// Create provider
	provider := openai.NewProvider(cfg)

	// Create tool manager and register tools
	toolManager := tool.NewManager()
	if err := toolManager.Register(tools.DescribeImageTool, tools.DescribeImageHandler(provider)); err != nil {
		log.Fatalf("Failed to register describe_image tool: %v", err)
	}
	if err := toolManager.Register(tools.CompareImagesTool, tools.CompareImagesHandler(provider)); err != nil {
		log.Fatalf("Failed to register compare_images tool: %v", err)
	}
	if err := toolManager.Register(tools.GenerateImageTool, tools.GenerateImageHandler(provider)); err != nil {
		log.Fatalf("Failed to register generate_image tool: %v", err)
	}
	if err := toolManager.Register(tools.EditImageTool, tools.EditImageHandler(provider)); err != nil {
		log.Fatalf("Failed to register edit_image tool: %v", err)
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

	// Main message loop
	log.Println("Vision MCP Server started")
	if err := runMessageLoop(ctx, trans, mcpHandler); err != nil {
		log.Fatalf("Message loop error: %v", err)
	}
	log.Println("Vision MCP Server stopped")
}

// runMessageLoop runs the main message processing loop.
func runMessageLoop(ctx context.Context, trans *transport.StdioTransport, handler *protocol.MCPHandler) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			// Read message
			data, err := trans.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				// Check if context was cancelled
				select {
				case <-ctx.Done():
					return nil
				default:
					return fmt.Errorf("failed to read message: %w", err)
				}
			}

			// Skip empty messages
			if len(data) == 0 {
				continue
			}

			// Handle message
			resp, err := handler.HandleMessage(data)
			if err != nil {
				log.Printf("Error handling message: %v", err)
				continue
			}

			// Skip if no response (e.g., for notifications)
			if resp == nil {
				continue
			}

			// Encode and send response
			respData, err := protocol.EncodeResponse(resp)
			if err != nil {
				log.Printf("Error encoding response: %v", err)
				continue
			}

			if err := trans.Write(respData); err != nil {
				log.Printf("Error writing response: %v", err)
			}
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
