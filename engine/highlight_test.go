package engine

import (
	"strings"
	"testing"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/query"
)

// defaultAnalyzer is the same analyzer the engine uses by default.
func defaultAnalyzer() *analysis.Analyzer {
	return analysis.NewAnalyzer(&analysis.StandardTokenizer{})
}

// --- HighlightDoc unit tests ---

func TestHighlightDoc_SingleMatch(t *testing.T) {
	doc := Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "Go is a fast language"},
		},
	}

	highlights := HighlightDoc(doc, []string{"go"}, defaultAnalyzer(), "<em>", "</em>")

	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}
	if highlights[0].Field != "body" {
		t.Errorf("expected field 'body', got %q", highlights[0].Field)
	}
	if !strings.Contains(highlights[0].Snippet, "<em>Go</em>") {
		t.Errorf("expected <em>Go</em> in snippet, got %q", highlights[0].Snippet)
	}
}

func TestHighlightDoc_MultipleMatchesInOneField(t *testing.T) {
	doc := Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "Go is fast and Go is simple"},
		},
	}

	highlights := HighlightDoc(doc, []string{"go"}, defaultAnalyzer(), "<em>", "</em>")

	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight entry, got %d", len(highlights))
	}
	snippet := highlights[0].Snippet
	count := strings.Count(snippet, "<em>Go</em>")
	if count != 2 {
		t.Errorf("expected 2 wrapped occurrences of 'Go', got %d in %q", count, snippet)
	}
}

func TestHighlightDoc_MultipleFields(t *testing.T) {
	doc := Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: "Go programming"},
			"body":  {Value: "Go is a compiled language"},
		},
	}

	highlights := HighlightDoc(doc, []string{"go"}, defaultAnalyzer(), "<em>", "</em>")

	if len(highlights) != 2 {
		t.Fatalf("expected 2 highlights (title + body), got %d", len(highlights))
	}
}

func TestHighlightDoc_NoMatchProducesNoEntry(t *testing.T) {
	doc := Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "Python is a great language"},
		},
	}

	highlights := HighlightDoc(doc, []string{"go"}, defaultAnalyzer(), "<em>", "</em>")

	if len(highlights) != 0 {
		t.Errorf("expected no highlights, got %d", len(highlights))
	}
}

func TestHighlightDoc_CustomMarkers(t *testing.T) {
	doc := Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "Go is fast"},
		},
	}

	highlights := HighlightDoc(doc, []string{"go"}, defaultAnalyzer(), "**", "**")

	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}
	if !strings.Contains(highlights[0].Snippet, "**Go**") {
		t.Errorf("expected **Go** in snippet, got %q", highlights[0].Snippet)
	}
}

func TestHighlightDoc_MultipleTerms(t *testing.T) {
	doc := Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "Go is fast and simple"},
		},
	}

	highlights := HighlightDoc(doc, []string{"go", "fast"}, defaultAnalyzer(), "<em>", "</em>")

	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}
	snippet := highlights[0].Snippet
	if !strings.Contains(snippet, "<em>Go</em>") {
		t.Errorf("expected <em>Go</em> in snippet, got %q", snippet)
	}
	if !strings.Contains(snippet, "<em>fast</em>") {
		t.Errorf("expected <em>fast</em> in snippet, got %q", snippet)
	}
}

func TestHighlightDoc_EmptyTermsProducesNoHighlights(t *testing.T) {
	doc := Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "Go is fast"},
		},
	}

	highlights := HighlightDoc(doc, []string{}, defaultAnalyzer(), "<em>", "</em>")

	if len(highlights) != 0 {
		t.Errorf("expected no highlights for empty terms, got %d", len(highlights))
	}
}

// --- Search integration tests ---

func TestSearch_HighlightsPopulated(t *testing.T) {
	e := New()
	e.Index(doc("1", "Go is a fast compiled language"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10).Hits

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Highlights) == 0 {
		t.Fatal("expected highlights, got none")
	}
	snippet := results[0].Highlights[0].Snippet
	if !strings.Contains(snippet, "<em>Go</em>") {
		t.Errorf("expected <em>Go</em> in snippet, got %q", snippet)
	}
}

func TestSearch_MustNotTermNotHighlighted(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body": {Value: "Go is fast and python is slow"},
		},
	})

	// doc "1" contains "go" but the query excludes docs with python,
	// so we need a doc that matches Must but doesn't contain MustNot term.
	e.Index(doc("2", "Go is a great language"))

	results := e.Search(query.NewBuilder().Must("body", "go").MustNot("body", "python").Build(), 10).Hits

	for _, r := range results {
		for _, h := range r.Highlights {
			if strings.Contains(h.Snippet, "<em>python</em>") || strings.Contains(h.Snippet, "<em>Python</em>") {
				t.Errorf("MustNot term 'python' should not be highlighted, got %q", h.Snippet)
			}
		}
	}
}

func TestSearch_ShouldTermHighlighted(t *testing.T) {
	e := New()
	e.Index(doc("1", "Go is fast and simple"))

	q := query.NewBuilder().
		Must("body", "go").
		Should("body", "fast").
		Build()
	results := e.Search(q, 10).Hits

	if len(results) == 0 {
		t.Fatal("expected results")
	}
	snippet := results[0].Highlights[0].Snippet
	if !strings.Contains(snippet, "<em>fast</em>") {
		t.Errorf("Should term 'fast' should be highlighted, got %q", snippet)
	}
}

func TestSearch_KeywordFieldNotHighlighted(t *testing.T) {
	e := New(WithMapping(Mapping{
		"body":     {Type: FieldTypeText, Index: true, Store: true},
		"category": {Type: FieldTypeKeyword, Index: true, Store: true},
	}))

	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body":     {Value: "Go is a fast language"},
			"category": {Value: "go"},
		},
	})

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10).Hits

	if len(results) == 0 {
		t.Fatal("expected results")
	}
	for _, h := range results[0].Highlights {
		if h.Field == "category" {
			t.Errorf("keyword field 'category' should not be highlighted")
		}
	}
}

func TestSearch_NoHighlightsOnNoMatch(t *testing.T) {
	e := New()
	e.Index(doc("1", "Go is fast"))
	// Should clause: doc is included even if "fast" isn't there, but still searchable via Must
	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10).Hits

	if len(results) == 0 {
		t.Fatal("expected results")
	}
	for _, h := range results[0].Highlights {
		if h.Field == "body" && !strings.Contains(h.Snippet, "<em>") {
			t.Errorf("highlight entry exists but no markers in snippet %q", h.Snippet)
		}
	}
}
