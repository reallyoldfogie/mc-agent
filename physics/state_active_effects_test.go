package physics

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise SetActiveEffects/Tick() end-to-end (Phase 4a: Slow
// Falling, Levitation) — effects.go's TestEffectiveGravity/
// TestLevitationVerticalVelocity already cover the pure formulas in
// isolation; these confirm state.go actually wires them into a real tick.

func TestState_SlowFallingSlowsDescent(t *testing.T) {
	world, shapes := createFlatWorld()

	normal := NewState(shapes)
	normal.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	normal.SetVelocity(models.V3{})

	slowed := NewState(shapes)
	slowed.SetPositionSimple(models.V3{X: 10, Y: 50, Z: 0})
	slowed.SetVelocity(models.V3{})
	slowed.SetActiveEffects(models.ActiveEffects{HasSlowFalling: true})

	const ticks = 40
	for range ticks {
		require.NoError(t, normal.Tick(Inputs{}, world))
		require.NoError(t, slowed.Tick(Inputs{}, world))
	}

	normalDrop := 50 - normal.Position().Y
	slowedDrop := 50 - slowed.Position().Y

	t.Logf("after %d ticks: normal drop=%.2f, slow-falling drop=%.2f", ticks, normalDrop, slowedDrop)
	assert.Greater(t, normalDrop, slowedDrop*5, "slow falling should fall dramatically slower than normal gravity")
	assert.Less(t, slowed.Velocity().Y, 0.0, "should still be descending, just slowly")
	// Drag (0.98) still compounds a small per-tick gravity increment into a
	// real terminal velocity given enough ticks (steady-state ~ -0.49
	// blocks/tick for the 0.01 cap, vs. normal gravity's much larger ~-3.92)
	// — the cap bounds the per-tick increment, not the eventual terminal
	// speed, so the meaningful comparison is against normal's velocity
	// magnitude at the same tick count, not a small fixed magic number.
	assert.Less(t, math.Abs(slowed.Velocity().Y), math.Abs(normal.Velocity().Y)*0.2,
		"slow falling's descent speed should still be far below normal gravity's at the same tick count")
}

func TestState_SlowFallingWhileRisingIsNotCapped(t *testing.T) {
	// Java's getEffectiveGravity() only caps gravity while velocityY <= 0 —
	// a jump's initial rise should decelerate at the normal rate even with
	// Slow Falling active. See EffectiveGravity's doc comment.
	world, shapes := createFlatWorld()

	normal := NewState(shapes)
	normal.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	normal.SetVelocity(models.V3{Y: JumpVelocity})

	slowFalling := NewState(shapes)
	slowFalling.SetPositionSimple(models.V3{X: 10, Y: 50, Z: 0})
	slowFalling.SetVelocity(models.V3{Y: JumpVelocity})
	slowFalling.SetActiveEffects(models.ActiveEffects{HasSlowFalling: true})

	require.NoError(t, normal.Tick(Inputs{}, world))
	require.NoError(t, slowFalling.Tick(Inputs{}, world))

	assert.InDelta(t, normal.Velocity().Y, slowFalling.Velocity().Y, 1e-9,
		"the first tick of a rise should be identical with or without slow falling")
}

func TestState_LevitationLiftsPlayer(t *testing.T) {
	world, shapes := createFlatWorld()

	state := NewState(shapes)
	state.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	state.SetVelocity(models.V3{})
	state.SetActiveEffects(models.ActiveEffects{HasLevitation: true, LevitationAmplifier: 0})

	const ticks = 60
	for range ticks {
		require.NoError(t, state.Tick(Inputs{}, world))
	}

	t.Logf("after %d ticks: Y=%.3f vel.Y=%.4f", ticks, state.Position().Y, state.Velocity().Y)
	assert.Greater(t, state.Position().Y, 50.0, "levitation should lift the player upward, not let it fall")
	assert.Greater(t, state.Velocity().Y, 0.0, "should be rising")
}

func TestState_LevitationHigherAmplifierLiftsFaster(t *testing.T) {
	world, shapes := createFlatWorld()

	level1 := NewState(shapes)
	level1.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	level1.SetActiveEffects(models.ActiveEffects{HasLevitation: true, LevitationAmplifier: 0})

	level3 := NewState(shapes)
	level3.SetPositionSimple(models.V3{X: 10, Y: 50, Z: 0})
	level3.SetActiveEffects(models.ActiveEffects{HasLevitation: true, LevitationAmplifier: 2})

	const ticks = 30
	for range ticks {
		require.NoError(t, level1.Tick(Inputs{}, world))
		require.NoError(t, level3.Tick(Inputs{}, world))
	}

	assert.Greater(t, level3.Position().Y, level1.Position().Y, "a higher levitation level should lift the player higher in the same time")
}

func TestState_JumpBoostRaisesJumpVelocity(t *testing.T) {
	world, shapes := createFlatWorld()

	settle := func(s models.PhysicsState) {
		s.SetPositionSimple(models.V3{X: 0, Y: 1, Z: 0})
		s.SetVelocity(models.V3{})
		for range 20 {
			require.NoError(t, s.Tick(Inputs{}, world))
			if s.OnGround() && math.Abs(s.Velocity().Y) < 0.01 {
				break
			}
		}
		require.True(t, s.OnGround(), "player should settle on ground before jumping")
	}

	normal := NewState(shapes)
	settle(normal)
	require.NoError(t, normal.Tick(Inputs{Jump: true}, world))

	boosted := NewState(shapes)
	boosted.SetActiveEffects(models.ActiveEffects{HasJumpBoost: true, JumpBoostAmplifier: 0})
	settle(boosted)
	require.NoError(t, boosted.Tick(Inputs{Jump: true}, world))

	t.Logf("normal jump velY=%.4f, jump-boosted velY=%.4f", normal.Velocity().Y, boosted.Velocity().Y)
	assert.Greater(t, boosted.Velocity().Y, normal.Velocity().Y, "jump boost should raise jump velocity above normal")
	// The jump-input tick also applies gravity/drag to the freshly-set
	// velocity before the tick ends (Vel.Y = (JumpVelocity - Gravity) * Drag,
	// not the raw JumpVelocity constant), so the bonus itself is scaled by
	// Drag too — the meaningful check is the delta between the two states,
	// not either one's absolute value against the raw constants.
	assert.InDelta(t, JumpBoostVelocityBonus(0)*Drag, boosted.Velocity().Y-normal.Velocity().Y, 1e-9,
		"jump boost's velocity delta should match the bonus formula scaled by the same tick's drag")
}

func TestState_SpeedAndSlownessScaleGroundAcceleration(t *testing.T) {
	world, shapes := createFlatWorld()

	settle := func(s models.PhysicsState, x float64) {
		s.SetPositionSimple(models.V3{X: x, Y: 1, Z: 0})
		s.SetVelocity(models.V3{})
		for range 20 {
			require.NoError(t, s.Tick(Inputs{}, world))
			if s.OnGround() && math.Abs(s.Velocity().Y) < 0.01 {
				break
			}
		}
		require.True(t, s.OnGround(), "player should settle on ground before moving")
	}

	normal := NewState(shapes)
	settle(normal, 0)

	speedy := NewState(shapes)
	speedy.SetActiveEffects(models.ActiveEffects{HasSpeed: true, SpeedAmplifier: 1}) // Speed II
	settle(speedy, 10)

	slow := NewState(shapes)
	slow.SetActiveEffects(models.ActiveEffects{HasSlowness: true, SlownessAmplifier: 1}) // Slowness II
	settle(slow, 20)

	forward := Inputs{ThrottleZ: 1.0}
	const ticks = 20
	for range ticks {
		require.NoError(t, normal.Tick(forward, world))
		require.NoError(t, speedy.Tick(forward, world))
		require.NoError(t, slow.Tick(forward, world))
	}

	normalDist := math.Abs(normal.Position().Z)
	speedyDist := math.Abs(speedy.Position().Z)
	slowDist := math.Abs(slow.Position().Z)
	t.Logf("after %d ticks: normal=%.4f, speed II=%.4f, slowness II=%.4f", ticks, normalDist, speedyDist, slowDist)

	assert.Greater(t, speedyDist, normalDist, "Speed II should move the player further than normal in the same number of ticks")
	assert.Less(t, slowDist, normalDist, "Slowness II should move the player less far than normal in the same number of ticks")
}

func TestState_HighSlownessClampsMovementToZero(t *testing.T) {
	world, shapes := createFlatWorld()

	frozen := NewState(shapes)
	frozen.SetPositionSimple(models.V3{X: 0, Y: 1, Z: 0})
	frozen.SetVelocity(models.V3{})
	// Amplifier 10 (Slowness XI): 1 + (-0.15)*11 = -0.65, clamped to 0.
	frozen.SetActiveEffects(models.ActiveEffects{HasSlowness: true, SlownessAmplifier: 10})
	for range 5 {
		require.NoError(t, frozen.Tick(Inputs{}, world))
	}

	forward := Inputs{ThrottleZ: 1.0}
	for range 20 {
		require.NoError(t, frozen.Tick(forward, world))
	}

	assert.InDelta(t, 0.0, frozen.Position().Z, 1e-9, "sufficiently high slowness should clamp movement_speed to zero, not reverse it")
}

func TestState_SlowFallingAndLevitationNegateFallDamageAccumulation(t *testing.T) {
	world, shapes := createFlatWorld()

	normal := NewState(shapes)
	normal.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})

	slowFalling := NewState(shapes)
	slowFalling.SetPositionSimple(models.V3{X: 10, Y: 50, Z: 0})
	slowFalling.SetActiveEffects(models.ActiveEffects{HasSlowFalling: true})

	levitating := NewState(shapes)
	levitating.SetPositionSimple(models.V3{X: 20, Y: 50, Z: 0})
	levitating.SetActiveEffects(models.ActiveEffects{HasLevitation: true})

	const ticks = 30
	for range ticks {
		require.NoError(t, normal.Tick(Inputs{}, world))
		require.NoError(t, slowFalling.Tick(Inputs{}, world))
		require.NoError(t, levitating.Tick(Inputs{}, world))
	}

	t.Logf("fall distance after %d ticks: normal=%.3f slowFalling=%.3f levitating=%.3f",
		ticks, normal.FallDistance(), slowFalling.FallDistance(), levitating.FallDistance())

	assert.Greater(t, normal.FallDistance(), 1.0, "ordinary freefall should accumulate real fall distance")
	// Slow falling resets fallDistance to 0 at the start of every tick, but
	// that same tick's own (slow) descent re-accumulates onto it before the
	// tick ends — so the observable floor is roughly one tick's worth of
	// descent at the capped gravity (~0.2), not literally 0. The real
	// signal is that it never grows tick-over-tick the way normal's does.
	assert.Less(t, slowFalling.FallDistance(), 1.0, "slow falling should keep fall distance pinned to about one tick's worth, never accumulating")
	assert.Less(t, levitating.FallDistance(), 0.1, "levitation should keep fall distance pinned near zero every tick (it's rising, not falling, so nothing re-accumulates)")
}

func TestState_DolphinsGraceIncreasesHorizontalSwimSpeed(t *testing.T) {
	world, shapes := createWaterPool(10)

	normal := NewState(shapes)
	normal.SetPosition(models.V3{X: 0, Y: 5, Z: 0}, 0, 0, false)
	normal.SetVelocity(models.V3{})

	graced := NewState(shapes)
	graced.SetPosition(models.V3{X: 0, Y: 5, Z: 0}, 0, 0, false)
	graced.SetVelocity(models.V3{})
	graced.SetActiveEffects(models.ActiveEffects{HasDolphinsGrace: true})

	forward := Inputs{ThrottleX: 1.0}
	const ticks = 20
	for range ticks {
		require.NoError(t, normal.Tick(forward, world))
		require.NoError(t, graced.Tick(forward, world))
	}

	normalDist := math.Abs(normal.Position().X)
	gracedDist := math.Abs(graced.Position().X)
	t.Logf("after %d ticks swimming: normal=%.4f, dolphins grace=%.4f", ticks, normalDist, gracedDist)

	assert.Greater(t, gracedDist, normalDist, "Dolphin's Grace should let the player swim horizontally further than normal in the same number of ticks")
}

func TestState_CobwebSlowsMovementAndWeavingHalvesSeverity(t *testing.T) {
	world, shapes := createFlatWorld()
	shapes.SetPassable(BlockCobweb, true)
	shapes.SetCobweb(BlockCobweb, true)

	// Cover a patch of the floor's surface (Y=1, the block cell just above
	// the Y=0 stone floor) with cobwebs, wide enough that a correctly-slowed
	// player never leaves it within the test's tick budget.
	for x := -3; x <= 3; x++ {
		for z := -3; z <= 3; z++ {
			world.SetBlock(x, 1, z, BlockCobweb)
		}
	}

	settle := func(s models.PhysicsState, x float64) {
		s.SetPositionSimple(models.V3{X: x, Y: 1, Z: 0})
		s.SetVelocity(models.V3{})
		for range 20 {
			require.NoError(t, s.Tick(Inputs{}, world))
			if s.OnGround() && math.Abs(s.Velocity().Y) < 0.01 {
				break
			}
		}
		require.True(t, s.OnGround(), "player should settle on ground before moving")
	}

	normal := NewState(shapes)
	settle(normal, -15) // far outside the cobweb patch

	webbed := NewState(shapes)
	settle(webbed, 0) // inside the cobweb patch, no Weaving

	woven := NewState(shapes)
	woven.SetActiveEffects(models.ActiveEffects{HasWeaving: true})
	settle(woven, 0) // inside the same cobweb patch, with Weaving

	forward := Inputs{ThrottleX: 1.0}
	const ticks = 20
	for range ticks {
		require.NoError(t, normal.Tick(forward, world))
		require.NoError(t, webbed.Tick(forward, world))
		require.NoError(t, woven.Tick(forward, world))
	}

	normalDist := math.Abs(normal.Position().X - (-15))
	webbedDist := math.Abs(webbed.Position().X)
	wovenDist := math.Abs(woven.Position().X)
	t.Logf("after %d ticks: normal=%.4f, cobweb=%.4f, cobweb+weaving=%.4f", ticks, normalDist, webbedDist, wovenDist)

	assert.Greater(t, normalDist, webbedDist, "plain cobweb should slow horizontal movement well below normal")
	assert.Greater(t, wovenDist, webbedDist, "weaving should let the player move further than plain cobweb slowdown")
	assert.Less(t, wovenDist, normalDist, "weaving should still not restore full normal speed")
}

// TestState_PowderSnowSlowsMovementWithoutLeatherBoots verifies §5.1: a
// player without leather boots sinks into a powder snow layer (it has no
// static collision box — see getSurroundingBoxes) and is slowed by
// PowderSnowSlowdownMultiplier while their body overlaps it, the same
// Entity.slowMovement mechanism cobwebs use (see
// TestState_CobwebSlowsMovementAndWeavingHalvesSeverity above) but with
// powder snow's own per-axis multiplier.
func TestState_PowderSnowSlowsMovementWithoutLeatherBoots(t *testing.T) {
	world, shapes := createFlatWorld()
	shapes.SetPassable(BlockPowderSnow, true)
	shapes.SetPowderSnow(BlockPowderSnow, true)

	// Cover a patch of the floor's surface (Y=1, the block cell just above
	// the Y=0 stone floor) with powder snow, wide enough that a correctly
	// slowed player never leaves it within the test's tick budget.
	for x := -3; x <= 3; x++ {
		for z := -3; z <= 3; z++ {
			world.SetBlock(x, 1, z, BlockPowderSnow)
		}
	}

	settle := func(s models.PhysicsState, x float64) {
		s.SetPositionSimple(models.V3{X: x, Y: 1, Z: 0})
		s.SetVelocity(models.V3{})
		for range 20 {
			require.NoError(t, s.Tick(Inputs{}, world))
			if s.OnGround() && math.Abs(s.Velocity().Y) < 0.01 {
				break
			}
		}
		require.True(t, s.OnGround(), "player should settle on ground before moving")
	}

	normal := NewState(shapes)
	settle(normal, -15) // far outside the powder snow patch

	sunk := NewState(shapes)
	settle(sunk, 0) // inside the patch, no leather boots

	forward := Inputs{ThrottleX: 1.0}
	const ticks = 20
	for range ticks {
		require.NoError(t, normal.Tick(forward, world))
		require.NoError(t, sunk.Tick(forward, world))
	}

	normalDist := math.Abs(normal.Position().X - (-15))
	sunkDist := math.Abs(sunk.Position().X)
	t.Logf("after %d ticks: normal=%.4f, powder snow (no boots)=%.4f", ticks, normalDist, sunkDist)

	assert.Greater(t, normalDist, sunkDist, "powder snow should slow horizontal movement well below normal without leather boots")
	assert.Equal(t, 1.0, sunk.Position().Y, "player should rest on the real floor beneath the (collisionless) powder snow layer, not sink through it")
}

// TestState_LeatherBootsWalkOnPowderSnowAtNormalSpeed verifies §5.1's other
// half: Java PowderSnowBlock.canWalkOnPowderSnow — a player wearing leather
// boots stands on top of a powder snow layer instead of sinking into it
// (getSurroundingBoxes adds a synthetic solid box for it only when booted),
// and never triggers the slowMovement multiplier, since their body no
// longer overlaps the block itself (see isOverlappingPowderSnow's doc
// comment).
func TestState_LeatherBootsWalkOnPowderSnowAtNormalSpeed(t *testing.T) {
	world, shapes := createFlatWorld()
	shapes.SetPassable(BlockPowderSnow, true)
	shapes.SetPowderSnow(BlockPowderSnow, true)

	for x := -3; x <= 3; x++ {
		for z := -3; z <= 3; z++ {
			world.SetBlock(x, 1, z, BlockPowderSnow)
		}
	}

	settleFromAbove := func(s models.PhysicsState, x, startY float64) {
		s.SetPositionSimple(models.V3{X: x, Y: startY, Z: 0})
		s.SetVelocity(models.V3{})
		for range 40 {
			require.NoError(t, s.Tick(Inputs{}, world))
			if s.OnGround() && math.Abs(s.Velocity().Y) < 0.01 {
				break
			}
		}
		require.True(t, s.OnGround(), "player should settle on ground before moving")
	}

	normal := NewState(shapes)
	settleFromAbove(normal, -15, 1) // far outside the patch, plain stone floor

	booted := NewState(shapes)
	booted.SetLeatherBootsEquipped(true)
	settleFromAbove(booted, 0, 5) // falling onto the patch from above, wearing boots

	require.Equal(t, 2.0, booted.Position().Y, "leather boots should let the player stand on top of the powder snow layer (Y=2), not sink into it (Y=1)")

	forward := Inputs{ThrottleX: 1.0}
	const ticks = 20
	for range ticks {
		require.NoError(t, normal.Tick(forward, world))
		require.NoError(t, booted.Tick(forward, world))
	}

	normalDist := math.Abs(normal.Position().X - (-15))
	bootedDist := math.Abs(booted.Position().X)
	t.Logf("after %d ticks: normal=%.4f, powder snow (leather boots)=%.4f", ticks, normalDist, bootedDist)

	assert.InDelta(t, normalDist, bootedDist, 1e-6, "leather boots should let the player cross powder snow at full normal speed, with no slowdown")
}

// TestState_HoneyBlockSlowsGroundMovement verifies §5.2: walking on a honey
// block floor now applies HoneyBlockVelocityMultiplier (a direct per-tick
// horizontal velocity multiplier, mirroring Java's Entity.getVelocityMultiplier/
// move()) instead of doing nothing. This is deliberately NOT modeled as a
// ground-friction slipperiness change — honey doesn't override
// slipperiness in vanilla at all (see HoneyBlockVelocityMultiplier's doc
// comment in physics/constants.go); using its 0.4 as a slipperiness input
// would actually make ground acceleration faster, not slower, since the
// formula is not monotonic (see movement/riding_physics_test.go's
// TestTravelMidAirTopSpeedOnIce, which caught this on the riding side
// first).
func TestState_HoneyBlockSlowsGroundMovement(t *testing.T) {
	world, shapes := createFlatWorld()
	shapes.SetHoneyBlock(BlockHoney, true)

	// Replace a patch of the floor itself (Y=0) with honey blocks, wide
	// enough that a correctly-slowed player never leaves it within the
	// test's tick budget.
	for x := -3; x <= 3; x++ {
		for z := -3; z <= 3; z++ {
			world.SetBlock(x, 0, z, BlockHoney)
		}
	}

	settle := func(s models.PhysicsState, x float64) {
		s.SetPositionSimple(models.V3{X: x, Y: 1, Z: 0})
		s.SetVelocity(models.V3{})
		for range 20 {
			require.NoError(t, s.Tick(Inputs{}, world))
			if s.OnGround() && math.Abs(s.Velocity().Y) < 0.01 {
				break
			}
		}
		require.True(t, s.OnGround(), "player should settle on ground before moving")
	}

	normal := NewState(shapes)
	settle(normal, -15) // plain stone floor

	sticky := NewState(shapes)
	settle(sticky, 0) // honey block floor

	forward := Inputs{ThrottleX: 1.0}
	const ticks = 20
	for range ticks {
		require.NoError(t, normal.Tick(forward, world))
		require.NoError(t, sticky.Tick(forward, world))
	}

	normalDist := math.Abs(normal.Position().X - (-15))
	stickyDist := math.Abs(sticky.Position().X)
	t.Logf("after %d ticks: normal=%.4f, honey block floor=%.4f", ticks, normalDist, stickyDist)

	assert.Greater(t, normalDist, stickyDist, "honey block should slow horizontal ground movement below normal")
}

// TestState_HoneyBlockReducesJumpHeight verifies §5.2's other half: honey
// block also registers `.jumpVelocityMultiplier(0.5F)` in Blocks.java
// (alongside the horizontal `.velocityMultiplier(0.4F)`
// TestState_HoneyBlockSlowsGroundMovement covers) — jumping from honey
// mirrors Java's getJumpVelocity() (`attribute * strength *
// getJumpVelocityMultiplier() + getJumpBoostVelocityModifier()`), so the
// base jump velocity is roughly halved, independent of the horizontal
// slowdown.
func TestState_HoneyBlockReducesJumpHeight(t *testing.T) {
	world, shapes := createFlatWorld()
	shapes.SetHoneyBlock(BlockHoney, true)

	for x := -3; x <= 3; x++ {
		for z := -3; z <= 3; z++ {
			world.SetBlock(x, 0, z, BlockHoney)
		}
	}

	settle := func(s models.PhysicsState, x float64) {
		s.SetPositionSimple(models.V3{X: x, Y: 1, Z: 0})
		s.SetVelocity(models.V3{})
		for range 20 {
			require.NoError(t, s.Tick(Inputs{}, world))
			if s.OnGround() && math.Abs(s.Velocity().Y) < 0.01 {
				break
			}
		}
		require.True(t, s.OnGround(), "player should settle on ground before jumping")
	}

	normal := NewState(shapes)
	settle(normal, -15) // plain stone floor

	sticky := NewState(shapes)
	settle(sticky, 0) // honey block floor

	peakHeight := func(s models.PhysicsState) float64 {
		startY := s.Position().Y
		peak := startY
		jump := Inputs{Jump: true}
		idle := Inputs{}
		for i := 0; i < 40; i++ {
			input := idle
			if i == 0 {
				input = jump
			}
			require.NoError(t, s.Tick(input, world))
			if s.Position().Y > peak {
				peak = s.Position().Y
			}
			if i > 0 && s.OnGround() {
				break
			}
		}
		return peak - startY
	}

	normalPeak := peakHeight(normal)
	stickyPeak := peakHeight(sticky)
	t.Logf("jump height: normal=%.4f, honey block floor=%.4f", normalPeak, stickyPeak)

	assert.Greater(t, normalPeak, stickyPeak, "honey block should reduce jump height below normal")
	assert.Greater(t, stickyPeak, 0.0, "honey block should still allow some jump, not fully block it")
}

// TestState_IceAcceleratesSlowerButCoastsFurther verifies the descoped-then-
// implemented ice ground-friction item: walking onto ice now uses
// IceSlipperiness (0.98, a genuine AbstractBlock.Settings.slipperiness()
// value — see physics/state.go's Tick() ground friction switch) instead of
// always falling through to the flat Slipperiness default. The formula
// (movementSpeed * 0.216 / slipperiness³ for acceleration,
// Inertia * slipperiness for momentum retention) predicts the real vanilla
// "feel": slower to build up speed, but carries much more momentum once
// moving — covering less ground during a short burst from rest, but coasting
// further after input stops. See movement/riding_physics_test.go's
// TestTravelMidAirTopSpeedOnIce for the same formula's ordering already
// verified in isolation on the riding side.
func TestState_IceAcceleratesSlowerButCoastsFurther(t *testing.T) {
	world, shapes := createFlatWorld()
	shapes.SetIce(BlockIce, true)

	for x := -3; x <= 3; x++ {
		for z := -3; z <= 3; z++ {
			world.SetBlock(x, 0, z, BlockIce)
		}
	}

	settle := func(s models.PhysicsState, x float64) {
		s.SetPositionSimple(models.V3{X: x, Y: 1, Z: 0})
		s.SetVelocity(models.V3{})
		for range 20 {
			require.NoError(t, s.Tick(Inputs{}, world))
			if s.OnGround() && math.Abs(s.Velocity().Y) < 0.01 {
				break
			}
		}
		require.True(t, s.OnGround(), "player should settle on ground before moving")
	}

	normal := NewState(shapes)
	settle(normal, -15) // plain stone floor

	icy := NewState(shapes)
	settle(icy, 0) // ice floor

	// Burst phase: hold forward throttle from rest for a short window.
	const burstTicks = 15
	forward := Inputs{ThrottleX: 1.0}
	normalBurstStart := normal.Position().X
	icyBurstStart := icy.Position().X
	for range burstTicks {
		require.NoError(t, normal.Tick(forward, world))
		require.NoError(t, icy.Tick(forward, world))
	}
	normalBurstDist := math.Abs(normal.Position().X - normalBurstStart)
	icyBurstDist := math.Abs(icy.Position().X - icyBurstStart)
	t.Logf("burst (%d ticks from rest): normal=%.4f, ice=%.4f", burstTicks, normalBurstDist, icyBurstDist)
	assert.Greater(t, normalBurstDist, icyBurstDist, "ice should accelerate slower than normal ground from a standing start")

	// Coast phase: release throttle and let momentum carry each player.
	const coastTicks = 30
	idle := Inputs{}
	normalCoastStart := normal.Position().X
	icyCoastStart := icy.Position().X
	for range coastTicks {
		require.NoError(t, normal.Tick(idle, world))
		require.NoError(t, icy.Tick(idle, world))
	}
	normalCoastDist := math.Abs(normal.Position().X - normalCoastStart)
	icyCoastDist := math.Abs(icy.Position().X - icyCoastStart)
	t.Logf("coast (%d ticks, no input): normal=%.4f, ice=%.4f", coastTicks, normalCoastDist, icyCoastDist)
	assert.Greater(t, icyCoastDist, normalCoastDist, "ice should carry momentum further than normal ground once moving")
}

// TestState_HoneyBlockSideSlideCapsDescent verifies §5.2's other descoped
// item: falling past the side of a honey column (not standing on top of it)
// now caps descent to a slow glide, mirroring HoneyBlock.isSliding/
// updateSlidingVelocity — see physics/state.go's isSlidingOnHoney and
// HoneySlideDescentRate's doc comments for the simplified formula this uses
// in place of vanilla's exact getOldVelocityY/getNewVelocityY round-trip.
func TestState_HoneyBlockSideSlideCapsDescent(t *testing.T) {
	world, shapes := createFlatWorld()
	shapes.SetHoneyBlock(BlockHoney, true)

	// A single honey block the player is falling directly alongside (not
	// on top of — see the X offset below).
	world.SetBlock(0, 5, 0, BlockHoney)

	s := NewState(shapes)
	// Positioned beside the honey block (its cell is x=[0,1)), roughly
	// level with its top, already falling fast with some horizontal drift.
	s.SetPositionSimple(models.V3{X: 1.05, Y: 5.5, Z: 0.5})
	s.SetVelocity(models.V3{X: 0.3, Y: -0.5, Z: 0})

	require.NoError(t, s.Tick(Inputs{}, world))

	vel := s.Velocity()
	t.Logf("after 1 tick beside honey: vel=(%.4f, %.4f, %.4f)", vel.X, vel.Y, vel.Z)

	// The slide clamp sets Vel.Y to -HoneySlideDescentRate (-0.05) and
	// Vel.X to 0.3 * (-0.05/-0.5) = 0.03 before tickPosition runs, but
	// applyEnvironmentForces still applies one step of gravity+drag to
	// both afterward, later in this same Tick() call — which happens to
	// be exactly vanilla's own getNewVelocityY(-0.05) round-trip
	// ((-0.05-Gravity)*Drag), just arrived at via this engine's normal
	// per-tick force application rather than a dedicated helper. The
	// *stored* value at the end of the tick is this composed result; next
	// tick's isSlidingOnHoney entry check (reading that stored value) is
	// still well below HoneySlideEntryThreshold, so sliding continues and
	// the clamp re-applies fresh every tick — this is the steady state,
	// not a one-off transient.
	expectedY := (-HoneySlideDescentRate - Gravity) * Drag
	expectedX := (0.3 * (-HoneySlideDescentRate / -0.5)) * Inertia
	assert.InDelta(t, expectedY, vel.Y, 1e-9, "honey side-slide should clamp descent to the vanilla -0.05 steady glide speed (post-gravity/drag)")
	assert.InDelta(t, expectedX, vel.X, 1e-9, "fast horizontal drift should be damped proportionally to the descent-speed reduction")
}

// TestState_HoneyBlockSideSlideRequiresEstablishedFall verifies
// isSlidingOnHoney's entry threshold: a small downward velocity (not yet a
// genuine fall) beside a honey block should not trigger sliding, matching
// HoneyBlock.isSliding's `getOldVelocityY(velocity.y) >= -0.08` early
// return.
func TestState_HoneyBlockSideSlideRequiresEstablishedFall(t *testing.T) {
	world, shapes := createFlatWorld()
	shapes.SetHoneyBlock(BlockHoney, true)
	world.SetBlock(0, 5, 0, BlockHoney)

	s := NewState(shapes)
	s.SetPositionSimple(models.V3{X: 1.05, Y: 5.5, Z: 0.5})
	s.SetVelocity(models.V3{X: 0, Y: -0.02, Z: 0}) // barely falling, below the entry threshold

	require.NoError(t, s.Tick(Inputs{}, world))

	vel := s.Velocity()
	t.Logf("after 1 tick with a small fall speed: vel.Y=%.4f", vel.Y)
	// Without sliding, ordinary gravity+drag applies: (-0.02 - Gravity) * Drag.
	expected := (-0.02 - Gravity) * Drag
	assert.InDelta(t, expected, vel.Y, 1e-9, "a small downward velocity should not trigger honey side-sliding — ordinary gravity/drag should apply instead")
}
