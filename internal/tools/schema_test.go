package tools

import (
	"sort"
	"testing"

	"github.com/AoManoh/openPic-mcp/internal/provider"
	"github.com/AoManoh/openPic-mcp/internal/service/tool"
	"github.com/AoManoh/openPic-mcp/internal/taskstore"
	"github.com/AoManoh/openPic-mcp/pkg/types"
)

// TestToolArraySchemasDeclareItems is the strict-MCP-client safety net.
// Windsurf and other strict clients reject any `array` schema that
// omits `items` (cf. issue: list_tasks states/kinds had this defect and
// blocked the entire MCP server from loading on Windows). The test
// walks every tool returned by allKnownTools() — sync AND async — so a
// future tool added to RegisterAll cannot regress the contract simply
// by being absent from this file.
func TestToolArraySchemasDeclareItems(t *testing.T) {
	for _, tool := range allKnownTools() {
		for name, property := range tool.InputSchema.Properties {
			assertArrayItems(t, tool.Name+".properties."+name, property)
		}
	}
}

func assertArrayItems(t *testing.T, path string, property types.Property) {
	t.Helper()
	if property.Type == "array" {
		if property.Items == nil {
			t.Fatalf("%s is array but items is nil", path)
		}
		assertArrayItems(t, path+".items", *property.Items)
	}
}

// TestAllKnownToolsCoversRegisterAll is the drift-prevention meta-test
// for the contract documented on allKnownTools(). It actually invokes
// RegisterAll with a stub provider and an enabled async bundle, then
// asserts that the set of registered tool names equals the set of
// names returned by allKnownTools(). If you add a tool to RegisterAll
// but forget to add it here (or vice-versa) this fails with a clear
// diff so the schema validator above cannot silently miss the new
// tool.
func TestAllKnownToolsCoversRegisterAll(t *testing.T) {
	manager := tool.NewManager()
	stubProv := &mockVisionProvider{}
	asyncBundle := &AsyncBundle{
		Store:      taskstore.NewMemory(taskstore.MemoryConfig{}),
		Dispatcher: &stubDispatcher{},
	}

	if err := RegisterAll(manager, stubProv, stubProv, WithAsync(asyncBundle)); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	registered := manager.List()
	gotNames := make([]string, 0, len(registered))
	for _, tl := range registered {
		gotNames = append(gotNames, tl.Name)
	}
	sort.Strings(gotNames)

	known := allKnownTools()
	wantNames := make([]string, 0, len(known))
	for _, tl := range known {
		wantNames = append(wantNames, tl.Name)
	}
	sort.Strings(wantNames)

	if len(gotNames) != len(wantNames) {
		t.Fatalf("RegisterAll registered %d tools, allKnownTools() lists %d; "+
			"both lists must be updated together.\nregistered=%v\nallKnown=%v",
			len(gotNames), len(wantNames), gotNames, wantNames)
	}
	for i := range gotNames {
		if gotNames[i] != wantNames[i] {
			t.Errorf("position %d: registered=%q allKnown=%q", i, gotNames[i], wantNames[i])
		}
	}
}

// Compile-time assertion: mockVisionProvider must satisfy both
// provider interfaces RegisterAll requires. The meta-test above relies
// on this; making the assertion explicit gives a clearer compile error
// if either interface evolves.
var (
	_ provider.VisionProvider = (*mockVisionProvider)(nil)
	_ provider.ImageProvider  = (*mockVisionProvider)(nil)
)

func TestCompareImagesToolImagesSchema(t *testing.T) {
	images := CompareImagesTool.InputSchema.Properties["images"]
	if images.Type != "array" {
		t.Fatalf("images type = %q, want array", images.Type)
	}
	if images.Items == nil {
		t.Fatal("images.items must not be nil")
	}
	if images.Items.Type != "string" {
		t.Fatalf("images.items.type = %q, want string", images.Items.Type)
	}
	if images.MinItems != MinImages {
		t.Fatalf("images.minItems = %d, want %d", images.MinItems, MinImages)
	}
	if images.MaxItems != DefaultMaxImages {
		t.Fatalf("images.maxItems = %d, want %d", images.MaxItems, DefaultMaxImages)
	}
}

// TestListTasksToolStatesItemsAreEnumerated is the targeted regression
// test for the Windsurf-on-Windows blocker (states/kinds were arrays
// without items). It pins three properties simultaneously:
//
//  1. `states` is an array whose item schema is a string with a
//     non-empty enum.
//  2. The enum values are exactly the canonical task-lifecycle states
//     (queued / running / completed / failed / cancelled / abandoned)
//     — so a future state addition must update the schema or this
//     test fires.
//  3. The enum values match the runtime validator validState(); the
//     two cannot drift.
//
// The same shape is asserted for `kinds` against KindGenerateImage /
// KindEditImage.
func TestListTasksToolStatesItemsAreEnumerated(t *testing.T) {
	props := ListTasksTool.InputSchema.Properties

	states, ok := props["states"]
	if !ok {
		t.Fatal("ListTasksTool must declare a 'states' property")
	}
	if states.Type != "array" {
		t.Fatalf("states.type = %q, want array", states.Type)
	}
	if states.Items == nil {
		t.Fatal("states.items must not be nil — strict MCP clients reject this")
	}
	if states.Items.Type != "string" {
		t.Fatalf("states.items.type = %q, want string", states.Items.Type)
	}
	wantStates := []string{
		string(taskstore.StateQueued),
		string(taskstore.StateRunning),
		string(taskstore.StateCompleted),
		string(taskstore.StateFailed),
		string(taskstore.StateCancelled),
		string(taskstore.StateAbandoned),
	}
	assertEnumEqual(t, "states.items.enum", states.Items.Enum, wantStates)
	// Pin schema-runtime parity: every enum value must be accepted
	// by the runtime validator.
	for _, s := range states.Items.Enum {
		if !validState(taskstore.State(s)) {
			t.Errorf("states.items.enum has %q but runtime validState rejects it", s)
		}
	}

	kinds, ok := props["kinds"]
	if !ok {
		t.Fatal("ListTasksTool must declare a 'kinds' property")
	}
	if kinds.Type != "array" {
		t.Fatalf("kinds.type = %q, want array", kinds.Type)
	}
	if kinds.Items == nil {
		t.Fatal("kinds.items must not be nil — strict MCP clients reject this")
	}
	if kinds.Items.Type != "string" {
		t.Fatalf("kinds.items.type = %q, want string", kinds.Items.Type)
	}
	wantKinds := []string{
		string(taskstore.KindGenerateImage),
		string(taskstore.KindEditImage),
	}
	assertEnumEqual(t, "kinds.items.enum", kinds.Items.Enum, wantKinds)
	for _, k := range kinds.Items.Enum {
		if !taskstore.Kind(k).IsValid() {
			t.Errorf("kinds.items.enum has %q but taskstore.Kind.IsValid rejects it", k)
		}
	}
}

func assertEnumEqual(t *testing.T, path string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: len=%d want %d (got=%v want=%v)", path, len(got), len(want), got, want)
	}
	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)
	sort.Strings(gotCopy)
	sort.Strings(wantCopy)
	for i := range gotCopy {
		if gotCopy[i] != wantCopy[i] {
			t.Errorf("%s[%d]: got=%q want=%q (full got=%v want=%v)",
				path, i, gotCopy[i], wantCopy[i], got, want)
		}
	}
}
