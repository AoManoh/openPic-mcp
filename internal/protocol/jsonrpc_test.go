package protocol

import (
	"encoding/json"
	"testing"

	"github.com/anthropic/vision-mcp-server/pkg/types"
)

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *types.JSONRPCRequest
		wantErr bool
	}{
		{
			name:  "valid request",
			input: `{"jsonrpc":"2.0","id":1,"method":"test","params":{"key":"value"}}`,
			want: &types.JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      float64(1), // JSON numbers are decoded as float64
				Method:  "test",
				Params:  json.RawMessage(`{"key":"value"}`),
			},
			wantErr: false,
		},
		{
			name:  "notification (no id)",
			input: `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			want: &types.JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      nil,
				Method:  "notifications/initialized",
			},
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid}`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "wrong JSON-RPC version",
			input:   `{"jsonrpc":"1.0","id":1,"method":"test"}`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeRequest([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				if got.JSONRPC != tt.want.JSONRPC {
					t.Errorf("JSONRPC = %v, want %v", got.JSONRPC, tt.want.JSONRPC)
				}
				if got.Method != tt.want.Method {
					t.Errorf("Method = %v, want %v", got.Method, tt.want.Method)
				}
			}
		})
	}
}

func TestEncodeResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    *types.JSONRPCResponse
		wantErr bool
	}{
		{
			name: "success response",
			resp: &types.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      1,
				Result:  map[string]string{"status": "ok"},
			},
			wantErr: false,
		},
		{
			name: "error response",
			resp: &types.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      1,
				Error: &types.JSONRPCError{
					Code:    -32600,
					Message: "Invalid Request",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeResponse(tt.resp)
			if (err != nil) != tt.wantErr {
				t.Errorf("EncodeResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Verify it's valid JSON
				var decoded types.JSONRPCResponse
				if err := json.Unmarshal(got, &decoded); err != nil {
					t.Errorf("EncodeResponse() produced invalid JSON: %v", err)
				}
			}
		})
	}
}

func TestNewSuccessResponse(t *testing.T) {
	resp := NewSuccessResponse(1, map[string]string{"status": "ok"})
	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %v, want 2.0", resp.JSONRPC)
	}
	if resp.ID != 1 {
		t.Errorf("ID = %v, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("Error = %v, want nil", resp.Error)
	}
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse(1, -32600, "Invalid Request", nil)
	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %v, want 2.0", resp.JSONRPC)
	}
	if resp.Error == nil {
		t.Fatal("Error = nil, want error")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("Error.Code = %v, want -32600", resp.Error.Code)
	}
}

func TestIsNotification(t *testing.T) {
	tests := []struct {
		name string
		req  *types.JSONRPCRequest
		want bool
	}{
		{
			name: "notification",
			req:  &types.JSONRPCRequest{ID: nil, Method: "test"},
			want: true,
		},
		{
			name: "request with id",
			req:  &types.JSONRPCRequest{ID: 1, Method: "test"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotification(tt.req); got != tt.want {
				t.Errorf("IsNotification() = %v, want %v", got, tt.want)
			}
		})
	}
}
