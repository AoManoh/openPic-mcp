package taskstore

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestStore(t *testing.T, cfg MemoryConfig) *MemoryStore {
	t.Helper()
	if cfg.PID == 0 {
		cfg.PID = 12345 // deterministic test PID
	}
	return NewMemory(cfg)
}

func mustSubmit(t *testing.T, s *MemoryStore, kind Kind) TaskID {
	t.Helper()
	id, err := s.Submit(context.Background(), kind, RequestSummary{Prompt: "p", Model: "m"})
	if err != nil {
		t.Fatalf("Submit err=%v", err)
	}
	return id
}

// TestSubmit_Defaults asserts the queued state, PID stamping and timestamp
// invariants on a fresh task.
func TestSubmit_Defaults(t *testing.T) {
	s := newTestStore(t, MemoryConfig{PID: 999})
	id := mustSubmit(t, s, KindGenerateImage)

	tk, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get err=%v", err)
	}
	if tk.State != StateQueued {
		t.Errorf("State = %q, want %q", tk.State, StateQueued)
	}
	if tk.PID != 999 {
		t.Errorf("PID = %d, want 999", tk.PID)
	}
	if tk.Kind != KindGenerateImage {
		t.Errorf("Kind = %q", tk.Kind)
	}
	if tk.SubmittedAt.IsZero() {
		t.Error("SubmittedAt zero")
	}
	if !tk.StartedAt.IsZero() || !tk.FinishedAt.IsZero() {
		t.Error("StartedAt / FinishedAt should be zero before run")
	}
}

func TestSubmit_InvalidKind(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	_, err := s.Submit(context.Background(), Kind("totally-bogus"), RequestSummary{})
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

func TestSubmit_QueueFull(t *testing.T) {
	s := newTestStore(t, MemoryConfig{MaxQueued: 2})
	if _, err := s.Submit(context.Background(), KindGenerateImage, RequestSummary{}); err != nil {
		t.Fatalf("Submit 1 err=%v", err)
	}
	if _, err := s.Submit(context.Background(), KindGenerateImage, RequestSummary{}); err != nil {
		t.Fatalf("Submit 2 err=%v", err)
	}
	_, err := s.Submit(context.Background(), KindGenerateImage, RequestSummary{})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
	if !strings.Contains(err.Error(), "max_queued=2") {
		t.Errorf("error message must surface limit, got %q", err.Error())
	}
}

func TestSubmit_AfterClose(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	_ = s.Close()
	_, err := s.Submit(context.Background(), KindGenerateImage, RequestSummary{})
	if !errors.Is(err, ErrStoreClosed) {
		t.Fatalf("expected ErrStoreClosed, got %v", err)
	}
}

func TestTransition_HappyPath(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id := mustSubmit(t, s, KindGenerateImage)

	// queued -> running
	if err := s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateRunning
		return nil
	}); err != nil {
		t.Fatalf("queued->running err=%v", err)
	}

	// running -> completed with result
	if err := s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateCompleted
		t.Result = &Result{FilePath: "/tmp/img.png", SizeBytes: 1024, Format: "png"}
		return nil
	}); err != nil {
		t.Fatalf("running->completed err=%v", err)
	}

	tk, _ := s.Get(context.Background(), id)
	if tk.State != StateCompleted {
		t.Errorf("State = %q, want completed", tk.State)
	}
	if tk.Result == nil || tk.Result.FilePath != "/tmp/img.png" {
		t.Errorf("Result not persisted: %+v", tk.Result)
	}
	if tk.StartedAt.IsZero() {
		t.Error("StartedAt should be auto-stamped on first running")
	}
	if tk.FinishedAt.IsZero() {
		t.Error("FinishedAt should be auto-stamped on terminal")
	}
}

func TestTransition_IllegalTransitionsRejected(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id := mustSubmit(t, s, KindGenerateImage)

	// queued -> completed is illegal (must go through running)
	err := s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateCompleted
		return nil
	})
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("expected ErrIllegalTransition, got %v", err)
	}
	tk, _ := s.Get(context.Background(), id)
	if tk.State != StateQueued {
		t.Fatalf("state must be reverted on illegal transition, got %q", tk.State)
	}

	// terminal -> anything is illegal
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateRunning
		return nil
	})
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateCompleted
		return nil
	})
	err = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateRunning
		return nil
	})
	if !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("expected ErrIllegalTransition for terminal mutation, got %v", err)
	}
}

// TestTransition_AtomicityUnderRace fires 50 concurrent terminal
// transitions at the same task. The SPEC §2.1 atomicity invariant is:
//
//  1. The task ends in exactly one terminal state.
//  2. That terminal state is one of the targets some racer wanted.
//  3. Every racer targeting the winning state returns nil (idempotent
//     retry: setting State=current_terminal is a benign no-op once the
//     winning racer arrived first); every racer targeting a different
//     state returns ErrIllegalTransition (non-winning terminals can never
//     overwrite a winning terminal).
//
// Failure mode this catches: counter drift, lost updates, two different
// terminal states racing, or terminal->terminal cross-overwrite.
func TestTransition_AtomicityUnderRace(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id := mustSubmit(t, s, KindGenerateImage)
	// Move to running so terminal transitions are all legal candidates.
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateRunning
		return nil
	})

	const racers = 50
	targets := []State{StateCompleted, StateFailed, StateCancelled, StateAbandoned}
	wantPerTarget := make(map[State]int)
	for i := 0; i < racers; i++ {
		wantPerTarget[targets[i%len(targets)]]++
	}

	var wg sync.WaitGroup
	wg.Add(racers)
	var winners atomic.Int32
	var rejected atomic.Int32
	for i := 0; i < racers; i++ {
		target := targets[i%len(targets)]
		go func(tgt State) {
			defer wg.Done()
			err := s.Transition(context.Background(), id, func(t *Task) error {
				t.State = tgt
				return nil
			})
			switch {
			case err == nil:
				winners.Add(1)
			case errors.Is(err, ErrIllegalTransition):
				rejected.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(target)
	}
	wg.Wait()

	tk, _ := s.Get(context.Background(), id)
	if !tk.State.IsTerminal() {
		t.Fatalf("final state must be terminal, got %q", tk.State)
	}
	expectedWinners := wantPerTarget[tk.State]
	if got := int(winners.Load()); got != expectedWinners {
		t.Fatalf("winners = %d, want %d (racers targeting winning state %s)",
			got, expectedWinners, tk.State)
	}
	if got := int(rejected.Load()); got != racers-expectedWinners {
		t.Fatalf("rejected = %d, want %d", got, racers-expectedWinners)
	}
}

func TestList_FilterAndPIDIsolation(t *testing.T) {
	s := newTestStore(t, MemoryConfig{PID: 100})
	idA := mustSubmit(t, s, KindGenerateImage)
	_ = mustSubmit(t, s, KindEditImage)

	// Forge a foreign-PID record by direct field write under lock —
	// this is the test-only escape hatch. Production code path can never
	// produce a foreign PID record because Submit always stamps s.pid.
	s.mu.Lock()
	if rec, ok := s.tasks[idA]; ok {
		rec.task.PID = 999 // pretend this came from another process
	}
	s.mu.Unlock()

	// Default filter: only own PID.
	own, err := s.List(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("List own err=%v", err)
	}
	if len(own) != 1 {
		t.Fatalf("own list = %d, want 1", len(own))
	}
	if own[0].PID != 100 {
		t.Errorf("PID = %d, want 100", own[0].PID)
	}

	// All=true: include foreign.
	all, _ := s.List(context.Background(), Filter{All: true})
	if len(all) != 2 {
		t.Fatalf("all list = %d, want 2", len(all))
	}

	// Kind filter combines with PID.
	gens, _ := s.List(context.Background(), Filter{Kinds: []Kind{KindGenerateImage}})
	if len(gens) != 0 {
		t.Errorf("KindGenerateImage was reassigned to foreign PID, expected 0 own results, got %d", len(gens))
	}
}

// TestCancel_CrossPIDRejected locks SPEC §2.1: Cancel against a foreign
// PID must return ErrCrossPID, never silently mutate a peer's task.
func TestCancel_CrossPIDRejected(t *testing.T) {
	s := newTestStore(t, MemoryConfig{PID: 100})
	id := mustSubmit(t, s, KindGenerateImage)

	s.mu.Lock()
	if rec, ok := s.tasks[id]; ok {
		rec.task.PID = 999
	}
	s.mu.Unlock()

	err := s.Cancel(context.Background(), id, "")
	if !errors.Is(err, ErrCrossPID) {
		t.Fatalf("expected ErrCrossPID, got %v", err)
	}
}

func TestCancel_TriggersCancelFunc(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id := mustSubmit(t, s, KindGenerateImage)

	// Move to running so a cancel func is meaningful.
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateRunning
		return nil
	})

	cancelled := make(chan struct{})
	if err := s.RegisterCancel(id, func() { close(cancelled) }); err != nil {
		t.Fatalf("RegisterCancel err=%v", err)
	}

	if err := s.Cancel(context.Background(), id, "test"); err != nil {
		t.Fatalf("Cancel err=%v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("registered cancel func was not invoked")
	}

	tk, _ := s.Get(context.Background(), id)
	if tk.State != StateCancelled {
		t.Errorf("State = %q, want cancelled", tk.State)
	}
	if tk.CancelHint != "test" {
		t.Errorf("CancelHint = %q, want test", tk.CancelHint)
	}
}

func TestCancel_RaceWithTerminalIsBenign(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id := mustSubmit(t, s, KindGenerateImage)
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateRunning
		return nil
	})

	// Win the race with a synchronous Completed transition first.
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateCompleted
		return nil
	})

	// Cancel now must be a no-op and return nil (not Illegal/NotFound).
	if err := s.Cancel(context.Background(), id, "client"); err != nil {
		t.Fatalf("Cancel after terminal err=%v, want nil", err)
	}
	tk, _ := s.Get(context.Background(), id)
	if tk.State != StateCompleted {
		t.Fatalf("State = %q, want completed (no-op cancel)", tk.State)
	}
}

func TestWait_ReturnsImmediatelyIfTerminal(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id := mustSubmit(t, s, KindGenerateImage)
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateRunning
		return nil
	})
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateCompleted
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	tk, err := s.Wait(ctx, id)
	if err != nil {
		t.Fatalf("Wait err=%v", err)
	}
	if tk.State != StateCompleted {
		t.Errorf("State = %q", tk.State)
	}
}

func TestWait_BlocksUntilTransition(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id := mustSubmit(t, s, KindGenerateImage)

	done := make(chan Task, 1)
	go func() {
		tk, _ := s.Wait(context.Background(), id)
		done <- tk
	}()

	time.Sleep(20 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("Wait returned before transition")
	default:
	}

	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateRunning
		return nil
	})
	_ = s.Transition(context.Background(), id, func(t *Task) error {
		t.State = StateFailed
		t.Error = "boom"
		return nil
	})

	select {
	case tk := <-done:
		if tk.State != StateFailed {
			t.Errorf("State = %q, want failed", tk.State)
		}
		if tk.Error != "boom" {
			t.Errorf("Error = %q", tk.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not unblock after terminal transition")
	}
}

func TestWait_CtxCancelReturnsCurrent(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id := mustSubmit(t, s, KindGenerateImage)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	tk, err := s.Wait(ctx, id)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if tk.State != StateQueued {
		t.Errorf("State = %q, want queued snapshot", tk.State)
	}
}

func TestEviction_OldestTerminalDroppedFirst(t *testing.T) {
	clock := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	s := newTestStore(t, MemoryConfig{
		MaxRetained: 3,
		Now:         func() time.Time { clock = clock.Add(time.Second); return clock },
	})

	finish := func(id TaskID) {
		_ = s.Transition(context.Background(), id, func(t *Task) error { t.State = StateRunning; return nil })
		_ = s.Transition(context.Background(), id, func(t *Task) error { t.State = StateCompleted; return nil })
	}

	ids := make([]TaskID, 5)
	for i := range ids {
		ids[i] = mustSubmit(t, s, KindGenerateImage)
		finish(ids[i])
	}

	// First 2 must have been evicted; last 3 must remain.
	for _, id := range ids[:2] {
		_, err := s.Get(context.Background(), id)
		if !errors.Is(err, ErrTaskNotFound) {
			t.Errorf("expected eviction of %s, got err=%v", id, err)
		}
	}
	for _, id := range ids[2:] {
		if _, err := s.Get(context.Background(), id); err != nil {
			t.Errorf("expected retention of %s, got err=%v", id, err)
		}
	}
}

func TestClamp_HardUpperEnforced(t *testing.T) {
	s := NewMemory(MemoryConfig{MaxQueued: 999_999, MaxRetained: 999_999})
	if s.maxQueue != hardUpperMaxQueued {
		t.Errorf("MaxQueued not clamped: got %d, want %d", s.maxQueue, hardUpperMaxQueued)
	}
	if s.maxRet != hardUpperRetained {
		t.Errorf("MaxRetained not clamped: got %d, want %d", s.maxRet, hardUpperRetained)
	}

	s2 := NewMemory(MemoryConfig{MaxQueued: -5, MaxRetained: 0})
	if s2.maxQueue != defaultMaxQueued {
		t.Errorf("zero/negative not defaulted: got %d, want %d", s2.maxQueue, defaultMaxQueued)
	}
	if s2.maxRet != defaultMaxRetained {
		t.Errorf("zero MaxRetained not defaulted: got %d, want %d", s2.maxRet, defaultMaxRetained)
	}
}

// TestConcurrentMixedOps is the SPEC §9 stress test: 100 goroutines doing
// 100 mixed ops each (Submit / Transition / Get / Cancel / List), under
// race detector. Failure modes this catches: data races, deadlocks,
// counter drift, double-close on done channel.
func TestConcurrentMixedOps(t *testing.T) {
	s := newTestStore(t, MemoryConfig{MaxQueued: 10_000, MaxRetained: 10_000})
	const goroutines = 100
	const opsPerG = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			ctx := context.Background()
			ids := make([]TaskID, 0, opsPerG)
			for i := 0; i < opsPerG; i++ {
				switch (gid + i) % 5 {
				case 0:
					id, err := s.Submit(ctx, KindGenerateImage, RequestSummary{Prompt: "x"})
					if err != nil && !errors.Is(err, ErrQueueFull) {
						t.Errorf("Submit err=%v", err)
					}
					if err == nil {
						ids = append(ids, id)
					}
				case 1:
					if len(ids) == 0 {
						continue
					}
					id := ids[i%len(ids)]
					_ = s.Transition(ctx, id, func(t *Task) error {
						if t.State == StateQueued {
							t.State = StateRunning
						}
						return nil
					})
				case 2:
					if len(ids) == 0 {
						continue
					}
					id := ids[i%len(ids)]
					_ = s.Transition(ctx, id, func(t *Task) error {
						if t.State == StateRunning {
							t.State = StateCompleted
							t.Result = &Result{FilePath: "/tmp/x", Format: "png"}
						}
						return nil
					})
				case 3:
					if len(ids) == 0 {
						continue
					}
					id := ids[i%len(ids)]
					_ = s.Cancel(ctx, id, "race")
				case 4:
					_, _ = s.List(ctx, Filter{States: []State{StateRunning}})
				}
			}
		}(g)
	}
	wg.Wait()

	// Counter sanity: queued + running + terminal heap size == map size.
	s.mu.RLock()
	defer s.mu.RUnlock()
	terminal := s.terminal.Len()
	total := len(s.tasks)
	if s.queued+s.running+terminal != total {
		t.Fatalf("counter drift: queued=%d running=%d terminal=%d total=%d",
			s.queued, s.running, terminal, total)
	}
}

// TestRequestSummary_CloneIsolation verifies the Clone contract: mutating
// the returned task does not affect the canonical record.
func TestRequestSummary_CloneIsolation(t *testing.T) {
	s := newTestStore(t, MemoryConfig{})
	id, _ := s.Submit(context.Background(), KindGenerateImage, RequestSummary{
		Prompt: "original",
		Extras: map[string]string{"k": "v"},
	})
	tk, _ := s.Get(context.Background(), id)
	tk.Request.Prompt = "tampered"
	tk.Request.Extras["k"] = "tampered"

	tk2, _ := s.Get(context.Background(), id)
	if tk2.Request.Prompt != "original" {
		t.Errorf("Prompt leaked: %q", tk2.Request.Prompt)
	}
	if tk2.Request.Extras["k"] != "v" {
		t.Errorf("Extras leaked: %q", tk2.Request.Extras["k"])
	}
}
