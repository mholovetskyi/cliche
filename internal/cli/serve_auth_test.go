package cli

import "testing"

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:7878": true,
		"localhost:7878": true,
		"[::1]:7878":     true,
		"127.0.0.1":      true,
		":7878":          false, // all interfaces → needs a token
		"0.0.0.0:7878":   false,
		"192.168.1.5:80": false,
		"10.0.0.1:7878":  false,
	}
	for addr, want := range cases {
		if got := isLoopback(addr); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestGenTokenUnique(t *testing.T) {
	a, b := genToken(), genToken()
	if a == "" || a == b {
		t.Fatalf("genToken should return unique non-empty tokens, got %q and %q", a, b)
	}
	if len(a) < 20 {
		t.Fatalf("token too short: %q", a)
	}
}
