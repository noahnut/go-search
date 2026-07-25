package wal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func tmpWAL(t *testing.T) (*WAL, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wal.log")
	wl, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { wl.Close() })
	return wl, path
}

// TestWAL_Append_Replay verifies a basic write + replay roundtrip.
func TestWAL_Append_Replay(t *testing.T) {
	wl, path := tmpWAL(t)

	want := []Entry{
		{Op: OpIndex, DocID: "1", Fields: map[string]any{"body": "hello"}},
		{Op: OpIndex, DocID: "2", Fields: map[string]any{"body": "world"}},
		{Op: OpDelete, DocID: "1"},
	}
	for _, e := range want {
		if err := wl.Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	var got []Entry
	if err := Replay(path, func(e Entry) { got = append(got, e) }); err != nil {
		t.Fatalf("Replay: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i].Op != want[i].Op {
			t.Errorf("[%d] Op: want %v, got %v", i, want[i].Op, got[i].Op)
		}
		if got[i].DocID != want[i].DocID {
			t.Errorf("[%d] DocID: want %v, got %v", i, want[i].DocID, got[i].DocID)
		}
	}
}

// TestWAL_Replay_Order verifies entries come back in write order.
func TestWAL_Replay_Order(t *testing.T) {
	wl, path := tmpWAL(t)

	for i := 0; i < 5; i++ {
		wl.Append(Entry{Op: OpIndex, DocID: fmt.Sprintf("%d", i)})
	}

	var order []string
	Replay(path, func(e Entry) { order = append(order, e.DocID) })

	for i, id := range order {
		if id != fmt.Sprintf("%d", i) {
			t.Errorf("order[%d] = %q, want %q", i, id, fmt.Sprintf("%d", i))
		}
	}
}

// TestWAL_Delete_NoFields verifies delete entries have nil Fields after replay.
func TestWAL_Delete_NoFields(t *testing.T) {
	wl, path := tmpWAL(t)
	wl.Append(Entry{Op: OpDelete, DocID: "42"})

	var got Entry
	Replay(path, func(e Entry) { got = e })

	if got.Op != OpDelete {
		t.Errorf("Op: want %v, got %v", OpDelete, got.Op)
	}
	if got.DocID != "42" {
		t.Errorf("DocID: want 42, got %s", got.DocID)
	}
	if got.Fields != nil {
		t.Errorf("delete entry Fields should be nil, got %v", got.Fields)
	}
}

// TestWAL_Truncate verifies the file is empty and replay returns nothing after Truncate.
func TestWAL_Truncate(t *testing.T) {
	wl, path := tmpWAL(t)
	wl.Append(Entry{Op: OpIndex, DocID: "1"})
	wl.Append(Entry{Op: OpIndex, DocID: "2"})

	if err := wl.Truncate(); err != nil {
		t.Fatalf("Truncate: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("expected empty file after Truncate, got size %d", info.Size())
	}

	var count int
	Replay(path, func(Entry) { count++ })
	if count != 0 {
		t.Errorf("expected 0 entries after Truncate, got %d", count)
	}
}

// TestWAL_PartialLastLine verifies a truncated last line (crash mid-write) is ignored.
// Acceptance criteria: partial last line is skipped without error.
func TestWAL_PartialLastLine(t *testing.T) {
	wl, path := tmpWAL(t)
	wl.Append(Entry{Op: OpIndex, DocID: "1"})
	wl.Append(Entry{Op: OpIndex, DocID: "2"})
	wl.Close()

	// simulate crash: cut the last 5 bytes of the file (mid-JSON)
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := f.Stat()
	f.Truncate(info.Size() - 5)
	f.Close()

	var got []Entry
	err = Replay(path, func(e Entry) { got = append(got, e) })
	if err != nil {
		t.Errorf("expected nil error on partial last line, got: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 complete entry replayed, got %d", len(got))
	}
}

// TestWAL_ReplayFileNotFound verifies an error is returned for a missing file.
func TestWAL_ReplayFileNotFound(t *testing.T) {
	err := Replay("/nonexistent/path/wal.log", func(Entry) {})
	if err == nil {
		t.Error("expected error replaying non-existent file, got nil")
	}
}

// TestWAL_Fields_Keys verifies field keys survive a JSON round-trip.
func TestWAL_Fields_Keys(t *testing.T) {
	wl, path := tmpWAL(t)
	fields := map[string]any{
		"body":  "hello world",
		"title": "greeting",
	}
	wl.Append(Entry{Op: OpIndex, DocID: "1", Fields: fields})

	var got Entry
	Replay(path, func(e Entry) { got = e })

	if got.Fields == nil {
		t.Fatal("Fields is nil after replay")
	}
	for key := range fields {
		if _, ok := got.Fields[key]; !ok {
			t.Errorf("field %q missing after replay", key)
		}
	}
}

// TestWAL_Sync verifies Sync does not return an error.
func TestWAL_Sync(t *testing.T) {
	wl, _ := tmpWAL(t)
	wl.Append(Entry{Op: OpIndex, DocID: "1"})
	if err := wl.Sync(); err != nil {
		t.Errorf("Sync: %v", err)
	}
}

// TestWAL_EmptyFile verifies Replay on an empty file returns nil error and zero entries.
func TestWAL_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	var count int
	if err := Replay(path, func(Entry) { count++ }); err != nil {
		t.Errorf("expected nil error on empty file, got: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 entries from empty file, got %d", count)
	}
}
