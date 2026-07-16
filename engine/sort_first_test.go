package engine

import (
	"fmt"
	"testing"

	"github.com/noahfan/go-search/query"
)

// --- Sort-first fast path ---

func TestSortFirst_TopK_EarlyStop(t *testing.T) {
	// 5 docs; topK=2 → only 2 Bitcask fetches should happen (verified by correctness)
	e := New()
	for i := 1; i <= 5; i++ {
		e.Index(priceDoc(fmt.Sprintf("%d", i), "item", fmt.Sprintf("%d", i*10)))
	}
	// ascending: 1(10) 2(20) 3(30) 4(40) 5(50) → topK=2 returns [1 2]
	q := query.NewBuilder().Must("body", "item").Build()
	results := e.Search(q, 2, SortBy("price", Asc)).Hits
	got := ids(results)
	if len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Errorf("expected [1 2], got %v", got)
	}
}

func TestSortFirst_MissingFieldAppearsLast(t *testing.T) {
	e := New()
	e.Index(priceDoc("1", "item", "30"))
	e.Index(Document{
		ID:     "2",
		Fields: map[string]Field{"body": {Value: "item", Boost: 1.0}}, // no price
	})
	e.Index(priceDoc("3", "item", "10"))

	results := searchAll(e, "item", SortBy("price", Asc))
	got := ids(results)

	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %v", got)
	}
	// price-sorted first: 3(10) then 1(30); doc 2 (no price) at end
	if got[0] != "3" || got[1] != "1" || got[2] != "2" {
		t.Errorf("expected [3 1 2], got %v", got)
	}
}

func TestSortFirst_FromSize(t *testing.T) {
	e := New()
	for i := 1; i <= 5; i++ {
		e.Index(priceDoc(fmt.Sprintf("%d", i), "item", fmt.Sprintf("%d", i*10)))
	}
	// asc: [1 2 3 4 5]; page 2 (From=2, Size=2) → [3 4]
	q := query.NewBuilder().Must("body", "item").Build()
	opts := SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		From: 2,
		Size: 2,
	}
	results := e.Search(q, 0, opts).Hits
	got := ids(results)
	if len(got) != 2 || got[0] != "3" || got[1] != "4" {
		t.Errorf("expected [3 4], got %v", got)
	}
}

func TestSortFirst_SearchAfter_MultiPage(t *testing.T) {
	e := New()
	for i := 1; i <= 5; i++ {
		e.Index(priceDoc(fmt.Sprintf("%d", i), "item", fmt.Sprintf("%d", i*10)))
	}
	q := query.NewBuilder().Must("body", "item").Build()
	opts := SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		Size: 2,
	}

	sr1 := e.Search(q, 0, opts)
	got1 := ids(sr1.Hits)
	if len(got1) != 2 || got1[0] != "1" || got1[1] != "2" {
		t.Fatalf("page 1: expected [1 2], got %v", got1)
	}
	if sr1.NextCursor == nil {
		t.Fatal("expected NextCursor after page 1")
	}

	opts.SearchAfter = sr1.NextCursor
	sr2 := e.Search(q, 0, opts)
	got2 := ids(sr2.Hits)
	if len(got2) != 2 || got2[0] != "3" || got2[1] != "4" {
		t.Fatalf("page 2: expected [3 4], got %v", got2)
	}

	opts.SearchAfter = sr2.NextCursor
	sr3 := e.Search(q, 0, opts)
	got3 := ids(sr3.Hits)
	if len(got3) != 1 || got3[0] != "5" {
		t.Errorf("page 3: expected [5], got %v", got3)
	}
	if sr3.NextCursor == nil {
		t.Error("expected NextCursor on last page")
	}
}

func TestSortFirst_FallbackWhenFieldAbsent(t *testing.T) {
	// "nonexistent" has no entries in FieldIndex → falls back to e.Sort()
	// With no sort-field data all docs tie; result is sorted by BM25 score
	e := engineWithPricedDocs()
	results := searchAll(e, "item", SortBy("nonexistent", Asc))
	if len(results) != 3 {
		t.Errorf("expected 3 results even for absent sort field, got %d", len(results))
	}
}

func TestSortFirst_DescWithMissingField(t *testing.T) {
	e := New()
	e.Index(priceDoc("1", "item", "10"))
	e.Index(priceDoc("2", "item", "50"))
	e.Index(Document{
		ID:     "3",
		Fields: map[string]Field{"body": {Value: "item", Boost: 1.0}},
	})

	results := searchAll(e, "item", SortBy("price", Desc))
	got := ids(results)

	if len(got) != 3 {
		t.Fatalf("expected 3, got %v", got)
	}
	// desc: 2(50) 1(10) then 3 (no price) last
	if got[0] != "2" || got[1] != "1" || got[2] != "3" {
		t.Errorf("expected [2 1 3], got %v", got)
	}
}

func TestSortFirst_AllDocsHaveField(t *testing.T) {
	e := engineWithPricedDocs()
	results := searchAll(e, "item", SortBy("price", Asc))
	got := ids(results)
	// priceDoc: 1=30, 2=10, 3=20 → asc: [2 3 1]
	if len(got) != 3 || got[0] != "2" || got[1] != "3" || got[2] != "1" {
		t.Errorf("expected [2 3 1], got %v", got)
	}
}

func TestSortFirst_EmptyResult(t *testing.T) {
	e := engineWithPricedDocs()
	q := query.NewBuilder().Must("body", "nonexistent").Build()
	sr := e.Search(q, 10, SortBy("price", Asc))
	if len(sr.Hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(sr.Hits))
	}
	if sr.NextCursor != nil {
		t.Error("expected nil NextCursor for empty result")
	}
}

// engineWithPricedDocs creates an engine with 3 docs with known prices.
// Prices: doc1=30, doc2=10, doc3=20.
func engineWithPricedDocs() *Engine {
	e := New()
	e.Index(priceDoc("1", "item", "30"))
	e.Index(priceDoc("2", "item", "10"))
	e.Index(priceDoc("3", "item", "20"))
	return e
}
