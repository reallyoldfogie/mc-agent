package combat

import (
	"testing"
	"time"
)

func TestControllerCooldownOnlyCommitsSuccessfulAttacks(t *testing.T) {
	c := NewController()
	obs := Observation{Health: 20, MaxHealth: 20, HasTarget: true, TargetVisible: true, TargetDistance: 3, TargetDirectionX: 1}
	t0 := time.Unix(100, 0)
	decision, _ := c.Next(t0, obs)
	if !decision.Attack {
		t.Fatal("first attack should be ready")
	}
	// No commit means a failed transport action does not consume cooldown.
	decision, _ = c.Next(t0.Add(1*time.Millisecond), obs)
	if !decision.Attack {
		t.Fatal("uncommitted attack should remain ready")
	}
	c.CommitAttack(t0.Add(1 * time.Millisecond))
	decision, _ = c.Next(t0.Add(400*time.Millisecond), obs)
	if decision.Attack {
		t.Fatal("committed melee attack should be on cooldown")
	}
	decision, _ = c.Next(t0.Add(501*time.Millisecond), obs)
	if !decision.Attack {
		t.Fatal("melee attack should be ready after cooldown")
	}
}

func TestControllerAlternatesLateralMovement(t *testing.T) {
	c := NewController()
	obs := Observation{Health: 20, MaxHealth: 20, HasTarget: true, TargetVisible: true, TargetDistance: 3, TargetDirectionX: 1}
	first, _ := c.Next(time.Unix(100, 0), obs)
	second, _ := c.Next(time.Unix(100, 0).Add(1*time.Second), obs)
	if first.Attack == false || second.Attack == false {
		t.Fatal("both attacks should be ready before any commit")
	}
	firstIntent := MovementFor(obs, Decide(obs), false)
	secondIntent := MovementFor(obs, Decide(obs), true)
	if firstIntent.ThrottleZ == secondIntent.ThrottleZ {
		t.Fatal("expected alternating strafe directions")
	}
}
