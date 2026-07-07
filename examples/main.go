// Package main demonstrates common go-search SDK usage patterns.
package main

import (
	"fmt"
	"log"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/engine"
	"github.com/noahfan/go-search/query"
)

// Article is an example domain type indexed with struct tags.
type Article struct {
	ID       string  `search:"id"`
	Title    string  `search:"field:title,boost:2.0"`
	Body     string  `search:"field:body"`
	Category string  `search:"field:category"`
	Score    float64 // no tag — skipped
}

func main() {
	basicSearch()
	structTagIndexing()
	synonymSearch()
	prefixSearch()
	aggregation()
	fieldMappings()
}

// basicSearch shows indexing documents and running keyword queries.
func basicSearch() {
	fmt.Println("=== Basic search ===")

	e := engine.New()

	docs := []engine.Document{
		{ID: "1", Fields: map[string]engine.Field{
			"title": {Value: "Getting started with Go", Boost: 2.0},
			"body":  {Value: "Go is a fast, statically typed compiled language"},
		}},
		{ID: "2", Fields: map[string]engine.Field{
			"title": {Value: "Python for beginners", Boost: 2.0},
			"body":  {Value: "Python is a dynamically typed interpreted language"},
		}},
		{ID: "3", Fields: map[string]engine.Field{
			"title": {Value: "Concurrency in Go", Boost: 2.0},
			"body":  {Value: "Goroutines make concurrent programming simple"},
		}},
	}

	for _, d := range docs {
		if err := e.Index(d); err != nil {
			log.Fatal(err)
		}
	}

	// Must: both fields must match; MustNot: exclude python docs.
	q := query.NewBuilder().
		Must("body", "go").
		MustNot("body", "python").
		Build()

	results := e.Search(q, 10)
	fmt.Printf("query 'go' NOT 'python' → %d result(s)\n", len(results))
	for _, r := range results {
		fmt.Printf("  id=%s score=%.4f title=%q\n", r.ID, r.Score, r.Fields["title"].Value)
	}
	fmt.Println()
}

// structTagIndexing shows how to index typed structs without constructing Document manually.
func structTagIndexing() {
	fmt.Println("=== Struct tag indexing ===")

	e := engine.New()

	articles := []Article{
		{ID: "a1", Title: "Go concurrency patterns", Body: "goroutines channels select", Category: "go"},
		{ID: "a2", Title: "Go generics guide",       Body: "type parameters constraints", Category: "go"},
		{ID: "a3", Title: "Python async io",          Body: "asyncio coroutines event loop", Category: "python"},
	}

	for _, a := range articles {
		if err := e.IndexStruct(a); err != nil {
			log.Fatal(err)
		}
	}

	q := query.NewBuilder().Must("body", "goroutines").Build()
	results := e.Search(q, 10)
	fmt.Printf("body:goroutines → %d result(s)\n", len(results))
	for _, r := range results {
		fmt.Printf("  id=%s title=%q\n", r.ID, r.Fields["title"].Value)
	}
	fmt.Println()
}

// synonymSearch shows query-time synonym expansion.
func synonymSearch() {
	fmt.Println("=== Synonym search ===")

	synonyms := analysis.NewSynonymMap(map[string][]string{
		"car":        {"automobile", "vehicle"},
		"automobile": {"car", "vehicle"},
		"vehicle":    {"car", "automobile"},
	})

	e := engine.New(engine.WithSynonyms(synonyms))

	e.Index(engine.Document{ID: "1", Fields: map[string]engine.Field{
		"body": {Value: "the automobile is parked outside"},
	}})
	e.Index(engine.Document{ID: "2", Fields: map[string]engine.Field{
		"body": {Value: "a vehicle needs regular maintenance"},
	}})
	e.Index(engine.Document{ID: "3", Fields: map[string]engine.Field{
		"body": {Value: "the bicycle is in the garage"},
	}})

	// "car" expands to car|automobile|vehicle via Should clause
	q := query.NewBuilder().Should("body", "car").Build()
	results := e.Search(q, 10)
	fmt.Printf("should:car (expanded) → %d result(s)\n", len(results))
	for _, r := range results {
		fmt.Printf("  id=%s body=%q\n", r.ID, r.Fields["body"].Value)
	}
	fmt.Println()
}

// prefixSearch shows autocomplete-style prefix matching backed by a trie.
func prefixSearch() {
	fmt.Println("=== Prefix search ===")

	e := engine.New()

	e.Index(engine.Document{ID: "1", Fields: map[string]engine.Field{
		"body": {Value: "golang goroutine goroutines"},
	}})
	e.Index(engine.Document{ID: "2", Fields: map[string]engine.Field{
		"body": {Value: "google golem gold"},
	}})

	results := e.PrefixSearch("body", "go")
	fmt.Printf("prefix 'go' → %d result(s)\n", len(results))
	for _, r := range results {
		fmt.Printf("  id=%s\n", r.ID)
	}
	fmt.Println()
}

// aggregation shows grouping results by a keyword field (faceted search).
func aggregation() {
	fmt.Println("=== Aggregation ===")

	e := engine.New(
		engine.WithMapping(engine.Mapping{
			"category": {Type: engine.FieldTypeKeyword, Index: true, Store: true},
			"body":     {Type: engine.FieldTypeText, Index: true, Store: true},
		}),
	)

	docs := []engine.Document{
		{ID: "1", Fields: map[string]engine.Field{"body": {Value: "goroutines"}, "category": {Value: "go"}}},
		{ID: "2", Fields: map[string]engine.Field{"body": {Value: "channels"}, "category": {Value: "go"}}},
		{ID: "3", Fields: map[string]engine.Field{"body": {Value: "asyncio"}, "category": {Value: "python"}}},
		{ID: "4", Fields: map[string]engine.Field{"body": {Value: "closures"}, "category": {Value: "javascript"}}},
	}
	for _, d := range docs {
		e.Index(d)
	}

	// Aggregate over all docs — no filter query needed.
	agg := e.Aggregate(query.NewBuilder().Build(), "category", 5)
	fmt.Println("category counts:")
	for _, b := range agg.Buckets {
		fmt.Printf("  %s: %d\n", b.Key, b.Count)
	}
	fmt.Println()
}

// fieldMappings shows explicit schema control and how different types are indexed.
func fieldMappings() {
	fmt.Println("=== Field mappings ===")

	e := engine.New(
		engine.WithMapping(engine.Mapping{
			"title":    {Type: engine.FieldTypeText,    Index: true,  Store: true},
			"status":   {Type: engine.FieldTypeKeyword, Index: true,  Store: true},
			"priority": {Type: engine.FieldTypeInteger, Index: true,  Store: true},
			"internal": {Type: engine.FieldTypeSkip},
		}),
	)

	e.Index(engine.Document{ID: "1", Fields: map[string]engine.Field{
		"title":    {Value: "Fix login bug"},
		"status":   {Value: "open"},
		"priority": {Value: "1"},
		"internal": {Value: "will not appear in results"},
	}})
	e.Index(engine.Document{ID: "2", Fields: map[string]engine.Field{
		"title":    {Value: "Add dark mode"},
		"status":   {Value: "closed"},
		"priority": {Value: "3"},
		"internal": {Value: "also not in results"},
	}})

	// Exact-match on keyword field.
	q := query.NewBuilder().Must("status", "open").Build()
	results := e.Search(q, 10)
	fmt.Printf("status:open → %d result(s)\n", len(results))
	for _, r := range results {
		fmt.Printf("  id=%s title=%q priority=%s\n", r.ID, r.Fields["title"].Value, r.Fields["priority"].Value)
	}
}
