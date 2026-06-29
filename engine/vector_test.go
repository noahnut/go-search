package engine

import (
	"math"
	"testing"
)

// --- CosineSimilarity unit tests ---

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float64{1, 0, 0}
	if got := CosineSimilarity(a, a); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("identical vectors: expected 1.0, got %.6f", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float64{1, 0}
	b := []float64{0, 1}
	if got := CosineSimilarity(a, b); got != 0.0 {
		t.Errorf("orthogonal vectors: expected 0.0, got %.6f", got)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float64{1, 0}
	b := []float64{-1, 0}
	if got := CosineSimilarity(a, b); math.Abs(got+1.0) > 1e-9 {
		t.Errorf("opposite vectors: expected -1.0, got %.6f", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float64{0, 0, 0}
	b := []float64{1, 2, 3}
	if got := CosineSimilarity(a, b); got != 0.0 {
		t.Errorf("zero vector: expected 0.0, got %.6f", got)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	a := []float64{1, 2}
	b := []float64{1, 2, 3}
	if got := CosineSimilarity(a, b); got != 0.0 {
		t.Errorf("mismatched lengths: expected 0.0, got %.6f", got)
	}
}

func TestCosineSimilarity_ScaledVector(t *testing.T) {
	// magnitude doesn't matter — only direction
	a := []float64{1, 0}
	b := []float64{5, 0}
	if got := CosineSimilarity(a, b); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("scaled same-direction vectors: expected 1.0, got %.6f", got)
	}
}

// --- VectorSearch integration tests ---

func vecDoc(id string, vector []float64) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"embedding": {Value: "", Boost: 1.0, Vector: vector},
		},
	}
}

func TestVectorSearch_RankedBySimilarity(t *testing.T) {
	e := New()

	// doc1 vector points in same direction as query
	e.Index(vecDoc("1", []float64{1, 0, 0}))
	// doc2 vector is at an angle
	e.Index(vecDoc("2", []float64{1, 1, 0}))

	query := []float64{1, 0, 0}
	results := e.VectorSearch("embedding", query, 10)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1' (closest) to rank first, got %s", results[0].ID)
	}
	if results[0].Score <= results[1].Score {
		t.Error("results should be ordered by descending similarity")
	}
}

func TestVectorSearch_ExactMatch(t *testing.T) {
	e := New()
	e.Index(vecDoc("1", []float64{1, 0, 0}))
	e.Index(vecDoc("2", []float64{0, 1, 0}))

	results := e.VectorSearch("embedding", []float64{1, 0, 0}, 10)

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
	if math.Abs(results[0].Score-1.0) > 1e-9 {
		t.Errorf("expected score 1.0, got %.6f", results[0].Score)
	}
}

func TestVectorSearch_NoVectorField(t *testing.T) {
	e := New()
	// doc has no vector
	e.Index(doc("1", "go is fast"))

	results := e.VectorSearch("embedding", []float64{1, 0, 0}, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for doc with no vector, got %d", len(results))
	}
}

func TestVectorSearch_WrongField(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title":     {Value: "go", Boost: 1.0, Vector: []float64{1, 0, 0}},
			"embedding": {Value: "", Boost: 1.0, Vector: []float64{0, 1, 0}},
		},
	})

	// searching "title" field vector, but query points along embedding direction
	results := e.VectorSearch("title", []float64{0, 1, 0}, 10)
	// title vector is {1,0,0}, query is {0,1,0} — orthogonal, score=0, excluded
	for _, r := range results {
		if r.ID == "1" {
			t.Error("doc should not match: title vector is orthogonal to query")
		}
	}
}

func TestVectorSearch_TopK(t *testing.T) {
	e := New()
	e.Index(vecDoc("1", []float64{1, 0, 0}))
	e.Index(vecDoc("2", []float64{1, 0.1, 0}))
	e.Index(vecDoc("3", []float64{1, 0.2, 0}))

	results := e.VectorSearch("embedding", []float64{1, 0, 0}, 2)
	if len(results) != 2 {
		t.Errorf("expected topK=2 results, got %d", len(results))
	}
}

func TestVectorSearch_ResultPreservesFields(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body":      {Value: "go is fast", Boost: 1.0},
			"embedding": {Value: "", Boost: 1.0, Vector: []float64{1, 0, 0}},
		},
	})

	results := e.VectorSearch("embedding", []float64{1, 0, 0}, 10)
	if len(results) == 0 {
		t.Fatal("expected 1 result")
	}
	if results[0].Fields["body"].Value != "go is fast" {
		t.Errorf("body field not preserved: got %q", results[0].Fields["body"].Value)
	}
}

func TestVectorSearch_MixedDocsWithAndWithoutVectors(t *testing.T) {
	e := New()
	e.Index(vecDoc("1", []float64{1, 0, 0}))
	e.Index(doc("2", "no vector here"))    // no vector
	e.Index(vecDoc("3", []float64{1, 0, 0}))

	results := e.VectorSearch("embedding", []float64{1, 0, 0}, 10)
	if len(results) != 2 {
		t.Errorf("expected 2 results (only docs with vectors), got %d", len(results))
	}
	for _, r := range results {
		if r.ID == "2" {
			t.Error("doc '2' has no vector and should not appear in results")
		}
	}
}
