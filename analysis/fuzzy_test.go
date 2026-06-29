package analysis

import "testing"

func TestEditDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"go", "go", 0},          // identical
		{"goo", "go", 1},         // 1 deletion
		{"go", "goo", 1},         // 1 insertion
		{"cat", "cut", 1},        // 1 substitution
		{"", "go", 2},            // insert both chars
		{"go", "", 2},            // delete both chars
		{"kitten", "sitting", 3}, // classic example
		{"golang", "go", 4},      // 4 deletions
	}

	for _, tt := range tests {
		got := EditDistance(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("EditDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
