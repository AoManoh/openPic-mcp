package taskstore

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryConfig tunes [MemoryStore]. Zero values pick reasonable defaults.
// All fields are also subject to hard upper bounds enforced in [NewMemory]
// to prevent operator misconfig from inflating memory beyond the SPEC §2.2
// envelope.
type MemoryConfig struct {
	// MaxQueued caps the number of StateQueued tasks. Submit returns
	// ErrQueueFull when this is reached. Default 256, hard upper 10000.
	MaxQueued int

	// MaxRetained caps the number of terminal tasks kept for query. When
	// a transition pushes terminal count past this, the oldest terminal
	// (by FinishedAt) is evicted in O(log N). Default 1024, hard upper
	// 100000.
	MaxRetained int

	// Now is the clock. Tests inject a fake clock so timestamps are
	// deterministic. Production callers leave it nil for time.Now.
	Now func() time.Time

	// PID overrides the process PID embedded in TaskIDs and used for
	// cross-PID isolation. Tests use this to simulate multiple processes
	// sharing one store. Production callers leave it 0 for os.Getpid.
	PID int

	// OnTransition is invoked under the write lock after Submit succeeds
	// or any TransitionFn applies a non-nil mutation. The supplied Task
	// is a deep copy that the hook may consume freely. Latency in the
	// hook directly extends lock-held time, so disk implementations
	// should keep it fast (single fsync + rename).
	//
	// Errors from the hook are intentionally not propagated — the
	// in-memory mutation has already committed and reverting it would
	// open a richer set of inconsistency scenarios than just logging the
	// disk failure. Hooks must surface their own failures via logs/metrics.
	OnTransition func(snapshot Task)

	// OnEvict fires under the write lock when a terminal task is dropped
	// from the retention window. Disk implementations use it to delete
	// the corresponding manifest. Same constraints as OnTransition.
	OnEvict func(id TaskID)
}

const (
	defaultMaxQueued   = 256
	defaultMaxRetained = 1024
	hardUpperMaxQueued = 10000
	hardUpperRetained  = 100000
)

// clamp returns v constrained to [lo, hi]. If v <= 0 returns def.
func clamp(v, def, hi int) int {
	if v <= 0 {
		return def
	}
	if v > hi {
		return hi
	}
	return v
}

// record bundles the canonical task state with the synchronization
// primitives the store needs around it. It is never exposed to callers;
// every public method returns Task by value via [Task.Clone].
type record struct {
	task   Task
	cancel context.CancelFunc // populated by RegisterCancel; nil otherwise
	done   chan struct{}      // closed when task reaches terminal state
	heapIx int                // index in terminalHeap; -1 when not in heap
}

// MemoryStore is the in-process [Store] implementation. It is the core
// substrate; the disk store wraps it via the OnTransition / OnEvict hooks.
//
// The lock model is a single sync.RWMutex protecting the tasks map and
// all per-record fields except `done` (which is closed exactly once
// under the write lock and then read concurrently by Wait callers).
type MemoryStore struct {
	mu           sync.RWMutex
	tasks        map[TaskID]*record
	terminal     terminalHeap // min-heap on FinishedAt for O(log N) eviction
	queued       int          // counts records in StateQueued
	running      int          // counts records in StateRunning
	now          func() time.Time
	pid          int
	maxQueue     int
	maxRet       int
	onTransition func(Task)
	onEvict      func(TaskID)
	closed       atomic.Bool
}

// NewMemory constructs a MemoryStore. Configuration is clamped to the
// hard upper bounds; misconfiguration never inflates memory beyond SPEC
// §2.2.
func NewMemory(cfg MemoryConfig) *MemoryStore {
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	pid := cfg.PID
	if pid <= 0 {
		pid = os.Getpid()
	}
	return &MemoryStore{
		tasks:        make(map[TaskID]*record),
		terminal:     terminalHeap{},
		now:          now,
		pid:          pid,
		maxQueue:     clamp(cfg.MaxQueued, defaultMaxQueued, hardUpperMaxQueued),
		maxRet:       clamp(cfg.MaxRetained, defaultMaxRetained, hardUpperRetained),
		onTransition: cfg.OnTransition,
		onEvict:      cfg.OnEvict,
	}
}

// PID returns the process PID this store reports as its owner. Used by
// the test suite to assert cross-PID isolation; production callers
// usually do not need it.
func (s *MemoryStore) PID() int { return s.pid }

// Submit implements [Store.Submit].
func (s *MemoryStore) Submit(_ context.Context, kind Kind, req RequestSummary) (TaskID, error) {
	if s.closed.Load() {
		return "", ErrStoreClosed
	}
	if !kind.IsValid() {
		return "", fmt.Errorf("taskstore: invalid kind %q", kind)
	}
	id, err := NewTaskID()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return "", ErrStoreClosed
	}
	if s.queued >= s.maxQueue {
		return "", fmt.Errorf("%w: max_queued=%d", ErrQueueFull, s.maxQueue)
	}
	if _, ok := s.tasks[id]; ok {
		// Three-segment ID collision is a programming error: it means the
		// time + crypto/rand + counter combo all coincided. Panic loudly
		// rather than silently overwrite a peer task.
		panic(fmt.Sprintf("taskstore: TaskID collision %q (impossible by construction)", id))
	}
	now := s.now()
	rec := &record{
		task: Task{
			ID:          id,
			Kind:        kind,
			State:       StateQueued,
			PID:         s.pid,
			SubmittedAt: now,
			Request:     req.Clone(),
		},
		done:   make(chan struct{}),
		heapIx: -1,
	}
	s.tasks[id] = rec
	s.queued++
	if s.onTransition != nil {
		// Disk persistence is best-effort: errors are surfaced through the
		// hook itself (logs/metrics) so a failed manifest write never
		// rejects an otherwise-valid in-memory submit. The in-memory state
		// is the runtime source of truth; the manifest is recovery aid.
		s.onTransition(rec.task.Clone())
	}
	return id, nil
}

// Get implements [Store.Get].
func (s *MemoryStore) Get(_ context.Context, id TaskID) (Task, error) {
	if id == "" {
		return Task{}, errEmptyID
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.tasks[id]
	if !ok {
		return Task{}, ErrTaskNotFound
	}
	return rec.task.Clone(), nil
}

// List implements [Store.List].
func (s *MemoryStore) List(_ context.Context, f Filter) ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Task, 0, len(s.tasks))
	for _, rec := range s.tasks {
		if !f.All && rec.task.PID != s.pid {
			continue
		}
		if !f.matches(&rec.task) {
			continue
		}
		out = append(out, rec.task.Clone())
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SubmittedAt.Before(out[j].SubmittedAt)
	})
	return out, nil
}

// Cancel implements [Store.Cancel]. It is a convenience wrapper that
// (a) invokes any registered cancel func to interrupt in-flight HTTP/IO
// and (b) attempts to transition the task to StateCancelled. If the task
// is already terminal the cancel func is still invoked (cheap, idempotent)
// but the state is not changed.
func (s *MemoryStore) Cancel(ctx context.Context, id TaskID, hint string) error {
	if id == "" {
		return errEmptyID
	}
	if hint == "" {
		hint = "client"
	}

	// Step 1 — fire the registered cancel func WITHOUT holding the write
	// lock. The cancel func may run handlers that call back into the
	// store (e.g. Transition to record an error). Holding the lock here
	// would deadlock those.
	s.mu.RLock()
	rec, ok := s.tasks[id]
	if !ok {
		s.mu.RUnlock()
		return ErrTaskNotFound
	}
	if rec.task.PID != s.pid {
		s.mu.RUnlock()
		return ErrCrossPID
	}
	cancelFn := rec.cancel
	alreadyTerminal := rec.task.State.IsTerminal()
	s.mu.RUnlock()

	if cancelFn != nil {
		cancelFn()
	}
	if alreadyTerminal {
		return nil
	}

	// Step 2 — record StateCancelled. ErrIllegalTransition here means a
	// concurrent worker reached a different terminal first; that's a
	// benign race and we should not surface it as an error.
	err := s.Transition(ctx, id, func(t *Task) error {
		t.State = StateCancelled
		t.CancelHint = hint
		t.FinishedAt = s.now()
		return nil
	})
	if err != nil && (errors.Is(err, ErrIllegalTransition) || errors.Is(err, ErrTaskNotFound)) {
		// Race with concurrent terminal transition or eviction. Treat as
		// success: cancellation intent has already been honored or
		// superseded.
		return nil
	}
	return err
}

// Transition implements [Store.Transition].
//
// The fn receives a writable *Task pointer that points at the canonical
// in-store record. State changes are validated against
// [allowedTransitions]; non-state field mutations always succeed. If the
// transition pushes the task to a terminal state the done channel is
// closed and any waiters are released. If terminal count exceeds
// MaxRetained the oldest terminal task is evicted under the same lock.
func (s *MemoryStore) Transition(_ context.Context, id TaskID, fn TransitionFn) error {
	if id == "" {
		return errEmptyID
	}
	if fn == nil {
		return errors.New("taskstore: nil TransitionFn")
	}
	if s.closed.Load() {
		return ErrStoreClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.tasks[id]
	if !ok {
		return ErrTaskNotFound
	}
	if rec.task.PID != s.pid {
		return ErrCrossPID
	}

	prevState := rec.task.State
	// fn mutates the live record; on validation failure we revert the
	// State field only. Other field mutations are kept because the
	// canonical model is "additive" (timestamps, error strings, results
	// can always grow) — only state has a lattice.
	prevSnapshot := rec.task
	if err := fn(&rec.task); err != nil {
		rec.task = prevSnapshot
		return err
	}
	if rec.task.State != prevState {
		if !canTransition(prevState, rec.task.State) {
			rec.task = prevSnapshot
			return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, prevState, rec.task.State)
		}
		s.applyStateChange(rec, prevState, rec.task.State)
	}
	if s.onTransition != nil {
		s.onTransition(rec.task.Clone())
	}
	return nil
}

// applyStateChange updates counters, the terminal heap and waiter
// channels in response to a validated state transition. Must be called
// under the write lock.
func (s *MemoryStore) applyStateChange(rec *record, from, to State) {
	switch from {
	case StateQueued:
		s.queued--
	case StateRunning:
		s.running--
	}
	switch to {
	case StateRunning:
		s.running++
		// First entry to StateRunning is the canonical place to stamp
		// StartedAt if the caller forgot. We never overwrite a
		// caller-provided value.
		if rec.task.StartedAt.IsZero() {
			rec.task.StartedAt = s.now()
		}
	}
	if to.IsTerminal() {
		if rec.task.FinishedAt.IsZero() {
			rec.task.FinishedAt = s.now()
		}
		heap.Push(&s.terminal, rec)
		// Release waiters and close done exactly once.
		select {
		case <-rec.done:
			// Already closed (defensive; should not happen because
			// canTransition forbids re-entering terminal states).
		default:
			close(rec.done)
		}
		// Detach cancel func so future Cancel calls are cheap no-ops and
		// the captured ctx can GC.
		rec.cancel = nil
		s.evictUntilUnderRetention()
	}
}

// evictUntilUnderRetention pops the oldest terminal records until the
// terminal population is within MaxRetained. Must be called under the
// write lock. The OnEvict hook fires once per dropped record so disk
// implementations can delete the corresponding manifest in lockstep.
func (s *MemoryStore) evictUntilUnderRetention() {
	for s.terminal.Len() > s.maxRet {
		rec := heap.Pop(&s.terminal).(*record)
		delete(s.tasks, rec.task.ID)
		if s.onEvict != nil {
			s.onEvict(rec.task.ID)
		}
	}
}

// Wait implements [Store.Wait].
func (s *MemoryStore) Wait(ctx context.Context, id TaskID) (Task, error) {
	if id == "" {
		return Task{}, errEmptyID
	}
	s.mu.RLock()
	rec, ok := s.tasks[id]
	if !ok {
		s.mu.RUnlock()
		return Task{}, ErrTaskNotFound
	}
	if rec.task.State.IsTerminal() {
		out := rec.task.Clone()
		s.mu.RUnlock()
		return out, nil
	}
	done := rec.done
	s.mu.RUnlock()

	select {
	case <-done:
		// Re-read under RLock to get the final state. The record
		// pointer is stable until eviction, and eviction only happens
		// while another goroutine holds the write lock that already
		// closed `done`; by the time we get here the terminal state is
		// installed.
		s.mu.RLock()
		defer s.mu.RUnlock()
		// Defensive: if the record was evicted between done close and
		// our re-acquire, fall through to NotFound rather than panic.
		rec2, ok := s.tasks[id]
		if !ok {
			return Task{}, ErrTaskNotFound
		}
		return rec2.task.Clone(), nil
	case <-ctx.Done():
		s.mu.RLock()
		defer s.mu.RUnlock()
		rec2, ok := s.tasks[id]
		if !ok {
			return Task{}, ctx.Err()
		}
		return rec2.task.Clone(), ctx.Err()
	}
}

// RegisterCancel implements [Store.RegisterCancel].
func (s *MemoryStore) RegisterCancel(id TaskID, cancel context.CancelFunc) error {
	if id == "" {
		return errEmptyID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.tasks[id]
	if !ok {
		return ErrTaskNotFound
	}
	if rec.task.PID != s.pid {
		return ErrCrossPID
	}
	if rec.task.State.IsTerminal() {
		// No-op: terminal tasks cannot be cancelled. Caller is free to
		// invoke its own cancel func; we just decline to retain it.
		return nil
	}
	rec.cancel = cancel
	return nil
}

// Close implements [Store.Close]. Subsequent Submit/Transition return
// ErrStoreClosed; Get/List/Wait still work.
func (s *MemoryStore) Close() error {
	s.closed.Store(true)
	return nil
}

// restore inserts a task verbatim from an external source (the disk
// replay path). It deliberately bypasses Submit's StateQueued constraint,
// the closed flag, and BOTH lifecycle hooks: replay is a load operation,
// not a state transition, and firing hooks here would re-emit the very
// manifest the caller just read.
//
// Callers MUST ensure the supplied Task has a valid ID and Kind. The
// store does field-level validation but not semantic checks (e.g. it
// will not refuse a task whose CancelHint is contradictory to State —
// that is the caller's job).
//
// Cross-PID records are accepted and retained as foreign: List default
// hides them, and Cancel/Transition will return ErrCrossPID so peers
// cannot mutate each other's records.
func (s *MemoryStore) restore(t Task) error {
	if !t.ID.IsValid() {
		return fmt.Errorf("taskstore: restore invalid id %q", t.ID)
	}
	if !t.Kind.IsValid() {
		return fmt.Errorf("taskstore: restore invalid kind %q", t.Kind)
	}
	switch t.State {
	case StateQueued, StateRunning,
		StateCompleted, StateFailed, StateCancelled, StateAbandoned:
		// ok
	default:
		return fmt.Errorf("taskstore: restore invalid state %q", t.State)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[t.ID]; exists {
		return fmt.Errorf("taskstore: restore duplicate id %q", t.ID)
	}
	rec := &record{
		task:   t.Clone(),
		done:   make(chan struct{}),
		heapIx: -1,
	}
	s.tasks[t.ID] = rec

	switch t.State {
	case StateQueued:
		// Foreign-PID queued tasks still increment our counter so the
		// MaxQueued admission check stays honest. They cannot be
		// promoted to running by our dispatcher (cross-PID rejection)
		// but they should still consume slots until Transition/eviction
		// removes them.
		s.queued++
	case StateRunning:
		s.running++
	default:
		// Terminal state — ensure FinishedAt is set, push onto the
		// retention heap, close the waiter channel, and trim if the
		// retention bound is now exceeded. The trim path here does NOT
		// fire OnEvict because replay is a pure load.
		if rec.task.FinishedAt.IsZero() {
			rec.task.FinishedAt = s.now()
		}
		heap.Push(&s.terminal, rec)
		close(rec.done)
		for s.terminal.Len() > s.maxRet {
			evict := heap.Pop(&s.terminal).(*record)
			delete(s.tasks, evict.task.ID)
		}
	}
	return nil
}

// terminalHeap is a min-heap of *record ordered by FinishedAt ASC. It is
// exclusively touched by [MemoryStore] under the write lock; the heap
// methods themselves are not goroutine-safe.
type terminalHeap []*record

func (h terminalHeap) Len() int { return len(h) }
func (h terminalHeap) Less(i, j int) bool {
	return h[i].task.FinishedAt.Before(h[j].task.FinishedAt)
}
func (h terminalHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIx = i
	h[j].heapIx = j
}

func (h *terminalHeap) Push(x any) {
	rec := x.(*record)
	rec.heapIx = len(*h)
	*h = append(*h, rec)
}

func (h *terminalHeap) Pop() any {
	old := *h
	n := len(old)
	rec := old[n-1]
	old[n-1] = nil
	rec.heapIx = -1
	*h = old[0 : n-1]
	return rec
}
