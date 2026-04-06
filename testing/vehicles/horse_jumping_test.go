package vehicles

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestHorseJumping verifies that the agent can jump while mounted on a horse
func TestHorseJumping(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "HorseJumpBot")
			defer cleanup()

			// Teleport agent to a location with solid ground
			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 100 65 100")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			x, y, z, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			// Summon a tamed and saddled horse
			horseEntityID, err := helper.SummonHorse(ctx, x, y, z)
			require.NoError(t, err, "summon horse")

			time.Sleep(500 * time.Millisecond)

			// Mount the horse
			err = helper.MountEntity(ctx, horseEntityID)
			require.NoError(t, err, "mount horse")

			// Wait for mount confirmation
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			// Test jumping with maximum power
			initialY := y
			err = helper.JumpVehicle(ctx, 100)
			require.NoError(t, err, "jump horse with power 100")

			// Wait a bit for jump to occur
			time.Sleep(1 * time.Second)

			_, jumpY, _, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			// Verify agent jumped upward
			require.Greater(t, jumpY, initialY, "agent should have jumped upward")

			// Test jumping with medium power
			time.Sleep(500 * time.Millisecond)
			err = helper.JumpVehicle(ctx, 50)
			require.NoError(t, err, "jump horse with power 50")

			time.Sleep(500 * time.Millisecond)

			// Test jumping with minimum power
			err = helper.JumpVehicle(ctx, 0)
			require.NoError(t, err, "jump horse with power 0")

			// Dismount
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount horse")

			// Verify dismount
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be dismounted")
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

			x, y, z, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			// Summon horse
			horseEntityID, err := helper.SummonHorse(ctx, x, y, z)
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
