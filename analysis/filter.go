// analysis/filter.go
package analysis

import "strings"

// TokenFilter transforms a slice of tokens — lowercasing, removing stop words, etc.
type TokenFilter interface {
	Filter(tokens []Token) []Token
}

// LowercaseFilter lowercases every token's Term.
type LowercaseFilter struct{}

func (f *LowercaseFilter) Filter(tokens []Token) []Token {
	filteredTokens := make([]Token, 0, len(tokens))
	for _, token := range tokens {
		filteredTokens = append(filteredTokens, Token{Term: strings.ToLower(token.Term), Position: token.Position})
	}
	return filteredTokens
}

// StopWordFilter removes tokens whose Term is in a configured stop word set.
type StopWordFilter struct {
	Words map[string]struct{} // use a map for O(1) lookup
}

func NewStopWordFilter(words []string) *StopWordFilter {
	wordsMap := make(map[string]struct{})
	for _, word := range words {
		wordsMap[strings.ToLower(word)] = struct{}{}
	}
	return &StopWordFilter{Words: wordsMap}
}

func (f *StopWordFilter) Filter(tokens []Token) []Token {
	if len(tokens) == 0 {
		return []Token{}
	}

	filteredTokens := make([]Token, 0, len(tokens))
	for _, token := range tokens {
		if _, ok := f.Words[token.Term]; !ok {
			filteredTokens = append(filteredTokens, token)
		}
	}
	return filteredTokens
}

func NewEnglishStopWordFilter() *StopWordFilter {
	stopWords := []string{
		"a", "an", "the", "and", "or", "but", "if", "in", "on", "at", "to", "for", "of", "with",
		"is", "are", "was", "were", "be", "been", "being", "have", "has", "had", "do", "does",
		"did", "will", "would", "could", "should", "may", "might", "shall", "can",
		"i", "me", "my", "we", "our", "you", "your", "he", "she", "it", "they", "their",
		"this", "that", "these", "those",
		"not", "no", "nor",
	}

	return NewStopWordFilter(stopWords)
}

// Add inserts additional words into an existing filter (returns the same filter for chaining).
func (f *StopWordFilter) Add(words ...string) *StopWordFilter {
	for _, word := range words {
		f.Words[strings.ToLower(word)] = struct{}{}
	}
	return f
}
