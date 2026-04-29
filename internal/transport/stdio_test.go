package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// connect is a small helper that exercises the public Transport API in tests
// so individual cases do not need to repeat the Connect ceremony.
func connect(t *testing.T, in io.Reader, out io.Writer) Connection {
	t.Helper()
	conn, err := NewStdioWithIO(in, out).Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestStdioConn_ReadFrames(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple message",
			input: `{"jsonrpc":"2.0","id":1,"method":"test"}` + "\n",
			want:  `{"jsonrpc":"2.0","id":1,"method":"test"}`,
		},
		{
			name:  "message with CRLF",
			input: `{"jsonrpc":"2.0","id":1,"method":"test"}` + "\r\n",
			want:  `{"jsonrpc":"2.0","id":1,"method":"test"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := connect(t, strings.NewReader(tt.input), &bytes.Buffer{})

			got, err := conn.Read(context.Background())
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Read = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestStdioConn_ReadMessageWithoutTrailingNewline(t *testing.T) {
	conn := connect(t, strings.NewReader(`{"id":1}`), &bytes.Buffer{})

	got, err := conn.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != `{"id":1}` {
		t.Errorf("Read = %q, want %q", string(got), `{"id":1}`)
	}

	if _, err = conn.Read(context.Background()); !errors.Is(err, io.EOF) {
		t.Errorf("second Read err = %v, want io.EOF", err)
	}
}

func TestStdioConn_MultipleReads(t *testing.T) {
	input := `{"id":1}` + "\n" + `{"id":2}` + "\n" + `{"id":3}` + "\n"
	conn := connect(t, strings.NewReader(input), &bytes.Buffer{})

	for i, want := range []string{`{"id":1}`, `{"id":2}`, `{"id":3}`} {
		got, err := conn.Read(context.Background())
		if err != nil {
			t.Fatalf("Read %d: %v", i, err)
		}
		if string(got) != want {
			t.Errorf("Read %d = %q, want %q", i, string(got), want)
		}
	}

	if _, err := conn.Read(context.Background()); !errors.Is(err, io.EOF) {
		t.Errorf("final Read err = %v, want io.EOF", err)
	}
}

func TestStdioConn_Write(t *testing.T) {
	out := &bytes.Buffer{}
	conn := connect(t, strings.NewReader(""), out)

	if err := conn.Write(context.Background(), []byte(`{"id":1,"result":"ok"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, want := out.String(), `{"id":1,"result":"ok"}`+"\n"; got != want {
		t.Errorf("Write produced %q, want %q", got, want)
	}
}

func TestStdioConn_Close(t *testing.T) {
	// A pipe lets us verify Close unblocks an already-pending Read without
	// relying on EOF from the underlying reader.
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })

	conn := connect(t, pr, &bytes.Buffer{})

	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := conn.Read(context.Background())
		done <- result{data: data, err: err}
	}()

	// Give Read time to enter the channel-select. The goroutine starts
	// readLoop synchronously so a short sleep is sufficient.
	time.Sleep(10 * time.Millisecond)
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case r := <-done:
		if !errors.Is(r.err, io.EOF) {
			t.Errorf("Read after Close err = %v, want io.EOF", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Read did not unblock within 1s after Close")
	}

	if err := conn.Write(context.Background(), []byte("test")); err == nil {
		t.Error("Write after Close should fail")
	}

	// Idempotent Close must not panic.
	if err := conn.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestStdioConn_ReadHonoursContextCancellation(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })

	conn := connect(t, pr, &bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())

	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := conn.Read(ctx)
		done <- result{data: data, err: err}
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case r := <-done:
		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("Read err = %v, want context.Canceled", r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Read did not honour ctx cancellation within 1s")
	}
}

func TestStdioConn_WriteRejectsCancelledContext(t *testing.T) {
	conn := connect(t, strings.NewReader(""), &bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := conn.Write(ctx, []byte("test")); !errors.Is(err, context.Canceled) {
		t.Errorf("Write err = %v, want context.Canceled", err)
	}
}

// TestStdioConn_ConcurrentWrites verifies that interleaving across multiple
// goroutines never breaks newline framing. Run with `go test -race` to also
// exercise the writeMu-protected critical section.
func TestStdioConn_ConcurrentWrites(t *testing.T) {
	out := &bytes.Buffer{}
	// bytes.Buffer is not goroutine-safe; wrap it so the underlying
	// transport is solely responsible for serialization. Any interleaving
	// at the byte level would corrupt the buffer or split frames.
	syncOut := &syncWriter{w: out}
	conn := connect(t, strings.NewReader(""), syncOut)

	const (
		writers     = 16
		perWriter   = 64
		framePrefix = "frame-"
	)

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				payload := framePrefix + string(rune('A'+i)) + "-" + string(rune('a'+(j%26)))
				if err := conn.Write(context.Background(), []byte(payload)); err != nil {
					t.Errorf("Write: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	// Every line must be a complete frame with the exact prefix; the
	// number of frames must equal writers * perWriter exactly.
	frames := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if got, want := len(frames), writers*perWriter; got != want {
		t.Fatalf("frame count = %d, want %d", got, want)
	}
	for i, frame := range frames {
		if !strings.HasPrefix(frame, framePrefix) {
			t.Fatalf("frame %d corrupted: %q", i, frame)
		}
	}
}

// syncWriter mirrors writes through a mutex so the test does not race on
// the unsafe bytes.Buffer underneath. It does NOT serialize the bytes the
// transport produces; that is the transport's job.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
