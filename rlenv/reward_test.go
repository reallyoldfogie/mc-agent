package rlenv

import (
	"testing"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
)

func TestComputeRewardProgressTowardTarget(t *testing.T) {
	reward, died := computeReward(stepOutcome{
		prevDistance: 10,
		newDistance:  8,
	})
	if died {
		t.Fatalf("died = true, want false")
	}
	want := distanceRewardScale*2 - timePenalty
	if reward != want {
		t.Fatalf("reward = %v, want %v", reward, want)
	}
}

func TestComputeRewardMovingAwayIsNegative(t *testing.T) {
	reward, _ := computeReward(stepOutcome{
		prevDistance: 5,
		newDistance:  8,
	})
	if reward >= 0 {
		t.Fatalf("reward = %v, want negative (moved away from target)", reward)
	}
}

func TestComputeRewardAppliesDamagePenalty(t *testing.T) {
	reward, died := computeReward(stepOutcome{
		prevDistance:      10,
		newDistance:       10,
		prevHealth:        20,
		newHealth:         14,
		healthKnownBefore: true,
		healthKnownAfter:  true,
	})
	if died {
		t.Fatalf("died = true, want false (health > 0)")
	}
	want := -timePenalty - damagePenaltyScale*6
	if reward != want {
		t.Fatalf("reward = %v, want %v", reward, want)
	}
}

func TestComputeRewardIgnoresHealthDeltaWhenUnknown(t *testing.T) {
	// Health swings wildly here, but healthKnownBefore is false (e.g. the
	// step right after Reset, before any HealthChange event has ever been
	// received) — this must not be misread as a huge, spurious damage hit.
	reward, died := computeReward(stepOutcome{
		prevDistance:      10,
		newDistance:       10,
		prevHealth:        0,
		newHealth:         20,
		healthKnownBefore: false,
		healthKnownAfter:  true,
	})
	if died {
		t.Fatalf("died = true, want false")
	}
	want := -timePenalty
	if reward != want {
		t.Fatalf("reward = %v, want %v (health delta should be ignored)", reward, want)
	}
}

func TestComputeRewardDeathAppliesPenaltyAndEndsEpisode(t *testing.T) {
	reward, died := computeReward(stepOutcome{
		prevDistance:      10,
		newDistance:       10,
		prevHealth:        4,
		newHealth:         0,
		healthKnownBefore: true,
		healthKnownAfter:  true,
	})
	if !died {
		t.Fatalf("died = false, want true (health reached 0)")
	}
	want := -timePenalty - damagePenaltyScale*4 + deathPenalty
	if reward != want {
		t.Fatalf("reward = %v, want %v", reward, want)
	}
}

func TestMovementTargetMapsActionsToTheRightCoordinates(t *testing.T) {
	e := &Environment{
		originX: 1, originY: 2, originZ: 3,
		targetX: 10, targetY: 20, targetZ: 30,
	}

	if x, y, z, isMovement, err := e.movementTarget(ActionWait); err != nil || isMovement {
		t.Fatalf("ActionWait: got (%v,%v,%v,%v,%v), want isMovement=false, err=nil", x, y, z, isMovement, err)
	}
	if x, y, z, isMovement, err := e.movementTarget(ActionGoToTarget); err != nil || !isMovement || x != 10 || y != 20 || z != 30 {
		t.Fatalf("ActionGoToTarget: got (%v,%v,%v,%v,%v), want (10,20,30,true,nil)", x, y, z, isMovement, err)
	}
	if x, y, z, isMovement, err := e.movementTarget(ActionReturnHome); err != nil || !isMovement || x != 1 || y != 2 || z != 3 {
		t.Fatalf("ActionReturnHome: got (%v,%v,%v,%v,%v), want (1,2,3,true,nil)", x, y, z, isMovement, err)
	}
	if _, _, _, _, err := e.movementTarget(rl.Action(99)); err == nil {
		t.Fatalf("out-of-range action: want error, got nil")
	}
}
