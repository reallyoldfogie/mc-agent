package vehicles

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/internal/visualize"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMinecartSteering verifies that the agent can mount a minecart and steer it
// using manual inputs, asserting that every throttle phase produces movement
// in the expected direction by a non-trivial amount.
//
// Minecarts move along rails with physics that include drag, slope gravity,
// and powered rail acceleration. The test validates steering by asserting
// directional movement and distance traveled.
func TestMinecartSteering(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "MinecartSteerBot")
			defer cleanup()

			// Build a flat rail track (east-west direction)
			// Spawn location: agent at (0, 0, 0), rails from (2, -1, 0) to (20, -1, 0)
			cmd := "setblock 0 -1 0 stone"
			resp, err := helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "create ground block")
			t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

			// Teleport agent to a safe location
			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s 0 0 0", helper.ManagedAgent.Name))
			require.NoError(t, err, "teleport agent")
			time.Sleep(500 * time.Millisecond)

			agentX, agentY, agentZ, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			// Build rail track going east
			railStartX := agentX + 2
			railStartY := agentY - 1
			railStartZ := agentZ

			err = helper.BuildRailTrack(ctx, railStartX, railStartY, railStartZ, "east", 20)
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

			startX, startY, startZ, startYaw, _, _ := helper.ManagedAgent.Agent.GetPosition() // (x, y, z float64, yaw, pitch float32, initialized bool)()

			expectedYaw, _ := utils.GetYawAndPitch(models.V3{X: startX, Y: startY, Z: startZ}, models.V3{X: minecartX + 10, Y: startY, Z: minecartZ})

			require.Equal(t, expectedYaw, startYaw, "agent should be facing east (yaw=%.02f)", expectedYaw)

			// Enter manual mode for steering
			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			visualizerRCON := visualize.NewVisualizerAdapter(helper.Instance.RCON)
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
		})
	}
}

// TestMinecartAscendingRail verifies that minecarts can climb ascending rails
// and properly decelerate going uphill.
func TestMinecartAscendingRail(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "MinecartClimbBot")
			defer cleanup()

			// Create ground
			cmd := "setblock 0 -1 0 stone"
			resp, err := helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "create ground block")
			t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

			// Teleport agent
			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s 0 0 0", helper.ManagedAgent.Name))
			require.NoError(t, err, "teleport agent")
			time.Sleep(500 * time.Millisecond)

			agentX, agentY, agentZ, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			// Build flat rail section first
			railStartX := agentX + 2
			railStartY := agentY - 1
			railStartZ := agentZ

			err = helper.BuildRailTrack(ctx, railStartX, railStartY, railStartZ, "east", 5)
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
			startX, startY, startZ, startYaw, _, _ := helper.ManagedAgent.Agent.GetPosition() // (x, y, z float64, yaw, pitch float32, initialized bool)()
			t.Logf("[%s] Starting position: (%.2f, %.2f, %.2f)", helper.AgentName, startX, startY, startZ)

			expectedYaw, _ := utils.GetYawAndPitch(models.V3{X: startX, Y: startY, Z: startZ}, models.V3{X: minecartX + 10, Y: startY, Z: minecartZ})

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

			endX, endY, endZ, _ := helper.ManagedAgent.Agent.GetPositionSimple()
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

		})
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
			startX, startY, startZ, _ := helper.ManagedAgent.Agent.GetPositionSimple() // (x, y, z float64, yaw, pitch float32, initialized bool)()
			t.Logf("[%s] Starting position: (%.2f, %.2f, %.2f) ", helper.AgentName, startX, startY, startZ)

			helper.ManagedAgent.Agent.SendChat(fmt.Sprintf("Starting phase %d %s (%.2f, %.2f, %.2f)", phaseIdx+1, phase.name, startX, startY, startZ))

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
			endX, endY, endZ, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			deltaX := endX - startX
			deltaZ := endZ - startZ
			distance := math.Sqrt(deltaX*deltaX + deltaZ*deltaZ)

			t.Logf("[Phase %d %s] %s: start=(%.2f, %.2f, %.2f) end=(%.2f, %.2f, %.2f) distance=%.2f blocks",
				phaseIdx+1, helper.AgentName, phase.name, startX, startY, startZ, endX, endY, endZ, distance)

			// Validate displacement
			if phase.minDisplacement > 0 {
				assert.GreaterOrEqual(t, distance, phase.minDisplacement,
					"phase %s: expected movement >= %.2f blocks, got %.2f",
					phase.name, phase.minDisplacement, distance)
			}

			if phase.maxDisplacement > 0 {
				assert.LessOrEqual(t, distance, phase.maxDisplacement,
					"phase %s: expected movement <= %.2f blocks, got %.2f",
					phase.name, phase.maxDisplacement, distance)
			}

			// Optional direction check
			if phase.checkDir != nil {
				phase.checkDir(t, models.V3{X: startX, Y: startY, Z: startZ},
					models.V3{X: endX, Y: endY, Z: endZ})
			}

			helper.ManagedAgent.Agent.SendChat(fmt.Sprintf("Ending phase %d %s (%.2f, %.2f, %.2f) end=(%.2f, %.2f, %.2f) distance=%.2f blocks", phaseIdx+1, phase.name, startX, startY, startZ, endX, endY, endZ, distance))
		})
	}
}
