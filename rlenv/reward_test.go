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

// TestComputeRewardHonorsAConfiguredTimePenalty: the default 0.01 is too small
// to make wasted steps costly next to a +10 success bonus, so it is
// configurable; 0 keeps the default.
func TestComputeRewardHonorsAConfiguredTimePenalty(t *testing.T) {
	reward, _ := computeReward(stepOutcome{prevDistance: 10, newDistance: 10, timePenalty: 0.25})
	if reward != -0.25 {
		t.Fatalf("reward = %v, want -0.25 (the configured penalty, no other terms)", reward)
	}
	reward, _ = computeReward(stepOutcome{prevDistance: 10, newDistance: 10})
	if reward != -timePenalty {
		t.Fatalf("reward = %v, want the default %v when unset", reward, timePenalty)
	}
}

// TestComputeRewardDeathIgnoresRespawnTeleportDistance: dying respawns the
// bot at world spawn, so the "distance change" on the death step is a
// teleport artifact (here 5000 blocks), not progress, and must not swamp
// the death penalty.
func TestComputeRewardDeathIgnoresRespawnTeleportDistance(t *testing.T) {
	reward, died := computeReward(stepOutcome{
		prevDistance:      3,
		newDistance:       5000,
		prevHealth:        1,
		newHealth:         0,
		healthKnownBefore: true,
		healthKnownAfter:  true,
	})
	if !died {
		t.Fatalf("died = false, want true")
	}
	want := -timePenalty - damagePenaltyScale*1 + deathPenalty
	if reward != want {
		t.Fatalf("reward = %v, want %v (distance change on the death step must be ignored)", reward, want)
	}
}

func TestResolveDispatchMapsActionsToTheRightTargetsAndArgs(t *testing.T) {
	e := &Environment{
		targetX: 10, targetY: 20, targetZ: 30,
	}

	if dispatch, ok, err := e.resolveDispatch(ActionWait); err != nil || ok {
		t.Fatalf("ActionWait: got (%v,%v,%v), want ok=false, err=nil", dispatch, ok, err)
	}
	if dispatch, ok, err := e.resolveDispatch(ActionGoToTarget); err != nil || !ok || dispatch.name != moveToActionName ||
		dispatch.args[0] != formatCoord(10) || dispatch.args[1] != formatCoord(20) || dispatch.args[2] != formatCoord(30) {
		t.Fatalf("ActionGoToTarget: got (%+v,%v,%v), want moveto(10,20,30)", dispatch, ok, err)
	}
	if dispatch, ok, err := e.resolveDispatch(ActionMine); err != nil || ok {
		t.Fatalf("ActionMine with no Config.MineTargetBlock: got (%+v,%v,%v), want ok=false, err=nil (safe no-op)", dispatch, ok, err)
	}
	e.cfg.MineTargetBlock = "minecraft:stone"
	if dispatch, ok, err := e.resolveDispatch(ActionMine); err != nil || ok {
		t.Fatalf("ActionMine with Config.MineTargetBlock set but nothing visible: got (%+v,%v,%v), want ok=false, err=nil (safe no-op)", dispatch, ok, err)
	}
	e.mineVisible = true
	e.mineX, e.mineY, e.mineZ = 40, 50, 60
	if dispatch, ok, err := e.resolveDispatch(ActionMine); err != nil || !ok || dispatch.name != mineActionName ||
		dispatch.args[0] != formatCoord(40) || dispatch.args[1] != formatCoord(50) || dispatch.args[2] != formatCoord(60) {
		t.Fatalf("ActionMine with a visible target: got (%+v,%v,%v), want mine(40,50,60) — Environment's own already-resolved coordinates, not the block name", dispatch, ok, err)
	}
	if _, _, err := e.resolveDispatch(rl.Action(99)); err == nil {
		t.Fatalf("out-of-range action: want error, got nil")
	}
}
