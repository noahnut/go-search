package engine

import (
	"time"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/index"
	"github.com/noahfan/go-search/scoring"
	"github.com/noahfan/go-search/storage"
)

type Option func(*Engine)

func WithAnalyzer(a *analysis.Analyzer) Option {
	return func(e *Engine) {
		e.analyzer = a
	}
}

func WithBM25Params(params scoring.Params) Option {
	return func(e *Engine) {
		e.bm25Params = params
	}
}

func WithSynonyms(m analysis.SynonymMap) Option {
	return func(e *Engine) {
		e.synonyms = m
	}
}

func WithDocStorage(storage storage.Storage) Option {
	return func(e *Engine) {
		e.docStorage = storage
	}
}

func WithLargeDocThreshold(threshold int) Option {
	return func(e *Engine) {
		e.largeDocThreshold = threshold
	}
}

// WithSnapshotInterval saves the inverted index to disk on a timer.
// Has no effect unless WithStorage is also set and snapshotDir is configured.
// Pair with WithStorage(local.New(...)) for full durability.
func WithSnapshotInterval(d time.Duration) Option {
	return func(e *Engine) {
		e.snapshotInterval = d
	}
}

// WithSnapshotDir sets the directory for snapshot files.
// Required when using WithSnapshotInterval.
func WithSnapshotDir(dir string) Option {
	return func(e *Engine) {
		e.snapshotDir = dir
	}
}

func WithMapping(m Mapping) Option {
	return func(e *Engine) {
		for fieldName, fieldMapping := range m {
			fieldMapping.Explicit = true
			e.schema.Set(fieldName, fieldMapping)
		}
	}
}

func WithFlushPolicy(p index.FlushPolicy) Option {
	return func(e *Engine) {
		e.flushPolicy = &p
	}
}

func WithMergePolicy(p index.MergePolicy) Option {
	return func(e *Engine) {
		e.mergePolicy = &p
	}
}
