package combat

import "testing"

func TestSpearSupportedVersion(t *testing.T) {
	for _, version := range []string{"1.21.11", "1.21.11-pre1", "1.22.1", "26.1"} {
		if !SpearSupportedVersion(version) {
			t.Errorf("%s should support spears", version)
		}
	}
	for _, version := range []string{"1.21.10", "1.21.5", "1.20.6", "unknown"} {
		if SpearSupportedVersion(version) {
			t.Errorf("%s should not support spears", version)
		}
	}
}
