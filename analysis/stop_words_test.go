package analysis

import (
	"testing"
)

// --- NewEnglishStopWordFilter ---

func TestEnglishStopWordFilter_RemovesCommonWords(t *testing.T) {
	f := NewEnglishStopWordFilter()

	cases := []string{"the", "a", "an", "is", "are", "and", "or", "in", "for", "of"}
	tokens := make([]Token, len(cases))
	for i, w := range cases {
		tokens[i] = Token{Term: w, Position: i}
	}

	got := f.Filter(tokens)
	if len(got) != 0 {
		remaining := make([]string, len(got))
		for i, t := range got {
			remaining[i] = t.Term
		}
		t.Errorf("expected all common stop words removed, still present: %v", remaining)
	}
}

func TestEnglishStopWordFilter_KeepsNonStopWords(t *testing.T) {
	f := NewEnglishStopWordFilter()

	nonStop := []string{"go", "rust", "search", "programming", "index"}
	tokens := make([]Token, len(nonStop))
	for i, w := range nonStop {
		tokens[i] = Token{Term: w, Position: i}
	}

	got := f.Filter(tokens)
	if len(got) != len(nonStop) {
		t.Errorf("expected all %d non-stop words kept, got %d", len(nonStop), len(got))
	}
}

func TestEnglishStopWordFilter_PositionsPreserved(t *testing.T) {
	f := NewEnglishStopWordFilter()

	// "the" at 0 is a stop word; "go" at 1 and "is" at 2 are not / are stop words
	tokens := []Token{
		{Term: "the", Position: 0},
		{Term: "go", Position: 1},
		{Term: "is", Position: 2},
		{Term: "fast", Position: 3},
	}

	got := f.Filter(tokens)

	// "go" and "fast" should survive; positions must be original
	for _, tok := range got {
		switch tok.Term {
		case "go":
			if tok.Position != 1 {
				t.Errorf("'go' position: want 1, got %d", tok.Position)
			}
		case "fast":
			if tok.Position != 3 {
				t.Errorf("'fast' position: want 3, got %d", tok.Position)
			}
		}
	}
}

func TestEnglishStopWordFilter_EmptyInput(t *testing.T) {
	f := NewEnglishStopWordFilter()
	got := f.Filter([]Token{})
	if len(got) != 0 {
		t.Errorf("expected empty output for empty input, got %d tokens", len(got))
	}
}

func TestEnglishStopWordFilter_AllTokensRemoved(t *testing.T) {
	f := NewEnglishStopWordFilter()

	tokens := []Token{
		{Term: "the", Position: 0},
		{Term: "a", Position: 1},
		{Term: "and", Position: 2},
	}

	got := f.Filter(tokens)
	if len(got) != 0 {
		t.Errorf("expected empty result when all tokens are stop words, got %v", got)
	}
}

// --- Add ---

func TestStopWordFilter_Add_SingleWord(t *testing.T) {
	f := NewEnglishStopWordFilter().Add("golang")

	tokens := []Token{
		{Term: "golang", Position: 0},
		{Term: "fast", Position: 1},
	}
	got := f.Filter(tokens)

	if len(got) != 1 || got[0].Term != "fast" {
		t.Errorf("expected only 'fast' after adding 'golang' as stop word, got %v", got)
	}
}

func TestStopWordFilter_Add_MultipleWords(t *testing.T) {
	f := NewEnglishStopWordFilter().Add("golang", "python", "rust")

	tokens := []Token{
		{Term: "golang", Position: 0},
		{Term: "python", Position: 1},
		{Term: "rust", Position: 2},
		{Term: "search", Position: 3},
	}
	got := f.Filter(tokens)

	if len(got) != 1 || got[0].Term != "search" {
		t.Errorf("expected only 'search' to survive, got %v", got)
	}
}

func TestStopWordFilter_Add_ReturnsFilter(t *testing.T) {
	f := NewEnglishStopWordFilter()
	returned := f.Add("golang")
	if returned != f {
		t.Error("Add should return the same filter for chaining")
	}
}

func TestStopWordFilter_Add_Chaining(t *testing.T) {
	f := NewStopWordFilter([]string{"the"}).Add("a").Add("an")

	tokens := []Token{
		{Term: "the", Position: 0},
		{Term: "a", Position: 1},
		{Term: "an", Position: 2},
		{Term: "fox", Position: 3},
	}
	got := f.Filter(tokens)

	if len(got) != 1 || got[0].Term != "fox" {
		t.Errorf("expected only 'fox' after chained Add, got %v", got)
	}
}

// --- NewStopWordFilter case insensitivity ---

func TestNewStopWordFilter_CaseInsensitiveConstruction(t *testing.T) {
	// Stop word list authored with mixed case — should still match lowercased tokens.
	f := NewStopWordFilter([]string{"The", "IS", "A"})

	tokens := []Token{
		{Term: "the", Position: 0},
		{Term: "is", Position: 1},
		{Term: "a", Position: 2},
		{Term: "fox", Position: 3},
	}
	got := f.Filter(tokens)

	if len(got) != 1 || got[0].Term != "fox" {
		t.Errorf("expected only 'fox' when stop words constructed with mixed case, got %v", got)
	}
}

func TestStopWordFilter_Add_CaseInsensitive(t *testing.T) {
	f := NewStopWordFilter([]string{"the"}).Add("GOLANG")

	tokens := []Token{
		{Term: "golang", Position: 0},
		{Term: "fast", Position: 1},
	}
	got := f.Filter(tokens)

	if len(got) != 1 || got[0].Term != "fast" {
		t.Errorf("Add with uppercase word should still remove lowercased token, got %v", got)
	}
}

// --- Integration: NewEnglishStopWordFilter with analyzer ---

func TestEnglishStopWordFilter_WithAnalyzer(t *testing.T) {
	a := NewAnalyzer(
		&StandardTokenizer{},
		NewEnglishStopWordFilter(),
	)

	tokens := a.Analyze("Go is a fast and simple language")

	for _, tok := range tokens {
		switch tok.Term {
		case "is", "a", "and":
			t.Errorf("stop word %q should have been removed", tok.Term)
		}
	}

	found := false
	for _, tok := range tokens {
		if tok.Term == "go" || tok.Term == "fast" || tok.Term == "simple" || tok.Term == "language" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected non-stop content words to remain after filtering")
	}
}
