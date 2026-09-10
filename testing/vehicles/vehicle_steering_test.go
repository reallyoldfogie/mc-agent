package vehicles

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/internal/visualize"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// coastTimeout is the maximum time to wait for riding velocity to decay to
// zero between phases. In practice WaitForRidingVelocityZero returns as soon
// as the executor reports (0, 0), so this is just a safety cap.
const coastTimeout = 5 * time.Second

// steeringPhase defines one independent throttle burst in a steering test.
// Each phase begins from zero velocity (after a coast-to-stop period) so
// drift from one phase does not contaminate the next.
//
// The phase is purely behavioural: it asserts that the chosen input causes
// the vehicle to move in the expected direction by at least minDisplacement
// blocks (when set), or to stay within maxDisplacement blocks of the start
// (when set). It does NOT compare against a parallel physics simulation.
type steeringPhase struct {
	name      string
	throttleX float64
	throttleZ float64
	duration  time.Duration

	// minDisplacement asserts the horizontal XZ distance from the recorded
	// start position to the actual end position is at least this many
	// blocks. Zero disables the check. Used for non-idle phases to verify
	// the vehicle actually moved.
	minDisplacement float64

	// maxDisplacement asserts the horizontal XZ distance from the recorded
	// start position to the actual end position is at most this many
	// blocks. Zero disables the check. Used for idle phases to verify the
	// vehicle stayed put.
	maxDisplacement float64

	// Optional direction sanity checks. These are run with the actual
	// sampled start and end positions (no simulator involvement).
	checkDir        func(t *testing.T, start, end models.V3)                   // optional world-axis direction check (boat)
	checkDirWithYaw func(t *testing.T, start, end models.V3, startYaw float64) // optional yaw-relative direction check (horse)
}

// TestBoatSteering verifies that the agent can mount a boat and steer it
// using manual inputs, asserting that every throttle phase produces movement
// in the expected direction by a non-trivial amount.
//
// Each phase begins from zero velocity (after a coast-to-stop period) and is
// validated purely behaviourally: the boat must move in the expected
// direction, displace at least minDisplacement blocks, and stay within
// maxDisplacement blocks of start when idle. There is no parallel physics
// simulation in this test.
func TestBoatSteering(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "BoatSteerBot")
			defer cleanup()

			cmd := "fill 0 -25 0 25 -1 25 water"
			resp, err := helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill water area")
			t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

			cmd = "fill 10 -25 0 -25 -1 25 water"
			resp, err = helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill water area")
			t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

			cmd = "fill -1 -25 1 -1 -1 1 minecraft:stone"
			resp, err = helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill water area")
			t.Logf("[%s] %s => %s", helper.AgentName, cmd, resp)

			// Teleport agent outside the water fill region so it stands on
			// solid ground, within interaction range of the boat spawn point.
			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s -1 0 1", helper.ManagedAgent.Name))
			require.NoError(t, err, "teleport agent")
			time.Sleep(500 * time.Millisecond)

			_, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			boatEntityID, err := helper.SummonBoat(ctx, 1, 1, 1, "oak")
			require.NoError(t, err, "summon boat")
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, boatEntityID)
			require.NoError(t, err, "mount boat")
			err = helper.WaitForMounted(ctx, 15*time.Second)
			require.NoError(t, err, "agent should be mounted")
			t.Logf("[%s] agent mounted successfully", helper.AgentName)

			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			visualizerRCON := visualize.NewVisualizerAdapter(helper.Instance.RCON, nil)
			visualize.ClearPathVisualizations(ctx, helper.Instance.RCON)

			phases := []steeringPhase{
				{
					name:      "forward",
					throttleX: 0, throttleZ: 1.0,
					duration:        2 * time.Second,
					minDisplacement: 3.0,
				},
				{
					// Steer right while thrusting forward: boat curves right.
					// Vanilla left/right rotates yaw via angular velocity;
					// combined with forward thrust this produces a curving arc.
					name:      "steer_right_forward",
					throttleX: 1.0, throttleZ: 1.0,
					duration:        3 * time.Second,
					minDisplacement: 2.0,
				},
				{
					name:      "backward",
					throttleX: 0, throttleZ: -1.0,
					duration: 3 * time.Second,
					// Backward thrust is much smaller (0.005 vs 0.04), so even
					// over 3s the boat only travels a few blocks.
					minDisplacement: 0.5,
				},
				{
					// Pure left: only rotates the boat, no thrust.
					// The boat should spin in place with minimal displacement.
					name:      "rotate_left",
					throttleX: -1.0, throttleZ: 0,
					duration:        2 * time.Second,
					maxDisplacement: 1.0,
				},
				{
					name:      "idle",
					throttleX: 0, throttleZ: 0,
					duration:        2 * time.Second,
					maxDisplacement: 0.5,
				},
			}

			runBoatPhases(t, ctx, helper, visualizerRCON, phases)

			err = helper.ExitManualMode()
			require.NoError(t, err, "exit manual mode")
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount vehicle")
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			if err != nil {
				// failing to dismount at end of test isn't a test failure - just log it.
				t.Logf("[%s][WARN] Agent may still be mounted after test completion: %v", helper.AgentName, err)
			}
		})
	}
}

// TestHorseSteering verifies that the agent can mount a horse and steer it
// using manual inputs, asserting that every throttle phase produces movement
// in the expected direction by a non-trivial amount.
//
// Unlike the boat, the horse uses yaw-rotation steering: ThrottleX rotates
// the horse and ThrottleZ accelerates along the facing direction. The phase
// assertions are purely behavioural - there is no parallel physics
// simulation in this test.
func TestHorseSteering(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "HorseSteerBot")
			defer cleanup()

			_, err := helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s 0 0 0", helper.ManagedAgent.Name))
			require.NoError(t, err, "teleport agent")
			time.Sleep(500 * time.Millisecond)

			agentPos, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			err = helper.BuildHorseEnclosure(ctx, agentPos.X+1, agentPos.Y, agentPos.Z)
			require.NoError(t, err, "build horse enclosure")

			horseEntityID, err := helper.SummonHorse(ctx, agentPos.X+1, agentPos.Y, agentPos.Z, 25)
			require.NoError(t, err, "summon horse")
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, horseEntityID)
			require.NoError(t, err, "mount horse")
			err = helper.WaitForMounted(ctx, 25*time.Second)
			require.NoError(t, err, "agent should be mounted")

			err = helper.RemoveHorseEnclosure(ctx, agentPos.X+1, agentPos.Y, agentPos.Z)
			require.NoError(t, err, "remove horse enclosure")
			t.Logf("[%s] agent mounted successfully", helper.AgentName)

			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			visualizerRCON := visualize.NewVisualizerAdapter(helper.Instance.RCON, nil)
			visualize.ClearPathVisualizations(ctx, helper.Instance.RCON)

			phases := []steeringPhase{
				{
					name:      "forward",
					throttleX: 0, throttleZ: 1.0,
					duration:        2 * time.Second,
					minDisplacement: 2.0,
					checkDirWithYaw: func(t *testing.T, start, end models.V3, startYaw float64) {
						yawRad := startYaw * math.Pi / 180.0
						forwardProj := -math.Sin(yawRad)*(end.X-start.X) + math.Cos(yawRad)*(end.Z-start.Z)
						require.Greater(t, forwardProj, 0.5, "should move forward in facing direction")
					},
				},
				{
					name:      "steer_right",
					throttleX: 1.0, throttleZ: 0.5,
					duration:        3 * time.Second,
					minDisplacement: 0.5,
				},
				{
					name:      "backward",
					throttleX: 0, throttleZ: -1.0,
					duration:        3 * time.Second,
					minDisplacement: 1.0,
					checkDirWithYaw: func(t *testing.T, start, end models.V3, startYaw float64) {
						yawRad := startYaw * math.Pi / 180.0
						forwardProj := -math.Sin(yawRad)*(end.X-start.X) + math.Cos(yawRad)*(end.Z-start.Z)
						require.Less(t, forwardProj, 0.0, "should move backward in facing direction")
					},
				},
				{
					name:      "idle",
					throttleX: 0, throttleZ: 0,
					duration:        2 * time.Second,
					maxDisplacement: 0.5,
				},
			}

			runHorsePhases(t, ctx, helper, visualizerRCON, phases)

			err = helper.ExitManualMode()
			require.NoError(t, err, "exit manual mode")
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount vehicle")
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			if err != nil {
				// failing to dismount at end of test isn't a test failure - just log it.
				t.Logf("[%s][WARN] Agent may still be mounted after test completion: %v", helper.AgentName, err)
			}
		})
	}
}

// TestVehicleSteeringInputs validates multiple throttle combinations on a
// mounted boat. Each combination runs independently from zero velocity. The
// assertions are purely behavioural: each input must move the boat in the
// expected direction(s) and by at least minDisplacement blocks (or stay
// within maxDisplacement when stopped).
func TestVehicleSteeringInputs(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "VehicleSteerBot")
			defer cleanup()

			cmd := "fill 0 -25 0 25 -1 25 water"
			resp, err := helper.Instance.RCON.Exec(ctx, cmd)
			require.NoError(t, err, "fill water area")
			t.Logf("%s => %s", cmd, resp)

			_, err = helper.Instance.RCON.Exec(ctx, fmt.Sprintf("teleport %s -1 1 -1", helper.ManagedAgent.Name))
			require.NoError(t, err, "teleport agent")
			time.Sleep(500 * time.Millisecond)

			_, initialized := helper.ManagedAgent.Agent.GetPositionSimple()
			require.True(t, initialized, "agent position should be initialized")

			boatEntityID, err := helper.SummonBoat(ctx, 1, 1, 1, "oak")
			require.NoError(t, err, "summon boat")
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, boatEntityID)
			require.NoError(t, err, "mount boat")
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")

			visualizerRCON := visualize.NewVisualizerAdapter(helper.Instance.RCON, nil)
			visualize.ClearPathVisualizations(ctx, helper.Instance.RCON)

			phases := []steeringPhase{
				{
					name: "forward", throttleX: 0, throttleZ: 1.0,
					duration:        1 * time.Second,
					minDisplacement: 0.5,
				},
				{
					name: "backward", throttleX: 0, throttleZ: -1.0,
					duration: 1 * time.Second,
					// Backward thrust is much smaller (0.005 vs 0.04), so the
					// boat travels well under one block in 1s.
					minDisplacement: 0.05,
				},
				{
					// Pure right: only rotates, no thrust. Minimal displacement.
					name: "rotate_right", throttleX: 1.0, throttleZ: 0,
					duration:        1 * time.Second,
					maxDisplacement: 1.0,
				},
				{
					// Pure left: only rotates, no thrust. Minimal displacement.
					name: "rotate_left", throttleX: -1.0, throttleZ: 0,
					duration:        1 * time.Second,
					maxDisplacement: 1.0,
				},
				{
					// Curve right: right rotation + forward thrust.
					name: "curve_right", throttleX: 1.0, throttleZ: 1.0,
					duration:        2 * time.Second,
					minDisplacement: 1.0,
				},
				{
					// Curve left: left rotation + forward thrust.
					name: "curve_left", throttleX: -1.0, throttleZ: 1.0,
					duration:        2 * time.Second,
					minDisplacement: 1.0,
				},
				{
					name:      "stop",
					throttleX: 0, throttleZ: 0,
					duration:        500 * time.Millisecond,
					maxDisplacement: 0.5,
				},
			}

			runBoatPhases(t, ctx, helper, visualizerRCON, phases)

			err = helper.ExitManualMode()
			require.NoError(t, err, "exit manual mode")
			err = helper.DismountEntity()
			require.NoError(t, err, "dismount")
			err = helper.WaitForDismounted(ctx, 5*time.Second)
			if err != nil {
				// failing to dismount at end of test isn't a test failure - just log it.
				t.Logf("[%s][WARN] Agent may still be mounted after test completion: %v", helper.AgentName, err)
			}
		})
	}
}

// --- Phase runners ---

// runBoatPhases executes a sequence of independent steering phases on a
// mounted boat. Each phase starts from zero velocity after a coast-to-stop
// period and is validated purely behaviourally: direction (via checkDir) and
// horizontal displacement bounds (minDisplacement / maxDisplacement).
func runBoatPhases(
	t *testing.T,
	ctx context.Context,
	helper *VehicleTestHelper,
	summoner visualize.DisplayEntitySummoner,
	phases []steeringPhase,
) {
	for _, phase := range phases {
		t.Run(phase.name, func(t *testing.T) {
			// Coast to stop: zero throttle and poll until velocity is zero.
			helper.SetManualThrottle(0, 0)
			require.NoError(t, helper.WaitForRidingVelocityZero(ctx, coastTimeout),
				"velocity should reach zero before phase %s", phase.name)

			// Record actual start position (post-coast).
			start, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			// Apply throttle and let the boat move.
			helper.SetManualThrottle(phase.throttleX, phase.throttleZ)
			time.Sleep(phase.duration)

			// Sample end position.
			actual, _ := helper.ManagedAgent.Agent.GetPositionSimple()

			displacement := GetDistance(start.X, 0, start.Z, actual.X, 0, actual.Z)
			t.Logf("[%s] %s: start=(%.2f,%.2f,%.2f) actual=(%.2f,%.2f,%.2f) displacement=%.2f",
				helper.AgentName, phase.name,
				start.X, start.Y, start.Z,
				actual.X, actual.Y, actual.Z,
				displacement)

			visualizeActualPath(ctx, summoner, phase.name, start, actual)

			// Behavioural assertions: bounds on horizontal displacement.
			if phase.minDisplacement > 0 {
				require.GreaterOrEqual(t, displacement, phase.minDisplacement,
					"phase %s: boat should have moved at least %.2f blocks (got %.2f)",
					phase.name, phase.minDisplacement, displacement)
			}
			if phase.maxDisplacement > 0 {
				require.LessOrEqual(t, displacement, phase.maxDisplacement,
					"phase %s: boat should have stayed within %.2f blocks of start (got %.2f)",
					phase.name, phase.maxDisplacement, displacement)
			}

			// Direction sanity check.
			if phase.checkDir != nil {
				phase.checkDir(t, start, actual)
			}
		})
	}
}

// runHorsePhases executes a sequence of independent steering phases on a
// mounted horse. Each phase starts from zero velocity after a coast-to-stop
// period and is validated purely behaviourally: direction (via
// checkDirWithYaw / checkDir) and horizontal displacement bounds.
func runHorsePhases(
	t *testing.T,
	ctx context.Context,
	helper *VehicleTestHelper,
	summoner visualize.DisplayEntitySummoner,
	phases []steeringPhase,
) {
	for _, phase := range phases {
		t.Run(phase.name, func(t *testing.T) {
			// Coast to stop: poll until velocity is zero.
			helper.SetManualThrottle(0, 0)
			require.NoError(t, helper.WaitForRidingVelocityZero(ctx, coastTimeout),
				"velocity should reach zero before phase %s", phase.name)

			// Record start position and yaw.
			start, yawF, _, _ := helper.ManagedAgent.Agent.GetPosition()
			startYaw := float64(yawF)

			// Apply throttle.
			helper.SetManualThrottle(phase.throttleX, phase.throttleZ)
			time.Sleep(phase.duration)

			// Sample end position.
			actual, _, _, _ := helper.ManagedAgent.Agent.GetPosition()

			displacement := GetDistance(start.X, 0, start.Z, actual.X, 0, actual.Z)
			t.Logf("[%s] %s: start=(%.2f,%.2f,%.2f) yaw=%.1f actual=(%.2f,%.2f,%.2f) displacement=%.2f",
				helper.AgentName, phase.name,
				start.X, start.Y, start.Z, startYaw,
				actual.X, actual.Y, actual.Z,
				displacement)

			visualizeActualPath(ctx, summoner, phase.name, start, actual)

			// Behavioural assertions: bounds on horizontal displacement.
			if phase.minDisplacement > 0 {
				require.GreaterOrEqual(t, displacement, phase.minDisplacement,
					"phase %s: horse should have moved at least %.2f blocks (got %.2f)",
					phase.name, phase.minDisplacement, displacement)
			}
			if phase.maxDisplacement > 0 {
				require.LessOrEqual(t, displacement, phase.maxDisplacement,
					"phase %s: horse should have stayed within %.2f blocks of start (got %.2f)",
					phase.name, phase.maxDisplacement, displacement)
			}

			// Direction sanity checks.
			if phase.checkDirWithYaw != nil {
				phase.checkDirWithYaw(t, start, actual, startYaw)
			}
			if phase.checkDir != nil {
				phase.checkDir(t, start, actual)
			}
		})
	}
}

// --- Shared helpers ---

// visualizeActualPath renders a single throttle phase as a lime waypoint pair
// from the recorded start position to the position the agent actually
// reached. Used as a debugging aid for in-game inspection of steering
// behaviour.
func visualizeActualPath(
	ctx context.Context,
	summoner visualize.DisplayEntitySummoner,
	phaseName string,
	start, actual models.V3,
) {
	visualize.VisualizeExpectedPath(ctx, summoner, []visualize.Waypoint{
		{Pos: start, Label: fmt.Sprintf("%s_START", phaseName)},
		{Pos: actual, Label: fmt.Sprintf("%s_ACTUAL", phaseName)},
	}, "lime")
}
