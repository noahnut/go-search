package engine

type FieldType string

const (
	FieldTypeText    FieldType = "text"
	FieldTypeKeyword FieldType = "keyword"
	FieldTypeInteger FieldType = "integer"
	FieldTypeFloat   FieldType = "float"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeSkip    FieldType = "skip"
)

// FieldMapping describes how one field should be handled.
type FieldMapping struct {
	Type  FieldType
	Index bool // default true  — add to inverted index
	Store bool // default true  — include in Search results
}

// Mapping is a schema: field name → how to handle it.
// Fields not in the mapping use the dynamic default (text, indexed, stored).
type Mapping map[string]FieldMapping
