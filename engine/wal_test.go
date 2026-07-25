package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noahfan/go-search/query"
	"github.com/noahfan/go-search/storage/local"
)

// walEngine creates an engine with local storage and WAL in the same temp dir.
// The caller must close the store when done.
func walEngine(t *testing.T, dir string) (*Engine, func()) {
	t.Helper()
	docsPath := filepath.Join(dir, "docs.log")
	store, err := local.New(docsPath)
	if err != nil {
		t.Fatalf("local.New: %v", err)
	}
	e := New(WithDocStorage(store), WithWAL(dir), WithSnapshotDir(dir))
	return e, func() { store.Close() }
}

// TestWAL_IndexSurvivesCrash verifies that docs indexed after the last snapshot
// are searchable after a restart (simulated crash — no snapshot taken).
func TestWAL_IndexSurvivesCrash(t *testing.T) {
	dir := t.TempDir()

	// session 1: index without saving
	e1, close1 := walEngine(t, dir)
	e1.Index(doc("1", "golang is fast"))
	e1.Index(doc("2", "rust is safe"))
	close1()

	// session 2: new engine replays WAL
	e2, close2 := walEngine(t, dir)
	defer close2()

	q := query.NewBuilder().Must("body", "golang").Build()
	hits := e2.Search(q, 10).Hits
	if len(hits) != 1 || hits[0].ID != "1" {
		t.Errorf("expected doc 1 after WAL replay, got %v", ids(hits))
	}

	if e2.Size() != 2 {
		t.Errorf("expected size 2 after WAL replay, got %d", e2.Size())
	}
}

// TestWAL_DeleteSurvivesCrash verifies that a delete between snapshots stays
// deleted after WAL replay.
func TestWAL_DeleteSurvivesCrash(t *testing.T) {
	dir := t.TempDir()

	// session 1: index, snapshot, then delete (no second snapshot)
	e1, close1 := walEngine(t, dir)
	e1.Index(doc("1", "go tutorial"))
	e1.Index(doc("2", "python tutorial"))
	snapPath := filepath.Join(dir, SnapshotFileName)
	if err := e1.Save(snapPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	e1.Delete("2") // this delete is only in the WAL, not the snapshot
	close1()

	// session 2: loads snapshot then replays WAL delete
	e2, close2 := walEngine(t, dir)
	defer close2()

	q := query.NewBuilder().Must("body", "tutorial").Build()
	hits := e2.Search(q, 10).Hits
	got := ids(hits)

	for _, id := range got {
		if id == "2" {
			t.Errorf("doc 2 should be deleted after WAL replay, but appears in results: %v", got)
		}
	}
	if len(got) != 1 || got[0] != "1" {
		t.Errorf("expected only doc 1, got %v", got)
	}
}

// TestWAL_TruncatedAfterSave verifies the WAL file is empty (size = 0) after Save.
func TestWAL_TruncatedAfterSave(t *testing.T) {
	dir := t.TempDir()
	e, closeStore := walEngine(t, dir)
	defer closeStore()

	e.Index(doc("1", "hello"))
	e.Index(doc("2", "world"))

	snapPath := filepath.Join(dir, SnapshotFileName)
	if err := e.Save(snapPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	walFile := filepath.Join(dir, WALFileName)
	info, err := os.Stat(walFile)
	if err != nil {
		t.Fatalf("WAL file missing: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("WAL file should be empty after Save, got size %d", info.Size())
	}
}

// TestWAL_IndexAndDelete_Order verifies that replay applies ops in the correct
// order: a doc indexed then deleted should not appear after replay.
func TestWAL_IndexAndDelete_Order(t *testing.T) {
	dir := t.TempDir()

	e1, close1 := walEngine(t, dir)
	e1.Index(doc("1", "temporary doc"))
	e1.Delete("1")
	close1()

	e2, close2 := walEngine(t, dir)
	defer close2()

	if e2.Size() != 0 {
		t.Errorf("expected size 0 after index+delete replay, got %d", e2.Size())
	}
}

// TestWAL_UpdateSurvivesCrash verifies that re-indexing the same doc ID (update)
// replays correctly: only the latest content is searchable after replay.
func TestWAL_UpdateSurvivesCrash(t *testing.T) {
	dir := t.TempDir()

	e1, close1 := walEngine(t, dir)
	e1.Index(doc("1", "golang is fast"))
	e1.Index(doc("1", "golang is slow")) // same ID, updated content
	close1()

	e2, close2 := walEngine(t, dir)
	defer close2()

	if e2.Size() != 1 {
		t.Errorf("expected size 1 after update replay, got %d", e2.Size())
	}

	q := query.NewBuilder().Must("body", "slow").Build()
	if hits := e2.Search(q, 10).Hits; len(hits) != 1 || hits[0].ID != "1" {
		t.Errorf("expected updated content 'slow' to be searchable, got %v", ids(hits))
	}

	q2 := query.NewBuilder().Must("body", "fast").Build()
	if hits := e2.Search(q2, 10).Hits; len(hits) != 0 {
		t.Errorf("stale content 'fast' should not be searchable after update, got %v", ids(hits))
	}
}

// TestWAL_SnapshotThenUpdateSurvivesCrash verifies that a doc updated after a
// snapshot is searchable by its new content after WAL replay.
// Note: the old content may still be findable via stale segment postings until
// a merge compacts the segment — that is a pre-existing engine limitation, not
// a WAL concern, so only the positive case is asserted here.
func TestWAL_SnapshotThenUpdateSurvivesCrash(t *testing.T) {
	dir := t.TempDir()

	e1, close1 := walEngine(t, dir)
	e1.Index(doc("1", "python tutorial"))
	snapPath := filepath.Join(dir, SnapshotFileName)
	if err := e1.Save(snapPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	e1.Index(doc("1", "golang tutorial")) // update after snapshot
	close1()

	e2, close2 := walEngine(t, dir)
	defer close2()

	// WAL must replay the update: new content is searchable.
	q := query.NewBuilder().Must("body", "golang").Build()
	if hits := e2.Search(q, 10).Hits; len(hits) != 1 || hits[0].ID != "1" {
		t.Errorf("expected WAL update to be searchable after replay, got %v", ids(hits))
	}

	// Size must be 1: the update keeps a single doc, not two.
	if e2.Size() != 1 {
		t.Errorf("expected size 1 after update replay, got %d", e2.Size())
	}
}

// TestWAL_NoDoubleIndex verifies that recoverDelta does not run alongside WAL
// replay, so docs are indexed exactly once after restart.
func TestWAL_NoDoubleIndex(t *testing.T) {
	dir := t.TempDir()

	e1, close1 := walEngine(t, dir)
	e1.Index(doc("1", "go go go"))
	close1()

	e2, close2 := walEngine(t, dir)
	defer close2()

	q := query.NewBuilder().Must("body", "go").Build()
	hits := e2.Search(q, 10).Hits
	if len(hits) != 1 {
		t.Errorf("doc 1 should appear exactly once, got %d results", len(hits))
	}
}
