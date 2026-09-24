package testing

import (
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// TestFurnace tests furnace functionality
func (s *ContainerTestSuite) TestFurnace() {
	s.spawnContainerAgent("FurnaceBot", "container_furnace")
	s.T().Log("=== Testing Furnace ===")

	// Teleport to furnace
	pos := s.teleportToContainer("furnace")

	// Open furnace
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("furnace opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 14)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "furnace window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "furnace should be a GenericContainer")
	s.Require().Equal(int32(14), genericContainer.Type, "should be type 14 (furnace)")

	// Furnace has 3 container slots (input, fuel, output) + 36 player = 39 total
	s.Require().Equal(39, len(genericContainer.Slots), "furnace should have 39 total slots")
	s.Require().Equal(3, genericContainer.ContainerSlots, "furnace should have 3 container slots")

	// Verify slots are empty
	for i := 0; i < 3; i++ {
		s.Require().Equal(int32(0), int32(genericContainer.Slots[i].Count),
			"furnace slot %d should be empty", i)
	}

	// Close furnace
	_ = s.leader.Agent.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Furnace test passed")
}

// TestBlastFurnace tests blast furnace functionality
func (s *ContainerTestSuite) TestBlastFurnace() {
	s.spawnContainerAgent("BlastFurnaceBot", "container_blast_furnace")
	s.T().Log("=== Testing Blast Furnace ===")

	// Teleport to blast furnace
	pos := s.teleportToContainer("blast_furnace")

	// Open blast furnace
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("blast furnace opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 10)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "blast furnace window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "blast furnace should be a GenericContainer")
	s.Require().Equal(int32(10), genericContainer.Type, "should be type 10 (blast_furnace)")

	// Blast furnace has same layout as furnace: 3 + 36 = 39
	s.Require().Equal(39, len(genericContainer.Slots), "blast furnace should have 39 total slots")
	s.Require().Equal(3, genericContainer.ContainerSlots, "blast furnace should have 3 container slots")

	// Close blast furnace
	_ = s.leader.Agent.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Blast furnace test passed")
}

// TestSmoker tests smoker functionality
func (s *ContainerTestSuite) TestSmoker() {
	s.spawnContainerAgent("SmokerBot", "container_smoker")
	s.T().Log("=== Testing Smoker ===")

	// Teleport to smoker
	pos := s.teleportToContainer("smoker")

	// Open smoker
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("smoker opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 22)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "smoker window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "smoker should be a GenericContainer")
	s.Require().Equal(int32(22), genericContainer.Type, "should be type 22 (smoker)")

	// Smoker has same layout as furnace: 3 + 36 = 39
	s.Require().Equal(39, len(genericContainer.Slots), "smoker should have 39 total slots")
	s.Require().Equal(3, genericContainer.ContainerSlots, "smoker should have 3 container slots")

	// Close smoker
	_ = s.leader.Agent.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Smoker test passed")
}

// TestHopper tests hopper functionality
func (s *ContainerTestSuite) TestHopper() {
	s.spawnContainerAgent("HopperBot", "container_hopper")
	s.T().Log("=== Testing Hopper ===")

	// Teleport to hopper
	pos := s.teleportToContainer("hopper")

	// Open hopper
	windowID := s.openContainer(pos, models.FaceUp)
	s.T().Logf("hopper opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 16)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "hopper window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "hopper should be a GenericContainer")
	s.Require().Equal(int32(16), genericContainer.Type, "should be type 16 (hopper)")

	// Hopper has 5 container slots + 36 player = 41 total
	s.Require().Equal(41, len(genericContainer.Slots), "hopper should have 41 total slots")
	s.Require().Equal(5, genericContainer.ContainerSlots, "hopper should have 5 container slots")

	// Close hopper
	_ = s.leader.Agent.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Hopper test passed")
}

// TestDispenser tests dispenser functionality
func (s *ContainerTestSuite) TestDispenser() {
	s.spawnContainerAgent("DispenserBot", "container_dispenser")
	s.T().Log("=== Testing Dispenser ===")

	// Teleport to dispenser
	pos := s.teleportToContainer("dispenser")

	// Open dispenser
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("dispenser opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 6)
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "dispenser window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "dispenser should be a GenericContainer")
	s.Require().Equal(int32(6), genericContainer.Type, "should be type 6 (generic_3x3)")

	// Dispenser has 9 container slots (3x3) + 36 player = 45 total
	s.Require().Equal(45, len(genericContainer.Slots), "dispenser should have 45 total slots")
	s.Require().Equal(9, genericContainer.ContainerSlots, "dispenser should have 9 container slots")

	// Close dispenser
	_ = s.leader.Agent.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Dispenser test passed")
}

// TestDropper tests dropper functionality
func (s *ContainerTestSuite) TestDropper() {
	s.spawnContainerAgent("DropperBot", "container_dropper")
	s.T().Log("=== Testing Dropper ===")

	// Teleport to dropper
	pos := s.teleportToContainer("dropper")

	// Open dropper
	windowID := s.openContainer(pos, models.FaceEast)
	s.T().Logf("dropper opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (type 6) - same as dispenser
	screen, ok := s.screenMgr.Screens()[int(windowID)]
	s.Require().True(ok, "dropper window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	s.Require().True(ok, "dropper should be a GenericContainer")
	s.Require().Equal(int32(6), genericContainer.Type, "should be type 6 (generic_3x3)")

	// Dropper has same layout as dispenser: 9 + 36 = 45
	s.Require().Equal(45, len(genericContainer.Slots), "dropper should have 45 total slots")
	s.Require().Equal(9, genericContainer.ContainerSlots, "dropper should have 9 container slots")

	// Close dropper
	_ = s.leader.Agent.CloseContainer()
	time.Sleep(100 * time.Millisecond)

	s.T().Log("✓ Dropper test passed")
}
