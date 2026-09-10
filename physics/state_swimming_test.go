package physics

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createWaterPool sets up a world with a stone floor at Y=0 and a water pool from Y=1 to Y=poolDepth.
// The pool spans from X/Z -5 to +5.
func createWaterPool(poolDepth int) (*mockWorld, *mockShapeProvider) {
	world := newMockWorld()
	shapes := newMockShapeProvider()

	shapes.SetPassable(BlockAir, true)
	shapes.SetPassable(BlockWater, true)
	shapes.SetPassable(BlockStone, false)

	// Stone floor at Y=0
	for x := -10; x <= 10; x++ {
		for z := -10; z <= 10; z++ {
			world.SetBlock(x, 0, z, BlockStone)
		}
	}

	// Water from Y=1 up to poolDepth
	for y := 1; y <= poolDepth; y++ {
		for x := -5; x <= 5; x++ {
			for z := -5; z <= 5; z++ {
				world.SetBlock(x, y, z, BlockWater)
			}
		}
	}

	return world, shapes
}

func TestSwimming_DetectionHeadInWater(t *testing.T) {
	world, shapes := createWaterPool(5)
	physicsState := NewState(shapes, nil)

	// Place player deep in water: feet at Y=2, head at Y=2+eyeHeight (~3.62)
	physicsState.SetPosition(models.V3{X: 0.5, Y: 2.0, Z: 0.5}, 0, 0, false)
	physicsState.SetVelocity(models.V3{})

	// One tick to detect water
	err := physicsState.Tick(Inputs{}, world)
	require.NoError(t, err)

	assert.True(t, physicsState.IsSwimming(), "expected IsSwimming when head is in water")
	assert.True(t, physicsState.IsInWater(), "expected IsInWater when submerged")
}

func TestSwimming_DetectionFeetOnlyInWater(t *testing.T) {
	// Pool only 1 block deep: water at Y=1, head is above at Y=1+eyeHeight (~2.62) which is air
	world, shapes := createWaterPool(1)
	physicsState := NewState(shapes, nil)

	// Place player with feet in water at Y=1
	physicsState.SetPosition(models.V3{X: 0.5, Y: 1.0, Z: 0.5}, 0, 0, false)
	physicsState.SetVelocity(models.V3{})

	err := physicsState.Tick(Inputs{}, world)
	require.NoError(t, err)

	assert.False(t, physicsState.IsSwimming(), "head is above water, should not be swimming")
	assert.True(t, physicsState.IsInWater(), "feet are in water, should be IsInWater")
}

func TestSwimming_NotInWater(t *testing.T) {
	world, shapes := createWaterPool(5)
	physicsState := NewState(shapes, nil)

	// Place player on ground above water (Y=6 is air since pool depth is 5)
	physicsState.SetPosition(models.V3{X: 0.5, Y: 6.0, Z: 0.5}, 0, 0, true)
	physicsState.SetVelocity(models.V3{})

	err := physicsState.Tick(Inputs{}, world)
	require.NoError(t, err)

	assert.False(t, physicsState.IsSwimming())
	assert.False(t, physicsState.IsInWater())
}

func TestSwimming_SwimUp(t *testing.T) {
	world, shapes := createWaterPool(10)
	physicsState := NewState(shapes, nil)

	// Place player deep in water
	physicsState.SetPosition(models.V3{X: 0.5, Y: 3.0, Z: 0.5}, 0, 0, false)
	physicsState.SetVelocity(models.V3{})

	startY := physicsState.Position().Y

	// Hold jump for several ticks to swim up
	for range 20 {
		err := physicsState.Tick(Inputs{Jump: true}, world)
		require.NoError(t, err)
	}

	endY := physicsState.Position().Y
	assert.Greater(t, endY, startY, "player should have moved up by swimming")
}

func TestSwimming_SwimDown(t *testing.T) {
	world, shapes := createWaterPool(10)
	physicsState := NewState(shapes, nil)

	// Place player near top of water
	physicsState.SetPosition(models.V3{X: 0.5, Y: 8.0, Z: 0.5}, 0, 0, false)
	physicsState.SetVelocity(models.V3{})

	startY := physicsState.Position().Y

	// Hold sneak for several ticks to swim down
	for range 20 {
		err := physicsState.Tick(Inputs{Sneak: true}, world)
		require.NoError(t, err)
	}

	endY := physicsState.Position().Y
	assert.Less(t, endY, startY, "player should have moved down by swimming")
}

func TestSwimming_FallDistanceResetsInWater(t *testing.T) {
	world, shapes := createWaterPool(5)
	physicsState := NewState(shapes, nil)

	// Place player above water with accumulated fall distance
	physicsState.SetPosition(models.V3{X: 0.5, Y: 10.0, Z: 0.5}, 0, 0, false)
	physicsState.SetVelocity(models.V3{Y: -0.5})
	physicsState.SetFallDistance(5.0) // Simulate having fallen 5 blocks already

	require.Equal(t, 5.0, physicsState.FallDistance())

	// Tick once while still in air (Y=10 is above pool depth 5)
	err := physicsState.Tick(Inputs{}, world)
	require.NoError(t, err)

	// Still above water, fall distance should be increasing
	assert.Greater(t, physicsState.FallDistance(), 0.0, "fall distance should accumulate in air")

	// Now teleport into water
	physicsState.SetPosition(models.V3{X: 0.5, Y: 3.0, Z: 0.5}, 0, 0, false)
	physicsState.SetFallDistance(5.0)

	err = physicsState.Tick(Inputs{}, world)
	require.NoError(t, err)

	assert.Equal(t, 0.0, physicsState.FallDistance(), "fall distance should reset in water")
}

func TestSwimming_JumpOnGroundStillWorks(t *testing.T) {
	world, shapes := createWaterPool(5)
	physicsState := NewState(shapes, nil)

	// Place player on ground OUTSIDE the water pool (X=8 is outside -5 to +5 range)
	physicsState.SetPosition(models.V3{X: 8.0, Y: 1.0, Z: 0.5}, 0, 0, true)
	physicsState.SetVelocity(models.V3{})

	// Advance past the jump cooldown (MinJumpTicks=14) so jump input is accepted
	for range 15 {
		err := physicsState.Tick(Inputs{}, world)
		require.NoError(t, err)
	}

	// Ensure still on ground before jumping
	physicsState.SetPosition(models.V3{X: 8.0, Y: 1.0, Z: 0.5}, 0, 0, true)
	physicsState.SetVelocity(models.V3{})

	err := physicsState.Tick(Inputs{Jump: true}, world)
	require.NoError(t, err)

	// Velocity should be jump velocity (0.42), not swim velocity (0.04)
	velY := physicsState.Velocity().Y
	assert.Greater(t, velY, 0.1, "jump on ground should produce significant upward velocity, got %.4f", velY)
}

func TestSwimming_NoJumpWithoutGroundOutsideWater(t *testing.T) {
	world, shapes := createWaterPool(5)
	physicsState := NewState(shapes, nil)

	// Place player in air outside the water pool
	physicsState.SetPosition(models.V3{X: 8.0, Y: 5.0, Z: 0.5}, 0, 0, false)
	physicsState.SetVelocity(models.V3{})

	err := physicsState.Tick(Inputs{Jump: true}, world)
	require.NoError(t, err)

	// Not in water and not on ground — jump should not fire
	velY := physicsState.Velocity().Y
	assert.LessOrEqual(t, velY, 0.0, "should not be able to jump in mid-air outside water")
}

func TestSwimming_SwimUpReachesSurface(t *testing.T) {
	world, shapes := createWaterPool(5)
	physicsState := NewState(shapes, nil)

	// Place player at bottom of water pool
	physicsState.SetPosition(models.V3{X: 0.5, Y: 1.5, Z: 0.5}, 0, 0, false)
	physicsState.SetVelocity(models.V3{})

	// Swim up for enough ticks to reach the surface
	for range 200 {
		err := physicsState.Tick(Inputs{Jump: true}, world)
		require.NoError(t, err)

		// Once above the pool, stop
		if physicsState.Position().Y > 5.5 {
			break
		}
	}

	assert.Greater(t, physicsState.Position().Y, 4.0,
		"player should have reached near the water surface (pool top Y=5)")
}
