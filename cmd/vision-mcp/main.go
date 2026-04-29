// Package main is the entry point for the Vision MCP Server.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/AoManoh/openPic-mcp/internal/config"
	"github.com/AoManoh/openPic-mcp/internal/protocol"
	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/internal/provider/openai"
	"github.com/AoManoh/openPic-mcp/internal/server"
	"github.com/AoManoh/openPic-mcp/internal/service/tool"
	"github.com/AoManoh/openPic-mcp/internal/taskstore"
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
	// Bootstrap the async task layer (store + dispatcher). When
	// TaskStoreEnabled=false this returns (nil, nil, nil) and we run
	// without async tools; when disk persistence is requested but the
	// directory cannot be created/written, we fail fast with a fatal
	// log so deployment misconfiguration is impossible to miss.
	taskStore, dispatcher, err := bootstrapTaskstore(cfg, logger, openaiProvider, imageOpts)
	if err != nil {
		log.Fatalf("Failed to bootstrap async task layer: %v", err)
	}

	toolManager := tool.NewManager()
	regOpts := []any{tools.WithImageHandlerOptions(imageOpts...)}
	if dispatcher != nil {
		regOpts = append(regOpts, tools.WithAsync(&tools.AsyncBundle{
			Store:      taskStore,
			Dispatcher: dispatcher,
		}))
	}
	if err := tools.RegisterAll(toolManager, openaiProvider, openaiProvider, regOpts...); err != nil {
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
	serverOpts := []server.Option{
		server.WithLogger(logger),
		server.WithCancelRegistry(mcpHandler.Cancellations()),
	}
	if dispatcher != nil {
		// The dispatcher implements server.AbandonHook. Wiring it lets
		// the engine call AbandonRunning("shutdown") before draining
		// inflight workers, so async tasks unwind via ctx instead of
		// being silently severed when the transport closes.
		serverOpts = append(serverOpts, server.WithAbandonHook(dispatcher))
	}
	eng := server.New(
		transport.NewStdio(),
		mcpHandler,
		server.Config{
			MaxConcurrentRequests: cfg.MaxConcurrentRequests,
			RequestQueueSize:      cfg.RequestQueueSize,
			RequestTimeout:        cfg.RequestTimeout,
			ShutdownTimeout:       cfg.ShutdownTimeout,
		},
		serverOpts...,
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
		"task_store_enabled", cfg.TaskStoreEnabled,
		"task_disk_persist", cfg.TaskDiskPersist,
	)
	runErr := eng.Run(ctx)

	// Graceful tear-down of the async task layer. The server engine
	// already fired AbandonHook before returning, so workers are
	// unwinding; Close blocks until they all exit. Order matters:
	// dispatcher first (writes terminal transitions), then store
	// (flushes any pending manifest writes via the OnTransition hook
	// that dispatcher's last calls just fired).
	if dispatcher != nil {
		_ = dispatcher.Close()
	}
	if taskStore != nil {
		_ = taskStore.Close()
	}

	if runErr != nil {
		logger.Error("server.exit", "err", runErr.Error())
		os.Exit(1)
	}
	logger.Info("server.exit", "completed", eng.Completed(), "fallback", eng.FallbackCount())
}

// bootstrapTaskstore constructs the async task layer per cfg. Returns
// nil, nil, nil when TaskStoreEnabled is false (the caller skips
// registering async tools entirely). Returns a non-nil error only when
// disk persistence is requested but the configured directory cannot be
// initialized — a fatal misconfiguration that must surface before the
// recv loop opens.
func bootstrapTaskstore(cfg *config.Config, logger *slog.Logger, prov provider.ImageProvider, imageOpts []tools.HandlerOption) (taskstore.Store, *tools.Dispatcher, error) {
	if !cfg.TaskStoreEnabled {
		logger.Info("server.task_store.disabled")
		return nil, nil, nil
	}

	var store taskstore.Store
	if cfg.TaskDiskPersist {
		dir := taskstoreDir(cfg.OutputDir)
		ds, err := taskstore.NewDisk(taskstore.DiskConfig{
			Dir:         dir,
			MaxQueued:   cfg.TaskMaxQueued,
			MaxRetained: cfg.TaskMaxRetained,
			Logger:      logger,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("tasks directory %q: %w", dir, err)
		}
		logger.Info("server.task_store.disk_ready",
			"dir", dir,
			"max_queued", cfg.TaskMaxQueued,
			"max_retained", cfg.TaskMaxRetained,
			"ttl", cfg.TaskTTL.String(),
		)
		store = ds
	} else {
		store = taskstore.NewMemory(taskstore.MemoryConfig{
			MaxQueued:   cfg.TaskMaxQueued,
			MaxRetained: cfg.TaskMaxRetained,
		})
		logger.Info("server.task_store.memory_only",
			"max_queued", cfg.TaskMaxQueued,
			"max_retained", cfg.TaskMaxRetained,
		)
	}

	disp, err := tools.NewDispatcher(tools.DispatcherConfig{
		Store:          store,
		Provider:       prov,
		Workers:        cfg.MaxConcurrentRequests,
		QueueSize:      cfg.TaskMaxQueued,
		Logger:         logger,
		HandlerOptions: imageOpts,
	})
	if err != nil {
		_ = store.Close()
		return nil, nil, fmt.Errorf("dispatcher: %w", err)
	}
	return store, disp, nil
}

// taskstoreDir derives the per-deployment manifest directory. We hang
// it off the configured OutputDir so operators only authorize one
// filesystem location; if OutputDir is unset we fall back to the same
// $TMPDIR/openpic-mcp legacy root the image-saving code uses, so the
// async layer follows the same data-locality story as everything else.
func taskstoreDir(outputDir string) string {
	base := outputDir
	if base == "" {
		base = filepath.Join(os.TempDir(), "openpic-mcp")
	}
	return filepath.Join(base, "tasks")
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
