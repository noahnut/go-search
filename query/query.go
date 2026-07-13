package query

import "strings"

// Clause type determines how a term affects the result.
type ClauseType string

const (
	Must    ClauseType = "must"     // document MUST contain this term
	Should  ClauseType = "should"   // document SHOULD contain this term (boosts score)
	MustNot ClauseType = "must_not" // document MUST NOT contain this term
	Phrase  ClauseType = "phrase"   // document MUST contain the given terms in consecutive order
	Range   ClauseType = "range"    // document MUST contain a term within the given range
	Fuzzy   ClauseType = "fuzzy"    // document MUST contain a term within the given max distance
	Regex   ClauseType = "regex"    // document MUST contain a term that matches the given regex
)

type RangeClause struct {
	Field string
	Gte   *float64 // >= (nil = no lower bound)
	Lte   *float64 // <= (nil = no upper bound)
	Gt    *float64 // >  (nil = no exclusive lower bound)
	Lt    *float64 // <  (nil = no exclusive upper bound)
}

type TermsClause struct {
	Field  string
	Values []string
}

type RegexClause struct {
	Field string
	Regex string
}

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
	Ranges  []RangeClause
	Terms   []TermsClause
	Regexes []RegexClause
}

// Builder constructs a Query fluently.
type Builder struct {
	clauses []Clause
	ranges  []RangeClause
	terms   []TermsClause
	regexes []RegexClause
}

func NewBuilder() *Builder {
	return &Builder{clauses: []Clause{}, ranges: []RangeClause{}, terms: []TermsClause{}, regexes: []RegexClause{}}
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

func (b *Builder) Range(field string, gte, lte *float64) *Builder {
	b.ranges = append(b.ranges, RangeClause{Field: field, Gte: gte, Lte: lte})
	return b
}

// Terms adds a mandatory multi-value filter: doc must match at least one value.
func (b *Builder) Terms(field string, values ...string) *Builder {
	b.terms = append(b.terms, TermsClause{Field: field, Values: values})
	return b
}

func (b *Builder) Regex(field, regex string) *Builder {
	b.regexes = append(b.regexes, RegexClause{Field: field, Regex: regex})
	return b
}

func (b *Builder) Build() Query {
	return Query{Clauses: b.clauses, Ranges: b.ranges, Terms: b.terms, Regexes: b.regexes}
}

func Ptr(v float64) *float64 { return &v }
