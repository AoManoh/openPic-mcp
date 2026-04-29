package taskstore

import (
	"strings"
	"sync"
	"testing"
)

// TestNewTaskID_FormatAndPID locks down the canonical ID format so any
// future tweak to id.go must update this test in lockstep.
func TestNewTaskID_FormatAndPID(t *testing.T) {
	id, err := NewTaskID()
	if err != nil {
		t.Fatalf("NewTaskID err=%v", err)
	}
	s := id.String()
	if !strings.HasPrefix(s, taskIDPrefix) {
		t.Fatalf("missing prefix: %q", s)
	}
	if !id.IsValid() {
		t.Fatalf("id %q reported invalid", s)
	}
	pid, ok := id.PID()
	if !ok || pid <= 0 {
		t.Fatalf("PID() = (%d, %v), want (>0, true)", pid, ok)
	}
}

// TestTaskID_NoCollision exercises the SPEC §2.1 invariant: 100 goroutines
// each generating 10k IDs must all be unique. Three independent sources
// of disambiguation (PID, ns timestamp, crypto/rand+counter) guarantee
// this; a regression here would mean someone weakened one of them.
func TestTaskID_NoCollision(t *testing.T) {
	const goroutines = 100
	const perGoroutine = 10_000
	total := goroutines * perGoroutine

	ids := make([]TaskID, total)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		base := g * perGoroutine
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				id, err := NewTaskID()
				if err != nil {
					t.Errorf("NewTaskID err=%v", err)
					return
				}
				ids[base+i] = id
			}
		}()
	}
	wg.Wait()

	seen := make(map[TaskID]struct{}, total)
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id detected: %q", id)
		}
		seen[id] = struct{}{}
	}
}

// TestTaskID_PIDMalformed verifies the defensive parser accepts only
// canonical IDs. Disk replay relies on this to reject corrupted manifest
// filenames.
func TestTaskID_PIDMalformed(t *testing.T) {
	cases := []TaskID{
		"",
		"not-a-task-id",
		"tsk_",
		"tsk_abc_xxx",      // pid not numeric
		"tsk_-1_xxx",       // pid not positive
		"prefix-tsk_1_xxx", // wrong prefix
	}
	for _, id := range cases {
		t.Run(string(id), func(t *testing.T) {
			if id.IsValid() {
				t.Fatalf("expected invalid: %q", id)
			}
		})
	}
}
