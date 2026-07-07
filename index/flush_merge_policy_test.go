package index

import (
	"fmt"
	"testing"
	"time"
)

// --- FlushPolicy: MaxTokens ---

func TestFlushPolicy_MaxTokensTriggersFlush(t *testing.T) {
	// "go is fast" → 3 tokens per doc; MaxTokens=6 → flush after every 2 docs
	a := newAnalyzer()
	idx := New(WithFlushPolicy(&FlushPolicy{MaxTokens: 6}))

	idx.Add("doc1", "go is fast", nil, a)
	if len(idx.segments) != 0 {
		t.Fatalf("after 1 doc (3 tokens): expected 0 segments, got %d", len(idx.segments))
	}

	idx.Add("doc2", "go is fast", nil, a)
	// 6 tokens accumulated → should have flushed
	if len(idx.segments) != 1 {
		t.Fatalf("after 2 docs (6 tokens): expected 1 segment, got %d", len(idx.segments))
	}

	idx.Add("doc3", "go is fast", nil, a)
	idx.Add("doc4", "go is fast", nil, a)
	if len(idx.segments) != 2 {
		t.Fatalf("after 4 docs (12 tokens): expected 2 segments, got %d", len(idx.segments))
	}
}

func TestFlushPolicy_MaxTokensZeroDisabled(t *testing.T) {
	// MaxTokens=0 means no auto-flush on token count
	a := newAnalyzer()
	idx := New(WithFlushPolicy(&FlushPolicy{MaxTokens: 0}))

	for i := 0; i < 10; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}

	if len(idx.segments) != 0 {
		t.Errorf("MaxTokens=0 should disable auto-flush; got %d segments", len(idx.segments))
	}
}

func TestFlushPolicy_AllDocsStillFindableAfterAutoFlush(t *testing.T) {
	a := newAnalyzer()
	idx := New(WithFlushPolicy(&FlushPolicy{MaxTokens: 6}))

	for i := 1; i <= 6; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go is fast", nil, a)
	}

	postings := idx.Lookup("go")
	if len(postings) != 6 {
		t.Errorf("expected 6 postings after auto-flush, got %d", len(postings))
	}
}

// --- FlushPolicy: FlushInterval ---

func TestFlushPolicy_FlushIntervalFlushesOnTimer(t *testing.T) {
	a := newAnalyzer()
	idx := New(WithFlushPolicy(&FlushPolicy{
		MaxTokens:     0,
		FlushInterval: 50 * time.Millisecond,
	}))
	defer idx.StopFlushTimer()

	idx.Add("doc1", "go is fast", nil, a)

	// before interval fires — still in buffer
	idx.mu.RLock()
	segsBeforeInterval := len(idx.segments)
	idx.mu.RUnlock()
	if segsBeforeInterval != 0 {
		t.Fatalf("before interval: expected 0 segments, got %d", segsBeforeInterval)
	}

	time.Sleep(120 * time.Millisecond)

	// after interval — should have flushed
	idx.mu.RLock()
	segsAfterInterval := len(idx.segments)
	idx.mu.RUnlock()
	if segsAfterInterval == 0 {
		t.Error("after interval: expected at least 1 segment")
	}

	postings := idx.Lookup("go")
	if len(postings) != 1 {
		t.Errorf("doc1 should be findable after interval flush, got %d postings", len(postings))
	}
}

// --- MergePolicy: MaxSegments ---

func TestMergePolicy_TriggersBackgroundMerge(t *testing.T) {
	a := newAnalyzer()
	// flush every doc, merge when segments > 2
	idx := New(
		WithFlushPolicy(&FlushPolicy{MaxTokens: 1}),
		WithMergePolicy(&MergePolicy{MaxSegments: 2}),
	)

	// add enough docs to create >2 segments and trigger background merge
	for i := 1; i <= 6; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "x", nil, a)
	}

	// wait for background merge to finish
	time.Sleep(50 * time.Millisecond)

	idx.mu.RLock()
	segs := len(idx.segments)
	idx.mu.RUnlock()

	if segs > 3 {
		t.Errorf("expected ≤3 segments after background merge, got %d", segs)
	}
}

func TestMergePolicy_AllDocsStillFindableAfterBackgroundMerge(t *testing.T) {
	a := newAnalyzer()
	idx := New(
		WithFlushPolicy(&FlushPolicy{MaxTokens: 1}),
		WithMergePolicy(&MergePolicy{MaxSegments: 2}),
	)

	for i := 1; i <= 6; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go", nil, a)
	}

	time.Sleep(50 * time.Millisecond)

	postings := idx.Lookup("go")
	if len(postings) != 6 {
		t.Errorf("expected 6 postings after background merge, got %d", len(postings))
	}
}

func TestMergePolicy_OnlyOneMergeConcurrently(t *testing.T) {
	a := newAnalyzer()
	// small MaxSegments so merge triggers often
	idx := New(
		WithFlushPolicy(&FlushPolicy{MaxTokens: 1}),
		WithMergePolicy(&MergePolicy{MaxSegments: 1}),
	)

	for i := 0; i < 20; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go", nil, a)
	}

	time.Sleep(100 * time.Millisecond)

	// just verify no panic and all docs findable
	postings := idx.Lookup("go")
	if len(postings) != 20 {
		t.Errorf("expected 20 postings, got %d", len(postings))
	}
}

func TestMergePolicy_ZeroDisabled(t *testing.T) {
	a := newAnalyzer()
	// flush every doc, no auto-merge
	idx := New(
		WithFlushPolicy(&FlushPolicy{MaxTokens: 1}),
		WithMergePolicy(&MergePolicy{MaxSegments: 0}),
	)

	for i := 0; i < 5; i++ {
		idx.Add(fmt.Sprintf("doc%d", i), "go", nil, a)
	}

	time.Sleep(20 * time.Millisecond)

	idx.mu.RLock()
	segs := len(idx.segments)
	idx.mu.RUnlock()

	if segs != 5 {
		t.Errorf("MaxSegments=0 should disable background merge; expected 5 segments, got %d", segs)
	}
}
