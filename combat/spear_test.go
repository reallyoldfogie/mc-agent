package combat

import "testing"

func TestClassifyMeleeItemRecognizesSpearTiers(t *testing.T) {
	for _, item := range []string{
		"minecraft:wooden_spear",
		"minecraft:stone_spear",
		"minecraft:copper_spear",
		"minecraft:iron_spear",
		"minecraft:golden_spear",
		"minecraft:diamond_spear",
		"minecraft:netherite_spear",
	} {
		if got := ClassifyMeleeItem(item); got != SpearMeleeWeapon {
			t.Errorf("%q classified as %v, want spear", item, got)
		}
	}
}

func TestSpearModeForDistance(t *testing.T) {
	if got := SpearModeForDistance(4, false); got != SpearJab {
		t.Fatalf("without charge support: got %v, want jab", got)
	}
	if got := SpearModeForDistance(4, true); got != SpearCharge {
		t.Fatalf("with charge support: got %v, want charge", got)
	}
}
