package combat

import "testing"

func TestFilterMovementStopsAtFallRisk(t *testing.T) {
	intent := MovementIntent{Action: ApproachTarget, ThrottleX: 1, Sprint: true, Jump: true}
	got := FilterMovement(intent, MovementEnvironment{GroundStable: true, FallRisk: true})
	if got.Action != HoldPosition || got.ThrottleX != 0 || got.Sprint || got.Jump {
		t.Fatalf("fall risk should stop movement: %#v", got)
	}
}

func TestFilterMovementMakesWaterExitConservative(t *testing.T) {
	intent := MovementIntent{Action: RetreatFromTarget, ThrottleZ: -1, Sprint: true}
	got := FilterMovement(intent, MovementEnvironment{GroundStable: true, InWater: true, CanJump: true})
	if got.Sprint || !got.Jump || got.ThrottleZ != -1 {
		t.Fatalf("water movement should preserve direction, stop sprinting, and jump: %#v", got)
	}
}

func TestFilterMovementEvadesExposedHazard(t *testing.T) {
	intent := MovementIntent{Action: ApproachTarget, ThrottleX: 1}
	got := FilterMovement(intent, MovementEnvironment{GroundStable: true, CanJump: false, TargetIsHazard: true})
	if got.Action != EvadeTarget || !got.Sprint || got.Jump {
		t.Fatalf("exposed hazard should trigger evasive movement: %#v", got)
	}
}
