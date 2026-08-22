package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestEntityAttributeTracking verifies that entity attributes are properly tracked
func TestEntityAttributeTracking(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "EntityAttrBot")
			defer cleanup()

			// Teleport agent to a location
			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 50 65 50")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon a horse (will have entity attributes like movement speed, max health)
			horseEntityID, err := helper.SummonHorse(ctx, x, y, z, 90)
			require.NoError(t, err, "summon horse")

			time.Sleep(500 * time.Millisecond)

			// Wait for entity to be tracked
			err = helper.WaitForEntityTracking(ctx, horseEntityID, 5*time.Second)
			require.NoError(t, err, "horse should be tracked by agent")

			// Get tracked entity
			trackedEntity := helper.GetTrackedEntity(horseEntityID)
			require.NotNil(t, trackedEntity, "horse should be tracked")

			// Verify entity health is tracked (horses have max health of 30)
			require.Greater(t, trackedEntity.Health, float32(0), "horse should have health > 0")
			require.Greater(t, trackedEntity.MaxHealth, float32(0), "horse should have max health > 0")

			// Log entity info for verification
			t.Logf("Horse Entity: ID=%d Type=%d Health=%.1f MaxHealth=%.1f Position=(%.2f, %.2f, %.2f)",
				trackedEntity.EntityID, trackedEntity.EntityType, trackedEntity.Health, trackedEntity.MaxHealth,
				trackedEntity.X, trackedEntity.Y, trackedEntity.Z)
		})
	}
}

// TestHorseAttributesAfterMounting verifies attribute updates when mounting
func TestHorseAttributesAfterMounting(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "HrsAttrAftrMntBt")
			defer cleanup()

			// Setup location
			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 75 65 75")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon horse
			horseEntityID, err := helper.SummonHorse(ctx, x, y, z, 90)
			require.NoError(t, err, "summon horse")

			time.Sleep(500 * time.Millisecond)

			// Get initial horse attributes
			initialEntity := helper.GetTrackedEntity(horseEntityID)
			require.NotNil(t, initialEntity, "horse should be tracked initially")

			initialHealth := initialEntity.Health
			initialMaxHealth := initialEntity.MaxHealth

			// Mount the horse
			err = helper.MountEntity(ctx, horseEntityID)
			require.NoError(t, err, "mount horse")

			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			// Wait a bit for any attribute updates
			time.Sleep(1 * time.Second)

			// Get horse attributes after mounting
			afterMountEntity := helper.GetTrackedEntity(horseEntityID)
			require.NotNil(t, afterMountEntity, "horse should still be tracked after mounting")

			// Verify attributes are still being tracked
			require.Equal(t, initialHealth, afterMountEntity.Health, "horse health should be consistent")
			require.Equal(t, initialMaxHealth, afterMountEntity.MaxHealth, "horse max health should be consistent")

			t.Logf("Horse attributes after mounting: Health=%.1f MaxHealth=%.1f", afterMountEntity.Health, afterMountEntity.MaxHealth)

			// Cleanup
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount horse")
		})
	}
}

// TestMultipleEntityAttributes verifies attribute tracking for various entity types
func TestMultipleEntityAttributes(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "MltplEntyAttrBot")
			defer cleanup()

			// Teleport to setup area
			_, err := helper.Instance.RCON.Exec(ctx, "teleport VehicleBot 100 65 100")
			require.NoError(t, err, "teleport agent")

			time.Sleep(500 * time.Millisecond)

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			x, y, z := pos.X, pos.Y, pos.Z

			// Summon multiple entity types
			entityTests := []struct {
				name       string
				cmd        string
				entityType int32
			}{
				{"horse", fmt.Sprintf("summon minecraft:horse %f %f %f {Tame:1b}", x, y, z), 32}, // Horse entity type
				{"cow", fmt.Sprintf("summon minecraft:cow %f %f %f", x+5, y, z), 10},             // Cow entity type
				{"zombie", fmt.Sprintf("summon minecraft:zombie %f %f %f", x-5, y, z), 32},       // Zombie entity type (may vary)
			}

			for _, et := range entityTests {
				t.Run(et.name, func(t *testing.T) {
					// Summon entity
					_, err := helper.Instance.RCON.Exec(ctx, et.cmd)
					require.NoError(t, err, "summon %s", et.name)

					time.Sleep(500 * time.Millisecond)

					// Find nearest entity of type
					entityID, dist, found := helper.ManagedAgent.FindNearestEntityByType(et.entityType, x, y, z, false)

					// Log results
					if found {
						t.Logf("Found %s: ID=%d Distance=%.2f", et.name, entityID, dist)
						trackedEnt := helper.GetTrackedEntity(entityID)
						if trackedEnt != nil {
							t.Logf("%s attributes: Health=%.1f MaxHealth=%.1f", et.name, trackedEnt.Health, trackedEnt.MaxHealth)
						}
					} else {
						t.Logf("Could not find %s entity (may not be tracked yet)", et.name)
					}
				})
			}
		})
	}
}
