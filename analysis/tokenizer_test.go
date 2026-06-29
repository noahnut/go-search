package analysis

import (
	"reflect"
	"testing"
)

func TestWhitespaceTokenizer(t *testing.T) {
	tok := &WhitespaceTokenizer{}

	tests := []struct {
		name  string
		input string
		want  []Token
	}{
		{
			"basic",
			"hello world",
			[]Token{{Term: "hello", Position: 0}, {Term: "world", Position: 1}},
		},
		{
			"keeps punctuation",
			"Hello, World!",
			[]Token{{Term: "Hello,", Position: 0}, {Term: "World!", Position: 1}},
		},
		{
			// strings.Split("", " ") returns [""] not [] — your code will fail this
			"empty string",
			"",
			[]Token{},
		},
		{
			// strings.Split produces empty-string tokens for consecutive spaces
			"double space",
			"hello  world",
			[]Token{{Term: "hello", Position: 0}, {Term: "world", Position: 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tok.Tokenize(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\ngot:  %v\nwant: %v", got, tt.want)
			}
		})
	}
}

func TestStandardTokenizer(t *testing.T) {
	tok := &StandardTokenizer{}

	tests := []struct {
		name  string
		input string
		want  []Token
	}{
		{
			// Your code does not lowercase — this will fail
			"lowercases",
			"Hello, World!",
			[]Token{{Term: "hello", Position: 0}, {Term: "world", Position: 1}},
		},
		{
			"strips punctuation",
			"Go is great.",
			[]Token{{Term: "go", Position: 0}, {Term: "is", Position: 1}, {Term: "great", Position: 2}},
		},
		{
			"hyphen splits words",
			"foo-bar",
			[]Token{{Term: "foo", Position: 0}, {Term: "bar", Position: 1}},
		},
		{
			// Position should be 0, 1, 2 — token index, not byte offset
			"positions are sequential",
			"one two three",
			[]Token{{Term: "one", Position: 0}, {Term: "two", Position: 1}, {Term: "three", Position: 2}},
		},
		{
			"double space",
			"hello  world",
			[]Token{{Term: "hello", Position: 0}, {Term: "world", Position: 1}},
		},
		{
			"empty string",
			"",
			[]Token{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tok.Tokenize(tt.input)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\ngot:  %v\nwant: %v", got, tt.want)
			}
		})
	}
}
