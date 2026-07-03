package local

import (
	"encoding/binary"
	"io"
	"os"
	"sync"

	"github.com/noahfan/go-search/storage"
)

var _ storage.Storage = (*Store)(nil)

const defaultFilePath = "local_store.log"

const headerSize = 1 + 4 + 4 // 1B type + 4B keyLen + 4B valLen = 9 Bytes

const (
	typePut    uint8 = 1
	typeDelete uint8 = 2
)

type recordHeader struct {
	Type   uint8  // 1 Byte
	KeyLen uint32 // 4 Bytes
	ValLen uint32 // 4 Bytes
}

type entry struct {
	offset int64
	size   int64
}

// Store is an append-only, log-structured key-value store.
// All writes are sequential appends; reads use an in-memory offset index.
type Store struct {
	filePath string
	file     *os.File

	entries map[string]entry
	mu      sync.RWMutex
}

// opens or creates the log file
func New(path string) (*Store, error) {
	if path == "" {
		path = defaultFilePath
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)

	if err != nil {
		return nil, err
	}

	entries := make(map[string]entry)

	s := &Store{
		filePath: path,
		file:     file,
		entries:  entries,
		mu:       sync.RWMutex{},
	}

	// Load existing entries from the log file into memory
	if err := s.loadEntries(); err != nil {
		return nil, err
	}

	return s, nil
}

// Store implements storage.Storage
func (s *Store) Put(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Move to the end of the file for appending
	offset, err := s.file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	header := recordHeader{
		Type:   typePut,
		KeyLen: uint32(len(key)),
		ValLen: uint32(len(value)),
	}

	headerBytes := s.encodeHeader(header)

	// Write the header to the file
	if _, err = s.file.Write(headerBytes); err != nil {
		return err
	}

	if _, err = s.file.WriteString(key); err != nil {
		return err
	}

	// Write the value to the file
	n, err := s.file.WriteString(value)
	if err != nil {
		return err
	}

	recordSize := int64(headerSize) + int64(len(key)) + int64(n)

	// Update the in-memory index with the new entry
	s.entries[key] = entry{offset: offset, size: recordSize}

	return nil
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.get(key)
}

func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Move to the end of the file for appending
	if _, err := s.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	header := recordHeader{
		Type:   typeDelete,
		KeyLen: uint32(len(key)),
		ValLen: 0,
	}

	headerBytes := s.encodeHeader(header)

	// Write the header to the file
	if _, err := s.file.Write(headerBytes); err != nil {
		return err
	}

	if _, err := s.file.WriteString(key); err != nil {
		return err
	}

	// Remove the entry from the in-memory index
	delete(s.entries, key)

	return nil
}

func (s *Store) Each(fn func(key, value string)) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for key := range s.entries {
		value, ok := s.get(key)
		if ok {
			fn(key, value)
		}
	}
}

func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.entries)
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.file.Sync() // Ensure all writes are flushed to disk

	return s.file.Close()
}

// Compact rewrites the log keeping only live keys, shrinking the file.
// Called by the user or the persistence manager after a snapshot.
func (s *Store) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a temporary file for the compacted log
	tempFile, err := os.CreateTemp("", "local_store_compact_*.log")
	if err != nil {
		return err
	}
	defer tempFile.Close()

	newEntries := make(map[string]entry)
	offset := int64(0)

	// Write only live entries to the temporary file
	for key := range s.entries {
		value, ok := s.get(key)
		if !ok {
			continue // Skip if the key is not found (should not happen)
		}

		header := recordHeader{
			Type:   typePut,
			KeyLen: uint32(len(key)),
			ValLen: uint32(len(value)),
		}

		headerBytes := s.encodeHeader(header)

		// Write the header, key, and value to the temporary file
		if _, err := tempFile.Write(headerBytes); err != nil {
			return err
		}
		if _, err := tempFile.WriteString(key); err != nil {
			return err
		}
		if _, err := tempFile.WriteString(value); err != nil {
			return err
		}

		recordSize := int64(headerSize) + int64(len(key)) + int64(len(value))
		newEntries[key] = entry{offset: offset, size: recordSize}
		offset += recordSize

	}

	// Close the original file and replace it with the compacted file
	s.file.Close()
	if err := os.Rename(tempFile.Name(), s.filePath); err != nil {
		return err
	}

	// Reopen the compacted file
	file, err := os.OpenFile(s.filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	s.file = file
	s.entries = newEntries

	return nil
}

func (s *Store) encodeHeader(header recordHeader) []byte {
	buf := make([]byte, headerSize)

	buf[0] = header.Type
	binary.BigEndian.PutUint32(buf[1:5], header.KeyLen)
	binary.BigEndian.PutUint32(buf[5:9], header.ValLen)

	return buf
}

func (s *Store) decodeHeader(buf []byte) recordHeader {
	recordType := buf[0]
	keyLen := binary.BigEndian.Uint32(buf[1:5])
	valLen := binary.BigEndian.Uint32(buf[5:9])

	return recordHeader{
		Type:   recordType,
		KeyLen: keyLen,
		ValLen: valLen,
	}
}

func (s *Store) get(key string) (string, bool) {
	entry, ok := s.entries[key]
	if !ok {
		return "", false
	}

	// Read the value from the file using the stored offset and size
	buf := make([]byte, entry.size)
	_, err := s.file.ReadAt(buf, entry.offset)
	if err != nil {
		return "", false
	}

	header := s.decodeHeader(buf[:headerSize])

	valueOffset := headerSize + int(header.KeyLen)
	value := buf[valueOffset : valueOffset+int(header.ValLen)]

	if header.Type != typePut {
		return "", false
	}

	return string(value), true
}

func (s *Store) loadEntries() error {
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var offset int64
	for {
		header, err := s.readHeader()
		if err == io.EOF {
			break
		}
		if err == io.ErrUnexpectedEOF {
			break // truncated last record — stop, don't error
		}
		if err != nil {
			return err
		}

		keyBuf := make([]byte, header.KeyLen)
		if _, err := io.ReadFull(s.file, keyBuf); err != nil {
			break // truncated
		}

		recordSize := int64(headerSize) + int64(header.KeyLen) + int64(header.ValLen)

		if header.Type == typePut {
			s.entries[string(keyBuf)] = entry{offset: offset, size: recordSize}
		} else {
			delete(s.entries, string(keyBuf))
		}

		// skip over the value bytes
		if _, err := s.file.Seek(int64(header.ValLen), io.SeekCurrent); err != nil {
			break
		}
		offset += recordSize
	}
	return nil
}

func (s *Store) readHeader() (recordHeader, error) {
	buf := make([]byte, headerSize)
	if _, err := io.ReadFull(s.file, buf); err != nil {
		return recordHeader{}, err
	}

	return s.decodeHeader(buf), nil
}
