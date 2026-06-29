package engine

import (
	"testing"
)

func TestPrefixSearch_BasicMatch(t *testing.T) {
	e := New()
	e.Index(doc("1", "golang is fast"))
	e.Index(doc("2", "python is popular"))

	results := e.PrefixSearch("body", "gol")

	if len(results) == 0 {
		t.Fatal("expected at least one result for prefix 'gol'")
	}
	found := false
	for _, r := range results {
		if r.ID == "1" {
			found = true
		}
	}
	if !found {
		t.Error("doc '1' contains 'golang' and should match prefix 'gol'")
	}
}

func TestPrefixSearch_MultipleTermsMatch(t *testing.T) {
	// "gold" and "golang" both match prefix "gol" — both docs should appear
	e := New()
	e.Index(doc("1", "golang is great"))
	e.Index(doc("2", "gold is valuable"))
	e.Index(doc("3", "python is different"))

	results := e.PrefixSearch("body", "gol")

	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	if !ids["1"] {
		t.Error("doc '1' (golang) should match prefix 'gol'")
	}
	if !ids["2"] {
		t.Error("doc '2' (gold) should match prefix 'gol'")
	}
	if ids["3"] {
		t.Error("doc '3' (python) should not match prefix 'gol'")
	}
}

func TestPrefixSearch_NoMatch(t *testing.T) {
	e := New()
	e.Index(doc("1", "golang is fast"))

	results := e.PrefixSearch("body", "xyz")
	if len(results) != 0 {
		t.Errorf("expected no results for prefix 'xyz', got %d", len(results))
	}
}

func TestPrefixSearch_FieldScoped(t *testing.T) {
	// "golang" only in "title", not in "body" — PrefixSearch("body", "gol") should return nothing
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: "golang tutorial", Boost: 1.0},
			"body":  {Value: "learn programming", Boost: 1.0},
		},
	})

	results := e.PrefixSearch("body", "gol")
	if len(results) != 0 {
		t.Errorf("prefix 'gol' in 'body' should not match a term only in 'title', got %v", results)
	}

	titleResults := e.PrefixSearch("title", "gol")
	if len(titleResults) == 0 {
		t.Error("prefix 'gol' in 'title' should match doc '1'")
	}
}

func TestPrefixSearch_NoDuplicateDocIDs(t *testing.T) {
	// doc has multiple terms matching the same prefix — it should appear once, not twice
	e := New()
	e.Index(doc("1", "gold golden"))

	results := e.PrefixSearch("body", "gold")
	count := 0
	for _, r := range results {
		if r.ID == "1" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("doc '1' appeared %d times — expected at most once per doc", count)
	}
}

func TestPrefixSearch_EmptyPrefixReturnsAll(t *testing.T) {
	e := New()
	e.Index(doc("1", "golang"))
	e.Index(doc("2", "python"))
	e.Index(doc("3", "rust"))

	results := e.PrefixSearch("body", "")
	if len(results) < 3 {
		t.Errorf("empty prefix should return all docs, got %d result(s)", len(results))
	}
}

func TestPrefixSearch_AfterFlush(t *testing.T) {
	// Terms flushed into segments must still be reachable via prefix search
	e := New()
	e.Index(doc("1", "golang is compiled"))
	e.index.Flush()
	e.Index(doc("2", "golangci runs checks")) // stays in buffer

	results := e.PrefixSearch("body", "golang")

	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	if !ids["1"] {
		t.Error("doc '1' was flushed to segment but should still appear in prefix search")
	}
	if !ids["2"] {
		t.Error("doc '2' is in buffer and should appear in prefix search")
	}
}

func TestPrefixSearch_Race(t *testing.T) {
	e := New()
	for i := 0; i < 20; i++ {
		e.Index(doc(string(rune('a'+i)), "golang"))
	}

	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func() {
			e.PrefixSearch("body", "go")
			done <- struct{}{}
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
}
