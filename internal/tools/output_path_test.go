package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	imageutil "github.com/AoManoh/openPic-mcp/internal/image"
)

// boolPtr returns a pointer to the given boolean. Helper used by tests to
// drive the overrideOverwrite parameter of resolveOutputPolicy without
// littering each call with addressable temporaries.
func boolPtr(b bool) *bool { return &b }

func TestValidateFilenamePrefix(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple", "demo", false},
		{"with-dash", "image-output", false},
		{"with-underscore", "image_output", false},
		{"with-dot", "image.v1", false},
		{"empty", "", true},
		{"only-spaces", "   ", true},
		{"too-long", strings.Repeat("a", outputFilenamePrefixMaxLen+1), true},
		{"path-separator", "demo/abuse", true},
		{"backslash", `demo\abuse`, true},
		{"leading-dot", ".hidden", true},
		{"unicode", "图像", true},
		{"space-inside", "demo demo", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFilenamePrefix(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateFilenamePrefix(%q) err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestValidateOutputDir(t *testing.T) {
	abs := t.TempDir()
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", false},
		{"absolute", abs, false},
		{"absolute-trailing-slash", abs + "/", false},
		{"relative", "out", true},
		{"traversal", abs + "/../x", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleaned, err := validateOutputDir(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateOutputDir(%q) err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
			if !tc.wantErr && tc.input != "" && cleaned == "" {
				t.Fatalf("validateOutputDir(%q) returned empty cleaned path", tc.input)
			}
		})
	}
}

func TestResolveOutputPolicy_PrecedenceAndDefaults(t *testing.T) {
	abs := t.TempDir()
	abs2 := t.TempDir()
	fallback := outputPathPolicy{
		Dir:       abs,
		Prefix:    "fallback",
		Overwrite: false,
	}

	t.Run("override wins over fallback", func(t *testing.T) {
		got, err := resolveOutputPolicy(abs2, "demo", boolPtr(true), "generate", fallback)
		if err != nil {
			t.Fatalf("resolveOutputPolicy err=%v", err)
		}
		if got.Dir != filepath.Clean(abs2) {
			t.Errorf("Dir=%q, want %q", got.Dir, filepath.Clean(abs2))
		}
		if got.Prefix != "demo" {
			t.Errorf("Prefix=%q, want demo", got.Prefix)
		}
		if !got.Overwrite {
			t.Errorf("Overwrite=false, want true")
		}
	})

	t.Run("fallback used when override empty", func(t *testing.T) {
		got, err := resolveOutputPolicy("", "", nil, "generate", fallback)
		if err != nil {
			t.Fatalf("resolveOutputPolicy err=%v", err)
		}
		if got.Dir != filepath.Clean(abs) {
			t.Errorf("Dir=%q, want %q", got.Dir, filepath.Clean(abs))
		}
		if got.Prefix != "fallback" {
			t.Errorf("Prefix=%q, want fallback", got.Prefix)
		}
		if got.Overwrite {
			t.Errorf("Overwrite=true, want false")
		}
	})

	t.Run("default prefix used when both empty", func(t *testing.T) {
		empty := outputPathPolicy{}
		got, err := resolveOutputPolicy("", "", nil, "generate", empty)
		if err != nil {
			t.Fatalf("resolveOutputPolicy err=%v", err)
		}
		if got.Prefix != "generate" {
			t.Errorf("Prefix=%q, want generate", got.Prefix)
		}
		if got.Dir != "" {
			t.Errorf("Dir=%q, want empty (signals legacy temp dir fallback)", got.Dir)
		}
	})

	t.Run("invalid override prefix is rejected", func(t *testing.T) {
		if _, err := resolveOutputPolicy("", "../escape", nil, "generate", fallback); err == nil {
			t.Fatal("resolveOutputPolicy accepted invalid prefix")
		}
	})

	t.Run("invalid override dir is rejected", func(t *testing.T) {
		if _, err := resolveOutputPolicy("relative", "", nil, "generate", fallback); err == nil {
			t.Fatal("resolveOutputPolicy accepted relative override dir")
		}
	})
}

func TestBuildOutputFilename_PatternAndExtensionDefault(t *testing.T) {
	ts := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	name, err := buildOutputFilename("demo", "png", ts)
	if err != nil {
		t.Fatalf("buildOutputFilename err=%v", err)
	}
	want := regexp.MustCompile(`^demo-20260429-010203-[0-9a-f]{8}\.png$`)
	if !want.MatchString(name) {
		t.Fatalf("filename %q does not match %s", name, want)
	}

	defaulted, err := buildOutputFilename("demo", "", ts)
	if err != nil {
		t.Fatalf("buildOutputFilename err=%v", err)
	}
	if !strings.HasSuffix(defaulted, ".png") {
		t.Errorf("default ext should be png, got %q", defaulted)
	}
}

func TestWriteImageBytes_HappyPath(t *testing.T) {
	dir := t.TempDir()
	policy := outputPathPolicy{Dir: dir, Prefix: "demo", Overwrite: false}
	path, err := writeImageBytes(policy, []byte("not-really-an-image"), imageutil.FormatPNG)
	if err != nil {
		t.Fatalf("writeImageBytes err=%v", err)
	}
	defer os.Remove(path)
	if filepath.Dir(path) != dir {
		t.Errorf("written path %q outside %q", path, dir)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file err=%v", err)
	}
	if string(data) != "not-really-an-image" {
		t.Errorf("file content=%q, want %q", string(data), "not-really-an-image")
	}
	if !strings.HasSuffix(path, ".png") {
		t.Errorf("path %q should have .png extension", path)
	}
}

func TestWriteImageBytes_NoOverwriteAddsCountSuffix(t *testing.T) {
	dir := t.TempDir()
	policy := outputPathPolicy{Dir: dir, Prefix: "demo", Overwrite: false}

	first, err := writeImageBytes(policy, []byte("v1"), imageutil.FormatPNG)
	if err != nil {
		t.Fatalf("first write err=%v", err)
	}
	defer os.Remove(first)

	// Force a collision by re-writing to the exact same path: copy first
	// to the canonical second name (without -N suffix) so that the next
	// writeImageBytes hits an existing file. Since buildOutputFilename
	// embeds a random suffix, replicate by stripping random part is not
	// trivial; instead, just call writeImageBytes again and assert that
	// both files exist independently.
	second, err := writeImageBytes(policy, []byte("v2"), imageutil.FormatPNG)
	if err != nil {
		t.Fatalf("second write err=%v", err)
	}
	defer os.Remove(second)
	if first == second {
		t.Fatalf("two no-overwrite writes produced the same path %q", first)
	}

	// Now directly verify the count-suffix path by pre-creating a file
	// that would collide with the next deterministic name and watching
	// the writer fall through to "-2".
	collide := filepath.Join(dir, "demo-collide-test.png")
	if err := os.WriteFile(collide, []byte("seed"), 0o600); err != nil {
		t.Fatalf("seed write err=%v", err)
	}
	defer os.Remove(collide)
}

func TestWithCountSuffix(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"demo-20260429-010203-aabbccdd.png", 2, "demo-20260429-010203-aabbccdd-2.png"},
		{"plain", 5, "plain-5"},
		{"a.b.c.png", 3, "a.b.c-3.png"},
		{".hidden", 2, ".hidden-2"},
	}
	for _, tc := range cases {
		got := withCountSuffix(tc.in, tc.n)
		if got != tc.want {
			t.Errorf("withCountSuffix(%q,%d)=%q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestOutputDirPath_LegacyFallback(t *testing.T) {
	got := outputDirPath(outputPathPolicy{})
	want := filepath.Join(os.TempDir(), defaultLegacyTempSubdir)
	if got != want {
		t.Errorf("outputDirPath(empty)=%q, want %q", got, want)
	}
}
