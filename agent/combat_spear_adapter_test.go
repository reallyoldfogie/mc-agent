package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/combat"
)

func TestSpearJabRequestForTargetUsesCombatTargetData(t *testing.T) {
	target := combat.Target{EntityID: 42, X: 10, Y: 64, Z: -4, Distance: 3}
	request, err := spearJabRequestForTarget(target, "minecraft:iron_spear")
	if err != nil {
		t.Fatal(err)
	}
	if request.Mode != combat.SpearJab || request.TargetID != target.EntityID ||
		request.TargetPosition.X != target.X || request.TargetPosition.Y != target.Y ||
		request.TargetPosition.Z != target.Z {
		t.Fatalf("request does not preserve spear target: %+v", request)
	}
}

func TestSpearJabRequestRejectsOutOfRangeTarget(t *testing.T) {
	_, err := spearJabRequestForTarget(combat.Target{EntityID: 42, Distance: 7}, "minecraft:iron_spear")
	if err == nil {
		t.Fatal("out-of-range spear target should be rejected")
	}
}
