package index

import (
	"sync"

	"github.com/noahfan/go-search/analysis"
)

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

// Index is an inverted index: maps each term to its list of postings.
type Index struct {
	mu         sync.RWMutex
	trie       *Trie
	buffer     map[string]map[string]Posting // in-memory write buffer
	bufferDocs map[string]struct{}
	segments   []*Segment          // immutable flushed segments
	tombstones map[string]struct{} // deleted docIDs
	flushSize  int                 // flush buffer → segment when buffer hits this
	docCount   int                 // total number of unique documents in the index
}

func New() *Index {
	return &Index{
		buffer:     make(map[string]map[string]Posting),
		bufferDocs: make(map[string]struct{}),
		segments:   []*Segment{},
		tombstones: make(map[string]struct{}),
		trie:       NewTrie(),
		flushSize:  128,
		docCount:   0,
	}
}

// flushSize = 128 by default
func NewWithFlushSize(n int) *Index {
	return &Index{
		buffer:     make(map[string]map[string]Posting),
		bufferDocs: make(map[string]struct{}),
		segments:   []*Segment{},
		tombstones: make(map[string]struct{}),
		trie:       NewTrie(),
		flushSize:  n,
		docCount:   0,
	}
}

// public API stays the same:
func (idx *Index) Add(docID string, text string, fieldName *string, analyzer *analysis.Analyzer) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// tokenize the text
	tokens := analyzer.Analyze(text)

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
	}

	idx.bufferDocs[docID] = struct{}{}
	idx.docCount++

	if len(idx.bufferDocs) >= idx.flushSize {
		idx.Flush()
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

func (idx *Index) Merge() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	mergedPostings := make(map[string]map[string]Posting)
	mergedDocs := make(map[string]struct{})

	for _, segment := range idx.segments {
		for term, docPostings := range segment.postings {
			if _, exists := mergedPostings[term]; !exists {
				mergedPostings[term] = make(map[string]Posting)
			}

			for docID, posting := range docPostings {
				if _, deleted := idx.tombstones[docID]; deleted {
					continue
				}
				mergedDocs[docID] = struct{}{}

				mergedPostings[term][docID] = posting
			}

		}
	}

	idx.segments = []*Segment{newSegment(mergedPostings, mergedDocs)}
	idx.tombstones = make(map[string]struct{})
}

func (idx *Index) PrefixSearch(prefix string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.trie.Search(prefix)
}
