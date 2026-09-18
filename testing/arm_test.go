package testing

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ArmSwingFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for arm-swing action tests: one server per
// version, shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern (each via
// setupStandaloneTestForEntity). WorldGen = WorldGenFlat and
// Difficulty = DifficultyEasy, matching setupStandaloneTestForEntity's own
// fixed choice.
type ArmSwingFlatSuite struct {
	VersionWorldSuite
}

func TestArmSwingFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ArmSwingFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestMainHand tests arm swing animation in main hand. Equivalent to the
// pre-Phase-1 TestArmSwing_MainHand.
func (s *ArmSwingFlatSuite) TestMainHand() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ArmSwingMainBot", "action_arm_swing")
	require.NoError(t, err, "spawn agent")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	actionHandler := leader.Config.VersionHandler.Play().Actions()
	botClient := leader.BotClient()

	// Send arm swing (main hand = 0)
	err = actionHandler.SendSwing(botClient.Conn(), 0)
	require.NoError(t, err, "send swing (main hand)")

	// Verify no disconnect occurred
	time.Sleep(100 * time.Millisecond)

	t.Log("✓ Main hand arm swing test passed")
}

// TestOffhand tests arm swing animation in offhand. Equivalent to the
// pre-Phase-1 TestArmSwing_Offhand.
func (s *ArmSwingFlatSuite) TestOffhand() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ArmSwingOffhandBot", "action_arm_swing_offhand")
	require.NoError(t, err, "spawn agent")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	actionHandler := leader.Config.VersionHandler.Play().Actions()
	botClient := leader.BotClient()

	// Send arm swing (offhand = 1)
	err = actionHandler.SendSwing(botClient.Conn(), 1)
	require.NoError(t, err, "send swing (offhand)")

	time.Sleep(100 * time.Millisecond)

	t.Log("✓ Offhand arm swing test passed")
}

// TestRapid tests rapid arm swinging. Equivalent to the pre-Phase-1
// TestArmSwing_Rapid.
func (s *ArmSwingFlatSuite) TestRapid() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ArmSwingRapidBot", "action_arm_swing_rapid")
	require.NoError(t, err, "spawn agent")

	if leader.Config.VersionHandler == nil {
		t.Skip("Version handler not available")
	}

	actionHandler := leader.Config.VersionHandler.Play().Actions()
	botClient := leader.BotClient()

	// Swing rapidly (simulating clicking spam)
	for i := 0; i < 10; i++ {
		hand := models.Hand(int32(i % 2)) // Alternate between main and offhand

		err := actionHandler.SendSwing(botClient.Conn(), hand)
		require.NoError(t, err, "send swing iteration %d", i)

		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)

	t.Log("✓ Rapid arm swing test passed")
}
