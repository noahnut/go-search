package engine

import (
	"testing"
)

// --- wildcardToRegexp unit tests ---

func TestWildcardToRegexp_StarMatchesAnything(t *testing.T) {
	re := wildcardToRegexp("go*")
	if !re.MatchString("golang") || !re.MatchString("go") || !re.MatchString("goroutine") {
		t.Error("go* should match 'golang', 'go', 'goroutine'")
	}
	if re.MatchString("python") {
		t.Error("go* should not match 'python'")
	}
}

func TestWildcardToRegexp_QuestionMatchesSingleChar(t *testing.T) {
	re := wildcardToRegexp("g?")
	if !re.MatchString("go") || !re.MatchString("go"[0:2]) {
		t.Error("g? should match a two-character word starting with 'g'")
	}
	if re.MatchString("golang") {
		t.Error("g? should not match 'golang' (too long)")
	}
	if re.MatchString("g") {
		t.Error("g? should not match 'g' (too short)")
	}
}

func TestWildcardToRegexp_StarInMiddle(t *testing.T) {
	re := wildcardToRegexp("g*g")
	if !re.MatchString("golang") {
		// golang does not end in 'g' — should not match
	}
	if !re.MatchString("gog") || !re.MatchString("gg") {
		t.Error("g*g should match 'gog' and 'gg'")
	}
	if re.MatchString("go") {
		t.Error("g*g should not match 'go'")
	}
}

func TestWildcardToRegexp_NoWildcard_ExactMatch(t *testing.T) {
	re := wildcardToRegexp("golang")
	if !re.MatchString("golang") {
		t.Error("no wildcard should match exactly 'golang'")
	}
	if re.MatchString("golangx") || re.MatchString("xgolang") {
		t.Error("no wildcard should not match substrings or superstrings")
	}
}

func TestWildcardToRegexp_SpecialRegexCharsEscaped(t *testing.T) {
	re := wildcardToRegexp("go.lang")
	if re.MatchString("golang") {
		t.Error("literal '.' should not match any character — it must be escaped")
	}
	if !re.MatchString("go.lang") {
		t.Error("go.lang should match the literal string 'go.lang'")
	}
}

// --- WildcardSearch integration tests ---

func TestWildcardSearch_StarSuffix(t *testing.T) {
	e := New()
	e.Index(doc("1", "golang is fast"))
	e.Index(doc("2", "python is popular"))
	e.Index(doc("3", "golangci linter"))

	results := e.WildcardSearch("body", "go*", 10)

	ids := resultIDs(results)
	if !ids["1"] {
		t.Error("doc '1' has 'golang' — should match 'go*'")
	}
	if !ids["3"] {
		t.Error("doc '3' has 'golangci' — should match 'go*'")
	}
	if ids["2"] {
		t.Error("doc '2' has no 'go*' token — should not match")
	}
}

func TestWildcardSearch_StarPrefix(t *testing.T) {
	e := New()
	e.Index(doc("1", "goroutine"))
	e.Index(doc("2", "subroutine"))
	e.Index(doc("3", "golang"))

	results := e.WildcardSearch("body", "*routine", 10)

	ids := resultIDs(results)
	if !ids["1"] {
		t.Error("'goroutine' should match '*routine'")
	}
	if !ids["2"] {
		t.Error("'subroutine' should match '*routine'")
	}
	if ids["3"] {
		t.Error("'golang' should not match '*routine'")
	}
}

func TestWildcardSearch_StarMiddle(t *testing.T) {
	e := New()
	e.Index(doc("1", "golang"))
	e.Index(doc("2", "goroutine"))
	e.Index(doc("3", "python"))

	results := e.WildcardSearch("body", "g*g", 10)

	ids := resultIDs(results)
	if !ids["1"] {
		t.Error("'golang' ends in 'g' — should match 'g*g'")
	}
	if ids["2"] {
		t.Error("'goroutine' does not end in 'g' — should not match 'g*g'")
	}
}

func TestWildcardSearch_QuestionMark(t *testing.T) {
	e := New()
	e.Index(doc("1", "go"))
	e.Index(doc("2", "golang"))
	e.Index(doc("3", "py"))

	results := e.WildcardSearch("body", "g?", 10)

	ids := resultIDs(results)
	if !ids["1"] {
		t.Error("'go' is 2 chars starting with 'g' — should match 'g?'")
	}
	if ids["2"] {
		t.Error("'golang' is too long — should not match 'g?'")
	}
	if ids["3"] {
		t.Error("'py' does not start with 'g' — should not match 'g?'")
	}
}

func TestWildcardSearch_NoMatch(t *testing.T) {
	e := New()
	e.Index(doc("1", "golang"))

	results := e.WildcardSearch("body", "rust*", 10)
	if len(results) != 0 {
		t.Errorf("expected no results for 'rust*', got %d", len(results))
	}
}

func TestWildcardSearch_FieldScoped(t *testing.T) {
	e := New()
	e.Index(Document{
		ID: "1",
		Fields: map[string]Field{
			"title": {Value: "golang tutorial", Boost: 1.0},
			"body":  {Value: "learn programming", Boost: 1.0},
		},
	})

	bodyResults := e.WildcardSearch("body", "go*", 10)
	if len(bodyResults) != 0 {
		t.Error("'go*' in body field should not match a token only in title")
	}

	titleResults := e.WildcardSearch("title", "go*", 10)
	if len(titleResults) == 0 {
		t.Error("'go*' in title field should match doc '1'")
	}
}

func TestWildcardSearch_NoDuplicateDocs(t *testing.T) {
	// doc has two tokens matching the pattern — should appear once
	e := New()
	e.Index(doc("1", "golang gold"))

	results := e.WildcardSearch("body", "go*", 10)
	count := 0
	for _, r := range results {
		if r.ID == "1" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("doc '1' appeared %d times — expected exactly once", count)
	}
}

func TestWildcardSearch_TopKLimits(t *testing.T) {
	e := New()
	for i := range 5 {
		e.Index(doc(string(rune('1'+i)), "golang"))
	}

	results := e.WildcardSearch("body", "go*", 3)
	if len(results) != 3 {
		t.Errorf("topK=3 should return at most 3 results, got %d", len(results))
	}
}

func TestWildcardSearch_DeletedDocExcluded(t *testing.T) {
	e := New()
	e.Index(doc("1", "golang"))
	e.Index(doc("2", "goroutine"))
	e.Delete("1")

	results := e.WildcardSearch("body", "go*", 10)
	ids := resultIDs(results)
	if ids["1"] {
		t.Error("deleted doc '1' should not appear in wildcard results")
	}
	if !ids["2"] {
		t.Error("doc '2' was not deleted and should appear")
	}
}

// resultIDs converts []Result to a set of IDs for easy lookup in tests.
func resultIDs(results []Result) map[string]bool {
	m := make(map[string]bool, len(results))
	for _, r := range results {
		m[r.ID] = true
	}
	return m
}
