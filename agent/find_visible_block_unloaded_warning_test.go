package agent

import "testing"

// TestFindVisibleBlockUnloadedWarningOnlyFiresWhenSomethingWasUnloaded pins
// the two cases that matter: a clean "nothing here" (every candidate
// genuinely loaded) stays silent, and a failure with at least one unloaded
// candidate gets flagged as possibly spurious, naming the counts.
func TestFindVisibleBlockUnloadedWarningOnlyFiresWhenSomethingWasUnloaded(t *testing.T) {
	if _, ok := findVisibleBlockUnloadedWarning("minecraft:dirt", 8, 200, 0); ok {
		t.Fatalf("warned with candidatesUnloaded=0: every candidate was genuinely checked, so a real absence isn't spurious")
	}

	msg, ok := findVisibleBlockUnloadedWarning("minecraft:dirt", 8, 200, 37)
	if !ok {
		t.Fatalf("did not warn with candidatesUnloaded=37, want a warning")
	}
	for _, want := range []string{"minecraft:dirt", "37", "200", "8"} {
		if !contains(msg, want) {
			t.Fatalf("warning %q missing %q", msg, want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
