// Package taskstore manages async tool tasks with bounded memory and
// crash-visible disk manifests.
//
// The package is the source of truth for the async submit/get path. Two
// implementations live here:
//
//   - [MemoryStore] is the in-process map-based store. It covers ephemeral
//     deployments and is the substrate every other implementation wraps.
//   - The disk-backed store (added in T2) extends MemoryStore with atomic
//     manifest writes under $OPENPIC_OUTPUT_DIR/tasks.
//
// All callers MUST mutate [Task] state through [Store.Transition]. Direct
// field writes are forbidden; the test suite asserts this with race
// detector + concurrent transitions.
package taskstore

import "errors"

// Sentinel errors returned by [Store] implementations. Always wrap with
// fmt.Errorf("...: %w", err) when adding context, never replace, so callers
// can route on errors.Is.
var (
	// ErrTaskNotFound is returned by Get / Cancel / Transition / Wait when
	// the requested ID is unknown to this store. Callers must surface this
	// as a tool-level error (not a panic) because cross-PID Get against a
	// store that only loaded its own PID's manifests legitimately produces
	// it.
	ErrTaskNotFound = errors.New("taskstore: task not found")

	// ErrIllegalTransition is returned when a TransitionFn attempts a state
	// move that is not in the canonical state machine (see SPEC §4). It is
	// always a programming error: handlers should return early on bad
	// inputs rather than try to bend the state graph.
	ErrIllegalTransition = errors.New("taskstore: illegal state transition")

	// ErrQueueFull is returned by Submit when the store is at MaxQueued.
	// The submit_image_task tool surfaces this verbatim so AI clients can
	// back off rather than retrying immediately.
	ErrQueueFull = errors.New("taskstore: task queue full")

	// ErrStoreClosed is returned by every mutating method after Close.
	// Reads can still succeed against a closed store so the engine can
	// drain and report final state during shutdown.
	ErrStoreClosed = errors.New("taskstore: store is closed")

	// ErrCrossPID is returned by mutating ops (Cancel, Transition) when
	// the target task was owned by a different process. List(All=true) and
	// Get may still read foreign manifests, but mutation is rejected so we
	// never race a peer instance.
	ErrCrossPID = errors.New("taskstore: cannot mutate task owned by another process")
)
