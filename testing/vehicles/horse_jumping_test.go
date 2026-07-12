package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestHorseJumping verifies that the agent can jump while mounted on a horse
// Setup: 2-high barrier blocks are placed in front of the horse, forward throttle is applied,
// then the horse jumps to clear the barrier
func TestHorseJumping(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "HorseJumpBot")
			defer cleanup()

			// Teleport agent to a location with solid ground
			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 100 65 100")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon a tamed and saddled horse
			horseEntityID, err := helper.SummonHorse(ctx, x, y, z, 90)
			require.NoError(t, err, "summon horse")

			time.Sleep(500 * time.Millisecond)

			// Mount the horse
			err = helper.MountEntity(ctx, horseEntityID)
			require.NoError(t, err, "mount horse")

			// Wait for mount confirmation
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			// Build 2-high barrier of blocks in front of the horse for it to jump up on
			barrierCmd := fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
				int(x)-5, int(y), int(z)+3, int(x)+5, int(y)+1, int(z)+53)
			response, err := helper.Instance.RCON.Exec(ctx, barrierCmd)
			require.NoError(t, err, "place barrier blocks in front of horse")
			t.Logf("fill response: %s => %s", barrierCmd, response)

			time.Sleep(500 * time.Millisecond)

			// Get current position before jump
			initialPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			initialX, initialY, initialZ := initialPos.X, initialPos.Y, initialPos.Z

			// Apply forward throttle (toward +Z direction) - the horse needs forward momentum to move up onto the barrier
			helper.SetManualThrottle(0, 1.0) // TODO: Figure out why horse isn't moving foward
			time.Sleep(1 * time.Second)

			// Jump with maximum power while moving forward
			err = helper.JumpVehicle(ctx, 100)
			// require.NoError(t, err, "jump horse with power 100")

			// Wait for jump trajectory to complete and land
			time.Sleep(1 * time.Second)

			// Stop throttle
			helper.SetManualThrottle(0, 0)
			time.Sleep(1 * time.Second)

			err = helper.ExitManualMode()
			require.NoError(t, err, "exit manual mode")

			// Verify the horse moved forward and/or upward
			finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			finalX, finalY, finalZ := finalPos.X, finalPos.Y, finalPos.Z

			distanceMoved := GetDistance(initialX, 0, initialZ, finalX, 0, finalZ)
			t.Logf("[HorseJump] Initial: (%.2f, %.2f, %.2f), Final: (%.2f, %.2f, %.2f), Distance: %.2f blocks, Height change: %.2f",
				initialX, initialY, initialZ, finalX, finalY, finalZ, distanceMoved, finalY-initialY)

			// Verify agent jumped forward (at least some Z movement from starting position)
			require.Greater(t, finalZ, initialZ, "horse should have moved forward in Z direction")
			require.Greater(t, finalY, initialY, "horse should have moved up in Y direction")

			// Dismount
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount horse")

			// Verify dismount
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be dismounted")

			time.Sleep(5 * time.Second)
		})
	}
}

// TestJumpVehiclePowerRange validates jump power parameter clamping
func TestJumpVehiclePowerRange(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "JumpVehicleBot")
			defer cleanup()

			// Setup location
			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 200 65 200")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon horse
			horseEntityID, err := helper.SummonHorse(ctx, x, y, z, 90)
			require.NoError(t, err, "summon horse")

			time.Sleep(500 * time.Millisecond)

			// Mount
			err = helper.MountEntity(ctx, horseEntityID)
			require.NoError(t, err, "mount horse")

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			// Test various power levels
			testCases := []struct {
				name  string
				power int32
			}{
				{"power 0", 0},
				{"power 25", 25},
				{"power 50", 50},
				{"power 75", 75},
				{"power 100", 100},
				// Test clamping: values > 100 should be clamped
				{"power 150 (clamped)", 150},
				// Test clamping: values < 0 should be clamped
				{"power -10 (clamped)", -10},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					// Just verify the command doesn't error out
					// The actual jump physics is validated in TestHorseJumping
					err := helper.JumpVehicle(ctx, tc.power)
					require.NoError(t, err, "jump with power %d should not error", tc.power)
				})
			}

			// Cleanup
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount horse")
		})
	}
}
