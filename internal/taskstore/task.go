package taskstore

import "time"

// Kind identifies the tool that produced a task. It is part of the
// canonical manifest schema; renaming a constant is a backwards-incompat
// change to existing on-disk manifests.
type Kind string

const (
	KindGenerateImage Kind = "generate_image"
	KindEditImage     Kind = "edit_image"
)

// IsValid reports whether k is one of the canonical kinds. New kinds must
// be added here and to the manifest replay path simultaneously.
func (k Kind) IsValid() bool {
	switch k {
	case KindGenerateImage, KindEditImage:
		return true
	default:
		return false
	}
}

// State enumerates the canonical task lifecycle. The transition rules are
// authoritative in [allowedTransitions]; do not duplicate them in callers.
type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
	StateAbandoned State = "abandoned"
)

// IsTerminal reports whether s is a terminal state. Terminal tasks are
// eligible for TTL eviction and never re-enter the worker pool.
func (s State) IsTerminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateCancelled, StateAbandoned:
		return true
	default:
		return false
	}
}

// allowedTransitions encodes the state machine from SPEC §4. Reads are
// safe without locking because this map is initialized once at package
// load and never mutated.
var allowedTransitions = map[State]map[State]struct{}{
	StateQueued: {
		StateRunning:   {},
		StateCancelled: {},
		StateAbandoned: {},
	},
	StateRunning: {
		StateCompleted: {},
		StateFailed:    {},
		StateCancelled: {},
		StateAbandoned: {},
	},
	// Terminal states have no outgoing transitions; absent map entries are
	// equivalent to an empty set for [canTransition].
}

// canTransition reports whether moving from `from` to `to` is allowed by
// the canonical state machine. Same-state transitions are rejected to
// keep TransitionFn idempotency the caller's responsibility, not the
// store's.
func canTransition(from, to State) bool {
	if from == to {
		return false
	}
	allowed, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = allowed[to]
	return ok
}

// RequestSummary captures the user-facing inputs for a task. It is
// embedded in [Task] and serialized verbatim to the on-disk manifest so
// users can inspect the prompt and rebuild context after a crash. API
// keys MUST never land here; APIBaseURL is fine because it is operator
// config, not a secret.
type RequestSummary struct {
	Prompt       string            `json:"prompt"`
	Model        string            `json:"model"`
	Size         string            `json:"size,omitempty"`
	AspectRatio  string            `json:"aspect_ratio,omitempty"`
	OutputFormat string            `json:"output_format,omitempty"`
	APIBaseURL   string            `json:"api_base_url,omitempty"`
	Extras       map[string]string `json:"extras,omitempty"`
}

// Clone returns a deep copy so callers can mutate the result without
// affecting the canonical record stored under the Transition lock.
func (r RequestSummary) Clone() RequestSummary {
	clone := r
	if len(r.Extras) > 0 {
		clone.Extras = make(map[string]string, len(r.Extras))
		for k, v := range r.Extras {
			clone.Extras[k] = v
		}
	}
	return clone
}

// Result is the terminal payload for completed tasks. It contains only
// metadata about the resulting image; the bytes themselves live on disk
// at FilePath. Keeping bytes out of the in-memory store is the cornerstone
// of the bounded-memory invariant.
type Result struct {
	FilePath  string   `json:"file_path"`
	SizeBytes int64    `json:"size_bytes"`
	Format    string   `json:"format"`
	Warnings  []string `json:"warnings,omitempty"`
}

// Clone returns a deep copy.
func (r Result) Clone() Result {
	clone := r
	if len(r.Warnings) > 0 {
		clone.Warnings = append([]string(nil), r.Warnings...)
	}
	return clone
}

// Task is the canonical record. It is safe to copy by value and is in fact
// always returned by value from [Store] methods so callers can never poke
// at the live record. All mutations must go through [Store.Transition].
type Task struct {
	ID          TaskID         `json:"task_id"`
	Kind        Kind           `json:"kind"`
	State       State          `json:"state"`
	PID         int            `json:"pid"`
	SubmittedAt time.Time      `json:"submitted_at"`
	StartedAt   time.Time      `json:"started_at,omitempty"`
	FinishedAt  time.Time      `json:"finished_at,omitempty"`
	Request     RequestSummary `json:"request"`
	Result      *Result        `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	CancelHint  string         `json:"cancel_hint,omitempty"`
}

// Clone returns a deep copy that is safe to hand to external callers.
// Pointer fields (Result) and slice fields (Warnings, Extras) are also
// cloned so a mutation by the caller cannot bleed back into the store.
func (t Task) Clone() Task {
	clone := t
	clone.Request = t.Request.Clone()
	if t.Result != nil {
		r := t.Result.Clone()
		clone.Result = &r
	}
	return clone
}

// Filter narrows List results. Empty fields are wildcards. All=true
// includes tasks owned by other PIDs (read-only).
type Filter struct {
	States []State
	Kinds  []Kind
	Since  time.Time
	All    bool
}

// matches reports whether t satisfies f. PID filtering is delegated to
// the caller because the store knows its own PID and Filter does not.
func (f Filter) matches(t *Task) bool {
	if !f.Since.IsZero() && t.SubmittedAt.Before(f.Since) {
		return false
	}
	if len(f.States) > 0 && !containsState(f.States, t.State) {
		return false
	}
	if len(f.Kinds) > 0 && !containsKind(f.Kinds, t.Kind) {
		return false
	}
	return true
}

func containsState(haystack []State, needle State) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func containsKind(haystack []Kind, needle Kind) bool {
	for _, k := range haystack {
		if k == needle {
			return true
		}
	}
	return false
}
