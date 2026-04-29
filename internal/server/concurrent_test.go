package server

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// TestServer_ConcurrentToolsCallActuallyOverlap is the marquee acceptance
// test for this commit. Five `tools/call` requests with simulated 200ms
// upstream latency must complete in less than the serial-baseline of 5 *
// 200ms because the engine dispatches them onto worker goroutines.
func TestServer_ConcurrentToolsCallActuallyOverlap(t *testing.T) {
	const (
		concurrent = 5
		toolDelay  = 200 * time.Millisecond
		// Serial baseline is concurrent * toolDelay = 1000ms. With 5
		// workers the wall-clock should be ~toolDelay; allow generous
		// slack for CI noise.
		budget = 3 * toolDelay
	)

	pt := newPipeTransport()
	h := newStubHandler()
	registry := newStubRegistry()

	var entered atomic.Int32
	h.On(MethodToolsCall, func(ctx context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		entered.Add(1)
		select {
		case <-time.After(toolDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "done"}, nil
	})

	s := New(pt, h, Config{
		MaxConcurrentRequests: concurrent,
		RequestQueueSize:      concurrent,
		ShutdownTimeout:       2 * time.Second,
	},
		WithLogger(discardLogger()),
		WithCancelRegistry(registry),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	// Push the burst as fast as possible so all workers see work.
	start := time.Now()
	for i := 1; i <= concurrent; i++ {
		pt.Send(t, toolsCall(i, "tool"))
	}

	// Wait for every response.
	got := 0
	for got < concurrent {
		_ = pt.Recv(t, 2*time.Second)
		got++
	}
	wall := time.Since(start)
	if wall > budget {
		t.Fatalf("%d concurrent tools/call took %s, want < %s (concurrency broken?)", concurrent, wall, budget)
	}
	if entered.Load() != concurrent {
		t.Fatalf("entered = %d, want %d", entered.Load(), concurrent)
	}

	// Wait until the engine has accounted for every completion. The
	// runJob defer is ordered so registry.Done fires before s.Completed
	// is incremented and s.inflight.Done releases the WaitGroup.
	waitForCompleted(t, s, int64(concurrent), 2*time.Second)

	// Registry should have observed the full register/done cycle.
	r, d := registry.Snapshot()
	if r != concurrent || d != concurrent {
		t.Fatalf("registry registers=%d dones=%d, want %d/%d", r, d, concurrent, concurrent)
	}

	pt.CloseInbound()
	if err := <-runDone; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

// waitForCompleted polls Server.Completed until it reaches the target or
// the deadline elapses. It is a small affordance for tests that want to
// observe state mutated inside the worker's defer block without sleeping.
func waitForCompleted(t *testing.T, s *Server, target int64, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if s.Completed() >= target {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("Completed() = %d, want >= %d", s.Completed(), target)
}

// TestServer_QueueFullFallbackKeepsServiceUp verifies that when the work
// queue saturates the engine still services requests via the synchronous
// fallback path. The legacy serial behaviour is the worst-case floor.
func TestServer_QueueFullFallbackKeepsServiceUp(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()

	const toolDelay = 50 * time.Millisecond
	h.On(MethodToolsCall, func(ctx context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		select {
		case <-time.After(toolDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "ok"}, nil
	})

	// 1 worker + 1-slot queue forces fallback when 4 requests arrive in a
	// burst before any worker can pick up.
	s := New(pt, h, Config{
		MaxConcurrentRequests: 1,
		RequestQueueSize:      1,
		ShutdownTimeout:       2 * time.Second,
	}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	const burst = 4
	for i := 1; i <= burst; i++ {
		pt.Send(t, toolsCall(i, "tool"))
	}

	for i := 0; i < burst; i++ {
		_ = pt.Recv(t, 2*time.Second)
	}

	waitForCompleted(t, s, int64(burst), 2*time.Second)
	if s.FallbackCount() == 0 {
		t.Fatalf("FallbackCount = 0, expected >0 under saturation")
	}

	pt.CloseInbound()
	if err := <-runDone; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

// TestServer_CancelRegistryInterruptsInflight simulates a `notifications/cancelled`
// arriving while a long-running tool is executing. Firing the registered
// cancel func must propagate ctx cancellation into the handler.
func TestServer_CancelRegistryInterruptsInflight(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	registry := newStubRegistry()

	entered := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	h.On(MethodToolsCall, func(ctx context.Context, _ *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		entered <- struct{}{}
		select {
		case <-time.After(2 * time.Second):
			t.Error("handler ran to completion despite cancellation")
			return nil, errors.New("not cancelled")
		case <-ctx.Done():
			cancelled <- struct{}{}
			return nil, ctx.Err()
		}
	})

	s := New(pt, h, Config{
		MaxConcurrentRequests: 1,
		RequestQueueSize:      1,
		ShutdownTimeout:       1 * time.Second,
	},
		WithLogger(discardLogger()),
		WithCancelRegistry(registry),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	pt.Send(t, toolsCall(99, "long-tool"))

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler never started")
	}

	// Simulate the protocol layer's notifications/cancelled handler.
	if !registry.Cancel(float64(99)) {
		t.Fatal("registry.Cancel returned false; cancel func not registered")
	}

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("handler did not observe ctx cancellation within 1s")
	}

	pt.CloseInbound()
	if err := <-runDone; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

// TestServer_GracefulShutdownWaitsForInflight verifies that closing the
// inbound stream lets in-flight workers finish before Run returns.
func TestServer_GracefulShutdownWaitsForInflight(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()

	finished := atomic.Int32{}
	h.On(MethodToolsCall, func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		time.Sleep(150 * time.Millisecond)
		finished.Add(1)
		return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "done"}, nil
	})

	s := New(pt, h, Config{
		MaxConcurrentRequests: 2,
		RequestQueueSize:      4,
		ShutdownTimeout:       2 * time.Second,
	}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	const n = 4
	for i := 1; i <= n; i++ {
		pt.Send(t, toolsCall(i, "tool"))
	}

	// Close the inbound stream to begin shutdown while jobs are in flight.
	pt.CloseInbound()

	for i := 0; i < n; i++ {
		_ = pt.Recv(t, 2*time.Second)
	}

	if err := <-runDone; err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got, want := int(finished.Load()), n; got != want {
		t.Fatalf("finished = %d, want %d", got, want)
	}
}

// TestServer_ShutdownTimeoutForcesCancellation verifies that handlers that
// ignore ctx beyond the configured shutdown budget are eventually
// abandoned. The handler must, however, still observe the cancellation
// event so resources have a chance to unwind.
func TestServer_ShutdownTimeoutForcesCancellation(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()

	entered := make(chan struct{}, 1)
	cancelObserved := make(chan struct{}, 1)
	h.On(MethodToolsCall, func(ctx context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			cancelObserved <- struct{}{}
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "should-not-happen"}, nil
		}
	})

	s := New(pt, h, Config{
		MaxConcurrentRequests: 1,
		RequestQueueSize:      1,
		ShutdownTimeout:       100 * time.Millisecond,
	}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	pt.Send(t, toolsCall(1, "stuck"))

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler never started")
	}

	pt.CloseInbound()

	select {
	case <-cancelObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not observe forced cancellation within 2s")
	}

	if err := <-runDone; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

// TestServer_ConcurrentResponsesArePerFrame ensures that concurrent
// workers writing back to the connection never interleave bytes. The pipe
// transport stores each Write atomically, but this guards against future
// regressions where someone might call Write multiple times per response.
func TestServer_ConcurrentResponsesArePerFrame(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	const workers = 16

	var wg sync.WaitGroup
	gate := make(chan struct{})
	h.On(MethodToolsCall, func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		// Synchronize with the dispatcher so all workers race the write.
		wg.Done()
		<-gate
		return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "done"}, nil
	})

	s := New(pt, h, Config{
		MaxConcurrentRequests: workers,
		RequestQueueSize:      workers,
		ShutdownTimeout:       2 * time.Second,
	}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	wg.Add(workers)
	for i := 1; i <= workers; i++ {
		pt.Send(t, toolsCall(i, "tool"))
	}
	wg.Wait()
	close(gate)

	got := 0
	for got < workers {
		data := pt.Recv(t, 2*time.Second)
		// Each frame must be a self-contained JSON object beginning with
		// `{` and ending with `}`. The pipe transport stores frames
		// individually so this also catches any accidental Multi-Write.
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
			t.Fatalf("frame %d corrupted: %q", got, data)
		}
		if !strings.Contains(string(trimmed), `"jsonrpc"`) {
			t.Fatalf("frame %d missing jsonrpc field: %q", got, data)
		}
		got++
	}

	pt.CloseInbound()
	if err := <-runDone; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}
