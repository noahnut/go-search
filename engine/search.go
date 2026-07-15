package engine

import (
	"regexp"
	"sort"
	"strconv"
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
	candidateDocs := make(map[string]map[string]index.Posting)

	q = e.expandQueryWithSynonyms(q)

	result := make([]Result, 0)

	fieldAvgDocLens := e.fieldAvgDocLens()

	if len(q.Clauses) == 0 && len(q.Ranges) > 0 {
		for _, rc := range q.Ranges {
			for _, docID := range e.index.RangeQuery(rc.Field, rc.Gte, rc.Lte, rc.Gt, rc.Lt) {
				candidateDocs[docID] = map[string]index.Posting{}
			}
		}
	}

	for _, clause := range q.Clauses {

		switch clause.Type {
		case query.Phrase:
			for _, term := range strings.Fields(clause.Term) {
				indexKey := clause.Field + ":" + term
				postings := e.index.Lookup(indexKey)
				if len(postings) == 0 {
					continue
				}
				for _, posting := range e.index.Lookup(indexKey) {
					if candidateDocs[posting.DocID] == nil {
						candidateDocs[posting.DocID] = make(map[string]index.Posting)
					}
					candidateDocs[posting.DocID][term] = posting
				}
			}
			continue
		default:
			indexKey := clause.Field + ":" + clause.Term
			postings := e.index.Lookup(indexKey)
			if len(postings) == 0 {
				continue
			}

			for _, posting := range postings {
				if _, ok := candidateDocs[posting.DocID]; !ok {
					candidateDocs[posting.DocID] = make(map[string]index.Posting)
				}
				candidateDocs[posting.DocID][clause.Field+":"+clause.Term] = posting
			}
		}
	}

	seen := map[string]Result{}
	for docID, postings := range candidateDocs {
		if !query.Match(q, postings) {
			continue
		}

		resolvedDocID := docID

		totalScore := 0.0

		document, exists := e.docStorage.Get(resolvedDocID)
		if !exists {
			continue
		}

		var doc Document
		if err := doc.UnmarshalJSON(document); err != nil {
			continue
		}

		if passesRangeFilters(doc, q.Ranges) == false {
			continue
		}

		if passTermsFilters(doc, q.Terms) == false {
			continue
		}

		if passRegexFilters(doc, q.Regexes) == false {
			continue
		}

		var terms []string

		for _, clause := range q.Clauses {

			if clause.Type == query.MustNot {
				continue
			}
			_, ok := postings[clause.Field+":"+clause.Term]
			if !ok {
				continue
			}

			if clause.Type == query.Must || clause.Type == query.Should {
				terms = append(terms, clause.Term)
			}

			totalScore += scoring.Score(
				float64(postings[clause.Field+":"+clause.Term].Frequency),
				e.docLengths[resolvedDocID+":"+clause.Field],
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

		if existing, ok := seen[resolvedDocID]; !ok || totalScore > existing.Score {
			highlights := HighlightDoc(Document{ID: doc.ID, Fields: textFields}, terms, e.analyzer, HighlightMarkerOpen, HighlightMarkerClose)
			seen[resolvedDocID] = Result{ID: resolvedDocID, Fields: doc.Fields, Score: totalScore, Highlights: highlights}
		}
	}

	for _, res := range seen {
		result = append(result, res)
	}

	// opts is variadic for backward compatibility; nil means "use defaults".
	var searchOpts *SearchOptions
	if len(opts) > 0 {
		searchOpts = &opts[0]
	}

	if searchOpts != nil && len(searchOpts.Sort) > 0 {
		result = e.Sort(result, *searchOpts)
	} else {
		sort.Slice(result, func(i, j int) bool {
			return result[i].Score > result[j].Score
		})
	}

	if searchOpts != nil && searchOpts.SearchAfter != nil {
		cursor := searchOpts.SearchAfter
		found := false
		for i, r := range result {
			if r.ID == cursor.DocID {
				result = result[i+1:]
				found = true
			}
		}

		if !found {
			result = nil
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

	searchResult := SearchResult{Hits: result}
	if searchOpts != nil && len(searchOpts.Sort) > 0 && len(searchResult.Hits) > 0 {
		last := searchResult.Hits[len(searchResult.Hits)-1]
		searchResult.NextCursor = &SearchCursor{
			SortValue: last.Fields[searchOpts.Sort[0].Field].Value,
			DocID:     last.ID,
		}
	}

	return searchResult
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

func passesRangeFilters(doc Document, ranges []query.RangeClause) bool {
	for _, rc := range ranges {
		field, ok := doc.Fields[rc.Field]
		if !ok {
			return false // field absent → exclude
		}
		val, err := strconv.ParseFloat(field.Value, 64)
		if err != nil {
			return false // non-numeric → exclude
		}
		if rc.Gte != nil && val < *rc.Gte {
			return false
		}
		if rc.Lte != nil && val > *rc.Lte {
			return false
		}
		if rc.Gt != nil && val <= *rc.Gt {
			return false
		}
		if rc.Lt != nil && val >= *rc.Lt {
			return false
		}
	}
	return true
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
