package pricing

import (
	"sort"
	"testing"
)

func TestModelsSortedAndComplete(t *testing.T) {
	ms := Models()
	if len(ms) != len(table) {
		t.Fatalf("Models() returned %d entries, table has %d", len(ms), len(table))
	}
	if !sort.SliceIsSorted(ms, func(i, j int) bool { return ms[i].Model < ms[j].Model }) {
		t.Fatal("Models() must be sorted by model id")
	}
	for _, e := range ms {
		if e.Price != table[e.Model] {
			t.Fatalf("Models() price for %q disagrees with table", e.Model)
		}
	}
	if Fallback() != fallback {
		t.Fatal("Fallback() must equal the internal fallback price")
	}
}

func TestLookupKnownAndUnknown(t *testing.T) {
	if p, ok := Lookup("claude-sonnet-4-6"); !ok || p.InputPerM != 3 || p.OutputPerM != 15 {
		t.Fatalf("known model lookup wrong: %+v ok=%v", p, ok)
	}
	p, ok := Lookup("totally-made-up-model")
	if ok {
		t.Fatal("unknown model should report ok=false")
	}
	if p != fallback {
		t.Fatalf("unknown model should use fallback, got %+v", p)
	}
}

func TestFallbackIsConservative(t *testing.T) {
	// The fallback must be >= every table entry so an unknown model can never
	// under-estimate cost.
	for name, p := range table {
		if fallback.InputPerM < p.InputPerM || fallback.OutputPerM < p.OutputPerM {
			t.Fatalf("fallback (%v) is cheaper than %q (%v)", fallback, name, p)
		}
	}
}

func TestCostUSDCached(t *testing.T) {
	p := Price{InputPerM: 3, OutputPerM: 15}
	// 1M uncached in + 1M out = $3 + $15 = $18; plus 1M cache-read @0.1× = $0.30;
	// plus 1M cache-write @1.25× = $3.75. Total $22.05.
	got := p.CostUSDCached(1_000_000, 1_000_000, 1_000_000, 1_000_000)
	if got < 22.04 || got > 22.06 {
		t.Fatalf("CostUSDCached = %v, want ~22.05", got)
	}
	// With no cached tokens it must equal CostUSD.
	if p.CostUSDCached(1000, 500, 0, 0) != p.CostUSD(1000, 500) {
		t.Fatal("CostUSDCached with no cache should equal CostUSD")
	}
	// A cache read is far cheaper than the same tokens uncached.
	if p.CostUSDCached(0, 0, 1_000_000, 0) >= p.CostUSD(1_000_000, 0) {
		t.Fatal("cache reads should cost less than uncached input")
	}
}

func TestCostUSD(t *testing.T) {
	p := Price{InputPerM: 3, OutputPerM: 15}
	// 1,000,000 in + 1,000,000 out = $3 + $15 = $18.
	if got := p.CostUSD(1_000_000, 1_000_000); got != 18 {
		t.Fatalf("CostUSD = %v, want 18", got)
	}
	if got := p.CostUSD(0, 0); got != 0 {
		t.Fatalf("CostUSD(0,0) = %v, want 0", got)
	}
}
