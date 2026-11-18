package memorystore

import (
	"testing"
	"time"
)

func TestSighting_Confidence(t *testing.T) {
	now := time.Now()

	fresh := Sighting{ObservedAt: now}
	if got := fresh.Confidence(now); got != 1 {
		t.Errorf("Confidence at age 0 = %v, want 1", got)
	}

	oneHalfLife := Sighting{ObservedAt: now.Add(-sightingConfidenceHalfLife)}
	if got := oneHalfLife.Confidence(now); got < 0.49 || got > 0.51 {
		t.Errorf("Confidence at one half-life = %v, want ~0.5", got)
	}

	twoHalfLives := Sighting{ObservedAt: now.Add(-2 * sightingConfidenceHalfLife)}
	if got := twoHalfLives.Confidence(now); got < 0.24 || got > 0.26 {
		t.Errorf("Confidence at two half-lives = %v, want ~0.25", got)
	}

	veryOld := Sighting{ObservedAt: now.Add(-1000 * sightingConfidenceHalfLife)}
	if got := veryOld.Confidence(now); got < 0 || got >= 0.01 {
		t.Errorf("Confidence at 1000 half-lives = %v, want near 0 (but non-negative)", got)
	}

	// Clock skew / a sighting timestamped slightly in the future must not
	// produce confidence above 1 or a negative age.
	future := Sighting{ObservedAt: now.Add(time.Hour)}
	if got := future.Confidence(now); got != 1 {
		t.Errorf("Confidence for a future ObservedAt = %v, want 1 (clamped, not >1)", got)
	}
}
