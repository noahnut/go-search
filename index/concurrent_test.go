package index

import (
	"fmt"
	"sync"
	"testing"
)

// TestConcurrent_LookupAcrossSegments verifies correctness: all docs are found
// after multiple flushes create multiple segments, searched concurrently.
func TestConcurrent_LookupAcrossSegments(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	for i := 1; i <= 6; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}
	// 6 docs with flushSize=2 → 3 segments, buffer empty

	postings := idx.Lookup("go")
	if len(postings) != 6 {
		t.Errorf("expected 6 postings across 3 segments, got %d", len(postings))
	}
}

// TestConcurrent_LookupFiltersDeleted verifies tombstone filtering works
// when results are merged from concurrent segment goroutines.
func TestConcurrent_LookupFiltersDeleted(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	for i := 1; i <= 4; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}
	// 2 segments

	idx.Delete("doc1")
	idx.Delete("doc3")

	for _, p := range idx.Lookup("go") {
		if p.DocID == "doc1" || p.DocID == "doc3" {
			t.Errorf("deleted doc %s should not appear in concurrent Lookup", p.DocID)
		}
	}

	postings := idx.Lookup("go")
	if len(postings) != 2 {
		t.Errorf("expected 2 postings after deleting 2 of 4, got %d", len(postings))
	}
}

// TestConcurrent_ConcurrentLookups runs many Lookup calls in parallel to
// detect data races. Run with: go test -race ./index/...
func TestConcurrent_ConcurrentLookups(t *testing.T) {
	idx := NewWithFlushSize(2)
	a := newAnalyzer()

	for i := 1; i <= 8; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}
	// 4 segments, each searched concurrently by Lookup

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			postings := idx.Lookup("go")
			if len(postings) != 8 {
				t.Errorf("expected 8 postings, got %d", len(postings))
			}
		}()
	}
	wg.Wait()
}

// TestConcurrent_LookupWhileAdding runs concurrent reads and writes to verify
// the RWMutex prevents data races between Lookup and Add.
func TestConcurrent_LookupWhileAdding(t *testing.T) {
	idx := NewWithFlushSize(50)
	a := newAnalyzer()

	// seed some docs first
	for i := 1; i <= 10; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}

	var wg sync.WaitGroup

	// concurrent writers
	for i := 11; i <= 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			idx.Add(fmt.Sprintf("doc%d", n), "go is fast", nil, a)
		}(i)
	}

	// concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx.Lookup("go")
		}()
	}

	wg.Wait()
}

// TestConcurrent_BufferSearchedSequentially confirms that a term found only in
// the buffer (no segments) is still returned correctly.
func TestConcurrent_BufferSearchedSequentially(t *testing.T) {
	idx := NewWithFlushSize(100) // high flush size so nothing flushes
	a := newAnalyzer()

	idx.Add("doc1", "go is fast", nil, a)
	idx.Add("doc2", "go runs", nil, a)

	if len(idx.segments) != 0 {
		t.Fatalf("precondition: expected 0 segments, got %d", len(idx.segments))
	}

	postings := idx.Lookup("go")
	if len(postings) != 2 {
		t.Errorf("expected 2 postings from buffer-only search, got %d", len(postings))
	}
}
