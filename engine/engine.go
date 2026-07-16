package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/noahfan/go-search/analysis"
	"github.com/noahfan/go-search/index"
	"github.com/noahfan/go-search/query"
	"github.com/noahfan/go-search/scoring"
	"github.com/noahfan/go-search/storage"
	"github.com/noahfan/go-search/storage/memory"
)

const DefaultLargeDocThreshold = 64 * 1024 // 64 KB
const SnapshotFileName = "snapshot.gob"

const HighlightMarkerOpen = "<em>"
const HighlightMarkerClose = "</em>"

type Field struct {
	Value  string    `json:"value"`
	Boost  float64   `json:"boost"`  // score multiplier, 1.0 = no boost
	Vector []float64 `json:"vector"` // optional vector representation for semantic search
}

type Document struct {
	ID     string           `json:"id"`
	Fields map[string]Field `json:"fields"` // e.g. "title", "body", "tags"
}

func (d Document) MarshalJSON() ([]byte, error) {
	type Alias Document
	fields := make(map[string]Field)
	for k, v := range d.Fields {
		fields[k] = v
	}

	return json.Marshal(&struct {
		ID     string           `json:"id"`
		Fields map[string]Field `json:"fields"`
	}{
		ID:     d.ID,
		Fields: fields,
	})
}

func (d *Document) UnmarshalJSON(data []byte) error {
	type Alias Document
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(d),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	return nil
}

type Result struct {
	ID         string
	Fields     map[string]Field
	Score      float64
	Highlights []Highlight // ← new; nil when no matches
}

// Engine is the public SDK. Create one with New, then call Index and Search.
type Engine struct {
	index             *index.Index
	analyzer          *analysis.Analyzer
	docStorage        storage.Storage                 // key-value storage for large documents
	vectors           map[string]map[string][]float64 // docID → fieldName → vector
	snapshotDir       string
	snapshotInterval  time.Duration
	largeDocThreshold int
	schema            *Schema
	bm25Params        scoring.Params
	synonyms          analysis.SynonymMap
	flushPolicy       *index.FlushPolicy
	mergePolicy       *index.MergePolicy
	docLengths        map[string]int
	mu                sync.RWMutex
	wg                sync.WaitGroup
	done              chan struct{}
}

// newBase creates the engine struct and applies options without any startup recovery.
// Used by both New and Load to avoid recursive snapshot loading.
func newBase(opts ...Option) *Engine {
	e := &Engine{
		analyzer:          analysis.NewAnalyzer(&analysis.StandardTokenizer{}),
		vectors:           make(map[string]map[string][]float64),
		docLengths:        make(map[string]int),
		schema:            NewSchema(),
		largeDocThreshold: DefaultLargeDocThreshold,
		bm25Params:        scoring.DefaultParams(),
		synonyms:          analysis.NewSynonymMap(nil),
		mu:                sync.RWMutex{},
		done:              make(chan struct{}),
	}
	for _, opt := range opts {
		opt(e)
	}

	e.index = index.New(e.docStorage, index.WithFlushPolicy(e.flushPolicy), index.WithMergePolicy(e.mergePolicy))

	if e.docStorage == nil {
		e.docStorage = memory.New()
	}
	return e
}

func New(opts ...Option) *Engine {
	e := newBase(opts...)

	if e.docStorage.Type() == storage.LocalFileStorage {
		if e.snapshotDir != "" {
			snapShotFile := filepath.Join(e.snapshotDir, SnapshotFileName)
			if _, err := os.Stat(snapShotFile); err == nil {
				if loaded, err := Load(snapShotFile, opts...); err == nil {
					e = loaded
				}
			}
		}
		// re-index all docs not already in the snapshot (or everything if no snapshot)
		if err := e.recoverDelta(); err != nil {
			fmt.Printf("Error recovering delta: %v\n", err)
		}
	}

	if e.snapshotDir != "" && e.snapshotInterval > 0 {
		e.periodicSnapshot()
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

	if e.docStorage.Has(doc.ID) {
		e.index.Delete(doc.ID)
		e.docStorage.Delete(doc.ID)
	}

	tempDocument := Document{ID: doc.ID, Fields: make(map[string]Field)}

	for fieldName, field := range doc.Fields {

		fm, err := e.schema.Resolve(fieldName, field.Value)
		if err != nil {
			return err
		}

		if fm.Type == FieldTypeSkip {
			continue
		}

		if fm.Store {
			tempDocument.Fields[fieldName] = field
		}

		if fm.Index {
			switch fm.Type {
			case FieldTypeInteger:
				e.index.AddRaw(doc.ID, fieldName, field.Value)
				if intValue, err := strconv.ParseInt(field.Value, 10, 64); err == nil {
					e.index.AddNumericInt(fieldName, doc.ID, intValue)
					e.index.AddFieldValue(fieldName, doc.ID, field.Value, true)
				}
			case FieldTypeFloat:
				e.index.AddRaw(doc.ID, fieldName, field.Value)
				if floatValue, err := strconv.ParseFloat(field.Value, 64); err == nil {
					e.index.AddNumeric(fieldName, doc.ID, floatValue)
					e.index.AddFieldValue(fieldName, doc.ID, field.Value, true)
				}
			case FieldTypeBoolean:
				e.index.AddRaw(doc.ID, fieldName, field.Value)
				if boolValue, err := strconv.ParseBool(field.Value); err == nil {
					intVal := int64(0)
					if boolValue {
						intVal = 1
					}
					e.index.AddNumericInt(fieldName, doc.ID, intVal)
				}
			case FieldTypeKeyword:
				e.index.AddRaw(doc.ID, fieldName, field.Value)
				e.index.AddFieldValue(fieldName, doc.ID, field.Value, false)
			case FieldTypeText:
				e.index.Add(doc.ID, field.Value, &fieldName, e.analyzer)
				e.index.AddFieldValue(fieldName, doc.ID, field.Value, false)
			}
		}

		if field.Vector != nil {
			if e.vectors[doc.ID] == nil {
				e.vectors[doc.ID] = make(map[string][]float64)
			}
			e.vectors[doc.ID][fieldName] = field.Vector
		}

		e.docLengths[doc.ID+":"+fieldName] += len(e.analyzer.Analyze(field.Value))
	}

	documentJSON, err := json.Marshal(tempDocument)

	if err != nil {
		return err
	}

	e.docStorage.Put(doc.ID, documentJSON) // store an empty value to indicate the doc exists

	return nil
}

// Delete removes a document by ID.
func (e *Engine) Delete(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k := range e.docLengths {
		if strings.HasPrefix(k, id+":") {
			delete(e.docLengths, k)
		}
	}
	e.index.Delete(id)
	e.index.DeleteNumeric(id)
	e.index.DeleteFieldValues(id)
	e.docStorage.Delete(id)
}

// Size returns the number of indexed documents.
func (e *Engine) Size() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.docStorage.Size()
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

func (e *Engine) Close() error {
	if e.done != nil {
		close(e.done)
	}
	e.wg.Wait()
	if err := e.Snapshot(); err != nil {
		return err
	}
	return e.docStorage.Close()
}

func (e *Engine) Schema() *Schema {
	return e.schema
}

func (e *Engine) Reindex(parallelism int, opts ...Option) error {
	if e.docStorage.Type() == storage.MemoryStorage {
		return errors.New("reindex requires a storage backend")
	}

	// Build into a shadow engine — never touches the live engine.
	// Carry the current explicit schema so field types stay consistent.
	shadowOpts := append([]Option{
		WithDocStorage(e.docStorage),
		WithMapping(e.schema.Fields()),
	}, opts...)
	shadow := newBase(shadowOpts...)

	if err := buildIndex(shadow, e.docStorage, parallelism); err != nil {
		return err
	}

	// Atomic swap — searches in flight finish on old state; new ones see shadow.
	e.mu.Lock()
	e.index = shadow.index
	e.docLengths = shadow.docLengths
	e.schema = shadow.schema
	e.mu.Unlock()

	return nil
}

func buildIndex(target *Engine, store storage.Storage, parallelism int) error {
	type job struct{ raw []byte }
	jobs := make(chan job, parallelism*2)
	errc := make(chan error, parallelism)

	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				var doc Document
				if err := json.Unmarshal(j.raw, &doc); err != nil {
					errc <- err
					return
				}
				if err := target.Index(doc); err != nil {
					errc <- err
					return
				}
			}
		}()
	}

	go func() {
		store.Each(func(_ string, v []byte) { jobs <- job{v} })
		close(jobs)
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) periodicSnapshot() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(e.snapshotInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := e.Snapshot(); err != nil {
					fmt.Printf("Error during periodic snapshot: %v\n", err)
				}
			case <-e.done:
				return
			}
		}
	}()
}
