package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// waitForVisibleItem polls FindNearestVisibleItem until it finds a match or
// timeout elapses — a freshly-summoned entity needs a moment to reach the
// client and register (see TestLookAround_ListsNearbyEntities's comment).
func waitForVisibleItem(ctx context.Context, agent models.CommandAgent, maxDistance float64, timeout time.Duration) (entityID int32, x, y, z float64, found bool, err error) {
	deadline := time.Now().Add(timeout)
	for {
		entityID, x, y, z, found, err = agent.FindNearestVisibleItem(ctx, maxDistance)
		if err != nil || found {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// waitForInventoryItem polls GetInventoryItems until the given item ID
// appears anywhere in the bot's inventory or timeout elapses.
func waitForInventoryItem(env *StandaloneTestEnv, itemID string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		items, err := GetInventoryItems(env.Ctx, env.Inst.RCON, env.BotName)
		if err != nil {
			return false, err
		}
		for _, item := range items {
			if item.ID == itemID {
				return true, nil
			}
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// TestLookAround_ListsNearbyEntities tests FindAllVisibleEntitiesInSphere:
// summon a dropped item near the bot and verify it shows up in the visible
// entity list with the expected (namespace-stripped, see
// models.VisibleEntityInfo) type name.
func TestLookAround_ListsNearbyEntities(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "look_around", tt.MCVersion)
			defer env.Cancel()

			ctx := context.Background()

			itemPos := models.V3{X: env.ContainerPos.X + 2, Y: env.ContainerPos.Y, Z: env.ContainerPos.Z}
			summonCmd := fmt.Sprintf(`summon minecraft:item %.1f %.1f %.1f {Item:{id:"minecraft:stone",Count:1}}`,
				itemPos.X, itemPos.Y, itemPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, summonCmd)
			require.NoError(t, err, "summon item")

			// The summoned entity needs a moment to reach the client (an
			// AddEntity packet, then registration in the entity registry
			// FindAllVisibleEntitiesInSphere resolves type names from) —
			// poll rather than a single fixed-delay check, matching
			// entity_interaction_test.go's ~3s spawn-to-visible convention.
			var entities []models.VisibleEntityInfo
			found := false
			deadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(deadline) {
				entities, err = env.Agent.Agent.FindAllVisibleEntitiesInSphere(ctx, 16)
				require.NoError(t, err, "look around")
				for _, e := range entities {
					if e.TypeName == "item" {
						found = true
						break
					}
				}
				if found {
					break
				}
				time.Sleep(250 * time.Millisecond)
			}
			if !found {
				t.Logf("[DEBUG] tracked entities: %+v", env.Agent.GetTrackedEntities())
			}
			require.True(t, found, "should have seen the dropped item (got: %+v)", entities)

			t.Log("✓ Look around test passed")
		})
	}
}

// TestPickUpNearbyItem_WalksToAndCollects tests the FindNearestVisibleItem +
// MoveToWithChat combination the pickUpNearbyItem chat action is built from
// (see actions/commands.go's PickUpNearbyItem): summon a dropped item within
// range, locate it, walk to it, and verify vanilla's automatic pickup-on-
// approach actually put it in the bot's inventory.
func TestPickUpNearbyItem_WalksToAndCollects(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "pickup_item", tt.MCVersion)
			defer env.Cancel()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			itemPos := models.V3{X: env.ContainerPos.X + 3, Y: env.ContainerPos.Y, Z: env.ContainerPos.Z}
			summonCmd := fmt.Sprintf(`summon minecraft:item %.1f %.1f %.1f {Item:{id:"minecraft:diamond",Count:1}}`,
				itemPos.X, itemPos.Y, itemPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, summonCmd)
			require.NoError(t, err, "summon item")

			entityID, x, y, z, found, err := waitForVisibleItem(ctx, env.Agent.Agent, 8, 6*time.Second)
			require.NoError(t, err, "find nearest item")
			require.True(t, found, "should find the summoned item")
			t.Logf("Found item entity %d at (%.1f,%.1f,%.1f)", entityID, x, y, z)

			err = env.Agent.Agent.MoveToWithChat(ctx, x, y, z)
			require.NoError(t, err, "move to item")

			collected, err := waitForInventoryItem(env, "minecraft:diamond", 5*time.Second)
			require.NoError(t, err, "check inventory")
			require.True(t, collected, "should have picked up the diamond")

			t.Log("✓ Pick up nearby item test passed")
		})
	}
}
