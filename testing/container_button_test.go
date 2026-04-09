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
)

// Helper functions are defined in container_standalone_test.go but we need them here too
// Note: Ideally these should be in a shared test utilities file

// TestStonecutter_Standalone tests stonecutter button interaction
func TestStonecutter_Standalone(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "stonecutter", tt.MCVersion)
			defer env.Cancel()

			// Teleport near stonecutter
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Verify stonecutter block is actually placed
			// IMPORTANT: Use math.Floor for negative coordinates (int() truncates towards zero)
			blockX := int(math.Floor(env.ContainerPos.X))
			blockY := int(math.Floor(env.ContainerPos.Y))
			blockZ := int(math.Floor(env.ContainerPos.Z))
			blockCheckCmd := fmt.Sprintf("data get block %d %d %d", blockX, blockY, blockZ)
			blockData, err := env.Inst.RCON.Exec(env.Ctx, blockCheckCmd)
			require.NoError(t, err, "check block data")
			t.Logf("Block at (%d,%d,%d): %s", blockX, blockY, blockZ, blockData)

			// Give the bot a stone block in hotbar slot 0
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with stone 64", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give stone block")
			time.Sleep(500 * time.Millisecond)

			t.Logf("Attempting to open stonecutter at exact pos: %+v", env.ContainerPos)

			// Open stonecutter using agent (handles rotation and continuous position packets automatically)
			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceNorth, 5*time.Second)
			require.NoError(t, err, "open stonecutter")
			t.Logf("stonecutter opened with window ID: %d", windowID)

			// Verify it's a GenericContainer (type 24)
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "stonecutter window should exist")

			genericContainer, ok := screen.(*mcscreen.GenericContainer)
			require.True(t, ok, "stonecutter should be a GenericContainer")
			require.Equal(t, int32(24), genericContainer.Type, "should be type 24 (stonecutter)")
			require.Equal(t, 2, genericContainer.ContainerSlots, "stonecutter should have 2 container slots (input + output)")

			// Create button clicker
			buttonClicker := items.NewButtonClicker(env.Agent.BotClient().Conn(), env.Agent.Config.PacketMgr)
			if env.Agent.Config.VersionHandler != nil {
				buttonClicker.SetContainerHandler(env.Agent.Config.VersionHandler.Play().Containers())
			}

			// Click button to select a recipe (e.g., stone -> stone stairs)
			// Button ID corresponds to recipe index in the stonecutter's recipe list
			err = buttonClicker.ClickButton(byte(windowID), 0) // Click first recipe
			require.NoError(t, err, "click stonecutter button")
			time.Sleep(500 * time.Millisecond)

			t.Log("✓ Button click sent successfully")

			// Close stonecutter using agent
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close stonecutter")

			t.Log("✓ Stonecutter standalone test passed")
		})
	}
}

// TestLoom_Survival tests loom pattern selection in survival mode
func TestLoom_Survival(t *testing.T) {
	prevPacketDebug := botpkg.PacketDebugEnabled
	botpkg.PacketDebugEnabled = true
	t.Cleanup(func() {
		botpkg.PacketDebugEnabled = prevPacketDebug
	})

	env := setupStandaloneTestWithMode(t, "loom", "survival", "1.21.5")
	defer env.Cancel()

	// Teleport near loom
	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
	_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	// Give the bot a banner and dye
	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with white_banner 1", env.BotName)
	_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
	require.NoError(t, err, "give banner")

	giveCmd = fmt.Sprintf("item replace entity %s hotbar.1 with black_dye 1", env.BotName)
	_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
	require.NoError(t, err, "give dye")
	time.Sleep(500 * time.Millisecond)

	t.Logf("Attempting to open loom in SURVIVAL mode at pos: %+v ", env.ContainerPos)

	// Open loom using agent (handles rotation and continuous position packets automatically)
	windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceNorth, 5*time.Second)
	require.NoError(t, err, "open loom")
	t.Logf("loom opened with window ID: %d", windowID)

	// Verify it's a GenericContainer (loom type)
	screen, ok := env.ScreenMgr.Screens()[int(windowID)]
	require.True(t, ok, "loom window should exist")

	genericContainer, ok := screen.(*mcscreen.GenericContainer)
	require.True(t, ok, "loom should be a GenericContainer")

	// Get the expected loom type ID from the registry (version-agnostic)
	expectedLoomType := mcscreen.GetContainerTypeIDByIdentifier("loom")
	require.NotEqual(t, int32(-1), expectedLoomType, "loom type should be registered")
	require.Equal(t, expectedLoomType, genericContainer.Type, "should be loom type")
	require.Equal(t, 4, genericContainer.ContainerSlots, "loom should have 4 container slots (banner + dye + pattern + output)")

	// Create button clicker
	buttonClicker := items.NewButtonClicker(env.Agent.BotClient().Conn(), env.Agent.Config.PacketMgr)
	if env.Agent.Config.VersionHandler != nil {
		buttonClicker.SetContainerHandler(env.Agent.Config.VersionHandler.Play().Containers())
	}

	// Click button to select a pattern
	err = buttonClicker.ClickButton(byte(windowID), 1) // Select first pattern
	require.NoError(t, err, "click loom button")
	time.Sleep(500 * time.Millisecond)

	t.Log("✓ Button click sent successfully")

	// Close loom using agent
	err = env.Agent.Agent.CloseContainer()
	require.NoError(t, err, "close loom")

	t.Log("✓ Loom survival mode test passed")
}

// TestLoom_Standalone tests loom pattern selection (creative mode)
func TestLoom_Standalone(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "loom", tt.MCVersion)
			defer env.Cancel()

			// Teleport near loom
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Give the bot a banner and dye
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with white_banner 1", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give banner")

			giveCmd = fmt.Sprintf("item replace entity %s hotbar.1 with black_dye 1", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give dye")
			time.Sleep(500 * time.Millisecond)

			// Try all different faces to see if any work
			t.Log("Trying different block faces for loom...")

			// Try FaceUp (top of block) - agent handles rotation and continuous position packets
			t.Log("Trying FaceUp...")
			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceUp, 2*time.Second)
			if err != nil {
				t.Logf("FaceUp failed: %v", err)

				// Try FaceEast
				t.Log("Trying FaceEast...")
				windowID, err = OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceEast, 2*time.Second)
				if err != nil {
					t.Logf("FaceEast failed: %v", err)

					// Try FaceNorth
					t.Log("Trying FaceNorth...")
					windowID, err = OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceNorth, 2*time.Second)
					require.NoError(t, err, "open loom - all faces failed")
				}
			}
			t.Logf("Loom opened successfully with window ID: %d", windowID)

			// Verify it's a GenericContainer (loom type)
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "loom window should exist")

			genericContainer, ok := screen.(*mcscreen.GenericContainer)
			require.True(t, ok, "loom should be a GenericContainer")

			// Get the expected loom type ID from the registry (version-agnostic)
			expectedLoomType := mcscreen.GetContainerTypeIDByIdentifier("loom")
			require.NotEqual(t, int32(-1), expectedLoomType, "loom type should be registered")
			require.Equal(t, expectedLoomType, genericContainer.Type, "should be loom type")
			require.Equal(t, 4, genericContainer.ContainerSlots, "loom should have 4 container slots (banner + dye + pattern + output)")

			// Create button clicker
			buttonClicker := items.NewButtonClicker(env.Agent.BotClient().Conn(), env.Agent.Config.PacketMgr)
			if env.Agent.Config.VersionHandler != nil {
				buttonClicker.SetContainerHandler(env.Agent.Config.VersionHandler.Play().Containers())
			}

			// Click button to select a pattern
			// Button ID corresponds to pattern index (0 = no pattern, 1+ = pattern types)
			err = buttonClicker.ClickButton(byte(windowID), 1) // Select first pattern
			require.NoError(t, err, "click loom button")
			time.Sleep(500 * time.Millisecond)

			t.Log("✓ Button click sent successfully")

			// Close loom using agent
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close loom")

			t.Log("✓ Loom standalone test passed")
		})
	}
}

// TestEnchantingTable_Standalone tests enchanting table enchantment selection
func TestEnchantingTable_Standalone(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "enchanting_table", tt.MCVersion)
			defer env.Cancel()

			// Teleport near enchanting table
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Give the bot an enchantable item and lapis
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with diamond_sword 1", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give diamond sword")

			giveCmd = fmt.Sprintf("item replace entity %s hotbar.1 with lapis_lazuli 64", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give lapis")

			// Set player to max level for enchanting
			_, err = env.Inst.RCON.Exec(env.Ctx, fmt.Sprintf("xp set %s 30 levels", env.BotName))
			require.NoError(t, err, "set xp levels")
			time.Sleep(500 * time.Millisecond)

			// Open enchanting table using agent (handles rotation and continuous position packets automatically)
			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceNorth, 5*time.Second)
			require.NoError(t, err, "open enchanting table")
			t.Logf("enchanting table opened with window ID: %d", windowID)

			// Verify it's a GenericContainer (type 13)
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "enchanting table window should exist")

			genericContainer, ok := screen.(*mcscreen.GenericContainer)
			require.True(t, ok, "enchanting table should be a GenericContainer")
			require.Equal(t, int32(13), genericContainer.Type, "should be type 13 (enchanting_table)")
			require.Equal(t, 2, genericContainer.ContainerSlots, "enchanting table should have 2 container slots (item + lapis)")

			// Create button clicker
			buttonClicker := items.NewButtonClicker(env.Agent.BotClient().Conn(), env.Agent.Config.PacketMgr)
			if env.Agent.Config.VersionHandler != nil {
				buttonClicker.SetContainerHandler(env.Agent.Config.VersionHandler.Play().Containers())
			}

			// Click button to select an enchantment
			// Button ID 0-2 corresponds to the three enchantment options
			err = buttonClicker.ClickButton(byte(windowID), 0) // Select first enchantment
			require.NoError(t, err, "click enchanting table button")
			time.Sleep(500 * time.Millisecond)

			t.Log("✓ Button click sent successfully")

			// Close enchanting table using agent
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close enchanting table")

			t.Log("✓ Enchanting table standalone test passed")
		})
	}
}

// TestBeacon_Standalone tests beacon effect selection
func TestBeacon_Standalone(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "beacon", tt.MCVersion)
			defer env.Cancel()

			// Teleport near beacon
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Beacons need a pyramid base to function - create a simple 3x3 iron block base
			// IMPORTANT: Use math.Floor for negative coordinates (int() truncates towards zero)
			beaconX := int(math.Floor(env.ContainerPos.X))
			beaconY := int(math.Floor(env.ContainerPos.Y))
			beaconZ := int(math.Floor(env.ContainerPos.Z))
			t.Logf("Beacon block coordinates: X=%d Y=%d Z=%d", beaconX, beaconY, beaconZ)

			// Build 3x3 iron block pyramid base (1 layer for simplicity)
			for dx := -1; dx <= 1; dx++ {
				for dz := -1; dz <= 1; dz++ {
					fillCmd := fmt.Sprintf("setblock %d %d %d iron_block", beaconX+dx, beaconY-1, beaconZ+dz)
					_, err = env.Inst.RCON.Exec(env.Ctx, fillCmd)
					require.NoError(t, err, "place iron block base")
				}
			}

			// Ensure clear sky access - clear blocks above beacon to world height
			t.Log("Clearing sky access for beacon...")
			clearCmd := fmt.Sprintf("fill %d %d %d %d 320 %d air", beaconX, beaconY+1, beaconZ, beaconX, beaconZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, clearCmd)
			require.NoError(t, err, "clear sky access")

			// Wait longer for beacon to activate
			t.Log("Waiting 5 seconds for beacon to activate...")
			time.Sleep(5 * time.Second)

			// Check if beacon is activated by examining its block entity data
			beaconDataCmd := fmt.Sprintf("data get block %d %d %d", beaconX, beaconY, beaconZ)
			beaconData, err := env.Inst.RCON.Exec(env.Ctx, beaconDataCmd)
			require.NoError(t, err, "get beacon data")
			t.Logf("Beacon block entity data: %s", beaconData)

			// Give the bot payment items (iron/gold/diamond/emerald/netherite)
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with iron_ingot 64", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give iron ingots")
			time.Sleep(500 * time.Millisecond)

			// Open beacon using agent (handles rotation and continuous position packets automatically)
			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceNorth, 5*time.Second)
			require.NoError(t, err, "open beacon")
			t.Logf("beacon opened with window ID: %d", windowID)

			// Verify it's a GenericContainer (type 9)
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "beacon window should exist")

			genericContainer, ok := screen.(*mcscreen.GenericContainer)
			require.True(t, ok, "beacon should be a GenericContainer")
			require.Equal(t, int32(9), genericContainer.Type, "should be type 9 (beacon)")
			require.Equal(t, 1, genericContainer.ContainerSlots, "beacon should have 1 container slot (payment)")

			// Create button clicker
			buttonClicker := items.NewButtonClicker(env.Agent.BotClient().Conn(), env.Agent.Config.PacketMgr)
			if env.Agent.Config.VersionHandler != nil {
				buttonClicker.SetContainerHandler(env.Agent.Config.VersionHandler.Play().Containers())
			}

			// Click button to confirm effect selection
			// Button ID 0 confirms the selected primary/secondary effects
			err = buttonClicker.ClickButton(byte(windowID), 0) // Confirm effect
			require.NoError(t, err, "click beacon button")
			time.Sleep(500 * time.Millisecond)

			t.Log("✓ Button click sent successfully")

			// Close beacon using agent
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close beacon")

			t.Log("✓ Beacon standalone test passed")
		})
	}
}
