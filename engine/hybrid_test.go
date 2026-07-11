package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// hybridDoc builds a document with both a text body and a vector embedding.
func hybridDoc(id, body string, vector []float64) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"body":      {Value: body, Boost: 1.0},
			"embedding": {Value: "", Boost: 1.0, Vector: vector},
		},
	}
}

func TestHybridSearch_PureBM25(t *testing.T) {
	// alpha=1.0 → result order must match plain Search
	e := New()
	e.Index(hybridDoc("1", "go go go", []float64{1, 0}))
	e.Index(hybridDoc("2", "go is fast", []float64{0, 1}))

	q := query.NewBuilder().Must("body", "go").Build()

	bm25 := e.Search(q, 10).Hits
	hybrid := e.HybridSearch(q, "embedding", []float64{1, 0}, 1.0, 10)

	if len(hybrid) != len(bm25) {
		t.Fatalf("expected %d results, got %d", len(bm25), len(hybrid))
	}
	for i := range bm25 {
		if bm25[i].ID != hybrid[i].ID {
			t.Errorf("result[%d]: BM25=%s hybrid=%s — ordering differs at alpha=1.0", i, bm25[i].ID, hybrid[i].ID)
		}
	}
}

func TestHybridSearch_PureVector(t *testing.T) {
	// alpha=0.0 → result order must match plain VectorSearch
	e := New()
	e.Index(hybridDoc("1", "go is fast", []float64{1, 0}))
	e.Index(hybridDoc("2", "go runs", []float64{0, 1}))

	q := query.NewBuilder().Must("body", "go").Build()
	queryVec := []float64{1, 0}

	vec := e.VectorSearch("embedding", queryVec, 10)
	hybrid := e.HybridSearch(q, "embedding", queryVec, 0.0, 10)

	if len(hybrid) == 0 {
		t.Fatal("expected results at alpha=0.0")
	}
	if hybrid[0].ID != vec[0].ID {
		t.Errorf("top result: vector=%s hybrid=%s — ordering differs at alpha=0.0", vec[0].ID, hybrid[0].ID)
	}
}

func TestHybridSearch_VectorOnlyDocIncluded(t *testing.T) {
	// doc "2" has no keyword match but has a strong vector match.
	// With alpha < 1.0, it should appear in hybrid results.
	e := New()
	e.Index(hybridDoc("1", "go is fast", []float64{0, 1})) // keyword match, weak vector
	e.Index(hybridDoc("2", "python is great", []float64{1, 0})) // no keyword match, strong vector

	q := query.NewBuilder().Must("body", "go").Build()
	queryVec := []float64{1, 0} // points toward doc2's vector

	results := e.HybridSearch(q, "embedding", queryVec, 0.5, 10)

	found := false
	for _, r := range results {
		if r.ID == "2" {
			found = true
		}
	}
	if !found {
		t.Error("doc '2' has a strong vector match and should appear in hybrid results at alpha=0.5")
	}
}

func TestHybridSearch_BM25OnlyDocIncluded(t *testing.T) {
	// doc "1" has no vector but matches the keyword query.
	// With alpha > 0.0, it should appear in hybrid results.
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "go is fast", Boost: 1.0},
			// no embedding field
		},
	})
	e.Index(hybridDoc("2", "python", []float64{1, 0}))

	q := query.NewBuilder().Must("body", "go").Build()

	results := e.HybridSearch(q, "embedding", []float64{1, 0}, 0.5, 10)

	found := false
	for _, r := range results {
		if r.ID == "1" {
			found = true
		}
	}
	if !found {
		t.Error("doc '1' has a keyword match and should appear in hybrid results at alpha=0.5")
	}
}

func TestHybridSearch_TopK(t *testing.T) {
	e := New()
	for i := 1; i <= 5; i++ {
		e.Index(hybridDoc(
			string(rune('0'+i)),
			"go is fast",
			[]float64{1, 0},
		))
	}

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.HybridSearch(q, "embedding", []float64{1, 0}, 0.5, 3)

	if len(results) != 3 {
		t.Errorf("expected topK=3 results, got %d", len(results))
	}
}

func TestHybridSearch_ScoresDescending(t *testing.T) {
	e := New()
	e.Index(hybridDoc("1", "go go go", []float64{1, 0}))
	e.Index(hybridDoc("2", "go is fast", []float64{0.9, 0.1}))
	e.Index(hybridDoc("3", "python rocks", []float64{0.1, 0.9}))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.HybridSearch(q, "embedding", []float64{1, 0}, 0.5, 10)

	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: result[%d].Score=%.4f > result[%d].Score=%.4f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestHybridSearch_NormalizationPreventsBM25Dominance(t *testing.T) {
	// Without normalization, BM25 (unbounded) dominates vector (0–1) even at alpha=0.5.
	// With normalization, alpha=0.0 must produce a different ranking than alpha=1.0
	// when the vector signal disagrees with BM25.
	//
	// doc "1": high BM25 (repeated term), weak vector match
	// doc "2": low BM25 (term appears once), strong vector match
	e := New()
	e.Index(hybridDoc("1", "go go go go go", []float64{0, 1})) // high BM25, weak vector
	e.Index(hybridDoc("2", "go is fast", []float64{1, 0}))     // low BM25, strong vector

	q := query.NewBuilder().Must("body", "go").Build()
	queryVec := []float64{1, 0}

	bm25Results := e.Search(q, 10).Hits
	hybridResults := e.HybridSearch(q, "embedding", queryVec, 0.5, 10)

	if len(bm25Results) < 2 || len(hybridResults) < 2 {
		t.Fatal("expected at least 2 results")
	}

	// BM25 alone: doc1 ranks first (more "go" repetitions)
	if bm25Results[0].ID != "1" {
		t.Errorf("BM25: expected doc '1' first, got %s", bm25Results[0].ID)
	}

	// Hybrid at alpha=0.5 with normalized scores: vector signal should lift doc2
	if hybridResults[0].ID != "2" {
		t.Errorf("hybrid at alpha=0.5: expected doc '2' (strong vector) first, got %s — BM25 normalization may be missing", hybridResults[0].ID)
	}
}
