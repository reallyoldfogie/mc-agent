package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// waitForVisibleItem polls FindNearestVisibleItem until it finds a match or
// timeout elapses — a freshly-summoned entity needs a moment to reach the
// client and register (see LookaroundFlatSuite.TestListsNearbyEntities's
// comment).
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
// appears anywhere in the bot's inventory or timeout elapses. Takes an
// RCONHelper/botName directly (not *StandaloneTestEnv) so shared-server
// suite methods (VersionWorldSuite's s.Inst.RCON) can call it too, not
// just the pre-Phase-1 per-test-server pattern's env.Inst.RCON.
func waitForInventoryItem(ctx context.Context, rcon testenv.RCONHelper, botName, itemID string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		items, err := GetInventoryItems(ctx, rcon, botName)
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

// LookaroundFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for perception/pickup tests: one server per
// version, shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern (each via
// setupStandaloneTestForEntity). WorldGen = WorldGenFlat and
// Difficulty = DifficultyEasy, matching setupStandaloneTestForEntity's own
// fixed choice.
type LookaroundFlatSuite struct {
	VersionWorldSuite
}

func TestLookaroundFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &LookaroundFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestListsNearbyEntities tests FindAllVisibleEntitiesInSphere: summon a
// dropped item near the bot and verify it shows up in the visible entity
// list with the expected (namespace-stripped, see models.VisibleEntityInfo)
// type name. Equivalent to the pre-Phase-1 TestLookAround_ListsNearbyEntities.
func (s *LookaroundFlatSuite) TestListsNearbyEntities() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("LookaroundBot", "look_around")
	require.NoError(t, err, "spawn agent")

	itemPos := models.V3{X: leader.Origin.X + 2, Y: leader.Origin.Y, Z: leader.Origin.Z}
	summonCmd := fmt.Sprintf(`summon minecraft:item %.1f %.1f %.1f {Item:{id:"minecraft:stone",Count:1}}`,
		itemPos.X, itemPos.Y, itemPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, summonCmd)
	require.NoError(t, err, "summon item")

	// The summoned entity needs a moment to reach the client (an AddEntity
	// packet, then registration in the entity registry
	// FindAllVisibleEntitiesInSphere resolves type names from) - poll
	// rather than a single fixed-delay check, matching
	// entity_interaction_test.go's ~3s spawn-to-visible convention.
	var entities []models.VisibleEntityInfo
	found := false
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		entities, err = leader.Agent.FindAllVisibleEntitiesInSphere(s.Ctx, 16)
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
		t.Logf("[DEBUG] tracked entities: %+v", leader.GetTrackedEntities())
	}
	require.True(t, found, "should have seen the dropped item (got: %+v)", entities)

	t.Log("✓ Look around test passed")
}

// TestPickUpNearbyItemWalksToAndCollects tests the FindNearestVisibleItem +
// MoveToWithChat combination the pickUpNearbyItem chat action is built from
// (see actions/commands.go's PickUpNearbyItem): summon a dropped item within
// range, locate it, walk to it, and verify vanilla's automatic pickup-on-
// approach actually put it in the bot's inventory. Equivalent to the
// pre-Phase-1 TestPickUpNearbyItem_WalksToAndCollects.
func (s *LookaroundFlatSuite) TestPickUpNearbyItemWalksToAndCollects() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("PickupBot", "pickup_item")
	require.NoError(t, err, "spawn agent")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	itemPos := models.V3{X: leader.Origin.X + 3, Y: leader.Origin.Y, Z: leader.Origin.Z}
	summonCmd := fmt.Sprintf(`summon minecraft:item %.1f %.1f %.1f {Item:{id:"minecraft:diamond",Count:1}}`,
		itemPos.X, itemPos.Y, itemPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, summonCmd)
	require.NoError(t, err, "summon item")

	entityID, x, y, z, found, err := waitForVisibleItem(ctx, leader.Agent, 8, 6*time.Second)
	require.NoError(t, err, "find nearest item")
	require.True(t, found, "should find the summoned item")
	t.Logf("Found item entity %d at (%.1f,%.1f,%.1f)", entityID, x, y, z)

	err = leader.Agent.MoveToWithChat(ctx, x, y, z)
	require.NoError(t, err, "move to item")

	collected, err := waitForInventoryItem(ctx, s.Inst.RCON, leader.Name, "minecraft:diamond", 5*time.Second)
	require.NoError(t, err, "check inventory")
	require.True(t, collected, "should have picked up the diamond")

	t.Log("✓ Pick up nearby item test passed")
}
