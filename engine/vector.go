package engine

import (
	"math"
	"sort"
)

// CosineSimilarity returns the cosine similarity between two vectors.
// Returns 0 if either vector has zero magnitude.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, magA, magB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}

	if magA == 0 || magB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(magA) * math.Sqrt(magB))
}

// VectorSearch returns the topK documents ranked by cosine similarity
// between queryVector and each document's vector in the given field.
func (e *Engine) VectorSearch(field string, queryVector []float64, topK int) []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Result, 0)

	e.docStorage.Each(func(docID string, rawDocument []byte) {
		var doc Document
		if err := doc.UnmarshalJSON(rawDocument); err != nil {
			return
		}
		if docVector, ok := e.vectors[docID][field]; ok {
			score := CosineSimilarity(queryVector, docVector)
			if score > 0 {
				result = append(result, Result{
					ID:     docID,
					Score:  score,
					Fields: doc.Fields,
				})
			}
		}
	})

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	if topK <= 0 {
		return result
	}

	if len(result) < topK {
		topK = len(result)
	}

	return result[:topK]
}
