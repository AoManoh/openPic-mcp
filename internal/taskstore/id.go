package taskstore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// TaskID is the canonical opaque identifier. The string form is
// guaranteed unique across all tasks ever produced by any process given
// three independent sources of disambiguation:
//
//  1. PID — a single host running concurrent server processes against the
//     same OPENPIC_OUTPUT_DIR is the multi-PID hazard scenario from SPEC
//     §2.1; embedding PID gives O(1) ownership checks.
//  2. RFC3339Nano timestamp — monotonic enough at nanosecond resolution
//     that two IDs produced in the same goroutine cannot collide.
//  3. 8 hex chars from crypto/rand — covers the residual case where the
//     same PID produces two IDs in the same nanosecond from different
//     goroutines (rare but possible on coarse clocks).
//
// Do not parse this string for business logic. The exposed [TaskID.PID]
// accessor exists exclusively for the cross-PID isolation check.
type TaskID string

// taskIDPrefix marks the format. Bumping it is a breaking change to all
// on-disk manifests; coordinate via SPEC if it ever needs to move.
const taskIDPrefix = "tsk_"

// taskIDRandBytes is the number of crypto/rand bytes per ID. 4 bytes →
// 8 hex chars in the suffix; 2^32 collisions per (pid, ns) tuple is
// far below any realistic submit rate.
const taskIDRandBytes = 4

// idCounter is a process-wide monotonic suffix that defends against the
// pathological case where time.Now() returns identical UnixNano values
// for two consecutive calls (observed on some virtualized clocks). It is
// stitched into the random suffix space, not the timestamp, so the
// timestamp segment remains a true wall-clock anchor for sorting.
var idCounter atomic.Uint64

// idGen is the package-level generator. Tests swap it out via
// [setIDGenerator] to feed deterministic IDs.
var idGen idGenerator = realIDGenerator{}

type idGenerator interface {
	next() (TaskID, error)
}

type realIDGenerator struct{}

func (realIDGenerator) next() (TaskID, error) {
	pid := os.Getpid()
	ts := time.Now().UTC().Format("20060102T150405.000000000Z")
	var randBuf [taskIDRandBytes]byte
	if _, err := rand.Read(randBuf[:]); err != nil {
		// crypto/rand failing is a host-level catastrophe; let the caller
		// surface it rather than silently degrade to a weaker source.
		return "", fmt.Errorf("taskstore: crypto/rand: %w", err)
	}
	// Mix in the monotonic counter so identical-ns collisions still yield
	// distinct IDs. The counter only needs to disambiguate within the same
	// (pid, ns) bucket; the random bytes still dominate the suffix.
	counter := idCounter.Add(1)
	suffix := fmt.Sprintf("%s%04x", hex.EncodeToString(randBuf[:]), counter&0xFFFF)
	return TaskID(fmt.Sprintf("%s%d_%s_%s", taskIDPrefix, pid, ts, suffix)), nil
}

// NewTaskID produces a fresh ID via the active generator. Callers should
// not invoke this directly; [Store.Submit] is the only legitimate caller.
// It is exported for the disk store tests that need to forge IDs to
// exercise the replay path.
func NewTaskID() (TaskID, error) {
	return idGen.next()
}

// PID returns the producer process's PID. Returns 0 and false if the ID
// does not match the canonical format (defensive — the disk replay path
// is the realistic source of malformed IDs from corrupted manifests).
func (id TaskID) PID() (int, bool) {
	s := string(id)
	if !strings.HasPrefix(s, taskIDPrefix) {
		return 0, false
	}
	rest := s[len(taskIDPrefix):]
	sep := strings.IndexByte(rest, '_')
	if sep <= 0 {
		return 0, false
	}
	pid, err := strconv.Atoi(rest[:sep])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// IsValid reports whether id parses as the canonical format. Used by the
// replay path before trusting a manifest filename.
func (id TaskID) IsValid() bool {
	_, ok := id.PID()
	return ok
}

// String makes TaskID printable in fmt verbs without explicit conversion.
func (id TaskID) String() string {
	return string(id)
}

// errEmptyID is the sentinel returned by callers that try to operate on
// the zero TaskID. It is intentionally not exported because no caller
// ever has a legitimate reason to ask the store about an empty ID.
var errEmptyID = errors.New("taskstore: empty task id")
