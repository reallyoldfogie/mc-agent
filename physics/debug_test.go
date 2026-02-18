package physics

import (
	"fmt"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Debug test to understand ground collision
func TestDebug_GroundCollision(t *testing.T) {
	world := newMockWorld()
	shapes := newMockShapeProvider()

	// Single block platform at Y=0
	world.SetBlock(0, 0, 0, BlockStone)
	shapes.SetPassable(BlockStone, false)

	state := NewState(shapes)
	state.SetPositionSimple(models.V3{X: 0, Y: 2, Z: 0})
	state.SetVelocity(models.V3{})

	fmt.Printf("\n=== Ground Collision Debug ===\n")
	ID, found := world.GetBlockStatus(0, 0, 0)
	fmt.Printf("Block at (0,0,0): ID=%d Found=%v\n", ID, found)
	fmt.Printf("IsPassable(Stone=%d): %v\n", BlockStone, shapes.IsPassable(BlockStone))

	// Simulate falling
	for i := 0; i < 50; i++ {
		prevY := state.Position().Y
		state.Tick(Inputs{}, world)

		if i < 10 || state.OnGround() {
			fmt.Printf("Tick %2d: Y=%.3f (Δ%.3f) Vel.Y=%.3f onGround=%v\n",
				i, state.Position().Y, state.Position().Y-prevY, state.Velocity().Y, state.OnGround())
		}

		if state.OnGround() {
			fmt.Printf("Landed at Y=%.3f after %d ticks\n", state.Position().Y, i+1)
			break
		}
	}

	if !state.OnGround() {
		t.Errorf("Player never landed: Y=%.3f", state.Position().Y)
	} else {
		// Expected: player feet at Y=1.0 (standing on top of Y=0 block)
		expectedY := 1.0
		if state.Position().Y < expectedY-0.01 || state.Position().Y > expectedY+0.01 {
			t.Errorf("Player at wrong height: Y=%.3f (expected %.3f)", state.Position().Y, expectedY)
		}
	}
}
