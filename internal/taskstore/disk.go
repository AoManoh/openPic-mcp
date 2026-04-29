package taskstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DiskConfig configures a [DiskStore]. Dir is the only required field;
// everything else inherits MemoryStore defaults.
type DiskConfig struct {
	// Dir is the absolute filesystem path that holds task manifests.
	// MUST be absolute (no relative paths, no `..` segments). The
	// directory is created with 0o755 if missing; permission failures
	// are surfaced fail-fast from NewDisk so operator misconfig cannot
	// degrade silently into "tasks succeed but never persist". The
	// production deployment expectation is that this points at a
	// subdirectory of OPENPIC_OUTPUT_DIR (e.g. $OPENPIC_OUTPUT_DIR/tasks)
	// so operators only authorize one filesystem location.
	Dir string

	// MaxQueued, MaxRetained, Now, PID — see [MemoryConfig].
	MaxQueued   int
	MaxRetained int
	Now         func() time.Time
	PID         int

	// Logger receives replay warnings, atomic-write failures and evict
	// failures. Nil → events are dropped to io.Discard. Production callers
	// should pass the slog.Logger built by config.NewLogger so events
	// land on stderr alongside the rest of the server lifecycle stream.
	Logger *slog.Logger
}

// DiskStore extends [MemoryStore] with on-disk persistence.
//
// Persistence model:
//
//   - Each task lives in <Dir>/<task_id>.json. Manifest writes use the
//     canonical temp-file + fsync + rename atomic-replace pattern so a
//     crash mid-write never leaves a torn manifest.
//   - The OnTransition / OnEvict hooks run under MemoryStore's write lock,
//     which serializes manifest writes for a single task and gives them
//     monotonic ordering aligned with the in-memory state machine.
//   - Failed disk writes are logged but never roll back the in-memory
//     transition. The runtime source of truth is RAM; manifests are
//     recovery aid. This is the "best-effort" promise documented at
//     [MemoryConfig.OnTransition].
//
// Replay model:
//
//   - At startup NewDisk scans Dir, parses every *.json file, validates
//     ID and Kind, and restores each task into the in-memory store.
//   - Tasks recovered in StateQueued or StateRunning could not have been
//     gracefully terminated (the previous process didn't write a final
//     manifest) so they are transitioned to StateAbandoned with hint
//     "process_restart" and re-persisted. Callers can poll get_task_result
//     after restart and observe the abandonment instead of hanging on a
//     ghost task that never completes.
//
// DiskStore implements the full [Store] interface by virtue of embedding
// *MemoryStore. The wrapper itself owns only the disk side of the world.
type DiskStore struct {
	*MemoryStore
	dir string
	log *slog.Logger
}

// NewDisk constructs a DiskStore. Order of operations:
//
//  1. Validate that Dir is an absolute path with no ".." segments.
//  2. MkdirAll(Dir) and write-probe — failures here are fatal.
//  3. Read every *.json under Dir; emit a slog.Warn per malformed file
//     but never fail the whole load over one corrupted manifest.
//  4. Build the underlying MemoryStore WITH lifecycle hooks wired.
//  5. restore() each parsed task; the restore path skips hooks so the
//     just-read manifests are not re-emitted.
//  6. Transition every queued/running task to StateAbandoned. This DOES
//     fire OnTransition, persisting the abandonment.
//
// Returns the ready store and nil, or nil and a fatal error.
func NewDisk(cfg DiskConfig) (*DiskStore, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("taskstore: DiskConfig.Dir is required")
	}
	if !filepath.IsAbs(cfg.Dir) {
		return nil, fmt.Errorf("taskstore: DiskConfig.Dir must be absolute, got %q", cfg.Dir)
	}
	if strings.Contains(filepath.Clean(cfg.Dir), "..") {
		return nil, fmt.Errorf("taskstore: DiskConfig.Dir must not contain .. segments, got %q", cfg.Dir)
	}

	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("taskstore: mkdir %q: %w", cfg.Dir, err)
	}
	if err := probeWritable(cfg.Dir); err != nil {
		return nil, fmt.Errorf("taskstore: directory %q is not writable: %w", cfg.Dir, err)
	}

	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	d := &DiskStore{dir: cfg.Dir, log: log}

	// Phase 1 — read existing manifests. We tolerate corruption (skip
	// + warn) so a single bad file never kills the whole process.
	parsed, err := readAllManifests(cfg.Dir, log)
	if err != nil {
		return nil, fmt.Errorf("taskstore: read dir %q: %w", cfg.Dir, err)
	}

	// Phase 2 — build MemoryStore with hooks pointing back at d.
	d.MemoryStore = NewMemory(MemoryConfig{
		MaxQueued:    cfg.MaxQueued,
		MaxRetained:  cfg.MaxRetained,
		Now:          cfg.Now,
		PID:          cfg.PID,
		OnTransition: d.persistManifest,
		OnEvict:      d.deleteManifest,
	})

	// Phase 3 — restore parsed tasks. restore() skips hooks; we collect
	// IDs that need an abandonment transition for the next phase.
	abandonable := make([]TaskID, 0)
	for _, t := range parsed {
		if err := d.MemoryStore.restore(t); err != nil {
			log.Warn("taskstore.replay.skip_restore",
				"id", t.ID, "state", t.State, "err", err)
			continue
		}
		if t.PID == d.MemoryStore.pid &&
			(t.State == StateQueued || t.State == StateRunning) {
			abandonable = append(abandonable, t.ID)
		}
	}

	// Phase 4 — abandon recovered queued/running tasks. The hook fires
	// here, persisting StateAbandoned to disk in lockstep with memory.
	// Foreign-PID queued/running tasks are NOT abandoned: another peer
	// process may still own them and we must not forge state for it.
	for _, id := range abandonable {
		err := d.MemoryStore.Transition(context.Background(), id,
			func(t *Task) error {
				t.State = StateAbandoned
				if t.CancelHint == "" {
					t.CancelHint = "process_restart"
				}
				return nil
			})
		if err != nil {
			log.Error("taskstore.replay.abandon_failed",
				"id", id, "err", err)
		}
	}

	log.Info("taskstore.disk.ready",
		"dir", cfg.Dir,
		"loaded", len(parsed),
		"abandoned", len(abandonable))
	return d, nil
}

// Dir returns the directory holding manifests. Useful for tests and
// administrative tooling; not on the [Store] interface.
func (d *DiskStore) Dir() string { return d.dir }

// persistManifest is the OnTransition hook. It writes the current task
// snapshot via atomic temp + rename. Failures are logged but never
// propagated — see the package-level "best effort" doc.
func (d *DiskStore) persistManifest(t Task) {
	path := d.manifestPath(t.ID)
	if err := writeJSONAtomic(path, &t); err != nil {
		d.log.Error("taskstore.disk.write_failed",
			"id", t.ID, "state", t.State, "path", path, "err", err)
	}
}

// deleteManifest is the OnEvict hook. ENOENT is treated as success
// because replay can race with eviction in pathological orderings.
func (d *DiskStore) deleteManifest(id TaskID) {
	path := d.manifestPath(id)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		d.log.Error("taskstore.disk.delete_failed",
			"id", id, "path", path, "err", err)
	}
}

// manifestPath returns the canonical filesystem path for a given task.
func (d *DiskStore) manifestPath(id TaskID) string {
	return filepath.Join(d.dir, string(id)+".json")
}

// probeWritable verifies the caller has create + delete permission on
// the directory by writing and removing a small temp file. CreateTemp
// alone is not enough: some filesystems allow create but not unlink in
// readonly-ish overlay configurations.
func probeWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(name)
		return cerr
	}
	return os.Remove(name)
}

// writeJSONAtomic encodes v to path via the canonical temp + fsync +
// rename pattern. The temp file is created in the same directory as the
// final path so rename is atomic on every supported filesystem
// (cross-device rename would otherwise fail with EXDEV).
//
// On any error the temp file is removed so the directory never
// accumulates partial writes. Callers should pass values that JSON
// marshal cleanly; failure here is a programming error.
func writeJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-tsk_*")
	if err != nil {
		return fmt.Errorf("create temp in %q: %w", dir, err)
	}
	tmp := f.Name()

	cleanup := func() {
		_ = os.Remove(tmp)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("encode %q: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("fsync %q: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return fmt.Errorf("rename %q -> %q: %w", tmp, path, err)
	}
	return nil
}

// readAllManifests walks dir and returns every parsed task. Files that
// fail to read or parse are skipped with a slog.Warn so a single bad
// manifest never poisons the whole replay.
func readAllManifests(dir string, log *slog.Logger) ([]Task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip our own atomic-write tempfiles and any hidden files.
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			log.Warn("taskstore.replay.read_failed",
				"path", path, "err", err)
			continue
		}
		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			log.Warn("taskstore.replay.parse_failed",
				"path", path, "err", err)
			continue
		}
		if !t.ID.IsValid() {
			log.Warn("taskstore.replay.invalid_id",
				"path", path, "id", t.ID)
			continue
		}
		if !t.Kind.IsValid() {
			log.Warn("taskstore.replay.invalid_kind",
				"path", path, "id", t.ID, "kind", t.Kind)
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
