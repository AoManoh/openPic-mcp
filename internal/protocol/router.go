package protocol

import (
	"sync"

	"github.com/anthropic/vision-mcp-server/pkg/types"
)

// Handler is the function signature for message handlers.
type Handler func(*types.JSONRPCRequest) (*types.JSONRPCResponse, error)

// Router routes JSON-RPC requests to their handlers.
type Router struct {
	handlers map[string]Handler
	mu       sync.RWMutex
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	return &Router{
		handlers: make(map[string]Handler),
	}
}

// Register registers a handler for a method.
func (r *Router) Register(method string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[method] = handler
}

// Unregister removes a handler for a method.
func (r *Router) Unregister(method string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handlers, method)
}

// Route routes a request to its handler.
func (r *Router) Route(req *types.JSONRPCRequest) (*types.JSONRPCResponse, error) {
	r.mu.RLock()
	handler, ok := r.handlers[req.Method]
	r.mu.RUnlock()

	if !ok {
		return NewMethodNotFoundError(req.ID, req.Method), nil
	}

	return handler(req)
}

// HasHandler checks if a handler is registered for a method.
func (r *Router) HasHandler(method string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[method]
	return ok
}

// Methods returns a list of registered methods.
func (r *Router) Methods() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	methods := make([]string, 0, len(r.handlers))
	for method := range r.handlers {
		methods = append(methods, method)
	}
	return methods
}
