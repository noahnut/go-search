package engine

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/noahfan/go-search/query"
	"github.com/noahfan/go-search/storage/local"
)

// localEngine creates an engine with local storage at dir/docs.log.
// The store is closed automatically when the test ends.
func localEngine(t *testing.T, dir string) (*Engine, *local.Store) {
	t.Helper()
	store, err := local.New(filepath.Join(dir, "docs.log"))
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	return New(WithDocStorage(store)), store
}

// reopenEngine opens a new engine over the same local store written by a
// previous engine. The caller is responsible for closing the returned store.
func reopenEngine(t *testing.T, dir string, opts ...Option) (*Engine, *local.Store) {
	t.Helper()
	store, err := local.New(filepath.Join(dir, "docs.log"))
	if err != nil {
		t.Fatalf("local.New reopen: %v", err)
	}
	opts = append([]Option{WithDocStorage(store)}, opts...)
	return New(opts...), store
}

// --- Has ---

func TestHas_ExistingKey(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	if !e.docStorage.Has("1") {
		t.Error("Has should return true for indexed doc")
	}
}

func TestHas_MissingKey(t *testing.T) {
	e := New()
	if e.docStorage.Has("ghost") {
		t.Error("Has should return false for non-existent key")
	}
}

func TestHas_AfterDelete(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Delete("1")
	if e.docStorage.Has("1") {
		t.Error("Has should return false after Delete")
	}
}

// --- Size uses docStorage ---

func TestSize_UsesDocStorage(t *testing.T) {
	e := New()
	if e.Size() != 0 {
		t.Errorf("empty engine size: got %d, want 0", e.Size())
	}
	e.Index(doc("1", "go"))
	e.Index(doc("2", "python"))
	if e.Size() != 2 {
		t.Errorf("size after 2 index: got %d, want 2", e.Size())
	}
	e.Delete("1")
	if e.Size() != 1 {
		t.Errorf("size after delete: got %d, want 1", e.Size())
	}
}

// --- Startup recovery ---

func TestStartup_DocsSearchableAfterReopen(t *testing.T) {
	dir := t.TempDir()

	e, store := localEngine(t, dir)
	e.Index(doc("1", "go is compiled"))
	e.Index(doc("2", "python is interpreted"))
	store.Close()

	e2, store2 := reopenEngine(t, dir)
	defer store2.Close()

	q := query.NewBuilder().Must("body", "compiled").Build()
	results := e2.Search(q, 10).Hits
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("after reopen: expected doc '1', got %v", ids(results))
	}
}

func TestStartup_SizeRestoredAfterReopen(t *testing.T) {
	dir := t.TempDir()

	e, store := localEngine(t, dir)
	e.Index(doc("1", "go"))
	e.Index(doc("2", "rust"))
	e.Index(doc("3", "python"))
	store.Close()

	e2, store2 := reopenEngine(t, dir)
	defer store2.Close()

	if e2.Size() != 3 {
		t.Errorf("after reopen: expected size 3, got %d", e2.Size())
	}
}

func TestStartup_DeletedDocNotRestoredAfterReopen(t *testing.T) {
	dir := t.TempDir()

	e, store := localEngine(t, dir)
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is slow"))
	e.Delete("2")
	store.Close()

	e2, store2 := reopenEngine(t, dir)
	defer store2.Close()

	if e2.Size() != 1 {
		t.Errorf("deleted doc should not be restored: size=%d", e2.Size())
	}
	q := query.NewBuilder().Must("body", "python").Build()
	if results := e2.Search(q, 10).Hits; len(results) != 0 {
		t.Error("deleted doc should not be searchable after reopen")
	}
}

// --- Snapshot + delta recovery ---

func TestSnapshot_DeltaRecoveredAfterReopen(t *testing.T) {
	dir := t.TempDir()
	snapDir := t.TempDir()

	e, store := localEngine(t, dir)
	e2Opts := []Option{WithSnapshotDir(snapDir)}

	e = New(append([]Option{WithDocStorage(store)}, e2Opts...)...)
	e.Index(doc("1", "go is compiled"))
	e.Index(doc("2", "rust is safe"))

	// snapshot captures docs 1 and 2
	if err := e.Snapshot(); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// index doc 3 after the snapshot — this is the delta
	e.Index(doc("3", "python is dynamic"))
	store.Close()

	// reopen: snapshot restores 1+2, delta recovery re-indexes 3
	e3, store3 := reopenEngine(t, dir, WithSnapshotDir(snapDir))
	defer store3.Close()

	if e3.Size() != 3 {
		t.Errorf("expected 3 docs after recovery, got %d", e3.Size())
	}
	q := query.NewBuilder().Must("body", "dynamic").Build()
	results := e3.Search(q, 10).Hits
	if len(results) != 1 || results[0].ID != "3" {
		t.Errorf("delta doc '3' should be searchable after recovery, got %v", ids(results))
	}
}

func TestSnapshot_NoDuplicatesAfterRecovery(t *testing.T) {
	dir := t.TempDir()
	snapDir := t.TempDir()

	store, _ := local.New(filepath.Join(dir, "docs.log"))
	e := New(WithDocStorage(store), WithSnapshotDir(snapDir))
	e.Index(doc("1", "go go go"))
	e.Snapshot()
	store.Close()

	store2, _ := local.New(filepath.Join(dir, "docs.log"))
	defer store2.Close()
	e2 := New(WithDocStorage(store2), WithSnapshotDir(snapDir))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e2.Search(q, 10).Hits

	count := 0
	for _, r := range results {
		if r.ID == "1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("doc '1' should appear exactly once, got %d times", count)
	}
}

// --- Close ---

func TestClose_FinalSnapshotTaken(t *testing.T) {
	dir := t.TempDir()
	snapDir := t.TempDir()

	store, _ := local.New(filepath.Join(dir, "docs.log"))
	e := New(WithDocStorage(store), WithSnapshotDir(snapDir))
	e.Index(doc("1", "go is fast"))

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// snapshot file should exist
	snapPath := filepath.Join(snapDir, SnapshotFileName)
	store2, _ := local.New(filepath.Join(dir, "docs.log"))
	defer store2.Close()

	e2, err := Load(snapPath, WithDocStorage(store2))
	if err != nil {
		t.Fatalf("Load after Close: %v", err)
	}
	if e2.Size() != 1 {
		t.Errorf("expected 1 doc after Close+Load, got %d", e2.Size())
	}
}

// --- WithSnapshotInterval ---

func TestSnapshotInterval_FiresWithoutRace(t *testing.T) {
	dir := t.TempDir()
	snapDir := t.TempDir()

	store, _ := local.New(filepath.Join(dir, "docs.log"))
	e := New(
		WithDocStorage(store),
		WithSnapshotDir(snapDir),
		WithSnapshotInterval(50*time.Millisecond),
	)
	e.Index(doc("1", "go"))
	e.Index(doc("2", "rust"))

	// let the ticker fire a few times
	time.Sleep(200 * time.Millisecond)

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
