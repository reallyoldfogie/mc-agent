package movement

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNautilusMovementInput_ForwardFlat(t *testing.T) {
	sideways, vertical, forward := nautilusMovementInput(0, 1.0, 0)
	assert.InDelta(t, 0.0, sideways, 1e-9)
	assert.InDelta(t, 0.0, vertical, 1e-9)
	assert.InDelta(t, 1.0, forward, 1e-9)
}

func TestNautilusMovementInput_ForwardTracksPitch(t *testing.T) {
	// Looking down 45° splits the input between horizontal forward and a downward
	// vertical component (Java forward=cos(pitch), vertical=-sin(pitch)).
	sideways, vertical, forward := nautilusMovementInput(0, 1.0, 45)
	assert.InDelta(t, 0.0, sideways, 1e-9)
	assert.InDelta(t, math.Cos(math.Pi/4), forward, 1e-9)
	assert.InDelta(t, -math.Sin(math.Pi/4), vertical, 1e-9)
	assert.Negative(t, vertical, "looking down should drive the nautilus downward")
}

func TestNautilusMovementInput_BackwardIsHalfSpeed(t *testing.T) {
	// Backward reverses and halves the forward/vertical components.
	_, vertical, forward := nautilusMovementInput(0, -1.0, 0)
	assert.InDelta(t, -0.5, forward, 1e-9)
	assert.InDelta(t, 0.0, vertical, 1e-9)
}

func TestNautilusMovementInput_DiagonalNormalized(t *testing.T) {
	// Full strafe + full forward must normalize to unit length so the combined
	// input never exceeds the saddled speed.
	sideways, vertical, forward := nautilusMovementInput(1.0, 1.0, 0)
	length := math.Sqrt(sideways*sideways + vertical*vertical + forward*forward)
	assert.InDelta(t, 1.0, length, 1e-9)
	assert.InDelta(t, math.Sqrt2/2, sideways, 1e-9)
	assert.InDelta(t, math.Sqrt2/2, forward, 1e-9)
}

func TestNautilusInputToVelocity_RotatesByYaw(t *testing.T) {
	const speed = 0.0325

	// Yaw 0 (south): forward input maps to +Z.
	flat := nautilusInputToVelocity(0, 0, 1.0, 0, speed)
	assert.InDelta(t, 0.0, flat.X, 1e-9)
	assert.InDelta(t, speed, flat.Z, 1e-9)

	// Yaw 90 (west): forward input maps to -X.
	turned := nautilusInputToVelocity(0, 0, 1.0, 90, speed)
	assert.InDelta(t, -speed, turned.X, 1e-9)
	assert.InDelta(t, 0.0, turned.Z, 1e-9)
}

func TestNautilusWaterStep_AccumulatesAndDrags(t *testing.T) {
	const speed = 0.0325
	start := models.V3{X: 0, Y: 0, Z: 0}

	// From rest, forward input at yaw 0: position advances by the pre-drag
	// velocity (== speed) and the stored velocity is dragged by 0.9.
	newPos, newVel := nautilusWaterStep(start, models.V3{}, 0, 0, 1.0, 0, speed)
	assert.InDelta(t, speed, newPos.Z, 1e-9)
	assert.InDelta(t, speed*physics.NautilusWaterDrag, newVel.Z, 1e-9)
}

func TestNautilusWaterStep_DecaysWithoutInput(t *testing.T) {
	start := models.V3{X: 0, Y: 0, Z: 0}
	prevVel := models.V3{X: 0, Y: 0, Z: 1.0}

	// With no input the velocity simply advances the position and decays by 0.9.
	newPos, newVel := nautilusWaterStep(start, prevVel, 0, 0, 0, 0, 0.0325)
	assert.InDelta(t, 1.0, newPos.Z, 1e-9)
	assert.InDelta(t, physics.NautilusWaterDrag, newVel.Z, 1e-9)
}

func TestNautilusLandStep_AppliesGravityAndFriction(t *testing.T) {
	const speed = 0.02
	start := models.V3{X: 0, Y: 0, Z: 0}

	// Airborne, no input: gravity is subtracted from Y then dragged by 0.98.
	_, airVel, onGround := nautilusLandStep(start, models.V3{}, 0, 0, 0, 0, speed, 1.0, false)
	assert.False(t, onGround)
	assert.InDelta(t, (0-physics.Gravity)*physics.Drag, airVel.Y, 1e-9)

	// Grounded with horizontal momentum and no input: XZ velocity is scaled by
	// the block friction (slipperiness * 0.91), not the 0.9 water drag.
	prevVel := models.V3{X: 1.0, Y: 0, Z: 0}
	_, groundVel, grounded := nautilusLandStep(start, prevVel, 0, 0, 0, 0, speed, physics.DefaultSlipperiness, true)
	assert.True(t, grounded)
	assert.InDelta(t, physics.DefaultSlipperiness*physics.Inertia, groundVel.X, 1e-9)
}

func TestApplyNautilusDashCharge_ChargesThenFires(t *testing.T) {
	state := models.NewNautilusState(0, false)
	vel := models.V3{}

	// Rising edge starts charging; nothing fires while the key is still held.
	vel = applyNautilusDashCharge(state, true, false, 0, 0, 1.0, true, vel)
	require.True(t, state.GetIsCharging())
	assert.InDelta(t, 0.0, vel.Z, 1e-9)

	// Hold for several ticks to build charge.
	for range 10 {
		vel = applyNautilusDashCharge(state, true, true, 0, 0, 1.0, true, vel)
	}
	require.Greater(t, state.GetJumpChargeTicks(), 0)

	// Releasing fires the dash: +Z impulse (yaw 0, in water) and the cooldown starts.
	vel = applyNautilusDashCharge(state, false, true, 0, 0, 1.0, true, vel)
	assert.Greater(t, vel.Z, 0.5, "release should impart a forward dash impulse")
	assert.Equal(t, models.NautilusDashCooldownTicks, state.DashCooldownTicks)
	assert.True(t, state.IsDashing())
	assert.False(t, state.GetIsCharging())
}

func TestApplyNautilusDashCharge_CooldownBlocksRestart(t *testing.T) {
	state := models.NewNautilusState(0, false)
	state.ApplyDash() // put the dash on cooldown

	vel := models.V3{}
	// A fresh rising edge must not start a new charge while on cooldown.
	vel = applyNautilusDashCharge(state, true, false, 0, 0, 1.0, true, vel)
	assert.False(t, state.GetIsCharging(), "cooldown should block a new dash charge")
	assert.InDelta(t, 0.0, vel.Z, 1e-9)
}
