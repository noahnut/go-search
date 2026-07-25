package wal

import (
	"bufio"
	"encoding/json"
	"os"
)

type OpType string

const (
	OpIndex  OpType = "index"
	OpDelete OpType = "delete"
)

// Entry is one record in the WAL file.
type Entry struct {
	Op     OpType         `json:"op"`
	DocID  string         `json:"doc_id"`
	Fields map[string]any `json:"fields,omitempty"` // non-nil for OpIndex
}

// WAL is an append-only log file.
type WAL struct {
	f *os.File
	w *bufio.Writer
}

func Open(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &WAL{
		f: f,
		w: bufio.NewWriter(f),
	}, nil
}

// Append writes one entry and flushes to the OS buffer.
// For full durability, call Sync after Append.
func (wl *WAL) Append(e Entry) error {
	// Encode the entry as JSON and write it to the buffer.
	enc := json.NewEncoder(wl.w)
	if err := enc.Encode(e); err != nil {
		return err
	}
	return wl.w.Flush()
}

// Sync calls fsync to guarantee the entry is on disk.
func (wl *WAL) Sync() error {
	if err := wl.w.Flush(); err != nil {
		return err
	}
	return wl.f.Sync()
}

// Close flushes and closes the file.
func (wl *WAL) Close() error {
	if err := wl.w.Flush(); err != nil {
		return err
	}
	return wl.f.Close()
}

// Replay reads all entries from path and calls fn for each one.
// Used during recovery.
func Replay(path string, fn func(Entry)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip invalid entries
		}
		fn(e)
	}
	return scanner.Err()
}

// Truncate removes all entries up to (and including) the current position.
// Call after a successful snapshot so the log doesn't grow forever.
func (wl *WAL) Truncate() error {
	if err := wl.w.Flush(); err != nil {
		return err
	}
	return wl.f.Truncate(0)
}
