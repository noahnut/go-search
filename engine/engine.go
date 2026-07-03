package engine

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/index"
	"github.com/noahfan/go-search/query"
	"github.com/noahfan/go-search/scoring"
)

const DefaultLargeDocThreshold = 64 * 1024 // 64 KB

type Field struct {
	Value  string
	Boost  float64   // score multiplier, 1.0 = no boost
	Vector []float64 // optional vector representation for semantic search
}

type Document struct {
	ID     string
	Fields map[string]Field // e.g. "title", "body", "tags"
}

type Result struct {
	ID     string
	Fields map[string]Field
	Score  float64
}

// Engine is the public SDK. Create one with New, then call Index and Search.
type Engine struct {
	index             *index.Index
	analyzer          *analysis.Analyzer
	docs              map[string]Document             // stores original text for results
	dataStore         *DocStore                       // stores large field chunks
	vectors           map[string]map[string][]float64 // docID → fieldName → vector
	largeDocThreshold int
	bm25Params        scoring.Params
	synonyms          analysis.SynonymMap
	docLengths        map[string]int
	mu                sync.RWMutex
}

func New(opts ...Option) *Engine {
	e := &Engine{
		index:             index.New(),
		analyzer:          analysis.NewAnalyzer(&analysis.StandardTokenizer{}),
		vectors:           make(map[string]map[string][]float64),
		docs:              make(map[string]Document),
		docLengths:        make(map[string]int),
		dataStore:         NewDocStore(),
		largeDocThreshold: DefaultLargeDocThreshold,
		bm25Params:        scoring.DefaultParams(),
		synonyms:          analysis.NewSynonymMap(nil),
		mu:                sync.RWMutex{},
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Index adds a document to the engine.
func (e *Engine) Index(doc Document) error {
	if doc.ID == "" {
		return errors.New("document ID is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.docs[doc.ID]; ok {
		e.index.Delete(doc.ID)
		for _, chunk := range e.dataStore.ChunksFor(doc.ID) {
			e.index.Delete(chunk.ChunkID)
		}
		e.dataStore.DeleteParent(doc.ID)
	}

	tempDocument := Document{ID: doc.ID, Fields: make(map[string]Field)}

	for fieldName, field := range doc.Fields {

		if field.Vector != nil {
			if e.vectors[doc.ID] == nil {
				e.vectors[doc.ID] = make(map[string][]float64)
			}
			e.vectors[doc.ID][fieldName] = field.Vector
		}

		if len(field.Value) > e.largeDocThreshold {
			chunks := e.splitIntoChunks(field.Value, 200, 20)
			path, err := e.writeToTempFile(field.Value)
			if err != nil {
				return err
			}

			for i, chunk := range chunks {
				chunkID := e.getChunkFileName(doc.ID, fieldName, i)
				e.index.Add(chunkID, chunk.value, &fieldName, e.analyzer)
				e.docLengths[chunkID] = len(e.analyzer.Analyze(chunk.value))
				e.dataStore.PutChunk(ChunkMeta{
					ChunkID:  chunkID,
					ParentID: doc.ID,
					Field:    fieldName,
					Meta: FieldMeta{
						FilePath: path,
						Offset:   chunk.offset,
						Length:   chunk.length,
						Boost:    field.Boost,
					},
				})

			}

		} else {
			tempDocument.Fields[fieldName] = field
			e.index.Add(doc.ID, field.Value, &fieldName, e.analyzer)
			e.docLengths[doc.ID+":"+fieldName] += len(e.analyzer.Analyze(field.Value))
		}

	}

	e.docs[doc.ID] = tempDocument

	return nil
}

// Search runs a boolean query and returns results ranked by BM25 score.
// topK limits the number of results returned.
func (e *Engine) Search(q query.Query, topK int) []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()
	candidateDocs := make(map[string]map[string]index.Posting)

	q = e.expandQueryWithSynonyms(q)

	result := make([]Result, 0)

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

		if parentID, ok := e.dataStore.ParentOf(docID); ok {
			resolvedDocID = parentID
		}

		totalScore := 0.0

		for _, clause := range q.Clauses {

			if clause.Type == query.MustNot {
				continue
			}
			_, ok := postings[clause.Field+":"+clause.Term]
			if !ok {
				continue
			}
			totalScore += scoring.Score(
				float64(postings[clause.Field+":"+clause.Term].Frequency),
				e.docLengths[resolvedDocID+":"+clause.Field],
				fieldAvgDocLens[clause.Field],
				e.index.DocCount(),
				len(e.index.Lookup(clause.Field+":"+clause.Term)),
				e.bm25Params,
				e.docs[resolvedDocID].Fields[clause.Field].Boost,
			)
		}

		if existing, ok := seen[resolvedDocID]; !ok || totalScore > existing.Score {
			seen[resolvedDocID] = Result{ID: resolvedDocID, Fields: e.docs[resolvedDocID].Fields, Score: totalScore}
		}
	}

	for _, res := range seen {
		result = append(result, res)
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

// Delete removes a document by ID.
func (e *Engine) Delete(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.docs, id)
	delete(e.docLengths, id)
	e.index.Delete(id)
	e.dataStore.DeleteParent(id)
}

// Size returns the number of indexed documents.
func (e *Engine) Size() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.docs)
}

// FuzzySearch finds all indexed terms within maxDistance of the query term
// in the given field, then returns BM25-ranked results.
func (e *Engine) FuzzySearch(field, term string, maxDistance int, topK int) []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	allIndexTerms := e.index.Terms()

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
				if parentID, ok := e.dataStore.ParentOf(post.DocID); ok {
					resolvedID = parentID
				}

				sc := scoring.Score(
					float64(post.Frequency),
					e.docLengths[resolvedID+":"+field],
					fieldAvgDocLens[field],
					e.index.DocCount(),
					len(e.index.Lookup(field+":"+rawTerm)),
					e.bm25Params,
					e.docs[resolvedID].Fields[field].Boost,
				)

				if existing, ok := seen[resolvedID]; !ok || sc > existing.Score {
					seen[resolvedID] = Result{ID: resolvedID, Fields: e.docs[resolvedID].Fields, Score: sc}
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
) []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	q = e.expandQueryWithSynonyms(q)

	bm25Results := e.Search(q, 0)
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

func (e *Engine) expandQueryWithSynonyms(q query.Query) query.Query {
	if len(e.synonyms) == 0 {
		return q
	}

	expandedClauses := make([]query.Clause, 0, len(q.Clauses))
	for _, clause := range q.Clauses {

		if clause.Type == query.MustNot || clause.Type == query.Phrase {
			expandedClauses = append(expandedClauses, clause)
			continue
		}

		synonyms := e.synonyms.Get(clause.Term)

		if len(synonyms) == 0 {
			expandedClauses = append(expandedClauses, clause)
			continue
		}

		expandedClauses = append(expandedClauses, query.Clause{
			Field: clause.Field,
			Term:  clause.Term,
			Type:  query.Should,
		})

		for _, syn := range synonyms {
			expandedClauses = append(expandedClauses, query.Clause{
				Field: clause.Field,
				Term:  syn,
				Type:  query.Should,
			})
		}
	}

	return query.Query{Clauses: expandedClauses}
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
			if parentID, ok := e.dataStore.ParentOf(post.DocID); ok {
				resolvedID = parentID
			}

			if duplicateDocIDs[resolvedID] {
				continue
			}
			duplicateDocIDs[resolvedID] = true

			sc := scoring.Score(
				float64(post.Frequency),
				e.docLengths[resolvedID+":"+field],
				0, // avgDocLen is not used for prefix search
				e.index.DocCount(),
				len(e.index.Lookup(term)),
				e.bm25Params,
				e.docs[resolvedID].Fields[field].Boost,
			)

			result = append(result, Result{ID: resolvedID, Fields: e.docs[resolvedID].Fields, Score: sc})
		}
	}

	return result
}

// IndexStruct indexes any struct annotated with `search` tags.
// The struct must have exactly one field tagged `search:"id"`.
// Returns an error if the id field is missing or empty.
//
//	type Article struct {
//	    ID    string `search:"id"`
//	    Title string `search:"field:title,boost:2.0"`
//	    Body  string `search:"field:body,boost:1.0"`
//	    Tags  string `search:"field:tags"`
//	}
func (e *Engine) IndexStruct(v any) error {

	if v == nil {
		return errors.New("IndexStruct: input is nil")
	}

	rv := reflect.ValueOf(v)

	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	rt := rv.Type()

	if rt.Kind() != reflect.Struct {
		return errors.New("IndexStruct: input must be a struct")
	}

	doc := Document{Fields: make(map[string]Field)}

	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("search")

		if tag == "id" {
			if rv.Field(i).Kind() != reflect.String {
				return fmt.Errorf("IndexStruct: id field %q must be a string", rt.Field(i).Name)
			}
			doc.ID = rv.Field(i).String()
		} else if strings.HasPrefix(tag, "field:") {
			if rv.Field(i).Kind() != reflect.String {
				return fmt.Errorf("IndexStruct: field %q is not a string", rt.Field(i).Name)
			}
			fieldValue := rv.Field(i).String()

			if fieldValue == "" {
				continue
			}

			boost := 1.0
			fieldName := ""

			tagParts := strings.Split(tag, ",")
			for _, part := range tagParts {
				if strings.HasPrefix(part, "boost:") {
					boostStr := strings.TrimPrefix(part, "boost:")
					var err error
					boost, err = strconv.ParseFloat(boostStr, 64)
					if err != nil {
						return errors.New("IndexStruct: invalid boost value")
					}
				} else if strings.HasPrefix(part, "field:") {
					fieldName = strings.TrimPrefix(part, "field:")

				}
			}

			doc.Fields[fieldName] = Field{Value: fieldValue, Boost: boost}
		}
	}

	if doc.ID == "" {
		return errors.New("IndexStruct: no field tagged search:\"id\"")
	}

	return e.Index(doc)
}
