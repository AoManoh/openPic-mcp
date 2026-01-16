package transport

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestStdioTransport_Read(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "simple message",
			input:   `{"jsonrpc":"2.0","id":1,"method":"test"}` + "\n",
			want:    `{"jsonrpc":"2.0","id":1,"method":"test"}`,
			wantErr: false,
		},
		{
			name:    "message with CRLF",
			input:   `{"jsonrpc":"2.0","id":1,"method":"test"}` + "\r\n",
			want:    `{"jsonrpc":"2.0","id":1,"method":"test"}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			writer := &bytes.Buffer{}
			transport := NewStdioTransportWithIO(reader, writer)

			got, err := transport.Read()
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.want {
				t.Errorf("Read() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestStdioTransport_Write(t *testing.T) {
	writer := &bytes.Buffer{}
	transport := NewStdioTransportWithIO(strings.NewReader(""), writer)

	data := []byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}`)
	err := transport.Write(data)
	if err != nil {
		t.Fatalf("Write() error = %v, want nil", err)
	}

	want := `{"jsonrpc":"2.0","id":1,"result":"ok"}` + "\n"
	if writer.String() != want {
		t.Errorf("Write() wrote %v, want %v", writer.String(), want)
	}
}

func TestStdioTransport_Close(t *testing.T) {
	transport := NewStdioTransportWithIO(strings.NewReader(""), &bytes.Buffer{})

	err := transport.Close()
	if err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	// After close, Read should return EOF
	_, err = transport.Read()
	if err != io.EOF {
		t.Errorf("Read() after Close() error = %v, want io.EOF", err)
	}

	// After close, Write should return error
	err = transport.Write([]byte("test"))
	if err == nil {
		t.Error("Write() after Close() error = nil, want error")
	}
}

func TestStdioTransport_MultipleReads(t *testing.T) {
	input := `{"id":1}` + "\n" + `{"id":2}` + "\n" + `{"id":3}` + "\n"
	reader := strings.NewReader(input)
	transport := NewStdioTransportWithIO(reader, &bytes.Buffer{})

	expected := []string{`{"id":1}`, `{"id":2}`, `{"id":3}`}
	for i, want := range expected {
		got, err := transport.Read()
		if err != nil {
			t.Fatalf("Read() %d error = %v, want nil", i, err)
		}
		if string(got) != want {
			t.Errorf("Read() %d = %v, want %v", i, string(got), want)
		}
	}
}
