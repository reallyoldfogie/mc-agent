package movement

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
)

// These tests exercise the minecart rail-following and powered-rail-boost
// formulas directly, without a live Minecraft server or a
// PhysicsMovementExecutor — the last vehicle Priority 2 named as having no
// server-less coverage at all (movement/riding_minecart.go, the largest
// riding handler).

func TestRailShapeDirection(t *testing.T) {
	tests := []struct {
		shape          string
		wantDx, wantDz float64
		wantIsSlope    bool
	}{
		{shape: "north_south", wantDx: 0, wantDz: 1},
		{shape: "east_west", wantDx: 1, wantDz: 0},
		{shape: "ascending_north", wantDx: 0, wantDz: -1, wantIsSlope: true},
		{shape: "ascending_south", wantDx: 0, wantDz: 1, wantIsSlope: true},
		{shape: "ascending_east", wantDx: 1, wantDz: 0, wantIsSlope: true},
		{shape: "ascending_west", wantDx: -1, wantDz: 0, wantIsSlope: true},
		{shape: "unknown", wantDx: 0, wantDz: 0},
	}
	for _, tt := range tests {
		t.Run(tt.shape, func(t *testing.T) {
			dx, dz, isSlope := railShapeDirection(tt.shape)
			assert.InDelta(t, tt.wantDx, dx, 1e-9)
			assert.InDelta(t, tt.wantDz, dz, 1e-9)
			assert.Equal(t, tt.wantIsSlope, isSlope)
		})
	}
}

// TestRailShapeDirectionDiagonalsAreUnitVectors pins that the four diagonal
// shapes (south_east etc.) produce a unit-length direction vector, since
// their components are 1/sqrt2 rather than a plain axis value.
func TestRailShapeDirectionDiagonalsAreUnitVectors(t *testing.T) {
	for _, shape := range []string{"south_east", "south_west", "north_east", "north_west"} {
		dx, dz, isSlope := railShapeDirection(shape)
		length := math.Sqrt(dx*dx + dz*dz)
		assert.InDelta(t, 1.0, length, 1e-9, "shape %s should be a unit vector", shape)
		assert.False(t, isSlope, "diagonal rail shapes are not slopes")
	}
}

func TestIsSlope(t *testing.T) {
	assert.True(t, isSlope("ascending_north"))
	assert.True(t, isSlope("ascending_south"))
	assert.True(t, isSlope("ascending_east"))
	assert.True(t, isSlope("ascending_west"))
	assert.False(t, isSlope("north_south"))
	assert.False(t, isSlope("east_west"))
	assert.False(t, isSlope("south_east"))
	assert.False(t, isSlope(""))
}

func TestClamp01(t *testing.T) {
	assert.Equal(t, 0.0, clamp01(-0.5))
	assert.Equal(t, 0.0, clamp01(0.0))
	assert.Equal(t, 0.5, clamp01(0.5))
	assert.Equal(t, 1.0, clamp01(1.0))
	assert.Equal(t, 1.0, clamp01(1.5))
}

func TestRailYAtPosition(t *testing.T) {
	// Flat shapes ignore position and just return the rail's base height.
	flatBase := 10.0 + physics.MinecartRailYOffset
	assert.InDelta(t, flatBase, railYAtPosition("north_south", 0.5, 0.5, 0, 10, 0), 1e-9)
	assert.InDelta(t, flatBase, railYAtPosition("unknown", 0.5, 0.5, 0, 10, 0), 1e-9)

	// Ascending shapes interpolate within [base, base+1] across the block.
	base := 10.0 + physics.MinecartRailYOffset
	got := railYAtPosition("ascending_south", 0.0, 5.5, 0, 10, 5)
	assert.GreaterOrEqual(t, got, base)
	assert.LessOrEqual(t, got, base+1.0)
}

func TestMinecartRailNudge(t *testing.T) {
	t.Run("no input leaves velocity unchanged", func(t *testing.T) {
		velX, velZ, nudged := minecartRailNudge(1.0, 2.0, 90.0, false, false)
		assert.False(t, nudged)
		assert.Equal(t, 1.0, velX)
		assert.Equal(t, 2.0, velZ)
	})

	t.Run("forward at yaw 0 nudges toward -Z", func(t *testing.T) {
		velX, velZ, nudged := minecartRailNudge(0, 0, 0.0, true, false)
		assert.True(t, nudged)
		assert.InDelta(t, 0.0, velX, 1e-9)
		assert.InDelta(t, physics.MinecartNudgeImpulse, velZ, 1e-9)
	})

	t.Run("backward at yaw 0 nudges the opposite way of forward", func(t *testing.T) {
		_, forwardVelZ, _ := minecartRailNudge(0, 0, 0.0, true, false)
		_, backwardVelZ, nudged := minecartRailNudge(0, 0, 0.0, false, true)
		assert.True(t, nudged)
		assert.InDelta(t, -forwardVelZ, backwardVelZ, 1e-9)
	})

	t.Run("nudge is additive to existing velocity", func(t *testing.T) {
		velX, velZ, nudged := minecartRailNudge(0.5, 0.5, 0.0, true, false)
		assert.True(t, nudged)
		assert.InDelta(t, 0.5, velX, 1e-9)
		assert.InDelta(t, 0.5+physics.MinecartNudgeImpulse, velZ, 1e-9)
	})
}

func TestMinecartOnRailSpeed(t *testing.T) {
	t.Run("flat, no braking, no boost: speed unchanged", func(t *testing.T) {
		got := minecartOnRailSpeed(0.1, false, false, false, false)
		assert.InDelta(t, 0.1, got, 1e-9)
	})

	t.Run("slope reduces uphill speed", func(t *testing.T) {
		got := minecartOnRailSpeed(0.1, true, false, false, false)
		assert.Less(t, got, 0.1)
		assert.InDelta(t, 0.1-physics.MinecartSlopeGravity, got, 1e-9)
	})

	t.Run("water halves slope gravity", func(t *testing.T) {
		land := minecartOnRailSpeed(0.1, true, false, false, false)
		water := minecartOnRailSpeed(0.1, true, false, false, true)
		assert.Greater(t, water, land, "water's reduced slope gravity should slow the cart less")
	})

	t.Run("unpowered braking zeroes a slow cart", func(t *testing.T) {
		got := minecartOnRailSpeed(physics.MinecartUnpoweredBrakeThreshold-0.001, false, true, false, false)
		assert.Zero(t, got)
	})

	t.Run("unpowered braking halves a fast cart instead of zeroing it", func(t *testing.T) {
		got := minecartOnRailSpeed(0.2, false, true, false, false)
		assert.InDelta(t, 0.1, got, 1e-9)
	})

	t.Run("powered rail boosts a stopped cart forward", func(t *testing.T) {
		got := minecartOnRailSpeed(0.0, false, false, true, false)
		assert.InDelta(t, physics.MinecartPoweredRailBoost, got, 1e-9)
	})

	t.Run("powered rail boost preserves direction", func(t *testing.T) {
		forward := minecartOnRailSpeed(0.1, false, false, true, false)
		backward := minecartOnRailSpeed(-0.1, false, false, true, false)
		assert.Greater(t, forward, 0.1)
		assert.Less(t, backward, -0.1)
	})

	t.Run("speed is capped on land", func(t *testing.T) {
		got := minecartOnRailSpeed(10.0, false, false, false, false)
		assert.InDelta(t, physics.MinecartMaxSpeed, got, 1e-9)
	})

	t.Run("negative speed is capped symmetrically", func(t *testing.T) {
		got := minecartOnRailSpeed(-10.0, false, false, false, false)
		assert.InDelta(t, -physics.MinecartMaxSpeed, got, 1e-9)
	})

	t.Run("water caps speed lower than land", func(t *testing.T) {
		got := minecartOnRailSpeed(10.0, false, false, false, true)
		assert.InDelta(t, physics.MinecartWaterMaxSpeed, got, 1e-9)
	})
}
