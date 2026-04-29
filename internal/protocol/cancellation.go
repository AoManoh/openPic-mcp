package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// CancellationParams matches the payload of an MCP `notifications/cancelled`
// notification. The MCP spec defines requestId as either a string or a number,
// so it must be decoded into [any] and normalized before lookup.
type CancellationParams struct {
	RequestID any    `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}

// ParseCancellationParams decodes the params block of a
// `notifications/cancelled` request into a [CancellationParams]. Notifications
// without a requestId are rejected to avoid silently cancelling random
// in-flight work.
func ParseCancellationParams(req *types.JSONRPCRequest) (*CancellationParams, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("missing params for notifications/cancelled")
	}
	var params CancellationParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, fmt.Errorf("failed to parse cancellation params: %w", err)
	}
	if params.RequestID == nil {
		return nil, fmt.Errorf("missing requestId in cancellation params")
	}
	return &params, nil
}

// CancellationRegistry tracks the cancel funcs for in-flight requests so a
// `notifications/cancelled` notification can interrupt the matching
// long-running tool call.
//
// The server engine populates the registry on tools/call entry and clears it
// after the handler returns. The protocol layer queries it when a cancel
// notification arrives.
//
// All operations are safe for concurrent callers: the engine fan-out and
// the receive loop share the same registry instance.
type CancellationRegistry struct {
	mu      sync.Mutex
	pending map[string]context.CancelFunc
}

// NewCancellationRegistry constructs an empty registry. Callers normally do
// not need to call this directly; [MCPHandler] wires one in by default.
func NewCancellationRegistry() *CancellationRegistry {
	return &CancellationRegistry{pending: make(map[string]context.CancelFunc)}
}

// Register records cancel under the request ID. If the same ID is registered
// twice (which would indicate a duplicate-id bug elsewhere) the previous
// cancel is invoked before being replaced so the old request still has a
// chance to unwind.
func (r *CancellationRegistry) Register(id any, cancel context.CancelFunc) {
	key, ok := normalizeID(id)
	if !ok || cancel == nil {
		return
	}
	r.mu.Lock()
	prev, exists := r.pending[key]
	r.pending[key] = cancel
	r.mu.Unlock()
	if exists {
		prev()
	}
}

// Cancel triggers the cancel func registered for id, if any. It returns
// true when a matching pending request was found.
func (r *CancellationRegistry) Cancel(id any) bool {
	key, ok := normalizeID(id)
	if !ok {
		return false
	}
	r.mu.Lock()
	cancel, found := r.pending[key]
	if found {
		delete(r.pending, key)
	}
	r.mu.Unlock()
	if found {
		cancel()
	}
	return found
}

// Done removes id from the registry without invoking cancel. The server
// engine calls Done in the request handler's defer so successful completions
// do not leak entries.
func (r *CancellationRegistry) Done(id any) {
	key, ok := normalizeID(id)
	if !ok {
		return
	}
	r.mu.Lock()
	delete(r.pending, key)
	r.mu.Unlock()
}

// Len returns the number of currently registered cancellations. It is
// intended for tests and observability; production code should not branch
// on the value.
func (r *CancellationRegistry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pending)
}

// normalizeID coerces a JSON-RPC ID (which may be a string, number, or nil)
// into a deterministic string key. Numbers come through json.Unmarshal as
// float64; the %v formatter renders them identically across the wire types
// we accept (int, int64, float64, string), so collisions only occur for
// matching wire values.
func normalizeID(id any) (string, bool) {
	if id == nil {
		return "", false
	}
	return fmt.Sprintf("%v", id), true
}
