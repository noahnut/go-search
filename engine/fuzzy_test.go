package engine

import "testing"

func TestFuzzySearch_ExactMatch(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is popular"))

	results := e.FuzzySearch("body", "go", 0, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for exact match, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
}

func TestFuzzySearch_Typo(t *testing.T) {
	// "goo" is distance 1 from "go" — should match
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "python is popular"))

	results := e.FuzzySearch("body", "goo", 1, 10)
	if len(results) == 0 {
		t.Fatal("expected results for fuzzy match 'goo'→'go', got none")
	}
	found := false
	for _, r := range results {
		if r.ID == "1" {
			found = true
		}
	}
	if !found {
		t.Error("expected doc '1' to match fuzzy query 'goo' (distance 1 from 'go')")
	}
}

func TestFuzzySearch_NoMatch(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))

	// "xyz" is far from every term in the index
	results := e.FuzzySearch("body", "xyz", 1, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestFuzzySearch_WrongField(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: "go language", Boost: 1.0},
			"body":  {Value: "python is fast", Boost: 1.0},
		},
	})

	// "go" is in title, not body — fuzzy search on body should not find it
	results := e.FuzzySearch("body", "go", 1, 10)
	for _, r := range results {
		if r.ID == "1" {
			t.Error("should not match: 'go' is in title field, not body")
		}
	}
}

func TestFuzzySearch_RankedByScore(t *testing.T) {
	e := New()
	// doc1: "go" appears 3 times — should score higher
	e.Index(doc("1", "go go go"))
	// doc2: "go" appears once
	e.Index(doc("2", "go is fast"))

	// exact match, distance 0
	results := e.FuzzySearch("body", "go", 0, 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1' (higher freq) to rank first, got %s", results[0].ID)
	}
	if results[0].Score <= results[1].Score {
		t.Error("results should be ordered by descending score")
	}
}

func TestFuzzySearch_TopK(t *testing.T) {
	e := New()
	e.Index(doc("1", "go is fast"))
	e.Index(doc("2", "go is popular"))
	e.Index(doc("3", "go runs everywhere"))

	results := e.FuzzySearch("body", "go", 0, 2)
	if len(results) != 2 {
		t.Errorf("expected topK=2 results, got %d", len(results))
	}
}
