package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	botpkg "github.com/reallyoldfogie/mc-bot-go/bot"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ContainerButtonFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the button-clicking container tests below (stonecutter, loom,
// enchanting table, beacon): one server per version, shared by every method, instead of the
// previous per-test-function setupStandaloneTest server-per-test pattern. Part of the
// container-bound category unlocked by the window-ID re-measurement - see
// docs/plans/integration-test-shared-server/31-window-id-limit-remeasurement.md and
// 32-phase1-container-standalone-conversion.md (the proof-of-concept for this category) and
// 33-phase1-container-button-conversion.md (this conversion's own live validation). Unlike that
// proof of concept, these methods also exercise ContainerButtonClick (via items.ButtonClicker),
// a materially different load than plain open/close.
type ContainerButtonFlatSuite struct {
	VersionWorldSuite
}

func TestContainerButtonFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ContainerButtonFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

func newButtonClicker(leader *WorkingAreaAgent) *items.ButtonClicker {
	buttonClicker := items.NewButtonClicker(leader.BotClient().Conn(), leader.Config.PacketMgr)
	if leader.Config.VersionHandler != nil {
		buttonClicker.SetContainerHandler(leader.Config.VersionHandler.Play().Containers())
	}
	return buttonClicker
}

// TestStonecutter verifies stonecutter recipe-selection button clicks.
// Equivalent to the pre-Phase-1 TestStonecutter_Standalone.
func (s *ContainerButtonFlatSuite) TestStonecutter() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("StonecutterBot", "stonecutter_button")
	require.NoError(t, err, "spawn agent")

	containerPos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, containerPos, "minecraft:stonecutter", "stonecutter", 30*time.Second)
	require.NoError(t, err, "place stonecutter")

	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, containerPos.X-2, containerPos.Y, containerPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with stone 64", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give stone block")
	time.Sleep(500 * time.Millisecond)

	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, containerPos, models.FaceNorth, 5*time.Second)
	require.NoError(t, err, "open stonecutter")
	t.Logf("stonecutter opened with window ID: %d", windowID)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "stonecutter window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	require.True(t, ok, "stonecutter should be a GenericContainer")
	require.Equal(t, int32(24), genericContainer.Type, "should be type 24 (stonecutter)")
	require.Equal(t, 2, genericContainer.ContainerSlots, "stonecutter should have 2 container slots (input + output)")

	err = newButtonClicker(leader).ClickButton(byte(windowID), 0)
	require.NoError(t, err, "click stonecutter button")
	time.Sleep(500 * time.Millisecond)
	t.Log("✓ Button click sent successfully")

	err = leader.Agent.CloseContainer()
	require.NoError(t, err, "close stonecutter")

	t.Log("✓ Stonecutter test passed")
}

// TestLoomSurvivalDebug verifies loom pattern-selection button clicks with
// packet debug logging enabled, matching the pre-Phase-1 TestLoom_Survival's
// distinct purpose (debugging aid, single version). Deliberately kept
// separate from TestLoomAllFaces below rather than merged, preserving the
// original's own split.
func (s *ContainerButtonFlatSuite) TestLoomSurvivalDebug() {
	t := s.T()

	prevPacketDebug := botpkg.PacketDebugEnabled
	botpkg.PacketDebugEnabled = true
	t.Cleanup(func() { botpkg.PacketDebugEnabled = prevPacketDebug })

	leader, err := s.SpawnWorkingAreaAgent("LoomDebugBot", "loom_survival_debug")
	require.NoError(t, err, "spawn agent")

	containerPos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, containerPos, "minecraft:loom", "loom", 30*time.Second)
	require.NoError(t, err, "place loom")

	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, containerPos.X-2, containerPos.Y, containerPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with white_banner 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give banner")

	giveCmd = fmt.Sprintf("item replace entity %s hotbar.1 with black_dye 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give dye")
	time.Sleep(500 * time.Millisecond)

	t.Logf("Attempting to open loom in SURVIVAL mode at pos: %+v ", containerPos)

	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, containerPos, models.FaceNorth, 5*time.Second)
	require.NoError(t, err, "open loom")
	t.Logf("loom opened with window ID: %d", windowID)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "loom window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	require.True(t, ok, "loom should be a GenericContainer")

	expectedLoomType := mcscreen.GetContainerTypeIDByIdentifier("loom")
	require.NotEqual(t, int32(-1), expectedLoomType, "loom type should be registered")
	require.Equal(t, expectedLoomType, genericContainer.Type, "should be loom type")
	require.Equal(t, 4, genericContainer.ContainerSlots, "loom should have 4 container slots (banner + dye + pattern + output)")

	err = newButtonClicker(leader).ClickButton(byte(windowID), 1)
	require.NoError(t, err, "click loom button")
	time.Sleep(500 * time.Millisecond)
	t.Log("✓ Button click sent successfully")

	err = leader.Agent.CloseContainer()
	require.NoError(t, err, "close loom")

	t.Log("✓ Loom survival mode test passed")
}

// TestLoomAllFaces verifies loom pattern-selection button clicks, trying
// multiple block faces defensively (the original's own approach - see the
// FaceUp/FaceEast/FaceNorth fallback chain below). Equivalent to the
// pre-Phase-1 TestLoom_Standalone.
func (s *ContainerButtonFlatSuite) TestLoomAllFaces() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("LoomFacesBot", "loom_all_faces")
	require.NoError(t, err, "spawn agent")

	containerPos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, containerPos, "minecraft:loom", "loom", 30*time.Second)
	require.NoError(t, err, "place loom")

	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, containerPos.X-2, containerPos.Y, containerPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with white_banner 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give banner")

	giveCmd = fmt.Sprintf("item replace entity %s hotbar.1 with black_dye 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give dye")
	time.Sleep(500 * time.Millisecond)

	t.Log("Trying different block faces for loom...")
	t.Log("Trying FaceUp...")
	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, containerPos, models.FaceUp, 2*time.Second)
	if err != nil {
		t.Logf("FaceUp failed: %v", err)
		t.Log("Trying FaceEast...")
		windowID, err = OpenContainerWithLOS(s.Ctx, leader.Agent, containerPos, models.FaceEast, 2*time.Second)
		if err != nil {
			t.Logf("FaceEast failed: %v", err)
			t.Log("Trying FaceNorth...")
			windowID, err = OpenContainerWithLOS(s.Ctx, leader.Agent, containerPos, models.FaceNorth, 2*time.Second)
			require.NoError(t, err, "open loom - all faces failed")
		}
	}
	t.Logf("Loom opened successfully with window ID: %d", windowID)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "loom window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	require.True(t, ok, "loom should be a GenericContainer")

	expectedLoomType := mcscreen.GetContainerTypeIDByIdentifier("loom")
	require.NotEqual(t, int32(-1), expectedLoomType, "loom type should be registered")
	require.Equal(t, expectedLoomType, genericContainer.Type, "should be loom type")
	require.Equal(t, 4, genericContainer.ContainerSlots, "loom should have 4 container slots (banner + dye + pattern + output)")

	err = newButtonClicker(leader).ClickButton(byte(windowID), 1)
	require.NoError(t, err, "click loom button")
	time.Sleep(500 * time.Millisecond)
	t.Log("✓ Button click sent successfully")

	err = leader.Agent.CloseContainer()
	require.NoError(t, err, "close loom")

	t.Log("✓ Loom all-faces test passed")
}

// TestEnchantingTable verifies enchanting-table enchantment-selection
// button clicks. Equivalent to the pre-Phase-1 TestEnchantingTable_Standalone.
func (s *ContainerButtonFlatSuite) TestEnchantingTable() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("EnchantBot", "enchanting_table_button")
	require.NoError(t, err, "spawn agent")

	containerPos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, containerPos, "minecraft:enchanting_table", "enchanting_table", 30*time.Second)
	require.NoError(t, err, "place enchanting table")

	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, containerPos.X-2, containerPos.Y, containerPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with diamond_sword 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give diamond sword")

	giveCmd = fmt.Sprintf("item replace entity %s hotbar.1 with lapis_lazuli 64", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give lapis")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("xp set %s 30 levels", leader.Name))
	require.NoError(t, err, "set xp levels")
	time.Sleep(500 * time.Millisecond)

	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, containerPos, models.FaceNorth, 5*time.Second)
	require.NoError(t, err, "open enchanting table")
	t.Logf("enchanting table opened with window ID: %d", windowID)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "enchanting table window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	require.True(t, ok, "enchanting table should be a GenericContainer")
	require.Equal(t, int32(13), genericContainer.Type, "should be type 13 (enchanting_table)")
	require.Equal(t, 2, genericContainer.ContainerSlots, "enchanting table should have 2 container slots (item + lapis)")

	err = newButtonClicker(leader).ClickButton(byte(windowID), 0)
	require.NoError(t, err, "click enchanting table button")
	time.Sleep(500 * time.Millisecond)
	t.Log("✓ Button click sent successfully")

	err = leader.Agent.CloseContainer()
	require.NoError(t, err, "close enchanting table")

	t.Log("✓ Enchanting table test passed")
}

// TestBeacon verifies beacon effect-selection button clicks. Equivalent to
// the pre-Phase-1 TestBeacon_Standalone.
func (s *ContainerButtonFlatSuite) TestBeacon() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("BeaconBot", "beacon_button")
	require.NoError(t, err, "spawn agent")

	containerPos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, containerPos, "minecraft:beacon", "beacon", 30*time.Second)
	require.NoError(t, err, "place beacon")

	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, containerPos.X-2, containerPos.Y, containerPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	beaconX := int(math.Floor(containerPos.X))
	beaconY := int(math.Floor(containerPos.Y))
	beaconZ := int(math.Floor(containerPos.Z))
	t.Logf("Beacon block coordinates: X=%d Y=%d Z=%d", beaconX, beaconY, beaconZ)

	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			fillCmd := fmt.Sprintf("setblock %d %d %d iron_block", beaconX+dx, beaconY-1, beaconZ+dz)
			_, err = s.Inst.RCON.Exec(s.Ctx, fillCmd)
			require.NoError(t, err, "place iron block base")
		}
	}

	t.Log("Clearing sky access for beacon...")
	clearCmd := fmt.Sprintf("fill %d %d %d %d 320 %d air", beaconX, beaconY+1, beaconZ, beaconX, beaconZ)
	_, err = s.Inst.RCON.Exec(s.Ctx, clearCmd)
	require.NoError(t, err, "clear sky access")

	t.Log("Waiting 5 seconds for beacon to activate...")
	time.Sleep(5 * time.Second)

	beaconDataCmd := fmt.Sprintf("data get block %d %d %d", beaconX, beaconY, beaconZ)
	beaconData, err := s.Inst.RCON.Exec(s.Ctx, beaconDataCmd)
	require.NoError(t, err, "get beacon data")
	t.Logf("Beacon block entity data: %s", beaconData)

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with iron_ingot 64", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give iron ingots")
	time.Sleep(500 * time.Millisecond)

	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, containerPos, models.FaceNorth, 5*time.Second)
	require.NoError(t, err, "open beacon")
	t.Logf("beacon opened with window ID: %d", windowID)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "beacon window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	require.True(t, ok, "beacon should be a GenericContainer")
	require.Equal(t, int32(9), genericContainer.Type, "should be type 9 (beacon)")
	require.Equal(t, 1, genericContainer.ContainerSlots, "beacon should have 1 container slot (payment)")

	err = newButtonClicker(leader).ClickButton(byte(windowID), 0)
	require.NoError(t, err, "click beacon button")
	time.Sleep(500 * time.Millisecond)
	t.Log("✓ Button click sent successfully")

	err = leader.Agent.CloseContainer()
	require.NoError(t, err, "close beacon")

	t.Log("✓ Beacon test passed")
}
