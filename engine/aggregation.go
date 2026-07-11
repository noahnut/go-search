package engine

import (
	"sort"
	"strconv"

	"github.com/noahfan/go-search/query"
)

type MetricResult struct {
	Min   float64
	Max   float64
	Sum   float64
	Avg   float64
	Count int
}

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

// Metrics computes numeric statistics for field across all documents that match q.
// Only documents where field holds a parseable numeric value are included.
// If no matching numeric documents exist, Count is 0 and all others are 0.
func (e *Engine) Metrics(q query.Query, field string) MetricResult {
	results := e.Search(q, e.Size()) // fetch all matching docs

	var res MetricResult
	res.Max = -1e308 // initialize to a very small number
	res.Min = 1e308  // initialize to a very large number
	for _, r := range results.Hits {
		v, ok := r.Fields[field]
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(v.Value, 64)
		if err != nil {
			continue
		}
		res.Count++
		res.Sum += f
		if f < res.Min {
			res.Min = f
		}
		if f > res.Max {
			res.Max = f
		}
	}
	if res.Count > 0 {
		res.Avg = res.Sum / float64(res.Count)
	}

	if res.Count == 0 {
		res.Min = 0
		res.Max = 0
	}

	return res
}
