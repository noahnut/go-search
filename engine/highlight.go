package engine

import (
	"strings"

	"github.com/noahfan/go-search/analysis"
)

// Highlight holds a matched snippet for one field.
type Highlight struct {
	Field   string
	Snippet string // the full field value with matched terms wrapped in markers
}

// HighlightDoc returns snippets for each text field of doc that contains at least
// one of the query terms. markerOpen and markerClose wrap each matched term.
func HighlightDoc(
	doc Document,
	terms []string, // bare terms to highlight (no "field:" prefix)
	analyzer *analysis.Analyzer,
	markerOpen, markerClose string,
) []Highlight {
	termSet := make(map[string]struct{})
	for _, t := range terms {
		termSet[t] = struct{}{}
	}

	highlights := make([]Highlight, 0, len(doc.Fields))
	for fieldName, field := range doc.Fields {
		words := strings.Fields(field.Value)

		matched := false
		for i, word := range words {
			tokens := analyzer.Analyze(word)
			if len(tokens) > 0 {
				if _, hit := termSet[tokens[0].Term]; hit {
					words[i] = markerOpen + word + markerClose
					matched = true
				}
			}
		}
		if matched {
			highlights = append(highlights, Highlight{
				Field:   fieldName,
				Snippet: strings.Join(words, " "),
			})
		}
	}

	return highlights
}
