package movement

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the riding physics formulas directly, without a live
// Minecraft server, a network connection, or a PhysicsMovementExecutor. They
// guard the arithmetic that the integration tests in testing/vehicles can only
// verify against a running 1.21.x server.

func TestSelectBoatSurface(t *testing.T) {
	tests := []struct {
		name        string
		isWater     bool
		isFlowing   bool
		isSubmerged bool
		blockName   string
		expected    boatSurface
	}{
		{name: "still water", isWater: true, expected: boatSurfaceWater},
		{name: "flowing water", isWater: true, isFlowing: true, expected: boatSurfaceFlowingWater},
		{name: "submerged water", isWater: true, isSubmerged: true, expected: boatSurfaceSubmerged},
		{
			name:      "flowing wins over submerged",
			isWater:   true,
			isFlowing: true, isSubmerged: true,
			expected: boatSurfaceFlowingWater,
		},
		{name: "plain land", blockName: "minecraft:stone", expected: boatSurfaceLand},
		{name: "unknown block is land", blockName: "", expected: boatSurfaceLand},
		{name: "ice", blockName: "minecraft:ice", expected: boatSurfaceIce},
		{name: "packed ice", blockName: "minecraft:packed_ice", expected: boatSurfaceIce},
		{name: "blue ice", blockName: "minecraft:blue_ice", expected: boatSurfaceBlueIce},
		{
			name:      "block name ignored while in water",
			isWater:   true,
			blockName: "minecraft:blue_ice",
			expected:  boatSurfaceWater,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			surface := selectBoatSurface(tt.isWater, tt.isFlowing, tt.isSubmerged, tt.blockName)
			require.Equal(t, tt.expected, surface, "surface mismatch for %q", tt.name)
		})
	}
}

func TestBoatSurfacePhysics(t *testing.T) {
	tests := []struct {
		name               string
		surface            boatSurface
		expectedMultiplier float64
		expectedGravity    float64
	}{
		{
			name:               "water",
			surface:            boatSurfaceWater,
			expectedMultiplier: physics.BoatInWaterVelocityMultiplier,
			expectedGravity:    physics.BoatInWaterGravity,
		},
		{
			name:               "flowing water has near-zero gravity",
			surface:            boatSurfaceFlowingWater,
			expectedMultiplier: physics.BoatUnderFlowingWaterVelocityMultiplier,
			expectedGravity:    physics.BoatUnderFlowingWaterGravity,
		},
		{
			name:               "submerged drags heavily",
			surface:            boatSurfaceSubmerged,
			expectedMultiplier: physics.BoatUnderWaterVelocityMultiplier,
			expectedGravity:    physics.BoatUnderWaterGravity,
		},
		{
			name:               "land",
			surface:            boatSurfaceLand,
			expectedMultiplier: physics.BoatOnLandStandardVelocityMultiplier,
			expectedGravity:    physics.BoatOnLandGravity,
		},
		{
			name:               "ice",
			surface:            boatSurfaceIce,
			expectedMultiplier: physics.BoatOnLandIceVelocityMultiplier,
			expectedGravity:    physics.BoatOnLandGravity,
		},
		{
			name:               "blue ice",
			surface:            boatSurfaceBlueIce,
			expectedMultiplier: physics.BoatOnLandBlueIceVelocityMultiplier,
			expectedGravity:    physics.BoatOnLandGravity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			velocityMultiplier, gravity := boatSurfacePhysics(tt.surface)
			assert.InDelta(t, tt.expectedMultiplier, velocityMultiplier, 1e-9)
			assert.InDelta(t, tt.expectedGravity, gravity, 1e-9)
		})
	}
}

// TestBoatSurfaceOrdering pins the relative drag of the surfaces, which is the
// property riders actually feel: ice glides, land grips, submersion crawls.
func TestBoatSurfaceOrdering(t *testing.T) {
	blueIceDrag, _ := boatSurfacePhysics(boatSurfaceBlueIce)
	iceDrag, _ := boatSurfacePhysics(boatSurfaceIce)
	waterDrag, _ := boatSurfacePhysics(boatSurfaceWater)
	landDrag, _ := boatSurfacePhysics(boatSurfaceLand)
	submergedDrag, _ := boatSurfacePhysics(boatSurfaceSubmerged)

	assert.Greater(t, blueIceDrag, iceDrag, "blue ice should be slipperier than ice")
	assert.Greater(t, iceDrag, waterDrag, "ice should be slipperier than water")
	assert.Greater(t, waterDrag, landDrag, "water should retain more speed than land")
	assert.Greater(t, landDrag, submergedDrag, "submersion should be the highest drag")
}

func TestBoatThrustSpeed(t *testing.T) {
	assert.InDelta(t, 0.0, boatThrustSpeed(false, false), 1e-9)
	assert.InDelta(t, physics.BoatForwardAcceleration, boatThrustSpeed(true, false), 1e-9)
	assert.InDelta(t, -physics.BoatBackwardAcceleration, boatThrustSpeed(false, true), 1e-9)

	// Vanilla applies both deltas when both keys are held; forward dominates.
	both := boatThrustSpeed(true, true)
	assert.InDelta(t, physics.BoatForwardAcceleration-physics.BoatBackwardAcceleration, both, 1e-9)
	assert.Greater(t, both, 0.0)
}

func TestBoatYawAcceleration(t *testing.T) {
	assert.InDelta(t, 0.0, boatYawAcceleration(false, false), 1e-9)
	assert.InDelta(t, -1.0, boatYawAcceleration(true, false), 1e-9)
	assert.InDelta(t, 1.0, boatYawAcceleration(false, true), 1e-9)
	assert.InDelta(t, 0.0, boatYawAcceleration(true, true), 1e-9, "opposing steering should cancel")
}

func TestZeroTinyVelocity(t *testing.T) {
	assert.InDelta(t, 0.0, zeroTinyVelocity(0.0029), 1e-12)
	assert.InDelta(t, 0.0, zeroTinyVelocity(-0.0029), 1e-12)
	assert.InDelta(t, physics.ResetVelocity, zeroTinyVelocity(physics.ResetVelocity), 1e-12,
		"the threshold itself is retained; only strictly-smaller values are zeroed")
	assert.InDelta(t, 0.5, zeroTinyVelocity(0.5), 1e-12)
	assert.InDelta(t, -0.5, zeroTinyVelocity(-0.5), 1e-12)
}

func TestForwardVelocityToWorld(t *testing.T) {
	tests := []struct {
		name       string
		yawDegrees float64
		expectedX  float64
		expectedZ  float64
	}{
		{name: "yaw 0 faces +Z", yawDegrees: 0, expectedX: 0, expectedZ: 1},
		{name: "yaw 90 faces -X", yawDegrees: 90, expectedX: -1, expectedZ: 0},
		{name: "yaw 180 faces -Z", yawDegrees: 180, expectedX: 0, expectedZ: -1},
		{name: "yaw 270 faces +X", yawDegrees: 270, expectedX: 1, expectedZ: 0},
		{name: "negative yaw matches its positive equivalent", yawDegrees: -90, expectedX: 1, expectedZ: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			velX, velZ := forwardVelocityToWorld(1.0, tt.yawDegrees)
			assert.InDelta(t, tt.expectedX, velX, 1e-9)
			assert.InDelta(t, tt.expectedZ, velZ, 1e-9)
		})
	}
}

// TestForwardVelocityToWorldPreservesMagnitude verifies the yaw rotation is a
// pure rotation: it must never change the speed, only its direction.
func TestForwardVelocityToWorldPreservesMagnitude(t *testing.T) {
	const forwardVelocity = 0.37
	for yawDegrees := -360.0; yawDegrees <= 360.0; yawDegrees += 17.0 {
		velX, velZ := forwardVelocityToWorld(forwardVelocity, yawDegrees)
		assert.InDelta(t, forwardVelocity, math.Hypot(velX, velZ), 1e-9,
			"magnitude changed at yaw %.1f", yawDegrees)
	}
}

func TestMountSurfacePhysics(t *testing.T) {
	const (
		waterDrag         = 0.8
		waterGravityDelta = -0.02
	)

	t.Run("water uses the caller-supplied version-gated values", func(t *testing.T) {
		velocityDrag, gravityDelta := mountSurfacePhysics(
			mountSurfaceWater, waterDrag, waterGravityDelta, physics.DefaultSlipperiness)
		assert.InDelta(t, waterDrag, velocityDrag, 1e-9)
		assert.InDelta(t, waterGravityDelta, gravityDelta, 1e-9)
	})

	t.Run("airborne uses air friction and full gravity", func(t *testing.T) {
		velocityDrag, gravityDelta := mountSurfacePhysics(
			mountSurfaceAirborne, waterDrag, waterGravityDelta, physics.DefaultSlipperiness)
		assert.InDelta(t, physics.Inertia, velocityDrag, 1e-9)
		assert.InDelta(t, -physics.Gravity, gravityDelta, 1e-9)
	})

	t.Run("ground derives friction from slipperiness with no gravity", func(t *testing.T) {
		velocityDrag, gravityDelta := mountSurfacePhysics(
			mountSurfaceGround, waterDrag, waterGravityDelta, physics.DefaultSlipperiness)
		assert.InDelta(t, physics.Inertia*physics.DefaultSlipperiness, velocityDrag, 1e-9)
		assert.InDelta(t, 0.0, gravityDelta, 1e-9)
	})

	t.Run("ice retains more speed than a default block", func(t *testing.T) {
		iceDrag, _ := mountSurfacePhysics(mountSurfaceGround, waterDrag, waterGravityDelta, physics.IceSlipperiness)
		stoneDrag, _ := mountSurfacePhysics(mountSurfaceGround, waterDrag, waterGravityDelta, physics.DefaultSlipperiness)
		assert.Greater(t, iceDrag, stoneDrag)
	})
}

func TestMountInputAcceleration(t *testing.T) {
	const groundSpeed = 0.225
	assert.InDelta(t, groundSpeed, mountInputAcceleration(groundSpeed, false), 1e-9)
	assert.InDelta(t, physics.RidingAirborneAcceleration, mountInputAcceleration(groundSpeed, true), 1e-9)
	assert.Less(t, mountInputAcceleration(groundSpeed, true), mountInputAcceleration(groundSpeed, false),
		"air control must be weaker than ground acceleration")
}

func TestMountForwardVelocity(t *testing.T) {
	t.Run("applies drag then adds throttle acceleration", func(t *testing.T) {
		velocity := mountForwardVelocity(0.5, 0.546, 1.0, 0.225)
		assert.InDelta(t, 0.5*0.546+0.225, velocity, 1e-9)
	})

	t.Run("coasts down toward zero with no throttle", func(t *testing.T) {
		velocity := 0.5
		for range 200 {
			velocity = mountForwardVelocity(velocity, 0.546, 0.0, 0.225)
		}
		assert.Zero(t, velocity, "a coasting mount must come to a complete stop")
	})

	t.Run("converges to a stable top speed under constant throttle", func(t *testing.T) {
		const (
			drag         = 0.546
			acceleration = 0.225
		)
		velocity := 0.0
		for range 200 {
			velocity = mountForwardVelocity(velocity, drag, 1.0, acceleration)
		}
		// Geometric series limit: accel / (1 - drag).
		assert.InDelta(t, acceleration/(1-drag), velocity, 1e-6)
	})

	t.Run("reverse throttle produces negative velocity", func(t *testing.T) {
		assert.Negative(t, mountForwardVelocity(0.0, 0.546, -1.0, 0.225))
	})
}

func TestMountVerticalVelocity(t *testing.T) {
	t.Run("grounded mount is unaffected by a zero gravity delta", func(t *testing.T) {
		assert.InDelta(t, 0.0, mountVerticalVelocity(0.0, 0.0, false), 1e-9)
	})

	t.Run("airborne mount accelerates downward with air drag", func(t *testing.T) {
		velocity := mountVerticalVelocity(0.0, -physics.Gravity, true)
		assert.InDelta(t, -physics.Gravity*physics.Drag, velocity, 1e-9)
		assert.Negative(t, velocity)
	})

	t.Run("a fall accelerates monotonically", func(t *testing.T) {
		velocity := 0.0
		previous := math.Inf(1)
		for range 20 {
			velocity = mountVerticalVelocity(velocity, -physics.Gravity, true)
			assert.Less(t, velocity, previous, "each tick should fall faster")
			previous = velocity
		}
	})
}

func TestMountJumpChargeStrength(t *testing.T) {
	tests := []struct {
		name                    string
		chargeTicks             int
		expectedStrengthPercent int
		expectedStrength        float64
	}{
		{name: "no charge is the 0.4 floor", chargeTicks: 0, expectedStrengthPercent: 0, expectedStrength: 0.4},
		{name: "half a ramp", chargeTicks: 5, expectedStrengthPercent: 50, expectedStrength: 0.4 + 0.4*50.0/90.0},
		{name: "tick 9 already saturates the clamp", chargeTicks: 9, expectedStrengthPercent: 90, expectedStrength: 1.0},
		{name: "ramp peaks at tick 10", chargeTicks: 10, expectedStrengthPercent: 100, expectedStrength: 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strengthPercent, strength := mountJumpChargeStrength(tt.chargeTicks)
			assert.Equal(t, tt.expectedStrengthPercent, strengthPercent)
			assert.InDelta(t, tt.expectedStrength, strength, 1e-9)
		})
	}

	t.Run("percent is never above 100", func(t *testing.T) {
		for chargeTicks := range mountJumpChargeCapTicks + 1 {
			strengthPercent, strength := mountJumpChargeStrength(chargeTicks)
			assert.LessOrEqual(t, strengthPercent, 100, "chargeTicks=%d", chargeTicks)
			assert.GreaterOrEqual(t, strengthPercent, 0, "chargeTicks=%d", chargeTicks)
			assert.GreaterOrEqual(t, strength, 0.4, "chargeTicks=%d", chargeTicks)
			assert.LessOrEqual(t, strength, 1.0, "chargeTicks=%d", chargeTicks)
		}
	})

	t.Run("a short hold reaches full strength", func(t *testing.T) {
		// Regression guard: clamping raw ticks instead of running the client
		// ramp made this roughly 10x too slow, so even a 3s hold fell short.
		_, strength := mountJumpChargeStrength(10)
		assert.InDelta(t, 1.0, strength, 1e-9)
	})
}

func TestTravelMidAirSpeedFactor(t *testing.T) {
	const movementSpeed = 0.1

	t.Run("off ground uses the reduced factor and ignores slipperiness", func(t *testing.T) {
		onIce := travelMidAirSpeedFactor(movementSpeed, physics.IceSlipperiness, false)
		onStone := travelMidAirSpeedFactor(movementSpeed, physics.DefaultSlipperiness, false)
		assert.InDelta(t, movementSpeed*travelMidAirOffGroundSpeedFactor, onIce, 1e-9)
		assert.InDelta(t, onStone, onIce, 1e-9)
	})

	t.Run("on ground compensates for slipperiness", func(t *testing.T) {
		slipperinessCubed := physics.DefaultSlipperiness * physics.DefaultSlipperiness * physics.DefaultSlipperiness
		expected := movementSpeed * (travelMidAirGroundSpeedFactor / slipperinessCubed)
		assert.InDelta(t, expected, travelMidAirSpeedFactor(movementSpeed, physics.DefaultSlipperiness, true), 1e-9)
	})

	t.Run("slippery ground needs less acceleration per tick", func(t *testing.T) {
		onIce := travelMidAirSpeedFactor(movementSpeed, physics.IceSlipperiness, true)
		onStone := travelMidAirSpeedFactor(movementSpeed, physics.DefaultSlipperiness, true)
		assert.Less(t, onIce, onStone,
			"the slipperiness compensation shrinks as friction drops")
	})

	t.Run("grounded acceleration exceeds air control", func(t *testing.T) {
		grounded := travelMidAirSpeedFactor(movementSpeed, physics.DefaultSlipperiness, true)
		airborne := travelMidAirSpeedFactor(movementSpeed, airSlipperiness, false)
		assert.Greater(t, grounded, airborne)
	})
}

// travelMidAirTopSpeed runs the vanilla ground loop
//
//	velocity = (velocity + speedFactor) * friction
//
// to convergence and returns the steady-state speed for a given block.
func travelMidAirTopSpeed(movementSpeed, slipperiness float64) float64 {
	speedFactor := travelMidAirSpeedFactor(movementSpeed, slipperiness, true)
	friction := slipperiness * physics.Inertia
	velocity := 0.0
	for range 500 {
		velocity = (velocity + speedFactor) * friction
	}
	return velocity
}

// TestTravelMidAirTopSpeedOnIce pins the end-to-end behaviour of the
// 0.216/slip³ compensation combined with the slip*0.91 friction: above the
// default slipperiness the compensation does not fully cancel out, so the
// steady-state speed keeps climbing. That is why mounts visibly slide faster
// across ice than across stone.
//
// Note the curve is NOT monotonic in slipperiness across the whole range — it
// bottoms out near the 0.6 default and rises again for very low values such as
// honey (0.4). Vanilla slows entities on honey through a separate explicit
// movement-slowdown mechanic, not through slipperiness, so this test only
// asserts the ordering the formula itself guarantees.
func TestTravelMidAirTopSpeedOnIce(t *testing.T) {
	const movementSpeed = 0.1

	onStone := travelMidAirTopSpeed(movementSpeed, physics.DefaultSlipperiness)
	onIce := travelMidAirTopSpeed(movementSpeed, physics.IceSlipperiness)
	onBlueIce := travelMidAirTopSpeed(movementSpeed, physics.BlueIceSlipperiness)

	assert.Less(t, onStone, onIce, "ice should out-run stone")
	assert.Less(t, onIce, onBlueIce, "blue ice should out-run ice")
}

// TestTravelMidAirTopSpeedMatchesClosedForm cross-checks the iterative loop
// against the geometric-series limit of the same recurrence, catching sign or
// ordering mistakes in the accelerate-then-apply-friction sequence.
func TestTravelMidAirTopSpeedMatchesClosedForm(t *testing.T) {
	const movementSpeed = 0.1

	for _, slipperiness := range []float64{
		physics.HoneyBlockSlipperiness,
		physics.DefaultSlipperiness,
		physics.SlimeBlockSlipperiness,
		physics.IceSlipperiness,
	} {
		speedFactor := travelMidAirSpeedFactor(movementSpeed, slipperiness, true)
		friction := slipperiness * physics.Inertia
		// v = (v + a)f converges to a*f/(1-f).
		expected := speedFactor * friction / (1 - friction)
		assert.InDelta(t, expected, travelMidAirTopSpeed(movementSpeed, slipperiness), 1e-6,
			"slipperiness=%.3f", slipperiness)
	}
}

func TestSaddledBoostMultiplier(t *testing.T) {
	const amplitude = physics.PigBoostSinAmplitude

	t.Run("no boost is neutral", func(t *testing.T) {
		assert.InDelta(t, 1.0, saddledBoostMultiplier(false, 50, 100, amplitude), 1e-9)
	})

	t.Run("unknown duration is neutral", func(t *testing.T) {
		assert.InDelta(t, 1.0, saddledBoostMultiplier(true, 50, 0, amplitude), 1e-9)
	})

	t.Run("boost starts and ends neutral", func(t *testing.T) {
		assert.InDelta(t, 1.0, saddledBoostMultiplier(true, 0, 100, amplitude), 1e-9)
		assert.InDelta(t, 1.0, saddledBoostMultiplier(true, 100, 100, amplitude), 1e-9)
	})

	t.Run("boost peaks at the halfway point", func(t *testing.T) {
		assert.InDelta(t, 1.0+amplitude, saddledBoostMultiplier(true, 50, 100, amplitude), 1e-9)
	})

	t.Run("boost never slows the mount down", func(t *testing.T) {
		for boostTicks := range 101 {
			assert.GreaterOrEqual(t, saddledBoostMultiplier(true, boostTicks, 100, amplitude), 1.0,
				"boostTicks=%d", boostTicks)
		}
	})
}

func TestStriderSaddledSpeed(t *testing.T) {
	const attributeSpeed = physics.StriderBaseMovementSpeed

	t.Run("warm strider uses the warm multiplier only", func(t *testing.T) {
		assert.InDelta(t,
			attributeSpeed*physics.StriderWarmSpeedMultiplier,
			striderSaddledSpeed(attributeSpeed, false, 1.0),
			1e-9)
	})

	t.Run("cold strider also carries the suffocating modifier", func(t *testing.T) {
		expected := attributeSpeed *
			(1.0 + physics.StriderSuffocatingModifier) *
			physics.StriderColdSpeedMultiplier
		assert.InDelta(t, expected, striderSaddledSpeed(attributeSpeed, true, 1.0), 1e-9)
	})

	t.Run("lava is meaningfully faster than land", func(t *testing.T) {
		warm := striderSaddledSpeed(attributeSpeed, false, 1.0)
		cold := striderSaddledSpeed(attributeSpeed, true, 1.0)
		require.Positive(t, cold)
		// The integration test TestStriderLavaVsLandSpeed asserts a >1.3 ratio
		// against a live server; the formula alone should clear that bar.
		assert.Greater(t, warm/cold, 1.3, "warm/cold ratio was %.2f", warm/cold)
	})

	t.Run("boost scales the result linearly", func(t *testing.T) {
		base := striderSaddledSpeed(attributeSpeed, false, 1.0)
		boosted := striderSaddledSpeed(attributeSpeed, false, 2.15)
		assert.InDelta(t, base*2.15, boosted, 1e-9)
	})
}
