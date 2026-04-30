package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AoManoh/openPic-mcp/internal/taskstore"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// taskDispatcher is the narrow interface the submit_image_task handler
// uses to hand work off to the async layer. The concrete [Dispatcher]
// type satisfies it. Defining the interface at the consumer side keeps
// this file testable without dragging in the full dispatcher
// implementation.
type taskDispatcher interface {
	Dispatch(ctx context.Context, id taskstore.TaskID, kind taskstore.Kind, args map[string]any) error
}

// promptTruncateLen caps the prompt length we record on the in-memory
// RequestSummary. The full prompt always lands in the on-disk manifest;
// this guard exists so List responses don't ship megabyte-sized payloads
// for pathological prompts.
const promptTruncateLen = 4096

// SubmitImageTaskTool is the MCP tool definition for submit_image_task.
//
// The schema deliberately delegates per-kind validation to the
// underlying sync tool: `params` is a free-form object that mirrors the
// args of generate_image / edit_image. The handler validates `kind`
// itself but lets the existing sync handler closure validate the rest
// when the worker actually runs.
//
// This keeps the schema stable when generate_image / edit_image evolve:
// adding a new field there does not require schema changes here.
var SubmitImageTaskTool = types.Tool{
	Name: "submit_image_task",
	Description: "Submit an image generation or edit task for asynchronous execution. Returns a task_id immediately; " +
		"poll get_task_result or list_tasks to retrieve the outcome. Use this for long-running prompts that would otherwise " +
		"block the conversation — recommended for any request that may exceed ~30 seconds (e.g. 2048x2048 with complex " +
		"prompts, edit_image with reference images, or any call near the upstream OPENPIC_TIMEOUT). The task survives " +
		"client disconnects and IDE restarts (manifest is persisted to disk by default), so a later get_task_result " +
		"by task_id can still retrieve the final result.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"kind": {
				Type:        "string",
				Description: "Which sync tool to run asynchronously. Currently 'generate_image' or 'edit_image'.",
				Enum:        []string{string(taskstore.KindGenerateImage), string(taskstore.KindEditImage)},
			},
			"params": {
				Type:        "object",
				Description: "Same field set as the corresponding sync tool. See generate_image / edit_image schemas.",
			},
		},
		Required: []string{"kind", "params"},
	},
}

// GetTaskResultTool returns the current state and result (if any) of a
// previously submitted task. Supports optional long-poll.
var GetTaskResultTool = types.Tool{
	Name: "get_task_result",
	Description: "Read a task's current state and result. Pass wait=\"30s\" to long-poll until the task reaches a terminal " +
		"state (completed/failed/cancelled/abandoned). wait=\"0s\" (the default) returns the current snapshot immediately.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"task_id": {
				Type:        "string",
				Description: "Task identifier returned from submit_image_task.",
			},
			"wait": {
				Type: "string",
				Description: "Optional Go duration (e.g. '0s', '30s', '2m'). Maximum 5m. Default '0s'. " +
					"When >0 the call blocks until the task is terminal or the deadline elapses.",
				Default: "0s",
			},
		},
		Required: []string{"task_id"},
	},
}

// listTasksStateEnum is the set of task states list_tasks accepts as a
// filter. It is built once at package load from the canonical State
// constants in package taskstore so a future state addition cannot
// silently drift between the schema and the runtime validator
// (buildListFilter / validState below). The order mirrors the task
// lifecycle so MCP clients see a deterministic and documentable list.
var listTasksStateEnum = []string{
	string(taskstore.StateQueued),
	string(taskstore.StateRunning),
	string(taskstore.StateCompleted),
	string(taskstore.StateFailed),
	string(taskstore.StateCancelled),
	string(taskstore.StateAbandoned),
}

// listTasksKindEnum mirrors taskstore.Kind for the same reason as
// listTasksStateEnum: schema-side enum and runtime IsValid() must
// share a single source of truth.
var listTasksKindEnum = []string{
	string(taskstore.KindGenerateImage),
	string(taskstore.KindEditImage),
}

// ListTasksTool returns a filtered snapshot of tasks. By default only
// tasks owned by the current process are visible; pass all=true to
// include foreign-PID manifests left over by previous server processes.
//
// Schema note: `states` and `kinds` are arrays whose `items` schema is
// fully populated — strict MCP clients (Windsurf, in particular) reject
// any array property that omits `items`. The schema_test.go suite walks
// every tool in this package to enforce this invariant; do not delete
// the Items blocks below without also revisiting that test.
var ListTasksTool = types.Tool{
	Name: "list_tasks",
	Description: "List tasks, sorted by submitted_at ascending. By default only tasks owned by this server process are " +
		"included; pass all=true to also see manifests left by previous processes.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"states": {
				Type:        "array",
				Description: "Optional filter on task state. Any of queued/running/completed/failed/cancelled/abandoned.",
				Items: &types.Property{
					Type: "string",
					Enum: listTasksStateEnum,
				},
			},
			"kinds": {
				Type:        "array",
				Description: "Optional filter on task kind. Any of generate_image/edit_image.",
				Items: &types.Property{
					Type: "string",
					Enum: listTasksKindEnum,
				},
			},
			"since": {
				Type:        "string",
				Description: "Optional RFC3339 timestamp; only tasks submitted at or after this point are returned.",
			},
			"all": {
				Type:        "boolean",
				Description: "When true, include tasks owned by other PIDs (read-only). Default false.",
			},
		},
	},
}

// CancelTaskTool requests cancellation of a queued or running task.
// Cross-PID cancellation is rejected at the store layer.
//
// Description note: the cancel-vs-quota disclosure here is the
// canonical surface for LLM agents — they don't read README. The
// wording mirrors the "上游链路与取消语义" section in README so the
// two stay in sync.
var CancelTaskTool = types.Tool{
	Name: "cancel_task",
	Description: "Cancel a queued or running task. Returns the cancelled task. Already-terminal tasks return their current " +
		"snapshot unchanged. " +
		"NOTE on upstream quota: cancelling closes openPic-mcp's HTTP connection to the upstream API immediately " +
		"(ctx is bound end-to-end via http.NewRequestWithContext), so local resources (worker slot, store quota) are " +
		"freed deterministically. However, whether the upstream provider rolls back its compute / billing on " +
		"client-disconnect is implementation-dependent: official OpenAI does not document a rollback guarantee, and " +
		"OpenAI-compatible proxies (sub2api, CLIProxyAPI, etc.) typically wait for the upstream response before " +
		"deciding to charge. Do not rely on cancel_task to save upstream quota; use it to release local slots and " +
		"abandon work the caller no longer needs.",
	InputSchema: types.InputSchema{
		Type: "object",
		Properties: map[string]types.Property{
			"task_id": {
				Type:        "string",
				Description: "Task identifier returned from submit_image_task.",
			},
			"hint": {
				Type:        "string",
				Description: "Optional cancellation reason (e.g. 'user', 'timeout'). Stored verbatim on the task. Default 'client'.",
			},
		},
		Required: []string{"task_id"},
	},
}

// SubmitImageTaskHandler returns the MCP handler for submit_image_task.
// The handler validates kind, builds a RequestSummary from params,
// records the task in the store, and dispatches it to the worker pool.
// On dispatch failure the task is rolled to StateFailed so polling
// callers see a definite outcome instead of a stuck queued task.
func SubmitImageTaskHandler(store taskstore.Store, dispatcher taskDispatcher) types.ToolHandler {
	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		kindRaw := stringArg(args, "kind")
		kind := taskstore.Kind(kindRaw)
		if !kind.IsValid() {
			return errorResult(fmt.Sprintf("kind must be one of %q or %q, got %q",
				taskstore.KindGenerateImage, taskstore.KindEditImage, kindRaw)), nil
		}
		paramsRaw, ok := args["params"]
		if !ok {
			return errorResult("params object is required"), nil
		}
		params, ok := paramsRaw.(map[string]any)
		if !ok {
			return errorResult("params must be a JSON object matching the underlying sync tool's schema"), nil
		}
		// Per-kind synchronous validation. submit must reject obviously
		// malformed payloads up-front so callers don't get a task_id
		// back for a request that's guaranteed to fail at worker time
		// (which would waste a worker slot and force a second RPC to
		// observe the failure). The underlying generate_image /
		// edit_image handlers still re-validate the full schema; here
		// we only catch the fields that are always required for the
		// chosen kind. Keeping this list narrow avoids drift between
		// the two validation layers.
		if prompt := stringArg(params, "prompt"); prompt == "" {
			return errorResult("params.prompt is required and must be a non-empty string"), nil
		}
		if kind == taskstore.KindEditImage {
			if image := stringArg(params, "image"); image == "" {
				return errorResult("params.image is required for edit_image and must be a string (file path, URL, data URI, or raw base64)"), nil
			}
		}
		// Defense-in-depth enum check (R5 follow-up). The schema also
		// advertises this enum, but JSON-RPC clients vary in how
		// strictly they enforce schemas; catching it here means the
		// caller gets a fielded error in the same submit round-trip
		// rather than a fall-through file_path response surprise after
		// 90 seconds of work.
		if rf := stringArg(params, "response_format"); rf != "" {
			if err := validateResponseFormat(rf); err != nil {
				return errorResult("params." + err.Error()), nil
			}
		}

		summary := buildRequestSummaryFromParams(kind, params)

		id, err := store.Submit(ctx, kind, summary)
		if err != nil {
			if errors.Is(err, taskstore.ErrQueueFull) {
				return errorResult(fmt.Sprintf("task queue full: %v; retry later or cancel a pending task", err)), nil
			}
			if errors.Is(err, taskstore.ErrStoreClosed) {
				return errorResult("task store is closed (server shutting down); retry not possible"), nil
			}
			return errorResult(fmt.Sprintf("failed to submit task: %v", err)), nil
		}

		// Dispatch to the worker pool. On failure roll the task to a
		// terminal state so the manifest reflects reality:
		//
		//   - ErrDispatcherClosed → StateAbandoned (server is shutting
		//     down, the task never had a chance to run; semantically
		//     the same as a queued task surviving graceful shutdown).
		//   - any other error (queue full, etc.) → StateFailed with
		//     the dispatcher's error message, so the operator can
		//     diagnose why backpressure tripped.
		if err := dispatcher.Dispatch(ctx, id, kind, params); err != nil {
			target := taskstore.StateFailed
			if errors.Is(err, ErrDispatcherClosed) {
				target = taskstore.StateAbandoned
			}
			_ = store.Transition(ctx, id, func(t *taskstore.Task) error {
				t.State = target
				if target == taskstore.StateAbandoned {
					if t.CancelHint == "" {
						t.CancelHint = "dispatcher_closed"
					}
				} else {
					t.Error = "dispatch: " + err.Error()
				}
				return nil
			})
			return errorResult(fmt.Sprintf("dispatch failed: %v; task %s recorded as %s", err, id, target)), nil
		}

		// Return the queued snapshot. The state may have already
		// progressed to running by the time we read it back; that is
		// fine and accurate.
		task, _ := store.Get(ctx, id)
		return jsonResult(submitResponse{
			TaskID:      id,
			State:       task.State,
			SubmittedAt: task.SubmittedAt,
		})
	}
}

// GetTaskResultHandler returns the MCP handler for get_task_result. It
// supports an optional wait duration that turns the call into a
// long-poll bounded at 5 minutes.
func GetTaskResultHandler(store taskstore.Store) types.ToolHandler {
	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		idStr := stringArg(args, "task_id")
		if idStr == "" {
			return errorResult("task_id is required"), nil
		}
		id := taskstore.TaskID(idStr)
		if !id.IsValid() {
			return errorResult(fmt.Sprintf("task_id %q has invalid format", idStr)), nil
		}

		waitArg := stringArg(args, "wait")
		wait := time.Duration(0)
		if waitArg != "" && waitArg != "0" && waitArg != "0s" {
			parsed, err := time.ParseDuration(waitArg)
			if err != nil {
				return errorResult(fmt.Sprintf("wait must be a Go duration (e.g. '30s', '2m'): %v", err)), nil
			}
			if parsed < 0 {
				return errorResult("wait must be non-negative"), nil
			}
			// Hard upper bound. The previous behaviour silently clamped
			// values >5m to 5m, but that violated the documented
			// "Maximum 5m" contract: callers passing wait="10m"
			// reasonably expected either to wait 10 minutes or to be
			// told the value was rejected. Silently truncating is the
			// worst of both worlds — symmetric with the negative-wait
			// rejection above, this is now a hard error.
			if parsed > 5*time.Minute {
				return errorResult(fmt.Sprintf(
					"wait must be <= 5m (got %s); long-poll budget caps at 5 minutes — re-poll if you need more time",
					parsed)), nil
			}
			wait = parsed
		}

		var (
			task taskstore.Task
			err  error
		)
		if wait > 0 {
			waitCtx, cancel := context.WithTimeout(ctx, wait)
			defer cancel()
			task, err = store.Wait(waitCtx, id)
			// Wait returns the current snapshot AND ctx.Err on timeout;
			// we surface the snapshot regardless because the timeout is
			// expected behaviour for non-terminal long-polls.
			if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
				if errors.Is(err, taskstore.ErrTaskNotFound) {
					return errorResult(fmt.Sprintf("task %s not found", id)), nil
				}
				return errorResult(fmt.Sprintf("get_task_result: %v", err)), nil
			}
		} else {
			task, err = store.Get(ctx, id)
			if err != nil {
				if errors.Is(err, taskstore.ErrTaskNotFound) {
					return errorResult(fmt.Sprintf("task %s not found", id)), nil
				}
				return errorResult(fmt.Sprintf("get_task_result: %v", err)), nil
			}
		}

		return jsonResult(buildTaskView(task))
	}
}

// ListTasksHandler returns the MCP handler for list_tasks.
func ListTasksHandler(store taskstore.Store) types.ToolHandler {
	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		filter, err := buildListFilter(args)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		tasks, err := store.List(ctx, filter)
		if err != nil {
			return errorResult(fmt.Sprintf("list_tasks: %v", err)), nil
		}
		views := make([]taskView, 0, len(tasks))
		for _, t := range tasks {
			views = append(views, buildTaskView(t))
		}
		return jsonResult(listResponse{Tasks: views, Count: len(views)})
	}
}

// CancelTaskHandler returns the MCP handler for cancel_task.
func CancelTaskHandler(store taskstore.Store) types.ToolHandler {
	return func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		idStr := stringArg(args, "task_id")
		if idStr == "" {
			return errorResult("task_id is required"), nil
		}
		id := taskstore.TaskID(idStr)
		if !id.IsValid() {
			return errorResult(fmt.Sprintf("task_id %q has invalid format", idStr)), nil
		}
		hint := stringArg(args, "hint")
		if hint == "" {
			hint = "client"
		}

		err := store.Cancel(ctx, id, hint)
		if err != nil {
			if errors.Is(err, taskstore.ErrTaskNotFound) {
				return errorResult(fmt.Sprintf("task %s not found", id)), nil
			}
			if errors.Is(err, taskstore.ErrCrossPID) {
				return errorResult(fmt.Sprintf("task %s is owned by another process and cannot be cancelled here", id)), nil
			}
			return errorResult(fmt.Sprintf("cancel_task: %v", err)), nil
		}

		// Return the post-cancel snapshot so the caller sees the
		// authoritative state (which may legitimately be Completed if
		// the worker beat us to the terminal transition).
		task, _ := store.Get(ctx, id)
		return jsonResult(buildTaskView(task))
	}
}

// submitResponse is the JSON shape returned by submit_image_task.
type submitResponse struct {
	TaskID      taskstore.TaskID `json:"task_id"`
	State       taskstore.State  `json:"state"`
	SubmittedAt time.Time        `json:"submitted_at"`
}

// listResponse is the JSON shape returned by list_tasks.
type listResponse struct {
	Tasks []taskView `json:"tasks"`
	Count int        `json:"count"`
}

// taskView is the public-facing projection of taskstore.Task. We keep
// it separate from the canonical Task type so internal additions to
// Task do not leak through MCP responses.
type taskView struct {
	TaskID      taskstore.TaskID         `json:"task_id"`
	Kind        taskstore.Kind           `json:"kind"`
	State       taskstore.State          `json:"state"`
	PID         int                      `json:"pid"`
	SubmittedAt time.Time                `json:"submitted_at"`
	StartedAt   *time.Time               `json:"started_at,omitempty"`
	FinishedAt  *time.Time               `json:"finished_at,omitempty"`
	Request     taskstore.RequestSummary `json:"request"`
	Result      *taskstore.Result        `json:"result,omitempty"`
	Error       string                   `json:"error,omitempty"`
	CancelHint  string                   `json:"cancel_hint,omitempty"`
}

// buildTaskView projects a Task into its public-facing view, dropping
// zero timestamps so the JSON stays readable.
func buildTaskView(t taskstore.Task) taskView {
	v := taskView{
		TaskID:      t.ID,
		Kind:        t.Kind,
		State:       t.State,
		PID:         t.PID,
		SubmittedAt: t.SubmittedAt,
		Request:     t.Request,
		Result:      t.Result,
		Error:       t.Error,
		CancelHint:  t.CancelHint,
	}
	if !t.StartedAt.IsZero() {
		started := t.StartedAt
		v.StartedAt = &started
	}
	if !t.FinishedAt.IsZero() {
		finished := t.FinishedAt
		v.FinishedAt = &finished
	}
	return v
}

// buildRequestSummaryFromParams lifts the relevant fields from the
// per-kind params map into a RequestSummary. The summary is what gets
// stored in memory; the disk manifest also persists the params verbatim
// inside Extras so the full context survives a restart for diagnostic
// purposes.
func buildRequestSummaryFromParams(kind taskstore.Kind, params map[string]any) taskstore.RequestSummary {
	prompt := stringArg(params, "prompt")
	if len(prompt) > promptTruncateLen {
		prompt = prompt[:promptTruncateLen]
	}
	summary := taskstore.RequestSummary{
		Prompt:       prompt,
		Model:        stringArg(params, "model"),
		Size:         stringArg(params, "size"),
		AspectRatio:  stringArg(params, "aspect_ratio"),
		OutputFormat: stringArg(params, "output_format"),
	}

	// Populate Extras with everything else for audit trail. We keep it
	// to scalar string values so the manifest stays small and
	// diff-friendly. Image bytes / mask bytes are decoded inside the
	// sync handler when the worker runs; we do not want to ship them
	// through the manifest.
	extras := map[string]string{
		"kind":            string(kind),
		"response_format": stringArg(params, "response_format"),
		"output_dir":      stringArg(params, "output_dir"),
		"filename_prefix": stringArg(params, "filename_prefix"),
	}
	if quality := stringArg(params, "quality"); quality != "" {
		extras["quality"] = quality
	}
	if _, ok := params["overwrite"]; ok {
		extras["overwrite"] = fmt.Sprintf("%v", params["overwrite"])
	}
	if _, ok := params["n"]; ok {
		extras["n"] = fmt.Sprintf("%v", params["n"])
	}
	if kind == taskstore.KindEditImage {
		// We do NOT store the image / mask bytes in extras (privacy +
		// size), but we record their presence so audit can confirm the
		// request shape.
		if _, ok := params["image"]; ok {
			extras["has_image"] = "true"
		}
		if _, ok := params["mask"]; ok {
			extras["has_mask"] = "true"
		}
	}
	// Drop empty values to keep the manifest tidy.
	for k, v := range extras {
		if v == "" {
			delete(extras, k)
		}
	}
	if len(extras) > 0 {
		summary.Extras = extras
	}
	return summary
}

// buildListFilter parses the list_tasks args into a taskstore.Filter.
// Unknown values inside states/kinds are surfaced as errors rather than
// silently ignored — typos should be loud.
func buildListFilter(args map[string]any) (taskstore.Filter, error) {
	filter := taskstore.Filter{}
	if v, ok := args["all"]; ok {
		switch b := v.(type) {
		case bool:
			filter.All = b
		default:
			return filter, fmt.Errorf("all must be a boolean")
		}
	}
	if raw, ok := args["states"]; ok {
		states, err := parseStringList(raw, "states")
		if err != nil {
			return filter, err
		}
		for _, s := range states {
			st := taskstore.State(strings.ToLower(strings.TrimSpace(s)))
			if !validState(st) {
				return filter, fmt.Errorf("unknown state %q", s)
			}
			filter.States = append(filter.States, st)
		}
	}
	if raw, ok := args["kinds"]; ok {
		kinds, err := parseStringList(raw, "kinds")
		if err != nil {
			return filter, err
		}
		for _, k := range kinds {
			kk := taskstore.Kind(strings.ToLower(strings.TrimSpace(k)))
			if !kk.IsValid() {
				return filter, fmt.Errorf("unknown kind %q", k)
			}
			filter.Kinds = append(filter.Kinds, kk)
		}
	}
	if since := stringArg(args, "since"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return filter, fmt.Errorf("since must be RFC3339, got %q: %w", since, err)
		}
		filter.Since = t
	}
	return filter, nil
}

// parseStringList accepts either a []any (typical JSON unmarshal) or a
// []string (typed call sites).
func parseStringList(raw any, fieldName string) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be an array of strings", fieldName)
			}
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		// Stable order so manifest diff stays readable.
		sort.Strings(out)
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", fieldName)
	}
}

func validState(s taskstore.State) bool {
	switch s {
	case taskstore.StateQueued, taskstore.StateRunning,
		taskstore.StateCompleted, taskstore.StateFailed,
		taskstore.StateCancelled, taskstore.StateAbandoned:
		return true
	}
	return false
}

// jsonResult marshals v as indented JSON and wraps it in a
// ToolCallResult. Marshalling errors are surfaced as user-visible error
// results because they indicate a programming bug — the fields we
// assemble are all stdlib-friendly types.
func jsonResult(v any) (*types.ToolCallResult, error) {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult(fmt.Sprintf("failed to encode response: %v", err)), nil
	}
	return &types.ToolCallResult{
		Content: []types.ContentItem{{Type: "text", Text: string(body)}},
	}, nil
}
