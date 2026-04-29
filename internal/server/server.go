// Package server is the concurrent dispatch engine that wires a transport
// and a protocol-level handler together for the Vision MCP server.
//
// The engine follows the model adopted by mark3labs/mcp-go and the official
// modelcontextprotocol/go-sdk, adapted to this project's footprint:
//
//   - A single recv loop reads frames from [transport.Connection].
//   - `tools/call` requests are pushed onto a bounded work queue and serviced
//     by a fixed-size worker pool. All other JSON-RPC messages are
//     processed synchronously on the recv loop because they are sub-millisecond.
//   - Each `tools/call` runs under a per-request context derived from a
//     long-lived engine context. The cancel func is published into a
//     [CancelRegistry] so the protocol layer's `notifications/cancelled`
//     handler can interrupt the matching in-flight worker.
//   - On engine shutdown the recv loop returns first, the connection is
//     closed, the work queue is drained, and the engine waits up to
//     [Config.ShutdownTimeout] for in-flight workers to finish before
//     forcibly cancelling them via the engine context.
//
// The engine is intentionally decoupled from the protocol package: it talks
// to a small [Handler] interface and an optional [CancelRegistry], which
// the protocol layer can plug in. Tests can stub both without dragging the
// JSON-RPC machinery in.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AoManoh/openPic-mcp/internal/transport"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// MethodToolsCall is the JSON-RPC method name dispatched onto the worker
// pool. All other methods run synchronously on the recv loop.
const MethodToolsCall = "tools/call"

// Default tunables for [Config]. Phase A targets a balanced default; Phase B
// will revisit once production load data is in.
const (
	DefaultMaxConcurrentRequests = 16
	DefaultRequestQueueSize      = 64
	DefaultShutdownTimeout       = 30 * time.Second

	// Caps mirror the upper bounds adopted by mark3labs/mcp-go.
	maxConcurrentRequestsCap = 100
	requestQueueSizeCap      = 10000

	// forceCancelGrace is the small window we still wait after firing the
	// engine-level cancel during a timed-out shutdown so well-behaved
	// handlers have a final chance to unwind.
	forceCancelGrace = 2 * time.Second
)

// Handler is the contract the engine uses to dispatch decoded requests.
//
// The protocol package's MCPHandler satisfies this interface as-is; tests
// can plug a stub.
type Handler interface {
	HandleMessage(ctx context.Context, raw []byte) (*types.JSONRPCResponse, error)
}

// CancelRegistry is the cancellation ledger the engine populates around
// each `tools/call`. The protocol package's CancellationRegistry satisfies
// this interface; an engine started without one simply skips cancel
// publishing (e.g. tests that never send `notifications/cancelled`).
type CancelRegistry interface {
	Register(id any, cancel context.CancelFunc)
	Done(id any)
}

// Config tunes the engine. All fields are optional; zero values fall back
// to the Default* constants above.
type Config struct {
	// MaxConcurrentRequests caps the number of `tools/call` handlers that
	// can run in parallel. The pool is fixed in size for the lifetime of
	// the engine; goroutines are reused.
	MaxConcurrentRequests int

	// RequestQueueSize bounds the buffered channel between the recv loop
	// and the worker pool. When the queue is full a synchronous fallback
	// kicks in so requests are never silently dropped.
	RequestQueueSize int

	// RequestTimeout, if non-zero, derives every `tools/call` ctx with a
	// timeout. Zero means callers/clients control timeouts (the default
	// expectation for image generation, which can legitimately take 90s+).
	RequestTimeout time.Duration

	// ShutdownTimeout bounds how long [Server.Run] waits for in-flight
	// workers to finish after the recv loop has stopped. After the timeout
	// the engine forcibly cancels them via the engine context.
	ShutdownTimeout time.Duration
}

// Option mutates a [Server]'s optional dependencies.
type Option func(*Server)

// WithLogger wires a *slog.Logger for engine events. Passing nil is a
// no-op so callers can pipe optional config straight through.
func WithLogger(l *slog.Logger) Option {
	return func(s *Server) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithCancelRegistry wires the protocol layer's cancellation ledger so the
// engine can register per-request cancel funcs around every `tools/call`.
// Passing nil is a no-op.
func WithCancelRegistry(r CancelRegistry) Option {
	return func(s *Server) {
		if r != nil {
			s.cancels = r
		}
	}
}

// Server is the engine. It is constructed once via [New] and driven by
// [Server.Run]. It is not safe to call Run concurrently; the engine
// expects a single owner goroutine.
type Server struct {
	transport transport.Transport
	handler   Handler
	cancels   CancelRegistry
	cfg       Config
	logger    *slog.Logger

	workQueue chan job
	inflight  sync.WaitGroup

	started  atomic.Bool
	shutdown atomic.Bool

	metrics Metrics
}

// Metrics exposes lightweight counters for observability and tests. All
// fields are intended to be read via the accessor methods on [Server].
type Metrics struct {
	inflight  atomic.Int64
	completed atomic.Int64
	fallback  atomic.Int64
	rejected  atomic.Int64
}

// job is one unit of asynchronous work pushed onto the work queue.
type job struct {
	ctx     context.Context
	cancel  context.CancelFunc
	id      any
	method  string
	raw     []byte
	enqueue time.Time
}

// New constructs a [Server]. The engine is inert until [Server.Run] is
// invoked. The default logger discards output so tests stay quiet by
// default; production callers should always inject a logger via
// [WithLogger].
func New(t transport.Transport, h Handler, cfg Config, opts ...Option) *Server {
	if t == nil {
		panic("server: transport must not be nil")
	}
	if h == nil {
		panic("server: handler must not be nil")
	}
	s := &Server{
		transport: t,
		handler:   h,
		cfg:       normalizeConfig(cfg),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	s.workQueue = make(chan job, s.cfg.RequestQueueSize)
	return s
}

// Config returns the effective configuration after defaults and caps have
// been applied. Useful for tests and observability surfaces.
func (s *Server) Config() Config { return s.cfg }

// Inflight returns the number of in-flight requests. It is intended for
// tests and observability surfaces.
func (s *Server) Inflight() int64 { return s.metrics.inflight.Load() }

// Completed returns the total number of completed requests since [New].
func (s *Server) Completed() int64 { return s.metrics.completed.Load() }

// FallbackCount returns the number of requests serviced via the
// queue-full synchronous fallback path.
func (s *Server) FallbackCount() int64 { return s.metrics.fallback.Load() }

// QueueDepth returns the current depth of the work queue.
func (s *Server) QueueDepth() int { return len(s.workQueue) }

// Run drives the engine until ctx is cancelled or the connection ends.
// It blocks on the recv loop and, on return, performs graceful shutdown.
//
// Run is single-shot; calling it twice on the same [Server] returns an
// error so callers do not accidentally start parallel recv loops.
func (s *Server) Run(ctx context.Context) error {
	if !s.started.CompareAndSwap(false, true) {
		return errors.New("server: already started")
	}

	conn, err := s.transport.Connect(ctx)
	if err != nil {
		return fmt.Errorf("server: connect: %w", err)
	}
	var connOnce sync.Once
	closeConn := func() { connOnce.Do(func() { _ = conn.Close() }) }
	defer closeConn()

	// engineCtx is the long-lived parent for every per-request ctx the
	// engine derives. It is detached from the caller's ctx so the recv
	// loop can stop on caller cancellation while in-flight workers keep
	// running until ShutdownTimeout elapses.
	engineCtx, engineCancel := context.WithCancel(context.Background())
	defer engineCancel()

	// Spawn workers. They live for the lifetime of the engine and exit
	// when the work queue is closed.
	var workersWG sync.WaitGroup
	workersWG.Add(s.cfg.MaxConcurrentRequests)
	for i := 0; i < s.cfg.MaxConcurrentRequests; i++ {
		workerID := i
		go func() {
			defer workersWG.Done()
			s.worker(workerID, conn)
		}()
	}

	s.logger.Info("server.started",
		"workers", s.cfg.MaxConcurrentRequests,
		"queue_size", s.cfg.RequestQueueSize,
		"shutdown_timeout", s.cfg.ShutdownTimeout.String(),
	)

	runErr := s.recvLoop(ctx, engineCtx, conn)

	// Begin graceful shutdown. Closing the work queue lets workers drain
	// any remaining buffered jobs and then exit. The connection stays
	// open here so in-flight workers can still flush their responses;
	// closing it before in-flight drains would silently drop replies.
	s.shutdown.Store(true)
	close(s.workQueue)

	// Wait for in-flight to settle. If they exceed the budget, fire the
	// engine ctx so any handler honouring ctx.Done returns promptly.
	waitDone := make(chan struct{})
	go func() {
		s.inflight.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(s.cfg.ShutdownTimeout):
		s.logger.Warn("server.shutdown_timeout_exceeded",
			"timeout", s.cfg.ShutdownTimeout.String(),
			"inflight", s.Inflight(),
		)
		engineCancel()
		select {
		case <-waitDone:
		case <-time.After(forceCancelGrace):
			s.logger.Warn("server.shutdown_force_abandon",
				"inflight", s.Inflight(),
			)
		}
	}

	// Now that no handler is touching the connection, it is safe to
	// release transport resources.
	closeConn()
	workersWG.Wait()
	s.logger.Info("server.stopped",
		"completed", s.Completed(),
		"fallback", s.metrics.fallback.Load(),
	)
	return runErr
}

// recvLoop is the single-goroutine read driver. It blocks on [Connection.Read],
// peeks the JSON-RPC envelope, and either dispatches synchronously
// (non-tools/call) or enqueues for the worker pool (tools/call).
func (s *Server) recvLoop(ctx context.Context, engineCtx context.Context, conn transport.Connection) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		raw, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("server: read: %w", err)
		}
		if len(raw) == 0 {
			continue
		}
		method, id := peekMethodAndID(raw)
		s.logger.Debug("req.received", "id", id, "method", method)

		// Non-tools/call traffic stays on the recv loop. These messages
		// are sub-millisecond (initialize, tools/list, notifications) and
		// pushing them through the queue would only add latency without a
		// concurrency win.
		//
		// inflight.Add is intentionally performed here in the single
		// recv-loop goroutine so that no Add can race with the
		// shutdown-time inflight.Wait() — see [Server.Run].
		if method != MethodToolsCall {
			s.inflight.Add(1)
			s.metrics.inflight.Add(1)
			s.runOnLoop(engineCtx, conn, raw, id, method)
			continue
		}

		// Per-request ctx with cancel; register so notifications/cancelled
		// can hit it. Timeouts (when configured) are layered on top.
		reqCtx, reqCancel := context.WithCancel(engineCtx)
		if s.cfg.RequestTimeout > 0 {
			reqCtx, reqCancel = context.WithTimeout(reqCtx, s.cfg.RequestTimeout)
		}
		if s.cancels != nil && id != nil {
			s.cancels.Register(id, reqCancel)
		}

		j := job{
			ctx:     reqCtx,
			cancel:  reqCancel,
			id:      id,
			method:  method,
			raw:     raw,
			enqueue: time.Now(),
		}

		// Reserve a slot in the inflight counter BEFORE enqueueing so
		// shutdown's inflight.Wait() never sees a transient zero in
		// between worker iterations.
		s.inflight.Add(1)
		s.metrics.inflight.Add(1)

		select {
		case s.workQueue <- j:
			s.logger.Debug("req.dispatched",
				"id", id, "method", method,
				"queue_depth", len(s.workQueue),
				"inflight", s.Inflight(),
			)
		default:
			// Queue full: synchronous fallback on the recv loop. This is
			// strictly better than dropping the request and never worse
			// than the legacy serial behaviour.
			s.metrics.fallback.Add(1)
			s.logger.Warn("req.queue_full_fallback",
				"id", id, "method", method,
				"queue_depth", len(s.workQueue),
				"inflight", s.Inflight(),
			)
			s.runJob(conn, j)
		}
	}
}

// worker consumes the work queue until it is closed. Each worker is
// dedicated to `tools/call` execution.
func (s *Server) worker(id int, conn transport.Connection) {
	for j := range s.workQueue {
		s.runJob(conn, j)
	}
	s.logger.Debug("server.worker_stopped", "worker_id", id)
}

// runJob executes one queued `tools/call` job under its per-request ctx.
// It is the only place that pairs Register with Done so the cancellation
// ledger never leaks even when the handler panics.
//
// inflight.Add is performed by the recv loop (single producer) before the
// job is published; runJob therefore only takes responsibility for Done.
// The defer ordering is deliberate: the registry must be cleared and the
// completed counter incremented BEFORE [sync.WaitGroup.Done] releases any
// concurrent observer. Tests that wait on s.Completed() == N can then
// safely snapshot the cancellation ledger without racing the worker.
func (s *Server) runJob(conn transport.Connection, j job) {
	defer func() {
		if s.cancels != nil && j.id != nil {
			s.cancels.Done(j.id)
		}
		j.cancel()
		s.metrics.inflight.Add(-1)
		s.metrics.completed.Add(1)
		s.inflight.Done()
	}()
	s.execute(j.ctx, conn, j.raw, j.id, j.method, j.enqueue)
}

// runOnLoop services non-tools/call traffic synchronously on the recv
// loop. inflight.Add is the recv loop's responsibility (see recvLoop);
// runOnLoop only signals Done in its defer.
func (s *Server) runOnLoop(engineCtx context.Context, conn transport.Connection, raw []byte, id any, method string) {
	defer func() {
		s.metrics.inflight.Add(-1)
		s.metrics.completed.Add(1)
		s.inflight.Done()
	}()
	s.execute(engineCtx, conn, raw, id, method, time.Now())
}

// execute dispatches one request to the handler, writes the response (if
// any), and emits the standard set of structured events. It must be
// resilient to handler panics so a single rogue tool cannot tear down a
// worker goroutine.
func (s *Server) execute(ctx context.Context, conn transport.Connection, raw []byte, id any, method string, enqueueAt time.Time) {
	start := time.Now()
	queueWait := start.Sub(enqueueAt)

	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("req.panic",
				"id", id, "method", method,
				"recovered", fmt.Sprintf("%v", rec),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}
	}()

	resp, err := s.handler.HandleMessage(ctx, raw)
	duration := time.Since(start)

	if err != nil {
		// Honour ctx cancellation as a non-error path; the worker is
		// expected to unwind quickly and the response may be intentionally
		// suppressed.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.logger.Info("req.cancelled",
				"id", id, "method", method,
				"reason", err.Error(),
				"duration_ms", duration.Milliseconds(),
			)
			return
		}
		s.logger.Error("req.failed",
			"id", id, "method", method,
			"err", err.Error(),
			"duration_ms", duration.Milliseconds(),
		)
		return
	}

	// Notifications return nil response by design.
	if resp == nil {
		s.logger.Debug("req.completed",
			"id", id, "method", method,
			"kind", "notification",
			"duration_ms", duration.Milliseconds(),
			"queue_wait_ms", queueWait.Milliseconds(),
		)
		return
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error("req.encode_failed",
			"id", id, "method", method,
			"err", err.Error(),
		)
		return
	}

	if err := conn.Write(ctx, payload); err != nil {
		// Write errors are logged but never re-enqueued: the connection
		// is the same single sink for every handler, so a write failure
		// after the recv loop has stopped is part of normal shutdown.
		s.logger.Error("req.write_failed",
			"id", id, "method", method,
			"err", err.Error(),
		)
		return
	}

	s.logger.Debug("req.completed",
		"id", id, "method", method,
		"duration_ms", duration.Milliseconds(),
		"queue_wait_ms", queueWait.Milliseconds(),
	)
}

// peekMethodAndID decodes only the envelope's method and id without
// allocating the full request body. The engine cares about these two
// fields for routing decisions; the handler still re-parses the raw
// frame for full validation.
func peekMethodAndID(raw []byte) (string, any) {
	var hdr struct {
		Method string `json:"method"`
		ID     any    `json:"id"`
	}
	_ = json.Unmarshal(raw, &hdr)
	return hdr.Method, hdr.ID
}

func normalizeConfig(c Config) Config {
	if c.MaxConcurrentRequests <= 0 {
		c.MaxConcurrentRequests = DefaultMaxConcurrentRequests
	} else if c.MaxConcurrentRequests > maxConcurrentRequestsCap {
		c.MaxConcurrentRequests = maxConcurrentRequestsCap
	}
	if c.RequestQueueSize <= 0 {
		c.RequestQueueSize = DefaultRequestQueueSize
	} else if c.RequestQueueSize > requestQueueSizeCap {
		c.RequestQueueSize = requestQueueSizeCap
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = DefaultShutdownTimeout
	}
	return c
}
