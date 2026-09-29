package combat

import (
	"testing"
	"time"
)

func TestSpearJabProfileCooldownAndTargetLimit(t *testing.T) {
	profile := SpearJabProfile{Cooldown: 250 * time.Millisecond, MaxTargets: 2}
	now := time.Unix(10, 0)
	if !profile.CanAttack(now, now.Add(-time.Second)) {
		t.Fatal("jab should be ready after cooldown")
	}
	if profile.CanAttack(now, now.Add(-time.Millisecond)) {
		t.Fatal("jab should remain on cooldown")
	}
	targets := []Target{{EntityID: 1}, {EntityID: 2}, {EntityID: 3}}
	selected := profile.SelectTargets(targets)
	if len(selected) != 2 || selected[0].EntityID != 1 || selected[1].EntityID != 2 {
		t.Fatalf("selected targets: %+v", selected)
	}
}
