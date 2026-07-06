package engine

// End-to-end tests exercise multiple features together in realistic scenarios.
// Each test represents a workflow a real caller would follow.

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/query"
	"github.com/noahfan/go-search/scoring"
	"github.com/noahfan/go-search/storage/local"
)

// --- helpers ---

func article(id, title, body, category string) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"title":    {Value: title, Boost: 2.0},
			"body":     {Value: body, Boost: 1.0},
			"category": {Value: category, Boost: 1.0},
		},
	}
}

// --- Scenario 1: full blog search ---

// Index a small corpus of articles, then exercise every query type and
// aggregation on the same engine instance.
func TestE2E_BlogSearch(t *testing.T) {
	e := New()
	e.Index(article("1", "Getting started with Go", "Go is a fast compiled language with goroutines", "go"))
	e.Index(article("2", "Go concurrency patterns", "Channels and goroutines make Go powerful", "go"))
	e.Index(article("3", "Python for data science", "Python has great libraries for machine learning", "python"))
	e.Index(article("4", "Rust memory safety", "Rust prevents memory bugs at compile time", "systems"))

	// Must: exact field match
	q := query.NewBuilder().Must("title", "go").Build()
	results := e.Search(q, 10)
	if len(results) != 2 {
		t.Errorf("Must('title','go'): expected 2, got %d", len(results))
	}

	// MustNot: exclude a field value
	q = query.NewBuilder().Must("body", "goroutines").MustNot("title", "concurrency").Build()
	results = e.Search(q, 10)
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("Must+MustNot: expected only doc '1', got %v", ids(results))
	}

	// Should: at least one must match, results boosted
	q = query.NewBuilder().Should("body", "goroutines").Should("body", "channels").Build()
	results = e.Search(q, 10)
	if len(results) == 0 {
		t.Error("Should: expected results")
	}

	// Phrase: word order matters
	q = query.NewBuilder().Phrase("body", "goroutines", "make", "go").Build()
	results = e.Search(q, 10)
	if len(results) != 1 || results[0].ID != "2" {
		t.Errorf("Phrase: expected doc '2', got %v", ids(results))
	}

	// FuzzySearch: typo tolerance ("goroutinez" vs "goroutines" = 1 substitution)
	results = e.FuzzySearch("body", "goroutinez", 1, 10)
	if len(results) == 0 {
		t.Error("FuzzySearch: expected results for near-match 'goroutinz'")
	}

	// PrefixSearch: autocomplete
	results = e.PrefixSearch("body", "gorou")
	if len(results) == 0 {
		t.Error("PrefixSearch: expected results for prefix 'gorou'")
	}

	// Aggregate by category
	q = query.NewBuilder().Should("body", "goroutines").Should("body", "channels").Build()
	agg := e.Aggregate(q, "category", 10)
	if len(agg.Buckets) == 0 {
		t.Error("Aggregate: expected at least one bucket")
	}
	if agg.Buckets[0].Key != "go" {
		t.Errorf("Aggregate: expected 'go' bucket first, got %q", agg.Buckets[0].Key)
	}
}

// --- Scenario 2: document lifecycle ---

// Index → search (found) → upsert (content replaced) → old term gone, new term found
// → delete → not found.
func TestE2E_DocumentLifecycle(t *testing.T) {
	e := New()

	// initial index
	e.Index(doc("1", "golang is fast"))
	q := query.NewBuilder().Must("body", "golang").Build()
	if len(e.Search(q, 10)) != 1 {
		t.Fatal("after Index: doc should be searchable")
	}

	// upsert: same ID, new content
	e.Index(doc("1", "python is flexible"))
	oldQ := query.NewBuilder().Must("body", "golang").Build()
	newQ := query.NewBuilder().Must("body", "python").Build()

	if len(e.Search(oldQ, 10)) != 0 {
		t.Error("after upsert: old term 'golang' should be gone")
	}
	if len(e.Search(newQ, 10)) != 1 {
		t.Error("after upsert: new term 'python' should be findable")
	}
	if e.Size() != 1 {
		t.Errorf("after upsert: Size should still be 1, got %d", e.Size())
	}

	// delete
	e.Delete("1")
	if len(e.Search(newQ, 10)) != 0 {
		t.Error("after Delete: doc should not appear in results")
	}
	if e.Size() != 0 {
		t.Errorf("after Delete: Size should be 0, got %d", e.Size())
	}
}

// --- Scenario 3: persist and resume ---

// Save engine state → load into fresh engine → search returns same results
// → index new docs in loaded engine → search includes them.
func TestE2E_PersistAndResume(t *testing.T) {
	dir := t.TempDir()
	docsPath := filepath.Join(dir, "docs.log")
	gobPath := filepath.Join(dir, "e2e.gob")

	store, _ := local.New(docsPath)
	original := New(WithDocStorage(store))
	original.Index(doc("1", "go is compiled"))
	original.Index(doc("2", "python is interpreted"))
	if err := original.Save(gobPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	store.Close()

	store2, err := local.New(docsPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer store2.Close()

	loaded, err := Load(gobPath, WithDocStorage(store2))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	q := query.NewBuilder().Must("body", "compiled").Build()
	results := loaded.Search(q, 10)
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("after Load: expected doc '1', got %v", ids(results))
	}

	loaded.Index(doc("3", "rust is also compiled"))
	results = loaded.Search(q, 10)
	if len(results) != 2 {
		t.Errorf("after adding doc to loaded engine: expected 2, got %d", len(results))
	}
}

// --- Scenario 4: synonyms + aggregation ---

// Synonym expansion must work transparently with Aggregate — docs found via
// synonym should count in the aggregation buckets.
func TestE2E_SynonymsAndAggregate(t *testing.T) {
	e := New(WithSynonyms(analysis.NewSynonymMap(map[string][]string{
		"car":        {"automobile", "vehicle"},
		"automobile": {"car", "vehicle"},
	})))

	e.Index(article("1", "fast automobile", "this automobile is quick", "transport"))
	e.Index(article("2", "electric vehicle", "this vehicle is green", "transport"))
	e.Index(article("3", "python tutorial", "python for beginners", "education"))

	// searching "car" should find docs with "automobile" and "vehicle"
	q := query.NewBuilder().Must("body", "car").Build()
	results := e.Search(q, 10)

	foundIDs := idSet(results)
	if !foundIDs["1"] {
		t.Error("synonym 'automobile' should match query 'car'")
	}
	if !foundIDs["2"] {
		t.Error("synonym 'vehicle' should match query 'car'")
	}
	if foundIDs["3"] {
		t.Error("unrelated doc '3' should not appear")
	}

	// aggregate should only count matched docs
	agg := e.Aggregate(q, "category", 10)
	counts := bucketMap(agg)
	if counts["transport"] != 2 {
		t.Errorf("expected transport=2, got %d", counts["transport"])
	}
	if counts["education"] != 0 {
		t.Error("education bucket should not appear — doc '3' did not match")
	}
}

// --- Scenario 5: IndexStruct full workflow ---

type BlogPost struct {
	ID       string `search:"id"`
	Title    string `search:"field:title,boost:2.0"`
	Body     string `search:"field:body"`
	Category string `search:"field:category"`
	Draft    bool   // no tag — skipped
}

func TestE2E_IndexStructWorkflow(t *testing.T) {
	e := New()

	posts := []BlogPost{
		{ID: "1", Title: "Go routines explained", Body: "goroutines are lightweight threads", Category: "go"},
		{ID: "2", Title: "Python async", Body: "asyncio is python concurrency", Category: "python"},
		{ID: "3", Title: "Go channels", Body: "channels connect goroutines", Category: "go"},
	}
	for _, p := range posts {
		if err := e.IndexStruct(p); err != nil {
			t.Fatalf("IndexStruct(%q): %v", p.ID, err)
		}
	}

	// search
	q := query.NewBuilder().Must("body", "goroutines").Build()
	results := e.Search(q, 10)
	if len(results) != 2 {
		t.Errorf("expected 2 goroutine docs, got %d", len(results))
	}

	// title boost: both posts have "go" in title, prefer the one with more title matches
	q = query.NewBuilder().Must("title", "go").Build()
	results = e.Search(q, 10)
	if len(results) != 2 {
		t.Errorf("expected 2 results for title:go, got %d", len(results))
	}

	// aggregate
	q = query.NewBuilder().Should("body", "goroutines").Should("body", "asyncio").Build()
	agg := e.Aggregate(q, "category", 10)
	counts := bucketMap(agg)
	if counts["go"] != 2 {
		t.Errorf("expected go=2, got %d", counts["go"])
	}
	if counts["python"] != 1 {
		t.Errorf("expected python=1, got %d", counts["python"])
	}
}

// --- Scenario 6: boost correctness across fields ---

// The same term queried in a high-boost field should outscore the same term
// in a low-boost field.
func TestE2E_BoostAffectsRanking(t *testing.T) {
	e := New()
	// doc "1": "golang" in title (boost 2.0)
	// doc "2": "golang" in body  (boost 1.0)
	e.Index(article("1", "golang tutorial", "introduction to programming", "go"))
	e.Index(article("2", "programming basics", "learn golang today", "go"))

	q := query.NewBuilder().Should("title", "golang").Should("body", "golang").Build()
	results := e.Search(q, 10)

	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("title match (boost 2.0) should rank first, got %s first", results[0].ID)
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("doc '1' score (%.4f) should be higher than doc '2' (%.4f)", results[0].Score, results[1].Score)
	}
}

// --- Scenario 7: custom BM25 params ---

func TestE2E_CustomBM25Params(t *testing.T) {
	// k1=0 makes term frequency irrelevant — all matching docs score the same
	e := New(WithBM25Params(scoring.Params{K1: 0, B: 0}))
	e.Index(doc("1", "go go go go go")) // "go" x5
	e.Index(doc("2", "go"))             // "go" x1

	q := query.NewBuilder().Must("body", "go").Build()
	results := e.Search(q, 10)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Score != results[1].Score {
		t.Errorf("with k1=0, all term frequencies should score equally: %.4f vs %.4f",
			results[0].Score, results[1].Score)
	}
}

// --- Scenario 9: concurrent access ---

func TestE2E_ConcurrentReadsDuringIndex(t *testing.T) {
	e := New()
	for i := 0; i < 10; i++ {
		e.Index(doc(string(rune('a'+i)), "golang is fast"))
	}

	var wg sync.WaitGroup
	q := query.NewBuilder().Must("body", "golang").Build()

	// concurrent searchers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Search(q, 10)
		}()
	}

	// concurrent indexers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			e.Index(doc(strings.Repeat("z", n+1), "golang concurrent"))
		}(i)
	}

	wg.Wait()
}

// --- helpers ---

func ids(results []Result) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.ID
	}
	return out
}

func idSet(results []Result) map[string]bool {
	m := map[string]bool{}
	for _, r := range results {
		m[r.ID] = true
	}
	return m
}

func bucketMap(agg AggResult) map[string]int {
	m := map[string]int{}
	for _, b := range agg.Buckets {
		m[b.Key] = b.Count
	}
	return m
}
