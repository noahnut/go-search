// analysis/analyzer.go
package analysis

// Analyzer runs a Tokenizer then applies zero or more TokenFilters in order.
type Analyzer struct {
	Tokenizer Tokenizer
	Filters   []TokenFilter
}

func NewAnalyzer(tokenizer Tokenizer, filters ...TokenFilter) *Analyzer {
	return &Analyzer{Tokenizer: tokenizer, Filters: filters}
}

func (a *Analyzer) Analyze(text string) []Token {
	tokens := a.Tokenizer.Tokenize(text)
	for _, filter := range a.Filters {
		tokens = filter.Filter(tokens)
	}

	return tokens
}
