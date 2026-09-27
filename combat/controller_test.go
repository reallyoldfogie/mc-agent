package combat

import (
	"testing"
	"time"
)

func TestChooseWeaponUsesHysteresis(t *testing.T) {
	if got := ChooseWeapon(4, RangedWeapon); got != MeleeWeapon {
		t.Fatalf("close target should switch to melee: %v", got)
	}
	if got := ChooseWeapon(6, MeleeWeapon); got != MeleeWeapon {
		t.Fatalf("transition-band target should retain melee: %v", got)
	}
	if got := ChooseWeapon(9, MeleeWeapon); got != RangedWeapon {
		t.Fatalf("far target should switch to ranged: %v", got)
	}
	if got := ChooseWeapon(6, RangedWeapon); got != RangedWeapon {
		t.Fatalf("transition-band target should retain ranged: %v", got)
	}
}

func TestDecideStateTransitions(t *testing.T) {
	tests := []struct {
		name   string
		obs    Observation
		state  State
		attack bool
	}{
		{"idle", Observation{}, Idle, false},
		{"engaging", Observation{Health: 20, MaxHealth: 20, HasTarget: true, TargetVisible: true, TargetDistance: 3}, Engaging, true},
		{"evading low health", Observation{Health: 7, MaxHealth: 20, HasTarget: true, TargetVisible: true, TargetDistance: 3}, Evading, false},
		{"evading no line of sight", Observation{Health: 20, MaxHealth: 20, HasTarget: true, TargetDistance: 3}, Evading, false},
		{"retreating critical", Observation{Health: 3, MaxHealth: 20, HasTarget: true, TargetVisible: true}, Retreating, false},
	}
	for _, test := range tests {
		got := Decide(test.obs)
		if got.State != test.state || got.Attack != test.attack {
			t.Errorf("%s: got state=%v attack=%v, want state=%v attack=%v", test.name, got.State, got.Attack, test.state, test.attack)
		}
	}
}

func TestReadyToAttackUsesInjectedTimes(t *testing.T) {
	now := time.Unix(100, 0)
	if !ReadyToAttack(now, time.Time{}, MeleeWeapon) {
		t.Fatal("first attack should be ready")
	}
	if ReadyToAttack(now.Add(499*time.Millisecond), now, MeleeWeapon) {
		t.Fatal("melee attack should respect cooldown")
	}
	if !ReadyToAttack(now.Add(500*time.Millisecond), now, MeleeWeapon) {
		t.Fatal("melee attack should be ready at cooldown boundary")
	}
}
