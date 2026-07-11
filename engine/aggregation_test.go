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

// --- Metrics tests ---

// metricDoc creates a doc with a searchable body and a numeric price field.
func metricDoc(id, body, price string) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"body":  {Value: body, Boost: 1.0},
			"price": {Value: price},
		},
	}
}

func TestMetrics_BasicStats(t *testing.T) {
	e := New()
	e.Index(metricDoc("1", "item", "10"))
	e.Index(metricDoc("2", "item", "20"))
	e.Index(metricDoc("3", "item", "30"))

	q := query.NewBuilder().Must("body", "item").Build()
	res := e.Metrics(q, "price")

	if res.Count != 3 {
		t.Errorf("expected count=3, got %d", res.Count)
	}
	if res.Min != 10 {
		t.Errorf("expected min=10, got %f", res.Min)
	}
	if res.Max != 30 {
		t.Errorf("expected max=30, got %f", res.Max)
	}
	if res.Sum != 60 {
		t.Errorf("expected sum=60, got %f", res.Sum)
	}
	if res.Avg != 20 {
		t.Errorf("expected avg=20, got %f", res.Avg)
	}
}

func TestMetrics_NonNumericFieldSkipped(t *testing.T) {
	e := New()
	e.Index(metricDoc("1", "item", "10"))
	e.Index(Document{
		ID: "2",
		Fields: map[string]Field{
			"body":  {Value: "item", Boost: 1.0},
			"price": {Value: "N/A"}, // non-numeric
		},
	})

	q := query.NewBuilder().Must("body", "item").Build()
	res := e.Metrics(q, "price")

	if res.Count != 1 {
		t.Errorf("non-numeric value should be excluded from count, got count=%d", res.Count)
	}
	if res.Min != 10 || res.Max != 10 {
		t.Errorf("expected min=max=10, got min=%f max=%f", res.Min, res.Max)
	}
}

func TestMetrics_MissingFieldSkipped(t *testing.T) {
	e := New()
	e.Index(metricDoc("1", "item", "50"))
	e.Index(Document{
		ID:     "2",
		Fields: map[string]Field{"body": {Value: "item", Boost: 1.0}}, // no price
	})

	q := query.NewBuilder().Must("body", "item").Build()
	res := e.Metrics(q, "price")

	if res.Count != 1 {
		t.Errorf("doc without price field should be excluded, got count=%d", res.Count)
	}
}

func TestMetrics_NoMatchingDocs_ZeroResult(t *testing.T) {
	e := New()
	e.Index(metricDoc("1", "item", "10"))

	q := query.NewBuilder().Must("body", "missing").Build()
	res := e.Metrics(q, "price")

	if res.Count != 0 {
		t.Errorf("expected count=0 for no matches, got %d", res.Count)
	}
	if res.Min != 0 || res.Max != 0 || res.Sum != 0 || res.Avg != 0 {
		t.Errorf("expected all zero for count=0, got min=%f max=%f sum=%f avg=%f", res.Min, res.Max, res.Sum, res.Avg)
	}
}

func TestMetrics_SingleDoc(t *testing.T) {
	e := New()
	e.Index(metricDoc("1", "item", "42"))

	q := query.NewBuilder().Must("body", "item").Build()
	res := e.Metrics(q, "price")

	if res.Count != 1 {
		t.Errorf("expected count=1, got %d", res.Count)
	}
	if res.Min != 42 || res.Max != 42 || res.Sum != 42 || res.Avg != 42 {
		t.Errorf("single doc: all stats should be 42, got min=%f max=%f sum=%f avg=%f", res.Min, res.Max, res.Sum, res.Avg)
	}
}

func TestMetrics_FloatValues(t *testing.T) {
	e := New()
	e.Index(metricDoc("1", "item", "4.50"))
	e.Index(metricDoc("2", "item", "9.99"))
	e.Index(metricDoc("3", "item", "19.99"))

	q := query.NewBuilder().Must("body", "item").Build()
	res := e.Metrics(q, "price")

	if res.Count != 3 {
		t.Errorf("expected count=3, got %d", res.Count)
	}
	if res.Min != 4.50 {
		t.Errorf("expected min=4.50, got %f", res.Min)
	}
	if res.Max != 19.99 {
		t.Errorf("expected max=19.99, got %f", res.Max)
	}
}

func TestMetrics_SubsetMatchedByQuery(t *testing.T) {
	e := New()
	e.Index(metricDoc("1", "laptop", "1000"))
	e.Index(metricDoc("2", "laptop", "2000"))
	e.Index(metricDoc("3", "phone", "500")) // won't match

	q := query.NewBuilder().Must("body", "laptop").Build()
	res := e.Metrics(q, "price")

	if res.Count != 2 {
		t.Errorf("expected count=2 (only laptop docs), got %d", res.Count)
	}
	if res.Min != 1000 || res.Max != 2000 {
		t.Errorf("expected min=1000 max=2000, got min=%f max=%f", res.Min, res.Max)
	}
	if res.Avg != 1500 {
		t.Errorf("expected avg=1500, got %f", res.Avg)
	}
}
