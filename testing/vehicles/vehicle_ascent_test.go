package vehicles

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// ascendingSlopeSteps is the number of 1-block-high steps a mount must climb in
// the staircase built by buildAscendingStaircase. Each step rises one block, so
// the top sits ascendingSlopeSteps blocks above the mount's starting level.
const ascendingSlopeSteps = 6

// buildAscendingStaircase clears a wide area around the spawn and builds a
// staircase climbing in the +Z direction: each step is one block higher than
// the previous, so a mount walking +Z (via LookAt) must repeatedly step up.
//
// The path is 9 blocks wide in X with NO side walls — walls would stall a
// ridden camel (see TestCamelGravityOffCliff) — so the width tolerates minor
// heading drift while a wide (1.7-block) mount climbs. A flat landing caps the
// top so the mount settles instead of walking straight off the last step.
func buildAscendingStaircase(t *testing.T, helper *VehicleTestHelper, ctx context.Context, baseX, baseY, baseZ float64) {
	t.Helper()
	cx, cy, cz := int(baseX), int(baseY), int(baseZ)

	execFill := func(desc, cmd string) {
		resp, err := helper.Instance.RCON.Exec(ctx, cmd)
		require.NoError(t, err, desc)
		require.NoError(t, rconResponseError(cmd, resp), desc)
		t.Logf("%s => %s", cmd, resp)
	}

	// Clear a tall, wide box so nothing obstructs the climb or the mount's head.
	execFill("clear ascent area", fmt.Sprintf("fill %d %d %d %d %d %d air",
		cx-5, cy, cz-4, cx+5, cy+ascendingSlopeSteps+4, cz+ascendingSlopeSteps+6))

	// Flat standing area at y-1 so the mount stands at base level (y).
	execFill("build flat start", fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
		cx-4, cy-1, cz-3, cx+4, cy-1, cz+1))

	// Staircase: step k sits at z=cz+1+k, filled from y-1 up to (cy-1+k) so its
	// top surface is at cy+k — one block higher than step k-1.
	for step := 1; step <= ascendingSlopeSteps; step++ {
		topY := cy - 1 + step
		z := cz + 1 + step
		execFill(fmt.Sprintf("build staircase step %d", step),
			fmt.Sprintf("fill %d %d %d %d %d %d grass_block", cx-4, cy-1, z, cx+4, topY, z))
	}

	// Flat landing at the top height so the mount ends grounded rather than
	// immediately walking off the last step.
	topLanding := cy - 1 + ascendingSlopeSteps
	execFill("build top landing", fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
		cx-4, cy-1, cz+ascendingSlopeSteps+2, cx+4, topLanding, cz+ascendingSlopeSteps+35))
}

// climbAscendingSlope drives the already-mounted, manual-mode agent forward up
// the staircase built by buildAscendingStaircase and asserts it gains height
// gradually without launching. baseY is the mount's starting feet level.
func climbAscendingSlope(t *testing.T, helper *VehicleTestHelper, label string, baseY float64, ctx context.Context) {
	t.Helper()
	startTime := time.Now()
	maxY := -1000.0
	minY := 1000.0
	sampleCount := 0

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	done := false
	for !done {
		select {
		case <-ctx.Done():
			t.Fatalf("Context cancelled")
		case <-ticker.C:
			// Drive forward for the whole window; the mount climbs the stairs.
			if time.Since(startTime) > 6*time.Second {
				ticker.Stop()
				done = true
				break
			}
			helper.SetManualThrottle(0, 1.0) // forward, up the stairs

			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			sampleCount++
			if pos.Y > maxY {
				maxY = pos.Y
			}
			if pos.Y < minY {
				minY = pos.Y
			}
			log.Printf("[%s] t=%.2fs Y=%.2f", label, time.Since(startTime).Seconds(), pos.Y)
		}
	}
	helper.SetManualThrottle(0, 0)

	require.Greater(t, sampleCount, 10, "should collect enough position samples")
	log.Printf("[%s] Position range: Y from %.2f to %.2f (%d samples)", label, minY, maxY, sampleCount)

	// The mount must still be mounted after the climb.
	require.True(t, helper.ManagedAgent.Agent.IsMounted(), "agent should remain mounted while climbing")

	// Sustained ascent: the mount should climb at least 3 blocks above where it
	// started (it walked up the staircase rather than being stopped at the base).
	require.GreaterOrEqual(t, maxY, baseY+3.0,
		"%s should climb at least 3 blocks up the slope (maxY=%.2f baseY=%.2f)", label, maxY, baseY)

	// It must climb grounded, not launch off the stairs: the top sits at
	// baseY+ascendingSlopeSteps, so exceeding that by more than ~2 blocks
	// indicates a spurious upward launch rather than a step-by-step climb.
	require.LessOrEqual(t, maxY, baseY+float64(ascendingSlopeSteps)+2.0,
		"%s should climb grounded, not launch into the air (maxY=%.2f baseY=%.2f)", label, maxY, baseY)

	t.Logf("✓ %s climbed %.2f blocks up the slope", label, maxY-baseY)
}

// TestHorseAscendingSlope verifies that a ridden horse can climb a sustained
// staircase (repeated 1-block step-ups) while staying grounded — the ascent
// counterpart to TestHorseGravityOffCliff.
func TestHorseAscendingSlope(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "HorseAscendBot")
			defer cleanup()

			agentPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			baseX, baseY, baseZ := agentPos.X, agentPos.Y, agentPos.Z

			log.Printf("[TestHorseAscendingSlope] Building staircase...")
			buildAscendingStaircase(t, helper, ctx, baseX, baseY, baseZ)
			time.Sleep(1 * time.Second)

			horseID, err := helper.SummonHorse(ctx, baseX, baseY, baseZ, 240)
			require.NoError(t, err, "spawn horse")
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, horseID)
			require.NoError(t, err, "mount horse")
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			// Turn the agent's BODY to face up the staircase (+Z) AFTER mounting.
			// The mounted movement direction is the body (physics-state) yaw the
			// riding handler reads; LookAt only turns the head and would leave the
			// mount pointed wherever it wandered before we mounted, so forward
			// throttle would drive the wrong way. TurnTowards sets that yaw.
			err = helper.ManagedAgent.Agent.TurnTowards(ctx, baseX, baseY, baseZ+float64(ascendingSlopeSteps)+5)
			require.NoError(t, err, "face up the slope")
			time.Sleep(500 * time.Millisecond)

			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")
			defer func() {
				_ = helper.ExitManualMode()
			}()

			climbAscendingSlope(t, helper, "TestHorseAscendingSlope", baseY, ctx)
		})
	}
}

// TestCamelAscendingSlope verifies that a ridden camel can climb a sustained
// staircase while staying grounded. The camel is boosted to a decisive speed
// (its vanilla 0.09 is too slow to climb reliably within the window), matching
// TestCamelGravityOffCliff. This exercises the wide (1.7-block) camel hitbox on
// a multi-step ascent.
func TestCamelAscendingSlope(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			helper, ctx, cleanup := NewVehicleTestHelper(t, tt.MCVersion, "CamelAscendBot")
			defer cleanup()

			agentPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			baseX, baseY, baseZ := agentPos.X, agentPos.Y, agentPos.Z

			log.Printf("[TestCamelAscendingSlope] Building staircase...")
			buildAscendingStaircase(t, helper, ctx, baseX, baseY, baseZ)
			time.Sleep(1 * time.Second)

			camelID, err := helper.SummonCamel(ctx, baseX, baseY, baseZ, 180)
			require.NoError(t, err, "spawn camel")
			time.Sleep(500 * time.Millisecond)

			// Boost movement speed so the slow vanilla camel (0.09) climbs
			// decisively within the sampling window; the agent reads this from
			// the entity attribute.
			speedResp, err := helper.Instance.RCON.Exec(ctx, "attribute @e[type=minecraft:camel,limit=1] minecraft:movement_speed base set 0.3")
			require.NoError(t, err, "set camel movement speed")
			require.NoError(t, rconResponseError("set camel movement speed", speedResp), "set camel movement speed")
			time.Sleep(500 * time.Millisecond)

			err = helper.MountEntity(ctx, camelID)
			require.NoError(t, err, "mount camel")
			err = helper.WaitForMounted(ctx, 5*time.Second)
			require.NoError(t, err, "agent should be mounted")

			// Turn the agent's BODY to face up the staircase (+Z) AFTER mounting.
			// The mounted movement direction is the body (physics-state) yaw the
			// riding handler reads; LookAt only turns the head and would leave the
			// mount pointed wherever it wandered before we mounted, so forward
			// throttle would drive the wrong way. TurnTowards sets that yaw.
			err = helper.ManagedAgent.Agent.TurnTowards(ctx, baseX, baseY, baseZ+float64(ascendingSlopeSteps)+5)
			require.NoError(t, err, "face up the slope")
			time.Sleep(500 * time.Millisecond)

			err = helper.EnterManualMode()
			require.NoError(t, err, "enter manual mode")
			defer func() {
				_ = helper.ExitManualMode()
			}()

			climbAscendingSlope(t, helper, "TestCamelAscendingSlope", baseY, ctx)
		})
	}
}
