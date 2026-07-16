package index

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/storage"
)

// FlushPolicy controls when the buffer flushes to a segment automatically.
type FlushPolicy struct {
	MaxTokens     int           // flush when token count >= this (0 = disabled)
	MaxBytes      int           // flush when estimated byte size >= this (0 = disabled)
	FlushInterval time.Duration // flush on a timer regardless of size (0 = disabled)
}

// MergePolicy controls when background segment merging is triggered.
type MergePolicy struct {
	MaxSegments int // trigger background merge when segment count > this (0 = disabled)
}

// Posting records one occurrence of a term in a document.
type Posting struct {
	DocID     string
	Frequency int   // how many times the term appears in this document
	Positions []int // positions from the tokenizer

}

type SegmentData struct {
	Postings map[string]map[string]Posting
	Docs     map[string]struct{}
}

type Option func(*Index)

func WithFlushPolicy(p *FlushPolicy) Option {
	return func(idx *Index) {
		if p == nil {
			return
		}
		idx.flushPolicy = *p
	}
}

func WithMergePolicy(p *MergePolicy) Option {
	return func(idx *Index) {
		if p == nil {
			return
		}
		idx.mergePolicy = *p
	}
}

// Index is an inverted index: maps each term to its list of postings.
type Index struct {
	mu          sync.RWMutex
	trie        *Trie
	buffer      map[string]map[string]Posting // in-memory write buffer
	bufferDocs  map[string]struct{}
	segments    []*Segment          // immutable flushed segments
	tombstones  map[string]struct{} // deleted docIDs
	flushSize   int                 // flush buffer → segment when buffer hits this
	flushPolicy FlushPolicy
	mergePolicy MergePolicy
	docCount    int           // total number of unique documents in the index
	tokenCount  int           // total number of tokens in the index
	merging     atomic.Int32  // 0 = idle, 1 = in progress
	numeric     *NumericIndex // numeric index
	fieldIndex  *FieldIndex   // field index
	stopFlush   chan struct{} // closed by StopFlushTimer
}

// NewWithFlushSize creates an index that flushes every n documents.
// Used by tests that need deterministic segment boundaries.
func NewWithFlushSize(n int, storage storage.Storage) *Index {
	idx := New(storage)
	idx.flushPolicy.MaxTokens = 0 // disable token-based auto-flush
	idx.flushSize = n
	return idx
}

func New(storage storage.Storage, opts ...Option) *Index {

	defaultFlushPolicy := FlushPolicy{
		MaxTokens:     128,
		MaxBytes:      1024 * 1024, // 1 MB
		FlushInterval: 0,           // disabled by default
	}

	defaultMergePolicy := MergePolicy{
		MaxSegments: 10, // trigger merge when segment count > 10

	}

	idx := &Index{
		buffer:      make(map[string]map[string]Posting),
		bufferDocs:  make(map[string]struct{}),
		segments:    []*Segment{},
		tombstones:  make(map[string]struct{}),
		trie:        NewTrie(),
		flushPolicy: defaultFlushPolicy,
		mergePolicy: defaultMergePolicy,
		docCount:    0,
		merging:     atomic.Int32{},
		numeric:     NewNumericIndex(storage),
		fieldIndex:  NewFieldIndex(),
	}

	for _, opt := range opts {
		opt(idx)
	}

	if idx.flushPolicy.FlushInterval > 0 {
		idx.stopFlush = make(chan struct{})
		go idx.flushOnInterval()
	}

	return idx
}

func (idx *Index) flushOnInterval() {
	ticker := time.NewTicker(idx.flushPolicy.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			idx.mu.Lock()
			if len(idx.bufferDocs) > 0 {
				idx.Flush()
				idx.tokenCount = 0
			}
			idx.mu.Unlock()
		case <-idx.stopFlush:
			return
		}
	}
}

func (idx *Index) StopFlushTimer() {
	if idx.stopFlush != nil {
		close(idx.stopFlush)
	}
}

// public API stays the same:
func (idx *Index) Add(docID string, text string, fieldName *string, analyzer *analysis.Analyzer) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// tokenize the text
	tokens := analyzer.Analyze(text)

	idx.clearTombstone(docID)

	for _, token := range tokens {

		if fieldName != nil {
			token.Term = *fieldName + ":" + token.Term
		}

		idx.trie.Insert(token.Term)

		if _, exists := idx.buffer[token.Term]; !exists {
			idx.buffer[token.Term] = make(map[string]Posting)
		}

		posting, exists := idx.buffer[token.Term][docID]
		if !exists {
			posting = Posting{
				DocID:     docID,
				Frequency: 0,
				Positions: []int{},
			}
		}
		posting.Frequency++
		posting.Positions = append(posting.Positions, token.Position)
		idx.buffer[token.Term][docID] = posting

		idx.tokenCount++
	}

	idx.bufferDocs[docID] = struct{}{}
	idx.docCount++

	if (idx.flushPolicy.MaxTokens > 0 && idx.tokenCount >= idx.flushPolicy.MaxTokens) ||
		(idx.flushSize > 0 && len(idx.bufferDocs) >= idx.flushSize) {
		idx.Flush()
		idx.tokenCount = 0
	}
}

func (idx *Index) AddRaw(docID string, fieldName string, fieldValue string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.clearTombstone(docID)

	term := fieldName + ":" + fieldValue
	idx.trie.Insert(term)

	if _, exists := idx.buffer[term]; !exists {
		idx.buffer[term] = make(map[string]Posting)
	}

	posting, exists := idx.buffer[term][docID]
	if !exists {
		posting = Posting{DocID: docID}
	}
	posting.Frequency++
	posting.Positions = append(posting.Positions, 0) // single token, always position 0
	idx.buffer[term][docID] = posting

	idx.bufferDocs[docID] = struct{}{}
	idx.docCount++
	idx.tokenCount++

	if (idx.flushPolicy.MaxTokens > 0 && idx.tokenCount >= idx.flushPolicy.MaxTokens) ||
		(idx.flushSize > 0 && len(idx.bufferDocs) >= idx.flushSize) {
		idx.Flush()
		idx.tokenCount = 0
	}
}

// searches buffer + all segments, filters tombstones
func (idx *Index) Lookup(term string) []Posting {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// search in buffer
	postingsMap := make(map[string]Posting)
	if docPostings, exists := idx.buffer[term]; exists {
		for docID, posting := range docPostings {

			if _, deleted := idx.tombstones[docID]; !deleted {
				postingsMap[docID] = posting
			}
		}
	}

	waitGroup := sync.WaitGroup{}
	var mapLock sync.Mutex

	// search in segments in parallel
	for _, segment := range idx.segments {
		waitGroup.Add(1)
		go func(s *Segment) {
			defer waitGroup.Done()

			segmentPostings := s.lookup(term)
			if len(segmentPostings) == 0 {
				return
			}

			mapLock.Lock()
			for _, posting := range segmentPostings {
				if _, deleted := idx.tombstones[posting.DocID]; !deleted {
					postingsMap[posting.DocID] = posting
				}
			}
			mapLock.Unlock()
		}(segment)
	}

	waitGroup.Wait()

	if len(postingsMap) == 0 {
		return nil
	}

	postings := make([]Posting, 0, len(postingsMap))
	for _, posting := range postingsMap {
		postings = append(postings, posting)
	}

	return postings
}

// adds to tombstones only — O(1)
func (idx *Index) Delete(docID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.tombstones[docID] = struct{}{}
	idx.docCount--
}

func (idx *Index) Terms() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	termsMap := make(map[string]struct{})

	for term := range idx.buffer {
		termsMap[term] = struct{}{}
	}

	for _, segment := range idx.segments {
		terms := segment.terms()
		for _, term := range terms {
			termsMap[term] = struct{}{}
		}
	}

	terms := make([]string, 0, len(termsMap))
	for term := range termsMap {
		terms = append(terms, term)
	}

	return terms

}

func (idx *Index) DocCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.docCount
}

func (idx *Index) TermCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return len(idx.Terms())
}

func (idx *Index) Flush() {
	seg := newSegment(idx.buffer, idx.bufferDocs)
	idx.segments = append(idx.segments, seg)
	idx.buffer = make(map[string]map[string]Posting)
	idx.bufferDocs = make(map[string]struct{})

	if idx.mergePolicy.MaxSegments > 0 && len(idx.segments) > idx.mergePolicy.MaxSegments {
		if idx.merging.CompareAndSwap(0, 1) {
			go func() {
				defer idx.merging.Store(0)
				idx.Merge()
				idx.FlushNumeric()
			}()
		}
	}
}

func (idx *Index) Snapshot() ([]SegmentData, map[string]struct{}) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	segs := make([]SegmentData, len(idx.segments))
	for i, s := range idx.segments {
		segs[i] = SegmentData{Postings: s.postings, Docs: s.docs}
	}
	return segs, idx.tombstones
}

func (idx *Index) Restore(segs []SegmentData, tombstones map[string]struct{}) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.segments = make([]*Segment, len(segs))

	docCount := 0

	for i, s := range segs {
		idx.segments[i] = newSegment(s.Postings, s.Docs)

		for docID := range s.Docs {
			if _, deleted := tombstones[docID]; !deleted {
				docCount++
			}
		}
	}
	idx.tombstones = tombstones
	idx.docCount = docCount
}

// atomic merge
// 1. set the atomic flag with Merge on process, will block any flush
// 2. Store all the
func (idx *Index) Merge() {
	// 1. snapshot under read lock (cheap)
	idx.mu.RLock()
	segments := idx.segments
	tombstones := make(map[string]struct{}, len(idx.tombstones))
	for k := range idx.tombstones {
		tombstones[k] = struct{}{}
	}
	idx.mu.RUnlock()

	// 2. expensive work — no lock held
	mergedPostings := make(map[string]map[string]Posting)
	mergedDocs := make(map[string]struct{})
	for _, seg := range segments {
		for term, docPostings := range seg.postings {
			if _, exists := mergedPostings[term]; !exists {
				mergedPostings[term] = make(map[string]Posting)
			}
			for docID, posting := range docPostings {
				if _, deleted := tombstones[docID]; deleted {
					continue
				}
				mergedDocs[docID] = struct{}{}
				mergedPostings[term][docID] = posting
			}
		}
	}
	merged := newSegment(mergedPostings, mergedDocs)

	// 3. swap under write lock (cheap — O(1))
	idx.mu.Lock()
	// segments added DURING merge are at idx.segments[len(segments):]
	// keep them, only replace the ones we merged
	idx.segments = append([]*Segment{merged}, idx.segments[len(segments):]...)
	// only remove tombstones we already applied
	for id := range tombstones {
		delete(idx.tombstones, id)
	}
	idx.mu.Unlock()
}

func (idx *Index) PrefixSearch(prefix string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.trie.Search(prefix)
}

func (idx *Index) Segments() []*Segment {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.segments
}

func (idx *Index) clearTombstone(docID string) {
	_, wasTombstoned := idx.tombstones[docID]
	delete(idx.tombstones, docID)

	if wasTombstoned {
		if _, inBuffer := idx.bufferDocs[docID]; inBuffer {
			for term, docPostings := range idx.buffer {
				delete(docPostings, docID)
				if len(docPostings) == 0 {
					delete(idx.buffer, term)
				}
			}
		}
	}
}

func (idx *Index) AddNumeric(field, docID string, value float64) {
	idx.numeric.AddFloat(field, docID, value)
}

func (idx *Index) AddNumericInt(field, docID string, value int64) {
	idx.numeric.AddInt(field, docID, value)
}
func (idx *Index) DeleteNumeric(docID string) {
	idx.numeric.Delete(docID)
}
func (idx *Index) FlushNumeric() {
	idx.numeric.Flush()
}
func (idx *Index) RangeQuery(field string, gte, lte, gt, lt *float64) []string {
	return idx.numeric.Range(field, gte, lte, gt, lt)
}

func (idx *Index) AddFieldValue(field, docID, rawValue string, isNumeric bool) {
	idx.fieldIndex.Add(field, docID, rawValue, isNumeric)
	idx.fieldIndex.Sort()
}

func (idx *Index) DeleteFieldValues(docID string) {
	idx.fieldIndex.Delete(docID)
}

func (idx *Index) RebuildFieldIndex(numericFields, keywordFields map[string]bool) {
	idx.fieldIndex.Rebuild(idx, numericFields, keywordFields)
}

func (idx *Index) FieldRange(field string, gte, lte, gt, lt *float64) []string {
	return idx.fieldIndex.Range(field, gte, lte, gt, lt)
}

func (idx *Index) FieldSortValues(field string, desc bool, after *string) []FieldEntry {
	return idx.fieldIndex.SortValues(field, desc, after)
}
