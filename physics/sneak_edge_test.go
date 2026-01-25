package physics

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestState_SneakEdgePrevention tests that sneaking prevents walking off block edges
func TestState_SneakEdgePrevention(t *testing.T) {
	t.Run("Prevents falling off edge while sneaking", func(t *testing.T) {
		world, shapes := createFlatWorld()

		// Clear all blocks first (create void except for our 3x3 platform)
		for x := -10; x <= 10; x++ {
			for z := -10; z <= 10; z++ {
				world.SetBlock(x, 0, z, BlockAir)
			}
		}

		// Create ONLY a 3x3 platform at Y=0
		for x := -1; x <= 1; x++ {
			for z := -1; z <= 1; z++ {
				world.SetBlock(x, 0, z, BlockStone)
			}
		}

		// Bot starts in center of platform
		state := NewState(shapes)
		state.SetPositionSimple(models.V3{X: 0.5, Y: 1.0, Z: 0.5})
		state.SetOnGround(true)
		state.SetSneaking(true)

		// Try to walk toward the edge (+X direction)
		input := models.Inputs{
			ThrottleX: 1.0, // Move right (toward X edge)
			ThrottleZ: 0.0,
			Sneak:     true,
		}

		// Run several ticks
		for i := 0; i < 20; i++ {
			state.Tick(input, world)
		}

		// Bot should stay on the platform, not fall off
		// X coordinate should not exceed the platform edge at X=1.5 (block edge + 0.3 player halfwidth)
		if state.Position().X > 1.3 {
			t.Errorf("Bot walked off edge while sneaking: X=%.3f (should be <= 1.3)", state.Position().X)
		}

		// Bot should still be on ground
		if !state.OnGround() {
			t.Error("Bot should still be on ground after edge prevention")
		}

		// Y should still be at platform level
		if state.Position().Y < 0.9 || state.Position().Y > 1.1 {
			t.Errorf("Bot fell or jumped unexpectedly: Y=%.3f (should be ~1.0)", state.Position().Y)
		}
	})

	t.Run("Prevents falling off diagonal edge", func(t *testing.T) {
		world, shapes := createFlatWorld()

		// Clear all blocks first
		for x := -10; x <= 10; x++ {
			for z := -10; z <= 10; z++ {
				world.SetBlock(x, 0, z, BlockAir)
			}
		}

		// Create ONLY a 3x3 platform
		for x := -1; x <= 1; x++ {
			for z := -1; z <= 1; z++ {
				world.SetBlock(x, 0, z, BlockStone)
			}
		}

		state := NewState(shapes)
		state.SetPositionSimple(models.V3{X: 0.5, Y: 1.0, Z: 0.5})
		state.SetOnGround(true)
		state.SetSneaking(true)

		// Try to walk diagonally toward corner (+X, +Z)
		input := models.Inputs{
			ThrottleX: 1.0,
			ThrottleZ: 1.0,
			Sneak:     true,
		}

		// Run several ticks
		for i := 0; i < 20; i++ {
			state.Tick(input, world)
		}

		// Bot should not exceed platform boundaries
		if state.Position().X > 1.3 || state.Position().Z > 1.3 {
			t.Errorf("Bot walked off diagonal edge: (%.3f, %.3f) - should stay within (1.3, 1.3)",
				state.Position().X, state.Position().Z)
		}

		if !state.OnGround() {
			t.Error("Bot should still be on ground")
		}
	})

	t.Run("Allows normal movement when not at edge", func(t *testing.T) {
		world, shapes := createFlatWorld()

		// Create a large platform (10x10)
		for x := -5; x <= 5; x++ {
			for z := -5; z <= 5; z++ {
				world.SetBlock(x, 0, z, BlockStone)
			}
		}

		state := NewState(shapes)
		state.SetPositionSimple(models.V3{X: 0.0, Y: 1.0, Z: 0.0})
		state.SetOnGround(true)
		state.SetSneaking(true)
		initialX := state.Position().X

		// Move right (plenty of room, not near edge)
		input := models.Inputs{
			ThrottleX: 1.0,
			Sneak:     true,
		}

		// Run a few ticks
		for i := 0; i < 10; i++ {
			state.Tick(input, world)
		}

		// Bot should have moved (not blocked by edge prevention)
		if state.Position().X <= initialX+0.1 {
			t.Errorf("Bot didn't move when not near edge: X=%.3f (started at %.3f)",
				state.Position().X, initialX)
		}
	})

	t.Run("Does not prevent movement when not sneaking", func(t *testing.T) {
		world, shapes := createFlatWorld()

		// Create a 3x3 platform
		for x := -1; x <= 1; x++ {
			for z := -1; z <= 1; z++ {
				world.SetBlock(x, 0, z, BlockStone)
			}
		}

		state := NewState(shapes)
		state.SetPositionSimple(models.V3{X: 0.5, Y: 1.0, Z: 0.5})
		state.SetOnGround(true)
		state.SetSneaking(false) // NOT sneaking

		initialX := state.Position().X

		// Try to walk toward edge without sneaking
		input := models.Inputs{
			ThrottleX: 1.0,
			Sneak:     false, // Not sneaking
		}

		// Run several ticks
		for i := 0; i < 10; i++ {
			state.Tick(input, world)
		}

		// Bot SHOULD walk off edge (edge prevention only works when sneaking)
		// Bot should have moved past the initial position
		if state.Position().X <= initialX+0.1 {
			t.Errorf("Bot should be able to walk off edge when not sneaking")
		}

		// Bot should have fallen (not on ground anymore)
		// Note: This depends on gravity simulation - after a few ticks, bot should start falling
	})

	t.Run("Prevents movement on slab edges", func(t *testing.T) {
		world, shapes := createFlatWorld()

		// Clear all blocks first
		for x := -10; x <= 10; x++ {
			for z := -10; z <= 10; z++ {
				world.SetBlock(x, 0, z, BlockAir)
			}
		}

		// Create ONLY a 3x3 platform of slabs (half-height blocks)
		// Slabs at Y=0, so top surface is at Y=0.5
		for x := -1; x <= 1; x++ {
			for z := -1; z <= 1; z++ {
				world.SetBlock(x, 0, z, BlockStone) // Use stone for now (TODO: add slab block type)
			}
		}

		state := NewState(shapes)
		// On top of block
		state.SetPositionSimple(models.V3{X: 0.5, Y: 1.0, Z: 0.5})
		state.SetOnGround(true)
		state.SetSneaking(true)

		// Try to walk toward edge
		input := models.Inputs{
			ThrottleX: 1.0,
			Sneak:     true,
		}

		// Run several ticks
		for i := 0; i < 20; i++ {
			state.Tick(input, world)
		}

		// Bot should stay on slab, not walk off
		// Edge is at X=1.5 (block edge) + halfwidth tolerance
		if state.Position().X > 1.3 {
			t.Errorf("Bot walked off slab edge: X=%.3f (should be <= 1.3)", state.Position().X)
		}

		if !state.OnGround() {
			t.Error("Bot should still be on ground (slab)")
		}
	})
}

// TestHasGroundSupportAt tests the ground support detection helper
func TestHasGroundSupportAt(t *testing.T) {
	t.Run("Detects ground directly below", func(t *testing.T) {
		world, shapes := createFlatWorld()
		world.SetBlock(0, 0, 0, BlockStone)

		state := NewState(shapes)
		pos := models.V3{X: 0.5, Y: 1.0, Z: 0.5}

		if !state.HasGroundSupportAt(pos, world) {
			t.Error("Should detect ground directly below")
		}
	})

	t.Run("Detects no ground when over void", func(t *testing.T) {
		world, shapes := createFlatWorld()

		// Clear all blocks to create void
		for x := -10; x <= 10; x++ {
			for z := -10; z <= 10; z++ {
				world.SetBlock(x, 0, z, BlockAir)
			}
		}

		state := NewState(shapes)
		pos := models.V3{X: 0.5, Y: 1.0, Z: 0.5}

		if state.HasGroundSupportAt(pos, world) {
			t.Error("Should not detect ground when over void")
		}
	})

	t.Run("Detects ground directly below (close distance)", func(t *testing.T) {
		world, shapes := createFlatWorld()

		// Clear area and add single block
		for x := -10; x <= 10; x++ {
			for z := -10; z <= 10; z++ {
				world.SetBlock(x, 0, z, BlockAir)
			}
		}
		world.SetBlock(0, 0, 0, BlockStone)

		state := NewState(shapes)
		// Position just above block (within 0.05 blocks)
		pos := models.V3{X: 0.5, Y: 1.02, Z: 0.5}

		if !state.HasGroundSupportAt(pos, world) {
			t.Error("Should detect ground directly below at close distance")
		}
	})

	t.Run("Does not detect air as ground", func(t *testing.T) {
		world, shapes := createFlatWorld()
		// Set air block (passable) below
		world.SetBlock(0, 0, 0, BlockAir)

		state := NewState(shapes)
		pos := models.V3{X: 0.5, Y: 1.0, Z: 0.5}

		if state.HasGroundSupportAt(pos, world) {
			t.Error("Should not detect air as ground support")
		}
	})
}
