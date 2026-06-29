package analysis

import (
	"reflect"
	"testing"
)

func TestLowercaseFilter(t *testing.T) {
	f := &LowercaseFilter{}

	tests := []struct {
		name  string
		input []Token
		want  []Token
	}{
		{
			"lowercases terms",
			[]Token{{Term: "Hello", Position: 0}, {Term: "World", Position: 1}},
			[]Token{{Term: "hello", Position: 0}, {Term: "world", Position: 1}},
		},
		{
			"preserves positions",
			[]Token{{Term: "FOO", Position: 3}, {Term: "BAR", Position: 7}},
			[]Token{{Term: "foo", Position: 3}, {Term: "bar", Position: 7}},
		},
		{
			"empty input",
			[]Token{},
			[]Token{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Filter(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\ngot:  %v\nwant: %v", got, tt.want)
			}
		})
	}
}

func TestStopWordFilter(t *testing.T) {
	f := NewStopWordFilter([]string{"the", "is", "a"})

	tests := []struct {
		name  string
		input []Token
		want  []Token
	}{
		{
			"removes stop words",
			[]Token{{Term: "the", Position: 0}, {Term: "fox", Position: 1}, {Term: "is", Position: 2}, {Term: "quick", Position: 3}},
			[]Token{{Term: "fox", Position: 1}, {Term: "quick", Position: 3}},
		},
		{
			// Positions of kept tokens must not change
			"preserves positions of kept tokens",
			[]Token{{Term: "a", Position: 0}, {Term: "fast", Position: 1}, {Term: "car", Position: 2}},
			[]Token{{Term: "fast", Position: 1}, {Term: "car", Position: 2}},
		},
		{
			"no stop words in input",
			[]Token{{Term: "go", Position: 0}, {Term: "runs", Position: 1}},
			[]Token{{Term: "go", Position: 0}, {Term: "runs", Position: 1}},
		},
		{
			"empty input",
			[]Token{},
			[]Token{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Filter(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\ngot:  %v\nwant: %v", got, tt.want)
			}
		})
	}
}

func TestAnalyzer(t *testing.T) {
	stopWords := []string{"a", "an", "the", "is", "are", "to", "of", "and", "in"}

	tests := []struct {
		name     string
		analyzer *Analyzer
		input    string
		want     []Token
	}{
		{
			"no filters",
			NewAnalyzer(&WhitespaceTokenizer{}),
			"Hello World",
			[]Token{{Term: "Hello", Position: 0}, {Term: "World", Position: 1}},
		},
		{
			"lowercase filter",
			NewAnalyzer(&WhitespaceTokenizer{}, &LowercaseFilter{}),
			"Hello World",
			[]Token{{Term: "hello", Position: 0}, {Term: "world", Position: 1}},
		},
		{
			// After removing "the" (position 0), remaining tokens keep their original positions.
			// "Quick" stays at position 1, "Fox" at position 2 — they are NOT renumbered to 0, 1.
			"stop word positions are preserved — not renumbered",
			NewAnalyzer(&WhitespaceTokenizer{}, &LowercaseFilter{}, NewStopWordFilter(stopWords)),
			"The Quick Fox",
			[]Token{{Term: "quick", Position: 1}, {Term: "fox", Position: 2}},
		},
		{
			"empty string",
			NewAnalyzer(&StandardTokenizer{}, &LowercaseFilter{}),
			"",
			[]Token{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.analyzer.Analyze(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\ngot:  %v\nwant: %v", got, tt.want)
			}
		})
	}
}
