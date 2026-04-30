package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AoManoh/openPic-mcp/internal/taskstore"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// stubDispatcher records Dispatch calls and lets tests inject errors.
type stubDispatcher struct {
	calls    []dispatchedCall
	failNext error
}

type dispatchedCall struct {
	id   taskstore.TaskID
	kind taskstore.Kind
	args map[string]any
}

func (s *stubDispatcher) Dispatch(_ context.Context, id taskstore.TaskID, kind taskstore.Kind, args map[string]any) error {
	if s.failNext != nil {
		err := s.failNext
		s.failNext = nil
		return err
	}
	s.calls = append(s.calls, dispatchedCall{id: id, kind: kind, args: args})
	return nil
}

func parseToolJSON(t *testing.T, r *types.ToolCallResult, into any) {
	t.Helper()
	if r == nil || len(r.Content) == 0 {
		t.Fatal("ToolCallResult had no content")
	}
	if err := json.Unmarshal([]byte(r.Content[0].Text), into); err != nil {
		t.Fatalf("unmarshal: %v\nbody=%s", err, r.Content[0].Text)
	}
}

// ---------- submit_image_task ----------

func TestSubmitImageTask_HappyPath(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	disp := &stubDispatcher{}
	h := SubmitImageTaskHandler(store, disp)

	res, err := h(context.Background(), map[string]any{
		"kind": "generate_image",
		"params": map[string]any{
			"prompt": "a cat",
			"size":   "1024x1024",
		},
	})
	if err != nil {
		t.Fatalf("handler err: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected isError: %s", res.Content[0].Text)
	}
	var resp submitResponse
	parseToolJSON(t, res, &resp)
	if resp.TaskID == "" {
		t.Fatal("task_id missing")
	}
	if resp.State != taskstore.StateQueued {
		t.Errorf("state = %q, want queued", resp.State)
	}

	// Dispatch was invoked with correct kind / args.
	if len(disp.calls) != 1 {
		t.Fatalf("Dispatch called %d times, want 1", len(disp.calls))
	}
	if disp.calls[0].kind != taskstore.KindGenerateImage {
		t.Errorf("kind = %q", disp.calls[0].kind)
	}
	if disp.calls[0].id != resp.TaskID {
		t.Errorf("dispatched id %q != response id %q", disp.calls[0].id, resp.TaskID)
	}

	// RequestSummary recorded prompt/size.
	stored, _ := store.Get(context.Background(), resp.TaskID)
	if stored.Request.Prompt != "a cat" {
		t.Errorf("Request.Prompt = %q", stored.Request.Prompt)
	}
	if stored.Request.Size != "1024x1024" {
		t.Errorf("Request.Size = %q", stored.Request.Size)
	}
}

func TestSubmitImageTask_RejectsInvalidKind(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := SubmitImageTaskHandler(store, &stubDispatcher{})

	res, _ := h(context.Background(), map[string]any{
		"kind":   "describe_image",
		"params": map[string]any{"prompt": "x"},
	})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
	if !strings.Contains(res.Content[0].Text, "kind must be") {
		t.Errorf("unexpected message: %q", res.Content[0].Text)
	}
}

func TestSubmitImageTask_RejectsMissingPrompt(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := SubmitImageTaskHandler(store, &stubDispatcher{})

	res, _ := h(context.Background(), map[string]any{
		"kind":   "generate_image",
		"params": map[string]any{}, // no prompt
	})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
	if !strings.Contains(res.Content[0].Text, "prompt") {
		t.Errorf("unexpected message: %q", res.Content[0].Text)
	}
}

func TestSubmitImageTask_RejectsMissingParamsObject(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := SubmitImageTaskHandler(store, &stubDispatcher{})

	res, _ := h(context.Background(), map[string]any{
		"kind": "generate_image",
		// no params at all
	})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

// TestSubmitImageTask_EditImageRejectsMissingImage is the regression
// test for R7 from the 5th-round stress report. The previous behaviour
// only validated `params.prompt` synchronously, so an edit_image
// submission with prompt-only payload would return a task_id and only
// fail later inside the worker — wasting a worker slot and forcing a
// second RPC to observe the error. This test pins that the missing
// image is now caught at submit time, symmetric with the prompt
// check, so callers get a single, fielded error in the same
// round-trip.
func TestSubmitImageTask_EditImageRejectsMissingImage(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	disp := &stubDispatcher{}
	h := SubmitImageTaskHandler(store, disp)

	res, _ := h(context.Background(), map[string]any{
		"kind": "edit_image",
		"params": map[string]any{
			"prompt": "a smile",
			// image deliberately omitted
		},
	})
	if !res.IsError {
		t.Fatal("expected IsError when image is missing for edit_image")
	}
	if !strings.Contains(res.Content[0].Text, "params.image is required") {
		t.Errorf("error must point at params.image, got %q", res.Content[0].Text)
	}
	// The dispatcher must NOT have been called: submit-time rejection
	// is the whole point of R7.
	if len(disp.calls) != 0 {
		t.Errorf("dispatcher.Dispatch was called %d times; expected 0", len(disp.calls))
	}
}

// TestSubmitImageTask_EditImageAcceptsWithImage is the symmetric
// happy-path: an edit_image submission carrying both prompt and image
// must be accepted and reach the dispatcher. This guards against an
// over-zealous regression that would also reject valid payloads.
func TestSubmitImageTask_EditImageAcceptsWithImage(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	disp := &stubDispatcher{}
	h := SubmitImageTaskHandler(store, disp)

	res, _ := h(context.Background(), map[string]any{
		"kind": "edit_image",
		"params": map[string]any{
			"prompt": "a smile",
			"image":  "/tmp/cat.png",
		},
	})
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Content[0].Text)
	}
	if len(disp.calls) != 1 {
		t.Errorf("dispatcher.Dispatch should have been called once; got %d", len(disp.calls))
	}
}

// TestSubmitImageTask_RejectsInvalidResponseFormat is the R5
// defense-in-depth regression: even though the schema advertises the
// `response_format` enum, the runtime must also reject unknown values.
// Without this guard an unknown response_format silently fell through
// to the file_path branch in the underlying handler — meaning a
// submission with response_format="banana" would queue successfully
// and only the get_task_result caller would notice the mismatch
// 90 seconds later (after a real upstream call had already happened).
func TestSubmitImageTask_RejectsInvalidResponseFormat(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	disp := &stubDispatcher{}
	h := SubmitImageTaskHandler(store, disp)

	res, _ := h(context.Background(), map[string]any{
		"kind": "generate_image",
		"params": map[string]any{
			"prompt":          "a cat",
			"response_format": "banana",
		},
	})
	if !res.IsError {
		t.Fatal("expected IsError on unknown response_format")
	}
	if !strings.Contains(res.Content[0].Text, "response_format") {
		t.Errorf("error must mention response_format, got %q", res.Content[0].Text)
	}
	if len(disp.calls) != 0 {
		t.Errorf("dispatcher must not be called for invalid enum; got %d calls", len(disp.calls))
	}
}

// TestSubmitImageTask_GenerateImageDoesNotRequireImage protects against
// an over-broad fix that would accidentally require `image` for
// generate_image too. generate_image legitimately has no image input.
func TestSubmitImageTask_GenerateImageDoesNotRequireImage(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	disp := &stubDispatcher{}
	h := SubmitImageTaskHandler(store, disp)

	res, _ := h(context.Background(), map[string]any{
		"kind": "generate_image",
		"params": map[string]any{
			"prompt": "a cat",
			// no image, and that's correct for generate_image
		},
	})
	if res.IsError {
		t.Fatalf("generate_image without image should succeed, got error: %s", res.Content[0].Text)
	}
}

func TestSubmitImageTask_DispatchFailureRollsToFailed(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	disp := &stubDispatcher{failNext: ErrDispatcherQueueFull}
	h := SubmitImageTaskHandler(store, disp)

	res, _ := h(context.Background(), map[string]any{
		"kind":   "generate_image",
		"params": map[string]any{"prompt": "x"},
	})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
	// Find the just-created task and check its state.
	all, _ := store.List(context.Background(), taskstore.Filter{})
	if len(all) != 1 {
		t.Fatalf("store has %d tasks, want 1", len(all))
	}
	if all[0].State != taskstore.StateFailed {
		t.Errorf("state = %q, want failed", all[0].State)
	}
	if !strings.Contains(all[0].Error, "dispatcher queue full") {
		t.Errorf("error = %q", all[0].Error)
	}
}

func TestSubmitImageTask_QueueFullSurfacesError(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{MaxQueued: 1})
	// Pre-fill the store.
	_, _ = store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "first"})

	h := SubmitImageTaskHandler(store, &stubDispatcher{})
	res, _ := h(context.Background(), map[string]any{
		"kind":   "generate_image",
		"params": map[string]any{"prompt": "second"},
	})
	if !res.IsError {
		t.Fatal("expected IsError on queue full")
	}
	if !strings.Contains(res.Content[0].Text, "task queue full") {
		t.Errorf("unexpected message: %q", res.Content[0].Text)
	}
}

// ---------- get_task_result ----------

func TestGetTaskResult_ImmediateSnapshot(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	id, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})

	h := GetTaskResultHandler(store)
	res, _ := h(context.Background(), map[string]any{"task_id": string(id)})
	if res.IsError {
		t.Fatalf("unexpected isError: %s", res.Content[0].Text)
	}
	var got taskView
	parseToolJSON(t, res, &got)
	if got.TaskID != id || got.State != taskstore.StateQueued {
		t.Errorf("got %+v", got)
	}
}

func TestGetTaskResult_NotFound(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := GetTaskResultHandler(store)
	res, _ := h(context.Background(), map[string]any{
		"task_id": "tsk_1_20260101T000000.000000000Z_aaaaaaaa",
	})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
	if !strings.Contains(res.Content[0].Text, "not found") {
		t.Errorf("unexpected: %q", res.Content[0].Text)
	}
}

func TestGetTaskResult_LongPollWaitsForTerminal(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	id, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = store.Transition(context.Background(), id, func(t *taskstore.Task) error {
			t.State = taskstore.StateRunning
			return nil
		})
		_ = store.Transition(context.Background(), id, func(t *taskstore.Task) error {
			t.State = taskstore.StateCompleted
			t.Result = &taskstore.Result{FilePath: "/tmp/x.png"}
			return nil
		})
	}()

	h := GetTaskResultHandler(store)
	res, _ := h(context.Background(), map[string]any{
		"task_id": string(id),
		"wait":    "1s",
	})
	if res.IsError {
		t.Fatalf("isError: %s", res.Content[0].Text)
	}
	var got taskView
	parseToolJSON(t, res, &got)
	if got.State != taskstore.StateCompleted {
		t.Errorf("state = %q, want completed", got.State)
	}
}

func TestGetTaskResult_LongPollTimeoutReturnsCurrent(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	id, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})

	h := GetTaskResultHandler(store)
	start := time.Now()
	res, _ := h(context.Background(), map[string]any{
		"task_id": string(id),
		"wait":    "100ms",
	})
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("long-poll took too long: %s", elapsed)
	}
	if res.IsError {
		t.Fatalf("unexpected isError: %s", res.Content[0].Text)
	}
	var got taskView
	parseToolJSON(t, res, &got)
	if got.State != taskstore.StateQueued {
		t.Errorf("state = %q, want queued (still pending)", got.State)
	}
}

func TestGetTaskResult_RejectsBadDuration(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := GetTaskResultHandler(store)
	res, _ := h(context.Background(), map[string]any{
		"task_id": "tsk_1_20260101T000000.000000000Z_aaaaaaaa",
		"wait":    "5banana",
	})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

// TestGetTaskResult_RejectsNegativeWait pins the negative-wait branch:
// passing a negative duration must produce a hard error rather than
// being silently coerced to zero. This is the symmetric counterpart of
// the >5m rejection below.
func TestGetTaskResult_RejectsNegativeWait(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := GetTaskResultHandler(store)
	res, _ := h(context.Background(), map[string]any{
		"task_id": "tsk_1_20260101T000000.000000000Z_aaaaaaaa",
		"wait":    "-1s",
	})
	if !res.IsError {
		t.Fatal("expected IsError on negative wait")
	}
	if !strings.Contains(res.Content[0].Text, "non-negative") {
		t.Errorf("unexpected error text: %q", res.Content[0].Text)
	}
}

// TestGetTaskResult_RejectsWaitOver5Minutes is the regression test for
// R3 from the 5th-round stress report. The previous implementation
// silently clamped wait>5m to 5m, violating the documented "Maximum
// 5m" contract. The fix turns this into a hard error symmetric with
// the negative-wait rejection. We pin both the IsError flag and the
// substring "<= 5m" so a future refactor cannot accidentally restore
// the clamp without also rewording the message.
func TestGetTaskResult_RejectsWaitOver5Minutes(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := GetTaskResultHandler(store)
	cases := []string{"5m1s", "6m", "10m", "1h"}
	for _, w := range cases {
		t.Run(w, func(t *testing.T) {
			res, _ := h(context.Background(), map[string]any{
				"task_id": "tsk_1_20260101T000000.000000000Z_aaaaaaaa",
				"wait":    w,
			})
			if !res.IsError {
				t.Fatalf("wait=%s: expected IsError", w)
			}
			text := res.Content[0].Text
			if !strings.Contains(text, "<= 5m") {
				t.Errorf("wait=%s: error must contain '<= 5m', got %q", w, text)
			}
			if !strings.Contains(text, w) {
				// The message echoes the offending value so callers
				// can confirm what was rejected.
				t.Errorf("wait=%s: error must echo the rejected value, got %q", w, text)
			}
		})
	}
}

// TestGetTaskResult_AcceptsExactly5Minutes pins the boundary: 5m
// itself is still a valid budget. This protects against an off-by-one
// regression where someone tightens the check to >=5m. We can't
// actually wait 5 minutes in a unit test, so we drive the task to a
// terminal state through the legal state machine
// (queued → running → completed) before invoking the handler — Wait
// then returns the snapshot synchronously instead of subscribing.
func TestGetTaskResult_AcceptsExactly5Minutes(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	id, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})
	if err := store.Transition(context.Background(), id, func(tk *taskstore.Task) error {
		tk.State = taskstore.StateRunning
		return nil
	}); err != nil {
		t.Fatalf("queued → running: %v", err)
	}
	if err := store.Transition(context.Background(), id, func(tk *taskstore.Task) error {
		tk.State = taskstore.StateCompleted
		return nil
	}); err != nil {
		t.Fatalf("running → completed: %v", err)
	}

	h := GetTaskResultHandler(store)
	res, _ := h(context.Background(), map[string]any{
		"task_id": string(id),
		"wait":    "5m",
	})
	if res.IsError {
		t.Fatalf("wait=5m should be accepted, got error: %s", res.Content[0].Text)
	}
}

func TestGetTaskResult_RejectsBadID(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := GetTaskResultHandler(store)
	res, _ := h(context.Background(), map[string]any{"task_id": "not-a-task-id"})
	if !res.IsError {
		t.Fatal("expected IsError on bad ID format")
	}
	if !strings.Contains(res.Content[0].Text, "invalid format") {
		t.Errorf("unexpected: %q", res.Content[0].Text)
	}
}

// ---------- list_tasks ----------

func TestListTasks_FilterByState(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	id1, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "a"})
	id2, _ := store.Submit(context.Background(), taskstore.KindEditImage, taskstore.RequestSummary{Prompt: "b"})
	_ = store.Transition(context.Background(), id1, func(t *taskstore.Task) error { t.State = taskstore.StateRunning; return nil })

	h := ListTasksHandler(store)
	res, _ := h(context.Background(), map[string]any{
		"states": []any{"running"},
	})
	if res.IsError {
		t.Fatalf("isError: %s", res.Content[0].Text)
	}
	var resp listResponse
	parseToolJSON(t, res, &resp)
	if resp.Count != 1 || resp.Tasks[0].TaskID != id1 {
		t.Errorf("unexpected list: %+v", resp)
	}
	_ = id2
}

func TestListTasks_FilterByKind(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	idGen, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "g"})
	_, _ = store.Submit(context.Background(), taskstore.KindEditImage, taskstore.RequestSummary{Prompt: "e"})

	h := ListTasksHandler(store)
	res, _ := h(context.Background(), map[string]any{"kinds": []any{"generate_image"}})
	if res.IsError {
		t.Fatal("unexpected isError")
	}
	var resp listResponse
	parseToolJSON(t, res, &resp)
	if resp.Count != 1 || resp.Tasks[0].TaskID != idGen {
		t.Errorf("unexpected: %+v", resp)
	}
}

func TestListTasks_RejectsUnknownState(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := ListTasksHandler(store)
	res, _ := h(context.Background(), map[string]any{"states": []any{"weirdstate"}})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

func TestListTasks_RejectsBadSince(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := ListTasksHandler(store)
	res, _ := h(context.Background(), map[string]any{"since": "yesterday"})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

func TestListTasks_AllShowsForeignPID(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{PID: 100})
	// Submit one of our own.
	mineID, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "mine"})

	// Inject a foreign-PID task by calling List+Cancel won't help; instead
	// we use the public API only: there is no public way to insert a
	// foreign-PID record into a MemoryStore from outside the package.
	// The DiskStore replay path is the production source of foreign
	// tasks. For this tool-layer test we just verify the All flag flows
	// through correctly by checking that filtering returns the same
	// single task whether All is true or false (since we have no
	// foreign records).
	h := ListTasksHandler(store)
	for _, all := range []bool{false, true} {
		res, _ := h(context.Background(), map[string]any{"all": all})
		var resp listResponse
		parseToolJSON(t, res, &resp)
		if resp.Count != 1 || resp.Tasks[0].TaskID != mineID {
			t.Errorf("all=%v: unexpected list: %+v", all, resp)
		}
	}
}

// ---------- cancel_task ----------

func TestCancelTask_HappyPath(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	id, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})

	h := CancelTaskHandler(store)
	res, _ := h(context.Background(), map[string]any{
		"task_id": string(id),
		"hint":    "user",
	})
	if res.IsError {
		t.Fatalf("isError: %s", res.Content[0].Text)
	}
	var got taskView
	parseToolJSON(t, res, &got)
	if got.State != taskstore.StateCancelled {
		t.Errorf("state = %q, want cancelled", got.State)
	}
	if got.CancelHint != "user" {
		t.Errorf("hint = %q, want user", got.CancelHint)
	}
}

func TestCancelTask_DefaultHintIsClient(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	id, _ := store.Submit(context.Background(), taskstore.KindGenerateImage, taskstore.RequestSummary{Prompt: "p"})
	h := CancelTaskHandler(store)
	res, _ := h(context.Background(), map[string]any{"task_id": string(id)})
	var got taskView
	parseToolJSON(t, res, &got)
	if got.CancelHint != "client" {
		t.Errorf("hint = %q", got.CancelHint)
	}
}

func TestCancelTask_NotFound(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := CancelTaskHandler(store)
	res, _ := h(context.Background(), map[string]any{
		"task_id": "tsk_1_20260101T000000.000000000Z_aaaaaaaa",
	})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

func TestCancelTask_BadID(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	h := CancelTaskHandler(store)
	res, _ := h(context.Background(), map[string]any{"task_id": "garbage"})
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

// ---------- end-to-end via Dispatcher ----------

func TestEndToEnd_SubmitDispatchPollComplete(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 2, QueueSize: 8})
	submit := SubmitImageTaskHandler(f.store, f.disp)
	get := GetTaskResultHandler(f.store)

	res, _ := submit(context.Background(), map[string]any{
		"kind": "generate_image",
		"params": map[string]any{
			"prompt":          "a cat",
			"output_dir":      f.tempDir,
			"filename_prefix": "test",
			"response_format": "file_path",
		},
	})
	if res.IsError {
		t.Fatalf("submit isError: %s", res.Content[0].Text)
	}
	var sresp submitResponse
	parseToolJSON(t, res, &sresp)

	// Long-poll to terminal.
	res2, _ := get(context.Background(), map[string]any{
		"task_id": string(sresp.TaskID),
		"wait":    "3s",
	})
	if res2.IsError {
		t.Fatalf("get isError: %s", res2.Content[0].Text)
	}
	var got taskView
	parseToolJSON(t, res2, &got)
	if got.State != taskstore.StateCompleted {
		t.Errorf("state = %q, want completed", got.State)
	}
	if got.Result == nil || got.Result.FilePath == "" {
		t.Errorf("Result missing: %+v", got.Result)
	}
}

func TestEndToEnd_SubmitCancelObserve(t *testing.T) {
	f := newDispatcherFixture(t, DispatcherConfig{Workers: 1, QueueSize: 4})
	hold := make(chan struct{})
	defer close(hold)
	f.provider.hold = hold

	submit := SubmitImageTaskHandler(f.store, f.disp)
	cancel := CancelTaskHandler(f.store)
	get := GetTaskResultHandler(f.store)

	res, _ := submit(context.Background(), map[string]any{
		"kind":   "generate_image",
		"params": map[string]any{"prompt": "p", "output_dir": f.tempDir},
	})
	var sresp submitResponse
	parseToolJSON(t, res, &sresp)

	// Wait until the worker enters the provider call.
	waitState(t, f.store, sresp.TaskID, taskstore.StateRunning, 2*time.Second)

	cres, _ := cancel(context.Background(), map[string]any{
		"task_id": string(sresp.TaskID),
		"hint":    "user-aborted",
	})
	if cres.IsError {
		t.Fatalf("cancel isError: %s", cres.Content[0].Text)
	}

	// Final state read.
	gres, _ := get(context.Background(), map[string]any{
		"task_id": string(sresp.TaskID),
		"wait":    "2s",
	})
	var got taskView
	parseToolJSON(t, gres, &got)
	if got.State != taskstore.StateCancelled {
		t.Errorf("state = %q, want cancelled", got.State)
	}
	if got.CancelHint != "user-aborted" {
		t.Errorf("hint = %q", got.CancelHint)
	}
}

func TestEndToEnd_AbandonOnDispatcherClose(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	provider := newFakeImageProvider()
	hold := make(chan struct{})
	defer close(hold)
	provider.hold = hold

	tmp := t.TempDir()
	disp, err := NewDispatcher(DispatcherConfig{
		Store:    store,
		Provider: provider,
		Workers:  1,
		HandlerOptions: []HandlerOption{
			WithDefaultOutputDir(tmp),
			WithDefaultFilenamePrefix("test"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer disp.Close()

	submit := SubmitImageTaskHandler(store, disp)
	res, _ := submit(context.Background(), map[string]any{
		"kind":   "generate_image",
		"params": map[string]any{"prompt": "p", "output_dir": tmp},
	})
	var sresp submitResponse
	parseToolJSON(t, res, &sresp)
	waitState(t, store, sresp.TaskID, taskstore.StateRunning, 2*time.Second)

	// Simulate server shutdown calling AbandonRunning.
	disp.AbandonRunning("shutdown")

	final := waitState(t, store, sresp.TaskID, taskstore.StateAbandoned, 2*time.Second)
	if final.CancelHint != "shutdown" {
		t.Errorf("hint = %q", final.CancelHint)
	}
}

// TestExtras_AreCaptured verifies the extras map captures audit fields.
func TestExtras_AreCaptured(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	disp := &stubDispatcher{}
	h := SubmitImageTaskHandler(store, disp)

	_, err := h(context.Background(), map[string]any{
		"kind": "edit_image",
		"params": map[string]any{
			"prompt":          "p",
			"image":           "data:image/png;base64,xxxx",
			"mask":            "data:image/png;base64,yyyy",
			"output_dir":      "/tmp",
			"filename_prefix": "edit",
			"overwrite":       true,
			"n":               1,
			"quality":         "hd",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	all, _ := store.List(context.Background(), taskstore.Filter{})
	if len(all) != 1 {
		t.Fatalf("len=%d", len(all))
	}
	extras := all[0].Request.Extras
	if extras["kind"] != "edit_image" {
		t.Errorf("kind = %q", extras["kind"])
	}
	if extras["has_image"] != "true" {
		t.Errorf("has_image = %q", extras["has_image"])
	}
	if extras["has_mask"] != "true" {
		t.Errorf("has_mask = %q", extras["has_mask"])
	}
	if extras["overwrite"] != "true" {
		t.Errorf("overwrite = %q", extras["overwrite"])
	}
	if extras["quality"] != "hd" {
		t.Errorf("quality = %q", extras["quality"])
	}
}

// TestPromptTruncation locks SPEC §2.2: in-memory prompt is truncated.
func TestPromptTruncation(t *testing.T) {
	store := taskstore.NewMemory(taskstore.MemoryConfig{})
	disp := &stubDispatcher{}
	h := SubmitImageTaskHandler(store, disp)

	long := strings.Repeat("x", promptTruncateLen+512)
	_, _ = h(context.Background(), map[string]any{
		"kind":   "generate_image",
		"params": map[string]any{"prompt": long},
	})
	all, _ := store.List(context.Background(), taskstore.Filter{})
	if len(all[0].Request.Prompt) != promptTruncateLen {
		t.Errorf("truncated len = %d, want %d", len(all[0].Request.Prompt), promptTruncateLen)
	}
}

// TestStubDispatcher_RecordsArgs sanity check on the test double.
func TestStubDispatcher_RecordsArgs(t *testing.T) {
	d := &stubDispatcher{}
	id := taskstore.TaskID("tsk_1_20260101T000000.000000000Z_aaaaaaaa")
	if err := d.Dispatch(context.Background(), id, taskstore.KindGenerateImage, map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if len(d.calls) != 1 {
		t.Fatalf("calls=%d", len(d.calls))
	}
	if d.calls[0].id != id || d.calls[0].args["k"] != "v" {
		t.Errorf("got %+v", d.calls[0])
	}

	d.failNext = errors.New("nope")
	if err := d.Dispatch(context.Background(), id, taskstore.KindGenerateImage, nil); err == nil {
		t.Error("expected injected error")
	}
	if d.failNext != nil {
		t.Error("failNext should reset after firing")
	}
}
