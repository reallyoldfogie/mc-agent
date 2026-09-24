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

// NautilusSuite is a version-parameterized suite: one shared flat-world server per
// version instead of one server per test function. Each method spawns its own
// uniquely-named agent in its own working area.
type NautilusSuite struct {
	testingpkg.VersionWorldSuite
}

func TestNautilusSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &NautilusSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestNautilusMounting verifies that the agent can mount and dismount a nautilus.
// Nautilus was added in Minecraft 1.21.11.
func (s *NautilusSuite) TestNautilusMounting() {
	t := s.T()
	// Nautilus entities were added in 1.21.11
	if !isVersionGreaterOrEqual(s.Version, "1.21.11") {
		t.Skipf("Nautilus entity not available in version %s (requires 1.21.11+)", s.Version)
	}

	leader, spawnErr := s.SpawnWorkingAreaAgent("NautilusMountBot", "nautilus_mounting")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create a water area for nautilus
	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("fill %.0f 0 %.0f %.0f 5 %.0f water", helper.AtX(0), helper.AtZ(0), helper.AtX(5), helper.AtZ(5)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Teleport agent into water
	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon nautilus in water
	nautilusID, err := helper.SummonNautilus(ctx, x+2, y, z)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Look at nautilus before mounting (required for mount to work)
	nautilusEntity := helper.GetTrackedEntity(nautilusID)
	require.NotNil(t, nautilusEntity)
	err = helper.ManagedAgent.Agent.LookAt(ctx, nautilusEntity.X, nautilusEntity.Y, nautilusEntity.Z)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Mount
	err = helper.MountEntity(ctx, nautilusID)
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

// TestNautilusSubmergedPosition verifies that agent position tracks nautilus in water.
// Nautilus was added in Minecraft 1.21.11.
func (s *NautilusSuite) TestNautilusSubmergedPosition() {
	t := s.T()
	// Nautilus entities were added in 1.21.11
	if !isVersionGreaterOrEqual(s.Version, "1.21.11") {
		t.Skipf("Nautilus entity not available in version %s (requires 1.21.11+)", s.Version)
	}

	leader, spawnErr := s.SpawnWorkingAreaAgent("NautilusSubmergedBot", "nautilus_submerged_position")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create water area
	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("fill %.0f 0 %.0f %.0f 5 %.0f water", helper.AtX(0), helper.AtZ(0), helper.AtX(5), helper.AtZ(5)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon nautilus
	nautilusID, err := helper.SummonNautilus(ctx, x, y, z)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Look at nautilus before mounting (required for mount to work)
	nautilusEntity := helper.GetTrackedEntity(nautilusID)
	require.NotNil(t, nautilusEntity)
	err = helper.ManagedAgent.Agent.LookAt(ctx, nautilusEntity.X, nautilusEntity.Y, nautilusEntity.Z)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Mount
	err = helper.MountEntity(ctx, nautilusID)
	require.NoError(t, err)

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	// Verify agent position tracks nautilus (they should be at same location)
	agentPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	nautilusEntity = helper.GetTrackedEntity(nautilusID)
	require.NotNil(t, nautilusEntity)

	// Agent should be mounted on nautilus, positions should be very close
	distance := GetDistance(agentPos.X, agentPos.Y, agentPos.Z, nautilusEntity.X, nautilusEntity.Y, nautilusEntity.Z)
	require.Less(t, distance, 5.0, "agent position should track nautilus position while mounted")

	err = helper.DismountEntity()
	require.NoError(t, err)

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}

// TestNautilusMovement verifies that a mounted nautilus responds to throttle inputs with 3D movement.
// Nautilus was added in Minecraft 1.21.11.
func (s *NautilusSuite) TestNautilusMovement() {
	t := s.T()
	// Nautilus entities were added in 1.21.11
	if !isVersionGreaterOrEqual(s.Version, "1.21.11") {
		t.Skipf("Nautilus entity not available in version %s (requires 1.21.11+)", s.Version)
	}

	leader, spawnErr := s.SpawnWorkingAreaAgent("NautilusMoveBot", "nautilus_movement")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create water area for nautilus movement
	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("fill %.0f 0 %.0f %.0f 10 %.0f water", helper.AtX(0), helper.AtZ(0), helper.AtX(20), helper.AtZ(20)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 3 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon and mount nautilus
	nautilusID, err := helper.SummonNautilus(ctx, x, y, z)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Look at nautilus before mounting (required for mount to work)
	nautilusEntity := helper.GetTrackedEntity(nautilusID)
	require.NotNil(t, nautilusEntity)
	err = helper.ManagedAgent.Agent.LookAt(ctx, nautilusEntity.X, nautilusEntity.Y, nautilusEntity.Z)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	err = helper.MountEntity(ctx, nautilusID)
	require.NoError(t, err)

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	// Enter manual mode for controlled movement
	err = helper.EnterManualMode()
	require.NoError(t, err)

	// Move forward in water
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(2 * time.Second)

	// Stop
	helper.SetManualThrottle(0, 0)
	time.Sleep(1 * time.Second)

	finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	displacement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

	// Nautilus should move in water
	require.Greater(t, displacement, 0.5)

	err = helper.ExitManualMode()
	require.NoError(t, err)

	err = helper.DismountEntity()
	require.NoError(t, err)

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}

// TestZombieNautilusMounting verifies mounting zombie nautilus variant.
// Zombie nautilus was added in Minecraft 1.21.11.
func (s *NautilusSuite) TestZombieNautilusMounting() {
	t := s.T()
	// Zombie nautilus entities were added in 1.21.11
	if !isVersionGreaterOrEqual(s.Version, "1.21.11") {
		t.Skipf("Zombie nautilus entity not available in version %s (requires 1.21.11+)", s.Version)
	}

	leader, spawnErr := s.SpawnWorkingAreaAgent("ZombieNautilusMountBot", "zombie_nautilus_mounting")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create water area at ground level
	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("fill %.0f 0 %.0f %.0f 5 %.0f water", helper.AtX(0), helper.AtZ(0), helper.AtX(5), helper.AtZ(5)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 1 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon zombie nautilus
	zombieNautilusID, err := helper.SummonZombieNautilus(ctx, x, y, z)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Look at zombie nautilus before mounting (required for mount to work)
	zombieNautilusEntity := helper.GetTrackedEntity(zombieNautilusID)
	require.NotNil(t, zombieNautilusEntity)
	err = helper.ManagedAgent.Agent.LookAt(ctx, zombieNautilusEntity.X, zombieNautilusEntity.Y, zombieNautilusEntity.Z)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Mount
	err = helper.MountEntity(ctx, zombieNautilusID)
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

// TestZombieNautilusMovement verifies that a mounted zombie nautilus responds to throttle inputs.
// Zombie nautilus was added in Minecraft 1.21.11.
func (s *NautilusSuite) TestZombieNautilusMovement() {
	t := s.T()
	// Zombie nautilus entities were added in 1.21.11
	if !isVersionGreaterOrEqual(s.Version, "1.21.11") {
		t.Skipf("Zombie nautilus entity not available in version %s (requires 1.21.11+)", s.Version)
	}

	leader, spawnErr := s.SpawnWorkingAreaAgent("ZombieNautilusMoveBot", "zombie_nautilus_movement")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create water area for zombie nautilus movement
	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("fill %.0f 0 %.0f %.0f 10 %.0f water", helper.AtX(0), helper.AtZ(0), helper.AtX(20), helper.AtZ(20)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 3 %.1f", helper.AgentName, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon and mount zombie nautilus
	zombieNautilusID, err := helper.SummonZombieNautilus(ctx, x, y, z)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// Look at zombie nautilus before mounting (required for mount to work)
	zombieNautilusEntity := helper.GetTrackedEntity(zombieNautilusID)
	require.NotNil(t, zombieNautilusEntity)
	err = helper.ManagedAgent.Agent.LookAt(ctx, zombieNautilusEntity.X, zombieNautilusEntity.Y, zombieNautilusEntity.Z)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	err = helper.MountEntity(ctx, zombieNautilusID)
	require.NoError(t, err)

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err)

	// Enter manual mode for controlled movement
	err = helper.EnterManualMode()
	require.NoError(t, err)

	// Move forward in water
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(2 * time.Second)

	// Stop
	helper.SetManualThrottle(0, 0)
	time.Sleep(1 * time.Second)

	finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	displacement := GetDistance(finalPos.X, finalPos.Y, finalPos.Z, x, y, z)

	// Zombie nautilus should move in water
	require.Greater(t, displacement, 0.5)

	err = helper.ExitManualMode()
	require.NoError(t, err)

	err = helper.DismountEntity()
	require.NoError(t, err)

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err)
}
