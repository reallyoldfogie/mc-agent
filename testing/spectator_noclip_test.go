package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SpectatorNoclipFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the spectator-noclip test: one server
// per version, shared by every test method below (currently one), instead
// of the previous per-test-function StartServer/StopServer pattern.
// WorldGen = WorldGenFlat, Difficulty = DifficultyEasy, and GameMode =
// spectator, all matching the pre-conversion test's own
// setupStandaloneTestWithModeAndBlockPlacement call exactly. Deliberately
// kept separate from CamFollowFlatSuite - see that suite's own doc comment
// for why: this suite's server-wide GameMode = spectator would put
// CamFollowFlatSuite's *main* agent into spectator at join too, breaking
// its premise that only the Cam companion switches to spectator.
type SpectatorNoclipFlatSuite struct {
	VersionWorldSuite
}

func TestSpectatorNoclipFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &SpectatorNoclipFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		s.GameMode = GameModeSpectator
		return s
	})
}

// TestAutoFliesAndClipsThroughWalls verifies PHASE 4.4 step 5 (spectator
// noclip) end to end on a live server: spectator mode starts already
// flying with no toggle needed (GameMode.setAbilities() sets
// `abilities.flying = true` unconditionally for spectator, unlike
// creative's allowFlying-only default - see testing/flying_ability_test.go's
// creative case for the contrast), and a spectator can fly straight
// through a solid wall a normal player would collide with. Equivalent to
// the pre-Phase-1 TestSpectatorAutoFliesAndClipsThroughWalls.
func (s *SpectatorNoclipFlatSuite) TestAutoFliesAndClipsThroughWalls() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("SpectatorNoclipBot", "spectator_noclip")
	require.NoError(t, err, "spawn agent")

	require.Eventually(t, func() bool {
		_, ok := leader.Agent.GetPlayerAbilities()
		return ok
	}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

	gameMode, ok := leader.Agent.GetGameMode()
	require.True(t, ok)
	assert.Equal(t, models.GameModeSpectator, gameMode)

	abilities, _ := leader.Agent.GetPlayerAbilities()
	assert.True(t, abilities.AllowFlying)
	assert.False(t, abilities.CreativeMode, "spectator is not creative mode")
	assert.True(t, abilities.Flying, "spectator should start already flying, unlike creative which needs an explicit toggle")

	startPos := leader.Origin

	// A solid wall of stone directly ahead (+Z), tall and wide enough that
	// going around or over it within the test window isn't plausible - the
	// only way through is noclip.
	wallZ := int(math.Floor(startPos.Z)) + 3
	baseX, baseY := int(math.Floor(startPos.X)), int(math.Floor(startPos.Y))
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:stone",
		baseX-3, baseY-1, wallZ, baseX+3, baseY+5, wallZ))
	require.NoError(t, err, "place wall")

	require.NoError(t, leader.Agent.EnterManualMode())
	defer func() { _ = leader.Agent.ExitManualMode() }()

	require.NoError(t, leader.Agent.SetManualThrottle(0, 1))
	time.Sleep(2 * time.Second)
	require.NoError(t, leader.Agent.SetManualThrottle(0, 0))

	endPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("start=(%.2f,%.2f,%.2f) end=(%.2f,%.2f,%.2f) wallZ=%d", startPos.X, startPos.Y, startPos.Z, endPos.X, endPos.Y, endPos.Z, wallZ)
	assert.Greater(t, endPos.Z, float64(wallZ)+1.0, "spectator should have flown straight through the wall")
}
