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

func TestCriticalHealthForcesRetreatWithoutAttacking(t *testing.T) {
	obs := Observation{
		Health: 4, MaxHealth: 20, HasTarget: true, TargetVisible: true,
		TargetDistance: 3, TargetDirectionX: 1,
	}
	decision := Decide(obs)
	if decision.State != Retreating || decision.Attack {
		t.Fatalf("critical health must retreat without attacking: %+v", decision)
	}

	intent := MovementFor(obs, decision, false)
	if intent.Action != RetreatFromTarget || intent.ThrottleX != -1 || !intent.Sprint || !intent.Jump {
		t.Fatalf("critical health should produce a sprinting retreat: %+v", intent)
	}
}

func TestMultipleEnemiesForceRetreatWithoutAttacking(t *testing.T) {
	decision := Decide(Observation{
		Health: 20, MaxHealth: 20, EnemyCount: 4,
		HasTarget: true, TargetVisible: true, TargetDistance: 3,
		TargetDirectionX: 1,
	})
	if decision.State != Retreating || decision.Attack {
		t.Fatalf("four active enemies should force retreat without attacking: %+v", decision)
	}
	intent := MovementFor(Observation{HasTarget: true, TargetDirectionX: 1}, decision, false)
	if intent.Action != RetreatFromTarget || intent.ThrottleX != -1 || !intent.Sprint || !intent.Jump {
		t.Fatalf("multi-target pressure should produce a sprinting retreat: %+v", intent)
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

func TestCriticalHitAllowedRequiresFallingUnobstructedState(t *testing.T) {
	base := Observation{VerticalVelocity: -0.1}
	if !CriticalHitAllowed(base) {
		t.Fatal("falling unobstructed agent should permit a critical hit")
	}
	cases := []struct {
		name   string
		mutate func(*Observation)
	}{
		{"on ground", func(obs *Observation) { obs.OnGround = true }},
		{"rising", func(obs *Observation) { obs.VerticalVelocity = 0.1 }},
		{"in water", func(obs *Observation) { obs.InWater = true }},
		{"on vehicle", func(obs *Observation) { obs.OnVehicle = true }},
		{"blind", func(obs *Observation) { obs.HasBlindness = true }},
		{"sprinting", func(obs *Observation) { obs.Sprinting = true }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			obs := base
			test.mutate(&obs)
			if CriticalHitAllowed(obs) {
				t.Fatalf("critical should be disallowed for %s", test.name)
			}
		})
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
