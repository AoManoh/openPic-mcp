// Package main is the entry point for the Vision MCP Server.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/AoManoh/openPic-mcp/internal/config"
	"github.com/AoManoh/openPic-mcp/internal/protocol"
	"github.com/AoManoh/openPic-mcp/internal/provider/openai"
	"github.com/AoManoh/openPic-mcp/internal/server"
	"github.com/AoManoh/openPic-mcp/internal/service/tool"
	"github.com/AoManoh/openPic-mcp/internal/tools"
	"github.com/AoManoh/openPic-mcp/internal/transport"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

func main() {
	// The bootstrap-only `log` package is still useful for fatal startup
	// errors before the structured logger is constructed. It writes to
	// stderr — stdout is reserved exclusively for the MCP JSON-RPC stream.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration. Misconfigured deployments must fail loudly here
	// rather than degrade silently inside the dispatch loop.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Build the structured logger that the server engine and (eventually)
	// the protocol/tools layers share. It writes to stderr regardless of
	// format so it can never poison the stdio MCP channel.
	logger := config.NewLogger(cfg)

	// The provider is shared between vision and image-generation tools.
	// Passing it twice keeps the seams explicit so a future deployment can
	// plug different providers in for each capability without touching
	// this entry point.
	openaiProvider := openai.NewProvider(cfg)

	// Tool registry. imageOpts threads deployment-level defaults (output
	// dir, filename prefix, overwrite, inline payload guard) into the
	// generation and edit tools; per-call MCP arguments still win via
	// tools.resolveOutputPolicy.
	imageOpts := []tools.HandlerOption{
		tools.WithDefaultOutputDir(cfg.OutputDir),
		tools.WithDefaultFilenamePrefix(cfg.FilenamePrefix),
		tools.WithDefaultOverwrite(cfg.Overwrite),
		tools.WithMaxInlinePayloadBytes(cfg.MaxInlinePayloadBytes),
	}
	toolManager := tool.NewManager()
	if err := tools.RegisterAll(toolManager, openaiProvider, openaiProvider,
		tools.WithImageHandlerOptions(imageOpts...),
	); err != nil {
		log.Fatalf("Failed to register MCP tools: %v", err)
	}

	// MCP protocol handler with tools/list and tools/call wired in.
	mcpHandler := protocol.NewMCPHandler()
	mcpHandler.RegisterToolsHandlers(
		createToolsListHandler(toolManager),
		createToolsCallHandler(toolManager),
	)

	// Compose the server engine. The engine owns the recv loop, the
	// worker pool, the bounded queue, and the cancellation/shutdown
	// machinery; main only assembles dependencies and translates signals
	// into ctx cancellation.
	eng := server.New(
		transport.NewStdio(),
		mcpHandler,
		server.Config{
			MaxConcurrentRequests: cfg.MaxConcurrentRequests,
			RequestQueueSize:      cfg.RequestQueueSize,
			RequestTimeout:        cfg.RequestTimeout,
			ShutdownTimeout:       cfg.ShutdownTimeout,
		},
		server.WithLogger(logger),
		server.WithCancelRegistry(mcpHandler.Cancellations()),
	)

	// Bind SIGINT/SIGTERM to ctx cancellation so a graceful shutdown
	// always flows through the engine's drain machinery (close work
	// queue → wait for in-flight → close transport).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("server.signal_received", "signal", sig.String())
		cancel()
	}()

	logger.Info("server.boot",
		"workers", cfg.MaxConcurrentRequests,
		"queue_size", cfg.RequestQueueSize,
		"request_timeout", cfg.RequestTimeout.String(),
		"shutdown_timeout", cfg.ShutdownTimeout.String(),
		"log_level", cfg.LogLevel,
		"log_format", cfg.LogFormat,
	)
	if err := eng.Run(ctx); err != nil {
		logger.Error("server.exit", "err", err.Error())
		os.Exit(1)
	}
	logger.Info("server.exit", "completed", eng.Completed(), "fallback", eng.FallbackCount())
}

// createToolsListHandler creates a handler for tools/list requests.
func createToolsListHandler(tm *tool.Manager) protocol.Handler {
	return func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		result := types.ToolsListResult{
			Tools: tm.List(),
		}
		return protocol.NewSuccessResponse(req.ID, result), nil
	}
}

// createToolsCallHandler creates a handler for tools/call requests.
//
// ctx is the per-request context the protocol/server layer derived for this
// call. It must reach Manager.Execute so that ctx-aware tools can interrupt
// upstream HTTP work when the engine cancels (shutdown) or the client sends
// a `notifications/cancelled`.
func createToolsCallHandler(tm *tool.Manager) protocol.Handler {
	return func(ctx context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		// Parse parameters
		params, err := protocol.ParseToolCallParams(req)
		if err != nil {
			return protocol.NewInvalidParamsError(req.ID, err.Error()), nil
		}

		// Execute tool
		result, err := tm.Execute(ctx, params.Name, params.Arguments)
		if err != nil {
			return protocol.NewToolExecutionError(req.ID, err.Error()), nil
		}

		return protocol.NewSuccessResponse(req.ID, result), nil
	}
}
