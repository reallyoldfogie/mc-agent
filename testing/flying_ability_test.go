package testing

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// FlyingSurvivalSuite is a version-parameterized suite for the single
// survival-mode flying-permission test below: one server per version
// instead of the previous per-test-function StartServer/StopServer
// pattern. Kept separate from FlyingCreativeSuite (testing/flying_test.go)
// because GameMode is server-wide for a VersionWorldSuite - that suite's
// shared GameModeCreative server would grant AllowFlying automatically,
// defeating the entire point of this test, which is that survival mode
// does NOT.
type FlyingSurvivalSuite struct {
	VersionWorldSuite
}

func TestFlyingSurvivalSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &FlyingSurvivalSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestPlayerAbilities_SurvivalDeniesFlying verifies the foundational
// plumbing for PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4
// (Creative/Spectator Flight): the clientbound Abilities packet and the
// player's own game mode (from the Login packet) are correctly parsed and
// tracked, and SetFlying respects the server-granted AllowFlying
// permission - here, that survival mode does NOT grant it. The equivalent
// creative-mode scenario (this function's own former "creative mode grants
// AllowFlying" subtest) lives in FlyingCreativeSuite (testing/flying_test.go).
// Equivalent to the original TestPlayerAbilities_SurvivalDeniesFlying.
func (s *FlyingSurvivalSuite) TestPlayerAbilities_SurvivalDeniesFlying() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("FlyingSurvivalBot", "flying_ability_survival")
	require.NoError(t, err, "spawn agent")

	require.Eventually(t, func() bool {
		_, ok := leader.Agent.GetPlayerAbilities()
		return ok
	}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

	gameMode, ok := leader.Agent.GetGameMode()
	require.True(t, ok)
	assert.Equal(t, models.GameModeSurvival, gameMode)

	abilities, _ := leader.Agent.GetPlayerAbilities()
	assert.False(t, abilities.AllowFlying, "survival mode should not grant AllowFlying")

	err = leader.Agent.SetFlying(context.Background(), true)
	assert.Error(t, err, "SetFlying(true) should be refused without server-granted AllowFlying")
}
