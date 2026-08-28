package physics

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise SetElytraEquipped/Tick() end-to-end — elytra_test.go
// already covers GlidingVelocity/CanGlide/CanStartGliding in isolation;
// these confirm state.go actually wires the glide-start/stop transition and
// GlidingVelocity into a real tick.

func TestState_ElytraStartsGlidingOnJumpWhileAirborne(t *testing.T) {
	world, shapes := createFlatWorld()

	state := NewState(shapes)
	state.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	state.SetVelocity(models.V3{Y: -0.5})
	state.SetElytraEquipped(true)

	require.False(t, state.IsGliding(), "should not start gliding before any jump input")

	require.NoError(t, state.Tick(Inputs{Jump: true}, world))

	assert.True(t, state.IsGliding(), "jump input while airborne with an elytra equipped should start gliding")
}

func TestState_ElytraDoesNotStartWithoutElytraEquipped(t *testing.T) {
	world, shapes := createFlatWorld()

	state := NewState(shapes)
	state.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	state.SetVelocity(models.V3{Y: -0.5})
	// SetElytraEquipped intentionally not called - defaults to false.

	require.NoError(t, state.Tick(Inputs{Jump: true}, world))

	assert.False(t, state.IsGliding(), "jump input without an elytra equipped should not start gliding")
}

func TestState_ElytraDoesNotStartOnGround(t *testing.T) {
	world, shapes := createFlatWorld()

	state := NewState(shapes)
	// Ground platform top surface is Y=1 (blocks placed at Y=0).
	state.SetPositionSimple(models.V3{X: 0, Y: 1, Z: 0})
	state.SetVelocity(models.V3{})
	state.SetElytraEquipped(true)
	state.SetOnGround(true)

	// Clear the jump cooldown (MinJumpTicks) before attempting to jump -
	// unrelated to gliding, just a fresh state's lastJump=0 starting point.
	for range MinJumpTicks {
		require.NoError(t, state.Tick(Inputs{}, world))
	}

	require.NoError(t, state.Tick(Inputs{Jump: true}, world))

	assert.False(t, state.IsGliding(), "jump while on ground should perform a normal jump, not start gliding")
	assert.Greater(t, state.Velocity().Y, 0.0, "should still perform the normal ground jump")
}

func TestState_ElytraStopsGlidingOnLanding(t *testing.T) {
	world, shapes := createFlatWorld()

	st := NewState(shapes).(*state)
	st.Pos = models.V3{X: 0, Y: 1.1, Z: 0}
	st.Vel = models.V3{Y: -0.05}
	st.elytraEquipped = true
	st.isGliding = true // force mid-glide, close enough to the ground to land within a tick or two

	for range 5 {
		require.NoError(t, st.Tick(Inputs{}, world))
		if !st.IsGliding() {
			break
		}
	}

	assert.False(t, st.IsGliding(), "landing should auto-stop gliding, mirroring tickGliding()'s per-tick canGlide() re-check")
	assert.True(t, st.OnGround(), "should have actually landed")
}

func TestState_ElytraGlideFallsSlowerThanNormalFalling(t *testing.T) {
	world, shapes := createFlatWorld()

	normal := NewState(shapes)
	normal.SetPositionSimple(models.V3{X: 0, Y: 100, Z: 0})
	normal.SetVelocity(models.V3{Y: -0.5})

	gliding := NewState(shapes)
	gliding.SetPositionSimple(models.V3{X: 10, Y: 100, Z: 0})
	gliding.SetVelocity(models.V3{Y: -0.5})
	gliding.SetElytraEquipped(true)
	require.NoError(t, gliding.Tick(Inputs{Jump: true}, world)) // start gliding
	require.True(t, gliding.IsGliding())

	const ticks = 40
	for range ticks {
		require.NoError(t, normal.Tick(Inputs{}, world))
		require.NoError(t, gliding.Tick(Inputs{}, world))
	}

	normalDrop := 100 - normal.Position().Y
	glidingDrop := 100 - gliding.Position().Y

	t.Logf("after %d ticks: normal drop=%.2f, gliding drop=%.2f", ticks, normalDrop, glidingDrop)
	assert.Greater(t, normalDrop, glidingDrop, "level-flight gliding should descend much more slowly than an ordinary fall")
}

func TestState_ElytraGlideCoversMoreHorizontalDistanceThanFalling(t *testing.T) {
	world, shapes := createFlatWorld()

	gliding := NewState(shapes)
	gliding.SetPositionSimple(models.V3{X: 0, Y: 100, Z: 0})
	gliding.SetVelocity(models.V3{Y: -0.5})
	gliding.SetYaw(0) // looking south (+Z)
	gliding.SetElytraEquipped(true)
	require.NoError(t, gliding.Tick(Inputs{Jump: true, Yaw: 0}, world))
	require.True(t, gliding.IsGliding())

	falling := NewState(shapes)
	falling.SetPositionSimple(models.V3{X: 10, Y: 100, Z: 0})
	falling.SetVelocity(models.V3{Y: -0.5})

	const ticks = 40
	for range ticks {
		require.NoError(t, gliding.Tick(Inputs{Yaw: 0}, world))
		require.NoError(t, falling.Tick(Inputs{}, world))
	}

	glidingHorizontalDist := gliding.Position().Z
	fallingHorizontalDist := falling.Position().Z

	t.Logf("after %d ticks: gliding horizontal Z=%.2f, falling horizontal Z=%.2f", ticks, glidingHorizontalDist, fallingHorizontalDist)
	assert.Greater(t, glidingHorizontalDist, fallingHorizontalDist,
		"gliding while looking forward should accumulate real forward speed unlike a straight fall (no WASD thrust either)")
}
