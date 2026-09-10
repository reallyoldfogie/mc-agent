package physics

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise SetFlying/Tick() end-to-end (PHASE 4.4 §step 3) —
// elytra_test.go's TestCanGlide already covers the flying/gliding exclusion
// in isolation; these confirm state.go actually wires the vertical
// override and ascend/descend impulse into a real tick.

const testFlySpeed = 0.05 // vanilla default (PlayerAbilities.DEFAULT_FLY_SPEED)

func TestState_FlyingHoldJumpClimbsToSteadyStateVelocity(t *testing.T) {
	world, shapes := createFlatWorld()

	s := NewState(shapes, nil)
	s.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	s.SetVelocity(models.V3{})
	s.SetFlying(true, testFlySpeed)

	// Steady state solves v = 0.6*(v + impulse) for v: v = 1.5*impulse =
	// 1.5*flySpeed*3.0 = 4.5*flySpeed. See FlyingVerticalDecay/
	// FlyingVerticalImpulseScale's doc comments.
	wantSteadyState := 4.5 * testFlySpeed

	const ticks = 200
	for range ticks {
		require.NoError(t, s.Tick(Inputs{Jump: true}, world))
	}

	assert.InDelta(t, wantSteadyState, s.Velocity().Y, 1e-6,
		"holding jump while flying should converge to the steady-state ascend velocity")
	assert.Greater(t, s.Position().Y, 50.0, "should have actually gained altitude")
}

func TestState_FlyingHoldSneakDescendsToSteadyStateVelocity(t *testing.T) {
	world, shapes := createFlatWorld()

	s := NewState(shapes, nil)
	s.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	s.SetVelocity(models.V3{})
	s.SetFlying(true, testFlySpeed)

	wantSteadyState := -4.5 * testFlySpeed

	// Fewer ticks than the jump test: descending long enough to reach the
	// ground (Y=0 platform) would land and zero vertical velocity, which
	// is correct behavior but would defeat this test's purpose.
	const ticks = 60
	for range ticks {
		require.NoError(t, s.Tick(Inputs{Sneak: true}, world))
	}

	assert.InDelta(t, wantSteadyState, s.Velocity().Y, 1e-6,
		"holding sneak while flying should converge to the steady-state descend velocity")
	assert.Less(t, s.Position().Y, 50.0, "should have actually lost altitude")
}

func TestState_FlyingNoInputDecaysRatherThanFalling(t *testing.T) {
	world, shapes := createFlatWorld()

	flying := NewState(shapes, nil)
	flying.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	flying.SetVelocity(models.V3{})
	flying.SetFlying(true, testFlySpeed)

	normal := NewState(shapes, nil)
	normal.SetPositionSimple(models.V3{X: 10, Y: 50, Z: 0})
	normal.SetVelocity(models.V3{})

	const ticks = 40
	for range ticks {
		require.NoError(t, flying.Tick(Inputs{}, world))
		require.NoError(t, normal.Tick(Inputs{}, world))
	}

	flyingDrop := 50 - flying.Position().Y
	normalDrop := 50 - normal.Position().Y

	t.Logf("after %d ticks with no input: flying drop=%.4f, normal-gravity drop=%.4f", ticks, flyingDrop, normalDrop)
	assert.InDelta(t, 0.0, flyingDrop, 1e-6, "flying with no ascend/descend input should hold altitude, not fall")
	assert.Greater(t, normalDrop, 1.0, "normal gravity should have produced real fall distance by comparison")
}

func TestState_FlyingHorizontalMovementMatchesNormalAirStrafing(t *testing.T) {
	// PlayerEntity.travel()'s flying branch only overrides vertical
	// velocity - LivingEntity.travel()/travelMidAir() has no
	// flying-specific horizontal branch at all, so horizontal acceleration
	// while flying should be identical to normal airborne (not onGround)
	// movement. See applyMovementInputs's doc comment.
	world, shapes := createFlatWorld()

	flying := NewState(shapes, nil)
	flying.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	flying.SetVelocity(models.V3{})
	flying.SetFlying(true, testFlySpeed)

	airborne := NewState(shapes, nil)
	airborne.SetPositionSimple(models.V3{X: 10, Y: 50, Z: 0})
	airborne.SetVelocity(models.V3{})

	input := Inputs{ThrottleX: 0, ThrottleZ: 1, Yaw: 0, Pitch: 0}
	require.NoError(t, flying.Tick(input, world))
	require.NoError(t, airborne.Tick(input, world))

	assert.InDelta(t, airborne.Velocity().X, flying.Velocity().X, 1e-12)
	assert.InDelta(t, airborne.Velocity().Z, flying.Velocity().Z, 1e-12)
}

func TestState_FlyingExcludesGliding(t *testing.T) {
	// canGlide() is !abilities.flying && super.canGlide() in vanilla - even
	// airborne with an elytra equipped and jump pressed (the normal glide
	// start trigger), flying must prevent gliding from ever starting.
	world, shapes := createFlatWorld()

	s := NewState(shapes, nil)
	s.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	s.SetVelocity(models.V3{})
	s.SetFlying(true, testFlySpeed)
	s.SetElytraEquipped(true)

	// Release-then-press jump, matching the real glide-start trigger.
	require.NoError(t, s.Tick(Inputs{Jump: false}, world))
	require.NoError(t, s.Tick(Inputs{Jump: true}, world))

	assert.False(t, s.IsGliding(), "flying should prevent gliding from starting even with an elytra equipped")
}
