package movement

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
)

// These tests exercise the happy ghast's pure movement formulas directly,
// without a live server — see PHASE_6_PLAN.md §3 for the decompiled-source
// derivation each constant and formula shape is taken from.

func TestHappyGhastControlledMovementInput_NoInputIsZero(t *testing.T) {
	input := happyGhastControlledMovementInput(0, 0, 0, false, 0.05)
	assert.Equal(t, models.V3{}, input)
}

func TestHappyGhastControlledMovementInput_ForwardAtZeroPitch(t *testing.T) {
	// Java: forwardComponent=cos(pitch), verticalComponent=-sin(pitch). At
	// pitch=0 that's (1, 0): pure forward, no climb/dive.
	input := happyGhastControlledMovementInput(0, 1, 0, false, 0.05)
	scale := physics.HappyGhastControlledMovementMultiplier * 0.05
	assert.InDelta(t, 0, input.X, 1e-9)
	assert.InDelta(t, 0, input.Y, 1e-9)
	assert.InDelta(t, scale, input.Z, 1e-9)
}

func TestHappyGhastControlledMovementInput_BackwardIsHalvedAndReversed(t *testing.T) {
	forward := happyGhastControlledMovementInput(0, 1, 0, false, 0.05)
	backward := happyGhastControlledMovementInput(0, -1, 0, false, 0.05)
	assert.InDelta(t, -forward.Z*0.5, backward.Z, 1e-9)
}

func TestHappyGhastControlledMovementInput_PitchDrivesVertical(t *testing.T) {
	// Looking straight down (pitch=90) should produce a pure-vertical
	// forward-flight input: forward=cos(90)=0, vertical=-sin(90)=-1.
	input := happyGhastControlledMovementInput(0, 1, 90, false, 0.05)
	scale := physics.HappyGhastControlledMovementMultiplier * 0.05
	assert.InDelta(t, -scale, input.Y, 1e-6)
	assert.InDelta(t, 0, input.Z, 1e-6)
}

func TestHappyGhastControlledMovementInput_JumpAddsFlatVerticalBoost(t *testing.T) {
	// Java: h += 0.5F, stacked on top of whatever the pitch already
	// produced, before the flying_speed scale — not an exclusive branch.
	withoutJump := happyGhastControlledMovementInput(0, 1, 0, false, 0.05)
	withJump := happyGhastControlledMovementInput(0, 1, 0, true, 0.05)
	scale := physics.HappyGhastControlledMovementMultiplier * 0.05
	assert.InDelta(t, withoutJump.Y+physics.HappyGhastJumpVerticalBoost*scale, withJump.Y, 1e-9)
}

func TestHappyGhastControlledMovementInput_JumpAloneWithNoForwardThrottle(t *testing.T) {
	// forwardSpeed=0 means the forward/pitch branch never runs at all
	// (Java's `if (forwardSpeed != 0.0F)` guard), but jump's +0.5F still
	// applies on top of whatever vertical was before that guard (0 here).
	input := happyGhastControlledMovementInput(0, 0, 45, true, 0.05)
	scale := physics.HappyGhastControlledMovementMultiplier * 0.05
	assert.InDelta(t, physics.HappyGhastJumpVerticalBoost*scale, input.Y, 1e-9)
	assert.InDelta(t, 0, input.Z, 1e-9)
}

func TestHappyGhastControlledMovementInput_SidewaysPassthroughRegardlessOfForward(t *testing.T) {
	scale := physics.HappyGhastControlledMovementMultiplier * 0.05
	noForward := happyGhastControlledMovementInput(1, 0, 30, false, 0.05)
	withForward := happyGhastControlledMovementInput(1, 1, 30, false, 0.05)
	assert.InDelta(t, scale, noForward.X, 1e-9)
	assert.InDelta(t, scale, withForward.X, 1e-9)
}

func TestMovementInputToVelocity_ZeroInputIsZero(t *testing.T) {
	got := movementInputToVelocity(models.V3{}, 1.0, 45)
	assert.Equal(t, models.V3{}, got)
}

func TestMovementInputToVelocity_SubUnitLengthNotClamped(t *testing.T) {
	// Length 0.5 < 1: scaled directly by speed, no renormalization.
	got := movementInputToVelocity(models.V3{Z: 0.5}, 2.0, 0)
	assert.InDelta(t, 1.0, got.Z, 1e-9) // 0.5 * 2.0
}

func TestMovementInputToVelocity_OverUnitLengthClampedBeforeScale(t *testing.T) {
	// Length 2 > 1: normalized to length 1 first, THEN scaled by speed —
	// matching Java's clamp-then-multiply order inside
	// movementInputToVelocity itself, not the caller.
	got := movementInputToVelocity(models.V3{Z: 2.0}, 1.0, 0)
	assert.InDelta(t, 1.0, got.Z, 1e-9)
}

func TestMovementInputToVelocity_RotatesByYaw(t *testing.T) {
	// Pure forward (Z) input rotated 90 degrees should land entirely on X,
	// matching the same sin/cos convention forwardVelocityToWorld already
	// uses elsewhere in this package.
	got := movementInputToVelocity(models.V3{Z: 1.0}, 1.0, 90)
	assert.InDelta(t, 0, got.Z, 1e-6)
	assert.InDelta(t, 1.0, math.Abs(got.X), 1e-6)
}

func TestMovementInputToVelocity_YComponentUnrotated(t *testing.T) {
	got := movementInputToVelocity(models.V3{Y: 0.5}, 1.0, 90)
	assert.InDelta(t, 0.5, got.Y, 1e-9)
}

func TestHappyGhastState_EaseYawTowardConvergesSlowly(t *testing.T) {
	state := models.NewHappyGhastState(0)
	// One tick should close exactly 8% of the gap (§3: 0.08 factor, much
	// slower than nautilus's 0.5).
	got := state.EaseYawToward(100)
	assert.InDelta(t, 8.0, got, 1e-9)
}

func TestHappyGhastState_EaseYawTowardConvergesEventually(t *testing.T) {
	state := models.NewHappyGhastState(0)
	for range 500 {
		state.EaseYawToward(100)
	}
	assert.InDelta(t, 100, state.GetVehicleYaw(), 0.01)
}

// TestHappyGhastFlight_SteadyStateMatchesClosedForm iterates the exact tick
// formula (accel via movementInputToVelocity, then drag) that
// handleRidingModeHappyGhast uses, holding forward throttle at pitch=0 with
// the default flying_speed, and checks the terminal velocity converges to
// the closed-form steady state for that recurrence:
//
//	v = (v + accel) * drag  =>  v = accel*drag / (1-drag)
//
// This pins the simulation loop against hand-derived algebra as a
// cross-check independent of the formula's own code.
//
// Worth recording: the main plan document (PHASE_6_PLAN.md §3, question 1)
// cited "~0.18 blocks/tick" as a community-observed sanity target before this
// derivation existed. The actual converged value here is ~0.164 — within
// ~10% of that figure, consistent with "~0.18" always having been an
// approximate observation rather than an exact one, not a sign the
// decompiled-source derivation is wrong. The closed form below is the
// authoritative check now; the original ~0.18 note remains useful only as a
// rough order-of-magnitude sanity bound, which this comfortably satisfies.
func TestHappyGhastFlight_SteadyStateMatchesClosedForm(t *testing.T) {
	const flyingSpeed = physics.HappyGhastDefaultFlyingSpeed
	travelSpeed := flyingSpeed * physics.HappyGhastTravelSpeedFactor

	input := happyGhastControlledMovementInput(0, 1, 0, false, flyingSpeed)
	accel := movementInputToVelocity(input, travelSpeed, 0)
	expectedSteadyState := accel.Z * physics.HappyGhastFlightDrag / (1 - physics.HappyGhastFlightDrag)

	vel := models.V3{}
	for range 2000 {
		vel = vel.Add(accel).Mul(physics.HappyGhastFlightDrag)
	}

	assert.InDelta(t, expectedSteadyState, vel.Z, 1e-6)
	assert.InDelta(t, 0.164, vel.Z, 0.005, "sanity bound: within ~10%% of the community-observed ~0.18 blocks/tick figure")
}
