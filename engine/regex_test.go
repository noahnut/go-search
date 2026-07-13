package engine

import (
	"testing"

	"github.com/noahfan/go-search/query"
)

// --- passRegexFilters unit tests ---

func makeRegexDoc(fields map[string]string) Document {
	f := make(map[string]Field, len(fields))
	for k, v := range fields {
		f[k] = Field{Value: v}
	}
	return Document{ID: "test", Fields: f}
}

func TestPassRegexFilters_Match(t *testing.T) {
	doc := makeRegexDoc(map[string]string{"status": "error_timeout"})
	regexes := []query.RegexClause{{Field: "status", Regex: "error_.*"}}
	if !passRegexFilters(doc, regexes) {
		t.Error("'error_timeout' should match regex 'error_.*'")
	}
}

func TestPassRegexFilters_NoMatch(t *testing.T) {
	doc := makeRegexDoc(map[string]string{"status": "ok"})
	regexes := []query.RegexClause{{Field: "status", Regex: "error_.*"}}
	if passRegexFilters(doc, regexes) {
		t.Error("'ok' should not match regex 'error_.*'")
	}
}

func TestPassRegexFilters_MissingField_Excluded(t *testing.T) {
	doc := makeRegexDoc(map[string]string{"body": "hello"})
	regexes := []query.RegexClause{{Field: "status", Regex: ".*"}}
	if passRegexFilters(doc, regexes) {
		t.Error("missing field should cause exclusion")
	}
}

func TestPassRegexFilters_InvalidRegex_Excluded(t *testing.T) {
	doc := makeRegexDoc(map[string]string{"body": "hello"})
	regexes := []query.RegexClause{{Field: "body", Regex: "["}} // invalid
	if passRegexFilters(doc, regexes) {
		t.Error("invalid regex should cause exclusion, not panic")
	}
}

func TestPassRegexFilters_MultipleRegexes_AllMustMatch(t *testing.T) {
	doc := makeRegexDoc(map[string]string{"code": "E404", "env": "production"})
	regexes := []query.RegexClause{
		{Field: "code", Regex: "E[0-9]+"},
		{Field: "env", Regex: "prod.*"},
	}
	if !passRegexFilters(doc, regexes) {
		t.Error("doc matching both regexes should pass")
	}

	// second regex fails
	regexes[1] = query.RegexClause{Field: "env", Regex: "staging"}
	if passRegexFilters(doc, regexes) {
		t.Error("doc failing one regex should be excluded")
	}
}

func TestPassRegexFilters_EmptyRegexes_AlwaysPass(t *testing.T) {
	doc := makeRegexDoc(map[string]string{"body": "anything"})
	if !passRegexFilters(doc, nil) {
		t.Error("no regex filters should always pass")
	}
}

func TestPassRegexFilters_AnchoredPattern(t *testing.T) {
	doc := makeRegexDoc(map[string]string{"code": "E404"})

	// anchored — exact match
	if !passRegexFilters(doc, []query.RegexClause{{Field: "code", Regex: "^E404$"}}) {
		t.Error("^E404$ should match 'E404'")
	}
	if passRegexFilters(doc, []query.RegexClause{{Field: "code", Regex: "^E40$"}}) {
		t.Error("^E40$ should not match 'E404'")
	}
}

// --- Builder.Regex integration tests ---

func regexDoc(id, status, body string) Document {
	return Document{
		ID: id,
		Fields: map[string]Field{
			"body":   {Value: body, Boost: 1.0},
			"status": {Value: status},
		},
	}
}

func TestBuilderRegex_BasicFilter(t *testing.T) {
	e := New()
	e.Index(regexDoc("1", "error_timeout", "request failed"))
	e.Index(regexDoc("2", "ok", "request succeeded"))

	q := query.NewBuilder().
		Must("body", "request").
		Regex("status", "error_.*").
		Build()

	res := e.Search(q, 10).Hits
	if len(res) != 1 || res[0].ID != "1" {
		t.Errorf("expected only doc '1' (error status), got %v", ids(res))
	}
}

func TestBuilderRegex_NoMatch_EmptyResult(t *testing.T) {
	e := New()
	e.Index(regexDoc("1", "ok", "request"))

	q := query.NewBuilder().
		Must("body", "request").
		Regex("status", "error_.*").
		Build()

	res := e.Search(q, 10).Hits
	if len(res) != 0 {
		t.Errorf("expected no results, got %v", ids(res))
	}
}

func TestBuilderRegex_MultipleRegexes(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body":   {Value: "item", Boost: 1.0},
			"code":   {Value: "E404"},
			"env":    {Value: "production"},
		},
	})
	e.Index(Document{
		ID: "2",
		Fields: map[string]Field{
			"body":   {Value: "item", Boost: 1.0},
			"code":   {Value: "E404"},
			"env":    {Value: "staging"},
		},
	})

	q := query.NewBuilder().
		Must("body", "item").
		Regex("code", "E[0-9]+").
		Regex("env", "prod.*").
		Build()

	res := e.Search(q, 10).Hits
	if len(res) != 1 || res[0].ID != "1" {
		t.Errorf("expected only doc '1' (production env), got %v", ids(res))
	}
}

func TestBuilderRegex_MissingFieldExcluded(t *testing.T) {
	e := New()
	e.Index(Document{
		ID:     "1",
		Fields: map[string]Field{"body": {Value: "item", Boost: 1.0}}, // no status field
	})

	q := query.NewBuilder().
		Must("body", "item").
		Regex("status", ".*").
		Build()

	res := e.Search(q, 10).Hits
	if len(res) != 0 {
		t.Errorf("doc missing the regex field should be excluded, got %v", ids(res))
	}
}

func TestBuilderRegex_MatchesRawFieldValue(t *testing.T) {
	// Regex runs against the full raw value, not tokenized terms.
	// "golang is fast" is the raw value; the regex "go.*fast" should match.
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body":  {Value: "golang is fast", Boost: 1.0},
			"title": {Value: "golang is fast"},
		},
	})

	q := query.NewBuilder().
		Must("body", "golang").
		Regex("title", "go.*fast").
		Build()

	res := e.Search(q, 10).Hits
	if len(res) != 1 {
		t.Errorf("regex against raw field value should match, got %v", ids(res))
	}
}

func TestBuilderRegex_CombinedWithRange(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"body":   {Value: "item", Boost: 1.0},
			"price":  {Value: "50"},
			"status": {Value: "active"},
		},
	})
	e.Index(Document{
		ID: "2",
		Fields: map[string]Field{
			"body":   {Value: "item", Boost: 1.0},
			"price":  {Value: "200"},
			"status": {Value: "active"},
		},
	})

	q := query.NewBuilder().
		Must("body", "item").
		Range("price", query.Ptr(0), query.Ptr(100)).
		Regex("status", "act.*").
		Build()

	res := e.Search(q, 10).Hits
	if len(res) != 1 || res[0].ID != "1" {
		t.Errorf("expected doc '1' (price in range, status matches), got %v", ids(res))
	}
}

// --- Builder unit test ---

func TestBuilderRegex_BuildsRegexClause(t *testing.T) {
	q := query.NewBuilder().
		Regex("status", "error_.*").
		Regex("env", "prod.*").
		Build()

	if len(q.Regexes) != 2 {
		t.Fatalf("expected 2 regex clauses, got %d", len(q.Regexes))
	}
	if q.Regexes[0].Field != "status" || q.Regexes[0].Regex != "error_.*" {
		t.Errorf("first regex clause wrong: %+v", q.Regexes[0])
	}
	if q.Regexes[1].Field != "env" || q.Regexes[1].Regex != "prod.*" {
		t.Errorf("second regex clause wrong: %+v", q.Regexes[1])
	}
}
