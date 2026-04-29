package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewLogger constructs a *slog.Logger from the project's configuration. It
// always writes to stderr because stdout is reserved for the MCP JSON-RPC
// channel; emitting a single log line on stdout would corrupt every client.
//
// Format selection mirrors [Config.LogFormat] (`text` or `json`); level
// mirrors [Config.LogLevel] using slog's standard names. Unknown levels
// fall back to info so a misconfigured deployment still produces logs
// rather than silently swallowing them.
//
// The returned logger is safe for concurrent use; the engine fan-out and
// the recv loop share the same logger instance.
func NewLogger(cfg *Config) *slog.Logger {
	return NewLoggerWithWriter(cfg, os.Stderr)
}

// NewLoggerWithWriter is the seam used by tests to capture log output. It
// behaves exactly like [NewLogger] but writes to the provided io.Writer
// instead of os.Stderr.
func NewLoggerWithWriter(cfg *Config, w io.Writer) *slog.Logger {
	if w == nil {
		w = io.Discard
	}
	level := parseLogLevel(cfgLogLevelOrDefault(cfg))
	handlerOpts := &slog.HandlerOptions{Level: level}
	switch cfgLogFormatOrDefault(cfg) {
	case "json":
		return slog.New(slog.NewJSONHandler(w, handlerOpts))
	default:
		return slog.New(slog.NewTextHandler(w, handlerOpts))
	}
}

func cfgLogLevelOrDefault(cfg *Config) string {
	if cfg == nil || cfg.LogLevel == "" {
		return DefaultLogLevel
	}
	return cfg.LogLevel
}

func cfgLogFormatOrDefault(cfg *Config) string {
	if cfg == nil || cfg.LogFormat == "" {
		return DefaultLogFormat
	}
	return cfg.LogFormat
}

// parseLogLevel maps a textual level (debug/info/warn/error) to slog's
// numeric level. Unknown values are treated as info because returning an
// error here would force every caller to plumb startup error paths just
// for log configuration.
func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		// Surface the misconfiguration on stderr so operators notice it,
		// but don't crash; logs are not the right place to be strict.
		fmt.Fprintf(os.Stderr, "openpic-mcp: unknown log level %q, falling back to info\n", s)
		return slog.LevelInfo
	}
}
