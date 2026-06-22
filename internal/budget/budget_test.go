package budget

import (
	"errors"
	"testing"
)

func TestTokenCapIsHard(t *testing.T) {
	k := New(Limits{MaxTokens: 1000})
	if err := k.Record("mock", 400, 200); err != nil {
		t.Fatalf("unexpected early cap: %v", err)
	}
	err := k.Record("mock", 300, 200) // total now 1100 >= 1000
	if !errors.Is(err, ErrTokenCap) {
		t.Fatalf("expected ErrTokenCap, got %v", err)
	}
}

func TestRecordCachedCountsTokensButDiscountsDollars(t *testing.T) {
	// mock pricing is $1/1M in and $1/1M out.
	k := New(Limits{MaxTokens: 2_000_000, MaxUSD: 100})
	// 100k uncached in + 50k out + 800k cache-read + 50k cache-write.
	if err := k.RecordCached("mock", 100_000, 50_000, 800_000, 50_000); err != nil {
		t.Fatalf("unexpected cap: %v", err)
	}
	u := k.Usage()
	// The HARD token cap counts ALL tokens (incl. cache read/write).
	if u.TotalTokens() != 100_000+50_000+800_000+50_000 {
		t.Fatalf("token total should include cache tokens, got %d", u.TotalTokens())
	}
	// Dollars are discounted: 100k in + 50k out (full) + 800k read @0.1× + 50k
	// write @1.25× = 0.1 + 0.05 + 0.08 + 0.0625 = $0.2925 — far below counting
	// every token at the full input rate ($1.0M-equiv would be ~$1.00).
	if u.USD > 0.30 {
		t.Fatalf("cache discount not applied to dollars, got $%.4f", u.USD)
	}
}

func TestUSDCap(t *testing.T) {
	// mock pricing is $1/1M in and $1/1M out.
	k := New(Limits{MaxUSD: 0.001}) // $0.001 == 1000 tokens worth at mock pricing
	err := k.Record("mock", 600, 600)
	if !errors.Is(err, ErrUSDCap) {
		t.Fatalf("expected ErrUSDCap, got %v", err)
	}
}

func TestPreflightDoesNotMutate(t *testing.T) {
	k := New(Limits{MaxTokens: 1000})
	if err := k.Preflight("mock", 2000, 0); !errors.Is(err, ErrTokenCap) {
		t.Fatalf("preflight should project a breach, got %v", err)
	}
	if got := k.Usage().TotalTokens(); got != 0 {
		t.Fatalf("preflight mutated usage: got %d, want 0", got)
	}
}

func TestMidStreamCatchesUnderestimate(t *testing.T) {
	k := New(Limits{MaxTokens: 10_000})
	// A small preflight estimate passes...
	if err := k.Preflight("mock", 500, 100); err != nil {
		t.Fatalf("preflight should pass: %v", err)
	}
	// ...but the ACTUAL turn is far larger and must be caught on Record.
	if err := k.Record("mock", 9000, 2000); !errors.Is(err, ErrTokenCap) {
		t.Fatalf("mid-stream record should catch the blowout, got %v", err)
	}
}

func TestScopedChildCapTripsFirst(t *testing.T) {
	root := New(Limits{MaxTokens: 1000})
	child := root.Scoped(Limits{MaxTokens: 300})
	if err := child.Record("mock", 200, 50); err != nil { // child 250, root 250
		t.Fatalf("unexpected: %v", err)
	}
	if err := child.Record("mock", 60, 0); !errors.Is(err, ErrTokenCap) { // child 310 >= 300
		t.Fatalf("expected child token cap, got %v", err)
	}
}

func TestScopedChildCapStillChargesRoot(t *testing.T) {
	root := New(Limits{MaxTokens: 10000})
	child := root.Scoped(Limits{MaxTokens: 100})
	if err := child.Record("mock", 80, 40); !errors.Is(err, ErrTokenCap) { // child 120 >= 100
		t.Fatalf("expected child cap, got %v", err)
	}
	if root.Usage().TotalTokens() != 120 {
		t.Fatalf("root must be charged even when the child cap trips, got %d", root.Usage().TotalTokens())
	}
}

func TestScopedSessionCapEnforcedThroughChild(t *testing.T) {
	root := New(Limits{MaxTokens: 100})
	child := root.Scoped(Limits{MaxTokens: 100000}) // huge sub-cap
	if err := child.Record("mock", 80, 40); !errors.Is(err, ErrTokenCap) {
		t.Fatalf("the session cap must be enforced through the child, got %v", err)
	}
}

func TestScopedSpendBubblesToRoot(t *testing.T) {
	root := New(Limits{MaxTokens: 1_000_000})
	child := root.Scoped(Limits{})
	_ = child.Record("mock", 100, 50)
	if root.Usage().TotalTokens() != 150 {
		t.Fatalf("child spend should bubble to root, got %d", root.Usage().TotalTokens())
	}
}

func TestScopedRemainingIsTightest(t *testing.T) {
	root := New(Limits{MaxTokens: 1000})
	_ = root.Record("mock", 100, 0) // root rem 900
	child := root.Scoped(Limits{MaxTokens: 300})
	if tok, _ := child.Remaining(); tok != 300 {
		t.Fatalf("remaining should be min(300, 900)=300, got %d", tok)
	}
	_ = child.Record("mock", 250, 0) // child rem 50, root rem 650
	if tok, _ := child.Remaining(); tok != 50 {
		t.Fatalf("remaining should now be 50, got %d", tok)
	}
}

func TestRemaining(t *testing.T) {
	k := New(Limits{MaxTokens: 1000, MaxUSD: 1})
	_ = k.Record("mock", 300, 0)
	tok, _ := k.Remaining()
	if tok != 700 {
		t.Fatalf("remaining tokens: got %d, want 700", tok)
	}
}
