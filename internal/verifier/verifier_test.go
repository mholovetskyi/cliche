package verifier

import "testing"

func TestFlagsDeletedTest(t *testing.T) {
	diff := "--- a/x_test.go\n+++ b/x_test.go\n-func TestPay(t *testing.T) {\n-}\n"
	v := Inspect(diff)
	if v.Status != StatusFlagged {
		t.Fatalf("expected flagged, got %s", v.Status)
	}
	if len(v.Findings) == 0 || v.Findings[0].Rule != "deleted_test" {
		t.Fatalf("expected deleted_test finding, got %+v", v.Findings)
	}
}

func TestFlagsSwallowedError(t *testing.T) {
	diff := "+try:\n+    do()\n+except: pass\n"
	if v := Inspect(diff); v.Status != StatusFlagged {
		t.Fatalf("expected flagged, got %s", v.Status)
	}
}

func TestFlagsTrivialAssertion(t *testing.T) {
	diff := "+assertTrue(True)\n"
	if v := Inspect(diff); v.Status != StatusFlagged {
		t.Fatalf("expected flagged, got %s", v.Status)
	}
}

func TestCleanDiffIsUnverifiedNotVerified(t *testing.T) {
	diff := "--- a/x.go\n+++ b/x.go\n-x := 1\n+x := 2\n"
	v := Inspect(diff)
	if v.Status != StatusUnverified {
		t.Fatalf("clean diff should be unverified (never verified without re-run), got %s", v.Status)
	}
}
