package index

// Segment is an immutable mini-index flushed from the write buffer.
type Segment struct {
	postings map[string]map[string]Posting // term → docID → Posting
	docs     map[string]struct{}
}

func newSegment(postings map[string]map[string]Posting, docs map[string]struct{}) *Segment {
	return &Segment{
		postings: postings,
		docs:     docs,
	}
}

func (s *Segment) Docs() map[string]struct{} {
	return s.docs
}

func (s *Segment) lookup(term string) []Posting {
	postings, ok := s.postings[term]
	if !ok {
		return nil
	}
	postingsList := make([]Posting, 0, len(postings))
	for _, posting := range postings {
		postingsList = append(postingsList, posting)
	}
	return postingsList
}

func (s *Segment) terms() []string {
	allTerms := make([]string, 0, len(s.postings))
	for term := range s.postings {
		allTerms = append(allTerms, term)
	}

	return allTerms
}
