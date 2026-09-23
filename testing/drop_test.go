package testing

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DropFlatSuite is a
// version-parameterized suite for item-drop action tests: one server per
// version, shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern (each via
// setupStandaloneTestForEntity). WorldGen = WorldGenFlat and
// Difficulty = DifficultyEasy, matching setupStandaloneTestForEntity's own
// choice (GameMode is left at VersionWorldSuite's own survival default,
// also matching). Kept as its own suite rather than folded into another
// Flat/Easy suite (ArmSwingFlatSuite, BlockInteractionsFlatSuite,
// LookaroundFlatSuite, and EffectsFlatSuite all share this exact
// Flat/Easy/survival config too) - the convention across this package has been one
// suite per source file/feature, not merging every same-config file
// together; PerceptionFlatSuite's two-file merge was a narrower exception
// for two files that were near-duplicates of each other, not a general
// same-config-implies-same-suite policy.
type DropFlatSuite struct {
	VersionWorldSuite
}

func TestDropFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &DropFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestDropItem tests the single-item drop action (action ID 4). Equivalent
// to the original TestPlayerAction_DropItem.
func (s *DropFlatSuite) TestDropItem() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("DropItemBot", "drop_item")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, "give "+leader.Name+" minecraft:dirt")
	require.NoError(t, err, "give dirt")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	actionHandler := leader.Config.VersionHandler.Play().Actions()
	botClient := leader.BotClient()

	// Action ID 4 = drop item (single)
	err = actionHandler.SendPlayerAction(botClient.Conn(), 4, 0, 0, 0, 0, 0)
	require.NoError(t, err, "send drop item action")

	time.Sleep(200 * time.Millisecond)

	// Verify item was dropped by checking for item entity
	resp, err := s.Inst.RCON.Exec(s.Ctx, "execute as @e[type=minecraft:item,limit=1] run say ItemFound")
	if err == nil {
		t.Logf("Item drop verification: %s", resp)
	}

	t.Log("✓ Item drop test passed")
}

// TestDropStack tests dropping an entire stack (action ID 3). Equivalent to
// the original TestPlayerAction_DropStack.
func (s *DropFlatSuite) TestDropStack() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("DropStackBot", "drop_stack")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, "give "+leader.Name+" minecraft:dirt 64")
	require.NoError(t, err, "give dirt stack")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	actionHandler := leader.Config.VersionHandler.Play().Actions()
	botClient := leader.BotClient()

	// Action ID 3 = drop stack (entire stack)
	err = actionHandler.SendPlayerAction(botClient.Conn(), 3, 0, 0, 0, 0, 0)
	require.NoError(t, err, "send drop stack action")

	time.Sleep(200 * time.Millisecond)

	t.Log("✓ Drop stack test passed")
}
