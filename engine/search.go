package engine

import (
	"regexp"
	"sort"
	"strings"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/index"
	"github.com/noahfan/go-search/query"
	"github.com/noahfan/go-search/scoring"
)

type SearchOptions struct {
	Sort        []SortClause  // if empty, sort by score descending (current behaviour)
	From        int           // skip this many results (offset pagination)
	Size        int           // max results to return (0 = use topK)
	SearchAfter *SearchCursor // keyset pagination
}

type SearchCursor struct {
	SortValue string // the sort field value of the last doc
	DocID     string // tiebreaker — the last doc's ID
}

type SearchResult struct {
	Hits       []Result
	NextCursor *SearchCursor // nil if no more pages
}

// Search runs a boolean query and returns results ranked by BM25 score.
// topK limits the number of results returned.
func (e *Engine) Search(q query.Query, topK int, opts ...SearchOptions) SearchResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	q = e.expandQueryWithSynonyms(q)
	fieldAvgDocLens := e.fieldAvgDocLens()

	var searchOpts *SearchOptions
	if len(opts) > 0 {
		searchOpts = &opts[0]
	}

	candidateDocs := make(map[string]map[string]index.Posting)

	if len(q.Clauses) == 0 && len(q.Ranges) > 0 {
		for _, docID := range e.index.MultiRangeQuery(rangesToFieldBounds(q.Ranges)) {
			candidateDocs[docID] = map[string]index.Posting{}
		}
	}

	for _, clause := range q.Clauses {
		switch clause.Type {
		case query.Phrase:
			for _, term := range strings.Fields(clause.Term) {
				indexKey := clause.Field + ":" + term
				if postings := e.index.Lookup(indexKey); len(postings) > 0 {
					for _, posting := range postings {
						if candidateDocs[posting.DocID] == nil {
							candidateDocs[posting.DocID] = make(map[string]index.Posting)
						}
						candidateDocs[posting.DocID][term] = posting
					}
				}
			}
		default:
			indexKey := clause.Field + ":" + clause.Term
			for _, posting := range e.index.Lookup(indexKey) {
				if _, ok := candidateDocs[posting.DocID]; !ok {
					candidateDocs[posting.DocID] = make(map[string]index.Posting)
				}
				candidateDocs[posting.DocID][clause.Field+":"+clause.Term] = posting
			}
		}
	}

	// Pre-filter text candidates by range using KDTree to reduce Bitcask reads.
	if len(q.Ranges) > 0 && len(q.Clauses) > 0 && len(candidateDocs) > 0 {
		rangeSet := make(map[string]struct{})
		for _, docID := range e.index.MultiRangeQuery(rangesToFieldBounds(q.Ranges)) {
			rangeSet[docID] = struct{}{}
		}
		for docID := range candidateDocs {
			if _, ok := rangeSet[docID]; !ok {
				delete(candidateDocs, docID)
			}
		}
	}

	var result []Result

	// Sort-first fast path: when the sort field is in the FieldIndex, walk entries
	// in sorted order and Bitcask-fetch only for docs that pass all filters.
	// This reduces Bitcask reads from O(candidates) to O(topK) for sort queries.
	if searchOpts != nil && len(searchOpts.Sort) > 0 {
		sc := searchOpts.Sort[0]
		if sortedEntries := e.index.FieldSortValues(sc.Field, sc.Order == Desc, nil); sortedEntries != nil {
			result = e.sortFirstSearch(q, candidateDocs, sortedEntries, fieldAvgDocLens, topK, searchOpts)
			return e.applyPagination(result, topK, searchOpts)
		}
	}

	// Score-first fallback: score all candidates, then sort.
	seen := map[string]Result{}
	for docID, postings := range candidateDocs {
		if !query.Match(q, postings) {
			continue
		}
		r, ok := e.scoreDoc(docID, postings, q, fieldAvgDocLens)
		if !ok {
			continue
		}
		if existing, exists := seen[docID]; !exists || r.Score > existing.Score {
			seen[docID] = r
		}
	}
	for _, r := range seen {
		result = append(result, r)
	}
	if searchOpts != nil && len(searchOpts.Sort) > 0 {
		result = e.Sort(result, *searchOpts)
	} else {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Score > result[j].Score
		})
	}
	return e.applyPagination(result, topK, searchOpts)
}

// sortFirstSearch walks sortedEntries in sort order and Bitcask-fetches only for
// qualifying docs, stopping once enough results are collected for the current page.
// Docs without the sort field are appended at the end, sorted by BM25 score.
func (e *Engine) sortFirstSearch(
	q query.Query,
	candidateDocs map[string]map[string]index.Posting,
	sortedEntries []index.FieldEntry,
	fieldAvgDocLens map[string]float64,
	topK int,
	searchOpts *SearchOptions,
) []Result {
	// Pre-qualify candidates using only in-memory state (no Bitcask reads).
	qualified := make(map[string]map[string]index.Posting, len(candidateDocs))
	for docID, postings := range candidateDocs {
		if query.Match(q, postings) {
			qualified[docID] = postings
		}
	}

	// Effective limit for early stop.
	// SearchAfter disables it: we must scan until we find the cursor position.
	limit := topK
	if searchOpts.SearchAfter != nil {
		limit = 0
	} else if searchOpts.Size > 0 {
		need := searchOpts.From + searchOpts.Size
		if limit <= 0 || need < limit {
			limit = need
		}
	}

	var result []Result
	inSortField := make(map[string]struct{})

	for _, fe := range sortedEntries {
		inSortField[fe.DocID] = struct{}{}
		postings, ok := qualified[fe.DocID]
		if !ok {
			continue
		}
		r, ok := e.scoreDoc(fe.DocID, postings, q, fieldAvgDocLens)
		if !ok {
			continue
		}
		result = append(result, r)
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	// Within equal sort field values, rank by BM25 score descending.
	// sort.SliceStable preserves FieldIndex order for non-equal values.
	sortField := searchOpts.Sort[0].Field
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Fields[sortField].Value == result[j].Fields[sortField].Value {
			return result[i].Score > result[j].Score
		}
		return false
	})

	// Docs without the sort field: collect at end, sorted by BM25 score.
	// Skipped when early stop has already filled the page.
	if limit <= 0 || len(result) < limit {
		var tail []Result
		for docID, postings := range qualified {
			if _, has := inSortField[docID]; has {
				continue
			}
			r, ok := e.scoreDoc(docID, postings, q, fieldAvgDocLens)
			if !ok {
				continue
			}
			tail = append(tail, r)
		}
		sort.Slice(tail, func(i, j int) bool { return tail[i].Score > tail[j].Score })
		if limit > 0 {
			if remaining := limit - len(result); len(tail) > remaining {
				tail = tail[:remaining]
			}
		}
		result = append(result, tail...)
	}

	return result
}

// scoreDoc fetches a doc from storage, applies all filters, and returns a scored Result.
func (e *Engine) scoreDoc(docID string, postings map[string]index.Posting, q query.Query, fieldAvgDocLens map[string]float64) (Result, bool) {
	rawDoc, exists := e.docStorage.Get(docID)
	if !exists {
		return Result{}, false
	}
	var doc Document
	if err := doc.UnmarshalJSON(rawDoc); err != nil {
		return Result{}, false
	}
	if !passTermsFilters(doc, q.Terms) {
		return Result{}, false
	}
	if !passRegexFilters(doc, q.Regexes) {
		return Result{}, false
	}

	totalScore := 0.0
	var terms []string
	for _, clause := range q.Clauses {
		if clause.Type == query.MustNot {
			continue
		}
		p, ok := postings[clause.Field+":"+clause.Term]
		if !ok {
			continue
		}
		if clause.Type == query.Must || clause.Type == query.Should {
			terms = append(terms, clause.Term)
		}
		totalScore += scoring.Score(
			float64(p.Frequency),
			e.docLengths[docID+":"+clause.Field],
			fieldAvgDocLens[clause.Field],
			e.index.DocCount(),
			len(e.index.Lookup(clause.Field+":"+clause.Term)),
			e.bm25Params,
			doc.Fields[clause.Field].Boost,
		)
	}

	textFields := make(map[string]Field)
	for name, field := range doc.Fields {
		fm, ok := e.schema.Get(name)
		if !ok || fm.Type == FieldTypeText {
			textFields[name] = field
		}
	}
	highlights := HighlightDoc(Document{ID: doc.ID, Fields: textFields}, terms, e.analyzer, HighlightMarkerOpen, HighlightMarkerClose)
	return Result{ID: docID, Fields: doc.Fields, Score: totalScore, Highlights: highlights}, true
}

// applyPagination handles SearchAfter / From+Size / topK on an already-ordered result slice
// and attaches NextCursor when a sort is active.
func (e *Engine) applyPagination(result []Result, topK int, searchOpts *SearchOptions) SearchResult {
	if searchOpts != nil && searchOpts.SearchAfter != nil {
		cutIdx := -1
		for i, r := range result {
			if r.ID == searchOpts.SearchAfter.DocID {
				cutIdx = i + 1
				break
			}
		}
		if cutIdx < 0 {
			result = nil
		} else {
			result = result[cutIdx:]
		}
		if searchOpts.Size > 0 && len(result) > searchOpts.Size {
			result = result[:searchOpts.Size]
		}
	} else if searchOpts != nil && (searchOpts.From > 0 || searchOpts.Size > 0) {
		start := searchOpts.From
		if start > len(result) {
			return SearchResult{Hits: []Result{}}
		}
		end := len(result)
		if searchOpts.Size > 0 && start+searchOpts.Size < end {
			end = start + searchOpts.Size
		}
		result = result[start:end]
	}

	if topK > 0 && len(result) > topK {
		result = result[:topK]
	}

	sr := SearchResult{Hits: result}
	if searchOpts != nil && len(searchOpts.Sort) > 0 && len(sr.Hits) > 0 {
		last := sr.Hits[len(sr.Hits)-1]
		sr.NextCursor = &SearchCursor{
			SortValue: last.Fields[searchOpts.Sort[0].Field].Value,
			DocID:     last.ID,
		}
	}
	return sr
}

// FuzzySearch finds all indexed terms within maxDistance of the query term
// in the given field, then returns BM25-ranked results.
func (e *Engine) FuzzySearch(field, term string, maxDistance int, topK int) []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	allIndexTerms := e.index.Terms()

	fieldAvgDocLens := e.fieldAvgDocLens()

	seen := map[string]Result{}

	for _, indexTerm := range allIndexTerms {
		splitWord := strings.Split(indexTerm, ":")
		indexfield := splitWord[0]
		rawTerm := splitWord[1]

		if field != indexfield {
			continue
		}

		if analysis.EditDistance(term, rawTerm) <= maxDistance {
			indexPositing := e.index.Lookup(field + ":" + rawTerm)

			for _, post := range indexPositing {
				resolvedID := post.DocID

				rawDocument, exists := e.docStorage.Get(resolvedID) // ensure the document exists

				if !exists {
					continue
				}

				var doc Document
				if err := doc.UnmarshalJSON(rawDocument); err != nil {
					continue
				}

				sc := scoring.Score(
					float64(post.Frequency),
					e.docLengths[resolvedID+":"+field],
					fieldAvgDocLens[field],
					e.index.DocCount(),
					len(e.index.Lookup(field+":"+rawTerm)),
					e.bm25Params,
					doc.Fields[field].Boost,
				)

				if existing, ok := seen[resolvedID]; !ok || sc > existing.Score {
					seen[resolvedID] = Result{ID: resolvedID, Fields: doc.Fields, Score: sc}
				}
			}
		}
	}

	result := make([]Result, 0, len(seen))
	for _, r := range seen {
		result = append(result, r)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	if topK <= 0 {
		return result
	}

	if len(result) < topK {
		topK = len(result)
	}

	return result[:topK]
}

// HybridSearch combines BM25 keyword search with vector similarity search.
// alpha controls the weight of BM25: 0.0 = pure vector, 1.0 = pure BM25.
// field is the vector field to search. q is the keyword query.
// queryVector is the semantic query embedding.
func (e *Engine) HybridSearch(
	q query.Query,
	field string,
	queryVector []float64,
	alpha float64,
	topK int,
	opts ...SearchOptions,
) []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	q = e.expandQueryWithSynonyms(q)

	bm25Results := e.Search(q, 0, opts...).Hits
	vectorResults := e.VectorSearch(field, queryVector, 0)

	// Create a map for quick lookup of vector scores by document ID
	vectorScores := make(map[string]float64)
	for _, res := range vectorResults {
		vectorScores[res.ID] = res.Score
	}

	// Combine BM25 and vector scores
	combinedResults := make([]Result, 0)
	for _, bm25Res := range bm25Results {
		vectorScore, exists := vectorScores[bm25Res.ID]
		if !exists {
			vectorScore = 0.0
		}
		combinedScore := alpha*bm25Res.Score + (1-alpha)*vectorScore
		if combinedScore > 0 {
			combinedResults = append(combinedResults, Result{
				ID:     bm25Res.ID,
				Fields: bm25Res.Fields,
				Score:  combinedScore,
			})
		}
	}

	// collect docIDs already added from BM25 pass
	seen := map[string]bool{}
	for _, r := range combinedResults {
		seen[r.ID] = true
	}

	// second pass: add docs that only appear in vector results
	for _, vecRes := range vectorResults {
		if seen[vecRes.ID] {
			continue
		}
		combinedScore := (1 - alpha) * vecRes.Score
		if combinedScore > 0 {
			combinedResults = append(combinedResults, Result{
				ID:     vecRes.ID,
				Fields: vecRes.Fields,
				Score:  combinedScore,
			})
		}
	}

	sort.Slice(combinedResults, func(i, j int) bool {
		return combinedResults[i].Score > combinedResults[j].Score
	})

	if topK <= 0 {
		return combinedResults
	}

	if len(combinedResults) < topK {
		topK = len(combinedResults)
	}

	return combinedResults[:topK]
}

func (e *Engine) WildcardSearch(field, pattern string, topK int) []Result {

	regex := wildcardToRegexp(pattern)

	e.mu.RLock()
	defer e.mu.RUnlock()

	matchingTerms := e.matchingTerm(field, regex)

	fieldAvgDocLens := e.fieldAvgDocLens()

	var result []Result
	seen := map[string]Result{}
	for _, term := range matchingTerms {
		postings := e.index.Lookup(term)
		for _, post := range postings {
			resolvedID := post.DocID

			rawDocument, exists := e.docStorage.Get(resolvedID) // ensure the document exists

			if !exists {
				continue
			}

			var doc Document
			if err := doc.UnmarshalJSON(rawDocument); err != nil {
				continue
			}

			fieldName := strings.SplitN(term, ":", 2)[0]
			sc := scoring.Score(
				float64(post.Frequency),
				e.docLengths[resolvedID+":"+fieldName],
				fieldAvgDocLens[fieldName],
				e.index.DocCount(),
				len(e.index.Lookup(term)),
				e.bm25Params,
				doc.Fields[fieldName].Boost,
			)

			if existing, ok := seen[resolvedID]; !ok || sc > existing.Score {
				seen[resolvedID] = Result{ID: resolvedID, Fields: doc.Fields, Score: sc}
			}
		}
	}

	for _, r := range seen {
		result = append(result, r)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	if topK <= 0 {
		return result
	}

	if len(result) < topK {
		topK = len(result)
	}

	return result[:topK]
}

func (e *Engine) PrefixSearch(field, prefix string) []Result {
	prefixWithField := field + ":" + prefix

	resultString := e.index.PrefixSearch(prefixWithField)

	result := make([]Result, 0)

	duplicateDocIDs := make(map[string]bool)

	for _, term := range resultString {
		positing := e.index.Lookup(term)
		for _, post := range positing {
			resolvedID := post.DocID

			if duplicateDocIDs[resolvedID] {
				continue
			}
			duplicateDocIDs[resolvedID] = true

			rawDocument, exists := e.docStorage.Get(resolvedID) // ensure the document exists
			if !exists {
				continue
			}

			var doc Document
			if err := doc.UnmarshalJSON(rawDocument); err != nil {
				continue
			}

			sc := scoring.Score(
				float64(post.Frequency),
				e.docLengths[resolvedID+":"+field],
				0, // avgDocLen is not used for prefix search
				e.index.DocCount(),
				len(e.index.Lookup(term)),
				e.bm25Params,
				doc.Fields[field].Boost,
			)

			result = append(result, Result{ID: resolvedID, Fields: doc.Fields, Score: sc})
		}
	}

	return result
}

func (e *Engine) matchingTerm(field string, re *regexp.Regexp) []string {
	prefix := field + ":"
	var matches []string
	for _, term := range e.index.Terms() {
		if !strings.HasPrefix(term, prefix) {
			continue
		}
		bare := strings.TrimPrefix(term, prefix)
		if re.MatchString(bare) {
			matches = append(matches, term)
		}
	}
	return matches
}

func (e *Engine) fieldAvgDocLens() map[string]float64 {
	fieldDocCounts := map[string]int{}
	fieldAvgDocLens := map[string]float64{}
	for key, docLen := range e.docLengths {
		fieldName := strings.SplitN(key, ":", 2)[1]
		fieldAvgDocLens[fieldName] += float64(docLen)
		fieldDocCounts[fieldName]++
	}
	for f := range fieldAvgDocLens {
		fieldAvgDocLens[f] /= float64(fieldDocCounts[f])
	}
	return fieldAvgDocLens
}

func wildcardToRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, ch := range pattern {
		switch ch {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func rangesToFieldBounds(ranges []query.RangeClause) []index.FieldBound {
	bounds := make([]index.FieldBound, len(ranges))
	for i, rc := range ranges {
		bounds[i] = index.FieldBound{Field: rc.Field, Gte: rc.Gte, Lte: rc.Lte, Gt: rc.Gt, Lt: rc.Lt}
	}
	return bounds
}


func passRegexFilters(doc Document, regexes []query.RegexClause) bool {
	for _, rc := range regexes {
		field, ok := doc.Fields[rc.Field]
		if !ok {
			return false // field absent → exclude
		}
		re, err := regexp.Compile(rc.Regex)
		if err != nil {
			return false // invalid regex → exclude
		}
		if !re.MatchString(field.Value) {
			return false // regex does not match → exclude
		}
	}
	return true
}

func passTermsFilters(doc Document, terms []query.TermsClause) bool {
	for _, tc := range terms {
		field, ok := doc.Fields[tc.Field]
		if !ok {
			return false // field absent → exclude
		}
		matched := false
		for _, v := range tc.Values {
			if field.Value == v {
				matched = true
				break
			}
		}
		if !matched {
			return false // none of the values match → exclude
		}
	}
	return true
}
