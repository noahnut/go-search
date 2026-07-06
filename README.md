# go-search

An Elasticsearch-inspired, in-process full-text search engine SDK written in Go.
Runs entirely inside the caller's Go process — no external service, no network, no dependencies.

## Features

- **Full-text search** with BM25 relevance ranking
- **Boolean queries** — must, should, must_not, phrase
- **Multi-field documents** with per-field boost multipliers
- **Fuzzy search** using Levenshtein edit distance
- **Vector search** (semantic / dense retrieval) using cosine similarity
- **Hybrid search** — combine BM25 and vector scores with a tunable alpha weight
- **Synonyms** — query-time expansion so "car" finds "automobile" and "vehicle"
- **Segment-based index** with O(1) deletes via tombstones
- **Concurrent segment search** — immutable segments searched in parallel
- **Segment merging** — collapse segments and permanently drop deleted documents
- **Persistence** — append-only WAL for document durability; gob snapshots for fast startup; automatic delta recovery on restart
- **Prefix search / autocomplete** — trie-backed, returns all docs whose terms start with a prefix
- **Aggregations** — group and count results by field value (faceted search)
- **Struct tag indexing** — annotate your own structs with `search:` tags; no manual `Document{}` construction
- **Field mappings** — declare each field as `text` (analyzed), `keyword` (exact match), or `skip` (not indexed); control `index` and `store` independently
- **Dynamic mapping** — engine infers field types automatically from values; schema is locked on first write and survives snapshots
- **Large document support** — fields above a configurable threshold are chunked and stored on disk; only byte offsets are kept in memory
- **Functional options** for custom analyzers, BM25 parameters, synonym maps, and large-doc threshold

## Installation

```bash
go get github.com/noahfan/go-search
```

## Quick start

```go
package main

import (
    "fmt"

    "github.com/noahfan/go-search/engine"
    "github.com/noahfan/go-search/query"
)

func main() {
    e := engine.New()

    e.Index(engine.Document{
        ID: "1",
        Fields: map[string]engine.Field{
            "title": {Value: "Getting started with Go", Boost: 2.0},
            "body":  {Value: "Go is a fast, statically typed language", Boost: 1.0},
        },
    })
    e.Index(engine.Document{
        ID: "2",
        Fields: map[string]engine.Field{
            "title": {Value: "Python for beginners", Boost: 2.0},
            "body":  {Value: "Python is a dynamically typed language", Boost: 1.0},
        },
    })

    q := query.NewBuilder().
        Must("title", "go").
        MustNot("body", "python").
        Build()

    results := e.Search(q, 10)
    for _, r := range results {
        fmt.Printf("id=%s score=%.4f title=%s\n", r.ID, r.Score, r.Fields["title"].Value)
    }
}
```

## API

### Engine

```go
// Default settings
e := engine.New()

// Custom options
e := engine.New(
    engine.WithAnalyzer(myAnalyzer),
    engine.WithBM25Params(scoring.Params{K1: 1.2, B: 0.75}),
    engine.WithSynonyms(analysis.NewSynonymMap(map[string][]string{
        "car":        {"automobile", "vehicle"},
        "automobile": {"car", "vehicle"},
        "vehicle":    {"car", "automobile"},
    })),
)
```

### Indexing

```go
// Upsert — re-indexing the same ID replaces the old document
err := e.Index(engine.Document{
    ID: "doc-1",
    Fields: map[string]engine.Field{
        "title":     {Value: "Hello world", Boost: 2.0},
        "body":      {Value: "Some content here", Boost: 1.0},
        "embedding": {Vector: []float64{0.1, 0.8, 0.3}}, // optional: for vector/hybrid search
    },
})

e.Delete("doc-1")
n := e.Size()
```

### Keyword search

```go
q := query.NewBuilder().
    Must("body", "go").        // MUST contain "go" in body
    Should("body", "fast").    // boosts score if "fast" also matches
    MustNot("body", "python"). // MUST NOT contain "python"
    Build()

results := e.Search(q, topK)
```

### Phrase search

```go
// Terms must appear consecutively in this order
q := query.NewBuilder().Phrase("body", "new", "york").Build()
results := e.Search(q, 10)
```

### Fuzzy search

```go
// Matches terms within edit distance 1 — "golong" matches "golang"
results := e.FuzzySearch("body", "golong", 1, 10)
```

### Vector search

```go
queryVector := []float64{0.1, 0.8, 0.3}
results := e.VectorSearch("embedding", queryVector, 10)
```

### Hybrid search

Combines BM25 keyword ranking with vector similarity. `alpha` controls the balance:
`0.0` = pure vector, `1.0` = pure BM25.

```go
q := query.NewBuilder().Must("body", "go").Build()
queryVector := []float64{0.1, 0.8, 0.3}

results := e.HybridSearch(q, "embedding", queryVector, 0.5, 10)
```

### Synonyms

Synonyms expand queries at search time — no re-indexing required. Configure once
on the engine; all `Search` and `HybridSearch` calls use them automatically.

```go
e := engine.New(
    engine.WithSynonyms(analysis.NewSynonymMap(map[string][]string{
        "car":        {"automobile", "vehicle"},
        "automobile": {"car", "vehicle"},
    })),
)

e.Index(doc("1", "automobile is fast"))

// Finds doc "1" even though it doesn't contain the word "car"
results := e.Search(query.NewBuilder().Must("body", "car").Build(), 10)
```

### Prefix search

Returns all documents containing a term that starts with the given prefix. Backed by a
trie — O(prefix length) to find all completions.

```go
// finds docs containing "golang", "gold", "golden", etc.
results := e.PrefixSearch("body", "gol")
```

### Aggregations

Groups matching documents by a field value and counts each group — equivalent to
SQL `GROUP BY field ORDER BY count DESC LIMIT K`.

```go
q := query.NewBuilder().Must("body", "go").Build()
agg := e.Aggregate(q, "language", 5) // top 5 languages among matching docs

for _, bucket := range agg.Buckets {
    fmt.Printf("%s: %d\n", bucket.Key, bucket.Count)
}
```

Pass an empty query to aggregate over all indexed documents:

```go
agg := e.Aggregate(query.NewBuilder().Build(), "language", 10)
```

### Struct tag indexing

Annotate your own structs with `search:` tags and call `IndexStruct` — no manual
`Document{}` construction needed. Uses reflection, the same mechanism as `encoding/json`.

```go
type Article struct {
    ID    string `search:"id"`
    Title string `search:"field:title,boost:2.0"`
    Body  string `search:"field:body"`
    Draft bool   // no tag — skipped
}

err := e.IndexStruct(Article{ID: "1", Title: "Go concurrency", Body: "goroutines"})
err = e.IndexStruct(&article) // pointer also works
```

Tag reference:

| Tag | Meaning |
|---|---|
| `search:"id"` | Document ID (required, must be a non-empty string) |
| `search:"field:name"` | Index under field "name" with boost 1.0 |
| `search:"field:name,boost:2.0"` | Index under "name" with boost 2.0 |
| `search:"-"` | Skip this field |
| *(no tag)* | Skip this field |

### Field mappings

Declare how each field should be handled. Fields not in the mapping are inferred
automatically (see dynamic mapping below).

```go
e := engine.New(
    engine.WithMapping(engine.Mapping{
        "title":    {Type: engine.FieldTypeText,    Index: true,  Store: true},
        "category": {Type: engine.FieldTypeKeyword, Index: true,  Store: true},
        "url":      {Type: engine.FieldTypeText,    Index: false, Store: true},  // stored but not searchable
        "internal": {Type: engine.FieldTypeSkip},                                // not indexed, not stored
    }),
)
```

| Field type | Indexing | Use for |
|---|---|---|
| `FieldTypeText` | analyzed (tokenized, lowercased) | body text, titles |
| `FieldTypeKeyword` | raw value, exact match only | categories, tags, status values |
| `FieldTypeInteger` | exact match (treated as keyword internally) | numeric IDs, counts |
| `FieldTypeFloat` | exact match | prices, scores |
| `FieldTypeBoolean` | exact match for `"true"` / `"false"` | flags |
| `FieldTypeSkip` | not indexed, not stored | internal metadata |

`Index: false` keeps the value in results but skips the inverted index.
`Store: false` makes the field searchable but omits it from returned results.

Keyword fields aggregate correctly — all docs with `category: "go"` share the same
bucket key, unlike text fields where the value is split into tokens.

### Dynamic mapping

With no explicit mapping, the engine infers types from the first value seen for each
field and locks that type for all subsequent documents:

```go
e := engine.New() // no WithMapping — fully dynamic

e.Index(engine.Document{ID: "1", Fields: map[string]engine.Field{
    "title":    {Value: "Go concurrency patterns"}, // → text (has spaces)
    "category": {Value: "programming"},             // → keyword (short, no spaces)
    "score":    {Value: "42"},                      // → integer
    "active":   {Value: "true"},                    // → boolean
}})

// inspect the inferred schema
for name, fm := range e.Schema().Fields() {
    fmt.Printf("%s: %s\n", name, fm.Type)
}
```

Inference rules (checked in order):

| Value | Inferred type |
|---|---|
| `"true"` / `"false"` | boolean |
| parseable as integer | integer |
| parseable as float | float |
| no spaces and len < 64 | keyword |
| otherwise | text |

If two documents disagree on an inferred type, the engine promotes to `text`
(the most general type) rather than erroring. Explicit mappings set via
`WithMapping` are always authoritative — they are never overridden by inference.

### Large documents

Fields whose value exceeds the threshold (default 64 KB) are automatically chunked
and stored on disk. Only byte offsets live in memory. The caller uses the same
`Index` and `Search` API — chunking is invisible.

```go
// default threshold: 64 KB
e := engine.New()

// custom threshold
e := engine.New(engine.WithLargeDocThreshold(32 * 1024))

// index as normal — engine routes large fields to disk automatically
e.Index(engine.Document{
    ID: "book-1",
    Fields: map[string]engine.Field{
        "title": {Value: "War and Peace"},
        "body":  {Value: veryLongString}, // chunked if > threshold
    },
})

// search returns "book-1", not internal chunk IDs
results := e.Search(query.NewBuilder().Must("body", "napoleon").Build(), 10)
```

### Persistence

By default the engine is in-memory and state is lost when the process exits.
For durability, wire up a local document store and optionally a snapshot directory.

**Document store (WAL)**

The document store is an append-only log that survives restarts. On startup the
engine replays it to rebuild the inverted index automatically.

```go
import "github.com/noahfan/go-search/storage/local"

store, err := local.New("data/docs.log")
if err != nil { ... }

e := engine.New(engine.WithDocStorage(store))
// Index, Search, Delete — as normal
e.Close() // flushes and closes the log
```

On the next run, pass the same path and the engine restores all documents:

```go
store, err := local.New("data/docs.log")
e := engine.New(engine.WithDocStorage(store)) // index rebuilt from log automatically
```

**Snapshots (faster startup)**

For large indexes, replaying the full WAL on every startup is slow. Snapshots
capture the inverted index at a point in time so startup only replays the delta
(documents written after the last snapshot).

```go
store, err := local.New("data/docs.log")
e := engine.New(
    engine.WithDocStorage(store),
    engine.WithSnapshotDir("data/snapshots"),          // where to write snapshot.gob
    engine.WithSnapshotInterval(5 * time.Minute),      // optional: snapshot on a timer
)

// Manual snapshot
err = e.Snapshot()

// Close triggers a final snapshot automatically
err = e.Close()
```

On restart, the engine loads the snapshot and re-indexes only the documents that
arrived after it was taken — the delta is recovered from the WAL.

**Low-level save / load**

For one-off serialization (backups, offline loading) without a running WAL:

```go
err := e.Save("/path/to/index.gob")

e, err := engine.Load("/path/to/index.gob", engine.WithDocStorage(store))
```

## Architecture

```
analysis/        tokenizer + filters + analyzer pipeline + synonym maps
index/           segment-based inverted index (term → posting list) + trie for prefix search
scoring/         BM25 relevance ranking
query/           boolean query builder and matching logic
storage/         Storage interface + in-memory implementation
storage/local/   append-only WAL (Bitcask-style) for document durability
engine/          public SDK — Index, IndexStruct, Search, FuzzySearch, VectorSearch,
                              HybridSearch, PrefixSearch, Aggregate, Snapshot, Save/Load,
                              Schema (field mapping + dynamic type inference)
```

### Query API

Fields and terms are always separate — the caller never writes `"field:term"` strings.
The engine handles all internal prefixing:

```go
// field and term are two separate arguments
query.NewBuilder().Must("title", "golang").MustNot("body", "deprecated").Build()
```

### Analysis pipeline

```
input text → Tokenizer → []Token → Filters → []Token → index
```

```go
analyzer := analysis.NewAnalyzer(
    &analysis.StandardTokenizer{},
    &analysis.LowercaseFilter{},
    analysis.NewStopWordFilter([]string{"the", "is", "a"}),
)
e := engine.New(engine.WithAnalyzer(analyzer))
```

Available tokenizers: `WhitespaceTokenizer`, `StandardTokenizer`.

Available filters: `LowercaseFilter`, `StopWordFilter`.

### Segment-based index

```
Add docs → buffer → [flush] → segment₀
                    [flush] → segment₁
                    [flush] → segment₂  ← searched concurrently
```

Deletes are O(1) tombstones. `Merge()` collapses all segments into one and permanently
removes deleted documents. Segments are immutable after creation, so reads need no
locking and scale across goroutines.

### BM25 scoring

```
Score(d, q) = IDF(q) × TF(q, d)
IDF = log(1 + (N - df + 0.5) / (df + 0.5))
TF  = freq × (k1 + 1) / (freq + k1 × (1 - b + b × docLen/avgDocLen))
```

Default: `K1=1.2`, `B=0.75`. Override with `WithBM25Params`.

### Hybrid scoring

BM25 scores are min-max normalized to [0, 1] before combining with cosine similarity,
so `alpha` behaves consistently regardless of BM25 magnitude:

```
hybrid = alpha × normalize(BM25) + (1 - alpha) × cosine_similarity
```

## Running tests

```bash
go test ./...

# With race detector
go test -race ./...

# E2E scenarios only
go test ./engine/... -run E2E -v
```
