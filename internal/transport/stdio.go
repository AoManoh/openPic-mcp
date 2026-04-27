package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// StdioTransport implements Transport using standard input/output.
// It reads JSON-RPC messages line by line from stdin and writes to stdout.
type StdioTransport struct {
	reader *bufio.Reader
	writer io.Writer
	mu     sync.Mutex
	closed bool
}

// NewStdioTransport creates a new StdioTransport using os.Stdin and os.Stdout.
func NewStdioTransport() *StdioTransport {
	return NewStdioTransportWithIO(os.Stdin, os.Stdout)
}

// NewStdioTransportWithIO creates a new StdioTransport with custom reader and writer.
// This is useful for testing.
func NewStdioTransportWithIO(r io.Reader, w io.Writer) *StdioTransport {
	return &StdioTransport{
		reader: bufio.NewReader(r),
		writer: w,
	}
}

// Read reads a single line (message) from stdin.
// Each message is expected to be a complete JSON-RPC message on a single line.
func (t *StdioTransport) Read() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, io.EOF
	}

	// Read until newline
	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			if len(line) > 0 {
				return trimLineEnding(line), nil
			}
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read from stdin: %w", err)
	}

	return trimLineEnding(line), nil
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

// Write writes a message to stdout followed by a newline.
func (t *StdioTransport) Write(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	// Write the data followed by a newline
	_, err := t.writer.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write to stdout: %w", err)
	}

	return nil
}

// Close closes the transport.
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	return nil
}
