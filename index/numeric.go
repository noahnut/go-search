package index

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/noahfan/go-search/storage"
)

const blockSize = 512 // entries per block

type numericEntry struct {
	value uint64
	docID string
}

// bkdNode is one node in the 1D BKD tree.
// Leaf nodes have blockKey != ""; internal nodes have left/right != nil.
type bkdNode struct {
	minEncoded  uint64
	maxEncoded  uint64
	left, right *bkdNode
	blockKey    string // non-empty only in leaf nodes
}

// NumericIndex is a per-field 1D BKD tree.
//
//   - Internal bkdNodes live in RAM (O(n/blockSize) nodes).
//   - Leaf block data lives in storage.Storage (Bitcask on disk).
//   - Pending entries live in a write buffer until Flush.
//   - Deletions are O(1) marks; compacted at Flush.
type NumericIndex struct {
	roots      map[string]*bkdNode       // field → tree root
	buffer     map[string][]numericEntry // write buffer
	deleted    map[string]struct{}       // liveDocs set
	fieldTypes map[string]string         // "float" or "int" — determines bound encoding in Range
	store      storage.Storage
	rwMutex    sync.RWMutex
}

func NewNumericIndex(store storage.Storage) *NumericIndex {
	return &NumericIndex{
		roots:      make(map[string]*bkdNode),
		buffer:     make(map[string][]numericEntry),
		deleted:    make(map[string]struct{}),
		fieldTypes: make(map[string]string),
		store:      store,
	}
}

// AddFloat stages a float64 entry. Call Flush to persist.
func (n *NumericIndex) AddFloat(field, docID string, v float64) {
	n.rwMutex.Lock()
	defer n.rwMutex.Unlock()

	n.fieldTypes[field] = "float"
	encoded := EncodeFloat64(v)
	n.buffer[field] = append(n.buffer[field], numericEntry{value: encoded, docID: docID})
}

// AddInt stages an int64 entry. Call Flush to persist.
func (n *NumericIndex) AddInt(field, docID string, v int64) {
	n.rwMutex.Lock()
	defer n.rwMutex.Unlock()

	n.fieldTypes[field] = "int"
	encoded := EncodeInt64(v)
	n.buffer[field] = append(n.buffer[field], numericEntry{value: encoded, docID: docID})
}

// Delete marks docID as deleted. O(1). Applied on next Flush.
func (n *NumericIndex) Delete(docID string) {
	n.rwMutex.Lock()
	defer n.rwMutex.Unlock()

	n.deleted[docID] = struct{}{}
}

// Flush sorts the write buffer, compacts deleted entries, splits into
// blocks of blockSize, and writes each block to storage.
func (n *NumericIndex) Flush() {
	n.rwMutex.Lock()
	defer n.rwMutex.Unlock()

	// Collect fields to process: anything in the buffer, plus any field
	// that already has an on-disk tree and has pending deletions to compact.
	fieldsToFlush := make(map[string]struct{})
	for field := range n.buffer {
		fieldsToFlush[field] = struct{}{}
	}
	if len(n.deleted) > 0 {
		for field := range n.roots {
			fieldsToFlush[field] = struct{}{}
		}
	}

	for field := range fieldsToFlush {
		fieldEntriesInDisk := n.loadAllEntries(field)
		allEntries := append(fieldEntriesInDisk, n.buffer[field]...)

		live := allEntries[:0]
		for _, e := range allEntries {
			if _, del := n.deleted[e.docID]; !del {
				live = append(live, e)
			}
		}

		sort.Slice(live, func(i, j int) bool {
			return live[i].value < live[j].value
		})

		var leaves []*bkdNode
		for i := 0; i < len(live); i += blockSize {
			end := i + blockSize
			if end > len(live) {
				end = len(live)
			}
			block := live[i:end]
			key := fmt.Sprintf("_numblock:%s:%d", field, i/blockSize)
			n.store.Put(key, encodeNumericBlock(block))
			leaves = append(leaves, &bkdNode{
				minEncoded: block[0].value,
				maxEncoded: block[len(block)-1].value,
				blockKey:   key,
			})
		}

		n.roots[field] = buildTree(leaves)
	}

	n.buffer = make(map[string][]numericEntry)
	n.deleted = make(map[string]struct{})
}

// Range returns docIDs where field satisfies the given bounds (expressed as float64).
// Uses the block directory to skip non-overlapping blocks.
func (n *NumericIndex) Range(field string, gte, lte, gt, lt *float64) []string {
	n.rwMutex.RLock()
	defer n.rwMutex.RUnlock()

	lo := uint64(0)
	hi := uint64(math.MaxUint64)
	if gte != nil {
		lo = n.encodeBound(field, *gte)
	}
	if gt != nil {
		lo = n.encodeBound(field, *gt) + 1
	}
	if lte != nil {
		hi = n.encodeBound(field, *lte)
	}
	if lt != nil {
		hi = n.encodeBound(field, *lt) - 1
	}

	var result []string
	n.searchTree(n.roots[field], lo, hi, &result)

	// also scan the write buffer (entries not yet flushed)
	for _, e := range n.buffer[field] {
		if _, del := n.deleted[e.docID]; del {
			continue
		}
		if e.value >= lo && e.value <= hi {
			result = append(result, e.docID)
		}
	}
	return result
}

// encodeBound converts a float64 bound to a uint64 using the same encoding
// that was used to store values for this field.
func (n *NumericIndex) encodeBound(field string, v float64) uint64 {
	if n.fieldTypes[field] == "int" {
		return EncodeInt64(int64(v))
	}
	return EncodeFloat64(v)
}

func (n *NumericIndex) searchTree(node *bkdNode, lo, hi uint64, result *[]string) {
	if node == nil {
		return
	}
	if node.maxEncoded < lo || node.minEncoded > hi {
		return
	}
	if node.blockKey != "" {
		blockEntries := n.loadBlock(node.blockKey)
		left, right := 0, len(blockEntries)-1
		for left <= right {
			mid := (left + right) / 2
			if blockEntries[mid].value < lo {
				left = mid + 1
			} else {
				right = mid - 1
			}
		}
		for i := left; i < len(blockEntries) && blockEntries[i].value <= hi; i++ {
			if _, del := n.deleted[blockEntries[i].docID]; !del {
				*result = append(*result, blockEntries[i].docID)
			}
		}
		return
	}
	n.searchTree(node.left, lo, hi, result)
	n.searchTree(node.right, lo, hi, result)
}

func buildTree(leaves []*bkdNode) *bkdNode {
	if len(leaves) == 0 {
		return nil
	}
	if len(leaves) == 1 {
		return &bkdNode{
			minEncoded: leaves[0].minEncoded,
			maxEncoded: leaves[0].maxEncoded,
			blockKey:   leaves[0].blockKey,
		}
	}
	mid := len(leaves) / 2
	left := buildTree(leaves[:mid])
	right := buildTree(leaves[mid:])
	return &bkdNode{
		minEncoded: left.minEncoded,
		maxEncoded: right.maxEncoded,
		left:       left,
		right:      right,
	}
}

func (n *NumericIndex) loadAllEntries(field string) []numericEntry {
	fieldRoot, ok := n.roots[field]
	if !ok {
		return nil
	}
	return n.loadEntries(fieldRoot)
}

func (n *NumericIndex) loadEntries(node *bkdNode) []numericEntry {
	if node == nil {
		return nil
	}
	if node.blockKey != "" {
		return n.loadBlock(node.blockKey)
	}
	leftEntries := n.loadEntries(node.left)
	rightEntries := n.loadEntries(node.right)
	return append(leftEntries, rightEntries...)
}

func (n *NumericIndex) loadBlock(blockKey string) []numericEntry {
	blockData, exist := n.store.Get(blockKey)
	if !exist {
		return nil
	}
	blockEntries, err := decodeNumericBlock(blockData)
	if err != nil {
		return nil
	}
	return blockEntries
}

// EncodeFloat64 maps float64 → uint64 preserving numeric order.
// Positive floats: flip sign bit. Negative floats: flip all bits.
// Result: 0.0 < 1.0 < 2.0 holds as uint64 ordering.
func EncodeFloat64(v float64) uint64 {
	bits := math.Float64bits(v)
	if bits>>63 == 1 {
		return bits ^ 0xffffffffffffffff
	}
	return bits ^ 0x8000000000000000
}

func DecodeFloat64(u uint64) float64 {
	if u>>63 == 0 {
		u ^= 0xffffffffffffffff
	} else {
		u ^= 0x8000000000000000
	}
	return math.Float64frombits(u)
}

// EncodeInt64 maps int64 → uint64 preserving numeric order.
// Flip the sign bit: MinInt64 → 0, 0 → 1<<63, MaxInt64 → MaxUint64.
func EncodeInt64(v int64) uint64 {
	return uint64(v) ^ 0x8000000000000000
}

func DecodeInt64(u uint64) int64 {
	return int64(u ^ 0x8000000000000000)
}

// decodeNumericBlock reads a variable-length encoded block.
// Format per entry: 8 bytes value | 4 bytes docID length | N bytes docID.
func decodeNumericBlock(data []byte) ([]numericEntry, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("data too short to contain entry count")
	}

	count := int(binary.BigEndian.Uint32(data[:4]))
	entries := make([]numericEntry, 0, count)
	offset := 4
	for i := 0; i < count; i++ {
		if offset+8 > len(data) {
			return nil, fmt.Errorf("unexpected end of data reading value at entry %d", i)
		}
		value := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if offset+4 > len(data) {
			return nil, fmt.Errorf("unexpected end of data reading docID length at entry %d", i)
		}
		idLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4

		if offset+idLen > len(data) {
			return nil, fmt.Errorf("unexpected end of data reading docID at entry %d", i)
		}
		docID := string(data[offset : offset+idLen])
		offset += idLen

		entries = append(entries, numericEntry{value: value, docID: docID})
	}

	return entries, nil
}

// encodeNumericBlock writes entries with variable-length docID encoding.
// Format: 4 bytes count | per entry: 8 bytes value | 4 bytes docID length | N bytes docID.
func encodeNumericBlock(entries []numericEntry) []byte {
	buf := new(bytes.Buffer)

	binary.Write(buf, binary.BigEndian, uint32(len(entries)))

	for _, entry := range entries {
		binary.Write(buf, binary.BigEndian, entry.value)
		docIDBytes := []byte(entry.docID)
		binary.Write(buf, binary.BigEndian, uint32(len(docIDBytes)))
		buf.Write(docIDBytes)
	}

	return buf.Bytes()
}
