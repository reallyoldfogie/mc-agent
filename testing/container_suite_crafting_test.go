package testing

import (
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// TestCraftingTable tests crafting table functionality
func (s *ContainerTestSuite) TestCraftingTable() {
	s.T().Log("=== Testing Crafting Table ===")

	// Teleport to crafting table
	pos := s.teleportToContainer("crafting_table")

	// Open crafting table
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("crafting table opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 12)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "crafting table window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "crafting table should be a GenericContainer")
	s.Require().Equal(int32(12), genericContainer.Type, "should be type 12 (crafting)")

	// Crafting table has 10 container slots (9 crafting grid + 1 output) + 36 player = 46 total
	s.Require().Equal(46, len(genericContainer.Slots), "crafting table should have 46 total slots")
	s.Require().Equal(10, genericContainer.ContainerSlots, "crafting table should have 10 container slots")

	// Verify slots are empty
	for i := 0; i < 10; i++ {
		s.Require().Equal(int32(0), int32(genericContainer.Slots[i].Count),
			"crafting table slot %d should be empty", i)
	}

	// Close crafting table
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Crafting table test passed")
}

// TestCrafter tests crafter block functionality
func (s *ContainerTestSuite) TestCrafter() {
	s.T().Log("=== Testing Crafter ===")

	// Teleport to crafter
	pos := s.teleportToContainer("crafter")

	// Open crafter
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("crafter opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 7)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "crafter window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "crafter should be a GenericContainer")
	s.Require().Equal(int32(7), genericContainer.Type, "should be type 7 (crafter_3x3)")

	// Crafter has 9 container slots (3x3 grid) + 36 player = 45 total
	s.Require().Equal(45, len(genericContainer.Slots), "crafter should have 45 total slots")
	s.Require().Equal(9, genericContainer.ContainerSlots, "crafter should have 9 container slots")

	// Close crafter
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Crafter test passed")
}

// TestBrewingStand tests brewing stand functionality
func (s *ContainerTestSuite) TestBrewingStand() {
	s.T().Log("=== Testing Brewing Stand ===")

	// Teleport to brewing stand
	pos := s.teleportToContainer("brewing_stand")

	// Open brewing stand
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("brewing stand opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 11)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "brewing stand window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "brewing stand should be a GenericContainer")
	s.Require().Equal(int32(11), genericContainer.Type, "should be type 11 (brewing_stand)")

	// Brewing stand has 5 container slots (3 bottles + 1 ingredient + 1 fuel) + 36 player = 41 total
	s.Require().Equal(41, len(genericContainer.Slots), "brewing stand should have 41 total slots")
	s.Require().Equal(5, genericContainer.ContainerSlots, "brewing stand should have 5 container slots")

	// Close brewing stand
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Brewing stand test passed")
}
