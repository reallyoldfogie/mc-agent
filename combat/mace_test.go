package combat

import "testing"

func TestClassifyMeleeItemRecognizesMace(t *testing.T) {
	if got := ClassifyMeleeItem("minecraft:mace"); got != MaceMeleeWeapon {
		t.Fatalf("mace classified as %v, want mace", got)
	}
}

func TestMaceSupportedVersion(t *testing.T) {
	if !MaceSupportedVersion("1.21.1") || MaceSupportedVersion("1.20.6") || MaceSupportedVersion("not-a-version") {
		t.Fatal("unexpected Mace version support result")
	}
}

func TestMaceSmashDamageProfile(t *testing.T) {
	r := MaceAttackRequest{TargetID: 1, ItemName: "mace", Distance: 2, MinReach: 1, MaxReach: 6, FallDistance: 4}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if !r.ShouldDealAdditionalDamage() || r.SmashBonusDamage() != 14 {
		t.Fatalf("unexpected smash profile: %+v", r)
	}
	r.Gliding = true
	if r.ShouldDealAdditionalDamage() || r.SmashBonusDamage() != 0 {
		t.Fatal("gliding attacker should not smash")
	}
}

func TestMaceKnockbackStrength(t *testing.T) {
	if got := MaceKnockbackStrength(1, 4); got != 1.75 {
		t.Fatalf("normal knockback = %v", got)
	}
	if got := MaceKnockbackStrength(1, 6); got != 3.5 {
		t.Fatalf("heavy knockback = %v", got)
	}
}
