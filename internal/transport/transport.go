// Package transport provides transport layer implementations for the Vision MCP Server.
package transport

import "io"

// Transport defines the interface for MCP message transport.
type Transport interface {
	// Read reads a single message from the transport.
	// Returns the raw message bytes or an error.
	Read() ([]byte, error)

	// Write writes a message to the transport.
	// Returns an error if the write fails.
	Write(data []byte) error

	// Close closes the transport and releases any resources.
	Close() error
}

// ReaderWriter combines io.Reader and io.Writer interfaces.
type ReaderWriter interface {
	io.Reader
	io.Writer
}
