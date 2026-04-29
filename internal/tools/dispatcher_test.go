package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/internal/taskstore"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// fakeImageProvider is a [provider.ImageProvider] for async tests. It
// can be configured to delay, succeed (returning a tiny PNG), or fail.
// All call counts are atomic so concurrent tests can assert on them.
type fakeImageProvider struct {
	generateCalls atomic.Int32
	editCalls     atomic.Int32

	delay   time.Duration
	failGen error
	failEdt error

	// hold lets a test pause the provider mid-call so it can drive the
	// cancel/abandon timing window deterministically.
	hold chan struct{}
}

func newFakeImageProvider() *fakeImageProvider { return &fakeImageProvider{} }

func (f *fakeImageProvider) Name() string { return "fake" }

// tinyPNG is a 1x1 transparent PNG (re-used across tests so we don't
// re-encode bytes for every fake response).
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func tinyPNGDataURI() string {
	const prefix = "data:image/png;base64,"
	enc := jsonBase64(tinyPNG)
	return prefix + enc
}

func jsonBase64(b []byte) string {
	// Use stdlib base64 (avoid pulling encoding/base64 directly to keep
	// imports tidy; we already have encoding/json in the file).
	v, _ := json.Marshal(b)
	// json.Marshal of []byte produces a quoted base64 string; strip quotes.
	s := string(v)
	return strings.Trim(s, `"`)
}

func (f *fakeImageProvider) GenerateImage(ctx context.Context, _ *provider.GenerateImageRequest) (*provider.GenerateImageResponse, error) {
	f.generateCalls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.failGen != nil {
		return nil, f.failGen
	}
	return &provider.GenerateImageResponse{
		Created: time.Now().Unix(),
		Images: []provider.GeneratedImage{
			{B64JSON: tinyPNGDataURI()},
		},
	}, nil
}

func (f *fakeImageProvider) EditImage(ctx context.Context, _ *provider.EditImageRequest) (*provider.EditImageResponse, error) {
	f.editCalls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.failEdt != nil {
		return nil, f.failEdt
	}
	return &provider.EditImageResponse{
		Created: time.Now().Unix(),
		Images: []provider.GeneratedImage{
			{B64JSON: tinyPNGDataURI()},
		},
	}, nil
}

// dispatcherFixture wires a memory taskstore + fakeImageProvider +
// Dispatcher with output-dir set to t.TempDir() so handler invocations
// produce real files we can inspect.
type dispatcherFixture struct {
	store    *taskstore.MemoryStore
	provider *fakeImageProvider
	disp     *Dispatcher
	tempDir  string
}

func newDispatcherFixture(t *testing.T, cfg DispatcherConfig) *dispatcherFixture {
	t.Helper()
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	provider := newFakeImageProvider()
	tmpDir := t.TempDir()
	cfg.Store = store
	cfg.Provider = provider
	if cfg.HandlerOptions == nil {
		cfg.HandlerOptions = []HandlerOption{
			WithDefaultOutputDir(tmpDir),
			WithDefaultFilenamePrefix("test"),
		}
	}
	disp, err := NewDispatcher(cfg)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	t.Cleanup(func() { _ = disp.Close() })
	return &dispatcherFixture{
		store:    store,
		provider: provider,
		disp:     disp,
		tempDir:  tmpDir,
	}
}

// waitState polls store.Get until task reaches the wanted state or
// timeout fires. Used by tests that don't want to use the long-poll
// tool path.
func waitState(t *testing.T, store taskstore.Store, id taskstore.TaskID, want taskstore.State, timeout time.Duration) taskstore.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got, err := store.Get(context.Background(), id)
		if err == nil && got.State == want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	got, _ := store.Get(context.Background(), id)
	t.Fatalf("task %s did not reach %q in %s; last state=%q err=%v", id, want, timeout, got.State, got.Error)
	return got
}

// TestDispatch_BasicSuccessFlow verifies queue → run → complete with a
// real saved file extracted into the task Result.
func TestDispatch_BasicSuccessFlow(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 2, QueueSize: 16})

	id, err := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{
		Prompt: "a cat",
		Model:  "test-model",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := f.disp.Dispatch(context.Background(), id, taskstore.KindGenerateImage, map[string]any{
		"prompt":          "a cat",
		"output_dir":      f.tempDir,
		"filename_prefix": "test",
		"response_format": "file_path",
	}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	final := waitState(t, f.store, id, taskstore.StateCompleted, 3*time.Second)
	if final.Result == nil {
		t.Fatal("expected Result to be populated")
	}
	if final.Result.FilePath == "" {
		t.Errorf("Result.FilePath empty")
	}
	if final.Result.Format != "png" {
		t.Errorf("Result.Format = %q, want png", final.Result.Format)
	}
	if final.Result.SizeBytes <= 0 {
		t.Errorf("Result.SizeBytes = %d, want >0", final.Result.SizeBytes)
	}
	if final.StartedAt.IsZero() {
		t.Error("StartedAt should be stamped")
	}
	if final.FinishedAt.IsZero() {
		t.Error("FinishedAt should be stamped")
	}
	if f.provider.generateCalls.Load() != 1 {
		t.Errorf("provider GenerateImage calls = %d, want 1", f.provider.generateCalls.Load())
	}
}

// TestDispatch_ProviderErrorMarksFailed verifies error path.
func TestDispatch_ProviderErrorMarksFailed(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 2, QueueSize: 16})
	f.provider.failGen = errors.New("upstream is down")

	id, _ := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "x"})
	if err := f.disp.Dispatch(context.Background(), id, taskstore.KindGenerateImage, map[string]any{
		"prompt":     "x",
		"output_dir": f.tempDir,
	}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	final := waitState(t, f.store, id, taskstore.StateFailed, 3*time.Second)
	if !strings.Contains(final.Error, "upstream is down") {
		t.Errorf("error should mention upstream failure, got %q", final.Error)
	}
}

// TestDispatch_CancelDuringRun: cancel_task fires while the worker is
// inside the provider call. The provider observes ctx cancellation and
// returns; the task ends up in StateCancelled with hint=client.
func TestDispatch_CancelDuringRun(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 2, QueueSize: 16})
	hold := make(chan struct{})
	f.provider.hold = hold // worker will block until cancelled

	id, _ := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})
	if err := f.disp.Dispatch(context.Background(), id, taskstore.KindGenerateImage, map[string]any{
		"prompt":     "p",
		"output_dir": f.tempDir,
	}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Wait until the task is running before cancelling.
	waitState(t, f.store, id, taskstore.StateRunning, 2*time.Second)

	if err := f.store.Cancel(context.Background(), id, "user"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	close(hold) // let the (now-cancelled) provider call return

	final := waitState(t, f.store, id, taskstore.StateCancelled, 2*time.Second)
	if final.CancelHint != "user" {
		t.Errorf("CancelHint = %q, want user", final.CancelHint)
	}
}

// TestDispatch_AbandonRunningOnShutdown: dispatcher.AbandonRunning
// transitions all running tasks to abandoned and unblocks workers.
func TestDispatch_AbandonRunningOnShutdown(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 4, QueueSize: 16})
	hold := make(chan struct{})
	f.provider.hold = hold

	const N = 3
	ids := make([]taskstore.TaskID, N)
	for i := range ids {
		id, _ := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})
		ids[i] = id
		if err := f.disp.Dispatch(context.Background(), id, taskstore.KindGenerateImage, map[string]any{
			"prompt":     "p",
			"output_dir": f.tempDir,
		}); err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
	}

	// Wait for all tasks to be running.
	for _, id := range ids {
		waitState(t, f.store, id, taskstore.StateRunning, 2*time.Second)
	}

	// Fire abandonment; this both transitions and cancels.
	f.disp.AbandonRunning("shutdown")
	close(hold)

	for _, id := range ids {
		final := waitState(t, f.store, id, taskstore.StateAbandoned, 2*time.Second)
		if final.CancelHint != "shutdown" {
			t.Errorf("%s: CancelHint = %q, want shutdown", id, final.CancelHint)
		}
	}
}

// TestDispatch_QueueFull synthetically saturates the dispatch queue and
// verifies the new dispatch returns ErrDispatcherQueueFull.
func TestDispatch_QueueFull(t *testing.T) {
	// 1 worker, 1 buffered slot — second dispatch will fail unless the
	// worker has picked the first job up. We pause the provider to keep
	// the worker busy.
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 1, QueueSize: 1})
	hold := make(chan struct{})
	f.provider.hold = hold
	defer close(hold)

	// First dispatch: occupies the worker (after pickup).
	id1, _ := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p1"})
	_ = f.disp.Dispatch(context.Background(), id1, taskstore.KindGenerateImage, map[string]any{"prompt": "p1", "output_dir": f.tempDir})
	// Wait until worker has picked it up.
	waitState(t, f.store, id1, taskstore.StateRunning, 2*time.Second)

	// Second dispatch: occupies the buffer slot.
	id2, _ := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p2"})
	if err := f.disp.Dispatch(context.Background(), id2, taskstore.KindGenerateImage, map[string]any{"prompt": "p2"}); err != nil {
		t.Fatalf("second dispatch should succeed (buffered): %v", err)
	}

	// Third dispatch: queue full.
	id3, _ := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p3"})
	err := f.disp.Dispatch(context.Background(), id3, taskstore.KindGenerateImage, map[string]any{"prompt": "p3"})
	if !errors.Is(err, ErrDispatcherQueueFull) {
		t.Fatalf("expected ErrDispatcherQueueFull, got %v", err)
	}
}

// TestDispatch_ClosedRejects verifies Dispatch after Close returns
// ErrDispatcherClosed.
func TestDispatch_ClosedRejects(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 2, QueueSize: 4})
	_ = f.disp.Close()

	id, _ := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})
	err := f.disp.Dispatch(context.Background(), id, taskstore.KindGenerateImage, map[string]any{"prompt": "p"})
	if !errors.Is(err, ErrDispatcherClosed) {
		t.Fatalf("expected ErrDispatcherClosed, got %v", err)
	}
}

// TestDispatch_HighConcurrency drives many tasks through the dispatcher
// in parallel under -race. All tasks must reach a terminal state.
func TestDispatch_HighConcurrency(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 8, QueueSize: 256})

	const N = 64
	var wg sync.WaitGroup
	wg.Add(N)
	ids := make([]taskstore.TaskID, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			id, err := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: fmt.Sprintf("p%d", i)})
			if err != nil {
				t.Errorf("submit: %v", err)
				return
			}
			ids[i] = id
			if err := f.disp.Dispatch(context.Background(), id, taskstore.KindGenerateImage, map[string]any{
				"prompt":     fmt.Sprintf("p%d", i),
				"output_dir": f.tempDir,
			}); err != nil {
				t.Errorf("dispatch: %v", err)
			}
		}(i)
	}
	wg.Wait()

	for _, id := range ids {
		if id == "" {
			continue
		}
		waitState(t, f.store, id, taskstore.StateCompleted, 5*time.Second)
	}
}

// TestNewDispatcher_RequiresStoreAndProvider locks the constructor
// validation path.
func TestNewDispatcher_RequiresStoreAndProvider(t *testing.T) {
	_, err := NewDispatcher(DispatcherConfig{Provider: newFakeImageProvider()})
	if err == nil || !strings.Contains(err.Error(), "Store") {
		t.Fatalf("missing store should fail; got %v", err)
	}
	_, err = NewDispatcher(DispatcherConfig{Store: taskstore.NewMemory(taskstore.MemoryConfig{})})
	if err == nil || !strings.Contains(err.Error(), "Provider") {
		t.Fatalf("missing provider should fail; got %v", err)
	}
}

// TestNewDispatcher_ClampsWorkers verifies clamping to upper bound.
func TestNewDispatcher_ClampsWorkers(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 9999, QueueSize: 99999})
	// We can't directly read the clamped values, but we can verify the
	// dispatcher is alive by submitting a quick task.
	id, _ := f.store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})
	_ = f.disp.Dispatch(context.Background(), id, taskstore.KindGenerateImage, map[string]any{
		"prompt":     "p",
		"output_dir": f.tempDir,
	})
	waitState(t, f.store, id, taskstore.StateCompleted, 3*time.Second)
}

// TestExtractTaskResult_NilGuards verifies parser robustness.
func TestExtractTaskResult_NilGuards(t *testing.T) {
	if extractTaskResult(nil) != nil {
		t.Error("nil result should yield nil Result")
	}
	if extractTaskResult(&types.ToolCallResult{}) != nil {
		t.Error("empty content should yield nil Result")
	}
	if extractTaskResult(&types.ToolCallResult{Content: []types.ContentItem{{Text: "not json"}}}) != nil {
		t.Error("non-JSON should yield nil Result")
	}
	if extractTaskResult(&types.ToolCallResult{Content: []types.ContentItem{{Text: `{"images":[]}`}}}) != nil {
		t.Error("no files[] should yield nil Result")
	}
	out := extractTaskResult(&types.ToolCallResult{Content: []types.ContentItem{{
		Text: `{"files":[{"index":0,"path":"/tmp/x.png","size_bytes":42,"format":"png"}]}`,
	}}})
	if out == nil || out.FilePath != "/tmp/x.png" || out.SizeBytes != 42 || out.Format != "png" {
		t.Errorf("unexpected extract output: %+v", out)
	}
}

// TestExtractErrText handles common cases.
func TestExtractErrText(t *testing.T) {
	if extractErrText(nil) != "unknown error" {
		t.Error("nil should yield generic")
	}
	if extractErrText(&types.ToolCallResult{}) != "unknown error" {
		t.Error("empty should yield generic")
	}
	got := extractErrText(&types.ToolCallResult{Content: []types.ContentItem{{Text: "boom"}}})
	if got != "boom" {
		t.Errorf("got %q", got)
	}
}
