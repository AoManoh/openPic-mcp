// Package tool provides tool management for the Vision MCP Server.
package tool

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/AoManoh/openPic-mcp/pkg/types"
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
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
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
func (m *Manager) Execute(ctx context.Context, name string, args map[string]any) (result *types.ToolCallResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("tool %s panicked: %v", name, recovered)
		}
	}()

	m.mu.RLock()
	handler, ok := m.handlers[name]
	toolDef := m.tools[name]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	if err := validateArguments(toolDef, args); err != nil {
		return nil, err
	}

	return handler(ctx, args)
}

func validateArguments(toolDef types.Tool, args map[string]any) error {
	schema := toolDef.InputSchema
	if schema.Type == "" {
		return nil
	}
	if args == nil {
		args = map[string]any{}
	}

	if !schema.AdditionalProperties {
		for key := range args {
			if _, ok := schema.Properties[key]; !ok {
				return fmt.Errorf("unknown parameter %q for tool %s", key, toolDef.Name)
			}
		}
	}

	for _, key := range schema.Required {
		if _, ok := args[key]; !ok {
			return fmt.Errorf("missing required parameter %q for tool %s", key, toolDef.Name)
		}
	}

	for key, value := range args {
		property, ok := schema.Properties[key]
		if !ok {
			continue
		}
		if err := validatePropertyValue(toolDef.Name, key, property, value); err != nil {
			return err
		}
	}

	return nil
}

func validatePropertyValue(toolName string, key string, property types.Property, value any) error {
	if value == nil {
		return nil
	}

	switch property.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("invalid parameter %q for tool %s: expected string", key, toolName)
		}
		if len(property.Enum) > 0 && !containsString(property.Enum, text) {
			return fmt.Errorf("invalid parameter %q for tool %s: expected one of %v", key, toolName, property.Enum)
		}
	case "integer":
		if !isInteger(value) {
			return fmt.Errorf("invalid parameter %q for tool %s: expected integer", key, toolName)
		}
	case "array":
		switch value.(type) {
		case []any, []string:
		default:
			return fmt.Errorf("invalid parameter %q for tool %s: expected array", key, toolName)
		}
	}

	return nil
}

func isInteger(value any) bool {
	switch number := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return float32(math.Trunc(float64(number))) == number
	case float64:
		return math.Trunc(number) == number
	default:
		return false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
