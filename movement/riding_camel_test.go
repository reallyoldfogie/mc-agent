package movement

import (
	"fmt"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
)

// TestCamelDashForwardVelocity exercises the dash-impulse composition at the
// handler's call site (movement/riding_camel.go's dash-apply block) — the
// piece Priority 2 named as untested, in contrast to models.DashImpulse
// itself (models/camel_test.go) and the charge/cooldown state machine
// (models/camel_test.go), both already covered.
func TestCamelDashForwardVelocity(t *testing.T) {
	const strength = 1.0
	const movementAcceleration = models.CamelDefaultMovementSpeed

	// The re-projection in camelDashForwardVelocity is a dot product of
	// DashImpulse's (X,Z) delta with the same yaw-facing unit vector that
	// produced it, which mathematically recovers exactly the horizontal
	// magnitude DashImpulse computed internally — independent of yaw. This
	// is the key invariant the handler's composition relies on.
	expectedHorizontal := models.CamelDashHorizontalFactor * strength * movementAcceleration
	expectedVertical := models.CamelDashVerticalFactor * strength * models.CamelJumpStrength

	for _, yaw := range []float64{0, 45, 90, 137.5, 180, -90, 359} {
		t.Run(fmt.Sprintf("yaw=%v", yaw), func(t *testing.T) {
			newForward, deltaVelY := camelDashForwardVelocity(0, yaw, strength, movementAcceleration)
			assert.InDelta(t, expectedHorizontal, newForward, 1e-9, "yaw=%v", yaw)
			assert.InDelta(t, expectedVertical, deltaVelY, 1e-9, "yaw=%v", yaw)
		})
	}
}

// TestCamelDashForwardVelocity_AddsToBase confirms the dash impulse is
// additive to the rider's existing forward velocity, not a replacement —
// the handler adds it to pe.ridingVelZ from before the tick's own velocity
// update, which is why baseForwardVelocity is a caller-supplied parameter
// rather than something this function derives itself.
func TestCamelDashForwardVelocity_AddsToBase(t *testing.T) {
	withZeroBase, _ := camelDashForwardVelocity(0, 0, 1.0, models.CamelDefaultMovementSpeed)
	withNonZeroBase, _ := camelDashForwardVelocity(0.5, 0, 1.0, models.CamelDefaultMovementSpeed)
	assert.InDelta(t, withZeroBase+0.5, withNonZeroBase, 1e-9)
}

// TestCamelDashForwardVelocity_ScalesWithStrength confirms a fuller charge
// produces a bigger impulse, matching DashImpulse's own linear scaling.
func TestCamelDashForwardVelocity_ScalesWithStrength(t *testing.T) {
	half, _ := camelDashForwardVelocity(0, 0, 0.5, models.CamelDefaultMovementSpeed)
	full, _ := camelDashForwardVelocity(0, 0, 1.0, models.CamelDefaultMovementSpeed)
	assert.Greater(t, full, half)
	assert.InDelta(t, half*2, full, 1e-9, "dash horizontal impulse scales linearly with strength")
}
