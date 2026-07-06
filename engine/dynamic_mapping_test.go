package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// --- InferType ---

func TestInferType_Boolean(t *testing.T) {
	if got := InferType("true"); got != FieldTypeBoolean {
		t.Errorf("InferType(\"true\"): want boolean, got %s", got)
	}
	if got := InferType("false"); got != FieldTypeBoolean {
		t.Errorf("InferType(\"false\"): want boolean, got %s", got)
	}
}

func TestInferType_Integer(t *testing.T) {
	cases := []string{"42", "-7", "0", "1000"}
	for _, v := range cases {
		if got := InferType(v); got != FieldTypeInteger {
			t.Errorf("InferType(%q): want integer, got %s", v, got)
		}
	}
}

func TestInferType_Float(t *testing.T) {
	cases := []string{"3.14", "-0.5", "1.0", "2.718"}
	for _, v := range cases {
		if got := InferType(v); got != FieldTypeFloat {
			t.Errorf("InferType(%q): want float, got %s", v, got)
		}
	}
}

func TestInferType_Keyword(t *testing.T) {
	cases := []string{"go", "machine-learning", "published", "status_ok"}
	for _, v := range cases {
		if got := InferType(v); got != FieldTypeKeyword {
			t.Errorf("InferType(%q): want keyword, got %s", v, got)
		}
	}
}

func TestInferType_Text(t *testing.T) {
	cases := []string{
		"Go is a fast language",
		"Lorem ipsum dolor sit amet",
	}
	for _, v := range cases {
		if got := InferType(v); got != FieldTypeText {
			t.Errorf("InferType(%q): want text, got %s", v, got)
		}
	}
}

func TestInferType_LongNoSpaceIsText(t *testing.T) {
	// 64-char string with no spaces should be text (len >= 64 threshold)
	long := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 64 chars
	if got := InferType(long); got != FieldTypeText {
		t.Errorf("InferType(64-char no-space): want text, got %s", got)
	}
}

// --- Schema.Resolve ---

func TestSchema_ResolveInfersAndLocks(t *testing.T) {
	s := NewSchema()

	fm, err := s.Resolve("age", "42")
	if err != nil {
		t.Fatalf("Resolve first: unexpected error: %v", err)
	}
	if fm.Type != FieldTypeInteger {
		t.Errorf("expected integer, got %s", fm.Type)
	}

	// second call with same type — should pass
	fm2, err := s.Resolve("age", "99")
	if err != nil {
		t.Fatalf("Resolve same type: unexpected error: %v", err)
	}
	if fm2.Type != FieldTypeInteger {
		t.Errorf("expected integer on second resolve, got %s", fm2.Type)
	}
}

func TestSchema_InferredConflictPromotesToText(t *testing.T) {
	s := NewSchema()

	fm, err := s.Resolve("status", "published") // keyword
	if err != nil || fm.Type != FieldTypeKeyword {
		t.Fatalf("first resolve: err=%v type=%s", err, fm.Type)
	}

	// "published" → keyword; "42" → integer; inferred conflict → promote to text, no error
	fm2, err := s.Resolve("status", "42")
	if err != nil {
		t.Fatalf("inferred conflict should promote, not error: %v", err)
	}
	if fm2.Type != FieldTypeText {
		t.Errorf("inferred conflict: expected promotion to text, got %s", fm2.Type)
	}
}

func TestSchema_ExplicitMappingIsAuthoritative(t *testing.T) {
	s := NewSchema()
	s.Set("category", FieldMapping{Type: FieldTypeKeyword, Index: true, Store: true})

	// "42" would normally infer integer, but explicit Set always wins — no error, returns keyword
	fm, err := s.Resolve("category", "42")
	if err != nil {
		t.Fatalf("explicit mapping should not conflict: %v", err)
	}
	if fm.Type != FieldTypeKeyword {
		t.Errorf("expected keyword (from explicit Set), got %s", fm.Type)
	}

	// same for a text-looking value
	fm2, err := s.Resolve("category", "a long string with spaces")
	if err != nil {
		t.Fatalf("explicit mapping should not conflict: %v", err)
	}
	if fm2.Type != FieldTypeKeyword {
		t.Errorf("expected keyword (from explicit Set), got %s", fm2.Type)
	}
}

func TestSchema_Fields(t *testing.T) {
	s := NewSchema()
	s.Resolve("title", "Go concurrency patterns") // text
	s.Resolve("age", "30")                        // integer

	fields := s.Fields()
	if fields["title"].Type != FieldTypeText {
		t.Errorf("title: want text, got %s", fields["title"].Type)
	}
	if fields["age"].Type != FieldTypeInteger {
		t.Errorf("age: want integer, got %s", fields["age"].Type)
	}
}

// --- Engine.Schema ---

func TestEngine_SchemaReflectsIndexedFields(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title":    {Value: "Go is fast"},
			"category": {Value: "programming"},
			"score":    {Value: "42"},
		},
	})

	schema := e.Schema()
	fields := schema.Fields()

	if fields["title"].Type != FieldTypeText {
		t.Errorf("title: want text, got %s", fields["title"].Type)
	}
	if fields["category"].Type != FieldTypeKeyword {
		t.Errorf("category: want keyword, got %s", fields["category"].Type)
	}
	if fields["score"].Type != FieldTypeInteger {
		t.Errorf("score: want integer, got %s", fields["score"].Type)
	}
}

// --- Mapping conflict at Index time ---

func TestEngine_InferredConflictPromotesAndIndexes(t *testing.T) {
	e := New()

	// "42" infers integer for "age"
	if err := e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"age": {Value: "42"}},
	}); err != nil {
		t.Fatalf("first index: %v", err)
	}

	// "old" infers text — conflict with integer, promotes to text, no error
	if err := e.Index(Document{
		ID:     "2",
		Fields: map[string]Field{"age": {Value: "old"}},
	}); err != nil {
		t.Fatalf("inferred conflict should not error: %v", err)
	}

	// both docs should be searchable after promotion to text
	q := query.NewBuilder().Must("age", "old").Build()
	results := e.Search(q, 10)
	if len(results) != 1 || results[0].ID != "2" {
		t.Errorf("after promotion to text, expected doc '2', got %v", ids(results))
	}
}

// --- Search with inferred types ---

func TestEngine_IntegerFieldExactMatch(t *testing.T) {
	e := New()
	e.Index(Document{ID: "1", Fields: map[string]Field{"score": {Value: "42"}}})
	e.Index(Document{ID: "2", Fields: map[string]Field{"score": {Value: "99"}}})

	q := query.NewBuilder().Must("score", "42").Build()
	results := e.Search(q, 10)
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("integer exact match: expected doc '1', got %v", ids(results))
	}
}

func TestEngine_FloatFieldExactMatch(t *testing.T) {
	e := New()
	e.Index(Document{ID: "1", Fields: map[string]Field{"price": {Value: "3.14"}}})
	e.Index(Document{ID: "2", Fields: map[string]Field{"price": {Value: "9.99"}}})

	q := query.NewBuilder().Must("price", "3.14").Build()
	results := e.Search(q, 10)
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("float exact match: expected doc '1', got %v", ids(results))
	}
}

func TestEngine_BooleanFieldExactMatch(t *testing.T) {
	e := New()
	e.Index(Document{ID: "1", Fields: map[string]Field{"active": {Value: "true"}}})
	e.Index(Document{ID: "2", Fields: map[string]Field{"active": {Value: "false"}}})

	q := query.NewBuilder().Must("active", "true").Build()
	results := e.Search(q, 10)
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("boolean exact match: expected doc '1', got %v", ids(results))
	}
}

func TestEngine_KeywordInferredExactMatch(t *testing.T) {
	e := New()
	e.Index(Document{ID: "1", Fields: map[string]Field{"status": {Value: "published"}}})
	e.Index(Document{ID: "2", Fields: map[string]Field{"status": {Value: "draft"}}})

	q := query.NewBuilder().Must("status", "published").Build()
	results := e.Search(q, 10)
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("inferred keyword: expected doc '1', got %v", ids(results))
	}
}
