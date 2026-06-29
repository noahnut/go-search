// analysis/tokenizer.go
package analysis

import (
	"strings"
	"unicode"
)

// Token represents a single extracted term with its position in the original text.
type Token struct {
	Term     string
	Position int // 0-based index of this token in the token stream
}

// Tokenizer splits raw text into a stream of tokens.
type Tokenizer interface {
	Tokenize(text string) []Token
}

// WhitespaceTokenizer splits on whitespace only.
type WhitespaceTokenizer struct{}

func (t *WhitespaceTokenizer) Tokenize(text string) []Token {
	if text == "" {
		return []Token{}
	}

	words := strings.Fields(text)
	tokens := make([]Token, 0, len(words))
	for i, word := range words {
		if word == "" {
			continue
		}
		tokens = append(tokens, Token{Term: word, Position: i})
	}
	return tokens
}

// StandardTokenizer splits on whitespace AND punctuation, and lowercases.
type StandardTokenizer struct{}

func (t *StandardTokenizer) Tokenize(text string) []Token {
	if text == "" {
		return []Token{}
	}

	token := make([]Token, 0)

	buffer := ""
	position := 0
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if buffer != "" {
				token = append(token, Token{Term: strings.ToLower(buffer), Position: position})
				position++
				buffer = ""
			}
		} else {
			if unicode.IsLetter(r) {
				buffer += string(r)
			}
		}
	}

	if buffer != "" {
		token = append(token, Token{Term: strings.ToLower(buffer), Position: position})
	}
	return token
}
