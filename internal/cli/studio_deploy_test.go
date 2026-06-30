package cli

import "testing"

func TestSanitizeRepoName(t *testing.T) {
	cases := map[string]string{
		"my site":          "my-site",
		"Cliché Studio!!!": "Clich-Studio", // drops non-ascii + punctuation, space→dash
		"already-fine_1.0": "already-fine_1.0",
		"":                 "cliche-site",
		"   ":              "cliche-site",
		"a/b\\c":           "abc",
	}
	for in, want := range cases {
		if got := sanitizeRepoName(in); got != want {
			t.Errorf("sanitizeRepoName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLastURL(t *testing.T) {
	cases := map[string]string{
		"Inspect: https://vercel.com/x/y\nProduction: https://my-app.vercel.app": "https://my-app.vercel.app",
		"no urls here":                 "",
		"trailing https://x.app/path.": "https://x.app/path",
	}
	for in, want := range cases {
		if got := lastURL(in); got != want {
			t.Errorf("lastURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseNetlifyURL(t *testing.T) {
	out := "Deploying...\n" + `{"site_id":"abc","url":"https://my-site.netlify.app","deploy_url":"https://deadbeef--my-site.netlify.app"}`
	if got := parseNetlifyURL(out); got != "https://my-site.netlify.app" {
		t.Fatalf("parseNetlifyURL = %q, want production url", got)
	}
	if got := parseNetlifyURL("no json"); got != "" {
		t.Fatalf("parseNetlifyURL(no json) = %q, want empty", got)
	}
}

func TestDeployTargetUnknown(t *testing.T) {
	if _, err := deployTarget(t.TempDir(), "s3"); err == nil {
		t.Fatal("unknown target should error")
	}
}
