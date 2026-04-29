package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AoManoh/openPic-mcp/internal/transport"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// ---------------- Test transport ----------------

// pipeTransport is a tiny in-memory [transport.Transport] used to drive the
// engine without touching real stdio. Tests push inbound frames via Send
// and observe outbound frames via Recv.
type pipeTransport struct {
	once sync.Once
	conn *pipeConn
}

func newPipeTransport() *pipeTransport {
	return &pipeTransport{
		conn: &pipeConn{
			inbound:  make(chan []byte, 64),
			outbound: make(chan []byte, 64),
			closed:   make(chan struct{}),
		},
	}
}

func (p *pipeTransport) Connect(_ context.Context) (transport.Connection, error) {
	return p.conn, nil
}

// Send pushes a frame the server will see on Read.
func (p *pipeTransport) Send(t *testing.T, payload []byte) {
	t.Helper()
	select {
	case p.conn.inbound <- payload:
	case <-p.conn.closed:
		t.Fatalf("Send: connection closed")
	case <-time.After(2 * time.Second):
		t.Fatalf("Send: timed out (queue full)")
	}
}

// Recv blocks until the server writes one frame back.
func (p *pipeTransport) Recv(t *testing.T, within time.Duration) []byte {
	t.Helper()
	select {
	case data := <-p.conn.outbound:
		return data
	case <-time.After(within):
		t.Fatalf("Recv: no response within %s", within)
	}
	return nil
}

// CloseInbound terminates the inbound side so the recv loop returns. Use
// this to drive a graceful shutdown without waiting on the caller ctx.
func (p *pipeTransport) CloseInbound() {
	p.once.Do(func() { close(p.conn.inbound) })
}

type pipeConn struct {
	inbound  chan []byte
	outbound chan []byte

	closeOnce sync.Once
	closed    chan struct{}
}

func (c *pipeConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, io.EOF
	case data, ok := <-c.inbound:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	}
}

func (c *pipeConn) Write(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	out := make([]byte, len(payload))
	copy(out, payload)
	select {
	case c.outbound <- out:
		return nil
	case <-c.closed:
		return errors.New("pipeConn: closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *pipeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

// ---------------- Stub handler / registry ----------------

// stubHandler is a [Handler] that lets tests script the response and an
// optional in-handler hook (delay, cancellation observation, panic).
type stubHandler struct {
	mu       sync.Mutex
	handlers map[string]func(ctx context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error)
}

func newStubHandler() *stubHandler {
	return &stubHandler{handlers: map[string]func(context.Context, *types.JSONRPCRequest) (*types.JSONRPCResponse, error){}}
}

func (h *stubHandler) On(method string, fn func(ctx context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[method] = fn
}

func (h *stubHandler) HandleMessage(ctx context.Context, raw []byte) (*types.JSONRPCResponse, error) {
	var req types.JSONRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	h.mu.Lock()
	fn, ok := h.handlers[req.Method]
	h.mu.Unlock()
	if !ok {
		return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "ok"}, nil
	}
	return fn(ctx, &req)
}

// stubRegistry records Register/Done calls so tests can assert the engine
// honoured the cancellation contract.
type stubRegistry struct {
	mu        sync.Mutex
	registers []any
	dones     []any
	cancels   map[any]context.CancelFunc
}

func newStubRegistry() *stubRegistry {
	return &stubRegistry{cancels: make(map[any]context.CancelFunc)}
}

func (r *stubRegistry) Register(id any, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registers = append(r.registers, id)
	r.cancels[fmt.Sprintf("%v", id)] = cancel
}

func (r *stubRegistry) Done(id any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dones = append(r.dones, id)
	delete(r.cancels, fmt.Sprintf("%v", id))
}

// CancelAll fires every recorded cancel func; tests use this to simulate
// a `notifications/cancelled` arriving from the client.
func (r *stubRegistry) Cancel(id any) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[fmt.Sprintf("%v", id)]
	if ok {
		delete(r.cancels, fmt.Sprintf("%v", id))
	}
	r.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (r *stubRegistry) Snapshot() (registers, dones int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.registers), len(r.dones)
}

// ---------------- Helpers ----------------

func toolsCall(id any, name string) []byte {
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  MethodToolsCall,
		"params":  map[string]any{"name": name},
	}
	raw, _ := json.Marshal(body)
	return raw
}

func notification(method string) []byte {
	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	raw, _ := json.Marshal(body)
	return raw
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---------------- Tests ----------------

func TestNormalizeConfig(t *testing.T) {
	cases := []struct {
		name string
		in   Config
		want Config
	}{
		{
			name: "zero falls back to defaults",
			in:   Config{},
			want: Config{
				MaxConcurrentRequests: DefaultMaxConcurrentRequests,
				RequestQueueSize:      DefaultRequestQueueSize,
				ShutdownTimeout:       DefaultShutdownTimeout,
			},
		},
		{
			name: "respects explicit values",
			in: Config{
				MaxConcurrentRequests: 4,
				RequestQueueSize:      8,
				RequestTimeout:        5 * time.Second,
				ShutdownTimeout:       2 * time.Second,
			},
			want: Config{
				MaxConcurrentRequests: 4,
				RequestQueueSize:      8,
				RequestTimeout:        5 * time.Second,
				ShutdownTimeout:       2 * time.Second,
			},
		},
		{
			name: "caps oversized values",
			in:   Config{MaxConcurrentRequests: 9999, RequestQueueSize: 99999},
			want: Config{
				MaxConcurrentRequests: maxConcurrentRequestsCap,
				RequestQueueSize:      requestQueueSizeCap,
				ShutdownTimeout:       DefaultShutdownTimeout,
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeConfig(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeConfig(%+v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNew_PanicsOnNilDeps(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil transport")
		}
	}()
	_ = New(nil, newStubHandler(), Config{})
}

func TestServer_RunRejectsDoubleStart(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	s := New(pt, h, Config{MaxConcurrentRequests: 1, RequestQueueSize: 1, ShutdownTimeout: 50 * time.Millisecond}, WithLogger(discardLogger()))

	// First start blocks; cancel via ctx so it returns.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Wait briefly so Run picks up.
	time.Sleep(20 * time.Millisecond)

	// Second Run call should be rejected immediately.
	if err := s.Run(context.Background()); err == nil {
		t.Fatal("second Run should fail")
	}

	cancel()
	pt.CloseInbound()
	if err := <-done; err != nil {
		t.Fatalf("Run returned err = %v", err)
	}
}

func TestServer_DispatchesNonToolsCallSynchronously(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	called := atomic.Int32{}
	h.On("ping", func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		called.Add(1)
		return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "pong"}, nil
	})
	s := New(pt, h, Config{MaxConcurrentRequests: 2, RequestQueueSize: 4, ShutdownTimeout: 100 * time.Millisecond}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Send a non-tools/call request.
	req := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	pt.Send(t, req)

	resp := pt.Recv(t, time.Second)
	if !bytes.Contains(resp, []byte(`"pong"`)) {
		t.Fatalf("unexpected response: %s", resp)
	}
	if called.Load() != 1 {
		t.Fatalf("handler called %d times, want 1", called.Load())
	}

	pt.CloseInbound()
	if err := <-done; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

func TestServer_NotificationProducesNoResponse(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	got := atomic.Int32{}
	h.On("notifications/initialized", func(_ context.Context, _ *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		got.Add(1)
		return nil, nil
	})
	s := New(pt, h, Config{MaxConcurrentRequests: 1, RequestQueueSize: 1, ShutdownTimeout: 100 * time.Millisecond}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	pt.Send(t, notification("notifications/initialized"))

	// No response should be produced.
	select {
	case data := <-pt.conn.outbound:
		t.Fatalf("unexpected response for notification: %s", data)
	case <-time.After(200 * time.Millisecond):
	}
	if got.Load() != 1 {
		t.Fatalf("handler called %d times, want 1", got.Load())
	}

	pt.CloseInbound()
	if err := <-done; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

func TestServer_ToolsCallRunsThroughWorker(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	registry := newStubRegistry()

	h.On(MethodToolsCall, func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "executed"}, nil
	})

	s := New(pt, h, Config{MaxConcurrentRequests: 2, RequestQueueSize: 4, ShutdownTimeout: 200 * time.Millisecond},
		WithLogger(discardLogger()),
		WithCancelRegistry(registry),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	pt.Send(t, toolsCall(7, "describe_image"))
	resp := pt.Recv(t, time.Second)
	if !bytes.Contains(resp, []byte(`"executed"`)) {
		t.Fatalf("unexpected response: %s", resp)
	}

	// Wait for the worker's defer block to settle the registry and
	// counters. See [Server.runJob] for the documented defer order.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && s.Completed() < 1 {
		time.Sleep(5 * time.Millisecond)
	}

	// Registry must show 1 register and 1 done for ID 7.
	r, d := registry.Snapshot()
	if r != 1 || d != 1 {
		t.Fatalf("registry registers=%d dones=%d, want 1/1", r, d)
	}

	pt.CloseInbound()
	if err := <-done; err != nil {
		t.Fatalf("Run err = %v", err)
	}

	if got, want := s.Completed(), int64(1); got != want {
		t.Fatalf("Completed() = %d, want %d", got, want)
	}
}

func TestServer_HandlerErrorDoesNotKillEngine(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	h.On(MethodToolsCall, func(_ context.Context, _ *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		return nil, errors.New("boom")
	})
	s := New(pt, h, Config{MaxConcurrentRequests: 2, RequestQueueSize: 2, ShutdownTimeout: 100 * time.Millisecond}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	pt.Send(t, toolsCall(1, "tool1"))
	pt.Send(t, toolsCall(2, "tool2"))

	// Both should drain even though handler errors. Wait for completion.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.Completed() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s.Completed() < 2 {
		t.Fatalf("Completed = %d, want >=2", s.Completed())
	}

	pt.CloseInbound()
	if err := <-done; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

func TestServer_HandlerPanicIsContained(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	h.On(MethodToolsCall, func(_ context.Context, _ *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		panic("intentional panic")
	})
	s := New(pt, h, Config{MaxConcurrentRequests: 2, RequestQueueSize: 2, ShutdownTimeout: 100 * time.Millisecond}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	pt.Send(t, toolsCall(1, "panic-tool"))

	// Send a second job to verify the worker survived.
	h2 := atomic.Int32{}
	h.On("ping", func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		h2.Add(1)
		return &types.JSONRPCResponse{JSONRPC: types.JSONRPCVersion, ID: req.ID, Result: "pong"}, nil
	})
	pt.Send(t, []byte(`{"jsonrpc":"2.0","id":2,"method":"ping"}`))

	resp := pt.Recv(t, time.Second)
	if !bytes.Contains(resp, []byte(`"pong"`)) {
		t.Fatalf("expected pong response after panic, got %s", resp)
	}
	if h2.Load() != 1 {
		t.Fatalf("ping handler called %d times, want 1", h2.Load())
	}

	pt.CloseInbound()
	if err := <-done; err != nil {
		t.Fatalf("Run err = %v", err)
	}
}

func TestServer_CallerCtxCancellationStopsRecvLoop(t *testing.T) {
	pt := newPipeTransport()
	h := newStubHandler()
	s := New(pt, h, Config{MaxConcurrentRequests: 1, RequestQueueSize: 1, ShutdownTimeout: 100 * time.Millisecond}, WithLogger(discardLogger()))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after caller ctx cancel within 1s")
	}
}
