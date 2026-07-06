package engine

import (
	"strconv"
	"strings"
	"sync"
)

type FieldType string

const (
	FieldTypeText    FieldType = "text"
	FieldTypeKeyword FieldType = "keyword"
	FieldTypeInteger FieldType = "integer"
	FieldTypeFloat   FieldType = "float"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeSkip    FieldType = "skip"
)

type FieldTypeConflictError struct {
	FieldName    string
	ExistingType FieldType
	NewType      FieldType
}

func (e *FieldTypeConflictError) Error() string {
	return "field type conflict for field '" + e.FieldName + "': existing type '" + string(e.ExistingType) + "', new type '" + string(e.NewType) + "'"
}

// FieldMapping describes how one field should be handled.
type FieldMapping struct {
	Type     FieldType
	Index    bool // default true  — add to inverted index
	Store    bool // default true  — include in Search results
	Explicit bool
}

// Mapping is a schema: field name → how to handle it.
// Fields not in the mapping use the dynamic default (text, indexed, stored).
type Mapping map[string]FieldMapping

func InferType(value string) FieldType {

	if _, err := strconv.Atoi(value); err == nil {
		return FieldTypeInteger
	}

	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return FieldTypeFloat
	}

	if value == "true" || value == "false" {
		return FieldTypeBoolean
	}

	// no spaces and len < 64 → FieldTypeKeyword  (category, status, tag)
	if !strings.Contains(value, " ") && len(value) < 64 {
		return FieldTypeKeyword
	}

	// otherwise → FieldTypeText
	return FieldTypeText
}

type Schema struct {
	mu     sync.RWMutex
	fields map[string]FieldMapping // field name → resolved mapping
}

func NewSchema() *Schema {
	return &Schema{
		fields: make(map[string]FieldMapping),
		mu:     sync.RWMutex{},
	}
}

// Resolve returns the mapping for a field, inferring it from value if not yet known.
// Returns an error if the inferred type conflicts with a previously locked type.
func (s *Schema) Resolve(fieldName, value string) (FieldMapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inferredType := InferType(value)

	if existingFM, ok := s.get(fieldName); ok {
		if existingFM.Explicit {
			// explicit mapping is authoritative — always return it as-is
			return existingFM, nil
		}
		if existingFM.Type != inferredType {
			// two inferred types disagree — promote to text (most general)
			promoted := FieldMapping{Type: FieldTypeText, Index: true, Store: true}
			s.fields[fieldName] = promoted
			return promoted, nil
		}
		return existingFM, nil
	}

	fm := FieldMapping{Type: inferredType, Index: true, Store: true}
	s.fields[fieldName] = fm
	return fm, nil
}

// Set explicitly sets a field mapping (from WithMapping). Overrides any inference.
func (s *Schema) Set(fieldName string, fm FieldMapping) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fm.Explicit = true
	s.fields[fieldName] = fm
}

// Get returns the current mapping for a field, if known.
func (s *Schema) Get(fieldName string) (FieldMapping, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.get(fieldName)
}

// Fields returns a snapshot of all field mappings (for inspection).
func (s *Schema) Fields() map[string]FieldMapping {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fieldsCopy := make(map[string]FieldMapping, len(s.fields))
	for k, v := range s.fields {
		fieldsCopy[k] = v
	}
	return fieldsCopy
}

func (s *Schema) get(fieldName string) (FieldMapping, bool) {
	fm, ok := s.fields[fieldName]
	return fm, ok
}
