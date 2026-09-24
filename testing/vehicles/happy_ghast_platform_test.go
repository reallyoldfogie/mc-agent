package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// HappyGhastPlatformSuite is a version-parameterized suite: one shared flat-world server per
// version instead of one server per test function. Each method spawns its own
// uniquely-named agent in its own working area.
type HappyGhastPlatformSuite struct {
	testingpkg.VersionWorldSuite
}

func TestHappyGhastPlatformSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &HappyGhastPlatformSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestHappyGhastSupportsStandingPlayer is the end-to-end confirmation of the
// standable-surface feature (PHASE_6_PLAN.md §5/§6.4): once a happy ghast's
// pilot dismounts, HappyGhastEntity's stillTimeout window makes it
// STAYING_STILL (and therefore, per HappyGhastEntity.isCollidable, a solid
// entity) for a few seconds — long enough that the just-dismounted rider,
// who a generic dismount places on top of the 4x4 hitbox, should be held up
// by the walking-player collision path (physics/state.go's
// computeCollisionYXZWithStandableEntities) rather than falling through.
//
// Observed once via a diagnostic run before writing this assertion (not
// guessed): dismounting from a ghast at y=8.49 places the agent at exactly
// y=12.49 — precisely ghast.Y + HappyGhastHeight, the top surface — and it
// stays there, unmoving, for the full multi-second sampling window.
func (s *HappyGhastPlatformSuite) TestHappyGhastSupportsStandingPlayer() {
	t := s.T()
	if !isVersionGreaterOrEqual(s.Version, happyGhastMinVersion) {
		t.Skipf("Happy ghast not available in version %s (requires %s+)", s.Version, happyGhastMinVersion)
	}

	leader, spawnErr := s.SpawnWorkingAreaAgent("GhastPlatBot", "happy_ghast_supports_standing_player")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 5 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	startX, startY, startZ := pos.X, pos.Y, pos.Z

	ghastID, err := helper.SummonHappyGhast(ctx, startX, startY, startZ, 0)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, helper.MountEntity(ctx, ghastID))
	require.NoError(t, helper.WaitForMounted(ctx, 5*time.Second))
	require.NoError(t, helper.EnableEntityAI(ctx, "minecraft:happy_ghast"))

	// Climb clear of the ground and let vertical velocity settle
	// before dismounting, so the dismount isn't confounded by
	// still-decaying jump momentum (same reasoning as
	// TestHappyGhastHoversWithZeroGravity).
	require.NoError(t, helper.EnterManualMode())
	helper.SetManualJump(true)
	time.Sleep(2 * time.Second)
	helper.SetManualJump(false)
	helper.SetManualThrottle(0, 0)
	time.Sleep(3 * time.Second)

	require.NoError(t, helper.DismountEntity())
	require.NoError(t, helper.WaitForDismounted(ctx, 5*time.Second))

	ghast := helper.GetTrackedEntity(ghastID)
	require.NotNil(t, ghast, "ghast should still be tracked after dismount")
	wantY := ghast.Y + physics.HappyGhastHeight

	const samples = 6
	for i := range samples {
		time.Sleep(300 * time.Millisecond)
		agentPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
		t.Logf("sample %d: agent.Y=%.3f want=%.3f (ghast.Y=%.3f)", i, agentPos.Y, wantY, ghast.Y)
		assert.InDelta(t, wantY, agentPos.Y, 0.5,
			"agent should be resting on top of the standable happy ghast, not falling through it")
	}

	require.NoError(t, helper.ExitManualMode())
}
