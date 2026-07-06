package local

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func store(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "store.log"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func val(s string) []byte { return []byte(s) }

// --- Basic operations ---

func TestPutGet(t *testing.T) {
	s := store(t)
	if err := s.Put("lang", val("go")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	v, ok := s.Get("lang")
	if !ok || string(v) != "go" {
		t.Errorf("Get('lang'): got %q ok=%v, want 'go' true", v, ok)
	}
}

func TestGet_Missing(t *testing.T) {
	s := store(t)
	v, ok := s.Get("missing")
	if ok || v != nil {
		t.Errorf("Get missing key: got %q ok=%v, want nil false", v, ok)
	}
}

func TestPut_Overwrite(t *testing.T) {
	s := store(t)
	s.Put("key", val("first"))
	s.Put("key", val("second"))

	v, ok := s.Get("key")
	if !ok || string(v) != "second" {
		t.Errorf("after overwrite: got %q ok=%v, want 'second' true", v, ok)
	}
}

func TestDelete(t *testing.T) {
	s := store(t)
	s.Put("key", val("value"))
	if err := s.Delete("key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	v, ok := s.Get("key")
	if ok || v != nil {
		t.Errorf("after Delete: got %q ok=%v, want nil false", v, ok)
	}
}

func TestDelete_NonExistent(t *testing.T) {
	s := store(t)
	if err := s.Delete("ghost"); err != nil {
		t.Errorf("Delete non-existent key: %v", err)
	}
}

// --- Size ---

func TestSize_Empty(t *testing.T) {
	s := store(t)
	if n := s.Size(); n != 0 {
		t.Errorf("empty store Size: got %d, want 0", n)
	}
}

func TestSize_LiveKeys(t *testing.T) {
	s := store(t)
	s.Put("a", val("1"))
	s.Put("b", val("2"))
	s.Put("c", val("3"))
	if n := s.Size(); n != 3 {
		t.Errorf("Size after 3 puts: got %d, want 3", n)
	}
}

func TestSize_AfterOverwrite(t *testing.T) {
	s := store(t)
	s.Put("a", val("1"))
	s.Put("a", val("2"))
	if n := s.Size(); n != 1 {
		t.Errorf("Size after overwrite: got %d, want 1", n)
	}
}

func TestSize_AfterDelete(t *testing.T) {
	s := store(t)
	s.Put("a", val("1"))
	s.Put("b", val("2"))
	s.Delete("a")
	if n := s.Size(); n != 1 {
		t.Errorf("Size after delete: got %d, want 1", n)
	}
}

// --- Each ---

func TestEach_VisitsAllLiveKeys(t *testing.T) {
	s := store(t)
	s.Put("a", val("1"))
	s.Put("b", val("2"))
	s.Put("c", val("3"))
	s.Delete("b")

	got := map[string]string{}
	s.Each(func(k string, v []byte) { got[k] = string(v) })

	if len(got) != 2 {
		t.Errorf("Each: got %d entries, want 2", len(got))
	}
	if got["a"] != "1" {
		t.Errorf("Each: a=%q, want '1'", got["a"])
	}
	if got["c"] != "3" {
		t.Errorf("Each: c=%q, want '3'", got["c"])
	}
	if _, ok := got["b"]; ok {
		t.Error("Each: deleted key 'b' should not appear")
	}
}

func TestEach_EmptyStore(t *testing.T) {
	s := store(t)
	count := 0
	s.Each(func(k string, v []byte) { count++ })
	if count != 0 {
		t.Errorf("Each on empty store: called %d times, want 0", count)
	}
}

// --- Crash recovery ---

func TestCrashRecovery_LiveKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.log")

	s, _ := New(path)
	s.Put("a", val("1"))
	s.Put("b", val("2"))
	s.Put("c", val("3"))
	s.Close()

	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	for k, want := range map[string]string{"a": "1", "b": "2", "c": "3"} {
		v, ok := s2.Get(k)
		if !ok || string(v) != want {
			t.Errorf("after recovery Get(%q): got %q ok=%v, want %q true", k, v, ok, want)
		}
	}
}

func TestCrashRecovery_DeletePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.log")

	s, _ := New(path)
	s.Put("a", val("1"))
	s.Put("b", val("2"))
	s.Delete("a")
	s.Close()

	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	if v, ok := s2.Get("a"); ok {
		t.Errorf("deleted key 'a' survived recovery: got %q", v)
	}
	if v, ok := s2.Get("b"); !ok || string(v) != "2" {
		t.Errorf("live key 'b' after recovery: got %q ok=%v, want '2' true", v, ok)
	}
}

func TestCrashRecovery_OverwritePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.log")

	s, _ := New(path)
	s.Put("key", val("old"))
	s.Put("key", val("new"))
	s.Close()

	s2, _ := New(path)
	defer s2.Close()

	v, ok := s2.Get("key")
	if !ok || string(v) != "new" {
		t.Errorf("after recovery: got %q ok=%v, want 'new' true", v, ok)
	}
	if s2.Size() != 1 {
		t.Errorf("after recovery Size: got %d, want 1", s2.Size())
	}
}

// --- Truncated record ---

func TestTruncatedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.log")

	s, _ := New(path)
	s.Put("a", val("1"))
	s.Put("b", val("2"))
	s.Close()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := os.Truncate(path, fi.Size()-4); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	s2, err := New(path)
	if err != nil {
		t.Fatalf("New after truncation: %v", err)
	}
	defer s2.Close()

	v, ok := s2.Get("a")
	if !ok || string(v) != "1" {
		t.Errorf("key 'a' should survive truncation: got %q ok=%v", v, ok)
	}
}

// --- Compact ---

func TestCompact_ShrinksFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.log")
	s, _ := New(path)

	for i := 0; i < 100; i++ {
		s.Put("key", val("value"))
	}

	fi, _ := os.Stat(path)
	sizeBefore := fi.Size()

	if err := s.Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	fi, _ = os.Stat(path)
	if fi.Size() >= sizeBefore {
		t.Errorf("Compact did not shrink file: before=%d after=%d", sizeBefore, fi.Size())
	}
}

func TestCompact_LiveKeysIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.log")
	s, _ := New(path)

	s.Put("a", val("1"))
	s.Put("b", val("2"))
	s.Put("a", val("updated"))
	s.Delete("b")
	s.Put("c", val("3"))

	if err := s.Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	if v, ok := s.Get("a"); !ok || string(v) != "updated" {
		t.Errorf("after Compact Get('a'): got %q ok=%v, want 'updated' true", v, ok)
	}
	if v, ok := s.Get("c"); !ok || string(v) != "3" {
		t.Errorf("after Compact Get('c'): got %q ok=%v, want '3' true", v, ok)
	}
	if _, ok := s.Get("b"); ok {
		t.Error("after Compact: deleted key 'b' should not be retrievable")
	}
	if n := s.Size(); n != 2 {
		t.Errorf("after Compact Size: got %d, want 2", n)
	}
}

func TestCompact_ThenReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.log")
	s, _ := New(path)

	s.Put("x", val("hello"))
	s.Put("x", val("world"))
	s.Compact()
	s.Close()

	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen after compact: %v", err)
	}
	defer s2.Close()

	v, ok := s2.Get("x")
	if !ok || string(v) != "world" {
		t.Errorf("after compact+reopen: got %q ok=%v, want 'world' true", v, ok)
	}
}

// --- Concurrency ---

func TestConcurrent_PutGet(t *testing.T) {
	s := store(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			s.Put(string(rune('a'+n%26)), val("v"))
		}(i)
		go func(n int) {
			defer wg.Done()
			s.Get(string(rune('a' + n%26)))
		}(i)
	}
	wg.Wait()
}

func TestConcurrent_EachDuringWrite(t *testing.T) {
	s := store(t)
	for i := 0; i < 10; i++ {
		s.Put(string(rune('a'+i)), val("v"))
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			s.Put("new", val("v"))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			s.Each(func(k string, v []byte) {})
		}
	}()

	wg.Wait()
}
