package combat

import (
	"testing"
	"time"
)

func TestMovementForApproachesMeleeTarget(t *testing.T) {
	obs := Observation{HasTarget: true, TargetDistance: 8, TargetDirectionX: 1}
	decision := Decision{State: Engaging, Weapon: MeleeWeapon}
	intent := MovementFor(obs, decision, false)
	if intent.Action != ApproachTarget || intent.ThrottleX != 1 || intent.ThrottleZ != 0 {
		t.Fatalf("unexpected approach intent: %#v", intent)
	}
}

func TestMovementForExplosiveTargetStrikesAndDisengages(t *testing.T) {
	decision, intent := NewController().Next(time.Now(), Observation{
		HasTarget: true, TargetVisible: true, TargetDistance: 2.5,
		TargetDirectionX: 1, TargetIsExplosive: true,
	})
	if !decision.Attack || decision.Weapon != MeleeWeapon {
		t.Fatalf("explosive target should be attacked in melee reach: %#v", decision)
	}
	if intent.Action != RetreatFromTarget || intent.ThrottleX >= 0 || !intent.Sprint {
		t.Fatalf("explosive target should trigger immediate disengagement: %#v", intent)
	}
	_, intent = NewController().Next(time.Now(), Observation{
		HasTarget: true, TargetVisible: true, TargetDistance: 3.0,
		TargetDirectionX: 1, TargetIsExplosive: true,
	})
	if intent.Action != RetreatFromTarget {
		t.Fatalf("explosive target should keep retreating until outside fuse range: %#v", intent)
	}

	_, intent = NewController().Next(time.Now(), Observation{
		HasTarget: true, TargetVisible: true, TargetDistance: 4,
		TargetDirectionX: 1, TargetIsExplosive: true,
	})
	if intent.Action != ApproachTarget || intent.ThrottleX <= 0 {
		t.Fatalf("explosive target outside melee reach should be approached: %#v", intent)
	}
}

func TestMovementForRetreatsAndEvades(t *testing.T) {
	obs := Observation{HasTarget: true, TargetDistance: 3, TargetDirectionZ: 1}
	retreat := MovementFor(obs, Decision{State: Retreating}, false)
	if retreat.Action != RetreatFromTarget || retreat.ThrottleZ != -1 || !retreat.Sprint || !retreat.Jump {
		t.Fatalf("unexpected retreat intent: %#v", retreat)
	}
	evade := MovementFor(obs, Decision{State: Evading}, true)
	if evade.Action != EvadeTarget || evade.ThrottleX != 1 || evade.ThrottleZ != 0 || !evade.Sprint {
		t.Fatalf("unexpected evade intent: %#v", evade)
	}
}

func TestMovementForAlternatesStrafeDirection(t *testing.T) {
	obs := Observation{HasTarget: true, TargetDistance: 3, TargetDirectionX: 1}
	decision := Decision{State: Engaging, Weapon: MeleeWeapon}
	left := MovementFor(obs, decision, false)
	right := MovementFor(obs, decision, true)
	if left.Action != StrafeTarget || right.Action != StrafeTarget || left.ThrottleZ == right.ThrottleZ {
		t.Fatalf("strafe direction should vary: left=%#v right=%#v", left, right)
	}
}
