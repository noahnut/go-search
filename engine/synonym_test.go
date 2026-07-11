package engine

import (
	"testing"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/query"
)

func synonymEngine() *Engine {
	return New(WithSynonyms(analysis.NewSynonymMap(map[string][]string{
		"car":        {"automobile", "vehicle"},
		"automobile": {"car", "vehicle"},
		"vehicle":    {"car", "automobile"},
	})))
}

func TestSynonym_BasicExpansion(t *testing.T) {
	// Searching "car" should find a doc that only contains "automobile"
	e := synonymEngine()
	e.Index(doc("1", "automobile is fast"))
	e.Index(doc("2", "python is popular"))

	q := query.NewBuilder().Must("body", "car").Build()
	results := e.Search(q, 10).Hits

	found := false
	for _, r := range results {
		if r.ID == "1" {
			found = true
		}
	}
	if !found {
		t.Error("searching 'car' should find doc containing 'automobile' via synonym expansion")
	}
}

func TestSynonym_Symmetric(t *testing.T) {
	// Searching "automobile" should also find doc containing "car"
	e := synonymEngine()
	e.Index(doc("1", "car is fast"))
	e.Index(doc("2", "python is popular"))

	q := query.NewBuilder().Must("body", "automobile").Build()
	results := e.Search(q, 10).Hits

	found := false
	for _, r := range results {
		if r.ID == "1" {
			found = true
		}
	}
	if !found {
		t.Error("searching 'automobile' should find doc containing 'car' (symmetric synonym)")
	}
}

func TestSynonym_ExactMatchStillWorks(t *testing.T) {
	// Original term still matches even with synonyms configured
	e := synonymEngine()
	e.Index(doc("1", "car is fast"))

	q := query.NewBuilder().Must("body", "car").Build()
	results := e.Search(q, 10).Hits

	if len(results) == 0 {
		t.Error("exact match 'car' should still work when synonyms are configured")
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc '1', got %s", results[0].ID)
	}
}

func TestSynonym_ShouldSemanticsBoostScore(t *testing.T) {
	// A doc with both the original term AND a synonym should score higher
	// than a doc with only the synonym.
	e := synonymEngine()
	e.Index(doc("1", "car automobile"))   // has both "car" and synonym "automobile"
	e.Index(doc("2", "automobile only"))  // has only the synonym

	q := query.NewBuilder().Must("body", "car").Build()
	results := e.Search(q, 10).Hits

	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("doc with both original and synonym should rank first, got %s", results[0].ID)
	}
}

func TestSynonym_MustNotNotExpanded(t *testing.T) {
	// MustNot("body", "car") should NOT also exclude "automobile"
	e := synonymEngine()
	e.Index(doc("1", "automobile is fast"))  // has synonym but not "car"
	e.Index(doc("2", "car is great"))        // has the exact term

	q := query.NewBuilder().
		Must("body", "automobile").
		MustNot("body", "car").
		Build()
	results := e.Search(q, 10).Hits

	// doc1 has "automobile" (must match) and no "car" (must_not safe) → should appear
	// doc2 has "car" (must_not triggered) → should not appear
	for _, r := range results {
		if r.ID == "2" {
			t.Error("doc '2' contains 'car' and should be excluded by MustNot")
		}
	}
}

func TestSynonym_PhraseNotExpanded(t *testing.T) {
	// Phrase clauses should not get synonym expansion —
	// word order matters and synonym substitution breaks it.
	e := synonymEngine()
	e.Index(doc("1", "fast car racing"))
	e.Index(doc("2", "fast automobile racing"))

	// Phrase "fast car" should match doc1 only, not doc2 via synonym
	q := query.NewBuilder().Phrase("body", "fast", "car").Build()
	results := e.Search(q, 10).Hits

	for _, r := range results {
		if r.ID == "2" {
			t.Error("phrase 'fast car' should not match doc with 'fast automobile' via synonym expansion")
		}
	}
}

func TestSynonym_NoSynonymsConfigured(t *testing.T) {
	// Without WithSynonyms, behavior is identical to before — no expansion
	e := New()
	e.Index(doc("1", "automobile is fast"))

	q := query.NewBuilder().Must("body", "car").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 0 {
		t.Error("without synonyms, 'car' should not match 'automobile'")
	}
}

func TestSynonym_MultipleSynonyms(t *testing.T) {
	// All synonyms of "car" should be findable
	e := synonymEngine()
	e.Index(doc("1", "automobile racing"))
	e.Index(doc("2", "vehicle speed"))
	e.Index(doc("3", "python coding"))

	q := query.NewBuilder().Must("body", "car").Build()
	results := e.Search(q, 10).Hits

	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	if !ids["1"] {
		t.Error("'automobile' should be found via 'car' synonym")
	}
	if !ids["2"] {
		t.Error("'vehicle' should be found via 'car' synonym")
	}
	if ids["3"] {
		t.Error("'python' doc should not appear")
	}
}

func TestSynonym_ShouldClauseExpanded(t *testing.T) {
	// Should clauses are also expanded
	e := synonymEngine()
	e.Index(doc("1", "automobile is fast"))
	e.Index(doc("2", "go is great"))

	// should(car) → should(car) + should(automobile) + should(vehicle)
	q := query.NewBuilder().Should("body", "car").Should("body", "go").Build()
	results := e.Search(q, 10).Hits

	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	if !ids["1"] {
		t.Error("doc with 'automobile' should match should('car') via expansion")
	}
	if !ids["2"] {
		t.Error("doc with 'go' should match should('go')")
	}
}
