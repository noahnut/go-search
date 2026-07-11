package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// statusDoc creates a document with a body text field and a status keyword field.
func statusDoc(id, body, status string) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"body":   {Value: body, Boost: 1.0},
			"status": {Value: status},
		},
	}
}

// --- passTermsFilters unit tests ---

func TestPassTermsFilters_Empty(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"status": {Value: "open"}}}
	if !passTermsFilters(doc, nil) {
		t.Error("empty terms should always pass")
	}
}

func TestPassTermsFilters_SingleValue_Match(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"status": {Value: "open"}}}
	terms := []query.TermsClause{{Field: "status", Values: []string{"open"}}}
	if !passTermsFilters(doc, terms) {
		t.Error("exact match should pass")
	}
}

func TestPassTermsFilters_SingleValue_NoMatch(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"status": {Value: "closed"}}}
	terms := []query.TermsClause{{Field: "status", Values: []string{"open"}}}
	if passTermsFilters(doc, terms) {
		t.Error("non-matching value should fail")
	}
}

func TestPassTermsFilters_MultipleValues_MatchFirst(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"status": {Value: "open"}}}
	terms := []query.TermsClause{{Field: "status", Values: []string{"open", "in-progress", "review"}}}
	if !passTermsFilters(doc, terms) {
		t.Error("matching the first value should pass")
	}
}

func TestPassTermsFilters_MultipleValues_MatchMiddle(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"status": {Value: "in-progress"}}}
	terms := []query.TermsClause{{Field: "status", Values: []string{"open", "in-progress", "review"}}}
	if !passTermsFilters(doc, terms) {
		t.Error("matching any value should pass")
	}
}

func TestPassTermsFilters_MultipleValues_NoMatch(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"status": {Value: "closed"}}}
	terms := []query.TermsClause{{Field: "status", Values: []string{"open", "in-progress", "review"}}}
	if passTermsFilters(doc, terms) {
		t.Error("value not in list should fail")
	}
}

func TestPassTermsFilters_FieldAbsent(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"body": {Value: "hello"}}}
	terms := []query.TermsClause{{Field: "status", Values: []string{"open"}}}
	if passTermsFilters(doc, terms) {
		t.Error("missing field should fail")
	}
}

func TestPassTermsFilters_MultipleClauses_AllMustPass(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{
		"status":   {Value: "open"},
		"priority": {Value: "high"},
	}}
	terms := []query.TermsClause{
		{Field: "status", Values: []string{"open", "in-progress"}},
		{Field: "priority", Values: []string{"low", "medium"}}, // high not in list
	}
	if passTermsFilters(doc, terms) {
		t.Error("all terms clauses must pass; one failure should exclude the doc")
	}
}

func TestPassTermsFilters_MultipleClauses_AllPass(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{
		"status":   {Value: "open"},
		"priority": {Value: "high"},
	}}
	terms := []query.TermsClause{
		{Field: "status", Values: []string{"open", "in-progress"}},
		{Field: "priority", Values: []string{"high", "critical"}},
	}
	if !passTermsFilters(doc, terms) {
		t.Error("both clauses match, doc should pass")
	}
}

// --- Search integration tests ---

func TestTerms_SingleValue(t *testing.T) {
	e := New()
	e.Index(statusDoc("1", "ticket", "open"))
	e.Index(statusDoc("2", "ticket", "closed"))
	e.Index(statusDoc("3", "ticket", "open"))

	q := query.NewBuilder().
		Must("body", "ticket").
		Terms("status", "open").
		Build()

	results := e.Search(q, 10).Hits
	got := ids(results)

	if len(got) != 2 {
		t.Fatalf("expected 2 results (open tickets), got %v", got)
	}
	for _, id := range []string{"1", "3"} {
		found := false
		for _, g := range got {
			if g == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected doc %q in results, got %v", id, got)
		}
	}
}

func TestTerms_MultipleValues(t *testing.T) {
	e := New()
	e.Index(statusDoc("1", "ticket", "open"))
	e.Index(statusDoc("2", "ticket", "closed"))
	e.Index(statusDoc("3", "ticket", "in-progress"))
	e.Index(statusDoc("4", "ticket", "review"))

	q := query.NewBuilder().
		Must("body", "ticket").
		Terms("status", "open", "in-progress").
		Build()

	results := e.Search(q, 10).Hits
	got := ids(results)

	if len(got) != 2 {
		t.Fatalf("expected 2 results (open or in-progress), got %v", got)
	}
	for _, id := range []string{"1", "3"} {
		found := false
		for _, g := range got {
			if g == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected doc %q in results, got %v", id, got)
		}
	}
}

func TestTerms_ExcludesAll(t *testing.T) {
	e := New()
	e.Index(statusDoc("1", "ticket", "closed"))
	e.Index(statusDoc("2", "ticket", "closed"))

	q := query.NewBuilder().
		Must("body", "ticket").
		Terms("status", "open").
		Build()

	results := e.Search(q, 10).Hits
	if len(results) != 0 {
		t.Errorf("expected no results, got %v", ids(results))
	}
}

func TestTerms_MissingFieldExcludesDoc(t *testing.T) {
	e := New()
	e.Index(statusDoc("1", "ticket", "open"))
	e.Index(Document{ // no status field
		ID:     "2",
		Fields: map[string]Field{"body": {Value: "ticket", Boost: 1.0}},
	})

	q := query.NewBuilder().
		Must("body", "ticket").
		Terms("status", "open").
		Build()

	results := e.Search(q, 10).Hits
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("expected only doc with status field, got %v", got)
	}
}

func TestTerms_CombinedWithMust(t *testing.T) {
	e := New()
	e.Index(statusDoc("1", "go tutorial", "open"))
	e.Index(statusDoc("2", "python tutorial", "open"))
	e.Index(statusDoc("3", "go tutorial", "closed"))

	q := query.NewBuilder().
		Must("body", "go").
		Terms("status", "open").
		Build()

	results := e.Search(q, 10).Hits
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("expected only doc 1 (body:go AND status:open), got %v", got)
	}
}

func TestTerms_MultipleTermsClauses(t *testing.T) {
	e := New()
	e.Index(Document{ID: "1", Fields: map[string]Field{
		"body":     {Value: "ticket", Boost: 1.0},
		"status":   {Value: "open"},
		"priority": {Value: "high"},
	}})
	e.Index(Document{ID: "2", Fields: map[string]Field{
		"body":     {Value: "ticket", Boost: 1.0},
		"status":   {Value: "open"},
		"priority": {Value: "low"}, // fails priority filter
	}})
	e.Index(Document{ID: "3", Fields: map[string]Field{
		"body":     {Value: "ticket", Boost: 1.0},
		"status":   {Value: "closed"}, // fails status filter
		"priority": {Value: "high"},
	}})

	q := query.NewBuilder().
		Must("body", "ticket").
		Terms("status", "open", "in-progress").
		Terms("priority", "high", "critical").
		Build()

	results := e.Search(q, 10).Hits
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("expected only doc 1 (both terms clauses satisfied), got %v", got)
	}
}

func TestTerms_SingleValueBehavesLikeMustKeyword(t *testing.T) {
	e := New()
	e.Index(statusDoc("1", "item", "active"))
	e.Index(statusDoc("2", "item", "inactive"))

	q := query.NewBuilder().
		Must("body", "item").
		Terms("status", "active").
		Build()

	results := e.Search(q, 10).Hits
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("single-value Terms should behave like a keyword filter, got %v", got)
	}
}

func TestTerms_CombinedWithRange(t *testing.T) {
	e := New()
	e.Index(Document{ID: "1", Fields: map[string]Field{
		"body":   {Value: "item", Boost: 1.0},
		"status": {Value: "open"},
		"price":  {Value: "50"},
	}})
	e.Index(Document{ID: "2", Fields: map[string]Field{
		"body":   {Value: "item", Boost: 1.0},
		"status": {Value: "open"},
		"price":  {Value: "200"}, // fails range
	}})
	e.Index(Document{ID: "3", Fields: map[string]Field{
		"body":   {Value: "item", Boost: 1.0},
		"status": {Value: "closed"}, // fails terms
		"price":  {Value: "50"},
	}})

	q := query.NewBuilder().
		Must("body", "item").
		Terms("status", "open").
		Range("price", query.Ptr(1), query.Ptr(100)).
		Build()

	results := e.Search(q, 10).Hits
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("expected only doc 1 (terms AND range satisfied), got %v", got)
	}
}
