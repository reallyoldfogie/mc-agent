package combat

import (
	"testing"
	"time"
)

func TestSpearChargeStages(t *testing.T) {
	profile := SpearChargeProfile{
		EngagedDuration: 500 * time.Millisecond,
		TiredDuration:   time.Second,
		ContactCooldown: 10 * time.Millisecond,
	}
	cases := []struct {
		name    string
		elapsed time.Duration
		want    SpearChargeStage
	}{
		{"engaged", 100 * time.Millisecond, SpearEngaged},
		{"tired", 750 * time.Millisecond, SpearTired},
		{"disengaged", 2 * time.Second, SpearDisengaged},
	}
	for _, tc := range cases {
		if got := profile.StageAt(tc.elapsed); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSpearChargeContactCooldown(t *testing.T) {
	profile := SpearChargeProfile{ContactCooldown: time.Second}
	now := time.Unix(100, 0)
	if !profile.CanContact(now, time.Time{}) {
		t.Fatal("first contact should be allowed")
	}
	if profile.CanContact(now.Add(500*time.Millisecond), now) {
		t.Fatal("contact cooldown should still be active")
	}
	if !profile.CanContact(now.Add(time.Second), now) {
		t.Fatal("contact should be allowed after cooldown")
	}
}
