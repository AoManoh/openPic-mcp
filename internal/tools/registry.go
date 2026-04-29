package tools

import (
	"fmt"

	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/internal/service/tool"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

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
// The variadic imageOpts are forwarded only to the image-producing
// handlers (generate_image, edit_image). describe_image and
// compare_images do not persist images so they ignore output-path
// options. Passing no options preserves the pre-P1 behaviour: writes go
// to os.TempDir()/openpic-mcp/ with a randomised name.
func RegisterAll(manager *tool.Manager, vp provider.VisionProvider, ip provider.ImageProvider, imageOpts ...HandlerOption) error {
	if manager == nil {
		return fmt.Errorf("tool manager is required")
	}
	if vp == nil {
		return fmt.Errorf("vision provider is required")
	}
	if ip == nil {
		return fmt.Errorf("image provider is required")
	}

	bindings := []toolBinding{
		{def: DescribeImageTool, handler: DescribeImageHandler(vp)},
		{def: CompareImagesTool, handler: CompareImagesHandler(vp)},
		{def: GenerateImageTool, handler: GenerateImageHandler(ip, imageOpts...)},
		{def: EditImageTool, handler: EditImageHandler(ip, imageOpts...)},
		{def: ListImageCapabilitiesTool, handler: ListImageCapabilitiesHandler()},
	}

	for _, b := range bindings {
		if err := manager.Register(b.def, b.handler); err != nil {
			return fmt.Errorf("register %s: %w", b.def.Name, err)
		}
	}
	return nil
}
