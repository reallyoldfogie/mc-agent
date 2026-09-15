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

func TestJumpBoostVelocityBonus(t *testing.T) {
	tests := []struct {
		name      string
		amplifier int32
		expected  float64
	}{
		{name: "level I (amplifier 0)", amplifier: 0, expected: JumpBoostVelocityPerLevel},
		{name: "level II (amplifier 1)", amplifier: 1, expected: JumpBoostVelocityPerLevel * 2},
		{name: "level III (amplifier 2)", amplifier: 2, expected: JumpBoostVelocityPerLevel * 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JumpBoostVelocityBonus(tt.amplifier)
			assert.InDelta(t, tt.expected, got, 1e-9)
		})
	}
}

func TestCanSprint(t *testing.T) {
	assert.True(t, CanSprint(false), "should be able to sprint with no blindness")
	assert.False(t, CanSprint(true), "should not be able to sprint while blind")
}

func TestEffectSpeedMultiplier(t *testing.T) {
	tests := []struct {
		name              string
		hasSpeed          bool
		speedAmplifier    int32
		hasSlowness       bool
		slownessAmplifier int32
		expected          float64
	}{
		{name: "no effect: unchanged", expected: 1.0},
		{name: "speed I (amplifier 0)", hasSpeed: true, speedAmplifier: 0, expected: 1.2},
		{name: "speed II (amplifier 1)", hasSpeed: true, speedAmplifier: 1, expected: 1.4},
		{name: "slowness I (amplifier 0)", hasSlowness: true, slownessAmplifier: 0, expected: 0.85},
		{name: "slowness II (amplifier 1)", hasSlowness: true, slownessAmplifier: 1, expected: 0.7},
		{
			name:     "both active: multiplicative, not additive",
			hasSpeed: true, speedAmplifier: 0, hasSlowness: true, slownessAmplifier: 0,
			expected: 1.2 * 0.85,
		},
		{
			name:        "high slowness clamps at zero, does not go negative",
			hasSlowness: true, slownessAmplifier: 10, // 1 + (-0.15)*11 = -0.65, should clamp
			expected: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectSpeedMultiplier(tt.hasSpeed, tt.speedAmplifier, tt.hasSlowness, tt.slownessAmplifier)
			assert.InDelta(t, tt.expected, got, 1e-9)
			assert.GreaterOrEqual(t, got, 0.0, "multiplier should never go negative")
		})
	}
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

func TestHorizontalWaterDrag(t *testing.T) {
	tests := []struct {
		name             string
		hasDolphinsGrace bool
		isSprinting      bool
		expected         float64
	}{
		{name: "no effect, not sprinting: normal water drag", hasDolphinsGrace: false, isSprinting: false, expected: WaterDrag},
		{name: "sprinting, no Dolphin's Grace: reduced drag", hasDolphinsGrace: false, isSprinting: true, expected: SprintWaterDrag},
		{name: "dolphins grace: overrides to flat 0.96 regardless of sprint", hasDolphinsGrace: true, isSprinting: false, expected: DolphinsGraceWaterDragMultiplier},
		{name: "dolphins grace takes precedence over sprint", hasDolphinsGrace: true, isSprinting: true, expected: DolphinsGraceWaterDragMultiplier},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HorizontalWaterDrag(tt.hasDolphinsGrace, tt.isSprinting)
			assert.InDelta(t, tt.expected, got, 1e-9)
		})
	}
}

func TestCobwebSlowdownMultiplier(t *testing.T) {
	tests := []struct {
		name                      string
		hasWeaving                bool
		expectX, expectY, expectZ float64
	}{
		{name: "no weaving: normal cobweb slowdown", hasWeaving: false, expectX: CobwebSlowdownX, expectY: CobwebSlowdownY, expectZ: CobwebSlowdownZ},
		{name: "weaving: halved severity, not bypassed", hasWeaving: true, expectX: WeavingCobwebSlowdownX, expectY: WeavingCobwebSlowdownY, expectZ: WeavingCobwebSlowdownZ},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, z := CobwebSlowdownMultiplier(tt.hasWeaving)
			assert.InDelta(t, tt.expectX, x, 1e-9)
			assert.InDelta(t, tt.expectY, y, 1e-9)
			assert.InDelta(t, tt.expectZ, z, 1e-9)
			assert.Less(t, x, 1.0, "cobweb slowdown should always be well below full speed, weaving or not")
		})
	}
}

func TestPowderSnowSlowdownMultiplier(t *testing.T) {
	x, y, z := PowderSnowSlowdownMultiplier()
	assert.InDelta(t, PowderSnowSlowdownX, x, 1e-9)
	assert.InDelta(t, PowderSnowSlowdownY, y, 1e-9)
	assert.InDelta(t, PowderSnowSlowdownZ, z, 1e-9)
	assert.Less(t, x, 1.0, "horizontal powder snow movement should be slowed below full speed")
	assert.Greater(t, y, 1.0, "vertical multiplier amplifies sinking rather than slowing it")
}
