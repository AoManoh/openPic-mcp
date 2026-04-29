package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/internal/taskstore"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// Default tunables for [Dispatcher]. The values mirror the server engine
// defaults so deployments that don't override anything still get a
// coherent end-to-end story.
const (
	defaultDispatcherWorkers   = 16
	defaultDispatcherQueueSize = 256
	maxDispatcherWorkers       = 100
	maxDispatcherQueueSize     = 10000
)

// ErrDispatcherClosed is returned by [Dispatcher.Dispatch] after the
// dispatcher has been closed. The submit_image_task handler converts
// this into a user-visible error result.
var ErrDispatcherClosed = errors.New("tools: dispatcher closed")

// ErrDispatcherQueueFull is returned by [Dispatcher.Dispatch] when the
// worker queue is at capacity. The submit_image_task handler converts
// this into a user-visible error result and rolls the task to
// StateFailed so polling callers see a definite outcome rather than a
// task stuck in StateQueued forever.
var ErrDispatcherQueueFull = errors.New("tools: dispatcher queue full")

// DispatcherConfig configures a [Dispatcher]. Store and Provider are
// required; everything else has a default.
type DispatcherConfig struct {
	Store          taskstore.Store
	Provider       provider.ImageProvider
	Workers        int
	QueueSize      int
	Logger         *slog.Logger
	HandlerOptions []HandlerOption
}

// Dispatcher owns the worker pool that executes async image tasks.
//
// Lifecycle:
//
//   - [NewDispatcher] starts the workers eagerly so Submit + Dispatch can
//     hand off work as soon as the constructor returns.
//   - [Dispatcher.Dispatch] is called by the submit_image_task handler
//     for every newly-created task; it puts the task on the worker queue
//     and returns immediately. Workers drain the queue concurrently.
//   - [Dispatcher.AbandonRunning] is called by the server engine during
//     graceful shutdown via the [server.AbandonHook] interface. It marks
//     every in-flight task as StateAbandoned and cancels the per-task
//     ctx so workers unwind promptly.
//   - [Dispatcher.Close] closes the work queue and waits for all
//     workers to exit. It is the caller's responsibility (typically
//     main.go) to invoke Close AFTER server.Run has returned so the
//     dispatcher's task workers complete their unwind before the
//     process exits.
//
// Concurrency invariants:
//
//   - Every per-task cancel func is registered with the store so
//     cancel_task can hit it via Store.Cancel.
//   - The dispatcher tracks its own copy of the cancel func map under a
//     dedicated mutex so AbandonRunning can iterate it without holding
//     the store's lock; the store's RegisterCancel is the source of
//     truth for cancel_task, the dispatcher's map is the source of
//     truth for shutdown bulk-cancel.
//   - Transition errors during shutdown races (e.g. the worker tries to
//     Complete after AbandonRunning has Abandoned) are logged but never
//     panicked over; the state machine guarantees terminal states are
//     never re-entered.
type Dispatcher struct {
	store        taskstore.Store
	generateHdlr types.ToolHandler
	editHdlr     types.ToolHandler
	log          *slog.Logger
	workQueue    chan dispatchJob
	workers      int
	wg           sync.WaitGroup
	closed       atomic.Bool

	runningMu sync.Mutex
	running   map[taskstore.TaskID]context.CancelFunc
}

// dispatchJob is one unit of async work. The submit_image_task handler
// has already validated args and recorded the task in the store; the
// worker only needs the kind and the original args map to invoke the
// underlying sync handler closure.
type dispatchJob struct {
	id   taskstore.TaskID
	kind taskstore.Kind
	args map[string]any
}

// NewDispatcher constructs and starts a [Dispatcher]. Returns an error
// if Store or Provider is nil; everything else is clamped to safe
// defaults so misconfiguration cannot brick async execution.
func NewDispatcher(cfg DispatcherConfig) (*Dispatcher, error) {
	if cfg.Store == nil {
		return nil, errors.New("tools: DispatcherConfig.Store is required")
	}
	if cfg.Provider == nil {
		return nil, errors.New("tools: DispatcherConfig.Provider is required")
	}
	workers := cfg.Workers
	if workers <= 0 {
		workers = defaultDispatcherWorkers
	} else if workers > maxDispatcherWorkers {
		workers = maxDispatcherWorkers
	}
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = defaultDispatcherQueueSize
	} else if queueSize > maxDispatcherQueueSize {
		queueSize = maxDispatcherQueueSize
	}

	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	d := &Dispatcher{
		store:        cfg.Store,
		generateHdlr: GenerateImageHandler(cfg.Provider, cfg.HandlerOptions...),
		editHdlr:     EditImageHandler(cfg.Provider, cfg.HandlerOptions...),
		log:          log,
		workQueue:    make(chan dispatchJob, queueSize),
		workers:      workers,
		running:      make(map[taskstore.TaskID]context.CancelFunc),
	}

	d.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go d.worker(i)
	}

	log.Info("tools.dispatcher.started",
		"workers", workers,
		"queue_size", queueSize,
	)
	return d, nil
}

// Dispatch enqueues a task for execution. Must only be called for tasks
// that have already been recorded in the store via [taskstore.Store.Submit];
// the dispatcher does NOT re-validate args (the submit_image_task
// handler is the validation point).
//
// Returns [ErrDispatcherClosed] if Close has been called and
// [ErrDispatcherQueueFull] if the worker queue is at capacity. Both
// errors are routed to a StateFailed transition by the submit handler.
func (d *Dispatcher) Dispatch(_ context.Context, id taskstore.TaskID, kind taskstore.Kind, args map[string]any) error {
	if d.closed.Load() {
		return ErrDispatcherClosed
	}
	job := dispatchJob{id: id, kind: kind, args: args}
	select {
	case d.workQueue <- job:
		return nil
	default:
		return ErrDispatcherQueueFull
	}
}

// AbandonRunning implements [server.AbandonHook]. It marks every
// currently-running task as StateAbandoned with the supplied reason,
// then fires their per-task cancel funcs so any in-flight provider
// HTTP call returns promptly. Safe to call concurrently with Dispatch.
func (d *Dispatcher) AbandonRunning(reason string) {
	if reason == "" {
		reason = "shutdown"
	}
	d.runningMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(d.running))
	ids := make([]taskstore.TaskID, 0, len(d.running))
	for id, c := range d.running {
		cancels = append(cancels, c)
		ids = append(ids, id)
	}
	d.runningMu.Unlock()

	for i, id := range ids {
		err := d.store.Transition(context.Background(), id, func(t *taskstore.Task) error {
			// Workers may race us to Completed/Failed. The state
			// machine accepts running→abandoned but rejects
			// terminal→abandoned. We log and proceed either way.
			t.State = taskstore.StateAbandoned
			if t.CancelHint == "" {
				t.CancelHint = reason
			}
			return nil
		})
		if err != nil && !errors.Is(err, taskstore.ErrIllegalTransition) {
			d.log.Warn("tools.dispatcher.abandon_transition_failed",
				"id", id, "err", err)
		}
		// Fire cancel even if the transition lost the race: the worker
		// may still be inside the handler and we want it to unwind.
		cancels[i]()
	}

	d.log.Info("tools.dispatcher.abandon_running",
		"reason", reason,
		"count", len(ids),
	)
}

// Close shuts the dispatcher down. It closes the work queue (workers
// drain remaining buffered jobs and then exit) and waits for all
// workers to return. Idempotent.
func (d *Dispatcher) Close() error {
	if !d.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(d.workQueue)
	d.wg.Wait()
	d.log.Info("tools.dispatcher.stopped")
	return nil
}

// worker pulls jobs off the queue until it is closed.
func (d *Dispatcher) worker(id int) {
	defer d.wg.Done()
	for job := range d.workQueue {
		d.runJob(job)
	}
	d.log.Debug("tools.dispatcher.worker_stopped", "worker_id", id)
}

// runJob is the per-task lifecycle: register cancel → transition to
// running → invoke handler → transition to completed/failed (unless
// shutdown raced and already abandoned us).
func (d *Dispatcher) runJob(job dispatchJob) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Publish cancel func to BOTH the store (for cancel_task) and our
	// own map (for AbandonRunning bulk-cancel). The two registrations
	// must be done before the running transition so a concurrent
	// cancel_task arriving during transition still works.
	if err := d.store.RegisterCancel(job.id, cancel); err != nil {
		d.log.Warn("tools.dispatcher.register_cancel_failed",
			"id", job.id, "err", err)
		// If registration failed because the task is already terminal
		// (e.g. cancel_task arrived between Submit and Dispatch) we
		// just no-op — there's nothing to run.
		return
	}
	d.runningMu.Lock()
	d.running[job.id] = cancel
	d.runningMu.Unlock()
	defer func() {
		d.runningMu.Lock()
		delete(d.running, job.id)
		d.runningMu.Unlock()
	}()

	// Transition queued → running. If this fails it's almost always
	// because cancel_task moved the task to cancelled before we got
	// here; that's a benign race and we abort.
	if err := d.store.Transition(ctx, job.id, func(t *taskstore.Task) error {
		t.State = taskstore.StateRunning
		return nil
	}); err != nil {
		d.log.Info("tools.dispatcher.skip_already_terminal",
			"id", job.id, "err", err)
		return
	}

	handler, err := d.handlerForKind(job.kind)
	if err != nil {
		d.markFailed(ctx, job.id, err)
		return
	}

	// Run the underlying sync handler with panic isolation. The handler
	// itself returns (*ToolCallResult, error) and obeys the same
	// contract as the sync tool: an error means hard failure, IsError
	// means user-visible failure, otherwise success.
	var result *types.ToolCallResult
	var handlerErr error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				handlerErr = fmt.Errorf("handler panic: %v", rec)
			}
		}()
		result, handlerErr = handler(ctx, job.args)
	}()

	// Cancellation path: if our ctx was cancelled, leave the terminal
	// transition to whoever cancelled us (cancel_task → cancelled,
	// AbandonRunning → abandoned). We do nothing here.
	if ctx.Err() != nil {
		d.log.Info("tools.dispatcher.cancelled_during_run",
			"id", job.id, "ctx_err", ctx.Err())
		return
	}

	if handlerErr != nil {
		d.markFailed(ctx, job.id, handlerErr)
		return
	}
	if result == nil {
		d.markFailed(ctx, job.id, errors.New("handler returned nil result"))
		return
	}
	if result.IsError {
		d.markFailed(ctx, job.id, errors.New(extractErrText(result)))
		return
	}

	// Success: extract structured task result from the handler's JSON
	// payload. If extraction fails we still mark completed but with a
	// nil Result; callers polling the task will see "completed without
	// file_path" and can fall back to inspecting the raw response if
	// they need it. This is unusual and warrants a log line.
	storeResult := extractTaskResult(result)
	if storeResult == nil {
		d.log.Warn("tools.dispatcher.result_extract_empty", "id", job.id)
	}
	if err := d.store.Transition(ctx, job.id, func(t *taskstore.Task) error {
		t.State = taskstore.StateCompleted
		t.Result = storeResult
		return nil
	}); err != nil {
		d.log.Warn("tools.dispatcher.complete_transition_failed",
			"id", job.id, "err", err)
	}
}

// markFailed transitions a task to StateFailed and records the error
// message. Used by the worker on handler error / IsError result. The
// transition may legitimately fail due to a concurrent terminal
// transition (e.g. AbandonRunning fired between handler return and our
// markFailed) — those are logged at info level and not surfaced.
func (d *Dispatcher) markFailed(ctx context.Context, id taskstore.TaskID, runErr error) {
	err := d.store.Transition(ctx, id, func(t *taskstore.Task) error {
		t.State = taskstore.StateFailed
		t.Error = runErr.Error()
		return nil
	})
	if err == nil {
		return
	}
	if errors.Is(err, taskstore.ErrIllegalTransition) {
		d.log.Info("tools.dispatcher.fail_transition_skipped",
			"id", id, "reason", "already terminal", "run_err", runErr.Error())
		return
	}
	d.log.Warn("tools.dispatcher.fail_transition_error",
		"id", id, "err", err, "run_err", runErr.Error())
}

func (d *Dispatcher) handlerForKind(kind taskstore.Kind) (types.ToolHandler, error) {
	switch kind {
	case taskstore.KindGenerateImage:
		return d.generateHdlr, nil
	case taskstore.KindEditImage:
		return d.editHdlr, nil
	default:
		return nil, fmt.Errorf("unknown task kind %q", kind)
	}
}

// extractTaskResult unmarshals the sync-handler JSON payload back into
// imageToolResponse and lifts the first file's metadata into a
// taskstore.Result. Returns nil when no file was produced (e.g.
// response_format=b64_json or upstream returned only a non-data URL).
func extractTaskResult(result *types.ToolCallResult) *taskstore.Result {
	if result == nil || len(result.Content) == 0 {
		return nil
	}
	text := result.Content[0].Text
	if text == "" {
		return nil
	}
	var resp imageToolResponse
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil
	}
	if len(resp.Files) == 0 {
		return nil
	}
	f := resp.Files[0]
	return &taskstore.Result{
		FilePath:  f.Path,
		SizeBytes: f.SizeBytes,
		Format:    f.Format,
		Warnings:  resp.Warnings,
	}
}

// extractErrText pulls the human-readable error message out of an
// IsError ToolCallResult. The sync tools encode errors via errorResult
// which puts a plain string in the first content item; we just lift it
// verbatim so the manifest's error field stays readable.
func extractErrText(result *types.ToolCallResult) string {
	if result == nil || len(result.Content) == 0 {
		return "unknown error"
	}
	if t := result.Content[0].Text; t != "" {
		return t
	}
	return "unknown error"
}
