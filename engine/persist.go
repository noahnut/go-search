package engine

import (
	"encoding/gob"
	"os"

	"github.com/noahfan/go-search/index"
	"github.com/noahfan/go-search/scoring"
)

type snapshot struct {
	Docs       map[string]Document
	DocLengths map[string]int
	BM25Params scoring.Params

	// replace the old flat Postings map with segment data:
	Segments   []segmentSnapshot
	Tombstones map[string]struct{}
}

type segmentSnapshot struct {
	Postings map[string]map[string]index.Posting
	Docs     map[string]struct{}
}

// Save serializes the engine state to a file at the given path.
func (e *Engine) Save(path string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	e.index.Flush()

	segments, tombstones := e.index.Snapshot()

	segmentSnapshots := make([]segmentSnapshot, len(segments))

	for i, s := range segments {
		segmentSnapshots[i] = segmentSnapshot{
			Postings: s.Postings,
			Docs:     s.Docs,
		}
	}

	data := snapshot{
		Docs:       e.docs,
		DocLengths: e.docLengths,
		BM25Params: e.bm25Params,
		Segments:   segmentSnapshots,
		Tombstones: tombstones,
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := gob.NewEncoder(f)
	return encoder.Encode(data)
}

// Load deserializes engine state from a file and returns a ready Engine.
// The returned engine uses default BM25 params and the standard analyzer.
// Pass options to override (e.g. WithAnalyzer, WithBM25Params).
func Load(path string, opts ...Option) (*Engine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	decoder := gob.NewDecoder(f)
	var data snapshot
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}

	e := New(opts...)
	e.docs = data.Docs

	segmentData := make([]index.SegmentData, len(data.Segments))

	for i, s := range data.Segments {
		segmentData[i] = index.SegmentData{
			Postings: s.Postings,
			Docs:     s.Docs,
		}
	}

	e.index.Restore(segmentData, data.Tombstones)
	e.docLengths = data.DocLengths
	e.bm25Params = data.BM25Params
	return e, nil
}
