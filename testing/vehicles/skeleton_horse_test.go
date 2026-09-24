package vehicles

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SkeletonHorseSuite is a version-parameterized suite: one shared flat-world server per
// version instead of one server per test function. Each method spawns its own
// uniquely-named agent in its own working area.
type SkeletonHorseSuite struct {
	testingpkg.VersionWorldSuite
}

func TestSkeletonHorseSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &SkeletonHorseSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestSkeletonHorseMounting verifies that the agent can mount and dismount a skeleton horse.
func (s *SkeletonHorseSuite) TestSkeletonHorseMounting() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("SkeletonHorseBot", "skeleton_horse_mounting")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Teleport agent to test location
	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon skeleton horse
	skeletonHorseID, err := helper.SummonSkeletonHorse(ctx, x, y, z)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Mount
	err = helper.MountEntity(ctx, skeletonHorseID)
	require.NoError(t, err)

	// Wait for mounted
	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	// Dismount
	err = helper.DismountEntity()
	require.NoError(t, err)

	// Wait for dismounted
	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}

// TestSkeletonHorseMovement verifies skeleton horse movement.
func (s *SkeletonHorseSuite) TestSkeletonHorseMovement() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("SkeletonHorseMoveBot", "skeleton_horse_movement")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon and mount skeleton horse
	skeletonHorseID, err := helper.SummonSkeletonHorse(ctx, x, y, z)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	err = helper.MountEntity(ctx, skeletonHorseID)
	require.NoError(t, err)

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	// Enter manual mode for controlled movement testing
	err = helper.EnterManualMode()
	require.NoError(t, err)

	// Move forward
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(2 * time.Second)

	// Stop
	helper.SetManualThrottle(0, 0)
	time.Sleep(1 * time.Second)

	finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	displacement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

	// Skeleton horse should move (similar speed to regular horse ~0.2)
	require.Greater(t, displacement, 1.0)

	err = helper.ExitManualMode()
	require.NoError(t, err)

	err = helper.DismountEntity()
	require.NoError(t, err)

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}

// TestZombieHorseMounting verifies zombie horse mounting.
func (s *SkeletonHorseSuite) TestZombieHorseMounting() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("ZombieHorseBot", "zombie_horse_mounting")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon zombie horse
	zombieHorseID, err := helper.SummonZombieHorse(ctx, x, y, z)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Mount
	err = helper.MountEntity(ctx, zombieHorseID)
	require.NoError(t, err)

	// Wait for mounted
	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	// Dismount
	err = helper.DismountEntity()
	require.NoError(t, err)

	// Wait for dismounted
	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}
