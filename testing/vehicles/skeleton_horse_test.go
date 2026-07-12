package vehicles

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestSkeletonHorseMounting verifies that the agent can mount and dismount a skeleton horse.
func TestSkeletonHorseMounting(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "SkeletonHorseBot")
			defer cleanup()

			// Teleport agent to test location
			_, err := helper.Instance.RCON.Exec(ctx, "teleport SkeletonHorseBot 100 1 100")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon skeleton horse
			skeletonHorseID, err := helper.SummonSkeletonHorse(ctx, x, y, z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Mount
			err = helper.MountEntity(ctx, skeletonHorseID)
			require.NoError(t, err)

			// Wait for mounted
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Dismount
			err = helper.DismountEntity()
			require.NoError(t, err)

			// Wait for dismounted
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}

// TestSkeletonHorseMovement verifies skeleton horse movement.
func TestSkeletonHorseMovement(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "SkeletonHorseMoveBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport SkeletonHorseMoveBot 200 1 200")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon and mount skeleton horse
			skeletonHorseID, err := helper.SummonSkeletonHorse(ctx, x, y, z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, skeletonHorseID)
			require.NoError(t, err)

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Enter manual mode for controlled movement testing
			err = helper.EnterManualMode()
			require.NoError(t, err)

			// Move forward
			helper.SetManualThrottle(0, 1.0)
			time.Sleep(2 * time.Second)

			// Stop
			helper.SetManualThrottle(0, 0)
			time.Sleep(1 * time.Second)

			finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			displacement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

			// Skeleton horse should move (similar speed to regular horse ~0.2)
			require.Greater(t, displacement, 1.0)

			err = helper.ExitManualMode()
			require.NoError(t, err)

			err = helper.DismountEntity()
			require.NoError(t, err)

			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}

// TestZombieHorseMounting verifies zombie horse mounting.
func TestZombieHorseMounting(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "ZombieHorseBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport ZombieHorseBot 300 1 300")
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon zombie horse
			zombieHorseID, err := helper.SummonZombieHorse(ctx, x, y, z)
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			// Mount
			err = helper.MountEntity(ctx, zombieHorseID)
			require.NoError(t, err)

			// Wait for mounted
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err)

			// Dismount
			err = helper.DismountEntity()
			require.NoError(t, err)

			// Wait for dismounted
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			require.NoError(t, err)
		})
	}
}
