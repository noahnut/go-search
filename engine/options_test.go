package engine

import (
	"testing"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/query"
	"github.com/noahfan/go-search/scoring"
)

func TestNew_DefaultsWork(t *testing.T) {
	e := New()
	if e == nil {
		t.Fatal("New() returned nil")
	}
	err := e.Index(doc("1", "hello world"))
	if err != nil {
		t.Errorf("Index with default engine failed: %v", err)
	}
}

func TestWithAnalyzer_CustomStopWords(t *testing.T) {
	a := analysis.NewAnalyzer(
		&analysis.StandardTokenizer{},
		analysis.NewStopWordFilter([]string{"go"}),
	)
	e := New(WithAnalyzer(a))
	e.Index(doc("1", "go is fast"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results: 'go' was stop-worded, got %d", len(results))
	}

	q2 := query.NewBuilder().Must("body", "fast").Build()
	results2 := e.Search(q2, 10)
	if len(results2) != 1 {
		t.Errorf("expected 1 result for 'fast', got %d", len(results2))
	}
}

func TestWithAnalyzer_UsedForQueryToo(t *testing.T) {
	a := analysis.NewAnalyzer(
		&analysis.StandardTokenizer{},
		&analysis.LowercaseFilter{},
	)
	e := New(WithAnalyzer(a))
	e.Index(doc("1", "Go Is Fast"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestWithBM25Params_AffectsScore(t *testing.T) {
	d := doc("1", "go go go go go")
	q := query.NewBuilder().Must("body", "go").Build()

	e1 := New()
	e1.Index(d)
	r1 := e1.Search(q, 10)

	e2 := New(WithBM25Params(scoring.Params{K1: 2.0, B: 0.75}))
	e2.Index(d)
	r2 := e2.Search(q, 10)

	if len(r1) == 0 || len(r2) == 0 {
		t.Fatal("expected results from both engines")
	}
	if r1[0].Score == r2[0].Score {
		t.Error("different BM25 params should produce different scores")
	}
}

func TestWithOptions_Combined(t *testing.T) {
	a := analysis.NewAnalyzer(
		&analysis.StandardTokenizer{},
		analysis.NewStopWordFilter([]string{"the", "is"}),
	)
	params := scoring.Params{K1: 1.5, B: 0.5}

	e := New(WithAnalyzer(a), WithBM25Params(params))
	e.Index(doc("1", "the go language is fast"))

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}
