package protocol

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

func TestCancellationRegistry_RegisterAndCancel(t *testing.T) {
	r := NewCancellationRegistry()
	_, cancel := context.WithCancel(context.Background())
	called := atomic.Bool{}
	wrapped := func() {
		called.Store(true)
		cancel()
	}

	r.Register(float64(7), wrapped)
	if got, want := r.Len(), 1; got != want {
		t.Fatalf("Len after Register = %d, want %d", got, want)
	}

	if !r.Cancel(float64(7)) {
		t.Fatal("Cancel returned false for registered ID")
	}
	if !called.Load() {
		t.Fatal("registered cancel func was not invoked")
	}
	if got, want := r.Len(), 0; got != want {
		t.Fatalf("Len after Cancel = %d, want %d", got, want)
	}
}

func TestCancellationRegistry_CancelUnknown(t *testing.T) {
	r := NewCancellationRegistry()
	if r.Cancel("missing") {
		t.Fatal("Cancel should return false for unknown ID")
	}
}

func TestCancellationRegistry_DoneRemovesWithoutCalling(t *testing.T) {
	r := NewCancellationRegistry()
	called := atomic.Bool{}
	r.Register("done-id", func() { called.Store(true) })

	r.Done("done-id")
	if got, want := r.Len(), 0; got != want {
		t.Fatalf("Len after Done = %d, want %d", got, want)
	}
	if called.Load() {
		t.Fatal("Done should not invoke cancel func")
	}
	if r.Cancel("done-id") {
		t.Fatal("Cancel should return false after Done")
	}
}

func TestCancellationRegistry_DuplicateRegisterCancelsPrevious(t *testing.T) {
	r := NewCancellationRegistry()
	prevCalled := atomic.Bool{}
	currCalled := atomic.Bool{}
	r.Register(int64(42), func() { prevCalled.Store(true) })
	r.Register(int64(42), func() { currCalled.Store(true) })

	if !prevCalled.Load() {
		t.Fatal("previous cancel func should fire when same ID is registered twice")
	}
	if currCalled.Load() {
		t.Fatal("current cancel func should not fire on registration")
	}
	if got, want := r.Len(), 1; got != want {
		t.Fatalf("Len = %d, want %d", got, want)
	}
	if !r.Cancel(int64(42)) {
		t.Fatal("Cancel should hit current cancel func")
	}
	if !currCalled.Load() {
		t.Fatal("current cancel func was not invoked")
	}
}

func TestCancellationRegistry_NormalizesIDTypes(t *testing.T) {
	r := NewCancellationRegistry()
	called := atomic.Bool{}
	r.Register(7, func() { called.Store(true) })

	// Float64 (json) should match int 7 via %v formatting.
	if !r.Cancel(float64(7)) {
		t.Fatal("Cancel(float64(7)) should match Register(7)")
	}
	if !called.Load() {
		t.Fatal("cancel func was not invoked")
	}
}

func TestCancellationRegistry_NilIDNoOp(t *testing.T) {
	r := NewCancellationRegistry()
	r.Register(nil, func() {}) // should not panic and should not store
	if got, want := r.Len(), 0; got != want {
		t.Fatalf("Len = %d, want %d", got, want)
	}
	if r.Cancel(nil) {
		t.Fatal("Cancel(nil) should always return false")
	}
}

func TestCancellationRegistry_ConcurrentRegisterCancel(t *testing.T) {
	r := NewCancellationRegistry()
	const N = 100

	var wg sync.WaitGroup
	wg.Add(N)
	cancelled := atomic.Int32{}
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			r.Register(i, func() { cancelled.Add(1) })
		}()
	}
	wg.Wait()

	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			r.Cancel(i)
		}()
	}
	wg.Wait()

	if got, want := int(cancelled.Load()), N; got != want {
		t.Fatalf("cancelled count = %d, want %d", got, want)
	}
	if got, want := r.Len(), 0; got != want {
		t.Fatalf("Len = %d, want %d", got, want)
	}
}

func TestParseCancellationParams(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantID  any
		wantErr bool
	}{
		{
			name:   "numeric id",
			raw:    `{"requestId":123,"reason":"client cancelled"}`,
			wantID: float64(123),
		},
		{
			name:   "string id",
			raw:    `{"requestId":"abc"}`,
			wantID: "abc",
		},
		{
			name:    "missing requestId",
			raw:     `{"reason":"oops"}`,
			wantErr: true,
		},
		{
			name:    "missing params",
			raw:     ``,
			wantErr: true,
		},
		{
			name:    "invalid json",
			raw:     `{not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.JSONRPCRequest{Method: MethodCancelled}
			if tt.raw != "" {
				req.Params = json.RawMessage(tt.raw)
			}
			got, err := ParseCancellationParams(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseCancellationParams() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.RequestID != tt.wantID {
				t.Errorf("RequestID = %#v, want %#v", got.RequestID, tt.wantID)
			}
		})
	}
}

func TestMCPHandler_NotificationsCancelledTriggersRegistry(t *testing.T) {
	h := NewMCPHandler()
	// Bypass initialization gating for the cancelled handler — the spec
	// allows notifications/cancelled to flow before initialization.
	cancelled := make(chan struct{}, 1)
	h.Cancellations().Register("req-1", func() {
		select {
		case cancelled <- struct{}{}:
		default:
		}
	})

	cancelMsg := []byte(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"req-1","reason":"user"}}`)
	resp, err := h.HandleMessage(context.Background(), cancelMsg)
	if err != nil {
		t.Fatalf("HandleMessage err = %v", err)
	}
	if resp != nil {
		t.Fatalf("notification should not produce a response, got %#v", resp)
	}

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("registered cancel func was not invoked within 1s")
	}
	if got, want := h.Cancellations().Len(), 0; got != want {
		t.Fatalf("registry Len = %d, want %d", got, want)
	}
}

func TestMCPHandler_NotificationsCancelledIgnoresMalformed(t *testing.T) {
	h := NewMCPHandler()
	// Should not crash and should not produce a response even when params
	// are malformed; the registry is a no-op on unknown IDs.
	resp, err := h.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/cancelled"}`))
	if err != nil {
		t.Fatalf("HandleMessage err = %v", err)
	}
	if resp != nil {
		t.Fatalf("notification should not produce a response, got %#v", resp)
	}
}

func TestMCPHandler_HandleMessagePassesContextToHandler(t *testing.T) {
	h := NewMCPHandler()
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "carry-me")

	got := make(chan any, 1)
	h.Router().Register("test/echo", func(c context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		got <- c.Value(ctxKey{})
		return NewSuccessResponse(req.ID, "ok"), nil
	})
	// Bypass initialization gate by marking initialized.
	h.initialized.Store(true)

	resp, err := h.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","id":9,"method":"test/echo"}`))
	if err != nil {
		t.Fatalf("HandleMessage err = %v", err)
	}
	if resp == nil || resp.Error != nil {
		t.Fatalf("unexpected response: %#v", resp)
	}
	select {
	case v := <-got:
		if v != "carry-me" {
			t.Fatalf("ctx value = %v, want %q", v, "carry-me")
		}
	case <-time.After(time.Second):
		t.Fatal("handler never received ctx")
	}
}

func TestMCPHandler_NotInitializedRejectsTools(t *testing.T) {
	h := NewMCPHandler()
	h.RegisterToolsHandlers(
		func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
			return NewSuccessResponse(req.ID, "should not run"), nil
		},
		func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
			return NewSuccessResponse(req.ID, "should not run"), nil
		},
	)

	resp, err := h.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("HandleMessage err = %v", err)
	}
	if resp == nil || resp.Error == nil {
		t.Fatalf("expected error response while uninitialized, got %#v", resp)
	}
	if resp.Error.Code != types.ErrCodeInvalidRequest {
		t.Errorf("error code = %v, want %v", resp.Error.Code, types.ErrCodeInvalidRequest)
	}
}

func TestMCPHandler_InitializedAllowsTools(t *testing.T) {
	h := NewMCPHandler()
	h.RegisterToolsHandlers(
		func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
			return NewSuccessResponse(req.ID, []string{"a", "b"}), nil
		},
		func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
			return NewSuccessResponse(req.ID, "ok"), nil
		},
	)

	if _, err := h.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)); err != nil {
		t.Fatalf("initialize err = %v", err)
	}
	if _, err := h.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); err != nil {
		t.Fatalf("initialized err = %v", err)
	}

	resp, err := h.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("tools/list err = %v", err)
	}
	if resp == nil || resp.Error != nil {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestMCPHandler_ConcurrentDispatchIsRaceFree(t *testing.T) {
	h := NewMCPHandler()
	h.initialized.Store(true)
	var counter atomic.Int32
	h.Router().Register("test/inc", func(_ context.Context, req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
		counter.Add(1)
		return NewSuccessResponse(req.ID, "ok"), nil
	})

	const N = 64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			payload := []byte(`{"jsonrpc":"2.0","id":` + intToStr(i) + `,"method":"test/inc"}`)
			if _, err := h.HandleMessage(context.Background(), payload); err != nil {
				t.Errorf("HandleMessage err = %v", err)
			}
		}()
	}
	wg.Wait()
	if got, want := int(counter.Load()), N; got != want {
		t.Fatalf("counter = %d, want %d", got, want)
	}
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	buf := [20]byte{}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
