package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// pageDoc creates a doc with body text and a numeric price field for pagination tests.
func pageDoc(id, price string) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"body":  {Value: "item", Boost: 1.0},
			"price": {Value: price},
		},
	}
}

func searchPage(e *Engine, opts SearchOptions) SearchResult {
	q := query.NewBuilder().Must("body", "item").Build()
	return e.Search(q, 0, opts)
}

// engineWith5PriceDocs indexes docs with prices 10,20,30,40,50 and IDs 1-5.
func engineWith5PriceDocs() *Engine {
	e := New()
	e.Index(pageDoc("1", "10"))
	e.Index(pageDoc("2", "20"))
	e.Index(pageDoc("3", "30"))
	e.Index(pageDoc("4", "40"))
	e.Index(pageDoc("5", "50"))
	return e
}

// --- Offset pagination ---

func TestPagination_Size_LimitsResults(t *testing.T) {
	e := engineWith5PriceDocs()
	res := searchPage(e, SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		Size: 2,
	})
	if len(res.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(res.Hits))
	}
	if res.Hits[0].ID != "1" || res.Hits[1].ID != "2" {
		t.Errorf("expected [1 2], got %v", ids(res.Hits))
	}
}

func TestPagination_From_SkipsResults(t *testing.T) {
	e := engineWith5PriceDocs()
	res := searchPage(e, SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		From: 2,
	})
	if len(res.Hits) != 3 {
		t.Fatalf("expected 3 hits after skipping 2, got %d", len(res.Hits))
	}
	if res.Hits[0].ID != "3" {
		t.Errorf("expected first hit to be doc 3 (price=30), got %v", res.Hits[0].ID)
	}
}

func TestPagination_FromAndSize(t *testing.T) {
	e := engineWith5PriceDocs()
	res := searchPage(e, SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		From: 2,
		Size: 2,
	})
	if len(res.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(res.Hits))
	}
	if res.Hits[0].ID != "3" || res.Hits[1].ID != "4" {
		t.Errorf("expected [3 4], got %v", ids(res.Hits))
	}
}

func TestPagination_FromBeyondCount_ReturnsEmpty(t *testing.T) {
	e := engineWith5PriceDocs()
	res := searchPage(e, SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		From: 100,
	})
	if len(res.Hits) != 0 {
		t.Errorf("expected empty hits when From > count, got %v", ids(res.Hits))
	}
}

func TestPagination_SizeLargerThanResults(t *testing.T) {
	e := engineWith5PriceDocs()
	res := searchPage(e, SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		Size: 100,
	})
	if len(res.Hits) != 5 {
		t.Fatalf("expected all 5 results, got %d", len(res.Hits))
	}
}

func TestPagination_NoOptions_AllResults(t *testing.T) {
	e := engineWith5PriceDocs()
	q := query.NewBuilder().Must("body", "item").Build()
	res := e.Search(q, 0)
	if len(res.Hits) != 5 {
		t.Fatalf("expected 5 results with no pagination, got %d", len(res.Hits))
	}
}

func TestPagination_TopK_TakesPriority(t *testing.T) {
	e := engineWith5PriceDocs()
	q := query.NewBuilder().Must("body", "item").Build()
	res := e.Search(q, 3)
	if len(res.Hits) != 3 {
		t.Fatalf("expected topK=3 to limit to 3 results, got %d", len(res.Hits))
	}
}

// --- SearchAfter (keyset) ---

func TestPagination_SearchAfter_SecondPage(t *testing.T) {
	e := engineWith5PriceDocs()

	// first page
	page1 := searchPage(e, SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		Size: 2,
	})
	if len(page1.Hits) != 2 {
		t.Fatalf("expected 2 hits on page 1, got %d", len(page1.Hits))
	}
	if page1.Hits[0].ID != "1" || page1.Hits[1].ID != "2" {
		t.Errorf("page 1 expected [1 2], got %v", ids(page1.Hits))
	}

	// NextCursor should point to last doc on page 1
	if page1.NextCursor == nil {
		t.Fatal("expected NextCursor after page 1")
	}
	if page1.NextCursor.DocID != "2" {
		t.Errorf("NextCursor.DocID should be '2' (last doc on page 1), got %q", page1.NextCursor.DocID)
	}
	if page1.NextCursor.SortValue != "20" {
		t.Errorf("NextCursor.SortValue should be '20', got %q", page1.NextCursor.SortValue)
	}

	// second page using the cursor
	page2 := searchPage(e, SearchOptions{
		Sort:        []SortClause{{Field: "price", Order: Asc}},
		Size:        2,
		SearchAfter: page1.NextCursor,
	})
	if len(page2.Hits) != 2 {
		t.Fatalf("expected 2 hits on page 2, got %d", len(page2.Hits))
	}
	if page2.Hits[0].ID != "3" || page2.Hits[1].ID != "4" {
		t.Errorf("page 2 expected [3 4], got %v", ids(page2.Hits))
	}
}

func TestPagination_SearchAfter_NoPagesOverlap(t *testing.T) {
	e := engineWith5PriceDocs()

	opts := SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		Size: 2,
	}

	page1 := searchPage(e, opts)
	opts.SearchAfter = page1.NextCursor
	page2 := searchPage(e, opts)
	opts.SearchAfter = page2.NextCursor
	page3 := searchPage(e, opts)

	// collect all IDs across pages
	all := append(append(ids(page1.Hits), ids(page2.Hits)...), ids(page3.Hits)...)

	if len(all) != 5 {
		t.Fatalf("expected 5 total results across 3 pages, got %d: %v", len(all), all)
	}

	// check no duplicates
	seen := map[string]bool{}
	for _, id := range all {
		if seen[id] {
			t.Errorf("duplicate doc %q across pages", id)
		}
		seen[id] = true
	}
}

func TestPagination_SearchAfter_LastPage_NoNextCursor(t *testing.T) {
	e := engineWith5PriceDocs()

	// get all 5 docs, cursor at last
	page1 := searchPage(e, SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Asc}},
		Size: 5,
	})
	if len(page1.Hits) != 5 {
		t.Fatalf("expected 5 hits, got %d", len(page1.Hits))
	}

	// page after the last doc should be empty
	page2 := searchPage(e, SearchOptions{
		Sort:        []SortClause{{Field: "price", Order: Asc}},
		Size:        2,
		SearchAfter: page1.NextCursor,
	})
	if len(page2.Hits) != 0 {
		t.Errorf("expected no results after last doc, got %v", ids(page2.Hits))
	}
	if page2.NextCursor != nil {
		t.Errorf("expected nil NextCursor on empty page, got %+v", page2.NextCursor)
	}
}

func TestPagination_SearchAfter_DescendingOrder(t *testing.T) {
	e := engineWith5PriceDocs()

	page1 := searchPage(e, SearchOptions{
		Sort: []SortClause{{Field: "price", Order: Desc}},
		Size: 2,
	})
	// price desc: 50, 40, 30, 20, 10 — first page is [5, 4]
	if len(page1.Hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(page1.Hits))
	}
	if page1.Hits[0].ID != "5" || page1.Hits[1].ID != "4" {
		t.Errorf("page 1 desc expected [5 4], got %v", ids(page1.Hits))
	}

	page2 := searchPage(e, SearchOptions{
		Sort:        []SortClause{{Field: "price", Order: Desc}},
		Size:        2,
		SearchAfter: page1.NextCursor,
	})
	if len(page2.Hits) != 2 {
		t.Fatalf("expected 2 hits on page 2, got %d", len(page2.Hits))
	}
	if page2.Hits[0].ID != "3" || page2.Hits[1].ID != "2" {
		t.Errorf("page 2 desc expected [3 2], got %v", ids(page2.Hits))
	}
}
