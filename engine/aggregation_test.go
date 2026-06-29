package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// langDoc builds a document with a searchable body and a raw "language" field for aggregation.
func langDoc(id, body, language string) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"body":     {Value: body, Boost: 1.0},
			"language": {Value: language, Boost: 1.0},
		},
	}
}

func TestAggregate_BasicCount(t *testing.T) {
	e := New()
	e.Index(langDoc("1", "concurrency patterns", "go"))
	e.Index(langDoc("2", "web frameworks", "go"))
	e.Index(langDoc("3", "data science", "python"))

	q := query.NewBuilder().Should("body", "concurrency").Should("body", "web").Should("body", "data").Build()
	agg := e.Aggregate(q, "language", 10)

	counts := map[string]int{}
	for _, b := range agg.Buckets {
		counts[b.Key] = b.Count
	}

	if counts["go"] != 2 {
		t.Errorf("expected go=2, got %d", counts["go"])
	}
	if counts["python"] != 1 {
		t.Errorf("expected python=1, got %d", counts["python"])
	}
}

func TestAggregate_SortedByCountDescending(t *testing.T) {
	e := New()
	e.Index(langDoc("1", "goroutines", "go"))
	e.Index(langDoc("2", "channels", "go"))
	e.Index(langDoc("3", "interfaces", "go"))
	e.Index(langDoc("4", "decorators", "python"))

	q := query.NewBuilder().
		Should("body", "goroutines").
		Should("body", "channels").
		Should("body", "interfaces").
		Should("body", "decorators").
		Build()
	agg := e.Aggregate(q, "language", 10)

	if len(agg.Buckets) < 2 {
		t.Fatalf("expected at least 2 buckets, got %d", len(agg.Buckets))
	}
	if agg.Buckets[0].Key != "go" {
		t.Errorf("expected 'go' first (count 3), got '%s'", agg.Buckets[0].Key)
	}
	if agg.Buckets[0].Count < agg.Buckets[1].Count {
		t.Errorf("buckets not sorted: [0].Count=%d < [1].Count=%d", agg.Buckets[0].Count, agg.Buckets[1].Count)
	}
}

func TestAggregate_OnlyMatchingDocsIncluded(t *testing.T) {
	// doc "3" matches the query but doc "4" does not — "rust" should not appear in buckets
	e := New()
	e.Index(langDoc("1", "goroutines in go", "go"))
	e.Index(langDoc("2", "channels in go", "go"))
	e.Index(langDoc("3", "python generators", "python"))
	e.Index(langDoc("4", "rust ownership", "rust")) // does not match the query

	q := query.NewBuilder().Must("body", "go").Build()
	agg := e.Aggregate(q, "language", 10)

	for _, b := range agg.Buckets {
		if b.Key == "rust" {
			t.Error("'rust' doc did not match the query and should not appear in aggregation")
		}
		if b.Key == "python" {
			t.Error("'python' doc did not match the query and should not appear in aggregation")
		}
	}
}

func TestAggregate_MissingFieldSkipped(t *testing.T) {
	// doc "2" has no "language" field — it should not contribute to any bucket
	e := New()
	e.Index(langDoc("1", "goroutines", "go"))
	e.Index(Document{
		ID:     "2",
		Fields: map[string]Field{"body": {Value: "channels", Boost: 1.0}},
	})

	q := query.NewBuilder().Should("body", "goroutines").Should("body", "channels").Build()
	agg := e.Aggregate(q, "language", 10)

	totalCount := 0
	for _, b := range agg.Buckets {
		totalCount += b.Count
	}
	if totalCount != 1 {
		t.Errorf("doc without 'language' field should be skipped, total count should be 1, got %d", totalCount)
	}
}

func TestAggregate_TopKLimitsBuckets(t *testing.T) {
	e := New()
	e.Index(langDoc("1", "go code", "go"))
	e.Index(langDoc("2", "python code", "python"))
	e.Index(langDoc("3", "rust code", "rust"))

	q := query.NewBuilder().Must("body", "code").Build()
	agg := e.Aggregate(q, "language", 2)

	if len(agg.Buckets) > 2 {
		t.Errorf("topK=2 should return at most 2 buckets, got %d", len(agg.Buckets))
	}
}

func TestAggregate_DeletedDocNotCounted(t *testing.T) {
	e := New()
	e.Index(langDoc("1", "goroutines", "go"))
	e.Index(langDoc("2", "channels", "go"))
	e.Delete("2")

	q := query.NewBuilder().Should("body", "goroutines").Should("body", "channels").Build()
	agg := e.Aggregate(q, "language", 10)

	counts := map[string]int{}
	for _, b := range agg.Buckets {
		counts[b.Key] = b.Count
	}
	if counts["go"] != 1 {
		t.Errorf("deleted doc should not be counted: expected go=1, got %d", counts["go"])
	}
}

func TestAggregate_EmptyQuery(t *testing.T) {
	// An empty query should aggregate over all indexed documents.
	e := New()
	e.Index(langDoc("1", "goroutines", "go"))
	e.Index(langDoc("2", "decorators", "python"))

	q := query.NewBuilder().Build() // no clauses
	agg := e.Aggregate(q, "language", 10)

	counts := map[string]int{}
	for _, b := range agg.Buckets {
		counts[b.Key] = b.Count
	}
	if counts["go"] != 1 || counts["python"] != 1 {
		t.Errorf("empty query should aggregate all docs, got buckets: %v", agg.Buckets)
	}
}

func TestAggregate_NoBuckets(t *testing.T) {
	e := New()
	e.Index(langDoc("1", "goroutines", "go"))

	q := query.NewBuilder().Must("body", "python").Build() // no match
	agg := e.Aggregate(q, "language", 10)

	if len(agg.Buckets) != 0 {
		t.Errorf("query with no results should produce 0 buckets, got %v", agg.Buckets)
	}
}
