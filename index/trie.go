package index

type TrieNode struct {
	children map[rune]*TrieNode
	isEnd    bool   // true if a complete term ends here
	term     string // the full term (only set when isEnd=true)
}

type Trie struct {
	root *TrieNode
}

func NewTrie() *Trie {
	return &Trie{
		root: &TrieNode{children: make(map[rune]*TrieNode)},
	}
}

func (t *Trie) Insert(term string) {
	node := t.root
	for _, r := range term {
		if _, exists := node.children[r]; !exists {
			node.children[r] = &TrieNode{children: make(map[rune]*TrieNode)}
		}
		node = node.children[r]
	}

	node.isEnd = true
	node.term = term
}

func (t *Trie) collectTerms(node *TrieNode, results *[]string) {
	if node.isEnd {
		*results = append(*results, node.term)
	}

	for _, child := range node.children {
		t.collectTerms(child, results)
	}
}

// returns all terms with this prefix
func (t *Trie) Search(prefix string) []string {
	node := t.root
	var results []string
	for _, r := range prefix {
		if _, exists := node.children[r]; !exists {
			return nil
		}
		node = node.children[r]

	}

	t.collectTerms(node, &results)
	return results
}
