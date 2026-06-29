package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// TestEngine_SegmentLifecycle exercises the full segment path through the engine:
// flush → delete → merge → search. Verifies that the engine's public API stays
// correct across all segment operations.
func TestEngine_SegmentLifecycle(t *testing.T) {
	e := New()

	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "go runs everywhere"))
	e.Index(doc("3", "python is popular"))

	// force docs into a segment without needing 128 docs
	e.index.Flush()

	// index one more doc after flush — it stays in the buffer
	e.Index(doc("4", "go is great"))

	// delete a doc that lives in the segment (tombstone path)
	e.Delete("2")

	// merge: collapses segments, drops tombstones
	e.index.Merge()

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	found := map[string]bool{}
	for _, r := range results {
		found[r.ID] = true
	}

	if found["2"] {
		t.Error("doc '2' was deleted and should not appear after merge")
	}
	if !found["1"] {
		t.Error("doc '1' should be findable from the merged segment")
	}
	if !found["4"] {
		t.Error("doc '4' should be findable from the buffer")
	}
}

// doc builds a single-field Document for tests that don't need multi-field.
func doc(id, body string) Document {
	return Document{
		ID:     id,
		Fields: map[string]Field{"body": {Value: body, Boost: 1.0}},
	}
}

func TestEngine_Index_EmptyID(t *testing.T) {
	e := New()
	err := e.Index(Document{ID: ""})
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestEngine_Size(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is slow"))

	if e.Size() != 2 {
		t.Errorf("expected size 2, got %d", e.Size())
	}
}

func TestEngine_Search_Must(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is popular"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
}

func TestEngine_Search_TopK_Smaller_Than_Results(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "go is popular"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 1)

	if len(results) != 1 {
		t.Errorf("expected 1 result (topK=1), got %d", len(results))
	}
}

func TestEngine_Search_TopK_Larger_Than_Results(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestEngine_Search_Ranked_By_Score(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is a language"))
	e.Index(doc("2", "go go go"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "2" {
		t.Errorf("expected doc '2' (higher freq) to rank first, got %s", results[0].ID)
	}
	if results[0].Score <= results[1].Score {
		t.Error("results should be ordered by descending score")
	}
}

func TestEngine_Search_MustNot(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "go and python"))

	q := query.NewBuilder().Must("body", "go").MustNot("body", "python").Build()
	results := e.Search(q, 10)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
}

func TestEngine_Search_NoResults(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))

	q := query.NewBuilder().Must("body", "python").Build()
	results := e.Search(q, 10)

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestEngine_Delete(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "go is popular"))

	e.Delete("1")

	if e.Size() != 1 {
		t.Errorf("expected size 1 after delete, got %d", e.Size())
	}

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)
	for _, r := range results {
		if r.ID == "1" {
			t.Error("deleted doc '1' should not appear in results")
		}
	}
}

func TestEngine_Upsert_SizeUnchanged(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("1", "go is great"))

	if e.Size() != 1 {
		t.Errorf("expected size 1 after re-indexing same ID, got %d", e.Size())
	}
}

func TestEngine_Upsert_OldTermsGone(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("1", "python is great"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) != 0 {
		t.Errorf("old term 'go' should not match after upsert, got %d results", len(results))
	}
}

func TestEngine_Upsert_FrequencyReflectsNewDoc(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("1", "go go go"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	e2 := New()
	e2.Index(doc("1", "go go go"))
	results2 := e2.Search(q, 10)

	if len(results2) != 1 {
		t.Fatalf("expected 1 result from fresh engine, got %d", len(results2))
	}
	if results[0].Score != results2[0].Score {
		t.Errorf("upserted score %.4f != fresh index score %.4f",
			results[0].Score, results2[0].Score)
	}
}

func TestEngine_Result_Fields(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "go is fast", Boost: 1.0},
		},
	})

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) == 0 {
		t.Fatal("expected 1 result")
	}
	if results[0].Fields["body"].Value != "go is fast" {
		t.Errorf("expected body 'go is fast', got %q", results[0].Fields["body"].Value)
	}
}
