package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SneakingFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for sneaking-movement tests: one server per
// version, shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern. WorldGen = WorldGenFlat,
// a deliberate deviation from the pre-conversion tests' own
// DefaultServerConfig() (WorldGenRandom): both methods dig out and rebuild
// their own platform from scratch regardless of what terrain was there, so
// nothing depends on it being "real" - matches the reasoning already used
// for inventory_integration_test.go's conversion. Difficulty is left at
// VersionWorldSuite's own Peaceful default, matching the original.
//
// Both methods build their platform in open sky (see skyPlatformY) rather
// than at natural ground level, the way the pre-conversion tests did (each
// dug a pit straight through the flat world's own ground, down to 5 blocks
// below the surface). That was safe when every test owned its own solo
// server, but is a real, live-confirmed bug on a shared server: whichever
// method's working area happens to land at offset (0,0) - the world's own
// fixed spawn point, since NextWorkingAreaOffset's very first claim needs
// no teleport at all - leaves a hole there for the rest of the suite's run.
// Every subsequent agent that joins this shared server does so at that same
// fixed spawn point before it ever gets its own working-area teleport, so
// it free-falls into the hole and reads a corrupted, several-blocks-too-low
// Y - which then becomes the *fallback* Y for its own teleport
// (VersionWorldSuite.teleportAndSettle's Flat-world branch), landing that
// agent embedded in solid stone at its own, otherwise perfectly fine,
// working area. Confirmed directly: TestSneakingFlatSuite/TestMovementAllowed
// entered the world at pos=(6.50, -6.00, -1.50) - already inside
// TestEdgePrevention's pit - on its very first join packet, before any of
// this suite's own code ran. See
// docs/plans/integration-test-shared-server/24-phase1-sneaking-conversion.md.
type SneakingFlatSuite struct {
	VersionWorldSuite
}

// skyPlatformY is the Y level both methods build their platform at, well
// above any of this suite's WorldGenFlat terrain (top of the flat preset's
// grass layer is Y=-1) - see SneakingFlatSuite's own doc comment for why
// building at natural ground level is unsafe on a shared server.
const skyPlatformY = 99

func TestSneakingFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &SneakingFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// TestEdgePrevention tests that the bot cannot walk off block edges while
// sneaking. Equivalent to the pre-Phase-1 TestSneaking_EdgePrevention.
//
// Spawned NoCam (preserved from the original, which disabled its Cam
// companion for this method but not TestMovementAllowed): the platform here
// is only 3x3, small enough that a companion entity standing on or near it
// risks physically interfering with the precise edge-stopping behavior
// under test, unlike TestMovementAllowed's much larger 11x11 platform.
func (s *SneakingFlatSuite) TestEdgePrevention() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgentNoCam("SneakEdgeBot", "sneaking_edge_prevention")
	require.NoError(t, err, "spawn agent")

	platformX := int(math.Floor(leader.Origin.X))
	platformY := skyPlatformY
	platformZ := int(math.Floor(leader.Origin.Z))

	// Build a 3x3 platform in open sky - no pit to dig, the surrounding air
	// already provides the fall-off-the-edge hazard the test needs (see
	// SneakingFlatSuite's own doc comment for why this isn't built at
	// natural ground level).
	require.NoError(t, BuildPlatform(s.Ctx, s.Inst.RCON, platformX-1, platformY, platformZ-1, 3, 3, "minecraft:grass_block"), "build platform")

	time.Sleep(3 * time.Second)

	// Teleport bot to center of platform.
	tp := fmt.Sprintf(`teleport %s %.1f %.1f %.1f`, leader.Name, float64(platformX)+0.5, float64(platformY+1)+0.5, float64(platformZ)+0.5)
	_, err = s.Inst.RCON.Exec(s.Ctx, tp)
	require.NoError(t, err, "teleport to platform")

	time.Sleep(500 * time.Millisecond)

	initialPos, initialized := leader.Agent.GetPositionSimple()
	require.True(t, initialized, "bot position should be available")

	// Start sneaking.
	err = leader.Agent.StartSneaking()
	require.NoError(t, err, "StartSneaking error")

	// Move forward while sneaking to test edge prevention.
	err = leader.Agent.MoveForward(s.Ctx, 3.0)
	if err != nil { // expected, since the edge prevention should stop movement to the full 3 blocks
		t.Logf("MoveForward error: %v", err)
	}

	// give time to move
	time.Sleep(6 * time.Second)

	// Check bot position - should not have walked off edge, but should have still moved.
	finalPos, initialized := leader.Agent.GetPositionSimple()
	require.True(t, initialized, "bot position should be available")

	distanceMoved := initialPos.DistanceTo(finalPos)
	leader.Agent.SendChat(fmt.Sprintf("Movement test: Moved from %s to %s (%.2f blocks)", initialPos, finalPos, distanceMoved))

	assert.NotEqual(t, initialPos, finalPos)

	// Bot should still be on platform (within reasonable bounds).
	assert.GreaterOrEqual(t, finalPos.Y, float64(platformY)+0.5,
		"bot should still be on platform level after sneaking movement attempt")
}

// TestMovementAllowed tests that the bot can move normally when not near
// edges. Equivalent to the pre-Phase-1 TestSneaking_MovementAllowed.
func (s *SneakingFlatSuite) TestMovementAllowed() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("SneakMoveBot", "sneaking_movement_allowed")
	require.NoError(t, err, "spawn agent")

	platformX := int(math.Floor(leader.Origin.X))
	platformY := skyPlatformY
	platformZ := int(math.Floor(leader.Origin.Z))

	// Build a large (11x11) platform in open sky, far from any edge (see
	// SneakingFlatSuite's own doc comment for why this isn't built at
	// natural ground level).
	require.NoError(t, BuildPlatform(s.Ctx, s.Inst.RCON, platformX-5, platformY, platformZ-5, 11, 11, "minecraft:grass_block"), "build platform")

	time.Sleep(500 * time.Millisecond)

	// Teleport bot to center.
	tp := fmt.Sprintf(`teleport %s %.1f %.1f %.1f`, leader.Name, float64(platformX)+0.5, float64(platformY+1)+0.5, float64(platformZ)+0.5)
	_, err = s.Inst.RCON.Exec(s.Ctx, tp)
	require.NoError(t, err, "teleport to platform")

	time.Sleep(500 * time.Millisecond)

	initialPos, initialized := leader.Agent.GetPositionSimple()
	require.True(t, initialized)

	// Start sneaking and move.
	err = leader.Agent.StartSneaking()
	require.NoError(t, err)

	err = leader.Agent.MoveForward(s.Ctx, 3.0)
	require.NoError(t, err, "bot should be able to move when not near edge")

	// give time to move
	time.Sleep(5 * time.Second)

	// Check bot moved.
	finalPos, initialized := leader.Agent.GetPositionSimple()
	require.True(t, initialized)

	distanceMoved := initialPos.DistanceTo(finalPos)
	leader.Agent.SendChat(fmt.Sprintf("Movement test: Moved from %s to %s (%.2f blocks)", initialPos, finalPos, distanceMoved))

	assert.NotEqual(t, initialPos, finalPos)

	// Should have moved a measurable distance.
	assert.InDeltaf(t, distanceMoved, 3.0, .15,
		"bot should move at least 3 blocks when sneaking and not near edge")
	assert.GreaterOrEqual(t, finalPos.Y, float64(platformY)+0.5,
		"bot should still be on platform")
}
