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
