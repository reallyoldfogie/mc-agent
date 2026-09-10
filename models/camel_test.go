package models

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCamelState_NewCamelState(t *testing.T) {
	state := NewCamelState(1000, false)

	assert.Equal(t, CamelPoseStanding, state.Pose)
	assert.False(t, state.IsSitting())
	assert.False(t, state.Dashing)
	assert.Equal(t, 0, state.DashCooldownTicks)
	assert.False(t, state.IsCamelHusk)
	// Should not be changing pose (initialized past the transition window)
	assert.False(t, state.IsChangingPose(1000))
	assert.False(t, state.IsStationary(1000))
}

func TestCamelState_NewCamelStateHusk(t *testing.T) {
	state := NewCamelState(1000, true)

	assert.True(t, state.IsCamelHusk)
	assert.Equal(t, CamelHuskChargingSpeedMultiplier, state.GetChargingSpeedMultiplier())
}

func TestCamelState_SitStandTransitions(t *testing.T) {
	worldTime := int64(1000)
	state := NewCamelState(worldTime, false)

	// Start standing, transition to sitting
	state.StartSitting(worldTime)
	assert.True(t, state.IsSitting())
	assert.Equal(t, -worldTime, state.LastPoseTick)

	// Should be changing pose right after transition
	assert.True(t, state.IsChangingPose(worldTime))
	assert.True(t, state.IsStationary(worldTime))

	// After full sit transition (40 ticks), no longer changing pose but still sitting
	assert.False(t, state.IsChangingPose(worldTime+CamelSitTransitionTicks))
	assert.True(t, state.IsSitting())
	assert.True(t, state.IsStationary(worldTime+CamelSitTransitionTicks)) // sitting = stationary

	// Transition back to standing
	standTime := worldTime + 100
	state.StartStanding(standTime)
	assert.False(t, state.IsSitting())
	assert.Equal(t, standTime, state.LastPoseTick)

	// Changing pose during stand transition
	assert.True(t, state.IsChangingPose(standTime))
	assert.True(t, state.IsStationary(standTime))

	// After full stand transition (52 ticks), no longer changing pose
	assert.False(t, state.IsChangingPose(standTime+CamelStandTransitionTicks))
	assert.False(t, state.IsStationary(standTime+CamelStandTransitionTicks))
}

func TestCamelState_SetStandingImmediate(t *testing.T) {
	worldTime := int64(1000)
	state := NewCamelState(worldTime, false)

	// Sit first
	state.StartSitting(worldTime)
	require.True(t, state.IsSitting())

	// SetStanding should immediately finish the transition
	state.SetStanding(worldTime + 10)
	assert.False(t, state.IsSitting())
	// Should not be in transition (initLastPoseTick sets it far in the past)
	assert.False(t, state.IsChangingPose(worldTime+10))
	assert.False(t, state.IsStationary(worldTime+10))
}

func TestCamelState_StartSittingIdempotent(t *testing.T) {
	worldTime := int64(1000)
	state := NewCamelState(worldTime, false)

	state.StartSitting(worldTime)
	tick := state.LastPoseTick

	// Calling again should be a no-op
	state.StartSitting(worldTime + 50)
	assert.Equal(t, tick, state.LastPoseTick)
}

func TestCamelState_StartStandingIdempotent(t *testing.T) {
	worldTime := int64(1000)
	state := NewCamelState(worldTime, false)

	tick := state.LastPoseTick

	// Already standing, calling StartStanding should be a no-op
	state.StartStanding(worldTime + 50)
	assert.Equal(t, tick, state.LastPoseTick)
}

func TestCamelState_DashCooldown(t *testing.T) {
	state := NewCamelState(1000, false)

	// No cooldown initially
	assert.True(t, state.CanStartDash(true))

	// Apply a dash
	state.ApplyDash()
	assert.Equal(t, CamelDashCooldownTicks, state.DashCooldownTicks)
	assert.True(t, state.Dashing)
	assert.False(t, state.CanStartDash(true))

	// Tick down cooldown
	for range CamelDashCooldownTicks - 1 {
		ready := state.TickDashCooldown()
		assert.False(t, ready)
		assert.False(t, state.CanStartDash(true))
	}

	// Last tick should signal ready
	ready := state.TickDashCooldown()
	assert.True(t, ready)
	assert.True(t, state.CanStartDash(true))

	// Further ticks should not signal again
	ready = state.TickDashCooldown()
	assert.False(t, ready)
}

func TestCamelState_DashEndCondition(t *testing.T) {
	state := NewCamelState(1000, false)

	state.ApplyDash()
	assert.True(t, state.Dashing)

	// Dashing should NOT end when cooldown >= 50
	assert.False(t, state.ShouldEndDash(true, false))

	// Tick down to 49 (5 ticks elapsed)
	for range CamelDashCooldownTicks - CamelDashEndThresholdTicks + 1 {
		state.TickDashCooldown()
	}
	assert.Less(t, state.DashCooldownTicks, CamelDashEndThresholdTicks)

	// Now dashing should end when on ground
	assert.True(t, state.ShouldEndDash(true, false))

	// But not when not on ground and not in fluid
	assert.False(t, state.ShouldEndDash(false, false))

	// Yes when in fluid
	assert.True(t, state.ShouldEndDash(false, true))
}

func TestCamelState_CannotDashInAir(t *testing.T) {
	state := NewCamelState(1000, false)

	assert.True(t, state.CanStartDash(true))   // on ground
	assert.False(t, state.CanStartDash(false)) // in air
}

func TestClampJumpStrength(t *testing.T) {
	// 0 ticks = minimum strength
	assert.InDelta(t, 0.4, ClampJumpStrength(0), 0.001)

	// 45 ticks = halfway
	assert.InDelta(t, 0.6, ClampJumpStrength(45), 0.001)

	// 90 ticks = maximum
	assert.InDelta(t, 1.0, ClampJumpStrength(90), 0.001)

	// 100 ticks = still max (capped at 90)
	assert.InDelta(t, 1.0, ClampJumpStrength(100), 0.001)
}

func TestDashImpulse(t *testing.T) {
	// Facing south (yaw=0), full strength, default camel speed
	deltaX, deltaY, deltaZ := DashImpulse(0.0, 1.0, CamelDefaultMovementSpeed, 1.0)

	// Facing south: sin(0)=0, cos(0)=1, so deltaX should be ~0 and deltaZ should be positive
	assert.InDelta(t, 0.0, deltaX, 0.001)
	assert.Greater(t, deltaZ, 0.0)
	assert.Greater(t, deltaY, 0.0) // Should have vertical component

	// Verify the magnitude matches the Java formula
	expectedHorizontal := CamelDashHorizontalFactor * 1.0 * CamelDefaultMovementSpeed * 1.0
	assert.InDelta(t, expectedHorizontal, deltaZ, 0.001)

	expectedVertical := CamelDashVerticalFactor * 1.0 * CamelJumpStrength
	assert.InDelta(t, expectedVertical, deltaY, 0.001)

	// Facing east (yaw=-90): sin(-(-90))=sin(90)=1 * -1 = -(-1) = should give +X
	deltaX, _, deltaZ = DashImpulse(-90.0, 1.0, CamelDefaultMovementSpeed, 1.0)
	assert.Greater(t, deltaX, 0.0)        // Positive X = east
	assert.InDelta(t, 0.0, deltaZ, 0.001) // No Z component

	// Half strength should give half the impulse
	deltaXHalf, deltaYHalf, deltaZHalf := DashImpulse(0.0, 0.5, CamelDefaultMovementSpeed, 1.0)
	deltaXFull, deltaYFull, deltaZFull := DashImpulse(0.0, 1.0, CamelDefaultMovementSpeed, 1.0)
	assert.InDelta(t, deltaXFull*0.5, deltaXHalf, 0.001)
	assert.InDelta(t, deltaYFull*0.5, deltaYHalf, 0.001)
	assert.InDelta(t, deltaZFull*0.5, deltaZHalf, 0.001)
}

func TestCamelState_ChargingSpeedMultiplier(t *testing.T) {
	regular := NewCamelState(1000, false)
	husk := NewCamelState(1000, true)

	assert.Equal(t, 1.0, regular.GetChargingSpeedMultiplier())
	assert.Equal(t, CamelHuskChargingSpeedMultiplier, husk.GetChargingSpeedMultiplier())
}

func TestDashImpulse_DirectionConsistency(t *testing.T) {
	// Test that DashImpulse produces consistent magnitudes regardless of direction
	speed := CamelDefaultMovementSpeed
	strength := 1.0
	velMul := 1.0

	for _, yaw := range []float64{0, 45, 90, 135, 180, -45, -90, -135, -180} {
		deltaX, deltaY, deltaZ := DashImpulse(yaw, strength, speed, velMul)
		horizontalMag := math.Sqrt(deltaX*deltaX + deltaZ*deltaZ)
		expectedMag := CamelDashHorizontalFactor * strength * speed * velMul
		assert.InDelta(t, expectedMag, horizontalMag, 0.001, "yaw=%.1f horizontal magnitude mismatch", yaw)
		expectedY := CamelDashVerticalFactor * strength * CamelJumpStrength
		assert.InDelta(t, expectedY, deltaY, 0.001, "yaw=%.1f vertical magnitude mismatch", yaw)
	}
}
