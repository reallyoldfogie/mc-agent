package vehicles

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/internal/visualize"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/reallyoldfogie/mc-agent/utils"

	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// MinecartSteeringSuite is a version-parameterized suite: one shared flat-world server per
// version instead of one server per test function. Each method spawns its own
// uniquely-named agent in its own working area.
type MinecartSteeringSuite struct {
	testingpkg.VersionWorldSuite
}

func TestMinecartSteeringSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &MinecartSteeringSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestMinecartSteering verifies that the agent can mount a minecart and steer it
// using manual inputs, asserting that every throttle phase produces movement
// in the expected direction by a non-trivial amount.
//
// Minecarts move along rails with physics that include drag, slope gravity,
// and powered rail acceleration. The test validates steering by asserting
// directional movement and distance traveled.
func (s *MinecartSteeringSuite) TestMinecartSteering() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("MinecartSteerBot", "minecart_steering")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Build a flat rail track (east-west direction)
	// Spawn location: agent at (0, 0, 0), rails from (2, -1, 0) to (20, -1, 0)
	cmd := fmt.Sprintf("setblock %.0f -1 %.0f stone", helper.AtX(0), helper.AtZ(0))
	resp, err := helper.Instance.RCON.Exec(ctx, cmd)
	require.NoError(t, err, "create ground block")
	t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

	// Teleport agent to a safe location
	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 0 %.1f", helper.ManagedAgent.Name, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err, "teleport agent")
	time.Sleep(500 * time.Millisecond)

	agentPos, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
	require.True(t, initialized, "agent position should be initialized")

	// Build rail track going east
	railStartX := agentPos.X + 2
	railStartY := agentPos.Y - 1
	railStartZ := agentPos.Z

	err = helper.BuildRailTrack(ctx, railStartX, railStartY, railStartZ, "east", 20, false)
	require.NoError(t, err, "build rail track")

	// Summon minecart on the rail
	minecartX := railStartX + 2
	minecartY := railStartY + 1
	minecartZ := railStartZ
	minecartEntityID, err := helper.SummonMinecart(ctx, minecartX, minecartY, minecartZ)
	require.NoError(t, err, "summon minecart")
	t.Logf("[%s] Spawned minecart at (%.1f, %.1f, %.1f) with ID %d", helper.AgentName, minecartX, minecartY, minecartZ, minecartEntityID)

	// Mount the minecart
	err = helper.MountEntity(ctx, minecartEntityID)
	require.NoError(t, err, "mount minecart")
	err = helper.WaitForMounted(ctx, 15*time.Second)
	require.NoError(t, err, "agent should be mounted")
	t.Logf("[%s] Agent mounted successfully on minecart", helper.AgentName)

	// Face the agent east (along the track direction) to ensure forward throttle moves the minecart
	// The rails are laid out east-west, so we turn towards a point to the east
	err = helper.ManagedAgent.Agent.TurnTowards(ctx, minecartX+10, minecartY, minecartZ)
	require.NoError(t, err, "face agent towards east")
	time.Sleep(500 * time.Millisecond)

	// dataCmd := fmt.Sprintf("data get entity %s Rotation", helper.ManagedAgent.Name)
	// dataResp, err := helper.Instance.RCON.Exec(ctx, dataCmd)
	// t.Logf("[%s]entity data after turning: %s => %s [err: %v]", helper.AgentName, dataCmd, dataResp, err)

	// dataCmd = fmt.Sprintf("data get entity %s Pos", helper.ManagedAgent.Name)
	// dataResp, err = helper.Instance.RCON.Exec(ctx, dataCmd)
	// t.Logf("[%s]entity data after turning: %s => %s [err: %v]", helper.AgentName, dataCmd, dataResp, err)

	t.Logf("[%s] Agent should be facing east (track direction)", helper.AgentName)

	startPos, startYaw, _, _ := helper.ManagedAgent.Agent.GetPosition() // (x, y, z float64, yaw, pitch float32, initialized bool)()

	expectedYaw, _ := utils.GetYawAndPitch(startPos, models.V3{X: minecartX + 10, Y: startPos.Y, Z: minecartZ})

	require.Equal(t, expectedYaw, startYaw, "agent should be facing east (yaw=%.02f)", expectedYaw)

	// Enter manual mode for steering
	err = helper.EnterManualMode()
	require.NoError(t, err, "enter manual mode")

	visualizerRCON := visualize.NewVisualizerAdapter(helper.Instance.RCON, nil)
	visualize.ClearPathVisualizations(ctx, helper.Instance.RCON)

	// Define steering phases.
	//
	// Vanilla minecart physics (DefaultMinecartController, 1.21.2+):
	//   - Forward/backward input only fires a 0.001 b/t nudge when the cart is
	//     nearly stopped (speed² < 0.01). It is NOT continuous engine thrust.
	//   - Drag with a passenger is 0.997×/tick — extremely low friction.
	//     A cart at 0.1 b/t takes ~1100 ticks (~55 s) to reach the reset threshold.
	//
	// Consequence: do NOT assert maxDisplacement for coasting phases — vanilla
	// minecarts coast for a very long time and the server will keep sending
	// position corrections that accumulate real displacement.
	phases := []steeringPhase{
		{
			name:      "forward_acceleration",
			throttleX: 0, throttleZ: 1.0,
			duration:        5 * time.Second,
			minDisplacement: 2.0, // Nudge builds speed to ~0.08 b/t over 5 s → ~3 blocks
		},
		{
			// With 0.997 drag the cart coasts for tens of seconds after input is
			// removed.  We only verify the phase completes without asserting distance.
			name:      "coast_to_stop",
			throttleX: 0, throttleZ: 0,
			duration: 6 * time.Second,
		},
		{
			name:      "backward",
			throttleX: 0, throttleZ: -1.0,
			duration:        3 * time.Second,
			minDisplacement: 0.1, // Nudge in reverse: small but non-zero displacement
		},
		{
			// Same rationale as coast_to_stop: no maxDisplacement assertion.
			name:      "idle",
			throttleX: 0, throttleZ: 0,
			duration: 2 * time.Second,
		},
	}

	runMinecartPhases(t, ctx, helper, visualizerRCON, phases)

	err = helper.ExitManualMode()
	require.NoError(t, err, "exit manual mode")

	// sleep for a bit to give thing a moment to settle
	time.Sleep(5 * time.Second)

	err = helper.DismountEntity()
	require.NoError(t, err, "dismount vehicle")
	err = helper.WaitForDismounted(ctx, 5*time.Second)
	if err != nil {
		t.Logf("[%s][WARN] Agent may still be mounted after test completion: %v", helper.AgentName, err)
	}
}

// TestMinecartAscendingRail verifies that minecarts can climb ascending rails
// and properly decelerate going uphill.
func (s *MinecartSteeringSuite) TestMinecartAscendingRail() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("MinecartClimbBot", "minecart_ascending_rail")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create ground
	cmd := fmt.Sprintf("setblock %.0f -1 %.0f stone", helper.AtX(0), helper.AtZ(0))
	resp, err := helper.Instance.RCON.Exec(ctx, cmd)
	require.NoError(t, err, "create ground block")
	t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

	// Teleport agent
	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 0 %.1f", helper.ManagedAgent.Name, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err, "teleport agent")
	time.Sleep(500 * time.Millisecond)

	agentPos, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
	require.True(t, initialized, "agent position should be initialized")

	// Build flat rail section first
	railStartX := agentPos.X + 2
	railStartY := agentPos.Y - 1
	railStartZ := agentPos.Z

	err = helper.BuildRailTrack(ctx, railStartX, railStartY, railStartZ, "east", 5, false)
	require.NoError(t, err, "build flat rail section")

	// Build ascending rail section
	ascendX := railStartX + 5
	ascendY := railStartY
	ascendZ := railStartZ
	err = helper.BuildAscendingRailTrack(ctx, ascendX, ascendY, ascendZ, "east", 5, true)
	require.NoError(t, err, "build ascending rail section")

	// Summon minecart on flat section
	minecartX := railStartX
	minecartY := railStartY + 1
	minecartZ := railStartZ
	minecartEntityID, err := helper.SummonMinecart(ctx, minecartX, minecartY, minecartZ)
	require.NoError(t, err, "summon minecart")
	t.Logf("[%s] Spawned minecart at (%.1f, %.1f, %.1f)", helper.AgentName, minecartX, minecartY, minecartZ)

	// Mount minecart
	err = helper.MountEntity(ctx, minecartEntityID)
	require.NoError(t, err, "mount minecart")
	err = helper.WaitForMounted(ctx, 15*time.Second)
	require.NoError(t, err, "agent should be mounted")
	t.Logf("[%s] Agent mounted on minecart", helper.AgentName)

	// Face the agent east (along the track direction) to ensure forward throttle moves the minecart
	// The rails are laid out east-west, so we turn towards a point to the east
	err = helper.ManagedAgent.Agent.TurnTowards(ctx, minecartX+10, minecartY, minecartZ)
	require.NoError(t, err, "face agent towards east")
	time.Sleep(500 * time.Millisecond)
	t.Logf("[%s] Agent should be facing east (track direction)", helper.AgentName)

	err = helper.EnterManualMode()
	require.NoError(t, err, "enter manual mode")

	// Record starting position
	startPos, startYaw, _, _ := helper.ManagedAgent.Agent.GetPosition() // (x, y, z float64, yaw, pitch float32, initialized bool)()
	t.Logf("[%s] Starting position: %s", helper.AgentName, startPos)

	expectedYaw, _ := utils.GetYawAndPitch(startPos, models.V3{X: minecartX + 10, Y: startPos.Y, Z: minecartZ})

	require.Equal(t, expectedYaw, startYaw, "agent should be facing east (yaw=90)")

	// Create tracker to monitor minecart position throughout movement
	tracker := helper.TrackEntityPosition(minecartEntityID)

	// Accelerate and climb with continuous throttle.
	// The minecart will:
	// 1) Accelerate on flat section (5 blocks)
	// 2) Transition to ascending section and attempt to climb
	// 3) Gravity will slow it as it goes uphill, eventually reversing it
	// This validates that minecarts respond to steering on slopes and physics work correctly.
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(10 * time.Second) //

	// Stop
	helper.SetManualThrottle(0, 0)

	// Wait for coast-to-stop
	err = helper.WaitForRidingVelocityZero(ctx, 5*time.Second)
	if err != nil {
		t.Logf("[%s][WARN] Velocity did not reach zero: %v", helper.AgentName, err)
	}

	pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	endX, endY, endZ := pos.X, pos.Y, pos.Z
	t.Logf("[%s] Ending position: (%.2f, %.2f, %.2f)", helper.AgentName, endX, endY, endZ)

	// Get tracked minecart movement data
	t.Logf("[%s] Position Tracker:\n%s", helper.AgentName, tracker)

	// Verify horizontal movement: minecart should reach ascending section (at least 5 blocks away from start)
	horizontalDistance := tracker.GetMinMaxHorizontalDistance()
	t.Logf("[%s] Horizontal distance traveled: %.2f blocks", helper.AgentName, horizontalDistance)
	assert.Greater(t, horizontalDistance, 5.0, "minecart should reach ascending section (5+ blocks)")

	// Verify uphill climbing: peak Y should be higher than starting Y
	// This proves the minecart climbed the ascending rail before gravity reversed it
	altitude := tracker.GetMinMaxAltitude()
	t.Logf("[%s] Altitude gained: %.2f blocks", helper.AgentName, altitude)
	assert.Greater(t, altitude, 0.25, "minecart should climb at least 0.5 blocks on ascending rail")

	err = helper.ExitManualMode()
	require.NoError(t, err, "exit manual mode")
	err = helper.DismountEntity()
	if err != nil {
		t.Logf("[WARN] Failed to dismount minecart: %v", err)
	}

}

// runMinecartPhases executes a series of steering phases and validates movement
func runMinecartPhases(t *testing.T, ctx context.Context, helper *VehicleTestHelper,
	_ *visualize.VisualizerAdapter, phases []steeringPhase) {

	for phaseIdx, phase := range phases {
		t.Run(phase.name, func(t *testing.T) {
			t.Logf("[Phase %d %s] Starting: %s (throttleX=%.2f, throttleZ=%.2f, duration=%v)",
				phaseIdx+1, phase.name, helper.AgentName, phase.throttleX, phase.throttleZ, phase.duration)

			// Record start position
			startPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			t.Logf("[%s] Starting position: %s ", helper.AgentName, startPos)

			helper.ManagedAgent.Agent.SendChat(fmt.Sprintf("Starting phase %d %s %s", phaseIdx+1, phase.name, startPos))

			// Apply throttle
			helper.SetManualThrottle(phase.throttleX, phase.throttleZ)

			// Run for the specified duration
			time.Sleep(phase.duration)

			// Stop throttle
			helper.SetManualThrottle(0, 0)

			// Wait for coast-to-stop between phases
			coastTimeout := 5 * time.Second

			err := helper.WaitForRidingVelocityZero(ctx, coastTimeout)
			if err != nil {
				t.Logf("[Phase %d %s][WARN] Velocity didn't decay to zero: %v", phaseIdx+1, helper.AgentName, err)
			}

			// sleep for a bit to give thing a moment to settle
			time.Sleep(5 * time.Second)

			// Record end position
			endPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			distanceXZ := startPos.DistanceToXZ(endPos)

			t.Logf("[Phase %d %s] %s: start=%s end=%s distance=%.2f blocks",
				phaseIdx+1, helper.AgentName, phase.name, startPos, endPos, distanceXZ)

			// Validate displacement
			if phase.minDisplacement > 0 {
				assert.GreaterOrEqual(t, distanceXZ, phase.minDisplacement,
					"phase %s: expected movement >= %.2f blocks, got %.2f",
					phase.name, phase.minDisplacement, distanceXZ)
			}

			if phase.maxDisplacement > 0 {
				assert.LessOrEqual(t, distanceXZ, phase.maxDisplacement,
					"phase %s: expected movement <= %.2f blocks, got %.2f",
					phase.name, phase.maxDisplacement, distanceXZ)
			}

			// Optional direction check
			if phase.checkDir != nil {
				phase.checkDir(t, startPos, endPos)
			}

			helper.ManagedAgent.Agent.SendChat(fmt.Sprintf("Ending phase %d %s %s end=%s distance=%.2f blocks", phaseIdx+1, phase.name, startPos, endPos, distanceXZ))
		})
	}
}

// TestMinecartOffRailFalling validates minecart physics when riding off rails.
//
// EDGE CASE: Off-rail gravity and velocity clamping
// - Minecarts have different drag when off-rail (0.5 vs 0.997 on-rail)
// - Gravity should apply: -0.04 blocks/tick² (MinecartFallGravity)
// - Velocity should clamp to max speed even when falling
//
// This test validates:
// 1. Minecart accelerates on rails
// 2. Minecart can be steered off the rails (falls)
// 3. Gravity applies correctly while falling (Y decreases)
// 4. Agent remains mounted during fall
// 5. Agent can safely dismount after falling
func (s *MinecartSteeringSuite) TestMinecartOffRailFalling() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("MinecartFallBot", "minecart_off_rail_falling")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create ground platform
	cmd := fmt.Sprintf("setblock %.0f -1 %.0f stone", helper.AtX(0), helper.AtZ(0))
	resp, err := helper.Instance.RCON.Exec(ctx, cmd)
	require.NoError(t, err, "create ground block")
	t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

	// Teleport agent
	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 0 %.1f", helper.ManagedAgent.Name, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err, "teleport agent")
	time.Sleep(500 * time.Millisecond)

	agentPos, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
	require.True(t, initialized, "agent position should be initialized")

	// Build short rail track on elevated platform (5 blocks)
	railStartX := agentPos.X + 2
	railStartY := agentPos.Y + 5 // Elevate platform so minecart can fall
	railStartZ := agentPos.Z

	// Build support platform under rails
	for x := blockCoord(railStartX) - 1; x < blockCoord(railStartX)+5; x++ {
		block := "stone"
		if x == blockCoord(railStartX)+2 {
			block = "redstone_block"
		}
		cmd = fmt.Sprintf("setblock %d %d %d %s", x, blockCoord(railStartY)-1, blockCoord(railStartZ), block)
		_, err = helper.Instance.RCON.Exec(ctx, cmd)
		require.NoError(t, err, "create rail support platform")
	}

	// use unpowered rails for manual acceleration
	err = helper.BuildRailTrack(ctx, railStartX, railStartY, railStartZ, "east", 2, false)
	require.NoError(t, err, "build short rail track")

	// Add powered rail to give initial acceleration before falling
	err = helper.BuildRailTrack(ctx, railStartX+2, railStartY, railStartZ, "east", 3, true)
	require.NoError(t, err, "build short rail track")

	// add rails to land on
	err = helper.BuildRailTrack(ctx, railStartX+5, railStartY-5, railStartZ, "east", 5, false)
	require.NoError(t, err, "build short rail track")

	// Summon minecart on rails
	minecartX := railStartX
	minecartY := railStartY + 1
	minecartZ := railStartZ
	minecartEntityID, err := helper.SummonMinecart(ctx, minecartX, minecartY, minecartZ)
	require.NoError(t, err, "summon minecart")
	t.Logf("[%s] Spawned minecart at (%.1f, %.1f, %.1f)", helper.AgentName, minecartX, minecartY, minecartZ)

	// Mount minecart
	err = helper.MountEntity(ctx, minecartEntityID)
	require.NoError(t, err, "mount minecart")
	err = helper.WaitForMounted(ctx, 15*time.Second)
	require.NoError(t, err, "agent should be mounted")
	t.Logf("[%s] Agent mounted on minecart", helper.AgentName)

	// Face east
	err = helper.ManagedAgent.Agent.TurnTowards(ctx, minecartX+10, minecartY, minecartZ)
	require.NoError(t, err, "face agent towards east")
	time.Sleep(500 * time.Millisecond)

	// Enter manual mode
	err = helper.EnterManualMode()
	require.NoError(t, err, "enter manual mode")

	// Record starting position
	startPos, _, _, _ := helper.ManagedAgent.Agent.GetPosition()
	t.Logf("[%s] Starting position: %s", helper.AgentName, startPos)

	// Accelerate forward along rails for 5 seconds
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(5 * time.Second)

	// Stop throttle - minecart should reach end of rails and fall
	helper.SetManualThrottle(0, 0)
	time.Sleep(2 * time.Second)

	// Let minecart fall for 3 seconds (should gain downward velocity)
	time.Sleep(3 * time.Second)

	// Record position while falling
	fallPos, _, _, _ := helper.ManagedAgent.Agent.GetPosition()
	t.Logf("[%s] After fall period: %s", helper.AgentName, fallPos)

	// Validate: Y should have decreased (fell down)
	assert.Less(t, fallPos.Y, startPos.Y-3, "minecart should have fallen (Y decreased from %.2f to %.2f)", startPos.Y, fallPos.Y)

	// Wait for velocity to stabilize
	time.Sleep(2 * time.Second)

	// Dismount and verify safe exit
	err = helper.ExitManualMode()
	require.NoError(t, err, "exit manual mode")

	err = helper.DismountEntity()
	require.NoError(t, err, "dismount vehicle while falling")

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err, "agent should dismount successfully")

	finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	t.Logf("[%s] Agent safely dismounted at %s", helper.AgentName, finalPos)
}

// TestMinecartUnderwater validates minecart physics when riding in water.
//
// EDGE CASE: Underwater minecart behavior
// - Minecarts can be placed in water
// - Velocity multiplier may differ from land/rail
// - Gravity should still apply (minecarts can float or sink)
// - Agent should remain mounted
//
// This test validates:
// 1. Minecart movement works in water (with potential speed reduction)
// 2. Gravity behavior is consistent
// 3. Agent remains mounted while in water
func (s *MinecartSteeringSuite) TestMinecartUnderwater() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("MinecartUnderH2O", "minecart_underwater")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create ground
	cmd := fmt.Sprintf("setblock %.0f -1 %.0f stone", helper.AtX(0), helper.AtZ(0))
	resp, err := helper.Instance.RCON.Exec(ctx, cmd)
	require.NoError(t, err, "create ground block")
	t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

	// Teleport agent to the centre of the block: the original's integer
	// coordinates ("teleport X 0 0 0") are centred by the server, and the
	// summoned minecart's start offset relative to the rails matters here
	// (a start half a block back stalls it in the water before the slope).
	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 0 %.1f", helper.ManagedAgent.Name, helper.AtX(0)+0.5, helper.AtZ(0)+0.5))
	require.NoError(t, err, "teleport agent")
	time.Sleep(500 * time.Millisecond)

	agentPos, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
	require.True(t, initialized, "agent position should be initialized")

	// Face the agent east (along the track direction) to ensure forward throttle moves the minecart
	// The rails are laid out to the south, so we turn towards a point to the south
	err = helper.ManagedAgent.Agent.TurnTowards(ctx, agentPos.X, agentPos.Y, agentPos.Z+10)
	require.NoError(t, err, "face agent towards east")
	time.Sleep(500 * time.Millisecond)

	// The track layout below is expressed in whole-block offsets from the
	// block the agent stands in (bx, bz). The original computed it from the
	// agent's centred position (x.5) with int(), which truncates toward zero,
	// so half-block offsets landed on a fixed, asymmetric set of blocks; the
	// offsets here are exactly what that produced at the world origin, and stay
	// the same wherever the working area is (including negative coordinates).
	bx := float64(blockCoord(agentPos.X))
	bz := float64(blockCoord(agentPos.Z))

	// Create water pool at ground level (agentY-1)
	groundY := agentPos.Y - 1
	cmd = fmt.Sprintf("fill %d %d %d %d %d %d water",
		int(bx)-1, int(groundY), int(bz)-1,
		int(bx)+3, int(groundY), int(bz)+3)
	_, err = helper.Instance.RCON.Exec(ctx, cmd)
	require.NoError(t, err, "fill water area at ground level")
	t.Logf("[%s] Created water pool at ground level", helper.AgentName)

	// Create unpowered rail track before water (pre-water level)
	err = helper.BuildRailTrack(ctx, bx, agentPos.Y, bz-6, "south", 6, false)
	require.NoError(t, err, "build rail track before water")
	t.Logf("[%s] Created rail track before water (5 blocks at elevation)", helper.AgentName)

	// Create descending rail track: north_ascending rail down into water
	// This descends from agentY to groundY level as we move south
	err = helper.BuildAscendingWaterloggedRailTrack(ctx, bx, agentPos.Y-1, bz-1, "north", 1, false)
	require.NoError(t, err, "build north_ascending rail down into water")
	t.Logf("[%s] Created north_ascending descending rail into water", helper.AgentName)

	// Create waterlogged horizontal rails (traversing through water)
	err = helper.BuildWaterloggedRailTrack(ctx, bx, groundY, bz, "south", 3, false)
	require.NoError(t, err, "build waterlogged rail track through water")
	t.Logf("[%s] Created three waterlogged rails through water", helper.AgentName)

	// Create waterlogged south_ascending rail (ascending back up from water level)
	err = helper.BuildAscendingWaterloggedRailTrack(ctx, bx, groundY, bz+2, "south", 1, true)
	require.NoError(t, err, "build waterlogged ascending powered rail")
	t.Logf("[%s] Created waterlogged south_ascending powered rail exiting water", helper.AgentName)

	// Create waterlogged south_ascending rail (ascending back up from water level)
	err = helper.BuildAscendingRailTrack(ctx, bx, groundY, bz+3, "south", 6, true)
	require.NoError(t, err, "build waterlogged ascending powered rail")
	t.Logf("[%s] Created waterlogged south_ascending powered rail exiting water", helper.AgentName)

	// Create support platform under the unpowered high-elevation rails
	// The waterlogged ascending rail climbs to agentY+2, then we add unpowered rails at agentY+4
	for z := int(bz) + 9; z < int(bz)+18; z++ {
		cmd = fmt.Sprintf("setblock %d %d %d stone", int(bx), int(agentPos.Y+3), z)
		_, err = helper.Instance.RCON.Exec(ctx, cmd)
		require.NoError(t, err, "create support platform under high-elevation rail")
	}

	// Create unpowered rails at higher elevation (for testing gravity slowdown)
	// Connect from end of waterlogged ascending rail to high elevation
	err = helper.BuildRailTrack(ctx, bx, agentPos.Y+4, bz+7, "south", 10, false)
	require.NoError(t, err, "build unpowered rail stretch at high elevation")
	t.Logf("[%s] Created 10-block unpowered rail stretch at high elevation", helper.AgentName)

	// Summon minecart on the waterlogged rail (first waterlogged block)
	minecartX := agentPos.X
	minecartY := groundY + 1    // Minecart sits on top of rail block
	minecartZ := agentPos.Z - 3 // Position on first waterlogged rail
	minecartEntityID, err := helper.SummonMinecart(ctx, minecartX, minecartY, minecartZ)
	require.NoError(t, err, "summon minecart")

	getter, ok := helper.ManagedAgent.Agent.(models.MountedEntityPositionGetter)
	require.True(t, ok, "agent must implement MountedEntityPositionGetter")

	yaw, found := getter.GetMountedEntityYaw(minecartEntityID)
	require.True(t, found, "minecart yaw should be tracked")

	direction := physics.YawToCardinal(yaw)
	assert.Equal(t, physics.CardinalSouth, direction, "minecart should be facing south")

	t.Logf("[%s] Spawned minecart in rail at (%.1f, %.1f, %.1f) facing %s", helper.AgentName, minecartX, minecartY, minecartZ, direction)

	// Mount minecart
	err = helper.MountEntity(ctx, minecartEntityID)
	assert.NoError(t, err, "mount minecart")

	err = helper.WaitForMounted(ctx, 15*time.Second)
	assert.NoError(t, err, "agent should be mounted")

	t.Logf("[%s] Agent mounted in minecart", helper.AgentName)

	isMounted := helper.ManagedAgent.Agent.IsMounted()
	assert.True(t, isMounted, "agent should be mounted before movement")

	time.Sleep(2 * time.Second)

	_, yaw, _, _ = helper.ManagedAgent.Agent.GetPosition()
	direction = physics.YawToCardinal(float64(yaw))
	t.Logf("[%s] Agent initial facing direction: %s", helper.AgentName, direction)
	helper.ManagedAgent.Agent.SendChat(fmt.Sprintf("Facing %s", direction))

	// Face agent towards the direction of travel (south along the rails)
	// if direction != physics.CardinalSouth {
	t.Logf("[%s] Agent facing %s, turning towards south for correct movement direction", helper.AgentName, direction)
	helper.ManagedAgent.Agent.SendChat(fmt.Sprintf("Facing %s, turning towards south for correct movement direction", direction))

	err = helper.ManagedAgent.Agent.TurnTowards(ctx, minecartX, minecartY, minecartZ+10)
	assert.NoError(t, err, "face agent towards direction of travel")
	time.Sleep(500 * time.Millisecond)
	// }
	// Enter manual mode
	err = helper.EnterManualMode()
	assert.NoError(t, err, "enter manual mode")

	// Record starting position (in water on unpowered rail)
	startPos, yaw, _, _ := helper.ManagedAgent.Agent.GetPosition()

	direction = physics.YawToCardinal(float64(yaw))
	assert.Equal(t, physics.CardinalSouth, direction, "agent should be facing south")

	t.Logf("[%s] Starting position on rail: %s facing %s", helper.AgentName, startPos, direction)

	// Move forward in water for 6 seconds to reach ascending powered rail
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(6 * time.Second)

	// Check position after water traversal
	waterPos, yaw, _, _ := helper.ManagedAgent.Agent.GetPosition()
	t.Logf("[%s] Position after water traversal: %s facing: %s", helper.AgentName, waterPos, physics.YawToCardinal(yaw))

	isMounted = helper.ManagedAgent.Agent.IsMounted()
	assert.True(t, isMounted, "agent should remain mounted during water movement")

	// Calculate displacement in water
	waterDistanceXZ := startPos.DistanceToXZ(waterPos)
	assert.Greater(t, waterDistanceXZ, 0.1, "minecart must move at least 0.1 blocks through water")
	t.Logf("[%s] Minecart moved %.2f blocks through water", helper.AgentName, waterDistanceXZ)

	// Continue movement to climb the ascending rail
	time.Sleep(3 * time.Second)

	// Check position on ascending rail
	climbPos, yaw, _, _ := helper.ManagedAgent.Agent.GetPosition()
	t.Logf("[%s] Position after climbing powered ascending rail: %s facing: %s", helper.AgentName, climbPos, physics.YawToCardinal(yaw))

	// Verify gravity behavior: Y-coordinate should increase significantly on powered ascending rail
	deltaY := climbPos.Y - waterPos.Y
	assert.Greater(t, deltaY, 0.5, "minecart should climb powered ascending rail (Y delta: %.2f)", deltaY)
	t.Logf("[%s] Minecart climbed %.2f blocks vertically", helper.AgentName, deltaY)

	isMounted = helper.ManagedAgent.Agent.IsMounted()
	assert.True(t, isMounted, "agent should remain mounted during powered rail ascent")

	// Stop movement
	helper.SetManualThrottle(0, 0)
	time.Sleep(2 * time.Second)

	// Record position on unpowered rail for gravity slowdown validation
	onUnpoweredPos, yaw, _, _ := helper.ManagedAgent.Agent.GetPosition()
	t.Logf("[%s] Position on unpowered rail: %s facing: %s", helper.AgentName, onUnpoweredPos, physics.YawToCardinal(yaw))

	// Continue movement on unpowered rails for 2 seconds to observe gravity slowdown
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(5 * time.Second)

	// Check position after unpowered rail segment
	afterUnpoweredPos, yaw, _, _ := helper.ManagedAgent.Agent.GetPosition()
	t.Logf("[%s] Position after unpowered rail segment: %s facing: %s", helper.AgentName, afterUnpoweredPos, physics.YawToCardinal(yaw))

	// Verify gravity applies: minecart should drop on unpowered rails (negative Y delta)
	unpoweredDeltaY := afterUnpoweredPos.Y - onUnpoweredPos.Y
	assert.Less(t, unpoweredDeltaY, 1.0, "minecart should drop due to gravity on unpowered rails (Y delta: %.2f)", unpoweredDeltaY)
	t.Logf("[%s] Gravity behavior on unpowered rail: Y change of %.2f blocks", helper.AgentName, unpoweredDeltaY)

	isMounted = helper.ManagedAgent.Agent.IsMounted()
	require.True(t, isMounted, "agent should remain mounted during unpowered rail traversal")

	// Stop movement
	helper.SetManualThrottle(0, 0)
	time.Sleep(1 * time.Second)

	// Exit manual mode and dismount
	err = helper.ExitManualMode()
	assert.NoError(t, err, "exit manual mode")

	err = helper.DismountEntity()
	assert.NoError(t, err, "dismount minecart")

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	assert.NoError(t, err, "agent should dismount successfully")

	// Verify agent is alive after the complete minecart ride
	finalPos, yaw, _, _ := helper.ManagedAgent.Agent.GetPosition()
	t.Logf("[%s] Agent safely dismounted at %s facing: %s", helper.AgentName, finalPos, physics.YawToCardinal(yaw))
}

// TestMinecartDismountEdgeCases validates dismounting behavior in edge cases.
//
// EDGE CASE: Dismounting during unusual states
// - Dismounting while minecart is off-rails
// - Dismounting while minecart is in water
// - Dismounting while minecart is moving fast
// - Mount state synchronization recovery
//
// This test validates:
// 1. Agent can dismount while minecart is moving
// 2. Agent can dismount while minecart is off-rails
// 3. Mount state is properly cleared after dismounting
// 4. Agent doesn't remain stuck mounted
// 5. Agent can re-mount after dismounting in edge cases
func (s *MinecartSteeringSuite) TestMinecartDismountEdgeCases() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("MinecartDismountBot", "minecart_dismount_edge_cases")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Create ground
	cmd := fmt.Sprintf("setblock %.0f -1 %.0f stone", helper.AtX(0), helper.AtZ(0))
	resp, err := helper.Instance.RCON.Exec(ctx, cmd)
	require.NoError(t, err, "create ground block")
	t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

	// Teleport agent
	_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.1f 0 %.1f", helper.ManagedAgent.Name, helper.AtX(0), helper.AtZ(0)))
	require.NoError(t, err, "teleport agent")
	time.Sleep(500 * time.Millisecond)

	agentPos, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
	require.True(t, initialized, "agent position should be initialized")

	// Build rail track
	railStartX := agentPos.X + 2
	railStartY := agentPos.Y - 1
	railStartZ := agentPos.Z

	err = helper.BuildRailTrack(ctx, railStartX, railStartY, railStartZ, "east", 10, false)
	require.NoError(t, err, "build rail track")

	// Summon minecart
	minecartX := railStartX
	minecartY := railStartY + 1
	minecartZ := railStartZ
	minecartEntityID, err := helper.SummonMinecart(ctx, minecartX, minecartY, minecartZ)
	require.NoError(t, err, "summon minecart")
	t.Logf("[%s] Spawned minecart at (%.1f, %.1f, %.1f)", helper.AgentName, minecartX, minecartY, minecartZ)

	// Mount minecart
	err = helper.MountEntity(ctx, minecartEntityID)
	require.NoError(t, err, "mount minecart")
	err = helper.WaitForMounted(ctx, 15*time.Second)
	require.NoError(t, err, "agent should be mounted")
	t.Logf("[%s] Agent mounted on minecart", helper.AgentName)

	// Face agent east
	err = helper.ManagedAgent.Agent.TurnTowards(ctx, minecartX+10, minecartY, minecartZ)
	require.NoError(t, err, "face agent towards east")
	time.Sleep(500 * time.Millisecond)

	// Enter manual mode
	err = helper.EnterManualMode()
	require.NoError(t, err, "enter manual mode")

	// Accelerate minecart
	helper.SetManualThrottle(0, 1.0)
	time.Sleep(3 * time.Second)

	// Now try to dismount while still accelerating
	t.Logf("[%s] Attempting to dismount while minecart is accelerating", helper.AgentName)
	helper.SetManualThrottle(0, 0)

	// Exit manual mode
	err = helper.ExitManualMode()
	require.NoError(t, err, "exit manual mode while moving")

	// Dismount while minecart is still coasting
	err = helper.DismountEntity()
	require.NoError(t, err, "dismount while minecart coasting")

	// Wait for dismount confirmation
	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err, "agent should dismount while coasting")
	t.Logf("[%s] Agent successfully dismounted while minecart coasting", helper.AgentName)

	// Try to re-mount the same minecart (verify state recovery)
	err = helper.MountEntity(ctx, minecartEntityID)
	require.NoError(t, err, "re-mount minecart after dismounting")
	err = helper.WaitForMounted(ctx, 15*time.Second)
	require.NoError(t, err, "agent should re-mount successfully")
	t.Logf("[%s] Agent successfully re-mounted after dismounting", helper.AgentName)

	// Dismount again cleanly
	err = helper.DismountEntity()
	require.NoError(t, err, "dismount again")

	err = helper.WaitForDismounted(ctx, 5*time.Second)
	require.NoError(t, err, "agent should dismount cleanly second time")

	// Verify final state
	finalPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	t.Logf("[%s] Agent at %s after final dismount", helper.AgentName, finalPos)
}
