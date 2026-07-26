package notifier

import (
	"math"
	"testing"
)

func TestComputeUsedByTypeDefaultsToSum(t *testing.T) {
	if got := computeUsedByType("", 10, 20); got != 30 {
		t.Fatalf("expected empty type to use sum, got %d", got)
	}
	if got := computeUsedByType("unknown", 10, 20); got != 30 {
		t.Fatalf("expected unknown type to use sum, got %d", got)
	}
	if got := computeUsedByType("sum", math.MaxInt64, 1); got != math.MaxInt64 {
		t.Fatalf("expected overflowing sum to saturate, got %d", got)
	}
}

func TestApplyTrafficMultiplier(t *testing.T) {
	tests := []struct {
		name       string
		used       int64
		multiplier float64
		want       int64
	}{
		{name: "one", used: 1024, multiplier: 1, want: 1024},
		{name: "double", used: 1024, multiplier: 2, want: 2048},
		{name: "fraction", used: 5, multiplier: 0.5, want: 3},
		{name: "invalid zero", used: 1024, multiplier: 0, want: 1024},
		{name: "invalid nan", used: 1024, multiplier: math.NaN(), want: 1024},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := applyTrafficMultiplier(test.used, test.multiplier); got != test.want {
				t.Fatalf("applyTrafficMultiplier() = %d, want %d", got, test.want)
			}
		})
	}
}
