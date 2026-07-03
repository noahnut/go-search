package engine

import (
	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/scoring"
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

func WithLargeDocThreshold(threshold int) Option {
	return func(e *Engine) {
		e.largeDocThreshold = threshold
	}
}
