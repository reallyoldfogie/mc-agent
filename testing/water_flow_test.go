package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// WaterFlowFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for water-current tests: one server per
// version, shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern. WorldGen = WorldGenFlat
// and GameMode = survival, matching every pre-conversion function's own
// explicit choice exactly (no deviation needed here, unlike several earlier
// conversions this session - these tests already picked Flat deliberately).
// Difficulty is left at VersionWorldSuite's own Peaceful default, matching
// the original (never set explicitly).
//
// ExtraEnv carries FORCE_GAMEMODE=true forward via VersionWorldSuite's
// ExtraEnv field (added for inventory_integration_test.go's conversion,
// see docs/plans/integration-test-shared-server/23-phase1-inventory-conversion.md) -
// the same knob these tests' own pre-conversion config set.
//
// None of the four pre-conversion functions enabled a Cam companion
// (each built its AgentConfig as a raw struct literal rather than via
// DefaultAgentConfig, so EnableCamAgent stayed at its Go zero value, false)
// - but unlike attribute_modifier_test.go/perception_nearest_player_test.go,
// there's no evidence this was deliberate (no comment, no functional
// dependency on Cam's absence): every position read in this file is by
// exact bot name via GetPlayerPosition, never "nearest player," so a Cam
// companion standing nearby couldn't affect any of these tests' assertions
// either way. Uses plain SpawnWorkingAreaAgent (Cam enabled), matching this
// session's general convention for every other conversion.
type WaterFlowFlatSuite struct {
	VersionWorldSuite
}

func TestWaterFlowFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &WaterFlowFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestLinearFlow verifies an agent placed in a directional water current
// (flowing +Z, walled on both sides to keep it directional) gets pushed
// south. Equivalent to the pre-Phase-1 TestWaterFlow_LinearFlow.
func (s *WaterFlowFlatSuite) TestLinearFlow() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("WaterFlowBot", "water_flow_linear")
	require.NoError(t, err, "spawn agent")

	// Create a water stream flowing in +Z direction (south).
	// Source block at (startX, startY+1, startZ).
	waterStartX := int(math.Floor(leader.Origin.X))
	waterY := int(math.Floor(leader.Origin.Y)) + 1
	waterStartZ := int(math.Floor(leader.Origin.Z))

	t.Logf("creating water stream at X=%d Y=%d starting Z=%d", waterStartX, waterY, waterStartZ)

	// Place solid block to direct flow: stone at -Z to prevent flow north.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterStartX, waterY, waterStartZ-1))
	require.NoError(t, err, "place stone to direct flow")

	// Place source block.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d water", waterStartX, waterY, waterStartZ))
	require.NoError(t, err, "place water source")

	// Place stone blocks on sides to keep flow directional.
	for z := waterStartZ; z <= waterStartZ+5; z++ {
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterStartX+1, waterY, z))
		require.NoError(t, err, "place stone at x+1 z=%d", z)
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterStartX-1, waterY, z))
		require.NoError(t, err, "place stone at x-1 z=%d", z)
	}

	time.Sleep(1 * time.Second) // Let server process blocks

	// Teleport agent into the flowing water.
	teleportZ := waterStartZ + 2
	teleportCmd := fmt.Sprintf("tp %s %d.5 %d.5 %d.5", leader.Name, waterStartX, waterY, teleportZ)
	t.Logf("teleporting agent: %s", teleportCmd)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport player")
	time.Sleep(500 * time.Millisecond)

	beforePos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position before water movement")
	t.Logf("position before water movement: (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

	// Wait for agent to be pushed by water current.
	time.Sleep(5 * time.Second)

	afterPos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position after water movement")
	t.Logf("position after water movement: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

	// Check that agent moved in +Z direction (toward higher Z values, which is "south" in Minecraft).
	zDisplacement := afterPos.Z - beforePos.Z
	t.Logf("Z displacement: %.2f", zDisplacement)
	require.Greater(t, zDisplacement, 0.01, "agent should move south (positive Z) in water current")
}

// TestSourceSurroundedByLevel1 verifies an agent standing in a water source
// block, surrounded on all 4 sides by the resulting level-1 flowing water,
// doesn't get pushed anywhere (no net flow direction). Equivalent to the
// pre-Phase-1 TestWaterFlow_SourceSurroundedByLevel1.
func (s *WaterFlowFlatSuite) TestSourceSurroundedByLevel1() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("WaterSourceBot", "water_flow_source_level1")
	require.NoError(t, err, "spawn agent")

	waterX := int(math.Floor(leader.Origin.X))
	waterY := int(math.Floor(leader.Origin.Y)) + 1
	waterZ := int(math.Floor(leader.Origin.Z))

	t.Logf("creating source block surrounded by level 1 water at X=%d Y=%d Z=%d", waterX, waterY, waterZ)

	// Place source block in center.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d water", waterX, waterY, waterZ))
	require.NoError(t, err, "place water source")

	// Place flowing water blocks around it (they will be level 1).
	neighbors := [][3]int{
		{waterX + 1, waterY, waterZ},
		{waterX - 1, waterY, waterZ},
		{waterX, waterY, waterZ + 1},
		{waterX, waterY, waterZ - 1},
	}
	for _, n := range neighbors {
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d water", n[0], n[1], n[2]))
		require.NoError(t, err, "place water at (%d,%d,%d)", n[0], n[1], n[2])
	}
	time.Sleep(1 * time.Second)

	// Teleport agent to center (in source block).
	teleportCmd := fmt.Sprintf("tp %s %d.5 %d.5 %d.5", leader.Name, waterX, waterY, waterZ)
	t.Logf("teleporting agent: %s", teleportCmd)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport player")
	time.Sleep(500 * time.Millisecond)

	beforePos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position before water movement")
	t.Logf("position before: (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

	time.Sleep(5 * time.Second)

	afterPos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position after water movement")
	t.Logf("position after: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

	// Should not move significantly (in source block, no net flow direction).
	totalDisplacement := math.Abs(afterPos.X-beforePos.X) + math.Abs(afterPos.Z-beforePos.Z)
	t.Logf("total horizontal displacement: %.2f", totalDisplacement)
	require.Less(t, totalDisplacement, 0.5, "agent in source block should not move significantly")
}

// TestMixedLevels verifies an agent straddling two vertically-stacked water
// currents (feet in one level, head in another, both flowing the same
// direction) still gets pushed by the combined flow. Equivalent to the
// pre-Phase-1 TestWaterFlow_MixedLevels.
func (s *WaterFlowFlatSuite) TestMixedLevels() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("WaterMixedBot", "water_flow_mixed_levels")
	require.NoError(t, err, "spawn agent")

	// Create water at different heights so agent straddles multiple levels.
	// Place source at Y+2 (high), flowing water continuing at Y+1 (low).
	waterX := int(math.Floor(leader.Origin.X))
	waterLowY := int(math.Floor(leader.Origin.Y)) + 1  // Agent's feet will be at Y+1.5 (in this level)
	waterHighY := int(math.Floor(leader.Origin.Y)) + 2 // Agent's head will be at Y+1.5-2.5 (in this level)
	waterZ := int(math.Floor(leader.Origin.Z))

	t.Logf("creating mixed-level water: low at Y=%d, high at Y=%d", waterLowY, waterHighY)

	// Place source block at high Y.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d water", waterX, waterHighY, waterZ))
	require.NoError(t, err, "place water source at high Y")

	// Place stone barriers to direct lateral flow.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX, waterHighY, waterZ-1))
	require.NoError(t, err, "place barrier at high Y z=%d", waterZ-1)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX, waterLowY, waterZ-1))
	require.NoError(t, err, "place barrier at low Y z=%d", waterZ-1)

	for z := waterZ; z <= waterZ+5; z++ {
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX+1, waterLowY, z))
		require.NoError(t, err, "place barrier at x+1 low Y z=%d", z)
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX-1, waterLowY, z))
		require.NoError(t, err, "place barrier at x-1 low Y z=%d", z)
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX+1, waterHighY, z))
		require.NoError(t, err, "place barrier at x+1 high Y z=%d", z)
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX-1, waterHighY, z))
		require.NoError(t, err, "place barrier at x-1 high Y z=%d", z)
	}

	time.Sleep(1 * time.Second)

	LogBlocksInArea(s.Ctx, t, s.Inst.RCON, leader.ManagedAgent, models.V3{X: float64(waterX), Y: float64(waterLowY), Z: float64(waterZ)}, 8)

	// Teleport agent to straddle the water levels (feet in low water, head in high water).
	teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, float64(waterX)+0.5, float64(waterLowY)+0.6, float64(waterZ)+1.5)
	t.Logf("teleporting agent to straddle water levels: %s", teleportCmd)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport player")
	time.Sleep(500 * time.Millisecond)

	beforePos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position before water movement")
	t.Logf("position before (straddling): (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

	time.Sleep(5 * time.Second)

	afterPos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position after water movement")
	t.Logf("position after: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

	// Agent should move in +Z direction from combined water flow (both levels flow in same direction).
	zDisplacement := afterPos.Z - beforePos.Z
	t.Logf("Z displacement: %.2f", zDisplacement)
	require.Greater(t, zDisplacement, 0.01, "agent straddling multiple water levels should move in flow direction")
}

// TestDiagonalFlow verifies an agent placed in a two-walled corner of
// flowing water (barriers on -X and -Z only) gets pushed diagonally, in
// both +X and +Z. Equivalent to the pre-Phase-1 TestWaterFlow_DiagonalFlow.
func (s *WaterFlowFlatSuite) TestDiagonalFlow() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("WaterDiagonalBot", "water_flow_diagonal")
	require.NoError(t, err, "spawn agent")

	// Create water flowing diagonally (+X and +Z).
	waterX := int(math.Floor(leader.Origin.X))
	waterY := int(math.Floor(leader.Origin.Y)) + 1
	waterZ := int(math.Floor(leader.Origin.Z))

	t.Logf("creating diagonal water flow at X=%d Y=%d Z=%d", waterX, waterY, waterZ)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d water", waterX, waterY+1, waterZ))
	require.NoError(t, err, "place water source")

	// Place barriers on -X and -Z sides to keep flow directional.
	for z := waterZ; z <= waterZ+9; z++ {
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX-1, waterY, z))
		require.NoError(t, err, "place barrier at x-1 z=%d", z)
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX-1, waterY+1, z))
		require.NoError(t, err, "place barrier at x-1 z=%d", z)
	}

	for x := waterX; x <= waterX+9; x++ {
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", x, waterY, waterZ-1))
		require.NoError(t, err, "place barrier at x=%d z-1", x)
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", x, waterY+1, waterZ-1))
		require.NoError(t, err, "place barrier at x=%d z-1", x)
	}

	time.Sleep(1 * time.Second)

	// Teleport agent into the diagonal flow.
	teleportCmd := fmt.Sprintf("tp %s %d.5 %d.5 %d.5", leader.Name, waterX+1, waterY, waterZ+1)
	t.Logf("teleporting agent to diagonal flow: %s", teleportCmd)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport player")
	time.Sleep(500 * time.Millisecond)

	beforePos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position before water movement")
	t.Logf("position before: (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

	time.Sleep(5 * time.Second)

	afterPos, err := GetPlayerPosition(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get position after water movement")
	t.Logf("position after: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

	// Agent should move diagonally (both +X and +Z).
	xDisplacement := afterPos.X - beforePos.X
	zDisplacement := afterPos.Z - beforePos.Z
	t.Logf("X displacement: %.2f, Z displacement: %.2f", xDisplacement, zDisplacement)

	require.Greater(t, xDisplacement, 0.01, "agent should move east (+X) in diagonal flow")
	require.Greater(t, zDisplacement, 0.01, "agent should move south (+Z) in diagonal flow")
}
