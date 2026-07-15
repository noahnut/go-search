package index

import (
	"github.com/noahfan/go-search/storage/memory"
	"fmt"
	"testing"
)

func TestSegment_FlushTriggered(t *testing.T) {
	idx := NewWithFlushSize(2, memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	if len(idx.segments) != 0 {
		t.Errorf("expected 0 segments before flush, got %d", len(idx.segments))
	}

	idx.Add("doc2", "go runs", nil, a) // bufferDocs hits 2 → flush
	if len(idx.segments) != 1 {
		t.Errorf("expected 1 segment after flush, got %d", len(idx.segments))
	}
	if len(idx.bufferDocs) != 0 {
		t.Errorf("expected empty buffer after flush, got %d docs", len(idx.bufferDocs))
	}
}

func TestSegment_LookupInSegment(t *testing.T) {
	idx := NewWithFlushSize(2, memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a) // triggers flush

	postings := idx.Lookup("go")
	if len(postings) != 2 {
		t.Fatalf("expected 2 postings for 'go' after flush, got %d", len(postings))
	}
}

func TestSegment_LookupAcrossBufferAndSegment(t *testing.T) {
	idx := NewWithFlushSize(2, memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a)    // flush → doc1+doc2 go to segment
	idx.Add("doc3", "go everywhere", nil, a) // stays in buffer

	postings := idx.Lookup("go")
	if len(postings) != 3 {
		t.Fatalf("expected 3 postings across buffer+segment, got %d", len(postings))
	}
}

func TestSegment_DeleteViaTombstone(t *testing.T) {
	idx := NewWithFlushSize(2, memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a) // triggers flush

	idx.Delete("doc1")

	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc1" {
			t.Error("doc1 should not appear in Lookup after delete")
		}
	}

	found := false
	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc2" {
			found = true
		}
	}
	if !found {
		t.Error("doc2 should still be findable after deleting doc1")
	}
}

func TestSegment_DeleteInBuffer(t *testing.T) {
	idx := NewWithFlushSize(10, memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a) // stays in buffer
	idx.Delete("doc1")

	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc1" {
			t.Error("doc1 should not appear after delete from buffer")
		}
	}
}

func TestSegment_AddAfterDelete(t *testing.T) {
	idx := NewWithFlushSize(10, memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Delete("doc1")
	idx.Add("doc1", "go is fast", nil, a) // re-add should clear tombstone

	found := false
	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc1" {
			found = true
		}
	}
	if !found {
		t.Error("doc1 should be findable after re-adding it post-delete")
	}
}

func TestSegment_DocCount(t *testing.T) {
	idx := NewWithFlushSize(2, memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a) // triggers flush

	if idx.DocCount() != 2 {
		t.Errorf("expected DocCount=2, got %d", idx.DocCount())
	}

	idx.Delete("doc1")
	if idx.DocCount() != 1 {
		t.Errorf("expected DocCount=1 after delete, got %d", idx.DocCount())
	}
}

func TestSegment_MultipleFlushes(t *testing.T) {
	idx := NewWithFlushSize(2, memory.New())
	a := newAnalyzer()

	for i := 1; i <= 5; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}
	// add doc1: buffer=1
	// add doc2: buffer=2 → flush → segments=1, buffer=0
	// add doc3: buffer=1
	// add doc4: buffer=2 → flush → segments=2, buffer=0
	// add doc5: buffer=1

	if len(idx.segments) != 2 {
		t.Errorf("expected 2 segments, got %d", len(idx.segments))
	}
	if len(idx.bufferDocs) != 1 {
		t.Errorf("expected 1 doc in buffer, got %d", len(idx.bufferDocs))
	}

	postings := idx.Lookup("go")
	if len(postings) != 5 {
		t.Errorf("expected 5 postings across all segments+buffer, got %d", len(postings))
	}
}

func TestSegment_SnapshotRestore(t *testing.T) {
	idx := NewWithFlushSize(2, memory.New())
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a) // triggers flush
	idx.Delete("doc2")

	segs, tombstones := idx.Snapshot()

	idx2 := New(memory.New())
	idx2.Restore(segs, tombstones)

	for _, p := range idx2.Lookup("go") {
		if p.DocID == "doc2" {
			t.Error("doc2 is tombstoned and should not appear after restore")
		}
	}

	found := false
	for _, p := range idx2.Lookup("go") {
		if p.DocID == "doc1" {
			found = true
		}
	}
	if !found {
		t.Error("doc1 should be findable after restore")
	}
}
