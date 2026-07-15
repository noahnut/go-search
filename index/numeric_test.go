package index

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/noahfan/go-search/storage/memory"
)

func newNumericIndex() *NumericIndex {
	return NewNumericIndex(memory.New())
}

// --- Encoding unit tests ---

func TestEncodeFloat64_Order(t *testing.T) {
	cases := []float64{-100.0, -1.0, -0.5, 0.0, 0.5, 1.0, 100.0}
	for i := 1; i < len(cases); i++ {
		a, b := EncodeFloat64(cases[i-1]), EncodeFloat64(cases[i])
		if a >= b {
			t.Errorf("EncodeFloat64(%v) >= EncodeFloat64(%v): %d >= %d", cases[i-1], cases[i], a, b)
		}
	}
}

func TestEncodeFloat64_RoundTrip(t *testing.T) {
	for _, v := range []float64{-1e308, -1.5, -0.0, 0.0, 1.5, 1e308} {
		if got := DecodeFloat64(EncodeFloat64(v)); got != v {
			t.Errorf("round-trip %v → %v", v, got)
		}
	}
}

func TestEncodeFloat64_NegativeOrderFix(t *testing.T) {
	// -2.0 < -1.0 must hold as uint64
	if EncodeFloat64(-2.0) >= EncodeFloat64(-1.0) {
		t.Error("EncodeFloat64(-2.0) should be < EncodeFloat64(-1.0)")
	}
}

func TestEncodeInt64_Bounds(t *testing.T) {
	if EncodeInt64(math.MinInt64) != 0 {
		t.Errorf("EncodeInt64(MinInt64) should be 0, got %d", EncodeInt64(math.MinInt64))
	}
	if EncodeInt64(math.MaxInt64) != math.MaxUint64 {
		t.Errorf("EncodeInt64(MaxInt64) should be MaxUint64, got %d", EncodeInt64(math.MaxInt64))
	}
}

func TestEncodeInt64_Order(t *testing.T) {
	cases := []int64{math.MinInt64, -100, -1, 0, 1, 100, math.MaxInt64}
	for i := 1; i < len(cases); i++ {
		a, b := EncodeInt64(cases[i-1]), EncodeInt64(cases[i])
		if a >= b {
			t.Errorf("EncodeInt64(%v) >= EncodeInt64(%v)", cases[i-1], cases[i])
		}
	}
}

func TestEncodeInt64_RoundTrip(t *testing.T) {
	for _, v := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
		if got := DecodeInt64(EncodeInt64(v)); got != v {
			t.Errorf("round-trip %v → %v", v, got)
		}
	}
}

// large int64 > 2^53 — float64 would lose precision here
func TestEncodeInt64_LargeValue(t *testing.T) {
	// 2^60 — safely > 2^53, float64 cannot represent it exactly
	large := int64(1) << 60
	enc := EncodeInt64(large)
	if DecodeInt64(enc) != large {
		t.Errorf("large int64 round-trip failed: %d", large)
	}
	if EncodeInt64(large-1) >= enc {
		t.Error("EncodeInt64(large-1) should be < EncodeInt64(large)")
	}
}

// --- Block encode/decode ---

func TestBlockRoundTrip(t *testing.T) {
	entries := []numericEntry{
		{value: EncodeFloat64(1.0), docID: "doc-1"},
		{value: EncodeFloat64(2.0), docID: "doc-2"},
		{value: EncodeFloat64(3.0), docID: "doc-3"},
	}
	data := encodeNumericBlock(entries)
	got, err := decodeNumericBlock(data)
	if err != nil {
		t.Fatalf("decodeNumericBlock error: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(got))
	}
	for i, e := range entries {
		if got[i].value != e.value || got[i].docID != e.docID {
			t.Errorf("entry %d: want {%d %q}, got {%d %q}", i, e.value, e.docID, got[i].value, got[i].docID)
		}
	}
}

func TestBlockRoundTrip_LongDocID(t *testing.T) {
	// docIDs longer than 16 bytes must survive encode/decode intact
	longID := "doc-1234567890abcdef"  // 20 bytes — longer than the 16-byte fixed slot
	entries := []numericEntry{{value: EncodeFloat64(1.0), docID: longID}}
	data := encodeNumericBlock(entries)
	got, err := decodeNumericBlock(data)
	if err != nil {
		t.Fatalf("decodeNumericBlock error: %v", err)
	}
	if got[0].docID != longID {
		t.Errorf("long docID corrupted: want %q, got %q", longID, got[0].docID)
	}
}

// --- Buffer-only Range (before Flush) ---

func TestRange_BufferOnly(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("price", "1", 10.0)
	n.AddFloat("price", "2", 50.0)
	n.AddFloat("price", "3", 90.0)

	lo, hi := 20.0, 80.0
	got := n.Range("price", nil, nil, &lo, &hi) // (20, 80) exclusive
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

func TestRange_BufferOnly_Inclusive(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("price", "1", 10.0)
	n.AddFloat("price", "2", 50.0)
	n.AddFloat("price", "3", 90.0)

	lo, hi := 10.0, 50.0
	got := n.Range("price", &lo, &hi, nil, nil) // [10, 50] inclusive
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %v", got)
	}
}

// --- After Flush (tree-based Range) ---

func TestRange_AfterFlush(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("price", "1", 10.0)
	n.AddFloat("price", "2", 50.0)
	n.AddFloat("price", "3", 90.0)
	n.Flush()

	lo, hi := 10.0, 50.0
	got := n.Range("price", &lo, &hi, nil, nil)
	sort.Strings(got)
	if len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Errorf("expected [1 2], got %v", got)
	}
}

func TestRange_AfterFlush_NoMatch(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("price", "1", 10.0)
	n.Flush()

	lo, hi := 100.0, 200.0
	got := n.Range("price", &lo, &hi, nil, nil)
	if len(got) != 0 {
		t.Errorf("expected no results, got %v", got)
	}
}

func TestRange_OpenEnded_NoUpperBound(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("score", "1", 3.0)
	n.AddFloat("score", "2", 4.5)
	n.AddFloat("score", "3", 5.0)
	n.Flush()

	lo := 4.5
	got := n.Range("score", &lo, nil, nil, nil)
	sort.Strings(got)
	if len(got) != 2 || got[0] != "2" || got[1] != "3" {
		t.Errorf("expected [2 3], got %v", got)
	}
}

func TestRange_NegativeValues(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("temp", "1", -20.0)
	n.AddFloat("temp", "2", -5.0)
	n.AddFloat("temp", "3", 10.0)
	n.Flush()

	lo, hi := -10.0, 0.0
	got := n.Range("temp", &lo, &hi, nil, nil)
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

// --- Delete ---

func TestDelete_BeforeFlush(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("price", "1", 10.0)
	n.AddFloat("price", "2", 20.0)
	n.Delete("1")

	lo, hi := 0.0, 100.0
	got := n.Range("price", &lo, &hi, nil, nil)
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2] after delete, got %v", got)
	}
}

func TestDelete_AfterFlush(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("price", "1", 10.0)
	n.AddFloat("price", "2", 20.0)
	n.Flush()
	n.Delete("1")

	lo, hi := 0.0, 100.0
	got := n.Range("price", &lo, &hi, nil, nil)
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2] after delete+flush, got %v", got)
	}
}

func TestDelete_CompactedOnNextFlush(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("price", "1", 10.0)
	n.AddFloat("price", "2", 20.0)
	n.Flush()
	n.Delete("1")
	n.Flush() // compact removes "1" from on-disk blocks

	lo, hi := 0.0, 100.0
	got := n.Range("price", &lo, &hi, nil, nil)
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2] after compacting flush, got %v", got)
	}
}

// --- AddInt ---

func TestRange_IntField(t *testing.T) {
	n := newNumericIndex()
	n.AddInt("timestamp", "1", 1000)
	n.AddInt("timestamp", "2", 2000)
	n.AddInt("timestamp", "3", 3000)
	n.Flush()

	lo, hi := 1500.0, 2500.0
	got := n.Range("timestamp", &lo, &hi, nil, nil)
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

// --- Multiple flushes ---

func TestRange_MultipleFlushes(t *testing.T) {
	n := newNumericIndex()
	n.AddFloat("price", "1", 10.0)
	n.Flush()
	n.AddFloat("price", "2", 20.0)
	n.Flush()
	n.AddFloat("price", "3", 30.0)
	n.Flush()

	lo, hi := 0.0, 100.0
	got := n.Range("price", &lo, &hi, nil, nil)
	sort.Strings(got)
	if len(got) != 3 {
		t.Errorf("expected 3 results across multiple flushes, got %v", got)
	}
}

// --- Large dataset: tree structure and pruning ---

func TestRange_LargeDataset_TreePruning(t *testing.T) {
	n := newNumericIndex()
	// insert 2×blockSize entries to force at least 2 leaf blocks and an internal node
	for i := 0; i < blockSize*2; i++ {
		n.AddFloat("val", fmt.Sprintf("doc-%d", i), float64(i))
	}
	n.Flush()

	// query only the lower half
	lo, hi := 0.0, float64(blockSize)-1
	got := n.Range("val", &lo, &hi, nil, nil)
	if len(got) != blockSize {
		t.Errorf("expected %d results, got %d", blockSize, len(got))
	}

	// tree must have an internal root (two leaf blocks were created)
	root := n.roots["val"]
	if root == nil {
		t.Fatal("root is nil after flush")
	}
	if root.blockKey != "" {
		t.Error("expected internal node at root for 2×blockSize entries")
	}
}

// --- Race detector ---

func TestNumericIndex_Race(t *testing.T) {
	n := newNumericIndex()
	done := make(chan struct{})

	go func() {
		for i := 0; i < 50; i++ {
			n.AddFloat("price", fmt.Sprintf("doc-%d", i), float64(i))
		}
		done <- struct{}{}
	}()
	go func() {
		lo, hi := 0.0, 100.0
		for i := 0; i < 50; i++ {
			n.Range("price", &lo, &hi, nil, nil)
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}
