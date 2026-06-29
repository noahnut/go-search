package engine

import (
	"os"
	"sync"
)

type FieldMeta struct {
	FilePath string
	Offset   int64
	Length   int
	Boost    float64
}

// ChunkMeta is one chunk of a large field written to a temp file.
type ChunkMeta struct {
	ChunkID  string // parentDocID + ":chunk-N"
	ParentID string
	Field    string
	Meta     FieldMeta
}

type DocStore struct {
	mu       sync.RWMutex
	chunks   map[string]ChunkMeta // chunkID → ChunkMeta
	parentOf map[string]string    // chunkID → parentDocID
}

func NewDocStore() *DocStore {
	return &DocStore{
		chunks:   make(map[string]ChunkMeta),
		parentOf: make(map[string]string),
	}
}

func (ds *DocStore) PutChunk(c ChunkMeta) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.chunks[c.ChunkID] = c
	ds.parentOf[c.ChunkID] = c.ParentID
}

func (ds *DocStore) ChunksFor(parentID string) []ChunkMeta {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	var chunks []ChunkMeta
	for chunkID, pID := range ds.parentOf {
		if pID == parentID {
			chunks = append(chunks, ds.chunks[chunkID])
		}
	}
	return chunks
}

func (ds *DocStore) ParentOf(chunkID string) (string, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	parentID, ok := ds.parentOf[chunkID]
	return parentID, ok
}

// removes all chunks for this parent
func (ds *DocStore) DeleteParent(parentID string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	for chunkID, pID := range ds.parentOf {
		if pID == parentID {
			delete(ds.parentOf, chunkID)
			delete(ds.chunks, chunkID)
		}
	}
}

// number of live parent documents
func (ds *DocStore) Size() int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	parents := make(map[string]struct{})
	for _, parentID := range ds.parentOf {
		parents[parentID] = struct{}{}
	}
	return len(parents)
}

// ReadChunk reads the text of one chunk from disk using file.ReadAt.
func (ds *DocStore) ReadChunk(chunkID string) (string, error) {
	ds.mu.RLock()
	chunkMeta, ok := ds.chunks[chunkID]
	ds.mu.RUnlock()

	if !ok {
		return "", nil
	}

	f, err := os.Open(chunkMeta.Meta.FilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, chunkMeta.Meta.Length)
	_, err = f.ReadAt(buf, chunkMeta.Meta.Offset)
	if err != nil {
		return "", err
	}

	return string(buf), nil
}
