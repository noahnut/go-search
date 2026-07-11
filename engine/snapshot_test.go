package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noahfan/go-search/query"
	"github.com/noahfan/go-search/storage/local"
)

func tmpFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "index.gob")
}

func TestSaveAndLoad_Size(t *testing.T) {
	dir := t.TempDir()
	docsPath := filepath.Join(dir, "docs.log")

	store, _ := local.New(docsPath)
	e := New(WithDocStorage(store))
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is popular"))
	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Close()

	store2, err := local.New(docsPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	e2, err := Load(path, WithDocStorage(store2))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e2.Size() != 2 {
		t.Errorf("expected size 2 after load, got %d", e2.Size())
	}
}

func TestSaveAndLoad_SearchReturnsResults(t *testing.T) {
	dir := t.TempDir()
	docsPath := filepath.Join(dir, "docs.log")

	store, _ := local.New(docsPath)
	e := New(WithDocStorage(store))
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is popular"))
	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Close()

	store2, err := local.New(docsPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	e2, err := Load(path, WithDocStorage(store2))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	q := query.NewBuilder().Must("body", "go").Build()
	results := e2.Search(q, 10).Hits
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
}

func TestSaveAndLoad_ScoresMatch(t *testing.T) {
	dir := t.TempDir()
	docsPath := filepath.Join(dir, "docs.log")

	store, _ := local.New(docsPath)
	e := New(WithDocStorage(store))
	e.Index(doc("1", "go go go"))
	e.Index(doc("2", "go is fast"))

	q := query.NewBuilder().Must("body", "go").Build()
	original := e.Search(q, 10).Hits

	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Close()

	store2, err := local.New(docsPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	e2, err := Load(path, WithDocStorage(store2))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	loaded := e2.Search(q, 10).Hits

	if len(original) != len(loaded) {
		t.Fatalf("result count mismatch: original=%d loaded=%d", len(original), len(loaded))
	}
	for i := range original {
		if original[i].ID != loaded[i].ID {
			t.Errorf("result[%d] ID: original=%s loaded=%s", i, original[i].ID, loaded[i].ID)
		}
		if original[i].Score != loaded[i].Score {
			t.Errorf("result[%d] score: original=%.6f loaded=%.6f", i, original[i].Score, loaded[i].Score)
		}
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/index.gob")
	if err == nil {
		t.Error("expected error loading non-existent file")
	}
}

func TestSaveAndLoad_OriginalGone(t *testing.T) {
	dir := t.TempDir()
	docsPath := filepath.Join(dir, "docs.log")
	path := tmpFile(t)

	func() {
		store, _ := local.New(docsPath)
		defer store.Close()
		e := New(WithDocStorage(store))
		e.Index(doc("1", "go is fast"))
		if err := e.Save(path); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}()

	store2, err := local.New(docsPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	e2, err := Load(path, WithDocStorage(store2))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e2.Size() != 1 {
		t.Errorf("expected size 1, got %d", e2.Size())
	}
}

func TestSaveAndLoad_IndexAfterLoad(t *testing.T) {
	dir := t.TempDir()
	docsPath := filepath.Join(dir, "docs.log")

	store, _ := local.New(docsPath)
	e := New(WithDocStorage(store))
	e.Index(doc("1", "go is fast"))
	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Close()

	store2, err := local.New(docsPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	e2, err := Load(path, WithDocStorage(store2))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if err := e2.Index(doc("2", "rust is safe")); err != nil {
		t.Fatalf("Index after load: %v", err)
	}
	if e2.Size() != 2 {
		t.Errorf("expected size 2, got %d", e2.Size())
	}
}

func TestSave_CreatesFile(t *testing.T) {
	e := New()
	e.Index(doc("1", "hello"))

	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("saved file is empty")
	}
}
