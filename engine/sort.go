package engine

import (
	"sort"
	"strconv"
)

type SortOrder int

const (
	Asc SortOrder = iota
	Desc
)

type SortClause struct {
	Field string
	Order SortOrder
}

func SortBy(field string, order SortOrder) SearchOptions {
	return SearchOptions{Sort: []SortClause{{Field: field, Order: order}}}
}

func (e *Engine) Sort(results []Result, options SearchOptions) []Result {
	if len(options.Sort) == 0 {
		return results // no sorting needed
	}

	sort.SliceStable(results, func(i, j int) bool {
		for _, sc := range options.Sort {
			vi := results[i].Fields[sc.Field].Value
			vj := results[j].Fields[sc.Field].Value
			// compare as float if parseable, otherwise as string
			cmp := compareFieldValues(vi, vj)
			if cmp != 0 {
				if sc.Order == Desc {
					return cmp > 0
				}
				return cmp < 0
			}
		}
		// tiebreaker: score descending
		return results[i].Score > results[j].Score
	})

	return results
}

func compareFieldValues(a, b string) int {
	// try to parse as float
	af, errA := strconv.ParseFloat(a, 64)
	bf, errB := strconv.ParseFloat(b, 64)
	if errA == nil && errB == nil {
		if af < bf {
			return -1
		} else if af > bf {
			return 1
		}
		return 0
	}

	// fallback to string comparison
	if a < b {
		return -1
	} else if a > b {
		return 1
	}
	return 0
}
