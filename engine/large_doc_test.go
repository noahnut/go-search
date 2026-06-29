package engine

import (
	"strings"
	"testing"

	"github.com/noahfan/go-search/query"
)

// largeEngine creates an engine with a tiny threshold so tests don't need huge strings.
func largeEngine() *Engine {
	return New(WithLargeDocThreshold(20)) // anything > 20 bytes goes to DocStore
}

// repeat builds a string long enough to exceed a given threshold.
func repeat(word string, n int) string {
	return strings.Repeat(word+" ", n)
}

// --- DocStore unit tests ---

func TestDocStore_PutAndReadChunk(t *testing.T) {
	e := largeEngine()
	content := repeat("golang", 10) // > 20 bytes → goes to DocStore

	err := e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: content, Boost: 1.0}},
	})
	if err != nil {
		t.Fatalf("Index returned error: %v", err)
	}

	chunks := e.dataStore.ChunksFor("1")
	if len(chunks) == 0 {
		t.Fatal("expected chunks in DocStore for large field")
	}

	text, err := e.dataStore.ReadChunk(chunks[0].ChunkID)
	if err != nil {
		t.Fatalf("ReadChunk returned error: %v", err)
	}
	if text == "" {
		t.Error("ReadChunk returned empty string")
	}
}

func TestDocStore_SmallFieldNotChunked(t *testing.T) {
	e := largeEngine()
	err := e.Index(doc("1", "short")) // "short" < 20 bytes → stays in e.docs
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	chunks := e.dataStore.ChunksFor("1")
	if len(chunks) != 0 {
		t.Errorf("small field should not be stored in DocStore, got %d chunk(s)", len(chunks))
	}

	if e.docs["1"].Fields["body"].Value != "short" {
		t.Error("small field should stay in e.docs")
	}
}

// --- Search transparency ---

func TestLargeDoc_SearchReturnsParentID(t *testing.T) {
	// Search must return the original doc ID, not a chunk ID.
	e := largeEngine()
	e.Index(Document{
		ID:     "doc-1",
		Fields: map[string]Field{"body": {Value: repeat("golang", 15), Boost: 1.0}},
	})

	q := query.NewBuilder().Must("body", "golang").Build()
	results := e.Search(q, 10)

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	for _, r := range results {
		if strings.Contains(r.ID, "chunk") {
			t.Errorf("Search returned a chunk ID %q instead of the parent doc ID", r.ID)
		}
		if r.ID != "doc-1" {
			t.Errorf("expected parent ID 'doc-1', got %q", r.ID)
		}
	}
}

func TestLargeDoc_ParentAppearsOnce(t *testing.T) {
	// If multiple chunks of the same parent match, the parent should appear exactly once.
	e := largeEngine()
	// "golang" appears many times → will be in multiple chunks
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: repeat("golang", 30), Boost: 1.0}},
	})

	q := query.NewBuilder().Must("body", "golang").Build()
	results := e.Search(q, 10)

	count := 0
	for _, r := range results {
		if r.ID == "1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("parent doc '1' should appear exactly once, appeared %d times", count)
	}
}

func TestLargeDoc_ThresholdRouting(t *testing.T) {
	// Field just above threshold goes to DocStore; field just below stays in e.docs.
	e := New(WithLargeDocThreshold(50))

	small := "hello world"                   // 11 bytes → e.docs
	large := repeat("golang", 10) // > 50 bytes → DocStore

	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: small, Boost: 1.0},
			"body":  {Value: large, Boost: 1.0},
		},
	})

	if e.docs["1"].Fields["title"].Value != small {
		t.Error("small field should remain in e.docs")
	}
	if _, hasBody := e.docs["1"].Fields["body"]; hasBody {
		t.Error("large field text should not be stored in e.docs")
	}
	if len(e.dataStore.ChunksFor("1")) == 0 {
		t.Error("large field should have chunks in DocStore")
	}
}

func TestLargeDoc_DeleteCleansDocStore(t *testing.T) {
	// Deleting a large doc must also remove its chunks from DocStore.
	e := largeEngine()
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: repeat("golang", 15), Boost: 1.0}},
	})

	if len(e.dataStore.ChunksFor("1")) == 0 {
		t.Fatal("precondition: doc should have chunks before delete")
	}

	e.Delete("1")

	if len(e.dataStore.ChunksFor("1")) != 0 {
		t.Error("Delete should remove all chunks from DocStore")
	}
}

func TestLargeDoc_NotDoubleIndexed(t *testing.T) {
	// A large doc should only be in the index as chunk IDs, not also as the parent doc ID.
	// If it's double-indexed, searching by parent would return duplicate results.
	e := largeEngine()
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: repeat("golang", 15), Boost: 1.0}},
	})

	q := query.NewBuilder().Must("body", "golang").Build()
	results := e.Search(q, 100)

	seen := map[string]int{}
	for _, r := range results {
		seen[r.ID]++
	}
	if seen["1"] > 1 {
		t.Errorf("doc '1' appears %d times — large doc is being double-indexed", seen["1"])
	}
}

func TestLargeDoc_SmallDocUnaffected(t *testing.T) {
	// Existing small-doc behaviour must be unchanged.
	e := largeEngine()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is dynamic"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) == 0 || results[0].ID != "1" {
		t.Error("small doc search should work exactly as before")
	}
}

func TestLargeDoc_MixedSmallAndLarge(t *testing.T) {
	// Both small and large docs should be searchable in the same engine.
	e := largeEngine()
	e.Index(doc("small", "go is fast"))
	e.Index(Document{
		ID:     "large",
		Fields: map[string]Field{"body": {Value: repeat("golang", 15), Boost: 1.0}},
	})

	q := query.NewBuilder().Should("body", "go").Should("body", "golang").Build()
	results := e.Search(q, 10)

	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	if !ids["small"] {
		t.Error("small doc should appear in results")
	}
	if !ids["large"] {
		t.Error("large doc should appear in results")
	}
}
