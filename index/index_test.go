package index

import (
	"fmt"
	"sync"
	"testing"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/storage/memory"
)

// shared analyzer for all tests: standard tokenizer, no stop words
func newAnalyzer() *analysis.Analyzer {
	return analysis.NewAnalyzer(&analysis.StandardTokenizer{})
}

func TestIndex_Add_Lookup(t *testing.T) {
	idx := New(memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)

	postings := idx.Lookup("go")
	if len(postings) != 1 {
		t.Fatalf("expected 1 posting for 'go', got %d", len(postings))
	}
	if postings[0].DocID != "doc1" {
		t.Errorf("expected DocID 'doc1', got %s", postings[0].DocID)
	}
}

func TestIndex_Lookup_Missing(t *testing.T) {
	idx := New(memory.New())

	got := idx.Lookup("nonexistent")
	if got != nil {
		t.Errorf("expected nil for missing term, got %v", got)
	}
}

func TestIndex_Frequency_And_Positions(t *testing.T) {
	idx := New(memory.New())
	a := newAnalyzer()

	// "go" appears twice
	idx.Add("doc1", "go fast go", nil, a)

	postings := idx.Lookup("go")
	if len(postings) != 1 {
		t.Fatalf("expected 1 posting, got %d", len(postings))
	}
	p := postings[0]
	if p.Frequency != 2 {
		t.Errorf("expected frequency 2, got %d", p.Frequency)
	}
	if len(p.Positions) != 2 {
		t.Errorf("expected 2 positions, got %v", p.Positions)
	}
}

func TestIndex_MultipleDocuments(t *testing.T) {
	idx := New(memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs everywhere", nil, a)

	postings := idx.Lookup("go")
	if len(postings) != 2 {
		t.Fatalf("expected 2 postings for 'go', got %d", len(postings))
	}
}

func TestIndex_DocCount(t *testing.T) {
	idx := New(memory.New())
	a := newAnalyzer()

	if idx.DocCount() != 0 {
		t.Errorf("expected 0 docs, got %d", idx.DocCount())
	}

	idx.Add("doc1", "hello world", nil, a)
	idx.Add("doc2", "foo bar", nil, a)

	if idx.DocCount() != 2 {
		t.Errorf("expected 2 docs, got %d", idx.DocCount())
	}
}

func TestIndex_TermCount(t *testing.T) {
	idx := New(memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go fast", nil, a)
	// "go" and "fast" = 2 unique terms
	if idx.TermCount() != 2 {
		t.Errorf("expected 2 terms, got %d", idx.TermCount())
	}

	// adding "go" again from a new doc should not increase term count
	idx.Add("doc2", "go slow", nil, a)
	// "go", "fast", "slow" = 3 unique terms
	if idx.TermCount() != 3 {
		t.Errorf("expected 3 terms after doc2, got %d", idx.TermCount())
	}
}

func TestIndex_Delete(t *testing.T) {
	idx := New(memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a)

	idx.Delete("doc1")

	// doc1 should be gone from "go" postings
	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc1" {
			t.Error("doc1 still present after delete")
		}
	}
	// doc2 should still be there
	found := false
	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc2" {
			found = true
		}
	}
	if !found {
		t.Error("doc2 should still be present after deleting doc1")
	}

	if idx.DocCount() != 1 {
		t.Errorf("expected 1 doc after delete, got %d", idx.DocCount())
	}
}

func TestIndex_ConcurrentAdd(t *testing.T) {
	idx := New(memory.New())
	a := analysis.NewAnalyzer(&analysis.StandardTokenizer{})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			idx.Add(fmt.Sprintf("doc%d", n), "go is fast", nil, a)
		}(i)
	}
	wg.Wait()
}
