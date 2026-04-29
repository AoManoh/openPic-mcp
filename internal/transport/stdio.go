package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// StdioTransport implements [Transport] over a pair of byte streams using
// newline-delimited JSON framing. The default constructor wires it to
// [os.Stdin] and [os.Stdout], which is the standard topology for an MCP
// server launched as a child process by an MCP client.
type StdioTransport struct {
	reader io.Reader
	writer io.Writer
}

// NewStdio constructs a [StdioTransport] backed by [os.Stdin] and
// [os.Stdout].
func NewStdio() *StdioTransport {
	return NewStdioWithIO(os.Stdin, os.Stdout)
}

// NewStdioWithIO constructs a [StdioTransport] backed by an arbitrary reader
// and writer. The intended consumer is tests, which can plug in
// [bytes.Buffer] or [io.Pipe] endpoints.
func NewStdioWithIO(r io.Reader, w io.Writer) *StdioTransport {
	return &StdioTransport{reader: r, writer: w}
}

// Connect implements [Transport].
//
// Connect spawns a dedicated read goroutine that pumps newline-delimited
// frames from the underlying reader into an internal channel. This pattern
// (borrowed from the canonical implementation in
// modelcontextprotocol/go-sdk's mcp/transport.go) lets [Connection.Read]
// observe ctx cancellation and [Connection.Close] without depending on the
// reader supporting deadlines.
func (t *StdioTransport) Connect(_ context.Context) (Connection, error) {
	return newIOConn(t.reader, t.writer), nil
}

// ioConn is the [Connection] implementation produced by [StdioTransport]. It
// is package-private because callers should always go through the
// [Transport] interface.
type ioConn struct {
	reader *bufio.Reader
	writer io.Writer

	// incoming carries each decoded line from readLoop to Read. It is closed
	// by readLoop once it observes a terminal read error or a Close signal,
	// allowing Read to unblock with io.EOF on subsequent calls.
	incoming chan readResult

	// closed is signalled by Close. readLoop selects on it so that it can
	// abandon a pending channel send if no Read is currently waiting.
	closed    chan struct{}
	closeOnce sync.Once

	// writeMu serializes Write calls so that concurrent producers cannot
	// interleave bytes within a single message frame.
	writeMu sync.Mutex
}

// readResult is the value type carried over the incoming channel.
type readResult struct {
	data []byte
	err  error
}

func newIOConn(r io.Reader, w io.Writer) *ioConn {
	c := &ioConn{
		reader:   bufio.NewReader(r),
		writer:   w,
		incoming: make(chan readResult),
		closed:   make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// readLoop pumps newline-delimited frames into c.incoming. It terminates on
// the first terminal read error (including [io.EOF]) or when c.closed is
// signalled.
//
// Note: when the underlying reader is a blocking pipe (e.g. [os.Stdin]) and
// the parent process never closes it, readLoop may block in ReadBytes
// indefinitely after Close is called. This is unavoidable for stdio without
// platform-specific mechanisms; the leaked goroutine is reclaimed by the OS
// when the process exits.
func (c *ioConn) readLoop() {
	defer close(c.incoming)
	for {
		line, err := c.reader.ReadBytes('\n')
		var send readResult
		if len(line) > 0 {
			send.data = trimLineEnding(line)
		}
		if err != nil {
			send.err = err
		}

		// Skip the empty zero-value frame that arises from a clean io.EOF
		// (no trailing data). The defer above will close the channel and
		// surface EOF on the next Read.
		if send.data == nil && send.err == nil {
			return
		}

		select {
		case c.incoming <- send:
		case <-c.closed:
			return
		}

		if err != nil {
			return
		}
	}
}

// Read implements [Connection.Read]. It is not safe for concurrent callers;
// the server engine is expected to drive Read from a single receive loop.
func (c *ioConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, io.EOF
	case r, ok := <-c.incoming:
		if !ok {
			return nil, io.EOF
		}
		if r.err != nil {
			// Trailing data delivered together with EOF is still a valid
			// terminal message; surface it now and let the next Read return
			// EOF via the closed-channel path.
			if errors.Is(r.err, io.EOF) {
				if len(r.data) > 0 {
					return r.data, nil
				}
				return nil, io.EOF
			}
			return nil, fmt.Errorf("transport read: %w", r.err)
		}
		return r.data, nil
	}
}

// Write implements [Connection.Write]. Concurrent callers are serialized via
// writeMu so that newline framing is preserved even under fan-out from
// multiple worker goroutines.
func (c *ioConn) Write(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-c.closed:
		return errors.New("transport closed")
	default:
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// Re-check after acquiring the lock to avoid writing on a closed
	// connection that was racing with us.
	select {
	case <-c.closed:
		return errors.New("transport closed")
	default:
	}

	// Single-call Write so the writer sees one contiguous slice that ends in
	// the framing delimiter. Pre-allocating spares one allocation versus
	// append on a capped slice.
	frame := make([]byte, 0, len(payload)+1)
	frame = append(frame, payload...)
	frame = append(frame, '\n')
	if _, err := c.writer.Write(frame); err != nil {
		return fmt.Errorf("transport write: %w", err)
	}
	return nil
}

// Close implements [Connection.Close]. It is idempotent and safe to call
// concurrently with Read or Write.
func (c *ioConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return nil
}

func trimLineEnding(line []byte) []byte {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line
}
