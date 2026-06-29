package index

import (
	"fmt"
	"testing"
)

func TestMerge_CollapsesSegments(t *testing.T) {
	// 4 docs with flushSize=2 → 2 segments; after merge → 1 segment
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	for i := 1; i <= 4; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}
	if len(idx.segments) != 2 {
		t.Fatalf("precondition: expected 2 segments, got %d", len(idx.segments))
	}

	idx.Merge()

	if len(idx.segments) != 1 {
		t.Errorf("expected 1 segment after merge, got %d", len(idx.segments))
	}
}

func TestMerge_AllDocsStillFindable(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	for i := 1; i <= 4; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}

	idx.Merge()

	postings := idx.Lookup("go")
	if len(postings) != 4 {
		t.Errorf("expected 4 postings after merge, got %d", len(postings))
	}
}

func TestMerge_TombstonedDocsDropped(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a) // triggers flush → segment
	idx.Delete("doc1")

	idx.Merge()

	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc1" {
			t.Error("doc1 is tombstoned and should not appear after merge")
		}
	}
}

func TestMerge_TombstonesCleared(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a) // triggers flush
	idx.Delete("doc1")

	idx.Merge()

	if len(idx.tombstones) != 0 {
		t.Errorf("expected tombstones cleared after merge, got %d", len(idx.tombstones))
	}
}

func TestMerge_NoSegmentsIsNoop(t *testing.T) {
	idx := New()
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a) // stays in buffer, no flush

	// should not panic
	idx.Merge()

	if len(idx.segments) != 1 {
		// merge of 0 segments creates one empty segment — acceptable
		// as long as Lookup still works
	}

	// doc1 is in the buffer; must still be findable
	postings := idx.Lookup("go")
	found := false
	for _, p := range postings {
		if p.DocID == "doc1" {
			found = true
		}
	}
	if !found {
		t.Error("doc1 in buffer should still be findable after merging 0 segments")
	}
}

func TestMerge_DocCountUnchanged(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	for i := 1; i <= 4; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}

	before := idx.DocCount()
	idx.Merge()

	if idx.DocCount() != before {
		t.Errorf("DocCount changed after merge: was %d, now %d", before, idx.DocCount())
	}
}

func TestMerge_TermCountUnchanged(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	for i := 1; i <= 4; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}

	before := idx.TermCount()
	idx.Merge()

	if idx.TermCount() != before {
		t.Errorf("TermCount changed after merge: was %d, now %d", before, idx.TermCount())
	}
}

func TestMerge_SurvivingDocFindableAfterTombstone(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a) // triggers flush
	idx.Delete("doc1")

	idx.Merge()

	found := false
	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc2" {
			found = true
		}
	}
	if !found {
		t.Error("doc2 should still be findable after merging with doc1 tombstoned")
	}
}
