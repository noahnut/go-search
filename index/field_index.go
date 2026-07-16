package index

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type FieldEntry struct {
	Value  string  // raw string value (numeric fields store the original string)
	NumVal float64 // parsed float; 0 if field is keyword
	DocID  string
}

// FieldIndex is a per-field sorted column store for numeric and keyword fields.
// It enables O(log n + k) range queries and O(k log k) sort without Bitcask reads.
type FieldIndex struct {
	entries   map[string][]FieldEntry // fieldName → sorted entries
	deleted   map[string]struct{}     // liveDocs set
	compacted atomic.Int32
	rwMutex   sync.RWMutex
}

func NewFieldIndex() *FieldIndex {
	return &FieldIndex{
		entries:   make(map[string][]FieldEntry),
		deleted:   make(map[string]struct{}),
		compacted: atomic.Int32{},
		rwMutex:   sync.RWMutex{},
	}
}

// Add inserts an entry for field. Call Sort after a batch of inserts.
func (fi *FieldIndex) Add(field, docID, rawValue string, isNumeric bool) {

	fi.rwMutex.Lock()
	defer fi.rwMutex.Unlock()

	delete(fi.deleted, docID)

	entry := FieldEntry{
		Value: rawValue,
		DocID: docID,
	}
	if isNumeric {
		// parse rawValue to float64
		if numVal, err := strconv.ParseFloat(rawValue, 64); err == nil {
			entry.NumVal = numVal
		}
	}
	fi.entries[field] = append(fi.entries[field], entry)
}

// Delete removes all entries for docID across all fields.
func (fi *FieldIndex) Delete(docID string) {
	fi.rwMutex.Lock()
	defer fi.rwMutex.Unlock()

	fi.deleted[docID] = struct{}{}
}

func (fi *FieldIndex) Compact() {

	if fi.compacted.Load() == 1 {
		return
	}

	fi.rwMutex.Lock()
	defer fi.rwMutex.Unlock()

	fi.compacted.Store(1)
	defer fi.compacted.Store(0)

	for field, entries := range fi.entries {
		compacted := make([]FieldEntry, 0, len(entries))
		for _, entry := range entries {
			if _, isDeleted := fi.deleted[entry.DocID]; !isDeleted {
				compacted = append(compacted, entry)
			}
		}
		fi.entries[field] = compacted
	}
}

// Sort sorts each field's entries. Must be called before Range or SortValues.
func (fi *FieldIndex) Sort() {
	fi.rwMutex.Lock()
	defer fi.rwMutex.Unlock()

	for field, entries := range fi.entries {

		sort.Slice(entries, func(i, j int) bool {
			if entries[i].NumVal != 0 && entries[j].NumVal != 0 {
				return entries[i].NumVal < entries[j].NumVal
			}
			return strings.Compare(entries[i].Value, entries[j].Value) < 0
		})
		fi.entries[field] = entries
	}
}

// Range returns docIDs where the numeric field satisfies the given bounds.
// Replaces NumericIndex from task 38.
func (fi *FieldIndex) Range(field string, gte, lte, gt, lt *float64) []string {
	fi.rwMutex.RLock()
	entries, ok := fi.entries[field]
	deleted := fi.deleted
	fi.rwMutex.RUnlock()

	if !ok {
		return nil
	}

	var result []string

	left, right := 0, len(entries)-1
	// find the left bound
	if gte != nil || gt != nil {
		for left <= right {
			mid := (left + right) / 2
			if gte != nil && entries[mid].NumVal < *gte {
				left = mid + 1
			} else if gt != nil && entries[mid].NumVal <= *gt {
				left = mid + 1
			} else {
				right = mid - 1
			}
		}
	}

	// find the right bound
	if lte != nil || lt != nil {
		for left <= right {
			mid := (left + right) / 2
			if lte != nil && entries[mid].NumVal > *lte {
				right = mid - 1
			} else if lt != nil && entries[mid].NumVal >= *lt {
				right = mid - 1
			} else {
				left = mid + 1
			}
		}
	}

	// collect results within bounds
	for _, entry := range entries[left : right+1] {
		if _, isDeleted := deleted[entry.DocID]; !isDeleted {
			result = append(result, entry.DocID)
		}
	}

	return result
}

// SortValues returns (docID, rawValue) pairs for the field in the given order,
// starting after the cursor (nil = from the beginning). Used by Search for sorting
// and SearchAfter without fetching documents from Bitcask.
func (fi *FieldIndex) SortValues(field string, desc bool, after *string) []FieldEntry {
	fi.rwMutex.RLock()
	entries, ok := fi.entries[field]
	deleted := fi.deleted
	fi.rwMutex.RUnlock()

	if !ok {
		return nil
	}

	var result []FieldEntry

	startIdx := 0
	if after != nil {
		for i, entry := range entries {
			if entry.DocID == *after {
				startIdx = i + 1
				break
			}
		}
	}

	if desc {
		for i := len(entries) - 1; i >= startIdx; i-- {
			entry := entries[i]
			if _, isDeleted := deleted[entry.DocID]; !isDeleted {
				result = append(result, entry)
			}
		}
	} else {
		for i := startIdx; i < len(entries); i++ {
			entry := entries[i]
			if _, isDeleted := deleted[entry.DocID]; !isDeleted {
				result = append(result, entry)
			}
		}
	}

	return result
}

func (fi *FieldIndex) Rebuild(idx *Index, numericFields, keywordFields map[string]bool) {
	fi.rwMutex.Lock()
	defer fi.rwMutex.Unlock()

	fi.entries = make(map[string][]FieldEntry)
	fi.deleted = make(map[string]struct{})

	for _, term := range idx.Terms() {
		parts := strings.SplitN(term, ":", 2)
		if len(parts) != 2 {
			continue
		}
		field, value := parts[0], parts[1]

		isNumeric := numericFields[field]
		isKeyword := keywordFields[field]
		if !isNumeric && !isKeyword {
			continue
		}

		for _, posting := range idx.Lookup(term) {
			fi.Add(field, posting.DocID, value, isNumeric)
		}
	}

	fi.Sort()
}
