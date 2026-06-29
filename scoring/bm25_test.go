package scoring

import (
	"math"
	"testing"
)

const eps = 1e-3 // tolerance for float comparisons

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < eps
}

func TestDefaultParams(t *testing.T) {
	p := DefaultParams()
	if p.K1 != 1.2 {
		t.Errorf("K1: got %v, want 1.2", p.K1)
	}
	if p.B != 0.75 {
		t.Errorf("B: got %v, want 0.75", p.B)
	}
}

func TestIDF(t *testing.T) {
	tests := []struct {
		name string
		N    int
		df   int
		want float64
	}{
		{
			// term appears in 1 of 10 docs → rare → high IDF
			// ln(1 + (10 - 1 + 0.5) / (1 + 0.5)) = ln(1 + 9.5/1.5) ≈ 1.993
			"rare term", 10, 1, 1.993,
		},
		{
			// term appears in every doc → common → near-zero IDF
			// ln(1 + (10 - 10 + 0.5) / (10 + 0.5)) = ln(1 + 0.5/10.5) ≈ 0.047
			"term in every doc", 10, 10, 0.047,
		},
		{
			// term in half the docs
			// ln(1 + (10 - 5 + 0.5) / (5 + 0.5)) = ln(1 + 5.5/5.5) = ln(2) ≈ 0.693
			"term in half", 10, 5, 0.693,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IDF(tt.N, tt.df)
			if !approxEqual(got, tt.want) {
				t.Errorf("IDF(%d, %d) = %.4f, want %.4f", tt.N, tt.df, got, tt.want)
			}
			// IDF must never be negative
			if got < 0 {
				t.Errorf("IDF(%d, %d) returned negative value: %.4f", tt.N, tt.df, got)
			}
		})
	}
}

func TestTF(t *testing.T) {
	p := DefaultParams()

	tests := []struct {
		name      string
		freq      float64
		docLen    int
		avgDocLen float64
		want      float64
	}{
		{
			// doc length == average → no length penalty
			// numerator = 1 * 2.2 = 2.2, denominator = 1 + 1.2 * 1.0 = 2.2 → 1.0
			"average length doc", 1, 5, 5.0, 1.0,
		},
		{
			// longer doc → length penalty → lower TF than average-length doc
			// denominator grows because docLen/avgDocLen > 1
			// numerator = 1 * 2.2 = 2.2, lengthPenalty = 0.25 + 0.75*2 = 1.75
			// denominator = 1 + 1.2*1.75 = 3.1 → 2.2/3.1 ≈ 0.710
			"long doc penalized", 1, 10, 5.0, 0.710,
		},
		{
			// higher freq → higher TF, but saturates below K1+1 = 2.2
			// freq=10: numerator=22, denominator=10+1.2=11.2 → 1.964
			"high frequency saturates", 10, 5, 5.0, 1.964,
		},
		{
			// freq=100: numerator=220, denominator=100+1.2=101.2 → 2.174
			// freq=10 above was 1.964 — the gap shrinks, showing saturation
			"very high frequency still below K1+1", 100, 5, 5.0, 2.174,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TF(tt.freq, tt.docLen, tt.avgDocLen, p)
			if !approxEqual(got, tt.want) {
				t.Errorf("TF(freq=%.0f, docLen=%d, avg=%.1f) = %.4f, want %.4f",
					tt.freq, tt.docLen, tt.avgDocLen, got, tt.want)
			}
		})
	}
}

func TestTF_Saturation(t *testing.T) {
	// TF must saturate: TF(100) should be only slightly above TF(10)
	p := DefaultParams()
	tf10 := TF(10, 5, 5.0, p)
	tf100 := TF(100, 5, 5.0, p)
	tf1000 := TF(1000, 5, 5.0, p)

	if tf100-tf10 >= 0.5 {
		t.Errorf("TF not saturating: tf10=%.3f tf100=%.3f (gap too large)", tf10, tf100)
	}
	if tf1000-tf100 >= 0.1 {
		t.Errorf("TF not saturating: tf100=%.3f tf1000=%.3f (gap too large)", tf100, tf1000)
	}
	// All must be below K1+1 = 2.2
	if tf1000 >= p.K1+1 {
		t.Errorf("TF(1000) = %.4f should be below K1+1 = %.1f", tf1000, p.K1+1)
	}
}

func TestScore(t *testing.T) {
	p := DefaultParams()

	// Rare term in short doc should score higher than common term in long doc
	rareInShort := Score(2, 5, 10.0, 100, 2, p, 1.0)
	commonInLong := Score(2, 20, 10.0, 100, 80, p, 1.0)

	if rareInShort <= commonInLong {
		t.Errorf("rare term in short doc (%.4f) should outscore common term in long doc (%.4f)",
			rareInShort, commonInLong)
	}

	// Score must be non-negative
	if rareInShort < 0 || commonInLong < 0 {
		t.Error("Score returned negative value")
	}
}
