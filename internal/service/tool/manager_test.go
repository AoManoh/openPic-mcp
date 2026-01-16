package tool

import (
	"context"
	"testing"

	"github.com/AoManoh/openPic-mcp/pkg/types"
)

func TestManager_Register(t *testing.T) {
	m := NewManager()

	tool := types.Tool{
		Name:        "test_tool",
		Description: "A test tool",
	}
	handler := func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		return &types.ToolCallResult{
			Content: []types.ContentItem{{Type: "text", Text: "ok"}},
		}, nil
	}

	// Register should succeed
	err := m.Register(tool, handler)
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	// Duplicate registration should fail
	err = m.Register(tool, handler)
	if err == nil {
		t.Fatal("Register() duplicate error = nil, want error")
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager()

	// Empty list
	tools := m.List()
	if len(tools) != 0 {
		t.Errorf("List() len = %d, want 0", len(tools))
	}

	// Add tools
	tool1 := types.Tool{Name: "tool1", Description: "Tool 1"}
	tool2 := types.Tool{Name: "tool2", Description: "Tool 2"}
	handler := func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		return nil, nil
	}

	m.Register(tool1, handler)
	m.Register(tool2, handler)

	tools = m.List()
	if len(tools) != 2 {
		t.Errorf("List() len = %d, want 2", len(tools))
	}
}

func TestManager_Execute(t *testing.T) {
	m := NewManager()

	tool := types.Tool{Name: "echo", Description: "Echo tool"}
	handler := func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		msg, _ := args["message"].(string)
		return &types.ToolCallResult{
			Content: []types.ContentItem{{Type: "text", Text: msg}},
		}, nil
	}

	m.Register(tool, handler)

	// Execute registered tool
	result, err := m.Execute(context.Background(), "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "hello" {
		t.Errorf("Execute() result = %v, want 'hello'", result)
	}

	// Execute non-existent tool
	_, err = m.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("Execute() non-existent error = nil, want error")
	}
}

func TestManager_Unregister(t *testing.T) {
	m := NewManager()

	tool := types.Tool{Name: "test", Description: "Test"}
	handler := func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		return nil, nil
	}

	m.Register(tool, handler)
	if !m.HasTool("test") {
		t.Fatal("HasTool() = false, want true")
	}

	m.Unregister("test")
	if m.HasTool("test") {
		t.Fatal("HasTool() after Unregister = true, want false")
	}
}

func TestManager_Count(t *testing.T) {
	m := NewManager()

	if m.Count() != 0 {
		t.Errorf("Count() = %d, want 0", m.Count())
	}

	tool := types.Tool{Name: "test", Description: "Test"}
	handler := func(ctx context.Context, args map[string]any) (*types.ToolCallResult, error) {
		return nil, nil
	}

	m.Register(tool, handler)
	if m.Count() != 1 {
		t.Errorf("Count() = %d, want 1", m.Count())
	}
}
