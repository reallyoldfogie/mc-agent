package testing

import (
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// TestChest tests basic chest functionality
func (s *ContainerTestSuite) TestChest() {
	s.T().Log("=== Testing Chest ===")

	// Teleport to chest
	pos := s.teleportToContainer("chest")

	// Open chest
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("chest opened with window ID: %d", windowID)

	// Verify it's a chest
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "chest window should exist")

	chest, ok := screen.(*mcscreen.Chest)
	s.Require().True(ok, "screen should be a Chest")
	s.Require().Equal(3, chest.Rows, "should be single chest (3 rows)")

	// Close chest
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Chest test passed")
}

// TestBarrel tests barrel functionality
func (s *ContainerTestSuite) TestBarrel() {
	s.T().Log("=== Testing Barrel ===")

	// Teleport to barrel
	pos := s.teleportToContainer("barrel")

	// Open barrel
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("barrel opened with window ID: %d", windowID)

	// Verify it's a chest-type container (barrels use same type as chests)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "barrel window should exist")

	chest, ok := screen.(*mcscreen.Chest)
	s.Require().True(ok, "barrel should use Chest container type")

	// Barrels are 27 slots like a chest
	s.Require().Equal(63, len(chest.Slots), "barrel should have 63 total slots")

	// Close barrel
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Barrel test passed")
}

// TestChestWithItems tests placing and taking items from a chest
func (s *ContainerTestSuite) TestChestWithItems() {
	s.T().Log("=== Testing Chest With Items ===")

	// Get chest position
	pos := s.getContainer("chest")

	// Place items in chest via RCON using direct NBT (1.21.5 format)
	blockSpec := `minecraft:chest{Items:[{Slot:0b,id:"diamond",Count:5b},{Slot:13b,id:"iron_ingot",Count:10b}]}`
	_, err := PlaceBlockAndWait(s.ctx, s.inst.RCON, s.agent, pos, blockSpec, "minecraft:chest", 10*time.Second)
	s.Require().NoError(err, "place chest with items")

	// Verify items via RCON
	chestItems, err := GetChestContents(s.ctx, s.inst.RCON, pos)
	s.Require().NoError(err, "get chest contents")
	s.Require().Len(chestItems, 2, "chest should have 2 items")
	s.T().Logf("chest contents via RCON: %+v", chestItems)

	// Teleport to chest
	s.teleportToContainer("chest")

	// Open chest
	windowID := s.openContainer(pos, models.FaceEast)

	// Verify client can see the items
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "chest window should exist")

	chest, ok := screen.(*mcscreen.Chest)
	s.Require().True(ok, "screen should be a chest")

	// Check slot 0 (diamond)
	s.Require().Greater(int(chest.Slots[0].ID), 0, "slot 0 should have an item")
	s.Require().Greater(int(chest.Slots[0].Count), 0, "slot 0 should have count > 0")
	s.T().Logf("slot 0: ID=%d Count=%d", chest.Slots[0].ID, chest.Slots[0].Count)

	// Check slot 13 (iron_ingot)
	s.Require().Greater(int(chest.Slots[13].ID), 0, "slot 13 should have an item")
	s.Require().Greater(int(chest.Slots[13].Count), 0, "slot 13 should have count > 0")
	s.T().Logf("slot 13: ID=%d Count=%d", chest.Slots[13].ID, chest.Slots[13].Count)

	// Close chest
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Chest with items test passed")
}

// TestShulkerBox tests shulker box functionality
func (s *ContainerTestSuite) TestShulkerBox() {
	s.T().Log("=== Testing Shulker Box ===")

	// Teleport to shulker box
	pos := s.teleportToContainer("shulker_box")

	// Open shulker box
	windowID := s.openContainer(pos, models.FaceUp)
	s.T().Logf("shulker box opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 20)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "shulker box window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "shulker box should be a GenericContainer")
	s.Require().Equal(int32(20), genericContainer.Type, "should be type 20 (shulker_box)")

	// Shulker boxes have 27 container slots + 36 player = 63 total
	s.Require().Equal(63, len(genericContainer.Slots), "shulker box should have 63 total slots")
	s.Require().Equal(27, genericContainer.ContainerSlots, "shulker box should have 27 container slots")

	// Close shulker box
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Shulker box test passed")
}
