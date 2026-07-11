package engine

import (
	"sort"

	"github.com/noahfan/go-search/query"
)

// AggResult holds the outcome of one aggregation.
type AggResult struct {
	Buckets []Bucket
}

// Bucket is one group: a field value and its document count.
type Bucket struct {
	Key   string
	Count int
}

// Aggregate runs a terms aggregation over the results of a query:
// groups documents by the value of `field` and counts each group.
// Returns buckets sorted by count descending.
func (e *Engine) Aggregate(q query.Query, field string, topK int) AggResult {

	bucketCounts := make(map[string]int)
	if len(q.Clauses) == 0 {
		e.mu.RLock()

		e.docStorage.Each(func(docID string, rawDocument []byte) {
			var doc Document
			if err := doc.UnmarshalJSON(rawDocument); err != nil {
				return
			}

			if f, ok := doc.Fields[field]; ok {
				bucketCounts[f.Value]++
			}
		})
		e.mu.RUnlock()
	} else {
		for _, r := range e.Search(q, 0).Hits {
			if f, ok := r.Fields[field]; ok {
				bucketCounts[f.Value]++
			}
		}
	}

	buckets := make([]Bucket, 0, len(bucketCounts))
	for key, count := range bucketCounts {
		buckets = append(buckets, Bucket{Key: key, Count: count})
	}

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Count > buckets[j].Count
	})

	if topK > 0 && len(buckets) > topK {
		buckets = buckets[:topK]
	}

	return AggResult{Buckets: buckets}
}
