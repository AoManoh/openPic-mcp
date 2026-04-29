package tools

// ImageHandlerOptions captures the deployment-level defaults the
// generate_image and edit_image handlers consult when the caller does
// not provide a per-call override. The zero value is safe: an empty
// DefaultOutputDir falls back to the legacy temp directory and a
// non-positive MaxInlinePayloadBytes is interpreted as "use the safe
// 1 MiB default" rather than "disable the guard".
//
// main.go derives one of these from config.Config and threads it
// through tools.RegisterAll; tests can also build one directly without
// ever constructing a Config so the handler factories stay decoupled
// from the configuration package.
type ImageHandlerOptions struct {
	DefaultOutputDir      string
	DefaultFilenamePrefix string
	DefaultOverwrite      bool
	MaxInlinePayloadBytes int64
}

// defaultImageHandlerMaxInlinePayloadBytes mirrors
// config.DefaultMaxInlinePayloadBytes. We duplicate the constant rather
// than importing the config package to keep the tools layer free of
// dependency on configuration loading; the value is reconciled in main.go
// when the deployment-level options are constructed from config.Config.
const defaultImageHandlerMaxInlinePayloadBytes = 1 << 20

// HandlerOption is the functional-option closure used to assemble an
// ImageHandlerOptions without exposing positional construction or
// requiring callers to know every field up front.
type HandlerOption func(*ImageHandlerOptions)

// WithDefaultOutputDir sets the deployment-level output directory.
func WithDefaultOutputDir(dir string) HandlerOption {
	return func(o *ImageHandlerOptions) { o.DefaultOutputDir = dir }
}

// WithDefaultFilenamePrefix sets the deployment-level filename prefix.
func WithDefaultFilenamePrefix(prefix string) HandlerOption {
	return func(o *ImageHandlerOptions) { o.DefaultFilenamePrefix = prefix }
}

// WithDefaultOverwrite sets the deployment-level overwrite policy.
func WithDefaultOverwrite(b bool) HandlerOption {
	return func(o *ImageHandlerOptions) { o.DefaultOverwrite = b }
}

// WithMaxInlinePayloadBytes overrides the inline payload guard. Values
// less than or equal to zero are ignored so callers cannot accidentally
// disable the guard by passing a stale or zeroed configuration value.
func WithMaxInlinePayloadBytes(n int64) HandlerOption {
	return func(o *ImageHandlerOptions) {
		if n > 0 {
			o.MaxInlinePayloadBytes = n
		}
	}
}

// applyImageHandlerOptions returns a fully-defaulted options struct.
func applyImageHandlerOptions(opts []HandlerOption) ImageHandlerOptions {
	out := ImageHandlerOptions{
		MaxInlinePayloadBytes: defaultImageHandlerMaxInlinePayloadBytes,
	}
	for _, fn := range opts {
		if fn == nil {
			continue
		}
		fn(&out)
	}
	if out.MaxInlinePayloadBytes <= 0 {
		out.MaxInlinePayloadBytes = defaultImageHandlerMaxInlinePayloadBytes
	}
	return out
}

// fallbackPolicy returns the outputPathPolicy that resolveOutputPolicy
// should treat as the deployment-level default. The returned value is
// not validated here; resolveOutputPolicy validates the merged policy
// once user overrides have been folded in.
func (o ImageHandlerOptions) fallbackPolicy() outputPathPolicy {
	return outputPathPolicy{
		Dir:       o.DefaultOutputDir,
		Prefix:    o.DefaultFilenamePrefix,
		Overwrite: o.DefaultOverwrite,
	}
}
