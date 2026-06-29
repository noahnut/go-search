package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noahfan/go-search/query"
)

func tmpFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "index.gob")
}

func TestSaveAndLoad_Size(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is popular"))

	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	e2, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e2.Size() != 2 {
		t.Errorf("expected size 2 after load, got %d", e2.Size())
	}
}

func TestSaveAndLoad_SearchReturnsResults(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is popular"))

	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	e2, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	q := query.NewBuilder().Must("body", "go").Build()
	results := e2.Search(q, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
}

func TestSaveAndLoad_ScoresMatch(t *testing.T) {
	e := New()
	e.Index(doc("1", "go go go"))
	e.Index(doc("2", "go is fast"))

	q := query.NewBuilder().Must("body", "go").Build()
	original := e.Search(q, 10)

	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	e2, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	loaded := e2.Search(q, 10)

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
	path := tmpFile(t)

	func() {
		e := New()
		e.Index(doc("1", "go is fast"))
		if err := e.Save(path); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}()

	e2, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if e2.Size() != 1 {
		t.Errorf("expected size 1, got %d", e2.Size())
	}
}

func TestSaveAndLoad_IndexAfterLoad(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))

	path := tmpFile(t)
	if err := e.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	e2, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	err = e2.Index(doc("2", "rust is safe"))
	if err != nil {
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
