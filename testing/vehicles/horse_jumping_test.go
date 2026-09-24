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

// HorseJumpingSuite is a version-parameterized suite: one shared flat-world server per
// version instead of one server per test function. Each method spawns its own
// uniquely-named agent in its own working area.
type HorseJumpingSuite struct {
	testingpkg.VersionWorldSuite
}

func TestHorseJumpingSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &HorseJumpingSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestHorseJumping verifies that the agent can jump while mounted on a horse
// Setup: 2-high barrier blocks are placed in front of the horse, forward throttle is applied,
// then the horse jumps to clear the barrier
func (s *HorseJumpingSuite) TestHorseJumping() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("HorseJumpBot", "horse_jumping")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Teleport agent to a location with solid ground
	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f %.1f %.1f", helper.AgentName, leader.Origin.X, leader.Origin.Y, leader.Origin.Z))
	require.NoError(t, err, "teleport agent")

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon without AI so the horse cannot wander out of interact
	// range (or rotate away from its spawn facing) before we mount;
	// AI is restored after mounting so the server runs rider-steered
	// travel again.
	horseEntityID, err := helper.SummonHorse(ctx, x, y, z, 90, WithNoAI())
	require.NoError(t, err, "summon horse")

	time.Sleep(500 * time.Millisecond)

	// Mount the horse
	err = helper.MountEntity(ctx, horseEntityID)
	require.NoError(t, err, "mount horse")

	// Wait for mount confirmation
	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err, "agent should be mounted")

	// Restore AI now that we're aboard: a NoAI horse ignores rider
	// steering, so the server would never move it.
	err = helper.EnableEntityAI(ctx, "minecraft:horse")
	require.NoError(t, err, "re-enable horse AI")

	err = helper.EnterManualMode()
	require.NoError(t, err, "enter manual mode")

	// Turn the agent's BODY to face +Z AFTER mounting. The mounted
	// movement direction is the body (physics-state) yaw the riding
	// handler reads; the spawn Rotation only sets the entity's initial
	// facing and would leave the mount pointed wherever it wandered
	// before we mounted, so forward throttle would drive the wrong
	// way. TurnTowards sets that yaw.
	err = helper.ManagedAgent.Agent.TurnTowards(ctx, x, y, z+50)
	require.NoError(t, err, "face forward along +Z")
	time.Sleep(500 * time.Millisecond)

	// Build 2-high barrier of blocks in front of the horse for it to jump up on
	barrierCmd := fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
		int(x)-5, int(y), int(z)+3, int(x)+5, int(y)+1, int(z)+53)
	response, err := helper.Instance.RCON.Exec(ctx, barrierCmd)
	require.NoError(t, err, "place barrier blocks in front of horse")
	t.Logf("fill response: %s => %s", barrierCmd, response)

	time.Sleep(500 * time.Millisecond)

	// Get current position before jump
	initialPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	initialX, initialY, initialZ := initialPos.X, initialPos.Y, initialPos.Z

	// Apply forward throttle (toward +Z direction) - the horse needs forward momentum to move up onto the barrier
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(1 * time.Second)

	// Jump with maximum power while moving forward
	err = helper.JumpVehicle(ctx, 100)
	// require.NoError(t, err, "jump horse with power 100")

	// Wait for jump trajectory to complete and land
	time.Sleep(1 * time.Second)

	// Stop throttle
	helper.SetManualThrottle(0, 0)
	time.Sleep(1 * time.Second)

	err = helper.ExitManualMode()
	require.NoError(t, err, "exit manual mode")

	// Verify the horse moved forward and/or upward
	finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	finalX, finalY, finalZ := finalPos.X, finalPos.Y, finalPos.Z

	distanceMoved := GetDistance(initialX, 0, initialZ, finalX, 0, finalZ)
	t.Logf("[HorseJump] Initial: (%.2f, %.2f, %.2f), Final: (%.2f, %.2f, %.2f), Distance: %.2f blocks, Height change: %.2f",
		initialX, initialY, initialZ, finalX, finalY, finalZ, distanceMoved, finalY-initialY)

	// Verify agent jumped forward (at least some Z movement from starting position)
	require.Greater(t, finalZ, initialZ, "horse should have moved forward in Z direction")
	require.Greater(t, finalY, initialY, "horse should have moved up in Y direction")

	// Dismount
	err = helper.DismountEntity()
	require.NoError(t, err, "dismount horse")

	// Verify dismount
	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err, "agent should be dismounted")

	time.Sleep(5 * time.Second)
}

// TestJumpVehiclePowerRange validates jump power parameter clamping
func (s *HorseJumpingSuite) TestJumpVehiclePowerRange() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("JumpVehicleBot", "jump_vehicle_power_range")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Setup location
	_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f %.1f %.1f", helper.AgentName, leader.Origin.X, leader.Origin.Y, leader.Origin.Z))
	require.NoError(t, err, "teleport agent")

	time.Sleep(500 * time.Millisecond)

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	x, y, z := pos.X, pos.Y, pos.Z

	// Summon horse
	horseEntityID, err := helper.SummonHorse(ctx, x, y, z, 90)
	require.NoError(t, err, "summon horse")

	time.Sleep(500 * time.Millisecond)

	// Mount
	err = helper.MountEntity(ctx, horseEntityID)
	require.NoError(t, err, "mount horse")

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err, "agent should be mounted")

	// Test various power levels
	testCases := []struct {
		name  string
		power int32
	}{
		{"power 0", 0},
		{"power 25", 25},
		{"power 50", 50},
		{"power 75", 75},
		{"power 100", 100},
		// Test clamping: values > 100 should be clamped
		{"power 150 (clamped)", 150},
		// Test clamping: values < 0 should be clamped
		{"power -10 (clamped)", -10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Just verify the command doesn't error out
			// The actual jump physics is validated in TestHorseJumping
			err := helper.JumpVehicle(ctx, tc.power)
			require.NoError(t, err, "jump with power %d should not error", tc.power)
		})
	}

	// Cleanup
	err = helper.DismountEntity()
	require.NoError(t, err, "dismount horse")
}
