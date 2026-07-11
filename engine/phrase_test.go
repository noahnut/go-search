package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

func TestEngine_PhraseSearch_Matches(t *testing.T) {
	e := New()
	e.Index(doc("1", "I love New York city"))
	e.Index(doc("2", "New York is great"))
	e.Index(doc("3", "York is new to me"))

	q := query.NewBuilder().Phrase("body", "new", "york").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 2 {
		t.Fatalf("expected 2 results for phrase 'new york', got %d", len(results))
	}
	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	if !ids["1"] || !ids["2"] {
		t.Errorf("expected docs 1 and 2, got %v", ids)
	}
	if ids["3"] {
		t.Error("doc 3 should not match — 'york' appears before 'new'")
	}
}

func TestEngine_PhraseSearch_NoMatch(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))

	q := query.NewBuilder().Phrase("body", "fast", "go").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 0 {
		t.Errorf("expected 0 results for wrong-order phrase, got %d", len(results))
	}
}

func TestEngine_PhraseSearch_NotConsecutive(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is very fast"))

	q := query.NewBuilder().Phrase("body", "go", "fast").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 0 {
		t.Errorf("expected 0 results: 'go fast' not consecutive, got %d", len(results))
	}
}

func TestEngine_PhraseSearch_ThreeWords(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "go fast is"))

	q := query.NewBuilder().Phrase("body", "go", "is", "fast").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
}

func TestEngine_PhraseSearch_RepeatedTerm(t *testing.T) {
	e := New()
	e.Index(doc("1", "go go go"))

	q := query.NewBuilder().Phrase("body", "go", "go").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 1 {
		t.Errorf("expected 1 result for 'go go' in 'go go go', got %d", len(results))
	}
}
