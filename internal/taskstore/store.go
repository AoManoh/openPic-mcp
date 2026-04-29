package taskstore

import "context"

// TransitionFn mutates a Task in place under the store's write lock. It
// MUST NOT call back into the Store (deadlock) and MUST NOT retain the
// pointer past return (the store may free the underlying memory). The
// returned error short-circuits the transition: the store reverts state
// and surfaces the error to the caller verbatim.
//
// Implementations should be small, pure, and side-effect free; the only
// external effect that is allowed is updating fields on the supplied
// task pointer. Disk persistence happens automatically after the
// callback returns nil.
type TransitionFn func(*Task) error

// Store is the canonical interface every taskstore implementation
// satisfies. It is intentionally narrow (8 methods) — adding a method is
// a SPEC change.
type Store interface {
	// Submit registers a fresh task in the StateQueued state. Returns
	// ErrQueueFull if the store is at MaxQueued, ErrStoreClosed after
	// Close, or a wrapped crypto/rand failure if ID generation fails.
	Submit(ctx context.Context, kind Kind, req RequestSummary) (TaskID, error)

	// Get returns a deep copy of the task with the given id. The copy
	// can be safely mutated by the caller. Returns ErrTaskNotFound if
	// unknown.
	Get(ctx context.Context, id TaskID) (Task, error)

	// List returns deep copies of every task that matches the filter.
	// The slice is freshly allocated and ordered by SubmittedAt ASC for
	// deterministic test output. Returns nil + nil if no tasks match.
	List(ctx context.Context, f Filter) ([]Task, error)

	// Cancel transitions the task to StateCancelled with the supplied
	// hint. It is a convenience wrapper around Transition that also
	// invokes any registered ctx cancel func so an in-flight worker
	// can return immediately.
	Cancel(ctx context.Context, id TaskID, hint string) error

	// Transition applies fn under the write lock. If fn returns nil and
	// the task's State changed, the new state must satisfy the canonical
	// state machine or ErrIllegalTransition is returned and the task is
	// left untouched. Mutations to non-State fields (timestamps, result,
	// error) always succeed regardless of state.
	Transition(ctx context.Context, id TaskID, fn TransitionFn) error

	// Wait blocks until the task reaches a terminal state or ctx is done.
	// On terminal it returns the final task (deep copy). On ctx.Done it
	// returns the current task (deep copy) plus ctx.Err. The caller can
	// then decide whether to retry.
	Wait(ctx context.Context, id TaskID) (Task, error)

	// RegisterCancel attaches a context.CancelFunc to a running task so
	// Cancel can interrupt the worker's HTTP/IO. Idempotent: subsequent
	// calls overwrite the previous cancel func. Calling on an unknown id
	// returns ErrTaskNotFound; calling on a terminal task is a no-op.
	RegisterCancel(id TaskID, cancel context.CancelFunc) error

	// Close prevents further Submit/Transition. Already-running tasks are
	// not touched (the engine handles abandonment via Cancel + the
	// shutdown timeout). Idempotent.
	Close() error
}
