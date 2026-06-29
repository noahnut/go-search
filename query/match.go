package query

import (
	"strings"

	"github.com/noahfan/go-search/index"
)

// Match checks whether a document's postings satisfy the boolean query.
// docPostings: map of term → Posting for a single document.
// Returns true if the document should be included in results.
func Match(q Query, docPostings map[string]index.Posting) bool {

	hasShould := false
	anyShoudMatched := false
	hasMust := false

	for _, clause := range q.Clauses {
		key := clause.Field + ":" + clause.Term
		switch clause.Type {
		case Must:
			if _, ok := docPostings[key]; !ok {
				return false
			}
			hasMust = true
		case Should:
			hasShould = true
			if _, ok := docPostings[key]; ok {
				anyShoudMatched = true
			}
		case MustNot:
			if _, ok := docPostings[key]; ok {
				return false
			}
		case Phrase:
			if !PhraseMatch(strings.Fields(clause.Term), docPostings) {
				return false
			}
		}
	}

	if !hasMust && hasShould && !anyShoudMatched {
		return false
	}
	return true
}
