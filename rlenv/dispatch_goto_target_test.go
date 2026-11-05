package rlenv_test

import (
	"context"
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// distanceFromOrigin is a small helper for the tests below - none of them
// care about direction, only how far a single ActionGoToTarget dispatch
// actually moved the (teleport-on-MoveTo) fake agent.
func distanceFromOrigin(x, y, z float64) float64 {
	return math.Sqrt(x*x + y*y + z*z)
}

// TestBlindExplorationGotoClampedToShortHop confirms the fix for the live
// gap found 2026-11-04 (docs/plans/21-buried-mining-success.md): while
// buried-search exploration is still blind (ore never yet seen), a single
// ActionGoToTarget dispatch must travel only a short hop (mirrors
// rlenv's own private blindExploreStepDistance, 4 blocks) toward the
// quadrant point, not the full remaining distance - so the once-per-Step
// refreshMineTarget call gets far more chances to notice something this
// dig exposed before the bot has wandered well past it. buriedTestConfig
// (MineSearchRadius 10, explore depth defaulting to half=5) puts the
// nearest quadrant point roughly sqrt(5^2+5^2+5^2) ≈ 8.7 blocks from the
// origin - comfortably farther than the 4-block clamp, so an unclamped
// dispatch would land far past where this test expects it to stop.
func TestBlindExplorationGotoClampedToShortHop(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := buriedTestConfig()
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(ctx, rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}

	got := distanceFromOrigin(agent.x, agent.y, agent.z)
	const wantMax = 4.5 // blindExploreStepDistance (4) plus float slack
	if got > wantMax {
		t.Fatalf("distance traveled = %.2f, want <= %.2f (clamped to a short hop) - landed at (%.2f,%.2f,%.2f)", got, wantMax, agent.x, agent.y, agent.z)
	}
	const wantMin = 3.5 // should use the full clamp budget, not stop short
	if got < wantMin {
		t.Fatalf("distance traveled = %.2f, want >= %.2f - the dispatch should use its full short-hop budget when the quadrant point is much farther away", got, wantMin)
	}
}

// TestOrdinaryGotoNotClamped confirms clamping is scoped to blind buried
// exploration only: an ordinary (non-buried) goto task's single fixed
// TargetOffset point must still be dispatched in full, exactly as before
// this change. testConfig's own TargetOffset is {5,0,0} - farther than
// the 4-block clamp, so a wrongly-applied clamp would be caught here.
func TestOrdinaryGotoNotClamped(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(ctx, rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}

	got := distanceFromOrigin(agent.x, agent.y, agent.z)
	const wantMin = 4.9 // should reach (close to) the full TargetOffset distance (5), not stop at the 4-block clamp
	if got < wantMin {
		t.Fatalf("distance traveled = %.2f, want >= %.2f - an ordinary goto task must not be clamped, landed at (%.2f,%.2f,%.2f)", got, wantMin, agent.x, agent.y, agent.z)
	}
}

// TestKnownBuriedTargetGotoNotClamped confirms clamping only applies
// while exploration is still blind: once the target has been seen even
// once (mineEverSeen), currentGoToTarget already switches to the exact
// known position, and dispatching there in full (not clamped to a short
// hop) is correct - there's nothing left to "notice mid-path" once the
// target is already known.
func TestKnownBuriedTargetGotoNotClamped(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:coal_ore", 8, 0, 0) // 8 blocks away, farther than the clamp
	cfg := buriedTestConfig()
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// First step resolves the target via refreshMineTarget (mineEverSeen
	// becomes true at the END of this step) but dispatches toward the
	// still-blind exploration point, since mineEverSeen was false when
	// THIS step's dispatch was computed.
	if _, err := env.Step(ctx, rlenv.ActionWait); err != nil {
		t.Fatalf("Step 1 (ActionWait, resolves the target): %v", err)
	}

	// Second step: mineEverSeen is now true, so this dispatch should go
	// straight to the known position in full, unclamped.
	if _, err := env.Step(ctx, rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step 2 (ActionGoToTarget): %v", err)
	}

	got := distanceFromOrigin(agent.x, agent.y, agent.z)
	const wantMin = 7.9 // should reach (close to) the known target's distance (8), not stop at the 4-block clamp
	if got < wantMin {
		t.Fatalf("distance traveled = %.2f, want >= %.2f - a known buried target must not be clamped, landed at (%.2f,%.2f,%.2f)", got, wantMin, agent.x, agent.y, agent.z)
	}
}
