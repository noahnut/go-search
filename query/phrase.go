package query

import "github.com/noahfan/go-search/index"

// PhraseMatch checks whether docPostings contains the given terms
// in consecutive order. terms must already be analyzed (lowercased, etc.)
func PhraseMatch(terms []string, docPostings map[string]index.Posting) bool {
	if len(terms) == 0 {
		return false
	}

	positions := docPostings[terms[0]].Positions
	if len(positions) == 0 {
		return false
	}

	for _, position := range positions {

		matched := true
		for i, term := range terms[1:] {
			nextPostings, ok := docPostings[term]
			if !ok {
				return false
			}

			nextPositions := nextPostings.Positions
			if len(nextPositions) == 0 {
				return false
			}

			if !containsPosition(nextPositions, position+i+1) {
				matched = false
				break
			}
		}

		if matched {
			return true
		}
	}

	return false
}

func containsPosition(positions []int, targetPosition int) bool {
	positionMap := make(map[int]bool)
	for _, position := range positions {
		positionMap[position] = true
	}
	return positionMap[targetPosition]
}
