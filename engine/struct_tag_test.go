package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

type Article struct {
	ID    string `search:"id"`
	Title string `search:"field:title,boost:2.0"`
	Body  string `search:"field:body"`
}

type ArticleWithSkip struct {
	ID       string `search:"id"`
	Body     string `search:"field:body"`
	Internal string `search:"-"`
	NoTag    string
}

type ArticleNoID struct {
	Title string `search:"field:title"`
}

type ArticleNonStringField struct {
	ID    string `search:"id"`
	Count int    `search:"field:count"`
}

func TestIndexStruct_Basic(t *testing.T) {
	e := New()
	err := e.IndexStruct(Article{
		ID:    "1",
		Title: "Go concurrency",
		Body:  "goroutines and channels",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	q := query.NewBuilder().Must("body", "goroutines").Build()
	results := e.Search(q, 10)
	if len(results) == 0 || results[0].ID != "1" {
		t.Error("indexed struct should be searchable by body field")
	}
}

func TestIndexStruct_BoostApplied(t *testing.T) {
	// title has boost:2.0, body has boost:1.0 (default)
	// a doc matching title should score higher than one matching body only
	e := New()
	e.IndexStruct(Article{ID: "1", Title: "golang", Body: "unrelated"})
	e.IndexStruct(Article{ID: "2", Title: "unrelated", Body: "golang"})

	q := query.NewBuilder().Should("title", "golang").Should("body", "golang").Build()
	results := e.Search(q, 10)

	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("doc with boost:2.0 title match should rank first, got %s", results[0].ID)
	}
}

func TestIndexStruct_BoostDefaultsToOne(t *testing.T) {
	// Body has no explicit boost in the tag — should default to 1.0
	e := New()
	err := e.IndexStruct(Article{ID: "1", Title: "hello", Body: "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, ok := getDoc(e, "1")
	if !ok {
		t.Fatal("document not found in storage")
	}
	if doc.Fields["body"].Boost != 1.0 {
		t.Errorf("expected boost 1.0 for unspecified boost, got %f", doc.Fields["body"].Boost)
	}
}

func TestIndexStruct_SkipDashTag(t *testing.T) {
	e := New()
	err := e.IndexStruct(ArticleWithSkip{
		ID:       "1",
		Body:     "go is fast",
		Internal: "secret",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, _ := getDoc(e, "1")
	if _, ok := doc.Fields["internal"]; ok {
		t.Error("field tagged search:\"-\" should not be indexed")
	}
}

func TestIndexStruct_SkipNoTag(t *testing.T) {
	e := New()
	err := e.IndexStruct(ArticleWithSkip{ID: "1", Body: "go", NoTag: "skip me"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, _ := getDoc(e, "1")
	if _, ok := doc.Fields["notag"]; ok {
		t.Error("field with no search tag should not be indexed")
	}
}

func TestIndexStruct_MissingIDReturnsError(t *testing.T) {
	e := New()
	err := e.IndexStruct(ArticleNoID{Title: "no id here"})
	if err == nil {
		t.Error("expected error when no field is tagged search:\"id\"")
	}
}

func TestIndexStruct_EmptyIDReturnsError(t *testing.T) {
	e := New()
	err := e.IndexStruct(Article{ID: "", Title: "hello", Body: "world"})
	if err == nil {
		t.Error("expected error when ID field is empty string")
	}
}

func TestIndexStruct_NonStringFieldReturnsError(t *testing.T) {
	e := New()
	err := e.IndexStruct(ArticleNonStringField{ID: "1", Count: 42})
	if err == nil {
		t.Error("expected error when a non-string field is tagged as field:")
	}
}

func TestIndexStruct_PointerInput(t *testing.T) {
	e := New()
	article := &Article{ID: "1", Title: "pointer test", Body: "via pointer"}
	err := e.IndexStruct(article)
	if err != nil {
		t.Fatalf("IndexStruct should accept a pointer to a struct, got error: %v", err)
	}

	q := query.NewBuilder().Must("body", "pointer").Build()
	results := e.Search(q, 10)
	if len(results) == 0 || results[0].ID != "1" {
		t.Error("struct passed as pointer should be searchable")
	}
}

func TestIndexStruct_NonStructReturnsError(t *testing.T) {
	e := New()
	err := e.IndexStruct("not a struct")
	if err == nil {
		t.Error("expected error when input is not a struct")
	}
}
