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

func TestRemaining(t *testing.T) {
	k := New(Limits{MaxTokens: 1000, MaxUSD: 1})
	_ = k.Record("mock", 300, 0)
	tok, _ := k.Remaining()
	if tok != 700 {
		t.Fatalf("remaining tokens: got %d, want 700", tok)
	}
}
