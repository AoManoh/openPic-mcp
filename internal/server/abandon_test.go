package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// stubAbandon records every AbandonRunning call (count + reason +
// monotonic timestamp) so tests can assert on lifecycle ordering.
type stubAbandon struct {
	calls   atomic.Int32
	reason  atomic.Pointer[string]
	whenNs  atomic.Int64
	onCall  func() // optional side-effect (sleep, panic)
	whenSet atomic.Bool
}

func (s *stubAbandon) AbandonRunning(reason string) {
	s.calls.Add(1)
	r := reason
	s.reason.Store(&r)
	if s.whenSet.CompareAndSwap(false, true) {
		s.whenNs.Store(time.Now().UnixNano())
	}
	if s.onCall != nil {
		s.onCall()
	}
}

func (s *stubAbandon) When() time.Time {
	return time.Unix(0, s.whenNs.Load())
}

// TestAbandonHook_NotCalledWithoutOption confirms the engine is a no-op
// on the abandon path when no hook is wired.
func TestAbandonHook_NotCalledWithoutOption(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	s := New(pt, h, Config{
		MaxConcurrentRequests: 1,
		RequestQueueSize:      1,
		ShutdownTimeout:       50 * time.Millisecond,
	}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	pt.CloseInbound()
	if err := <-done; err != nil {
		t.Fatalf("Run err = %v", err)
	}
	// No hook → nothing to assert beyond "Run returned cleanly".
}

// TestAbandonHook_NilOptionIsNoop: WithAbandonHook(nil) must not crash
// and must leave the field unset.
func TestAbandonHook_NilOptionIsNoop(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	s := New(pt, h, Config{ShutdownTimeout: 50 * time.Millisecond},
		WithLogger(discardLogger()),
		WithAbandonHook(nil),
	)
	if s.abandon != nil {
		t.Fatal("nil hook should leave field nil")
	}
}

// TestAbandonHook_FiresOnceOnShutdown is the core contract: a registered
// hook gets exactly one AbandonRunning("shutdown") call when Run returns.
func TestAbandonHook_FiresOnceOnShutdown(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	hook := &stubAbandon{}
	s := New(pt, h, Config{
		MaxConcurrentRequests: 1,
		RequestQueueSize:      1,
		ShutdownTimeout:       50 * time.Millisecond,
	}, WithLogger(discardLogger()), WithAbandonHook(hook))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	pt.CloseInbound()
	if err := <-done; err != nil {
		t.Fatalf("Run err = %v", err)
	}

	if got := hook.calls.Load(); got != 1 {
		t.Fatalf("AbandonRunning calls = %d, want 1", got)
	}
	if got := hook.reason.Load(); got == nil || *got != "shutdown" {
		t.Fatalf("reason = %v, want \"shutdown\"", got)
	}
}

// TestAbandonHook_FiresBeforeInflightDrains is the headline ordering
// invariant from SPEC §7: AbandonRunning must precede the in-flight wait.
//
// We verify this indirectly by capturing two timestamps:
//   - t1 = when the hook is invoked
//   - t2 = when an in-flight tools/call observes ctx cancellation
//
// If the engine called AbandonRunning AFTER inflight.Wait, the in-flight
// handler would never observe the engine ctx being cancelled (because
// the handler ctx is independent), and worse: the test would deadlock
// because the handler never returns. The test therefore asserts the
// engine ordering by relying on a real shutdown path.
func TestAbandonHook_FiresBeforeInflightDrains(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()

	handlerStarted := make(chan struct{})
	hookFired := make(chan struct{})

	hook := &stubAbandon{
		onCall: func() { close(hookFired) },
	}

	// The tool handler blocks until the hook has fired, then returns.
	// If the engine called AbandonRunning AFTER inflight.Wait, the
	// handler would be stuck forever and the test would hit its
	// deadline via t.Fatal below.
	h.On(MethodToolsCall, func(ctx context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		close(handlerStarted)
		select {
		case <-hookFired:
			return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "ok"}, nil
		case <-time.After(2 * time.Second):
			t.Errorf("handler timed out waiting for hook")
			return nil, nil
		}
	})

	s := New(pt, h, Config{
		MaxConcurrentRequests: 1,
		RequestQueueSize:      1,
		ShutdownTimeout:       3 * time.Second,
	}, WithLogger(discardLogger()), WithAbandonHook(hook))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Push one tools/call so the worker pool is busy.
	pt.Send(t, toolsCall(1, "tool"))

	// Wait until the handler is actually running, then close inbound to
	// trigger graceful shutdown.
	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start in time")
	}

	pt.CloseInbound()

	// Drain any response that may flush before closing.
	select {
	case <-pt.conn.outbound:
	case <-time.After(2 * time.Second):
	}

	if err := <-done; err != nil {
		t.Fatalf("Run err = %v", err)
	}

	if hook.calls.Load() != 1 {
		t.Fatalf("AbandonRunning calls = %d, want 1", hook.calls.Load())
	}
	if hook.When().IsZero() {
		t.Fatal("hook timestamp not recorded")
	}
}

// TestAbandonHook_PanicIsContained verifies a buggy hook cannot crash
// the shutdown path. The engine must log + recover + proceed.
func TestAbandonHook_PanicIsContained(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	hook := &stubAbandon{
		onCall: func() { panic("dispatcher exploded") },
	}
	s := New(pt, h, Config{
		MaxConcurrentRequests: 1,
		RequestQueueSize:      1,
		ShutdownTimeout:       50 * time.Millisecond,
	}, WithLogger(discardLogger()), WithAbandonHook(hook))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	pt.CloseInbound()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after panic-in-hook")
	}

	// The hook still recorded the call before panicking.
	if hook.calls.Load() != 1 {
		t.Fatalf("AbandonRunning calls = %d, want 1", hook.calls.Load())
	}
}

// TestAbandonHook_FiresOnCtxCancellation verifies that hooking still
// works when the caller cancels the engine ctx (vs the inbound EOF
// path). The two shutdown triggers must produce identical behaviour.
func TestAbandonHook_FiresOnCtxCancellation(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	hook := &stubAbandon{}
	s := New(pt, h, Config{
		MaxConcurrentRequests: 1,
		RequestQueueSize:      1,
		ShutdownTimeout:       100 * time.Millisecond,
	}, WithLogger(discardLogger()), WithAbandonHook(hook))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Let the recv loop spin once.
	time.Sleep(20 * time.Millisecond)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return on ctx cancel")
	}

	if hook.calls.Load() != 1 {
		t.Fatalf("AbandonRunning calls = %d, want 1", hook.calls.Load())
	}
}
