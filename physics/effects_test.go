package physics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These tests exercise the Phase 4a status-effect formulas directly, without
// a live Minecraft server or a physics.State instance — see effects.go's
// doc comment for the decompiled-source citations.

func TestEffectiveGravity(t *testing.T) {
	tests := []struct {
		name           string
		normalGravity  float64
		velocityY      float64
		hasSlowFalling bool
		expected       float64
	}{
		{name: "no slow falling: normal gravity", normalGravity: Gravity, velocityY: -0.5, hasSlowFalling: false, expected: Gravity},
		{name: "slow falling while falling: capped", normalGravity: Gravity, velocityY: -0.5, hasSlowFalling: true, expected: SlowFallingMaxGravity},
		{name: "slow falling while stationary: capped", normalGravity: Gravity, velocityY: 0, hasSlowFalling: true, expected: SlowFallingMaxGravity},
		{name: "slow falling while rising: NOT capped (Java's velocityY<=0 gate)", normalGravity: Gravity, velocityY: 0.3, hasSlowFalling: true, expected: Gravity},
		{
			name: "slow falling never increases gravity above normal",
			// A hypothetical normalGravity smaller than the cap should stay
			// at normalGravity, not be raised up to the cap.
			normalGravity: 0.005, velocityY: -0.1, hasSlowFalling: true, expected: 0.005,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveGravity(tt.normalGravity, tt.velocityY, tt.hasSlowFalling)
			assert.InDelta(t, tt.expected, got, 1e-9)
		})
	}
}

func TestLevitationVerticalVelocity(t *testing.T) {
	t.Run("level I from rest eases toward 0.05", func(t *testing.T) {
		got := LevitationVerticalVelocity(0, 0)
		want := (LevitationBaseVelocityPerLevel - 0) * LevitationLerpFactor
		assert.InDelta(t, want, got, 1e-9)
	})

	t.Run("higher amplifier means a higher target", func(t *testing.T) {
		level1 := LevitationVerticalVelocity(0, 0)
		level2 := LevitationVerticalVelocity(0, 1)
		level3 := LevitationVerticalVelocity(0, 2)
		assert.Greater(t, level2, level1)
		assert.Greater(t, level3, level2)
	})

	t.Run("converges to the target over repeated ticks", func(t *testing.T) {
		velY := -0.3 // falling fast when levitation kicks in
		target := LevitationBaseVelocityPerLevel * (0 + 1)
		for range 200 {
			velY = LevitationVerticalVelocity(velY, 0)
		}
		assert.InDelta(t, target, velY, 1e-6, "should converge to the level's target velocity given enough ticks")
	})

	t.Run("already at target stays at target", func(t *testing.T) {
		target := LevitationBaseVelocityPerLevel * (0 + 1)
		got := LevitationVerticalVelocity(target, 0)
		assert.InDelta(t, target, got, 1e-9)
	})
}

func TestPerceptionRadiusCap(t *testing.T) {
	tests := []struct {
		name                      string
		radius                    float64
		hasBlindness, hasDarkness bool
		expected                  float64
	}{
		{name: "no effect: unchanged", radius: 32.0, expected: 32.0},
		{name: "blindness caps to 5", radius: 32.0, hasBlindness: true, expected: BlindnessVisionRadius},
		{name: "darkness caps to 15", radius: 32.0, hasDarkness: true, expected: DarknessVisionRadius},
		{
			name:   "both active: more restrictive (blindness) wins",
			radius: 32.0, hasBlindness: true, hasDarkness: true, expected: BlindnessVisionRadius,
		},
		{
			name:   "requested radius already smaller than either cap: never raised",
			radius: 2.0, hasBlindness: true, hasDarkness: true, expected: 2.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PerceptionRadiusCap(tt.radius, tt.hasBlindness, tt.hasDarkness)
			assert.InDelta(t, tt.expected, got, 1e-9)
		})
	}
}
