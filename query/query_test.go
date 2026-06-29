package query

import (
	"testing"

	"github.com/noahfan/go-search/index"
)

// posting builds a minimal Posting for test input.
func posting(docID string) index.Posting {
	return index.Posting{DocID: docID, Frequency: 1, Positions: []int{0}}
}

func TestBuilder(t *testing.T) {
	q := NewBuilder().
		Must("body", "go").
		Should("body", "fast").
		MustNot("body", "python").
		Build()

	if len(q.Clauses) != 3 {
		t.Fatalf("expected 3 clauses, got %d", len(q.Clauses))
	}

	types := map[ClauseType]Clause{}
	for _, c := range q.Clauses {
		types[c.Type] = c
	}
	if types[Must].Field != "body" || types[Must].Term != "go" {
		t.Errorf("expected Must clause field='body' term='go', got field=%q term=%q", types[Must].Field, types[Must].Term)
	}
	if types[Should].Term != "fast" {
		t.Errorf("expected Should clause term='fast', got %q", types[Should].Term)
	}
	if types[MustNot].Term != "python" {
		t.Errorf("expected MustNot clause term='python', got %q", types[MustNot].Term)
	}
}

func TestBuilder_Phrase(t *testing.T) {
	q := NewBuilder().Phrase("body", "new", "york").Build()

	if len(q.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(q.Clauses))
	}
	c := q.Clauses[0]
	if c.Field != "body" {
		t.Errorf("expected field 'body', got %q", c.Field)
	}
	if c.Term != "new york" {
		t.Errorf("expected term 'new york', got %q", c.Term)
	}
	if c.Type != Phrase {
		t.Errorf("expected Phrase type, got %q", c.Type)
	}
}

func TestMatch_Must(t *testing.T) {
	q := NewBuilder().Must("body", "go").Build()

	if !Match(q, map[string]index.Posting{"body:go": posting("doc1")}) {
		t.Error("doc with 'go' should match must('go')")
	}
	if Match(q, map[string]index.Posting{"body:python": posting("doc1")}) {
		t.Error("doc without 'go' should not match must('go')")
	}
}

func TestMatch_MustNot(t *testing.T) {
	q := NewBuilder().MustNot("body", "python").Build()

	if Match(q, map[string]index.Posting{"body:python": posting("doc1")}) {
		t.Error("doc with 'python' should not match must_not('python')")
	}
	if !Match(q, map[string]index.Posting{"body:go": posting("doc1")}) {
		t.Error("doc without 'python' should match must_not('python')")
	}
}

func TestMatch_Should_NoMust(t *testing.T) {
	// When there are no must-clauses, at least one should must match.
	q := NewBuilder().Should("body", "go").Should("body", "rust").Build()

	if !Match(q, map[string]index.Posting{"body:go": posting("doc1")}) {
		t.Error("doc with 'go' should match should('go')|should('rust')")
	}
	if !Match(q, map[string]index.Posting{"body:rust": posting("doc1")}) {
		t.Error("doc with 'rust' should match should('go')|should('rust')")
	}
	if Match(q, map[string]index.Posting{"body:python": posting("doc1")}) {
		t.Error("doc with neither 'go' nor 'rust' should not match")
	}
}

func TestMatch_Should_WithMust(t *testing.T) {
	// When must-clauses exist, should is optional.
	q := NewBuilder().Must("body", "go").Should("body", "fast").Build()

	if !Match(q, map[string]index.Posting{"body:go": posting("doc1")}) {
		t.Error("doc with 'go' but not 'fast' should match when must exists")
	}
	if Match(q, map[string]index.Posting{"body:fast": posting("doc1")}) {
		t.Error("doc without 'go' should not match must('go')")
	}
}

func TestMatch_MultipleMust(t *testing.T) {
	q := NewBuilder().Must("body", "go").Must("body", "fast").Build()

	if !Match(q, map[string]index.Posting{
		"body:go": posting("doc1"), "body:fast": posting("doc1"),
	}) {
		t.Error("doc with both terms should match")
	}
	if Match(q, map[string]index.Posting{"body:go": posting("doc1")}) {
		t.Error("doc with only 'go' should not match must('go') AND must('fast')")
	}
}

func TestMatch_Combined(t *testing.T) {
	q := NewBuilder().Must("body", "go").MustNot("body", "python").Should("body", "fast").Build()

	if !Match(q, map[string]index.Posting{"body:go": posting("doc1")}) {
		t.Error("should match: has must, no must_not")
	}
	if Match(q, map[string]index.Posting{
		"body:go": posting("doc1"), "body:python": posting("doc1"),
	}) {
		t.Error("should not match: has must_not term")
	}
}
