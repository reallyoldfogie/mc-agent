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

func TestDecideShieldRaiseHoldAndRelease(t *testing.T) {
	base := Observation{
		Health: 20, MaxHealth: 20, HasTarget: true, TargetVisible: true,
		TargetDistance: 12, ShieldAvailable: true, TargetIsRanged: true,
	}
	if got := Decide(base); got.ShieldAction != RaiseShield || got.Attack {
		t.Fatalf("unraised shield should be raised before attacking: action=%v attack=%v", got.ShieldAction, got.Attack)
	}
	base.ShieldActive = true
	if got := Decide(base); got.ShieldAction != HoldShield || got.Attack {
		t.Fatalf("active shield should be held against ranged threat: action=%v attack=%v", got.ShieldAction, got.Attack)
	}
	base.TargetVisible = false
	if got := Decide(base); got.ShieldAction != LowerShield {
		t.Fatalf("shield should be lowered after losing line of sight: action=%v", got.ShieldAction)
	}
}

func TestDecideWithholdsMeleeAttackAgainstBlockingTarget(t *testing.T) {
	got := Decide(Observation{
		Health: 20, MaxHealth: 20, HasTarget: true, TargetVisible: true,
		TargetDistance: 3, CurrentWeapon: MeleeWeapon, TargetBlocking: true,
	})
	if got.State != Engaging || got.Weapon != MeleeWeapon || got.Attack {
		t.Fatalf("melee attack should be withheld against blocking target: %+v", got)
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
