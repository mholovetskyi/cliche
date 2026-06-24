package style

import "testing"

func TestApplyTheme(t *testing.T) {
	defer ApplyTheme("coral") // restore the default palette after the test

	if !ApplyTheme("ocean") {
		t.Fatal("ocean should apply")
	}
	if CurrentTheme != "ocean" {
		t.Fatalf("CurrentTheme = %q, want ocean", CurrentTheme)
	}
	if RedRGB != (RGB{56, 189, 248}) {
		t.Fatalf("accent not swapped by theme: %+v", RedRGB)
	}
	if ApplyTheme("nope") {
		t.Fatal("an unknown theme should return false")
	}
	if CurrentTheme != "ocean" {
		t.Fatal("a failed apply must not change the active theme")
	}
	if len(ThemeNames()) < 4 {
		t.Fatalf("expected several themes, got %v", ThemeNames())
	}
}
