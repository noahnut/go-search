package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// numDoc creates a document with a text body field and a numeric field.
func numDoc(id, body, field, value string) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"body":  {Value: body},
			field:   {Value: value},
		},
	}
}

// --- passesRangeFilters unit tests ---

func TestPassesRangeFilters_EmptyRanges(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "50"}}}
	if !passesRangeFilters(doc, nil) {
		t.Error("empty ranges should always pass")
	}
}

func TestPassesRangeFilters_Gte_Pass(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "10"}}}
	ranges := []query.RangeClause{{Field: "price", Gte: query.Ptr(10)}}
	if !passesRangeFilters(doc, ranges) {
		t.Error("value == Gte should pass (inclusive)")
	}
}

func TestPassesRangeFilters_Gte_Fail(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "9.99"}}}
	ranges := []query.RangeClause{{Field: "price", Gte: query.Ptr(10)}}
	if passesRangeFilters(doc, ranges) {
		t.Error("value < Gte should fail")
	}
}

func TestPassesRangeFilters_Lte_Pass(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "100"}}}
	ranges := []query.RangeClause{{Field: "price", Lte: query.Ptr(100)}}
	if !passesRangeFilters(doc, ranges) {
		t.Error("value == Lte should pass (inclusive)")
	}
}

func TestPassesRangeFilters_Lte_Fail(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "100.01"}}}
	ranges := []query.RangeClause{{Field: "price", Lte: query.Ptr(100)}}
	if passesRangeFilters(doc, ranges) {
		t.Error("value > Lte should fail")
	}
}

func TestPassesRangeFilters_Gt_ExcludesBoundary(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"score": {Value: "5"}}}
	ranges := []query.RangeClause{{Field: "score", Gt: query.Ptr(5)}}
	if passesRangeFilters(doc, ranges) {
		t.Error("value == Gt should fail (exclusive)")
	}
}

func TestPassesRangeFilters_Gt_Pass(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"score": {Value: "5.1"}}}
	ranges := []query.RangeClause{{Field: "score", Gt: query.Ptr(5)}}
	if !passesRangeFilters(doc, ranges) {
		t.Error("value > Gt should pass")
	}
}

func TestPassesRangeFilters_Lt_ExcludesBoundary(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"score": {Value: "5"}}}
	ranges := []query.RangeClause{{Field: "score", Lt: query.Ptr(5)}}
	if passesRangeFilters(doc, ranges) {
		t.Error("value == Lt should fail (exclusive)")
	}
}

func TestPassesRangeFilters_Lt_Pass(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"score": {Value: "4.9"}}}
	ranges := []query.RangeClause{{Field: "score", Lt: query.Ptr(5)}}
	if !passesRangeFilters(doc, ranges) {
		t.Error("value < Lt should pass")
	}
}

func TestPassesRangeFilters_BothBounds_InRange(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "50"}}}
	ranges := []query.RangeClause{{Field: "price", Gte: query.Ptr(10), Lte: query.Ptr(100)}}
	if !passesRangeFilters(doc, ranges) {
		t.Error("value inside [Gte, Lte] should pass")
	}
}

func TestPassesRangeFilters_BothBounds_BelowRange(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "5"}}}
	ranges := []query.RangeClause{{Field: "price", Gte: query.Ptr(10), Lte: query.Ptr(100)}}
	if passesRangeFilters(doc, ranges) {
		t.Error("value below Gte should fail")
	}
}

func TestPassesRangeFilters_BothBounds_AboveRange(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "200"}}}
	ranges := []query.RangeClause{{Field: "price", Gte: query.Ptr(10), Lte: query.Ptr(100)}}
	if passesRangeFilters(doc, ranges) {
		t.Error("value above Lte should fail")
	}
}

func TestPassesRangeFilters_FieldAbsent(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"body": {Value: "hello"}}}
	ranges := []query.RangeClause{{Field: "price", Gte: query.Ptr(10)}}
	if passesRangeFilters(doc, ranges) {
		t.Error("missing field should fail")
	}
}

func TestPassesRangeFilters_NonNumericField(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{"price": {Value: "cheap"}}}
	ranges := []query.RangeClause{{Field: "price", Gte: query.Ptr(10)}}
	if passesRangeFilters(doc, ranges) {
		t.Error("non-numeric value should fail")
	}
}

func TestPassesRangeFilters_MultipleRanges_AllMustPass(t *testing.T) {
	doc := Document{ID: "1", Fields: map[string]Field{
		"price": {Value: "50"},
		"score": {Value: "4.2"},
	}}
	ranges := []query.RangeClause{
		{Field: "price", Gte: query.Ptr(10), Lte: query.Ptr(100)},
		{Field: "score", Gte: query.Ptr(4.5)}, // score 4.2 < 4.5 → fail
	}
	if passesRangeFilters(doc, ranges) {
		t.Error("all range clauses must pass; one failure should exclude the doc")
	}
}

// --- Search integration tests ---

func TestRange_InclusiveBounds(t *testing.T) {
	e := New()
	e.Index(numDoc("1", "laptop", "price", "10"))
	e.Index(numDoc("2", "laptop", "price", "50"))
	e.Index(numDoc("3", "laptop", "price", "100"))
	e.Index(numDoc("4", "laptop", "price", "200"))

	q := query.NewBuilder().
		Must("body", "laptop").
		Range("price", query.Ptr(10), query.Ptr(100)).
		Build()

	results := e.Search(q, 10)
	got := ids(results)

	if len(got) != 3 {
		t.Fatalf("expected 3 results (price 10, 50, 100), got %v", got)
	}
	for _, id := range []string{"1", "2", "3"} {
		found := false
		for _, g := range got {
			if g == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected doc %q in results, got %v", id, got)
		}
	}
}

func TestRange_OpenLowerBound(t *testing.T) {
	e := New()
	e.Index(numDoc("1", "item", "score", "3.0"))
	e.Index(numDoc("2", "item", "score", "4.5"))
	e.Index(numDoc("3", "item", "score", "5.0"))

	q := query.NewBuilder().
		Must("body", "item").
		Range("score", query.Ptr(4.5), nil). // score >= 4.5, no upper bound
		Build()

	results := e.Search(q, 10)
	got := ids(results)
	if len(got) != 2 {
		t.Fatalf("expected docs 2 and 3 (score >= 4.5), got %v", got)
	}
}

func TestRange_OpenUpperBound(t *testing.T) {
	e := New()
	e.Index(numDoc("1", "item", "price", "5"))
	e.Index(numDoc("2", "item", "price", "15"))
	e.Index(numDoc("3", "item", "price", "50"))

	q := query.NewBuilder().
		Must("body", "item").
		Range("price", nil, query.Ptr(10)). // no lower bound, price <= 10
		Build()

	results := e.Search(q, 10)
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("expected only doc 1 (price <= 10), got %v", got)
	}
}

func TestRange_CombinedWithMust(t *testing.T) {
	e := New()
	e.Index(numDoc("1", "go tutorial", "price", "20"))
	e.Index(numDoc("2", "python tutorial", "price", "20"))
	e.Index(numDoc("3", "go tutorial", "price", "200"))

	q := query.NewBuilder().
		Must("body", "go").
		Range("price", nil, query.Ptr(50)).
		Build()

	results := e.Search(q, 10)
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("expected only doc 1 (body:go AND price<=50), got %v", got)
	}
}

func TestRange_ExcludesAll(t *testing.T) {
	e := New()
	e.Index(numDoc("1", "item", "price", "200"))
	e.Index(numDoc("2", "item", "price", "300"))

	q := query.NewBuilder().
		Must("body", "item").
		Range("price", nil, query.Ptr(10)).
		Build()

	results := e.Search(q, 10)
	if len(results) != 0 {
		t.Errorf("expected no results, got %v", ids(results))
	}
}

func TestRange_MissingFieldExcludesDoc(t *testing.T) {
	e := New()
	e.Index(numDoc("1", "item", "price", "50"))
	e.Index(Document{ // no price field
		ID:     "2",
		Fields: map[string]Field{"body": {Value: "item"}},
	})

	q := query.NewBuilder().
		Must("body", "item").
		Range("price", query.Ptr(10), query.Ptr(100)).
		Build()

	results := e.Search(q, 10)
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("expected only doc with price field, got %v", got)
	}
}

func TestRange_MultipleRangeClauses(t *testing.T) {
	e := New()
	e.Index(Document{ID: "1", Fields: map[string]Field{
		"body":  {Value: "item"},
		"price": {Value: "50"},
		"score": {Value: "4.8"},
	}})
	e.Index(Document{ID: "2", Fields: map[string]Field{
		"body":  {Value: "item"},
		"price": {Value: "50"},
		"score": {Value: "3.0"}, // fails score range
	}})
	e.Index(Document{ID: "3", Fields: map[string]Field{
		"body":  {Value: "item"},
		"price": {Value: "200"}, // fails price range
		"score": {Value: "4.8"},
	}})

	q := query.NewBuilder().
		Must("body", "item").
		Range("price", query.Ptr(10), query.Ptr(100)).
		Range("score", query.Ptr(4.0), nil).
		Build()

	results := e.Search(q, 10)
	got := ids(results)
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("expected only doc 1 (both ranges satisfied), got %v", got)
	}
}

func TestRange_ExclusiveBounds_ViaDirectConstruction(t *testing.T) {
	e := New()
	e.Index(numDoc("1", "item", "score", "5.0")) // exactly 5 — excluded by Gt/Lt
	e.Index(numDoc("2", "item", "score", "5.1"))
	e.Index(numDoc("3", "item", "score", "4.9"))

	// Use RangeClause directly to set Gt and Lt (Builder only exposes Gte/Lte)
	q := query.Query{
		Clauses: []query.Clause{{Field: "body", Term: "item", Type: query.Must}},
		Ranges:  []query.RangeClause{{Field: "score", Gt: query.Ptr(5.0)}},
	}

	results := e.Search(q, 10)
	got := ids(results)
	if len(got) != 1 || got[0] != "2" {
		t.Fatalf("expected only doc 2 (score > 5.0 exclusive), got %v", got)
	}
}
