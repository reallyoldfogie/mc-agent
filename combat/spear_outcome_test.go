package combat

import "testing"

func TestSpearContactOutcomeByStage(t *testing.T) {
	engaged := ContactOutcome(SpearEngaged)
	if !engaged.Damage || !engaged.Knockback || !engaged.Dismount {
		t.Fatal("engaged stage should allow all charge effects")
	}
	tired := ContactOutcome(SpearTired)
	if !tired.Damage || !tired.Knockback || tired.Dismount {
		t.Fatal("tired stage should allow damage/knockback only")
	}
	disengaged := ContactOutcome(SpearDisengaged)
	if !disengaged.Damage || disengaged.Knockback || disengaged.Dismount {
		t.Fatal("disengaged stage should allow damage only")
	}
}
