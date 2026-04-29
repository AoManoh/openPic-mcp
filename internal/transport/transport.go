// Package transport provides transport layer abstractions for the openPic MCP
// server.
//
// The package follows the two-tier model used by the official MCP Go SDK:
//
//   - [Transport] is a factory that knows how to establish a single logical
//     connection. It is consumed exactly once by a server engine.
//   - [Connection] is the bidirectional message stream itself. It is the
//     contract the server engine talks to; it is fully context aware so that
//     blocking reads can be unblocked promptly when the engine wants to shut
//     down or cancel a request.
//
// Implementations are expected to be safe for concurrent calls to Write and
// Close while Read is in flight. Read itself is not required to be safe for
// concurrent callers because the server engine drives it from a single
// receive loop; documenting this avoids hidden assumptions in implementations.
package transport

import "context"

// Transport is a factory that produces exactly one [Connection].
//
// Higher-level engines call [Transport.Connect] when starting up. The returned
// Connection is owned by the caller, which is responsible for invoking
// [Connection.Close] when the engine shuts down.
type Transport interface {
	// Connect establishes the underlying transport stream and returns a
	// [Connection] suitable for the server engine to drive. The returned
	// connection must respect ctx cancellation for any blocking operations
	// performed during Connect itself.
	Connect(ctx context.Context) (Connection, error)
}

// Connection is a bidirectional, message-framed JSON-RPC stream.
//
// Read returns the next inbound message. Implementations must let ctx
// cancellation or [Connection.Close] unblock a pending Read promptly.
//
// Write transmits a single outbound message. Implementations must serialize
// concurrent calls so that framed messages on the wire are never interleaved.
//
// Close releases any resources held by the connection. After Close, a
// pending Read should return [io.EOF] (or another terminal error). Close
// itself must be safe to call multiple times, possibly concurrently with
// Read or Write.
type Connection interface {
	// Read returns the next message from the peer or an error. Implementations
	// must observe ctx cancellation and Close.
	Read(ctx context.Context) ([]byte, error)

	// Write sends payload to the peer as a single framed message. The framing
	// is defined by the implementation (e.g. newline-delimited for stdio).
	Write(ctx context.Context, payload []byte) error

	// Close terminates the connection. After Close any pending Read must
	// return promptly.
	Close() error
}
