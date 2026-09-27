package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/combat"
)

func TestProjectileForCombatWeapon(t *testing.T) {
	tests := []struct {
		weapon  combat.ProjectileWeapon
		item    string
		wantErr bool
	}{
		{weapon: combat.Bow, item: "minecraft:bow"},
		{weapon: combat.Crossbow, item: "minecraft:crossbow"},
		{weapon: combat.Trident, item: "minecraft:trident"},
	}
	for _, tt := range tests {
		item, _, err := projectileForCombatWeapon(tt.weapon)
		if (err != nil) != tt.wantErr {
			t.Errorf("weapon %d: error=%v, wantErr=%v", tt.weapon, err, tt.wantErr)
		}
		if err == nil && item != tt.item {
			t.Errorf("weapon %d: item=%q, want %q", tt.weapon, item, tt.item)
		}
	}
}

func TestRangedRequestForTarget(t *testing.T) {
	target := combat.Target{EntityID: 42, X: 10, Y: 2, Z: -4, Distance: 30}
	request, err := rangedRequestForTarget(target, combat.Bow)
	if err != nil {
		t.Fatal(err)
	}
	if request.TargetID != target.EntityID || request.TargetPosition.X != target.X {
		t.Fatalf("request does not preserve target: %+v", request)
	}
	if request.FlightTime <= 0 || request.ProjectileSpeed <= 0 {
		t.Fatalf("request has invalid timing: %+v", request)
	}
}
