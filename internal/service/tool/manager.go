// Package tool provides tool management for the Vision MCP Server.
package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/anthropic/vision-mcp-server/pkg/types"
)

// Manager manages tool registration and execution.
type Manager struct {
	tools    map[string]types.Tool
	handlers map[string]types.ToolHandler
	mu       sync.RWMutex
}

// NewManager creates a new tool manager.
func NewManager() *Manager {
	return &Manager{
		tools:    make(map[string]types.Tool),
		handlers: make(map[string]types.ToolHandler),
	}
}

// Register registers a tool with its handler.
func (m *Manager) Register(tool types.Tool, handler types.ToolHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tools[tool.Name]; exists {
		return fmt.Errorf("tool %s already registered", tool.Name)
	}

	m.tools[tool.Name] = tool
	m.handlers[tool.Name] = handler
	return nil
}

// Unregister removes a tool.
func (m *Manager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tools, name)
	delete(m.handlers, name)
}

// List returns all registered tools.
func (m *Manager) List() []types.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make([]types.Tool, 0, len(m.tools))
	for _, tool := range m.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Get returns a tool by name.
func (m *Manager) Get(name string) (types.Tool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tool, ok := m.tools[name]
	return tool, ok
}

// Execute executes a tool by name with the given arguments.
func (m *Manager) Execute(ctx context.Context, name string, args map[string]any) (*types.ToolCallResult, error) {
	m.mu.RLock()
	handler, ok := m.handlers[name]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	return handler(ctx, args)
}

// HasTool checks if a tool is registered.
func (m *Manager) HasTool(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.tools[name]
	return ok
}

// Count returns the number of registered tools.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.tools)
}
