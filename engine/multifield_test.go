package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

func TestMultiField_SearchByField(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: "Go Language", Boost: 1.0},
			"body":  {Value: "Python is popular", Boost: 1.0},
		},
	})
	e.Index(Document{
		ID: "2",
		Fields: map[string]Field{
			"title": {Value: "Python Language", Boost: 1.0},
			"body":  {Value: "Go is fast", Boost: 1.0},
		},
	})

	// "go" only in title of doc1
	q := query.NewBuilder().Must("title", "go").Build()
	results := e.Search(q, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for title:go, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}

	// "go" only in body of doc2
	q2 := query.NewBuilder().Must("body", "go").Build()
	results2 := e.Search(q2, 10)
	if len(results2) != 1 {
		t.Fatalf("expected 1 result for body:go, got %d", len(results2))
	}
	if results2[0].ID != "2" {
		t.Errorf("expected doc '2', got %s", results2[0].ID)
	}
}

func TestMultiField_BoostRanking(t *testing.T) {
	e := New()
	// doc1: "go" in title (high boost)
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: "go language", Boost: 3.0},
			"body":  {Value: "programming", Boost: 1.0},
		},
	})
	// doc2: "go" in body (low boost)
	e.Index(Document{
		ID: "2",
		Fields: map[string]Field{
			"title": {Value: "programming language", Boost: 3.0},
			"body":  {Value: "go is fast", Boost: 1.0},
		},
	})

	// Both docs have "go", but in different fields with different boosts
	q := query.NewBuilder().Should("title", "go").Should("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// doc1 title match (boost=3) should outscore doc2 body match (boost=1)
	if results[0].ID != "1" {
		t.Errorf("expected doc '1' (title boost=3) to rank first, got %s", results[0].ID)
	}
	if results[0].Score <= results[1].Score {
		t.Error("title match with boost=3 should score higher than body match with boost=1")
	}
}

func TestMultiField_MustAcrossFields(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: "go language", Boost: 1.0},
			"body":  {Value: "fast and simple", Boost: 1.0},
		},
	})
	e.Index(Document{
		ID: "2",
		Fields: map[string]Field{
			"title": {Value: "go language", Boost: 1.0},
			"body":  {Value: "slow but powerful", Boost: 1.0},
		},
	})

	// must have "go" in title AND "fast" in body
	q := query.NewBuilder().Must("title", "go").Must("body", "fast").Build()
	results := e.Search(q, 10)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
}

func TestMultiField_FieldNotPresent(t *testing.T) {
	e := New()
	// doc has only body, no title
	e.Index(doc("1", "go is fast"))

	q := query.NewBuilder().Must("title", "go").Build()
	results := e.Search(q, 10)

	if len(results) != 0 {
		t.Errorf("expected 0 results: doc has no title field, got %d", len(results))
	}
}

func TestMultiField_ResultPreservesFields(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: "Go Language", Boost: 2.0},
			"body":  {Value: "Go is fast", Boost: 1.0},
		},
	})

	q := query.NewBuilder().Must("title", "go").Build()
	results := e.Search(q, 10)

	if len(results) == 0 {
		t.Fatal("expected 1 result")
	}
	r := results[0]
	if r.Fields["title"].Value != "Go Language" {
		t.Errorf("title value not preserved: got %q", r.Fields["title"].Value)
	}
	if r.Fields["body"].Value != "Go is fast" {
		t.Errorf("body value not preserved: got %q", r.Fields["body"].Value)
	}
}
