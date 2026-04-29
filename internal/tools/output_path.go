package tools

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// outputPathPolicy is the resolved deployment-or-call policy the tool
// layer uses when deciding where to persist generated images. It is
// produced by merging a deployment-level fallback (derived from
// config.Config) with optional per-call overrides parsed from MCP tool
// arguments. An empty Dir means "fall back to the OS temp dir under
// openpic-mcp/", preserving the pre-P1 behaviour for callers that have
// not opted into deterministic output paths yet.
type outputPathPolicy struct {
	Dir       string
	Prefix    string
	Overwrite bool
}

const (
	// outputFilenamePrefixMaxLen caps user-supplied prefixes to a length
	// that still leaves room for the timestamp + random suffix without
	// running into platform path-length limits.
	outputFilenamePrefixMaxLen = 32

	// defaultLegacyTempSubdir is appended to os.TempDir() when no
	// explicit output directory was configured, matching the legacy
	// path used by saveBase64Image so existing deployments keep working.
	defaultLegacyTempSubdir = "openpic-mcp"

	// outputFilenameMaxOverwriteRetries bounds the number of numeric
	// suffix retries we attempt when overwrite=false and a candidate
	// file path already exists. Random suffix collisions are extremely
	// rare; this guard exists purely so an unexpected loop terminates.
	outputFilenameMaxOverwriteRetries = 9
)

// outputFilenamePrefixRE restricts prefixes to a safe identifier set so
// they cannot smuggle directory separators, leading dots or whitespace
// into the final file path.
var outputFilenamePrefixRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateFilenamePrefix returns an error when the prefix is empty after
// trimming, exceeds outputFilenamePrefixMaxLen, contains characters
// outside the safe identifier set, or starts with a dot. An empty input
// is reported explicitly rather than silently accepted because callers
// always have a non-empty fallback to use when no override is supplied.
func validateFilenamePrefix(s string) error {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return fmt.Errorf("filename_prefix is empty after trimming")
	}
	if len(trimmed) > outputFilenamePrefixMaxLen {
		return fmt.Errorf("filename_prefix length %d exceeds %d", len(trimmed), outputFilenamePrefixMaxLen)
	}
	if strings.HasPrefix(trimmed, ".") {
		return fmt.Errorf("filename_prefix must not start with '.'")
	}
	if !outputFilenamePrefixRE.MatchString(trimmed) {
		return fmt.Errorf("filename_prefix %q contains characters outside [A-Za-z0-9._-]", trimmed)
	}
	return nil
}

// validateOutputDir verifies that the directory candidate is an absolute,
// non-traversal path. The directory is created on demand by the writer,
// so existence is not required up front; callers that pass a relative
// path or one that contains ".." segments get an immediate rejection
// before any provider work happens.
//
// Traversal segments are checked on the *raw* input rather than the
// filepath.Clean result, since cleaning silently rewrites e.g.
// "/tmp/foo/../x" into "/tmp/x" and would otherwise let callers smuggle
// intent past the guard.
func validateOutputDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	for _, seg := range strings.Split(filepath.ToSlash(dir), "/") {
		if seg == ".." {
			return "", fmt.Errorf("output_dir must not contain traversal segments: %q", dir)
		}
	}
	cleaned := filepath.Clean(dir)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("output_dir must be an absolute path: %q", dir)
	}
	return cleaned, nil
}

// resolveOutputPolicy merges per-call overrides with a deployment-level
// fallback policy and a tool-context default prefix, returning the
// resolved policy the writer should act on. Empty per-call values defer
// to the fallback; an empty fallback prefix in turn defers to the
// supplied defaultPrefix (typically "generate" or "edit"). The function
// is pure: it does no I/O so unit tests can drive every branch without
// touching the filesystem.
func resolveOutputPolicy(
	overrideDir string,
	overridePrefix string,
	overrideOverwrite *bool,
	defaultPrefix string,
	fallback outputPathPolicy,
) (outputPathPolicy, error) {
	dir := strings.TrimSpace(overrideDir)
	if dir == "" {
		dir = strings.TrimSpace(fallback.Dir)
	}
	cleanedDir, err := validateOutputDir(dir)
	if err != nil {
		return outputPathPolicy{}, err
	}

	prefix := strings.TrimSpace(overridePrefix)
	if prefix == "" {
		prefix = strings.TrimSpace(fallback.Prefix)
	}
	if prefix == "" {
		prefix = defaultPrefix
	}
	if err := validateFilenamePrefix(prefix); err != nil {
		return outputPathPolicy{}, err
	}

	overwrite := fallback.Overwrite
	if overrideOverwrite != nil {
		overwrite = *overrideOverwrite
	}

	return outputPathPolicy{
		Dir:       cleanedDir,
		Prefix:    prefix,
		Overwrite: overwrite,
	}, nil
}

// outputDirPath returns the directory the writer should target, falling
// back to the OS temp dir under defaultLegacyTempSubdir when no explicit
// directory has been resolved. The directory is not created here; that
// is the writer's responsibility so error reporting stays in one place.
func outputDirPath(policy outputPathPolicy) string {
	if policy.Dir != "" {
		return policy.Dir
	}
	return filepath.Join(os.TempDir(), defaultLegacyTempSubdir)
}

// buildOutputFilename composes "<prefix>-YYYYMMDD-HHMMSS-<8charRand>.<ext>".
// The timestamp uses UTC so file names compare consistently across hosts;
// the random suffix is 4 bytes (8 hex chars) which is long enough to make
// collisions effectively impossible for the parallel-generation workloads
// we expect, while still keeping names short.
func buildOutputFilename(prefix, ext string, ts time.Time) (string, error) {
	if ext == "" {
		ext = "png"
	}
	rnd := make([]byte, 4)
	if _, err := rand.Read(rnd); err != nil {
		return "", fmt.Errorf("failed to generate random filename suffix: %w", err)
	}
	return fmt.Sprintf("%s-%s-%s.%s",
		prefix,
		ts.UTC().Format("20060102-150405"),
		hex.EncodeToString(rnd),
		ext,
	), nil
}

// writeImageBytes persists data under the policy-resolved directory and
// returns the absolute file path actually written. When the policy has
// Overwrite=false (the default) and a candidate path already exists, the
// writer appends "-2", "-3", ... before the extension up to
// outputFilenameMaxOverwriteRetries attempts. The directory is created
// on demand with 0o700 permissions; files are written with 0o600 so
// generated images are never world-readable by accident.
func writeImageBytes(policy outputPathPolicy, data []byte, format string) (string, error) {
	dir := outputDirPath(policy)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create output directory %q: %w", dir, err)
	}

	ext := extensionForFormat(format)
	if ext == "" {
		ext = "png"
	}

	base, err := buildOutputFilename(policy.Prefix, ext, time.Now())
	if err != nil {
		return "", err
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !policy.Overwrite {
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}

	candidate := filepath.Join(dir, base)
	for attempt := 1; ; attempt++ {
		f, err := os.OpenFile(candidate, flag, 0o600)
		if err == nil {
			if _, werr := f.Write(data); werr != nil {
				_ = f.Close()
				return "", fmt.Errorf("failed to write output file %q: %w", candidate, werr)
			}
			if cerr := f.Close(); cerr != nil {
				return "", fmt.Errorf("failed to close output file %q: %w", candidate, cerr)
			}
			return candidate, nil
		}

		if policy.Overwrite || !os.IsExist(err) {
			return "", fmt.Errorf("failed to open output file %q: %w", candidate, err)
		}
		if attempt > outputFilenameMaxOverwriteRetries {
			return "", fmt.Errorf("output filename collisions exceeded retry budget for %q", base)
		}
		candidate = filepath.Join(dir, withCountSuffix(base, attempt+1))
	}
}

// withCountSuffix inserts "-N" before the file extension. When the input
// has no extension the suffix is appended at the end. The function is
// kept pure so unit tests can verify naming behaviour without touching
// the filesystem.
func withCountSuffix(name string, n int) string {
	dot := strings.LastIndex(name, ".")
	if dot <= 0 {
		return fmt.Sprintf("%s-%d", name, n)
	}
	return fmt.Sprintf("%s-%d%s", name[:dot], n, name[dot:])
}
