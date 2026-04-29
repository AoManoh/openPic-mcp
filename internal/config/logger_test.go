package config

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLogger_DefaultsAreInfoText(t *testing.T) {
	var buf bytes.Buffer
	cfg := &Config{}
	logger := NewLoggerWithWriter(cfg, &buf)
	logger.Debug("hidden")
	logger.Info("visible", "k", "v")

	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Errorf("debug message leaked at info level: %q", out)
	}
	if !strings.Contains(out, "visible") {
		t.Errorf("info message missing: %q", out)
	}
	if !strings.Contains(out, "k=v") {
		t.Errorf("text formatter expected (key=value), got %q", out)
	}
}

func TestNewLogger_JsonFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := &Config{LogFormat: "json", LogLevel: "info"}
	logger := NewLoggerWithWriter(cfg, &buf)
	logger.Info("hello", "id", 1)

	out := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(out, "{") || !strings.HasSuffix(out, "}") {
		t.Errorf("expected JSON-encoded record, got %q", out)
	}
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Errorf("expected msg field, got %q", out)
	}
}

func TestNewLogger_LogLevelFilters(t *testing.T) {
	cases := []struct {
		level   string
		want    slog.Level
		exclude []slog.Level
	}{
		{level: "debug", want: slog.LevelDebug},
		{level: "info", want: slog.LevelInfo, exclude: []slog.Level{slog.LevelDebug}},
		{level: "warn", want: slog.LevelWarn, exclude: []slog.Level{slog.LevelInfo}},
		{level: "warning", want: slog.LevelWarn, exclude: []slog.Level{slog.LevelInfo}},
		{level: "error", want: slog.LevelError, exclude: []slog.Level{slog.LevelWarn}},
		{level: "ERROR", want: slog.LevelError},
	}
	for _, tt := range cases {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := &Config{LogLevel: tt.level}
			logger := NewLoggerWithWriter(cfg, &buf)

			// At-or-above level must produce output; explicit excludes
			// must be silenced.
			logger.Log(context.Background(), tt.want, "wanted", "level", tt.level)
			if !strings.Contains(buf.String(), "wanted") {
				t.Errorf("at-level %s: missing %q", tt.level, buf.String())
			}
			for _, ex := range tt.exclude {
				buf.Reset()
				logger.Log(context.Background(), ex, "blocked", "level", tt.level)
				if strings.Contains(buf.String(), "blocked") {
					t.Errorf("level %s leaked %v message: %q", tt.level, ex, buf.String())
				}
			}
		})
	}
}

func TestNewLogger_UnknownLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	cfg := &Config{LogLevel: "verbose"}
	logger := NewLoggerWithWriter(cfg, &buf)

	logger.Debug("hidden")
	logger.Info("visible")
	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Errorf("debug leaked under unknown level fallback: %q", out)
	}
	if !strings.Contains(out, "visible") {
		t.Errorf("info missing: %q", out)
	}
}

func TestNewLogger_NilCfgIsSafe(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithWriter(nil, &buf)
	logger.Info("ok")
	if !strings.Contains(buf.String(), "ok") {
		t.Errorf("nil cfg path produced no output: %q", buf.String())
	}
}
