package testing

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ContainerFinderRandomSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the container-finder tests below: one server per version,
// shared by every method, instead of the previous per-test-function StartServer/StopServer
// pattern. WorldGen = WorldGenRandom + Difficulty = DifficultyPeaceful, matching both
// pre-conversion functions' own DefaultServerConfig() default exactly (unlike every other
// container-bound conversion so far - see 32/33/34-phase1-container-*-conversion.md - this file's
// originals never called setupStandaloneTest* and built their own random-terrain server inline).
// WorldGenRandom uses the pinned default SEED (06-pinned-default-seed.md), so terrain at each
// method's own SpawnWorkingAreaAgent offset is deterministic, not fresh-random per run.
type ContainerFinderRandomSuite struct {
	VersionWorldSuite
}

func TestContainerFinderRandomSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ContainerFinderRandomSuite{}
		s.WorldGen = WorldGenRandom
		s.Difficulty = DifficultyPeaceful
		return s
	})
}

// TestFindContainersNearby verifies items.ContainerFinder locates multiple
// nearby containers by radius and by type. Equivalent to the pre-Phase-1
// TestFindContainersNearby.
func (s *ContainerFinderRandomSuite) TestFindContainersNearby() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("FinderBot", "container_finder_nearby")
	require.NoError(t, err, "spawn agent")

	searchPos := leader.Origin
	baseX := int(math.Floor(searchPos.X))
	baseY := int(math.Floor(searchPos.Y))
	baseZ := int(math.Floor(searchPos.Z))
	chest1Pos := models.V3{X: float64(baseX + 2), Y: float64(baseY), Z: float64(baseZ)}
	chest2Pos := models.V3{X: float64(baseX - 3), Y: float64(baseY), Z: float64(baseZ)}
	chest3Pos := models.V3{X: float64(baseX), Y: float64(baseY), Z: float64(baseZ + 4)}
	barrelPos := models.V3{X: float64(baseX + 1), Y: float64(baseY + 1), Z: float64(baseZ)}

	t.Log("placing containers around player")
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, chest1Pos, "minecraft:chest", "minecraft:chest", 10*time.Second)
	require.NoError(t, err, "place chest1")
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, chest2Pos, "minecraft:chest", "minecraft:chest", 10*time.Second)
	require.NoError(t, err, "place chest2")
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, chest3Pos, "minecraft:trapped_chest", "minecraft:trapped_chest", 10*time.Second)
	require.NoError(t, err, "place trapped chest")
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, barrelPos, "minecraft:barrel", "minecraft:barrel", 10*time.Second)
	require.NoError(t, err, "place barrel")

	worldMgr := leader.Agent.GetWorld()
	require.NotNil(t, worldMgr, "world manager should be available")

	blockMgr := leader.Config.BlockMgr
	require.NotNil(t, blockMgr, "block manager should be available")

	containerFinder := items.NewContainerFinder(worldMgr, blockMgr)

	t.Log("searching for containers within radius 5")
	containers := containerFinder.FindContainersNearby(searchPos, 5)

	t.Logf("found %d containers", len(containers))
	for i, container := range containers {
		t.Logf("  [%d] %s at (%.0f, %.0f, %.0f)", i, container.Name, container.Position.X, container.Position.Y, container.Position.Z)
	}

	require.GreaterOrEqual(t, len(containers), 4, "should find at least 4 containers")

	t.Log("finding nearest container")
	nearest, found := containerFinder.FindNearestContainer(searchPos, 5)
	require.True(t, found, "should find at least one container")
	t.Logf("nearest container: %s at (%.0f, %.0f, %.0f)", nearest.Name, nearest.Position.X, nearest.Position.Y, nearest.Position.Z)

	nearestDist := searchPos.DistanceTo(nearest.Position)
	for _, container := range containers {
		dist := searchPos.DistanceTo(container.Position)
		require.LessOrEqual(t, nearestDist, dist, "nearest container should be closest")
	}

	t.Log("finding only chests")
	chests := containerFinder.FindContainersByType(searchPos, 5, "chest")
	t.Logf("found %d chests", len(chests))
	require.GreaterOrEqual(t, len(chests), 3, "should find at least 3 chests")

	for _, chest := range chests {
		require.True(t, items.IsChest(chest.Name), "container should be a chest: %s", chest.Name)
	}

	t.Log("finding only barrels")
	barrels := containerFinder.FindContainersByType(searchPos, 5, "barrel")
	t.Logf("found %d barrels", len(barrels))
	require.GreaterOrEqual(t, len(barrels), 1, "should find at least 1 barrel")

	t.Log("✓ Find containers nearby test passed")
}

// TestFindAndOpenContainer verifies items.ContainerFinder locates a
// specific chest, that it can then be opened, and that an item seeded into
// it can be taken into the player's inventory. Equivalent to the
// pre-Phase-1 TestFindAndOpenContainer.
func (s *ContainerFinderRandomSuite) TestFindAndOpenContainer() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("FindOpenBot", "find_open_container")
	require.NoError(t, err, "spawn agent")

	playerPos := leader.Origin

	// Preserved verbatim from the pre-Phase-1 original: an unformatted RCON
	// command (literal "%d" placeholders, no args) that no-ops rather than
	// clearing anything - harmless since the real clear immediately below
	// does the actual work, but not "fixed" here since this conversion is a
	// mechanical port, not a bug sweep.
	s.Inst.RCON.Exec(s.Ctx, "fill %d %d %d %d %d %d minecraft air")

	baseX := int(math.Floor(playerPos.X))
	baseY := int(math.Floor(playerPos.Y))
	baseZ := int(math.Floor(playerPos.Z))
	chestPos := models.V3{X: float64(baseX + 2), Y: float64(baseY), Z: float64(baseZ)}
	s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air",
		int(math.Floor(chestPos.X))-3, int(math.Floor(chestPos.Y))-1, int(math.Floor(chestPos.Z))-3,
		int(math.Floor(chestPos.X+3)), int(math.Floor(chestPos.Y+3)), int(math.Floor(chestPos.Z+3))),
	)

	t.Log("placing chest with diamond")
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, chestPos, "minecraft:chest", "minecraft:chest", 10*time.Second)
	require.NoError(t, err, "place chest")
	slotID := rand.Intn(26) + 1 // Chest has 27 slots
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("data merge block %d %d %d {Items:[{Slot:0b,id:\"minecraft:diamond\",Count:%db}]}", baseX+2, baseY, baseZ, slotID))
	require.NoError(t, err)

	worldMgr := leader.Agent.GetWorld()
	blockMgr := leader.Config.BlockMgr
	containerFinder := items.NewContainerFinder(worldMgr, blockMgr)

	t.Log("searching for chest")
	searchPos := models.V3{X: playerPos.X, Y: playerPos.Y, Z: playerPos.Z}
	nearest, found := containerFinder.FindNearestContainer(searchPos, 5)
	require.True(t, found, "should find the chest")
	t.Logf("found chest at (%.0f, %.0f, %.0f)", nearest.Position.X, nearest.Position.Y, nearest.Position.Z)

	require.Equal(t, math.Floor(chestPos.X), nearest.Position.X)
	require.Equal(t, math.Floor(chestPos.Y), nearest.Position.Y)
	require.Equal(t, math.Floor(chestPos.Z), nearest.Position.Z)

	itemUsage := items.NewItemUsage(leader.BotClient().Conn(), leader.Config.PacketMgr, nil)
	if leader.Config.VersionHandler != nil {
		itemUsage.SetContainerHandler(leader.Config.VersionHandler.Play().Containers())
	}

	t.Log("opening found chest")
	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, nearest.Position, models.FaceNorth, 5*time.Second)
	require.NoError(t, err)
	t.Logf("chest opened with window ID: %d", windowID)

	t.Log("taking diamond from chest")
	foundSlotID, slotIDFound, err := leader.Agent.FindSlotWith(s.Ctx, "minecraft:diamond", int(windowID))
	require.True(t, slotIDFound, "diamond should be in chest")
	require.NoError(t, err, "failed to find diamond in chest")

	err = leader.Agent.TakeItemFromChest(windowID, int16(foundSlotID))
	require.NoError(t, err)

	_ = leader.Agent.CloseContainer()
	time.Sleep(500 * time.Millisecond)

	invItems, err := GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err)
	hasDiamond := false
	for _, item := range invItems {
		if item.ID == "minecraft:diamond" {
			hasDiamond = true
			break
		}
	}
	require.True(t, hasDiamond, "player should have diamond after taking from chest")
	t.Log("confirmed: diamond successfully taken from found chest")
}
