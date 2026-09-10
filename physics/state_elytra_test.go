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

	state := NewState(shapes, nil)
	state.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	state.SetVelocity(models.V3{Y: -0.5})
	state.SetElytraEquipped(true)

	require.False(t, state.IsGliding(), "should not start gliding before any jump input")

	require.NoError(t, state.Tick(Inputs{Jump: true}, world))

	assert.True(t, state.IsGliding(), "jump input while airborne with an elytra equipped should start gliding")
}

// TestState_ElytraRequiresFreshJumpPressAfterGroundJump mirrors real vanilla
// behavior (traced to ClientPlayerEntity.tick()'s rising-edge check: jump
// state is captured before input.tick() refreshes it, and the glide-start
// check requires the old value false and the new one true): holding jump
// continuously through a normal ground jump's ascent must NOT also start
// gliding the instant the player becomes airborne. Only releasing jump and
// pressing it again produces the genuine "double jump" takeoff real players
// use to launch into flight from a standing start.
func TestState_ElytraRequiresFreshJumpPressAfterGroundJump(t *testing.T) {
	world, shapes := createFlatWorld()

	state := NewState(shapes, nil)
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

	// Ground jump: a single press, held continuously (never released).
	require.NoError(t, state.Tick(Inputs{Jump: true}, world))
	require.False(t, state.IsGliding(), "the ground-jump tick itself should not start gliding")

	// Keep holding jump for a couple more ticks while airborne (ascending
	// from the jump - JumpVelocity is small, so the whole hop only lasts a
	// few ticks). A level-based (non-edge) jump check would incorrectly
	// start gliding here; vanilla's genuine rising-edge requirement must not.
	for range 3 {
		require.NoError(t, state.Tick(Inputs{Jump: true}, world))
		require.False(t, state.IsGliding(),
			"holding jump continuously through a ground jump's ascent must not start gliding — a real double-tap (release then press) is required")
		require.False(t, state.OnGround(), "should still be airborne for this part of the test")
	}

	// Release jump for one tick, then press it again while still airborne:
	// this is the genuine double-jump takeoff input sequence.
	require.NoError(t, state.Tick(Inputs{Jump: false}, world))
	require.False(t, state.IsGliding())

	require.NoError(t, state.Tick(Inputs{Jump: true}, world))
	assert.True(t, state.IsGliding(), "a genuine release-then-press while airborne should start gliding")
}

func TestState_ElytraDoesNotStartWithoutElytraEquipped(t *testing.T) {
	world, shapes := createFlatWorld()

	state := NewState(shapes, nil)
	state.SetPositionSimple(models.V3{X: 0, Y: 50, Z: 0})
	state.SetVelocity(models.V3{Y: -0.5})
	// SetElytraEquipped intentionally not called - defaults to false.

	require.NoError(t, state.Tick(Inputs{Jump: true}, world))

	assert.False(t, state.IsGliding(), "jump input without an elytra equipped should not start gliding")
}

func TestState_ElytraDoesNotStartOnGround(t *testing.T) {
	world, shapes := createFlatWorld()

	state := NewState(shapes, nil)
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

	st := NewState(shapes, nil).(*state)
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

	normal := NewState(shapes, nil)
	normal.SetPositionSimple(models.V3{X: 0, Y: 100, Z: 0})
	normal.SetVelocity(models.V3{Y: -0.5})

	gliding := NewState(shapes, nil)
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

	gliding := NewState(shapes, nil)
	gliding.SetPositionSimple(models.V3{X: 0, Y: 100, Z: 0})
	gliding.SetVelocity(models.V3{Y: -0.5})
	gliding.SetYaw(0) // looking south (+Z)
	gliding.SetElytraEquipped(true)
	require.NoError(t, gliding.Tick(Inputs{Jump: true, Yaw: 0}, world))
	require.True(t, gliding.IsGliding())

	falling := NewState(shapes, nil)
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

// TestState_FireworkBoostRequiresGliding confirms the boost is gated on
// isGliding the same way vanilla re-checks isGliding() every
// FireworkRocketEntity tick — setting SetFireworkBoosting(true) without
// ever starting a glide should have no effect.
func TestState_FireworkBoostRequiresGliding(t *testing.T) {
	world, shapes := createFlatWorld()

	boosted := NewState(shapes, nil)
	boosted.SetPositionSimple(models.V3{X: 0, Y: 100, Z: 0})
	boosted.SetVelocity(models.V3{Y: -0.5})
	boosted.SetFireworkBoosting(true)
	// SetElytraEquipped/glide never triggered.

	plain := NewState(shapes, nil)
	plain.SetPositionSimple(models.V3{X: 10, Y: 100, Z: 0})
	plain.SetVelocity(models.V3{Y: -0.5})

	require.NoError(t, boosted.Tick(Inputs{}, world))
	require.NoError(t, plain.Tick(Inputs{}, world))

	assert.False(t, boosted.IsGliding())
	assert.InDelta(t, plain.Velocity().Y, boosted.Velocity().Y, 1e-9,
		"firework boosting without gliding should behave identically to a plain fall")
}

// TestState_FireworkBoostAcceleratesBeyondPlainGliding confirms the boost
// formula is actually wired into Tick(): a glider with an active firework
// boost should gain far more speed in the look direction than gliding
// alone, per FireworkRocketEntity.tick()'s much stronger per-tick ease
// (FireworkBoostEase=0.5) toward a higher target speed (FireworkBoostTarget
// plus the flat FireworkBoostBlend term) than gliding's own horizontal ease
// (GlideHorizontalEaseFactor=0.1) alone provides.
func TestState_FireworkBoostAcceleratesBeyondPlainGliding(t *testing.T) {
	world, shapes := createFlatWorld()

	boosted := NewState(shapes, nil)
	boosted.SetPositionSimple(models.V3{X: 0, Y: 200, Z: 0})
	boosted.SetVelocity(models.V3{Y: -0.5})
	boosted.SetYaw(0) // looking south (+Z)
	boosted.SetElytraEquipped(true)
	require.NoError(t, boosted.Tick(Inputs{Jump: true, Yaw: 0}, world))
	require.True(t, boosted.IsGliding())
	boosted.SetFireworkBoosting(true)

	plainGlide := NewState(shapes, nil)
	plainGlide.SetPositionSimple(models.V3{X: 10, Y: 200, Z: 0})
	plainGlide.SetVelocity(models.V3{Y: -0.5})
	plainGlide.SetYaw(0)
	plainGlide.SetElytraEquipped(true)
	require.NoError(t, plainGlide.Tick(Inputs{Jump: true, Yaw: 0}, world))
	require.True(t, plainGlide.IsGliding())

	const ticks = 20
	for range ticks {
		require.NoError(t, boosted.Tick(Inputs{Yaw: 0}, world))
		require.NoError(t, plainGlide.Tick(Inputs{Yaw: 0}, world))
	}

	boostedSpeed := boosted.Velocity().Z
	plainSpeed := plainGlide.Velocity().Z
	t.Logf("after %d ticks: boosted forward speed=%.4f, plain glide forward speed=%.4f", ticks, boostedSpeed, plainSpeed)
	assert.Greater(t, boostedSpeed, plainSpeed*2,
		"a firework boost should accelerate forward speed well beyond plain gliding alone")
}
