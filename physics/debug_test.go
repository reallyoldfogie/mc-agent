package physics

import (
	"fmt"
	"testing"
)

// Debug test to understand ground collision
func TestDebug_GroundCollision(t *testing.T) {
	world := newMockWorld()
	shapes := newMockShapeProvider()

	// Single block platform at Y=0
	world.SetBlock(0, 0, 0, BlockStone)
	shapes.SetPassable(BlockStone, false)

	state := NewState(shapes)
	state.Pos = V3{X: 0, Y: 2, Z: 0}
	state.Vel = V3{}

	fmt.Printf("\n=== Ground Collision Debug ===\n")
	fmt.Printf("Block at (0,0,0): ID=%d\n", world.GetBlockStatus(0, 0, 0))
	fmt.Printf("IsPassable(Stone=%d): %v\n", BlockStone, shapes.IsPassable(BlockStone))

	// Simulate falling
	for i := 0; i < 50; i++ {
		prevY := state.Pos.Y
		state.Tick(Inputs{}, world)

		if i < 10 || state.onGround {
			fmt.Printf("Tick %2d: Y=%.3f (Δ%.3f) Vel.Y=%.3f onGround=%v\n",
				i, state.Pos.Y, state.Pos.Y-prevY, state.Vel.Y, state.onGround)
		}

		if state.onGround {
			fmt.Printf("Landed at Y=%.3f after %d ticks\n", state.Pos.Y, i+1)
			break
		}
	}

	if !state.onGround {
		t.Errorf("Player never landed: Y=%.3f", state.Pos.Y)
	} else {
		// Expected: player feet at Y=1.0 (standing on top of Y=0 block)
		expectedY := 1.0
		if state.Pos.Y < expectedY-0.01 || state.Pos.Y > expectedY+0.01 {
			t.Errorf("Player at wrong height: Y=%.3f (expected %.3f)", state.Pos.Y, expectedY)
		}
	}
}
