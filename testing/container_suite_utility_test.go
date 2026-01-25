package testing

import (
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// TestAnvil tests anvil functionality
func (s *ContainerTestSuite) TestAnvil() {
	s.T().Log("=== Testing Anvil ===")

	// Teleport to anvil
	pos := s.teleportToContainer("anvil")

	// Open anvil
	windowID := s.openContainer(pos, items.FaceUp)
	s.T().Logf("anvil opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 8)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "anvil window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "anvil should be a GenericContainer")
	s.Require().Equal(int32(8), genericContainer.Type, "should be type 8 (anvil)")

	// Anvil has 3 container slots (2 input + 1 output) + 36 player = 39 total
	s.Require().Equal(39, len(genericContainer.Slots), "anvil should have 39 total slots")
	s.Require().Equal(3, genericContainer.ContainerSlots, "anvil should have 3 container slots")

	// Verify slots are empty
	for i := 0; i < 3; i++ {
		s.Require().Equal(int32(0), int32(genericContainer.Slots[i].Count),
			"anvil slot %d should be empty", i)
	}

	// Close anvil
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Anvil test passed")
}

// TestGrindstone tests grindstone functionality
func (s *ContainerTestSuite) TestGrindstone() {
	s.T().Log("=== Testing Grindstone ===")

	// Teleport to grindstone
	pos := s.teleportToContainer("grindstone")

	// Open grindstone
	windowID := s.openContainer(pos, items.FaceEast)
	s.T().Logf("grindstone opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 15)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "grindstone window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "grindstone should be a GenericContainer")
	s.Require().Equal(int32(15), genericContainer.Type, "should be type 15 (grindstone)")

	// Grindstone has 3 container slots (2 input + 1 output) + 36 player = 39 total
	s.Require().Equal(39, len(genericContainer.Slots), "grindstone should have 39 total slots")
	s.Require().Equal(3, genericContainer.ContainerSlots, "grindstone should have 3 container slots")

	// Close grindstone
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Grindstone test passed")
}

// TestSmithingTable tests smithing table functionality
func (s *ContainerTestSuite) TestSmithingTable() {
	s.T().Log("=== Testing Smithing Table ===")

	// Teleport to smithing table
	pos := s.teleportToContainer("smithing_table")

	// Open smithing table
	windowID := s.openContainer(pos, items.FaceEast)
	s.T().Logf("smithing table opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 21)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "smithing table window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "smithing table should be a GenericContainer")
	s.Require().Equal(int32(21), genericContainer.Type, "should be type 21 (smithing)")

	// Smithing table has 4 container slots (template + base + addition + output) + 36 player = 40 total
	s.Require().Equal(40, len(genericContainer.Slots), "smithing table should have 40 total slots")
	s.Require().Equal(4, genericContainer.ContainerSlots, "smithing table should have 4 container slots")

	// Close smithing table
	_ = s.containerHelper.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Smithing table test passed")
}
