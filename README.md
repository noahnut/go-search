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
- **Result highlighting** — matched terms wrapped in configurable markers (`<em>…</em>`) per field
- **Segment-based index** with O(1) deletes via tombstones
- **Concurrent segment search** — immutable segments searched in parallel
- **Segment merging** — collapse segments and permanently drop deleted documents
- **Persistence** — append-only WAL for document durability; gob snapshots for fast startup; automatic delta recovery on restart
- **Prefix search / autocomplete** — trie-backed, returns all docs whose terms start with a prefix
- **Aggregations** — group and count results by field value (faceted search)
- **Struct tag indexing** — annotate your own structs with `search:` tags; no manual `Document{}` construction
- **Field mappings** — declare each field as `text` (analyzed), `keyword` (exact match), or `skip` (not indexed); control `index` and `store` independently
- **Dynamic mapping** — engine infers field types automatically from values; schema is locked on first write and survives snapshots
- **Stop words** — built-in English preset; extend or replace with custom word lists

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

A runnable file covering all major features is in [`examples/main.go`](examples/main.go).

## API

### Engine

```go
// Default settings — in-memory, no persistence
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
        "embedding": {Vector: []float64{0.1, 0.8, 0.3}}, // for vector/hybrid search
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

The combined score is a weighted sum: `alpha × BM25 + (1 - alpha) × cosine_similarity`.
Documents that appear only in BM25 or only in vector results are included with the
missing side scored as 0.

### Synonyms

Synonyms expand queries at search time — no re-indexing required. Configure once
on the engine; all `Search` and `HybridSearch` calls use them automatically.
Only `Should` clauses are expanded; `Must`, `MustNot`, and `Phrase` clauses are not.

```go
e := engine.New(
    engine.WithSynonyms(analysis.NewSynonymMap(map[string][]string{
        "car":        {"automobile", "vehicle"},
        "automobile": {"car", "vehicle"},
    })),
)

e.Index(engine.Document{ID: "1", Fields: map[string]engine.Field{
    "body": {Value: "automobile is fast"},
}})

// Finds doc "1" because "car" expands to include "automobile"
results := e.Search(query.NewBuilder().Should("body", "car").Build(), 10)
```

### Prefix search

Returns all documents containing a term that starts with the given prefix. Backed by a
trie — O(prefix length) to find all completions.

```go
// finds docs containing "golang", "gold", "golden", etc.
results := e.PrefixSearch("body", "gol")
```

### Highlighting

`Search` automatically populates `Result.Highlights` — one entry per field that
contains a matched query term, with the matching word wrapped in `<em>…</em>`.

```go
results := e.Search(query.NewBuilder().Must("body", "go").Build(), 10)
for _, r := range results {
    for _, h := range r.Highlights {
        fmt.Printf("field=%s snippet=%s\n", h.Field, h.Snippet)
        // field=body snippet="<em>Go</em> is a fast compiled language"
    }
}
```

Only `Must` and `Should` terms are highlighted. `MustNot` terms and keyword/numeric
fields are excluded. To generate highlights with custom markers outside of `Search`:

```go
highlights := engine.HighlightDoc(doc, []string{"go", "fast"}, analyzer, "**", "**")
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

Keyword fields are ideal for aggregations — because they are indexed as a single
raw token, all docs with `category: "go"` share exactly the same bucket key.

### Struct tag indexing

Annotate your own structs with `search:` tags and call `IndexStruct` — no manual
`Document{}` construction needed.

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
| *(no tag)* | Skip this field |

### Field mappings

Declare how each field should be handled. Fields not in the mapping are inferred
automatically from their values (see dynamic mapping below).

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
| `FieldTypeInteger` | exact match (stored as raw string) | numeric IDs, counts |
| `FieldTypeFloat` | exact match (stored as raw string) | prices, scores |
| `FieldTypeBoolean` | exact match for `"true"` / `"false"` | flags |
| `FieldTypeSkip` | not indexed, not stored | internal metadata |

`Index: false` keeps the value in results but skips the inverted index — the field
can be displayed but not searched.

`Store: false` makes the field searchable but omits it from `Result.Fields`.

Explicit mappings are always authoritative — they are never overridden by inference.

### Dynamic mapping

With no explicit mapping, the engine infers field types from the first value seen
for each field and locks that type for all subsequent documents:

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

If two documents infer different types for the same field, the engine promotes to
`text` rather than erroring. Explicit mappings (via `WithMapping`) always win and
are never conflict-checked against inferred types.

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
arrived after it was taken — the delta is recovered from the WAL. The schema is
also persisted in snapshots and restored on load.

**Low-level save / load**

For one-off serialization (backups, offline loading) without a running WAL:

```go
err := e.Save("/path/to/index.gob")

e, err := engine.Load("/path/to/index.gob", engine.WithDocStorage(store))
```

### Flush and merge policy

Control when the in-memory write buffer flushes to a segment and when segments are merged.

```go
import "github.com/noahfan/go-search/index"

e := engine.New(
    // Flush to a new segment every 50,000 tokens or every second.
    engine.WithFlushPolicy(index.FlushPolicy{
        MaxTokens:     50_000,
        FlushInterval: time.Second,
    }),
    // Merge segments in the background whenever there are more than 5.
    engine.WithMergePolicy(index.MergePolicy{
        MaxSegments: 5,
    }),
)
```

| Option | Effect | Default |
|---|---|---|
| `MaxTokens` | Flush when the buffer holds this many tokens | 128 |
| `MaxBytes` | Flush when estimated buffer size exceeds this | 1 MB |
| `FlushInterval` | Flush on a timer regardless of size (0 = off) | off |
| `MaxSegments` | Trigger background merge above this segment count (0 = off) | 10 |

`MaxTokens` and `MaxBytes` cap memory. `FlushInterval` caps latency — new writes become searchable within the interval. Merging reduces the number of segments searched at query time.

### Reindex

Rebuild the index from scratch without taking the engine offline. Useful when you change the analyzer, add a field mapping, or swap the storage backend.

```go
// Parallel reindex using 8 workers.
// The live engine stays searchable during the rebuild.
err := e.Reindex(8)

// Reindex with a new analyzer — the rebuilt index uses it from the start.
err = e.Reindex(8, engine.WithAnalyzer(newAnalyzer))
```

`Reindex` requires a storage backend (`WithDocStorage`) — it returns an error on memory-only engines. Internally it builds a shadow engine and atomically swaps the index state when complete. If it fails partway, the live engine is unchanged.

## Architecture

```
analysis/        tokenizer + filters + analyzer pipeline + synonym maps
index/           segment-based inverted index (term → posting list) + trie for prefix search
scoring/         BM25 relevance ranking
query/           boolean query builder and matching logic
storage/         Storage interface + in-memory implementation
storage/local/   append-only WAL (Bitcask-style) for document durability
engine/          public SDK — Index, IndexStruct, Search, FuzzySearch, VectorSearch,
                              HybridSearch, PrefixSearch, Aggregate, HighlightDoc,
                              Schema, Reindex, Snapshot, Save, Load, Close
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

**Stop words** — custom list or built-in English preset:

```go
// Custom list
analysis.NewStopWordFilter([]string{"the", "is", "a"})

// Built-in English preset (~50 common words: the, a, is, are, and, or, ...)
analysis.NewEnglishStopWordFilter()

// Extend the preset with domain-specific words
analysis.NewEnglishStopWordFilter().Add("golang", "python", "rust")
```

`Add` is case-insensitive and returns the same filter for chaining. Stop word
construction is also case-insensitive — `NewStopWordFilter(["The", "IS"])` removes
lowercased tokens `"the"` and `"is"`.

Position numbers of surviving tokens are never renumbered — this preserves phrase
search correctness when stop words fall between phrase terms.

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
Score(d, q) = IDF(q) × TF(q, d) × boost
IDF = ln(1 + (N - df + 0.5) / (df + 0.5))
TF  = freq × (k1 + 1) / (freq + k1 × (1 - b + b × docLen/avgDocLen))
```

Default: `K1=1.2`, `B=0.75`. Override with `WithBM25Params`.

## Running tests

```bash
go test ./...

# With race detector
go test -race ./...

# E2E scenarios only
go test ./engine/... -run E2E -v
```
