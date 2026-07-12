package models

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNautilusStateDefaultMovementSpeed(t *testing.T) {
	nautilus := NewNautilusState(0, false)
	zombie := NewNautilusState(0, true)

	assert.InDelta(t, NautilusDefaultMovementSpeed, nautilus.DefaultMovementSpeed(), 1e-9)
	assert.InDelta(t, NautilusZombieMovementSpeed, zombie.DefaultMovementSpeed(), 1e-9)
	assert.Greater(t, zombie.DefaultMovementSpeed(), nautilus.DefaultMovementSpeed(),
		"zombie nautilus should default faster than the regular nautilus")
}

func TestNautilusStateEaseYawTowardHalvesDelta(t *testing.T) {
	state := NewNautilusState(0, false)

	// Each tick the vehicle yaw moves half the remaining delta toward the target.
	assert.InDelta(t, 45.0, state.EaseYawToward(90), 1e-9)
	assert.InDelta(t, 67.5, state.EaseYawToward(90), 1e-9)

	// Converges toward the target over time.
	for range 30 {
		state.EaseYawToward(90)
	}
	assert.InDelta(t, 90.0, state.GetVehicleYaw(), 1e-3)
}

func TestNautilusStateEaseYawTakesShortestPath(t *testing.T) {
	state := NewNautilusState(170, false)

	// Target -170 is only 20° away across the ±180 boundary, not 340° the long way.
	// delta = wrapDegrees(-170 - 170) = +20, so yaw moves to 170 + 20*0.5 = 180.
	got := state.EaseYawToward(-170)
	assert.InDelta(t, 180.0, got, 1e-9)
}

func TestNautilusStateDashLifecycle(t *testing.T) {
	state := NewNautilusState(0, false)

	require.True(t, state.CanStartDash(), "a fresh nautilus should be able to dash")
	require.False(t, state.IsDashing())

	state.ApplyDash()
	assert.Equal(t, NautilusDashCooldownTicks, state.DashCooldownTicks)
	assert.True(t, state.IsDashing())
	assert.False(t, state.CanStartDash(), "dash should be on cooldown immediately after firing")

	// The dashing flag clears once the active window elapses.
	clearedWithinWindow := false
	for range NautilusDashActiveTicks + 1 {
		state.TickDashCooldown()
		if !state.IsDashing() {
			clearedWithinWindow = true
			break
		}
	}
	assert.True(t, clearedWithinWindow, "dashing flag should clear shortly after the dash")

	// Run out the rest of the cooldown; it should report ready exactly once.
	readyCount := 0
	for range NautilusDashCooldownTicks {
		if state.TickDashCooldown() {
			readyCount++
		}
	}
	assert.Equal(t, 1, readyCount, "cooldown should signal ready exactly once")
	assert.True(t, state.CanStartDash(), "dash should be available again after the cooldown")
}

func TestNautilusDashImpulseDirection(t *testing.T) {
	const strength = 1.0
	const movementSpeed = 1.0
	const velocityMultiplier = 1.0

	// Looking flat south (yaw 0, pitch 0) in water: impulse is purely +Z and uses
	// the water factor (1.2).
	dx, dy, dz := NautilusDashImpulse(0, 0, strength, movementSpeed, velocityMultiplier, true)
	assert.InDelta(t, 0.0, dx, 1e-9)
	assert.InDelta(t, 0.0, dy, 1e-9)
	assert.InDelta(t, NautilusWaterDashFactor, dz, 1e-9)

	// On land the factor is smaller (0.5), so the same look yields a weaker dash.
	_, _, dzLand := NautilusDashImpulse(0, 0, strength, movementSpeed, velocityMultiplier, false)
	assert.InDelta(t, NautilusLandDashFactor, dzLand, 1e-9)
	assert.Less(t, dzLand, dz, "land dash should be weaker than water dash")

	// Looking straight up (pitch -90) sends the dash upward (+Y).
	_, dyUp, _ := NautilusDashImpulse(0, -90, strength, movementSpeed, velocityMultiplier, true)
	assert.Greater(t, dyUp, 0.0, "looking up should dash upward")
	assert.InDelta(t, NautilusWaterDashFactor, dyUp, 1e-6)
}

func TestNautilusDashImpulseScalesWithStrengthAndSpeed(t *testing.T) {
	_, _, weak := NautilusDashImpulse(0, 0, 0.4, 1.0, 1.0, true)
	_, _, strong := NautilusDashImpulse(0, 0, 1.0, 1.0, 1.0, true)
	assert.Greater(t, strong, weak, "higher charge strength should dash farther")

	_, _, slow := NautilusDashImpulse(0, 0, 1.0, 1.0, 1.0, true)
	_, _, fast := NautilusDashImpulse(0, 0, 1.0, 1.1, 1.0, true)
	assert.Greater(t, fast, slow, "higher movement speed should dash farther")
	assert.InDelta(t, slow*1.1, fast, 1e-9)
}

func TestNautilusClampJumpStrengthBounds(t *testing.T) {
	// Shared with the camel dash; verify the nautilus charge cap saturates it.
	assert.InDelta(t, 1.0, ClampJumpStrength(NautilusDashChargeCapTicks), 1e-9)
	assert.InDelta(t, 1.0, ClampJumpStrength(90), 1e-9)
	assert.Greater(t, ClampJumpStrength(45), 0.4)
	assert.Less(t, ClampJumpStrength(45), 1.0)
	assert.False(t, math.IsNaN(ClampJumpStrength(0)))
}
