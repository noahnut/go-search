package engine

import (
	"encoding/gob"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/noahfan/go-search/index"
	"github.com/noahfan/go-search/scoring"
)

type snapshot struct {
	Docs       map[string]struct{}
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

func (e *Engine) Snapshot() error {
	if e.snapshotDir == "" {
		return nil
	}
	return e.Save(filepath.Join(e.snapshotDir, SnapshotFileName))
}

// Save serializes the engine state to a file at the given path.
func (e *Engine) Save(path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

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

	e := newBase(opts...)

	segmentData := make([]index.SegmentData, len(data.Segments))

	for i, s := range data.Segments {
		segmentData[i] = index.SegmentData{
			Postings: s.Postings,
			Docs:     s.Docs,
		}
	}

	e.index.Restore(segmentData, data.Tombstones)
	e.docLengths = data.DocLengths
	if e.docLengths == nil {
		e.docLengths = make(map[string]int)
	}
	e.bm25Params = data.BM25Params

	return e, nil
}

func (e *Engine) recoverDelta() error {
	// build set of doc IDs already in the restored index

	indexedIDs := map[string]struct{}{}
	for _, segment := range e.index.Segments() {
		for docID := range segment.Docs() {
			indexedIDs[docID] = struct{}{}
		}
	}

	// re-index only docs that arrived after the last snapshot
	e.docStorage.Each(func(id string, raw []byte) {
		if _, ok := indexedIDs[id]; ok {
			return
		}
		var doc Document
		json.Unmarshal(raw, &doc)
		for fieldName, field := range doc.Fields {
			e.index.Add(id, field.Value, &fieldName, e.analyzer)
			e.docLengths[id+":"+fieldName] = len(e.analyzer.Analyze(field.Value))
		}
	})

	return nil
}
