package query

import "strings"

// Clause type determines how a term affects the result.
type ClauseType string

const (
	Must    ClauseType = "must"     // document MUST contain this term
	Should  ClauseType = "should"   // document SHOULD contain this term (boosts score)
	MustNot ClauseType = "must_not" // document MUST NOT contain this term
	Phrase  ClauseType = "phrase"   // document MUST contain the given terms in consecutive order
	Fuzzy   ClauseType = "fuzzy"    // document MUST contain a term within the given max distance
)

// Clause is one term in a boolean query.
type Clause struct {
	Field       string
	Term        string
	Type        ClauseType
	MaxDistance int
}

// Query is a boolean query with one or more clauses.
type Query struct {
	Clauses []Clause
}

// Builder constructs a Query fluently.
type Builder struct {
	clauses []Clause
}

func NewBuilder() *Builder {
	return &Builder{clauses: []Clause{}}
}

func (b *Builder) Must(field, term string) *Builder {
	b.clauses = append(b.clauses, Clause{Field: field, Term: term, Type: Must})
	return b
}
func (b *Builder) Should(field, term string) *Builder {
	b.clauses = append(b.clauses, Clause{Field: field, Term: term, Type: Should})
	return b
}
func (b *Builder) MustNot(field, term string) *Builder {
	b.clauses = append(b.clauses, Clause{Field: field, Term: term, Type: MustNot})
	return b
}

func (b *Builder) Phrase(field string, terms ...string) *Builder {
	b.clauses = append(b.clauses, Clause{Field: field, Term: strings.Join(terms, " "), Type: Phrase})
	return b
}

func (b *Builder) Build() Query {
	return Query{Clauses: b.clauses}
}
