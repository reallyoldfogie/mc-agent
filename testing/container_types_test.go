package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ContainerTypesRandomSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the container-type slot-count checks below (furnace, hopper):
// one server per version, shared by both methods, instead of the previous per-test-function
// StartServer/StopServer pattern. WorldGen = WorldGenRandom + GameMode = survival (with
// FORCE_GAMEMODE via ExtraEnv), matching both pre-conversion functions' own inline
// DefaultServerConfig() override exactly. Distinct from container_standalone_test.go's
// TestFurnace, which checks only ContainerSlots on flat terrain - this suite additionally checks
// the total slot count (container + player) on random terrain.
type ContainerTypesRandomSuite struct {
	VersionWorldSuite
}

func TestContainerTypesRandomSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ContainerTypesRandomSuite{}
		s.WorldGen = WorldGenRandom
		s.GameMode = GameModeSurvival
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestFurnaceInteraction verifies opening a furnace container reports the
// correct GenericContainer type and slot counts. Equivalent to the
// pre-Phase-1 TestFurnaceInteraction.
func (s *ContainerTypesRandomSuite) TestFurnaceInteraction() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("FurnaceTypeBot", "furnace_interaction")
	require.NoError(t, err, "spawn agent")

	baseX := int(math.Floor(leader.Origin.X))
	baseY := int(math.Floor(leader.Origin.Y))
	baseZ := int(math.Floor(leader.Origin.Z))
	furnacePos := models.V3{
		X: float64(baseX + 2),
		Y: float64(baseY + 1),
		Z: float64(baseZ),
	}
	t.Logf("placing furnace at %+v", furnacePos)
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, furnacePos, "minecraft:furnace", "minecraft:furnace", 10*time.Second)
	require.NoError(t, err, "place furnace")

	teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, furnacePos.X-0.6, furnacePos.Y, furnacePos.Z)
	t.Logf("teleporting player: %s", teleportCmd)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport player")
	time.Sleep(500 * time.Millisecond)

	worldMgr := leader.Agent.GetWorld()
	blockStateID, _ := worldMgr.GetBlockAt(furnacePos.X, furnacePos.Y, furnacePos.Z)
	t.Logf("client world shows blockStateID at furnace position: %d", blockStateID)
	if blockStateID != 0 {
		blockMgr := leader.Config.BlockMgr
		if blockID, ok := blockMgr.BlockIDByStateID(blockStateID); ok {
			if block, ok := blockMgr.GetByID(blockID); ok {
				t.Logf("client sees block: %s", block.Name)
			}
		}
	} else {
		t.Log("WARNING: client sees air/unloaded chunk at furnace position - attempting to open anyway")
	}

	itemUsage := items.NewItemUsage(leader.BotClient().Conn(), leader.Config.PacketMgr, nil)
	if leader.Config.VersionHandler != nil {
		itemUsage.SetContainerHandler(leader.Config.VersionHandler.Play().Containers())
	}

	t.Log("opening furnace")
	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, furnacePos, models.FaceEast, 5*time.Second)
	require.NoError(t, err, "open furnace")
	t.Logf("furnace opened with window ID: %d", windowID)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "furnace window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	require.True(t, ok, "screen should be a GenericContainer for furnace")
	require.Equal(t, int32(14), genericContainer.Type, "container type should be 14 (furnace)")
	t.Logf("furnace container has %d total slots (%d container + %d player)",
		len(genericContainer.Slots), genericContainer.ContainerSlots, len(genericContainer.Slots)-genericContainer.ContainerSlots)

	require.Equal(t, 39, len(genericContainer.Slots), "furnace should have 39 total slots")
	require.Equal(t, 3, genericContainer.ContainerSlots, "furnace should have 3 container slots")

	t.Log("closing furnace")
	_ = leader.Agent.CloseContainer()
	t.Log("furnace closed successfully")
}

// TestHopperInteraction verifies opening a hopper container reports the
// correct GenericContainer type and slot counts. Equivalent to the
// pre-Phase-1 TestHopperInteraction.
func (s *ContainerTypesRandomSuite) TestHopperInteraction() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("HopperTypeBot", "hopper_interaction")
	require.NoError(t, err, "spawn agent")

	baseX := int(math.Floor(leader.Origin.X))
	baseY := int(math.Floor(leader.Origin.Y))
	baseZ := int(math.Floor(leader.Origin.Z))
	hopperPos := models.V3{
		X: float64(baseX + 2),
		Y: float64(baseY),
		Z: float64(baseZ),
	}

	teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, hopperPos.X-0.6, hopperPos.Y, hopperPos.Z)
	t.Logf("teleporting player near hopper location: %s", teleportCmd)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport player")
	time.Sleep(1 * time.Second)

	t.Logf("placing hopper at %+v", hopperPos)
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, hopperPos, "minecraft:hopper", "minecraft:hopper", 10*time.Second)
	require.NoError(t, err, "place hopper")

	itemUsage := items.NewItemUsage(leader.BotClient().Conn(), leader.Config.PacketMgr, nil)
	if leader.Config.VersionHandler != nil {
		itemUsage.SetContainerHandler(leader.Config.VersionHandler.Play().Containers())
	}

	t.Log("opening hopper")
	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, hopperPos, models.FaceEast, 5*time.Second)
	require.NoError(t, err, "open hopper")
	t.Logf("hopper opened with window ID: %d", windowID)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "hopper window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	require.True(t, ok, "screen should be a GenericContainer for hopper")
	require.Equal(t, int32(16), genericContainer.Type, "container type should be 16 (hopper)")
	t.Logf("hopper container has %d total slots (%d container + %d player)",
		len(genericContainer.Slots), genericContainer.ContainerSlots, len(genericContainer.Slots)-genericContainer.ContainerSlots)

	require.Equal(t, 41, len(genericContainer.Slots), "hopper should have 41 total slots")
	require.Equal(t, 5, genericContainer.ContainerSlots, "hopper should have 5 container slots")

	t.Log("closing hopper")
	_ = leader.Agent.CloseContainer()
	t.Log("hopper closed successfully")
}
