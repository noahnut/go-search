package index

import (
	"math"
	"sort"
	"sync"
)

type FieldBound struct {
	Field    string
	Gte, Lte *float64 // inclusive bounds (nil = unbounded)
	Gt, Lt   *float64 // exclusive bounds (nil = unbounded)
}

// KDEntry holds the numeric values of all indexed fields for one document.
type KDEntry struct {
	Values  map[string]float64 // field → numeric value
	DocID   string
	deleted bool // tombstone; compacted on next Build
}

// kdNode is one node in the K-D tree.
// Internal nodes store a split; leaf nodes store entries.
type kdNode struct {
	// split dimension and value (internal nodes only)
	splitField string
	splitVal   float64

	// bounding box of all entries below this node (used for pruning)
	minVals map[string]float64
	maxVals map[string]float64

	left, right *kdNode   // nil in leaf nodes
	entries     []KDEntry // non-nil only in leaf nodes
}

const kdLeafSize = 64 // max entries per leaf before splitting

// KDTree indexes multi-dimensional numeric entries.
type KDTree struct {
	root   *kdNode
	fields []string // all indexed fields, in split-order rotation
	mu     sync.RWMutex
}

func NewKDTree() *KDTree {
	return &KDTree{
		root:   nil,
		fields: []string{},
		mu:     sync.RWMutex{},
	}
}

func (t *KDTree) Build(entries []KDEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	fieldSet := make(map[string]struct{})
	for _, entry := range entries {
		for field := range entry.Values {
			fieldSet[field] = struct{}{}
		}
	}

	t.fields = make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		t.fields = append(t.fields, field)
	}
	sort.Strings(t.fields)

	t.root = t.buildKDNode(entries, 0, t.fields)
}

func (t *KDTree) buildKDNode(entries []KDEntry, depth int, fields []string) *kdNode {

	if len(entries) == 0 {
		return nil
	}

	if len(entries) <= kdLeafSize {
		// Leaf node: store entries and compute bounding box.
		minVals := make(map[string]float64)
		maxVals := make(map[string]float64)
		for _, field := range fields {
			minVals[field] = math.Inf(1)
			maxVals[field] = math.Inf(-1)
		}
		for _, entry := range entries {
			for field, val := range entry.Values {
				if val < minVals[field] {
					minVals[field] = val
				}
				if val > maxVals[field] {
					maxVals[field] = val
				}
			}
		}
		return &kdNode{
			minVals: minVals,
			maxVals: maxVals,
			entries: entries,
		}
	}

	// Dimension rotation: choose split field based on depth.
	// TODO: could also choose field with largest spread for better balance (Max Span/Variance).
	splitField := fields[depth%len(fields)]
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Values[splitField] < entries[j].Values[splitField]
	})
	median := len(entries) / 2
	leftEntries := entries[:median]
	rightEntries := entries[median:]

	leftNode := t.buildKDNode(leftEntries, depth+1, fields)
	rightNode := t.buildKDNode(rightEntries, depth+1, fields)

	minVals := make(map[string]float64)
	maxVals := make(map[string]float64)
	for _, field := range fields {
		minVals[field] = math.Min(leftNode.minVals[field], rightNode.minVals[field])
		maxVals[field] = math.Max(leftNode.maxVals[field], rightNode.maxVals[field])
	}

	return &kdNode{
		splitField: splitField,
		splitVal:   entries[median].Values[splitField],
		minVals:    minVals,
		maxVals:    maxVals,
		left:       leftNode,
		right:      rightNode,
	}
}

func (t *KDTree) IsBuilt() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.root != nil
}

func (t *KDTree) MultiRange(bounds []FieldBound) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.root == nil {
		return nil
	}

	var result []string
	t.multiRangeNode(t.root, bounds, &result)
	return result
}

func (t *KDTree) multiRangeNode(node *kdNode, bounds []FieldBound, result *[]string) {
	if node == nil {
		return
	}

	// Check if node's bounding box intersects the query bounds.
	for _, bound := range bounds {
		if node.maxVals[bound.Field] < deref(bound.Gte, math.Inf(-1)) || node.minVals[bound.Field] > deref(bound.Lte, math.Inf(1)) {
			return
		}
	}

	if node.entries != nil {
		// Leaf node: check each entry.
		for _, entry := range node.entries {
			if entry.deleted {
				continue
			}
			matches := true
			for _, bound := range bounds {
				val := entry.Values[bound.Field]
				if (bound.Gte != nil && val < *bound.Gte) || (bound.Lte != nil && val > *bound.Lte) ||
					(bound.Gt != nil && val <= *bound.Gt) || (bound.Lt != nil && val >= *bound.Lt) {
					matches = false
					break
				}
			}
			if matches {
				*result = append(*result, entry.DocID)
			}
		}
		return
	}

	// Internal node: recurse into children.
	t.multiRangeNode(node.left, bounds, result)
	t.multiRangeNode(node.right, bounds, result)
}

func deref(ptr *float64, def float64) float64 {
	if ptr == nil {
		return def
	}
	return *ptr
}

func (t *KDTree) Insert(entry KDEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.root == nil {
		t.root = t.buildKDNode([]KDEntry{entry}, 0, t.fields)
		return
	}

	t.insertIntoNode(t.root, entry, 0)

}

func (t *KDTree) insertIntoNode(node *kdNode, entry KDEntry, depth int) {
	if node == nil {
		return
	}

	if node.entries != nil {
		// Leaf node: append entry and update bounding box.
		node.entries = append(node.entries, entry)
		for field, val := range entry.Values {
			if val < node.minVals[field] {
				node.minVals[field] = val
			}
			if val > node.maxVals[field] {
				node.maxVals[field] = val
			}
		}
		return
	}

	// Internal node: recurse into the appropriate child.
	splitField := node.splitField
	if entry.Values[splitField] < node.splitVal {
		t.insertIntoNode(node.left, entry, depth+1)
	} else {
		t.insertIntoNode(node.right, entry, depth+1)
	}

	// Update bounding box.
	for field, val := range entry.Values {
		if val < node.minVals[field] {
			node.minVals[field] = val
		}
		if val > node.maxVals[field] {
			node.maxVals[field] = val
		}
	}
}

// marks deleted; compacted on next Build
func (t *KDTree) Delete(docID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.deleteFromNode(t.root, docID)
}

func (t *KDTree) deleteFromNode(node *kdNode, docID string) {
	if node == nil {
		return
	}

	if node.entries != nil {
		// Leaf node: mark entry as deleted.
		for i := range node.entries {
			if node.entries[i].DocID == docID {
				node.entries[i].deleted = true
				return
			}
		}
		return
	}

	// Internal node: recurse into both children.
	t.deleteFromNode(node.left, docID)
	t.deleteFromNode(node.right, docID)
}
