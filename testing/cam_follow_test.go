package testing

import (
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// CamFollowFlatSuite is a
// version-parameterized suite for the cam-follow test: one server per
// version, shared by every test method below (currently one), instead of
// the previous per-test-function StartServer/StopServer pattern. WorldGen
// = WorldGenFlat and Difficulty = DifficultyEasy, matching the
// pre-conversion test's own setupStandaloneTestWithModeAndBlockPlacement
// call exactly (survival GameMode - VersionWorldSuite's own default, left
// unset). Deliberately kept separate from SpectatorNoclipFlatSuite despite
// both being single-function, low-value-alone, spectator/camera-themed
// files (00-plan.md's own prioritization note groups them together as
// future-consolidation candidates): that file needs GameMode = spectator
// server-wide, which would put this test's *main* agent into spectator at
// join too, breaking its whole premise (only the Cam companion switches to
// spectator, via StartCamFollow, while the main agent stays in survival) -
// no evidence adapting around that mismatch is worth the risk versus two
// small separate suites.
type CamFollowFlatSuite struct {
	VersionWorldSuite
}

func TestCamFollowFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &CamFollowFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestCamFollow verifies StartCamFollow end to end against a real server:
// the companion Cam agent switches itself to spectator mode via RCON and
// then continuously teleports (also via RCON) to stay within maxDistance
// blocks of the main agent, including snapping instantly when the main
// agent is itself teleported a long distance rather than walking there.
// Equivalent to the original TestCamFollow.
//
// Called directly on the already-spawned Cam agent (leader.Cam) rather
// than via AgentConfig.EnableCamFollow, so no VersionWorldSuite change is
// needed - StartCamFollow is a public agent capability, identical to what
// a real chat command or CLI flag would invoke.
func (s *CamFollowFlatSuite) TestCamFollow() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("CamFollowBot", "cam_follow")
	require.NoError(t, err, "spawn agent")
	require.NotNil(t, leader.Cam, "cam agent should be spawned by default")

	const maxDistance = 6.0
	require.NoError(t, leader.Cam.Agent.StartCamFollow(s.Ctx, leader.Name, maxDistance), "start cam-follow")

	require.Eventually(t, func() bool {
		gameMode, ok := leader.Cam.Agent.GetGameMode()
		return ok && gameMode == models.GameModeSpectator
	}, 10*time.Second, 200*time.Millisecond, "cam should switch to spectator mode via RCON")

	abilities, ok := leader.Cam.Agent.GetPlayerAbilities()
	require.True(t, ok)
	assert.True(t, abilities.Flying, "spectator mode should start already flying (see PHASE 4.4)")

	assertWithinDistance := func(label string) {
		t.Helper()
		require.Eventually(t, func() bool {
			mainPos, ok := leader.Agent.GetPositionSimple()
			if !ok {
				return false
			}
			camPos, ok := leader.Cam.Agent.GetPositionSimple()
			if !ok {
				return false
			}
			dx := mainPos.X - camPos.X
			dy := mainPos.Y - camPos.Y
			dz := mainPos.Z - camPos.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			t.Logf("%s: main=(%.1f,%.1f,%.1f) cam=(%.1f,%.1f,%.1f) dist=%.2f", label, mainPos.X, mainPos.Y, mainPos.Z, camPos.X, camPos.Y, camPos.Z, dist)
			return dist <= maxDistance
		}, 5*time.Second, 200*time.Millisecond, "%s: cam should be within %.1f blocks of the main agent", label, maxDistance)
	}

	// Baseline: cam should already have closed in on the main agent (its
	// own spawn position could be far from the main agent's).
	assertWithinDistance("initial")

	// Simulate an abrupt teleport (not a walk) - "including teleportation"
	// per the feature's own requirement.
	mainPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	farX, farY, farZ := mainPos.X+150, mainPos.Y, mainPos.Z+150
	tp := s.Inst.RCON.Teleport(s.Ctx, leader.Name, farX, farY, farZ)
	_, err = tp.Exec(s.Ctx)
	require.NoError(t, err, "teleport main agent far away")

	assertWithinDistance("after main agent teleported")
}
