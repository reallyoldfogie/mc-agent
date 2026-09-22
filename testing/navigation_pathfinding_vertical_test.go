package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// PathfindingVerticalFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the staircase-pathfinding vertical
// movement test: one server per version, shared by every test method below
// (currently one), instead of the previous per-test-function
// StartServer/StopServer pattern. WorldGen = WorldGenFlat, matching the
// pre-conversion test's own FlatWorldServerConfig(). Kept as its own new
// suite rather than folded into NavigationFlatSuite: this test's own
// VIEW_DISTANCE=12 override (carried forward via VersionWorldSuite.ExtraEnv,
// added for docs/plans/integration-test-shared-server/23-phase1-inventory-conversion.md)
// is suite-wide, not per-method - folding it into NavigationFlatSuite would
// silently change every one of that suite's already-passing methods' own
// VIEW_DISTANCE (6, via SharedFlatWorldServerConfig) with no evidence
// that's safe for them.
type PathfindingVerticalFlatSuite struct {
	VersionWorldSuite
}

func TestPathfindingVerticalFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &PathfindingVerticalFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.ExtraEnv = map[string]string{"VIEW_DISTANCE": "12"}
		return s
	})
}

// block2Chunk converts block coordinates to chunk coordinates.
func block2Chunk(blockX, blockZ int64) (chunkX, chunkZ int64) {
	chunkX = (int64)(math.Floor(float64(blockX) / 16))
	chunkZ = (int64)(math.Floor(float64(blockZ) / 16))
	return
}

// TestPathfindingVerticalMovement tests navigation with elevation changes
// using pathfinding: a gradual staircase is built from the agent's own
// working area toward a destination 70 blocks over and 17-ish blocks up,
// and the agent is commanded to MoveTo it. Equivalent to the pre-Phase-1
// TestPathfindingVerticalMovement (navigation_pathfinding_test.go).
//
// The pre-conversion test teleported to a fixed, literal (-8.50, 0.00, 5.50)
// before reading its own "start" position - a magic constant near world
// origin that made sense when this test owned the whole server (and,
// post-conversion, would sit dangerously close to the world's own fixed
// spawn point - see
// docs/plans/integration-test-shared-server/25-never-assign-spawn-point-as-working-area.md
// for why nothing should ever target that deliberately). Flat-world terrain
// is uniform everywhere, so nothing about the test's own logic actually
// depends on that specific X/Z - this uses leader.Origin (this test's own
// working area) as "start" instead, dropping the fixed teleport entirely.
func (s *PathfindingVerticalFlatSuite) TestPathfindingVerticalMovement() {
	t := s.T()
	logger := NewTestLogger(t)

	const (
		distBetweenStairs = 4 // how often to place a stair block in the staircase
		xDiff             = 70
		yDiff             = xDiff / distBetweenStairs
		zDiff             = 3
	)
	var resp string
	var err error

	leader, err := s.SpawnWorkingAreaAgent("PathfindingVerticalBot", "nav_pathfinding_vertical")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s log: %s", leader.Name, s.Framework.GetAgentLogFilename())

	start := leader.Origin
	startX, startY, startZ := start.X, start.Y, start.Z

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	// Navigate to higher elevation - build stairs to make path possible.
	destination := models.V3{
		X: startX + xDiff,
		Y: startY + yDiff,
		Z: startZ + zDiff,
	}

	points := utils.Line(start, destination)
	toBeLoaded := make(map[string]models.V3)
	var i, j int64
	for _, point := range points {
		chunkX, chunkZ := block2Chunk(int64(point.X)+i, int64(point.Z)+j)
		toBeLoaded[fmt.Sprintf("%d, %d", chunkX, chunkZ)] = point
	}

	// Forceload chunks before building to ensure chunks are loaded.
	for _, point := range toBeLoaded {
		blockX := int64(point.X) << 4 // forceload uses block coordinates
		blockZ := int64(point.Z) << 4 // forceload uses block coordinates
		forceloadCmd := fmt.Sprintf("forceload add %d %d", blockX, blockZ)
		logger.Logf("Executing %s", forceloadCmd)
		_, err = s.Inst.RCON.ExecuteWithRetry(s.Ctx, forceloadCmd, 3)
		if err != nil {
			logger.Logf("Warning: Failed to forceload chunks: %v", err)
		}
		time.Sleep(1 * time.Second) // Wait for chunks to load
	}

	resp, err = s.Inst.RCON.ExecuteWithRetry(s.Ctx, fmt.Sprintf(`summon block_display %f %f %f {block_state:{Name:"minecraft:diamond_block"}}`, destination.X, destination.Y, destination.Z), 3)
	if err != nil {
		logger.Logf("Warning: Failed to place display_block at (%f, %f, %f): %v [%s]", destination.X, destination.Y, destination.Z, err, resp)
	}

	// Build a platform at the destination.
	resp, err = s.Inst.RCON.ExecuteWithRetry(s.Ctx, fmt.Sprintf(`fill %d %d %d %d %d %d minecraft:stone_slab[type=top]`, int(destination.X)-3, int(destination.Y)-1, int(destination.Z)-5, int(destination.X)+5, int(destination.Y)-1, int(destination.Z)+5), 3)
	if err != nil {
		logger.Logf("Warning: Failed to fill stone_slab[type=top] at (%d, %d, %d => %d, %d, %d): %v [%s]", int(destination.X), int(destination.Y), int(destination.Z)-5, int(destination.X)+5, int(destination.Y), int(destination.Z)+5, err, resp)
	} else {
		logger.Logf("Filled stone_slab[type=top] at (%d, %d, %d => %d, %d, %d) [%s]\n", int(destination.X), int(destination.Y), int(destination.Z)-5, int(destination.X)+5, int(destination.Y), int(destination.Z)+5, resp)
	}

	// BUILD STAIRS to destination (fix for flat terrain).
	// Build a gradual staircase: 70 blocks horizontal, ~17 blocks vertical.
	logger.Logf("Building staircase from start to destination")
	stairZ := int(destination.Z)
	for i := 1; i <= xDiff; i++ {
		blockX := int(startX) + i
		blockY := int(startY) + (i / distBetweenStairs) // Gradual ascent: 1 block up every <distBetweenStairs> blocks horizontal
		blockZ := stairZ
		retry := 0

	RETRY:
		if blockY < int(destination.Y) {
			// place distBetweenStairs-1 slabs, then a stair
			if i == 1 || i%distBetweenStairs == 0 { // Place stair block facing east
				resp, err = s.Inst.RCON.ExecuteWithRetry(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone_stairs[facing=east,half=bottom]", blockX, blockY, blockZ), 3)
				if err != nil {
					str := fmt.Sprintf("Warning: Failed to place stone_stair[facing=east,half=bottom] at (%d, %d, %d): %v [%s]", blockX, blockY, blockZ, err, resp)
					logger.Log(str)
					time.Sleep(500 * time.Millisecond)
					retry++
					if retry < 4 {
						s.Inst.RCON.Reconnect(s.Ctx)
						goto RETRY
					}
				} else {
					logger.Logf("Placed stone_stair[facing=east,half=bottom] at (%d, %d, %d) [%s]", blockX, blockY, blockZ, resp)
				}

			} else { // Place slab
				resp, err = s.Inst.RCON.ExecuteWithRetry(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone_slab[type=top]", blockX, blockY, blockZ), 3)
				if err != nil {
					str := fmt.Sprintf("Warning: Failed to place stone_slab[type=top] at (%d, %d, %d): %v [%s]", blockX, blockY, blockZ, err, resp)
					logger.Log(str)
					time.Sleep(500 * time.Millisecond)
					retry++
					if retry < 4 {
						s.Inst.RCON.Reconnect(s.Ctx)
						goto RETRY
					}
				} else {
					logger.Logf("Placed stone_slab[type=top] at (%d, %d, %d) [%s]", blockX, blockY, blockZ, resp)
				}
			}
		}
	}
	logger.Logf("Staircase built, waiting for chunks to sync")
	time.Sleep(10 * time.Second)

	logger.Logf("Navigating from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f)",
		startX, startY, startZ, destination.X, destination.Y, destination.Z)

	s.Inst.RCON.Say(s.Ctx, fmt.Sprintf("Executing moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z))

	err = leader.Agent.MoveTo(s.Ctx, destination.X, destination.Y, destination.Z, true)
	assert.NoError(t, err, "moveTo should succeed")

	s.Inst.RCON.Say(s.Ctx, fmt.Sprintf("Done executing moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)).Exec(s.Ctx)

	// Verify the agent's location.
	currentPos, ok := tracker.GetPosition(leader.Name)
	require.True(t, ok, "should have current position after MoveTo command")
	s.Inst.RCON.Say(s.Ctx, fmt.Sprintf("current position after MoveTo command: %.2f, %.2f, %.2f", currentPos.X, currentPos.Y, currentPos.Z))
	logger.Logf("Current position after MoveTo command: %.2f, %.2f, %.2f", currentPos.X, currentPos.Y, currentPos.Z)

	// Longer timeout for vertical movement.
	distance := models.V3{X: startX, Y: startY, Z: startZ}.DistanceTo(destination)
	timeout := CalculateMovementTimeout(distance) * 2

	err = tracker.WaitForPosition(s.Ctx, leader.Name, destination, 2.0, timeout)
	if err != nil {
		logger.Logf("Note: Agent did not reach elevated destination : %v", err)
	}

	// Check progress instead of requiring full success.
	finalPos, ok := tracker.GetPosition(leader.Name)
	require.True(t, ok, "should have final position")

	horizontalProgress := (finalPos.X - startX) / (destination.X - startX)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalPos.X, finalPos.Y, finalPos.Z)
	logger.Logf("Horizontal progress: %.0f%%", horizontalProgress*100)

	// With pathfinding, agent should make progress in the right direction.
	assert.Greater(t, horizontalProgress, 0.3, "agent should make at least 30% progress toward destination")

	time.Sleep(5 * time.Second) // Allow time to observe final state before test ends
}
