package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// --- Default mapping (text, indexed, stored) ---

func TestMapping_DefaultIsText(t *testing.T) {
	e := New()
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: "go is fast"}},
	})

	// partial term match — analyzer splits "go is fast" into tokens
	q := query.NewBuilder().Must("body", "fast").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("default text field: expected doc '1', got %v", ids(results))
	}
}

func TestMapping_DefaultStored(t *testing.T) {
	e := New()
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: "go is fast"}},
	})

	d, ok := getDoc(e, "1")
	if !ok {
		t.Fatal("doc '1' not found in storage")
	}
	if d.Fields["body"].Value != "go is fast" {
		t.Errorf("expected stored value 'go is fast', got %q", d.Fields["body"].Value)
	}
}

// --- FieldTypeText ---

func TestMapping_TextAnalyzed(t *testing.T) {
	e := New(WithMapping(Mapping{
		"body": {Type: FieldTypeText, Index: true, Store: true},
	}))
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: "Go concurrency patterns"}},
	})

	q := query.NewBuilder().Must("body", "concurrency").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("text field: expected doc '1' for partial term, got %v", ids(results))
	}
}

// --- FieldTypeKeyword ---

func TestMapping_KeywordExactMatchHits(t *testing.T) {
	e := New(WithMapping(Mapping{
		"category": {Type: FieldTypeKeyword, Index: true, Store: true},
	}))
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"category": {Value: "machine-learning"}},
	})

	q := query.NewBuilder().Must("category", "machine-learning").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("keyword exact match: expected doc '1', got %v", ids(results))
	}
}

func TestMapping_KeywordPartialMatchMisses(t *testing.T) {
	e := New(WithMapping(Mapping{
		"category": {Type: FieldTypeKeyword, Index: true, Store: true},
	}))
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"category": {Value: "machine-learning"}},
	})

	// "machine" alone should not match — keyword is stored whole
	q := query.NewBuilder().Must("category", "machine").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 0 {
		t.Errorf("keyword partial match: expected 0 results, got %v", ids(results))
	}
}

func TestMapping_KeywordStoredInResults(t *testing.T) {
	e := New(WithMapping(Mapping{
		"category": {Type: FieldTypeKeyword, Index: true, Store: true},
	}))
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"category": {Value: "machine-learning"}},
	})

	q := query.NewBuilder().Must("category", "machine-learning").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Fields["category"].Value != "machine-learning" {
		t.Errorf("expected category value 'machine-learning' in result, got %q", results[0].Fields["category"].Value)
	}
}

func TestMapping_KeywordAggregationGroups(t *testing.T) {
	e := New(WithMapping(Mapping{
		"status": {Type: FieldTypeKeyword, Index: true, Store: true},
	}))
	e.Index(Document{ID: "1", Fields: map[string]Field{"status": {Value: "published"}}})
	e.Index(Document{ID: "2", Fields: map[string]Field{"status": {Value: "published"}}})
	e.Index(Document{ID: "3", Fields: map[string]Field{"status": {Value: "draft"}}})

	agg := e.Aggregate(query.NewBuilder().Build(), "status", 10)

	counts := map[string]int{}
	for _, b := range agg.Buckets {
		counts[b.Key] = b.Count
	}
	if counts["published"] != 2 {
		t.Errorf("expected 'published' count 2, got %d", counts["published"])
	}
	if counts["draft"] != 1 {
		t.Errorf("expected 'draft' count 1, got %d", counts["draft"])
	}
}

// --- FieldTypeSkip ---

func TestMapping_SkipNotIndexed(t *testing.T) {
	e := New(WithMapping(Mapping{
		"internal": {Type: FieldTypeSkip},
	}))
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"internal": {Value: "secret"}},
	})

	q := query.NewBuilder().Must("internal", "secret").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 0 {
		t.Errorf("skip field: expected 0 results, got %v", ids(results))
	}
}

func TestMapping_SkipNotStored(t *testing.T) {
	e := New(WithMapping(Mapping{
		"internal": {Type: FieldTypeSkip},
	}))
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body":     {Value: "hello world"},
			"internal": {Value: "secret"},
		},
	})

	d, ok := getDoc(e, "1")
	if !ok {
		t.Fatal("doc '1' not found in storage")
	}
	if _, found := d.Fields["internal"]; found {
		t.Error("skip field should not be stored in the document")
	}
}

// --- Index: false ---

func TestMapping_IndexFalseNotSearchable(t *testing.T) {
	e := New(WithMapping(Mapping{
		"url": {Type: FieldTypeText, Index: false, Store: true},
	}))
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"url": {Value: "https://example.com"}},
	})

	q := query.NewBuilder().Must("url", "example").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 0 {
		t.Errorf("index:false field: expected 0 results, got %v", ids(results))
	}
}

func TestMapping_IndexFalseStoredInResults(t *testing.T) {
	e := New(WithMapping(Mapping{
		"body": {Type: FieldTypeText, Index: true, Store: true},
		"url":  {Type: FieldTypeText, Index: false, Store: true},
	}))
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "go is fast"},
			"url":  {Value: "https://example.com"},
		},
	})

	q := query.NewBuilder().Must("body", "fast").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Fields["url"].Value != "https://example.com" {
		t.Errorf("index:false field should still appear in results, got %q", results[0].Fields["url"].Value)
	}
}

// --- Store: false ---

func TestMapping_StoreFalseSearchable(t *testing.T) {
	e := New(WithMapping(Mapping{
		"body": {Type: FieldTypeText, Index: true, Store: false},
	}))
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: "go is fast"}},
	})

	q := query.NewBuilder().Must("body", "fast").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("store:false field: expected doc '1' to be searchable, got %v", ids(results))
	}
}

func TestMapping_StoreFalseNotInResults(t *testing.T) {
	e := New(WithMapping(Mapping{
		"body": {Type: FieldTypeText, Index: true, Store: false},
	}))
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: "go is fast"}},
	})

	q := query.NewBuilder().Must("body", "fast").Build()
	results := e.Search(q, 10).Hits
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if _, found := results[0].Fields["body"]; found {
		t.Error("store:false field should not appear in result Fields")
	}
}

// --- Mixed mapping ---

func TestMapping_MultipleFieldsMixedTypes(t *testing.T) {
	e := New(WithMapping(Mapping{
		"title":    {Type: FieldTypeText, Index: true, Store: true},
		"category": {Type: FieldTypeKeyword, Index: true, Store: true},
		"internal": {Type: FieldTypeSkip},
	}))
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title":    {Value: "Go concurrency"},
			"category": {Value: "programming"},
			"internal": {Value: "secret"},
		},
	})

	// text field: partial match
	q := query.NewBuilder().Must("title", "concurrency").Build()
	if results := e.Search(q, 10).Hits; len(results) != 1 {
		t.Errorf("text field: expected 1 result, got %v", ids(results))
	}

	// keyword field: exact match
	q = query.NewBuilder().Must("category", "programming").Build()
	if results := e.Search(q, 10).Hits; len(results) != 1 {
		t.Errorf("keyword field: expected 1 result, got %v", ids(results))
	}

	// skip field: not searchable
	q = query.NewBuilder().Must("internal", "secret").Build()
	if results := e.Search(q, 10).Hits; len(results) != 0 {
		t.Errorf("skip field: expected 0 results, got %v", ids(results))
	}
}
