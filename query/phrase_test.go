package query

import (
	"testing"

	"github.com/noahfan/go-search/index"
)

// postingAt builds a Posting with specific positions — simulates what the index stores.
func postingAt(positions ...int) index.Posting {
	return index.Posting{DocID: "doc1", Frequency: len(positions), Positions: positions}
}

func TestPhraseMatch_Basic(t *testing.T) {
	// Doc: "I love New York city"
	// Positions: I=0 love=1 new=2 york=3 city=4
	docPostings := map[string]index.Posting{
		"i":    postingAt(0),
		"love": postingAt(1),
		"new":  postingAt(2),
		"york": postingAt(3),
		"city": postingAt(4),
	}

	if !PhraseMatch([]string{"new", "york"}, docPostings) {
		t.Error("'new york' should match")
	}
	if !PhraseMatch([]string{"york", "city"}, docPostings) {
		t.Error("'york city' should match")
	}
	if !PhraseMatch([]string{"i", "love", "new"}, docPostings) {
		t.Error("'i love new' should match (3-word phrase)")
	}
}

func TestPhraseMatch_WrongOrder(t *testing.T) {
	docPostings := map[string]index.Posting{
		"new":  postingAt(2),
		"york": postingAt(3),
	}

	if PhraseMatch([]string{"york", "new"}, docPostings) {
		t.Error("'york new' should NOT match — wrong order")
	}
}

func TestPhraseMatch_NotConsecutive(t *testing.T) {
	// "new" at 2, "city" at 4 — not consecutive (gap of 2)
	docPostings := map[string]index.Posting{
		"new":  postingAt(2),
		"york": postingAt(3),
		"city": postingAt(4),
	}

	if PhraseMatch([]string{"new", "city"}, docPostings) {
		t.Error("'new city' should NOT match — not consecutive")
	}
}

func TestPhraseMatch_RepeatedTerm(t *testing.T) {
	// Doc: "go go go" → positions [0, 1, 2]
	// "go go" should match via positions 0,1 or 1,2
	docPostings := map[string]index.Posting{
		"go": postingAt(0, 1, 2),
	}

	if !PhraseMatch([]string{"go", "go"}, docPostings) {
		t.Error("'go go' should match in 'go go go'")
	}
}

func TestPhraseMatch_PhraseAtDifferentStart(t *testing.T) {
	// Bug test: first starting position fails, second succeeds.
	// Doc: "go fast go rust" → go=[0,2] fast=[1] rust=[3]
	// Phrase "go rust": position 0 fails (rust not at 1), position 2 succeeds (rust at 3).
	// Current code returns false too early on first position failure.
	docPostings := map[string]index.Posting{
		"go":   postingAt(0, 2),
		"fast": postingAt(1),
		"rust": postingAt(3),
	}

	if !PhraseMatch([]string{"go", "rust"}, docPostings) {
		t.Error("'go rust' should match — phrase appears at positions 2,3 even though position 0 fails")
	}
}

func TestPhraseMatch_MissingTerm(t *testing.T) {
	docPostings := map[string]index.Posting{
		"new": postingAt(0),
		// "york" not present
	}

	if PhraseMatch([]string{"new", "york"}, docPostings) {
		t.Error("should not match when a term is missing from the document")
	}
}

func TestPhraseMatch_Empty(t *testing.T) {
	docPostings := map[string]index.Posting{
		"go": postingAt(0),
	}

	if PhraseMatch([]string{}, docPostings) {
		t.Error("empty phrase should not match")
	}
}

func TestMatch_PhraseClause(t *testing.T) {
	// Bug test: Match currently ignores Phrase clause type entirely.
	// A Phrase clause must be treated as MUST — if it doesn't match, exclude the doc.
	docPostings := map[string]index.Posting{
		"new":  postingAt(0),
		"york": postingAt(5), // not consecutive with "new" at 0
	}

	q := NewBuilder().Phrase("body", "new", "york").Build()

	// words exist but not as a phrase — should NOT match
	if Match(q, docPostings) {
		t.Error("Match should return false when Phrase clause does not match")
	}
}

func TestMatch_PhraseClause_Matches(t *testing.T) {
	docPostings := map[string]index.Posting{
		"new":  postingAt(2),
		"york": postingAt(3),
	}

	q := NewBuilder().Phrase("body", "new", "york").Build()

	if !Match(q, docPostings) {
		t.Error("Match should return true when Phrase clause matches")
	}
}
