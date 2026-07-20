package index

import (
	"fmt"
	"sort"
	"testing"

	"github.com/noahfan/go-search/storage/memory"
)

// --- helpers ---

func fp(v float64) *float64 { return &v }

func sortedIDs(ids []string) []string {
	out := make([]string, len(ids))
	copy(out, ids)
	sort.Strings(out)
	return out
}

func newKDIndex() *Index {
	return New(memory.New())
}

// --- KDTree.Build and MultiRange ---

func TestKDTree_Build_Empty(t *testing.T) {
	tree := NewKDTree()
	tree.Build(nil)
	got := tree.MultiRange([]FieldBound{{Field: "price", Gte: fp(0), Lte: fp(100)}})
	if len(got) != 0 {
		t.Errorf("expected empty result for empty tree, got %v", got)
	}
}

func TestKDTree_MultiRange_1D_Basic(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10}},
		{DocID: "2", Values: map[string]float64{"price": 50}},
		{DocID: "3", Values: map[string]float64{"price": 90}},
	})

	got := tree.MultiRange([]FieldBound{{Field: "price", Gte: fp(20), Lte: fp(80)}})
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

func TestKDTree_MultiRange_1D_Inclusive(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10}},
		{DocID: "2", Values: map[string]float64{"price": 50}},
		{DocID: "3", Values: map[string]float64{"price": 90}},
	})

	got := sortedIDs(tree.MultiRange([]FieldBound{{Field: "price", Gte: fp(10), Lte: fp(50)}}))
	if len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Errorf("expected [1 2], got %v", got)
	}
}

func TestKDTree_MultiRange_ExclusiveBounds(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10}},
		{DocID: "2", Values: map[string]float64{"price": 50}},
		{DocID: "3", Values: map[string]float64{"price": 90}},
	})

	// (10, 90) exclusive — only 50 matches
	got := tree.MultiRange([]FieldBound{{Field: "price", Gt: fp(10), Lt: fp(90)}})
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

func TestKDTree_MultiRange_OpenUpperBound(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"score": 3.0}},
		{DocID: "2", Values: map[string]float64{"score": 4.5}},
		{DocID: "3", Values: map[string]float64{"score": 5.0}},
	})

	got := sortedIDs(tree.MultiRange([]FieldBound{{Field: "score", Gte: fp(4.5)}}))
	if len(got) != 2 || got[0] != "2" || got[1] != "3" {
		t.Errorf("expected [2 3], got %v", got)
	}
}

func TestKDTree_MultiRange_OpenLowerBound(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10}},
		{DocID: "2", Values: map[string]float64{"price": 50}},
		{DocID: "3", Values: map[string]float64{"price": 90}},
	})

	got := sortedIDs(tree.MultiRange([]FieldBound{{Field: "price", Lte: fp(50)}}))
	if len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Errorf("expected [1 2], got %v", got)
	}
}

func TestKDTree_MultiRange_NoMatch(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10}},
		{DocID: "2", Values: map[string]float64{"price": 20}},
	})

	got := tree.MultiRange([]FieldBound{{Field: "price", Gte: fp(100), Lte: fp(200)}})
	if len(got) != 0 {
		t.Errorf("expected no results, got %v", got)
	}
}

func TestKDTree_MultiRange_NegativeValues(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"temp": -20}},
		{DocID: "2", Values: map[string]float64{"temp": -5}},
		{DocID: "3", Values: map[string]float64{"temp": 10}},
	})

	got := tree.MultiRange([]FieldBound{{Field: "temp", Gte: fp(-10), Lte: fp(0)}})
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

// --- Multi-dimensional queries ---

func TestKDTree_MultiRange_2D_Intersection(t *testing.T) {
	// price in [10, 100] AND rating in [4.0, 5.0] → only doc "2" satisfies both
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 50, "rating": 3.0}},  // rating too low
		{DocID: "2", Values: map[string]float64{"price": 80, "rating": 4.5}},  // both match
		{DocID: "3", Values: map[string]float64{"price": 200, "rating": 4.8}}, // price too high
	})

	got := tree.MultiRange([]FieldBound{
		{Field: "price", Gte: fp(10), Lte: fp(100)},
		{Field: "rating", Gte: fp(4.0), Lte: fp(5.0)},
	})
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

func TestKDTree_MultiRange_2D_AllMatch(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10, "rating": 4.0}},
		{DocID: "2", Values: map[string]float64{"price": 50, "rating": 4.5}},
		{DocID: "3", Values: map[string]float64{"price": 90, "rating": 5.0}},
	})

	got := sortedIDs(tree.MultiRange([]FieldBound{
		{Field: "price", Gte: fp(0), Lte: fp(100)},
		{Field: "rating", Gte: fp(4.0), Lte: fp(5.0)},
	}))
	if len(got) != 3 {
		t.Errorf("expected 3 results, got %v", got)
	}
}

func TestKDTree_MultiRange_MissingFieldExcludesDoc(t *testing.T) {
	// doc "2" has no "rating" field — must not match a rating bound
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 50, "rating": 4.5}},
		{DocID: "2", Values: map[string]float64{"price": 50}},
	})

	got := tree.MultiRange([]FieldBound{
		{Field: "price", Gte: fp(0), Lte: fp(100)},
		{Field: "rating", Gte: fp(4.0), Lte: fp(5.0)},
	})
	if len(got) != 1 || got[0] != "1" {
		t.Errorf("expected [1], got %v", got)
	}
}

// --- Large dataset (forces internal nodes and pruning) ---

func TestKDTree_LargeDataset_1D(t *testing.T) {
	const n = kdLeafSize*3 + 1 // forces multiple leaf nodes
	entries := make([]KDEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = KDEntry{
			DocID:  fmt.Sprintf("doc-%d", i),
			Values: map[string]float64{"val": float64(i)},
		}
	}
	tree := NewKDTree()
	tree.Build(entries)

	lo, hi := float64(kdLeafSize), float64(kdLeafSize*2-1)
	got := tree.MultiRange([]FieldBound{{Field: "val", Gte: &lo, Lte: &hi}})
	if len(got) != kdLeafSize {
		t.Errorf("expected %d results, got %d", kdLeafSize, len(got))
	}
}

func TestKDTree_LargeDataset_2D(t *testing.T) {
	const n = kdLeafSize * 4
	entries := make([]KDEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = KDEntry{
			DocID: fmt.Sprintf("doc-%d", i),
			Values: map[string]float64{
				"x": float64(i % 100),
				"y": float64(i / 100),
			},
		}
	}
	tree := NewKDTree()
	tree.Build(entries)

	// x in [0, 9], y in [0, 0] → first 10 entries
	got := tree.MultiRange([]FieldBound{
		{Field: "x", Gte: fp(0), Lte: fp(9)},
		{Field: "y", Gte: fp(0), Lte: fp(0)},
	})
	if len(got) != 10 {
		t.Errorf("expected 10 results, got %d: %v", len(got), got)
	}
}

// --- Insert ---

func TestKDTree_Insert_AfterBuild(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10}},
		{DocID: "2", Values: map[string]float64{"price": 50}},
	})

	tree.Insert(KDEntry{DocID: "3", Values: map[string]float64{"price": 90}})

	got := sortedIDs(tree.MultiRange([]FieldBound{{Field: "price", Gte: fp(0), Lte: fp(100)}}))
	if len(got) != 3 {
		t.Errorf("expected 3 results after Insert, got %v", got)
	}
}

func TestKDTree_Insert_IntoEmptyTree(t *testing.T) {
	tree := NewKDTree()
	tree.Insert(KDEntry{DocID: "1", Values: map[string]float64{"price": 42}})

	got := tree.MultiRange([]FieldBound{{Field: "price", Gte: fp(40), Lte: fp(50)}})
	if len(got) != 1 || got[0] != "1" {
		t.Errorf("expected [1], got %v", got)
	}
}

// --- Delete ---

func TestKDTree_Delete_AfterBuild(t *testing.T) {
	tree := NewKDTree()
	tree.Build([]KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10}},
		{DocID: "2", Values: map[string]float64{"price": 50}},
	})

	tree.Delete("1")
	got := tree.MultiRange([]FieldBound{{Field: "price", Gte: fp(0), Lte: fp(100)}})
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2] after Delete, got %v", got)
	}
}

func TestKDTree_Delete_CompactedOnRebuild(t *testing.T) {
	tree := NewKDTree()
	entries := []KDEntry{
		{DocID: "1", Values: map[string]float64{"price": 10}},
		{DocID: "2", Values: map[string]float64{"price": 50}},
	}
	tree.Build(entries)
	tree.Delete("1")

	// Rebuild excludes the deleted entry
	tree.Build([]KDEntry{entries[1]})
	got := tree.MultiRange([]FieldBound{{Field: "price", Gte: fp(0), Lte: fp(100)}})
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2] after rebuild, got %v", got)
	}
}

// --- Index-level tests ---

func TestKDIndex_RangeQuery_BeforeFlush(t *testing.T) {
	// Linear scan fallback (tree not built yet)
	idx := newKDIndex()
	idx.AddNumeric("price", "1", 10.0)
	idx.AddNumeric("price", "2", 50.0)
	idx.AddNumeric("price", "3", 90.0)

	got := sortedIDs(idx.RangeQuery("price", fp(20), fp(80), nil, nil))
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

func TestKDIndex_RangeQuery_AfterFlush(t *testing.T) {
	idx := newKDIndex()
	idx.AddNumeric("price", "1", 10.0)
	idx.AddNumeric("price", "2", 50.0)
	idx.AddNumeric("price", "3", 90.0)
	idx.Flush()

	got := sortedIDs(idx.RangeQuery("price", fp(10), fp(50), nil, nil))
	if len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Errorf("expected [1 2], got %v", got)
	}
}

func TestKDIndex_RangeQuery_MultipleFlushes(t *testing.T) {
	idx := newKDIndex()
	idx.AddNumeric("price", "1", 10.0)
	idx.Flush()
	idx.AddNumeric("price", "2", 20.0)
	idx.Flush()
	idx.AddNumeric("price", "3", 30.0)
	idx.Flush()

	got := sortedIDs(idx.RangeQuery("price", fp(0), fp(100), nil, nil))
	if len(got) != 3 {
		t.Errorf("expected 3 results across flushes, got %v", got)
	}
}

func TestKDIndex_AddNumericInt(t *testing.T) {
	idx := newKDIndex()
	idx.AddNumericInt("ts", "1", 1000)
	idx.AddNumericInt("ts", "2", 2000)
	idx.AddNumericInt("ts", "3", 3000)

	got := idx.RangeQuery("ts", fp(1500), fp(2500), nil, nil)
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

func TestKDIndex_MultiRangeQuery_Intersection(t *testing.T) {
	idx := newKDIndex()
	idx.AddNumeric("price", "1", 50)
	idx.AddNumeric("rating", "1", 3.0) // rating too low

	idx.AddNumeric("price", "2", 80)
	idx.AddNumeric("rating", "2", 4.5) // both match

	idx.AddNumeric("price", "3", 200)
	idx.AddNumeric("rating", "3", 4.8) // price too high

	got := idx.MultiRangeQuery([]FieldBound{
		{Field: "price", Gte: fp(10), Lte: fp(100)},
		{Field: "rating", Gte: fp(4.0), Lte: fp(5.0)},
	})
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2], got %v", got)
	}
}

func TestKDIndex_MultiField_SameDoc(t *testing.T) {
	// Multiple AddNumeric calls for the same doc must produce one complete entry
	idx := newKDIndex()
	idx.AddNumeric("price", "1", 50)
	idx.AddNumeric("rating", "1", 4.5)

	got := idx.MultiRangeQuery([]FieldBound{
		{Field: "price", Gte: fp(0), Lte: fp(100)},
		{Field: "rating", Gte: fp(4.0), Lte: fp(5.0)},
	})
	if len(got) != 1 || got[0] != "1" {
		t.Errorf("expected [1], got %v", got)
	}
}

func TestKDIndex_DeleteNumeric(t *testing.T) {
	idx := newKDIndex()
	idx.AddNumeric("price", "1", 10.0)
	idx.AddNumeric("price", "2", 50.0)
	idx.DeleteNumeric("1")

	got := idx.RangeQuery("price", fp(0), fp(100), nil, nil)
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2] after delete, got %v", got)
	}
}

func TestKDIndex_DeleteNumeric_AfterFlush(t *testing.T) {
	idx := newKDIndex()
	idx.AddNumeric("price", "1", 10.0)
	idx.AddNumeric("price", "2", 50.0)
	idx.Flush()
	idx.DeleteNumeric("1")

	got := idx.RangeQuery("price", fp(0), fp(100), nil, nil)
	if len(got) != 1 || got[0] != "2" {
		t.Errorf("expected [2] after delete+flush, got %v", got)
	}
}

// --- Race detector ---

func TestKDTree_Race(t *testing.T) {
	idx := newKDIndex()
	done := make(chan struct{})

	go func() {
		for i := 0; i < 50; i++ {
			idx.AddNumeric("price", fmt.Sprintf("doc-%d", i), float64(i))
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 50; i++ {
			idx.RangeQuery("price", fp(0), fp(100), nil, nil)
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}
