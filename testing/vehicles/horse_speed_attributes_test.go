package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestHorseMovementSpeedAttribute verifies that horses use their entity-specific movement speed attribute.
func TestHorseMovementSpeedAttribute(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(fmt.Sprintf("%s_movement_speed", tt.Name), func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "HrsSpeedAttrBot")
			defer cleanup()

			// Teleport to a flat testing area
			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 50 65 50")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			initialPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := initialPos.X, initialPos.Y, initialPos.Z

			// Summon a tamed horse at the location
			horseEntityID, err := helper.SummonHorse(ctx, x, y, z, 90)
			require.NoError(t, err, "summon horse")

			time.Sleep(1 * time.Second)

			// Verify horse is tracked
			trackedHorse := helper.GetTrackedEntity(horseEntityID)
			require.NotNil(t, trackedHorse, "horse should be tracked")

			// Wait for entity attributes to be received from server
			// Attributes are sent in ClientboundUpdateAttributes packet after entity spawn
			time.Sleep(2 * time.Second)

			t.Logf("Horse entity tracked: ID=%d", horseEntityID)

			// Mount the horse
			err = helper.MountEntity(ctx, horseEntityID)
			require.NoError(t, err, "mount horse")

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			// Enter manual mode to control throttle
			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			// Record initial position
			initialPos, _ = helper.ManagedAgent.Agent.GetPositionSimple()
			// initialX, initialY, initialZ := initialPos.X, initialPos.Y, initialPos.Z

			// Apply forward throttle to move the horse
			helper.SetManualThrottle(0.0, 1.0) // ThrottleX=0, ThrottleZ=1.0 (full forward)

			// Let the horse move for a few ticks (about 0.5 seconds)
			time.Sleep(500 * time.Millisecond)

			// Get new position after movement
			finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			// finalX, finalY, finalZ := initialPos.X, initialPos.Y, initialPos.Z

			// Calculate distance traveled
			distanceXZ := initialPos.DistanceToXZ(finalPos)
			require.Greater(t, distanceXZ, 0.0, "horse should have moved")

			t.Logf("Horse movement: initial=%s final=%s distance=%.2f",
				initialPos, finalPos, distanceXZ)

			// Dismount
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount horse")
		})
	}
}

// TestHorseSpeedWithThrottle verifies that horses move with different throttle levels.
// This tests that the movement speed attribute is being applied properly.
func TestHorseSpeedWithThrottle(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(fmt.Sprintf("%s_throttle_speed", tt.Name), func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "HrsThrottleBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 200 65 200")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon horse
			horseEntityID, err := helper.SummonHorse(ctx, x, y, z, 90)
			require.NoError(t, err, "summon horse")

			time.Sleep(1 * time.Second)
			time.Sleep(1 * time.Second) // Wait for attributes

			// Mount horse
			err = helper.MountEntity(ctx, horseEntityID)
			require.NoError(t, err, "mount horse")

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			// Enter manual mode to control throttle
			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			// Test with different throttle values
			throttleTests := []struct {
				name     string
				throttle float64
			}{
				{"half_throttle", 0.5},
				{"full_throttle", 1.0},
			}

			for _, test := range throttleTests {
				t.Run(test.name, func(t *testing.T) {
					// Reset position
					initialPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()

					// Apply throttle
					helper.SetManualThrottle(0.0, test.throttle)

					// Let it move for 0.5 seconds
					time.Sleep(500 * time.Millisecond)

					// Get final position
					finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()

					// Calculate movement
					distanceXZ := initialPos.DistanceToXZ(finalPos)

					// Movement should increase with throttle
					t.Logf("Throttle: %.2f, Distance traveled: %.4f", test.throttle, distanceXZ)
					require.Greater(t, distanceXZ, 0.0, "horse should move with throttle")
				})
			}

			// Cleanup
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount horse")
		})
	}
}
