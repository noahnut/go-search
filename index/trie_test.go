package index

import (
	"testing"
)

func TestTrie_ExactMatch(t *testing.T) {
	trie := NewTrie()
	trie.Insert("golang")

	results := trie.Search("golang")
	if len(results) != 1 || results[0] != "golang" {
		t.Errorf("expected [golang], got %v", results)
	}
}

func TestTrie_PrefixReturnsCompletions(t *testing.T) {
	trie := NewTrie()
	trie.Insert("golang")
	trie.Insert("gold")
	trie.Insert("go")

	results := trie.Search("gol")
	if len(results) != 2 {
		t.Fatalf("expected 2 completions for 'gol', got %v", results)
	}

	got := map[string]bool{}
	for _, r := range results {
		got[r] = true
	}
	if !got["gold"] || !got["golang"] {
		t.Errorf("expected {gold, golang}, got %v", results)
	}
}

func TestTrie_EmptyPrefixReturnsAll(t *testing.T) {
	trie := NewTrie()
	trie.Insert("go")
	trie.Insert("python")
	trie.Insert("rust")

	results := trie.Search("")
	if len(results) != 3 {
		t.Errorf("expected 3 results for empty prefix, got %v", results)
	}
}

func TestTrie_NoMatchReturnsNil(t *testing.T) {
	trie := NewTrie()
	trie.Insert("golang")

	results := trie.Search("xyz")
	if results != nil {
		t.Errorf("expected nil for non-existent prefix, got %v", results)
	}
}

func TestTrie_SingleCharPrefix(t *testing.T) {
	trie := NewTrie()
	trie.Insert("go")
	trie.Insert("golang")
	trie.Insert("python")

	results := trie.Search("g")
	if len(results) != 2 {
		t.Fatalf("expected 2 results for prefix 'g', got %v", results)
	}
	got := map[string]bool{}
	for _, r := range results {
		got[r] = true
	}
	if !got["go"] || !got["golang"] {
		t.Errorf("expected {go, golang}, got %v", results)
	}
}

func TestTrie_PrefixIsAlsoTerm(t *testing.T) {
	// "go" is both a complete term and a prefix of "golang"
	// Search("go") should return both
	trie := NewTrie()
	trie.Insert("go")
	trie.Insert("golang")

	results := trie.Search("go")
	if len(results) != 2 {
		t.Fatalf("expected {go, golang}, got %v", results)
	}
	got := map[string]bool{}
	for _, r := range results {
		got[r] = true
	}
	if !got["go"] || !got["golang"] {
		t.Errorf("expected {go, golang}, got %v", results)
	}
}

func TestTrie_DuplicateInsert(t *testing.T) {
	trie := NewTrie()
	trie.Insert("golang")
	trie.Insert("golang")

	results := trie.Search("go")
	if len(results) != 1 {
		t.Errorf("duplicate insert should not produce duplicate results, got %v", results)
	}
}
