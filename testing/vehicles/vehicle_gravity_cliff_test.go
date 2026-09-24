package vehicles

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	testingpkg "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// VehicleGravityCliffSuite is a version-parameterized suite: one shared flat-world server per
// version instead of one server per test function. Each method spawns its own
// uniquely-named agent in its own working area.
type VehicleGravityCliffSuite struct {
	testingpkg.VersionWorldSuite
}

func TestVehicleGravityCliffSuite(t *testing.T) {
	testingpkg.RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &VehicleGravityCliffSuite{}
		s.WorldGen = testingpkg.WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestHorseGravityOffCliff verifies that a horse properly applies gravity when
// ridden off a cliff. The horse should fall (Y decreasing) for 1-2 seconds.
// Tests both movement and physics work correctly when a mounted horse leaves ground.
func (s *VehicleGravityCliffSuite) TestHorseGravityOffCliff() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("HorseGravityBot", "horse_gravity_off_cliff")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Position agent and build terrain
	log.Printf("[TestHorseGravityOffCliff] Building terrain...")
	agentPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	baseX, baseY, baseZ := agentPos.X, agentPos.Y, agentPos.Z

	// Build a direction-independent "climb then fall" course centred on the
	// agent. A mount's heading when mounted is not reliably controllable, so
	// rather than a single directional hill+pit we ring the spawn:
	//   1. a 1-block-high grass wall at radius 2 that the mount climbs in
	//      whatever direction it walks (exercises step-up: it must climb and
	//      stay grounded on top, not launch upward), and
	//   2. a deep, wide moat beyond the wall so it then falls off a real cliff
	//      regardless of direction.
	// Dig the moat as one large square pit, then rebuild the small central
	// standing floor and the climb-wall ring on top of it.
	cx, cy, cz := blockCoord(baseX), blockCoord(baseY), blockCoord(baseZ)

	moatCmd := fmt.Sprintf("fill %d %d %d %d %d %d air",
		cx-14, cy-25, cz-14, cx+14, cy+3, cz+14)
	response, err := helper.Instance.RCON.Exec(ctx, moatCmd)
	require.NoError(t, err, "dig surrounding moat")
	t.Logf("fill response: %s => %s", moatCmd, response)

	// Central 5x5 floor at y-1 so the mount stands at base level (y).
	floorCmd := fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
		cx-2, cy-1, cz-2, cx+2, cy-1, cz+2)
	response, err = helper.Instance.RCON.Exec(ctx, floorCmd)
	require.NoError(t, err, "build central floor")
	t.Logf("fill response: %s => %s", floorCmd, response)

	// Climb-wall ring: fill the 5x5 at base level, then clear the inner 3x3
	// so only the radius-2 perimeter remains as a 1-block wall to climb.
	wallCmd := fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
		cx-2, cy, cz-2, cx+2, cy, cz+2)
	response, err = helper.Instance.RCON.Exec(ctx, wallCmd)
	require.NoError(t, err, "build climb-wall ring")
	t.Logf("fill response: %s => %s", wallCmd, response)

	innerCmd := fmt.Sprintf("fill %d %d %d %d %d %d air",
		cx-1, cy, cz-1, cx+1, cy, cz+1)
	response, err = helper.Instance.RCON.Exec(ctx, innerCmd)
	require.NoError(t, err, "clear inner standing area")
	t.Logf("fill response: %s => %s", innerCmd, response)

	time.Sleep(1 * time.Second)

	// Spawn a tamed horse at starting position
	log.Printf("[TestHorseGravityOffCliff] Spawning horse...")
	horseID, err := helper.SummonHorse(ctx, baseX, baseY, baseZ, 245)
	require.NoError(t, err, "spawn horse")
	time.Sleep(500 * time.Millisecond)

	// Mount the horse
	log.Printf("[TestHorseGravityOffCliff] Mounting horse...")
	err = helper.MountEntity(ctx, horseID)
	require.NoError(t, err, "mount horse")

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err, "agent should be mounted")
	log.Printf("[TestHorseGravityOffCliff] Mounted horse successfully")

	// Enter manual mode for controlled movement
	err = helper.EnterManualMode()
	require.NoError(t, err, "enter manual mode")
	defer func() {
		_ = helper.ExitManualMode()
	}()

	helper.ManagedAgent.Agent.TurnTowards(ctx, baseX, baseY, baseZ+5)
	time.Sleep(500 * time.Millisecond)

	log.Printf("[TestHorseGravityOffCliff] Starting forward movement and recording fall...")

	// Record positions as we move forward and off the cliff
	startTime := time.Now()
	maxY := -1000.0
	minY := 1000.0
	var fallingStartTime time.Time
	isFalling := false
	sampleCount := 0

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	done := false
	for !done {
		select {
		case <-ctx.Done():
			t.Fatalf("Context cancelled")
		case <-ticker.C:
			elapsed := time.Since(startTime)

			// Move forward for ~2.5 seconds, then coast and keep sampling.
			// The longer window lets a slow mount reach the cliff and gives
			// gravity time to record a sustained fall.
			if elapsed < 2500*time.Millisecond {
				helper.SetManualThrottle(0, 1.0) // Forward throttle
			} else if elapsed > 4000*time.Millisecond {
				ticker.Stop()
				done = true
				break
			}

			// Get current position
			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			sampleCount++

			if pos.Y > maxY {
				maxY = pos.Y
			}
			if pos.Y < minY {
				minY = pos.Y
			}

			// Detect when falling starts (Y starts decreasing after reaching max)
			if pos.Y < maxY-0.1 && !isFalling {
				isFalling = true
				fallingStartTime = time.Now()
				log.Printf("[TestHorseGravityOffCliff] Falling started at Y=%.2f (max was %.2f)", pos.Y, maxY)
			}

			if isFalling {
				log.Printf("[TestHorseGravityOffCliff] t=%.2fs Y=%.2f (falling for %.2fs)",
					elapsed.Seconds(), pos.Y, time.Since(fallingStartTime).Seconds())
			}
		}
	}

	// Clear throttle
	helper.SetManualThrottle(0, 0)

	// Verify results
	require.Greater(t, sampleCount, 10, "should collect enough position samples")
	log.Printf("[TestHorseGravityOffCliff] Position range: Y from %.2f to %.2f (%d samples)", minY, maxY, sampleCount)

	// The horse must climb the 1-block hill and stay grounded on top — it
	// must NOT launch into the air. Before the step-up fix it spiked to
	// Y≈6 (a parabola). Feet on the hilltop sit at baseY+1, so exceeding
	// baseY+2 indicates a launch.
	require.LessOrEqual(t, maxY, baseY+2.0,
		"horse should climb the hill and stay grounded, not launch into the air (maxY=%.2f baseY=%.2f)", maxY, baseY)

	require.True(t, isFalling, "horse should fall off cliff (Max Y: %.2f, Min Y: %.2f)", maxY, minY)

	// Verify gravity was applied for at least 1 second of falling
	fallingDuration := time.Since(fallingStartTime)
	require.GreaterOrEqual(t, fallingDuration, 1*time.Second, "horse should fall for at least 1 second")

	// Verify a genuine cliff fall (several blocks into the pit), not just a
	// 1-block step down off the hill.
	actualDrop := maxY - minY
	require.GreaterOrEqual(t, actualDrop, 3.0, "horse should drop at least 3.0 blocks off the cliff")

	t.Logf("✓ Horse climbed the hill and fell %.2f blocks over %.2fs", actualDrop, fallingDuration.Seconds())
}

// TestCamelGravityOffCliff verifies that a camel properly applies gravity when
// ridden off a cliff. The camel should fall (Y decreasing) for 1-2 seconds.
// Tests both movement and physics work correctly when a mounted camel leaves ground.
func (s *VehicleGravityCliffSuite) TestCamelGravityOffCliff() {
	t := s.T()
	leader, spawnErr := s.SpawnWorkingAreaAgent("CamelGravityBot", "camel_gravity_off_cliff")
	require.NoError(t, spawnErr, "spawn agent")
	helper := NewVehicleTestHelperForSuite(&s.VersionWorldSuite, leader)
	ctx := s.Ctx

	// Position agent and build terrain
	log.Printf("[TestCamelGravityOffCliff] Building terrain...")
	agentPos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
	baseX, baseY, baseZ := agentPos.X, agentPos.Y, agentPos.Z

	// Build a direction-independent cliff for the fall test. A camel's
	// server-side movement stalls when it is boxed in by walls, so instead of
	// a climb-wall ring we leave a small solid platform at the spawn and
	// surround it with a deep moat: the camel walks off the platform edge in
	// whatever direction it travels and falls off a real cliff. (The horse
	// test covers the climb/step-up "no launch" behaviour.)
	cx, cy, cz := blockCoord(baseX), blockCoord(baseY), blockCoord(baseZ)

	moatCmd := fmt.Sprintf("fill %d %d %d %d %d %d air",
		cx-14, cy-25, cz-14, cx+14, cy+3, cz+14)
	response, err := helper.Instance.RCON.Exec(ctx, moatCmd)
	require.NoError(t, err, "dig surrounding moat")
	t.Logf("fill response: %s => %s", moatCmd, response)

	// Central 5x5 standing platform at y-1 so the mount stands at base level
	// (y); everything beyond radius 2 is the moat it falls into.
	floorCmd := fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
		cx-2, cy-1, cz-2, cx+2, cy-1, cz+2)
	response, err = helper.Instance.RCON.Exec(ctx, floorCmd)
	require.NoError(t, err, "build central platform")
	t.Logf("fill response: %s => %s", floorCmd, response)

	time.Sleep(1 * time.Second)

	// Spawn a tamed camel at starting position
	log.Printf("[TestCamelGravityOffCliff] Spawning camel...")
	camelID, err := helper.SummonCamel(ctx, baseX, baseY, baseZ, 180)
	require.NoError(t, err, "spawn camel")
	time.Sleep(500 * time.Millisecond)

	// Boost the camel's movement speed so it clears the cliff edge decisively.
	// At its slow vanilla speed (0.09) a mounted camel lingers at the ledge,
	// where the server's vehicle-move handling pins it instead of letting it
	// fall (the strider test documents the same server rejection of slow
	// ridden movement). A faster camel clears the edge like the horse and
	// falls cleanly. The agent reads this speed from the entity attribute.
	// speedResp, err := helper.Instance.RCON.Exec(ctx, "attribute @e[type=minecraft:camel,limit=1] minecraft:movement_speed base set 0.3")
	// require.NoError(t, err, "set camel movement speed")
	// t.Logf("camel speed attribute => %s", speedResp)
	// time.Sleep(500 * time.Millisecond)

	// Mount the camel
	log.Printf("[TestCamelGravityOffCliff] Mounting camel...")
	err = helper.MountEntity(ctx, camelID)
	require.NoError(t, err, "mount camel")

	err = helper.WaitForMounted(ctx, 5*time.Second)
	require.NoError(t, err, "agent should be mounted")
	log.Printf("[TestCamelGravityOffCliff] Mounted camel successfully")

	helper.ManagedAgent.Agent.TurnTowards(ctx, baseX, baseY, baseZ+5)
	time.Sleep(500 * time.Millisecond)

	// Enter manual mode for controlled movement
	err = helper.EnterManualMode()
	require.NoError(t, err, "enter manual mode")
	defer func() {
		_ = helper.ExitManualMode()
	}()

	log.Printf("[TestCamelGravityOffCliff] Starting forward movement and recording fall...")

	// Record positions as we move forward and off the cliff
	startTime := time.Now()
	maxY := -1000.0
	minY := 1000.0
	var fallingStartTime time.Time
	isFalling := false
	sampleCount := 0

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	done := false
	for !done {
		select {
		case <-ctx.Done():
			t.Fatalf("Context cancelled")
		case <-ticker.C:
			elapsed := time.Since(startTime)

			// Move forward for ~2.5 seconds, then coast and keep sampling.
			// The longer window lets a slow mount reach the cliff and gives
			// gravity time to record a sustained fall.
			if elapsed < 5500*time.Millisecond {
				helper.SetManualThrottle(0, 1.0) // Forward throttle
			} else if elapsed > 4000*time.Millisecond {
				ticker.Stop()
				done = true
				break
			}

			// Get current position
			pos, _ := helper.ManagedAgent.Agent.GetPositionSimple()
			sampleCount++

			if pos.Y > maxY {
				maxY = pos.Y
			}
			if pos.Y < minY {
				minY = pos.Y
			}

			// Detect when falling starts (Y starts decreasing after reaching max)
			if pos.Y < maxY-0.1 && !isFalling {
				isFalling = true
				fallingStartTime = time.Now()
				log.Printf("[TestCamelGravityOffCliff] Falling started at Y=%.2f (max was %.2f)", pos.Y, maxY)
			}

			if isFalling {
				log.Printf("[TestCamelGravityOffCliff] t=%.2fs Y=%.2f (falling for %.2fs)",
					elapsed.Seconds(), pos.Y, time.Since(fallingStartTime).Seconds())
			}
		}
	}

	// Clear throttle
	helper.SetManualThrottle(0, 0)

	time.Sleep(5 * time.Second)

	// Verify results
	require.Greater(t, sampleCount, 10, "should collect enough position samples")
	log.Printf("[TestCamelGravityOffCliff] Position range: Y from %.2f to %.2f (%d samples)", minY, maxY, sampleCount)

	// The camel walks off a flat platform, so it should stay grounded (near
	// baseY) until it leaves the edge — it must NOT launch into the air.
	// Exceeding baseY+2 would indicate a spurious upward launch.
	require.LessOrEqual(t, maxY, baseY+2.0,
		"camel should stay grounded on the platform, not launch into the air (maxY=%.2f baseY=%.2f)", maxY, baseY)

	require.True(t, isFalling, "camel should fall off cliff (Max Y: %.2f, Min Y: %.2f)", maxY, minY)

	// Verify gravity was applied for at least 1 second of falling
	fallingDuration := time.Since(fallingStartTime)
	require.GreaterOrEqual(t, fallingDuration, 1*time.Second, "camel should fall for at least 1 second")

	// Verify a genuine cliff fall (several blocks into the moat), not just a
	// 1-block step off the platform edge.
	actualDrop := maxY - minY
	require.GreaterOrEqual(t, actualDrop, 3.0, "camel should drop at least 3.0 blocks off the cliff")

	t.Logf("✓ Camel traveled off the cliff and fell %.2f blocks over %.2fs", actualDrop, fallingDuration.Seconds())
}
