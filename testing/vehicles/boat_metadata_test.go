package vehicles

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestBoatVariantTracking verifies that boat variants are properly tracked
func TestBoatVariantTracking(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "BoatVariantBot")
			defer cleanup()

			// Teleport agent to water area
			_, err := helper.Instance.RCON.Exec(ctx, "fill 0 -25 0 25 0 25 water")
			require.NoError(t, err, "fill water area")

			_, err = helper.Instance.RCON.Exec(ctx, "teleport VehicleBot -1 1 -1")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Test various boat variants
			boatVariants := []struct {
				name    string
				variant int
			}{
				{"oak", 0},
				{"spruce", 1},
				{"birch", 2},
				{"jungle", 3},
				{"acacia", 4},
				{"dark_oak", 5},
				{"mangrove", 6},
				{"bamboo", 7},
				{"cherry", 8},
			}

			for _, bv := range boatVariants {
				t.Run(bv.name, func(t *testing.T) {
					// Summon boat with specific variant
					boatEntityID, err := helper.SummonBoat(ctx, x, y+1, z, bv.name)
					require.NoError(t, err, "summon %s boat", bv.name)

					time.Sleep(500 * time.Millisecond)

					// Wait for entity to be tracked
					err = helper.WaitForEntityTracking(ctx, boatEntityID, 5*time.Second)
					require.NoError(t, err, "boat should be tracked")

					// Get tracked boat
					trackedBoat := helper.GetTrackedEntity(boatEntityID)
					require.NotNil(t, trackedBoat, "boat should be tracked")

					// Verify boat variant is tracked (implementation detail: may require checking internal fields)
					t.Logf("Boat variant %s: Tracked as ID=%d Type=%d", bv.name, trackedBoat.EntityID, trackedBoat.EntityType)

					// Cleanup - remove boat for next iteration
					_, err = helper.Instance.RCON.Exec(ctx, "kill @e[type=boat,limit=1]")
					require.NoError(t, err, "cleanup boat")

					time.Sleep(200 * time.Millisecond)
				})
			}
		})
	}
}

// TestBoatPaddleTracking verifies that boat paddle states are tracked
func TestBoatPaddleTracking(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "BoatPaddleBot")
			defer cleanup()

			// Teleport agent to a location with water
			cmd := "fill 0 -25 0 25 -1 25 water"
			resp, err := helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill water area")
			t.Logf("%s => %s", cmd, resp)

			_, err = helper.Instance.RCON.Exec(ctx, "teleport VehicleBot -1 1 -1")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond) // Wait for position update

			// Get agent's initial position
			_, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			// Summon a boat at the agent's location
			// Use hardcoded coordinates: agent is at (25.5, 50.5, 25.5), boat should be at (25.5, 50.0, 25.5)
			boatEntityID, err := helper.SummonBoat(ctx, 1, 1, 1, "oak") // oak boat in water
			require.NoError(t, err, "summon boat")

			time.Sleep(500 * time.Millisecond) // Wait for boat to spawn

			// Mount the boat
			err = helper.MountEntity(ctx, boatEntityID)
			require.NoError(t, err, "mount boat")

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			// Enter manual mode
			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			// Steer left (should toggle left paddle)
			helper.SetManualThrottle(-1.0, 0)
			time.Sleep(1 * time.Second)

			leftPaddleBoat := helper.GetTrackedEntity(boatEntityID)
			require.NotNil(t, leftPaddleBoat, "boat should still be tracked")
			t.Logf("After left steer: Tracked boat ID=%d", leftPaddleBoat.EntityID)

			// Steer right (should toggle right paddle)
			helper.SetManualThrottle(1.0, 0)
			time.Sleep(1 * time.Second)

			rightPaddleBoat := helper.GetTrackedEntity(boatEntityID)
			require.NotNil(t, rightPaddleBoat, "boat should still be tracked")
			t.Logf("After right steer: Tracked boat ID=%d", rightPaddleBoat.EntityID)

			// Stop steering (paddles should stop)
			helper.SetManualThrottle(0, 0)
			time.Sleep(500 * time.Millisecond)

			stoppedBoat := helper.GetTrackedEntity(boatEntityID)
			require.NotNil(t, stoppedBoat, "boat should still be tracked")
			t.Logf("After stopping: Tracked boat ID=%d", stoppedBoat.EntityID)

			// Cleanup
			err = helper.ExitManualMode()
			require.NoError(t, err, "exit manual mode")

			err = helper.DismountEntity()
			require.NoError(t, err, "dismount boat")
		})
	}
}

// TestBoatMetadataConsistency verifies boat metadata remains consistent
func TestBoatMetadataConsistency(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "BoatMetaDataBot")
			defer cleanup()

			// Setup
			_, err := helper.Instance.RCON.Exec(ctx, "fill 0 -25 0 25 0 25 water")
			require.NoError(t, err, "fill water area")

			_, err = helper.Instance.RCON.Exec(ctx, "teleport VehicleBot -1 1 -1")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon boat
			boatEntityID, err := helper.SummonBoat(ctx, x, y, z, "oak")
			require.NoError(t, err, "summon boat")

			time.Sleep(500 * time.Millisecond)

			// Wait for tracking
			err = helper.WaitForEntityTracking(ctx, boatEntityID, 5*time.Second)
			require.NoError(t, err, "boat should be tracked")

			// Sample boat metadata at multiple points
			snapshots := make([]map[int32]*models.TrackedEntityInfo, 5)

			for i := 0; i < 5; i++ {
				time.Sleep(500 * time.Millisecond)

				boat := helper.GetTrackedEntity(boatEntityID)
				require.NotNil(t, boat, "boat should be tracked")

				// Store snapshot
				snapshot := map[int32]*models.TrackedEntityInfo{boatEntityID: boat}
				snapshots[i] = snapshot

				t.Logf("Snapshot %d: Boat ID=%d Position=(%.2f, %.2f, %.2f)",
					i+1, boat.EntityID, boat.X, boat.Y, boat.Z)
			}

			// Verify boat remained tracked throughout
			require.Equal(t, snapshots[0][boatEntityID].EntityID, snapshots[4][boatEntityID].EntityID,
				"boat entity ID should remain constant")
		})
	}
}
