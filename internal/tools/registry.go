package tools

import (
	"fmt"

	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/internal/service/tool"
	"github.com/AoManoh/openPic-mcp/internal/taskstore"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// AsyncBundle bundles the dependencies the async tools need. Pass it to
// [RegisterAll] via [WithAsync] when the deployment has the async layer
// enabled. A nil bundle skips registration of submit_image_task /
// get_task_result / list_tasks / cancel_task; existing sync tools
// remain available either way.
type AsyncBundle struct {
	Store      taskstore.Store
	Dispatcher taskDispatcher
}

// RegisterOption mutates the registration plan.
type RegisterOption func(*registerOptions)

type registerOptions struct {
	imageOpts []HandlerOption
	async     *AsyncBundle
}

// WithImageHandlerOptions threads deployment-level output-path options
// into the sync image-producing handlers (generate_image, edit_image).
// Existing call sites that pass these positionally to RegisterAll keep
// working via the variadic-compatible bridge below.
func WithImageHandlerOptions(opts ...HandlerOption) RegisterOption {
	return func(o *registerOptions) { o.imageOpts = append(o.imageOpts, opts...) }
}

// WithAsync registers the four async task tools backed by the given
// store + dispatcher. Passing nil is a no-op so callers can pipe an
// optional config-driven bundle straight through.
func WithAsync(b *AsyncBundle) RegisterOption {
	return func(o *registerOptions) {
		if b != nil && b.Store != nil && b.Dispatcher != nil {
			o.async = b
		}
	}
}

// toolBinding pairs a tool definition with the handler factory used to wire
// it against the provider abstractions. Keeping this as an internal slice
// rather than a global mutable registry keeps registration deterministic and
// easy to audit when reviewing the MCP capability surface.
type toolBinding struct {
	def     types.Tool
	handler types.ToolHandler
}

// RegisterAll registers every tool exported by this package against the
// given tool.Manager. Both providers must be supplied; passing nil for a
// provider that a tool depends on yields an error so the failure is visible
// at startup rather than at the first MCP call.
//
// The function is intentionally the only place where tool definitions are
// enumerated: cmd/vision-mcp/main.go calls it once during boot, future
// transports (e.g. Streamable HTTP) can reuse it unchanged, and adding a
// new tool only requires extending the slice below.
//
// imageOpts (the trailing variadic) are forwarded only to the
// image-producing sync handlers (generate_image, edit_image). The
// preferred call shape is to pass [WithImageHandlerOptions] /
// [WithAsync] via the new [RegisterOption] varargs; the trailing slice
// is retained for backwards compatibility with existing call sites.
func RegisterAll(manager *tool.Manager, vp provider.VisionProvider, ip provider.ImageProvider, opts ...any) error {
	if manager == nil {
		return fmt.Errorf("tool manager is required")
	}
	if vp == nil {
		return fmt.Errorf("vision provider is required")
	}
	if ip == nil {
		return fmt.Errorf("image provider is required")
	}

	regOpts := registerOptions{}
	for _, raw := range opts {
		switch o := raw.(type) {
		case HandlerOption:
			regOpts.imageOpts = append(regOpts.imageOpts, o)
		case RegisterOption:
			if o != nil {
				o(&regOpts)
			}
		case nil:
			// permitted; treat as zero-value option
		default:
			return fmt.Errorf("RegisterAll: unsupported option type %T", raw)
		}
	}

	bindings := []toolBinding{
		{def: DescribeImageTool, handler: DescribeImageHandler(vp)},
		{def: CompareImagesTool, handler: CompareImagesHandler(vp)},
		{def: GenerateImageTool, handler: GenerateImageHandler(ip, regOpts.imageOpts...)},
		{def: EditImageTool, handler: EditImageHandler(ip, regOpts.imageOpts...)},
		{def: ListImageCapabilitiesTool, handler: ListImageCapabilitiesHandler()},
	}

	if regOpts.async != nil {
		bindings = append(bindings,
			toolBinding{def: SubmitImageTaskTool, handler: SubmitImageTaskHandler(regOpts.async.Store, regOpts.async.Dispatcher)},
			toolBinding{def: GetTaskResultTool, handler: GetTaskResultHandler(regOpts.async.Store)},
			toolBinding{def: ListTasksTool, handler: ListTasksHandler(regOpts.async.Store)},
			toolBinding{def: CancelTaskTool, handler: CancelTaskHandler(regOpts.async.Store)},
		)
	}

	for _, b := range bindings {
		if err := manager.Register(b.def, b.handler); err != nil {
			return fmt.Errorf("register %s: %w", b.def.Name, err)
		}
	}
	return nil
}

// allKnownTools returns every tool definition this package can register,
// flattened across the sync (always-on) and async (gated on AsyncBundle)
// surfaces. It is the single source of truth consumed by schema_test.go
// so that adding a new tool to RegisterAll without also schema-validating
// it surfaces as a test failure.
//
// CONTRACT — when you add a new exported `var FooTool = types.Tool{...}`:
//
//  1. Wire it into RegisterAll above (sync slice or async slice).
//  2. Append it here in the SAME group order.
//
// The meta-test TestAllKnownToolsCoversRegisterAll walks the bindings
// list above by calling RegisterAll with stub providers and an enabled
// async bundle, then asserts the registered names equal the names
// returned here. That guarantees the two lists cannot drift.
func allKnownTools() []types.Tool {
	return []types.Tool{
		// Sync tools — always registered.
		DescribeImageTool,
		CompareImagesTool,
		GenerateImageTool,
		EditImageTool,
		ListImageCapabilitiesTool,
		// Async tools — registered when an AsyncBundle is provided.
		SubmitImageTaskTool,
		GetTaskResultTool,
		ListTasksTool,
		CancelTaskTool,
	}
}
