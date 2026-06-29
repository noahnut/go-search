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
		wordsMap[word] = struct{}{}
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
