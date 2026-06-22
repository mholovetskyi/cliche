package pricing

import "testing"

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
