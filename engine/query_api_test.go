package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// Task 19: verify callers never need to write "field:term" — the engine handles it.

func TestQueryAPI_FieldAndTermAreSeparate(t *testing.T) {
	q := query.NewBuilder().Must("title", "golang").Build()

	if len(q.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(q.Clauses))
	}
	c := q.Clauses[0]
	if c.Field != "title" {
		t.Errorf("Field should be 'title', got %q", c.Field)
	}
	if c.Term != "golang" {
		t.Errorf("Term should be 'golang', got %q", c.Term)
	}
}

func TestQueryAPI_EngineHandlesFieldPrefix(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is popular"))

	// caller writes Must("body", "go") — never "body:go"
	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("expected doc '1', got %v", results)
	}
}

func TestQueryAPI_MultiField_NoPrefixInCallSite(t *testing.T) {
	e := New()
	e.Index(document{
		"1",
		map[string]Field{
			"title": {Value: "golang concurrency", Boost: 1.0},
			"body":  {Value: "goroutines and channels", Boost: 1.0},
		},
	})

	// two different fields, no "field:term" strings visible to caller
	q := query.NewBuilder().
		Must("title", "golang").
		Must("body", "goroutines").
		Build()

	results := e.Search(q, 10).Hits
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("expected doc '1', got %v", results)
	}
}

func TestQueryAPI_WrongField_NoResults(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast")) // "go" is in "body"

	// searching the wrong field should return nothing
	q := query.NewBuilder().Must("title", "go").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 0 {
		t.Errorf("expected 0 results: 'go' is in body, not title; got %d", len(results))
	}
}

func TestQueryAPI_MustNot_TwoArgs(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "go and python"))

	q := query.NewBuilder().
		Must("body", "go").
		MustNot("body", "python").
		Build()

	results := e.Search(q, 10).Hits
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("expected only doc '1', got %v", results)
	}
}

func TestQueryAPI_Should_TwoArgs(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "rust is safe"))
	e.Index(doc("3", "python is popular"))

	// should(go) or should(rust) — "python" doc should not match
	q := query.NewBuilder().
		Should("body", "go").
		Should("body", "rust").
		Build()

	results := e.Search(q, 10).Hits
	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	if ids["3"] {
		t.Error("doc '3' (python) should not match should(go)|should(rust)")
	}
	if !ids["1"] || !ids["2"] {
		t.Error("docs '1' and '2' should match")
	}
}

// document is a convenience alias so tests don't repeat map[string]Field.
type document = Document
