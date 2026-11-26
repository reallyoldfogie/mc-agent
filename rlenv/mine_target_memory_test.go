package rlenv_test

import (
	"context"
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// mineTargetMemoryTestConfig poses a mine task with UseMineTargetMemory
// enabled.
func mineTargetMemoryTestConfig() rlenv.Config {
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:iron_ore"
	cfg.UseMineTargetMemory = true
	return cfg
}

func TestMineTargetMemory_FallsBackWhenLiveLOSFails(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true) // never resolvable via live FindVisibleBlock
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: 3, Y: 4, Z: 5}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), mineTargetMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestRememberedBlockCalls) != 1 {
		t.Fatalf("NearestRememberedBlock calls = %d, want 1", len(agent.nearestRememberedBlockCalls))
	}
	if agent.nearestRememberedBlockCalls[0] != "minecraft:iron_ore" {
		t.Errorf("NearestRememberedBlock called with %q, want minecraft:iron_ore", agent.nearestRememberedBlockCalls[0])
	}

	// mineVisible (index 12) and mineDx/Dy/Dz (9-11) are the existing
	// fields this fallback reuses - no new observation dims.
	if visible := obs.Values[12]; visible != 1 {
		t.Fatalf("mineVisible = %v, want 1 (filled in from memory)", visible)
	}
	if dx := obs.Values[9]; dx != 3 {
		t.Errorf("mineDx = %v, want 3", dx)
	}
	if dy := obs.Values[10]; dy != 4 {
		t.Errorf("mineDy = %v, want 4", dy)
	}
	if dz := obs.Values[11]; dz != 5 {
		t.Errorf("mineDz = %v, want 5", dz)
	}
}

func TestMineTargetMemory_LiveSightingTakesPrecedence(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlock("minecraft:iron_ore", 1, 1, 1) // actually visible live
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: 99, Y: 99, Z: 99}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), mineTargetMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Still called exactly once, unconditionally - mineRememberedSighting
	// (environment.go) backs mineRememberedCandidateExists, which
	// actionLegal's ActionMineRemembered case now needs regardless of
	// live LOS - but the routing below must still prefer the live
	// sighting over it.
	if len(agent.nearestRememberedBlockCalls) != 1 {
		t.Fatalf("NearestRememberedBlock calls = %d, want 1", len(agent.nearestRememberedBlockCalls))
	}
	if dx := obs.Values[9]; dx != 1 {
		t.Errorf("mineDx = %v, want 1 (the live sighting, not the memory one)", dx)
	}
}

func TestMineTargetMemory_IgnoredWhenDisabled(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: 3, Y: 4, Z: 5}, found: true}

	cfg := mineTargetMemoryTestConfig()
	cfg.UseMineTargetMemory = false

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Still called exactly once, unconditionally - mineRememberedSighting
	// backs mineRememberedCandidateExists (actionLegal's own gate for
	// ActionMineRemembered, independent of UseMineTargetMemory - see that
	// field's own doc comment), even though UseMineTargetMemory itself
	// stays correctly honored below (mineVisible/the dig-there routing
	// both stay off).
	if len(agent.nearestRememberedBlockCalls) != 1 {
		t.Fatalf("NearestRememberedBlock calls = %d, want 1", len(agent.nearestRememberedBlockCalls))
	}
	if visible := obs.Values[12]; visible != 0 {
		t.Fatalf("mineVisible = %v, want 0", visible)
	}
	if legal := env.ActionMask()[rlenv.ActionMineRemembered]; !legal {
		t.Fatalf("ActionMineRemembered legal = false, want true (a sighting exists, even though UseMineTargetMemory is off - that flag never gated this action)")
	}
}

func TestMineTargetMemory_OutsideSearchRadiusIsRejected(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: 1000, Y: 0, Z: 0}, found: true}

	cfg := mineTargetMemoryTestConfig()
	cfg.MineSearchRadius = 8

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if visible := obs.Values[12]; visible != 0 {
		t.Fatalf("mineVisible = %v, want 0 (remembered position is far outside MineSearchRadius)", visible)
	}
}

// buriedMineTargetMemoryTestConfig poses a buried mine task (the
// reachability problem ApproachOptions.SkipUnreachableFallback/this file's
// own buried-specific tests address) with UseMineTargetMemory enabled.
// MineBuriedExploreRadius 20 (half=10) keeps blind-exploration's own
// quadrant points far (~10 blocks horizontally) from the near memory
// positions these tests use, so a test can tell "routed toward the
// remembered position" apart from "fell back to blind exploration" just by
// how far the resulting MoveTo target is from origin.
func buriedMineTargetMemoryTestConfig() rlenv.Config {
	cfg := mineTargetMemoryTestConfig()
	cfg.MineTargetBuried = true
	cfg.MineSearchRadius = 16
	cfg.MineBuriedExploreRadius = 20
	cfg.ArrivalThreshold = 1.0
	cfg.StepTimeout = memoryResetTestStepTimeout
	return cfg
}

// TestMineTargetMemory_BuriedTaskRoutesThroughGoToTargetNotMineVisible is
// Phase 1 of the mine-target-memory/buried-reachability fix: a remembered
// position must steer ActionGoToTarget's dig-there navigation
// (mineKnownX/Y/Z/mineEverSeen), not get treated as directly mineable
// (mineVisible/ActionMine) the way TestMineTargetMemory_FallsBackWhenLiveLOSFails
// already confirms it still does for a non-buried task.
func TestMineTargetMemory_BuriedTaskRoutesThroughGoToTargetNotMineVisible(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true) // never resolvable via live FindVisibleBlock
	// Well within blindExploreStepDistance (4.0) of the origin anchor, so
	// dispatchGoToTarget's own clamp is a no-op here - if ActionGoToTarget's
	// MoveTo call lands close to this exact point, that's mineKnownX/Y/Z
	// routing; the blind-exploration quadrant point (half=10) would never
	// land this close to origin.
	const memX, memY, memZ = 2.0, 0.0, 2.0
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: memX, Y: memY, Z: memZ}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), buriedMineTargetMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Not obs.Values[12]: for a buried-style task, observationMineTarget
	// already substitutes mineKnownX/Y/Z/mineEverSeen into that same slot
	// (by design, see its own doc comment) regardless of how mineEverSeen
	// became true, so it reads 1 here correctly - mineEverSeen SHOULD be
	// true, that's the point. What must stay false is the raw
	// e.mineVisible field, checked here via the one thing that actually
	// reads it: ActionMine's own legality.
	if env.ActionMask()[rlenv.ActionMine] {
		t.Fatal("ActionMine must stay illegal: a remembered, not-yet-dug-to position must not count as mineVisible")
	}

	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step(ActionGoToTarget): %v", err)
	}
	if inner.moveToWithChatCalls == 0 {
		t.Fatal("expected ActionGoToTarget to dispatch a MoveTo")
	}
	if dist := distanceToForTest(inner.x, inner.y, inner.z, memX, memY, memZ); dist > 0.01 {
		t.Fatalf("bot ended up at (%.2f,%.2f,%.2f), %.2f blocks from the remembered position (%.2f,%.2f,%.2f) - want it steered there, not toward the blind-exploration quadrant point", inner.x, inner.y, inner.z, dist, memX, memY, memZ)
	}
}

// TestMineTargetMemory_BuriedTaskGivesUpAfterArrivingWithNothingFound is the
// safeguard required alongside Phase 1: once the bot arrives at a
// memory-sourced known target and it never turned visible (simulated here
// by teleporting the fake agent there directly, since mineBlockHidden stays
// true throughout), rejectMineKnownMemory must give up on it rather than
// leaving the bot permanently parked - a subsequent ActionGoToTarget
// dispatch must head toward blind exploration instead, not immediately
// re-lock onto the exact same remembered position (still offered by
// NearestRememberedBlock, which has no notion of "already tried").
func TestMineTargetMemory_BuriedTaskGivesUpAfterArrivingWithNothingFound(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	const memX, memY, memZ = 2.0, 0.0, 2.0
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: memX, Y: memY, Z: memZ}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), buriedMineTargetMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	callsAfterReset := len(agent.nearestRememberedBlockCalls)
	if callsAfterReset != 1 {
		t.Fatalf("NearestRememberedBlock calls after Reset = %d, want 1", callsAfterReset)
	}

	// Simulate arrival: teleport within ArrivalThreshold of the remembered
	// position without ever actually resolving it live (mineBlockHidden
	// stays true) - the real failure mode this guards: a stale or
	// deliberately-wrong memory (mc-rsi-trainer's own
	// MemorySeedInaccurateProb) that leads nowhere.
	inner.UpdatePosition(models.V3{X: memX + 0.5, Y: memY, Z: memZ}, 0, 0)
	if _, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step(ActionWait): %v", err)
	}
	if got := len(agent.nearestRememberedBlockCalls); got <= callsAfterReset {
		t.Fatalf("NearestRememberedBlock calls after arrival = %d, want more than %d (must look for a fresh target after giving up)", got, callsAfterReset)
	}

	// The exact same position is still the only one NearestRememberedBlock
	// offers (fakeMineTargetMemoryAgent is static) - a correct give-up
	// must not re-lock onto it. A subsequent ActionGoToTarget dispatch
	// should therefore move the bot away from this dead end, toward blind
	// exploration instead.
	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step(ActionGoToTarget): %v", err)
	}
	if dist := distanceToForTest(inner.x, inner.y, inner.z, memX+0.5, memY, memZ); dist < 0.5 {
		t.Fatalf("bot stayed at the rejected memory position (%.2f,%.2f,%.2f), moved only %.2f blocks - want it to resume blind exploration instead of re-locking onto a position that already proved empty", inner.x, inner.y, inner.z, dist)
	}
}

// TestMineTargetMemory_BuriedTaskUsesBuriedExploreRadiusNotMineSearchRadius
// is Phase 2 of the fix: rememberedMineTarget must bound a buried-style
// lookup by buriedExploreRadius() (the footprint memory-reset.go's own
// clear/seed feature actually populates), not MineSearchRadius - found
// live, 2026-11-25, this run's own curriculum sets MineSearchRadius=16 but
// MineBuriedExploreRadius=80 (half=40), so bounding by the former would
// silently exclude most of what gets seeded.
func TestMineTargetMemory_BuriedTaskUsesBuriedExploreRadiusNotMineSearchRadius(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	// Outside MineSearchRadius (8 below) but inside buriedExploreRadius()
	// (half of MineBuriedExploreRadius 80 = 40).
	const memX, memY, memZ = 20.0, 0.0, 0.0
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: memX, Y: memY, Z: memZ}, found: true}

	cfg := buriedMineTargetMemoryTestConfig()
	cfg.MineSearchRadius = 8
	cfg.MineBuriedExploreRadius = 80

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step(ActionGoToTarget): %v", err)
	}
	// dispatchGoToTarget clamps to blindExploreStepDistance (4.0) during a
	// buried task, so the bot won't have arrived at (20,0,0) yet after one
	// step either way - what distinguishes "routed toward the remembered
	// position" from "fell back to the blind-exploration quadrant point"
	// is direction, not distance: every quadrant point sits at
	// (+-buriedExploreRadius, anchorY-buriedExploreDepth, +-buriedExploreRadius)
	// - a diagonal with a meaningfully negative Y and a large |Z| - while
	// the remembered position here has Y=Z=0. A step toward it keeps Z
	// near 0; a step toward any quadrant moves Z by roughly the same
	// magnitude as X (both legs of the diagonal are the same length here).
	if z := math.Abs(inner.z); z > 1 {
		t.Fatalf("bot moved to z=%.2f - looks like it fell back to a blind-exploration quadrant point instead of the remembered position (20,0,0), meaning the radius bound rejected it", inner.z)
	}
	if inner.x <= 1 {
		t.Fatalf("bot barely moved in +X (x=%.2f) - expected real movement toward the remembered position (20,0,0)", inner.x)
	}
}

func distanceToForTest(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx, dy, dz := x1-x2, y1-y2, z1-z2
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func TestMineTargetMemory_NoOpWithoutCapability(t *testing.T) {
	agent := newFakeAgent(0, 0, 0) // plain fakeAgent: no NearestRememberedBlock
	agent.setMineBlockHidden(true)

	env, err := rlenv.New(agent, actions.NewRegistry(), mineTargetMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if visible := obs.Values[12]; visible != 0 {
		t.Fatalf("mineVisible = %v, want 0 (agent doesn't implement MineTargetMemoryAgent)", visible)
	}
}
