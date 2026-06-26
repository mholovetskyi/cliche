package cli

import "testing"

func TestSanitizeRepoName(t *testing.T) {
	cases := map[string]string{
		"my site":          "my-site",
		"Cliché Studio!!!":  "Clich-Studio", // drops non-ascii + punctuation, space→dash
		"already-fine_1.0":  "already-fine_1.0",
		"":                  "cliche-site",
		"   ":               "cliche-site",
		"a/b\\c":            "abc",
	}
	for in, want := range cases {
		if got := sanitizeRepoName(in); got != want {
			t.Errorf("sanitizeRepoName(%q) = %q, want %q", in, got, want)
		}
	}
}
