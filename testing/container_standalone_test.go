package testing

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/require"
)

// Standalone tests run each container test with its own server instance.
// This avoids the Minecraft server's window ID limit of ~6-7 containers per connection.
//
// Usage:
//   go test -run TestChest_Standalone     # Run single test
//   go test -run Test.*_Standalone        # Run all standalone tests
//   go test -run TestContainerSuite       # Run suite (shared server, faster but limited)

// StandaloneTestEnv holds all resources for a standalone test
type StandaloneTestEnv struct {
	Inst            *TestInstance
	Agent           *ManagedAgent
	ContainerHelper *items.ContainerHelper
	ScreenMgr       mcscreen.Manager
	Ctx             context.Context
	Cancel          context.CancelFunc
	ContainerPos    models.V3
	BotName         string
	Version         string // Minecraft version (e.g., "1.21.5")
}

// setupStandaloneTest creates a fresh server and agent for a single test (survival mode)
func setupStandaloneTest(t *testing.T, testName string, mcVersion string) *StandaloneTestEnv {
	return setupStandaloneTestWithMode(t, testName, "survival", mcVersion)
}

// setupStandaloneTestForEntity creates a fresh server and agent for entity container tests
// Unlike setupStandaloneTest, this does NOT place a block - entities are spawned by the test
func setupStandaloneTestForEntity(t *testing.T, testName string, mcVersion string) *StandaloneTestEnv {
	return setupStandaloneTestWithModeAndBlockPlacement(t, testName, "survival", false, mcVersion, DifficultyEasy, false)
}

func setupStandaloneTestForEntityWithReplay(t *testing.T, testName string, mcVersion string) *StandaloneTestEnv {
	return setupStandaloneTestWithModeAndBlockPlacement(t, testName, "survival", false, mcVersion, DifficultyEasy, true)
}

// setupStandaloneTestWithMode creates a fresh server and agent with specified game mode
func setupStandaloneTestWithMode(t *testing.T, testName string, gameMode GameMode, mcVersion string) *StandaloneTestEnv {
	return setupStandaloneTestWithModeAndBlockPlacement(t, testName, gameMode, true, mcVersion, DifficultyEasy, false)
}

// setupStandaloneTestWithModeAndBlockPlacement creates a fresh server and agent with specified game mode and optional block placement
func setupStandaloneTestWithModeAndBlockPlacement(t *testing.T, testName string, gameMode GameMode, placeBlock bool, mcVersion string, difficulty Difficulty, enableReplay bool) *StandaloneTestEnv {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	// Get working directory
	cwd, err := os.Getwd()
	require.NoError(t, err, "get current working directory")

	// Create framework
	framework, err := NewFramework()
	require.NoError(t, err, "create framework")
	t.Logf("framework initialized (gameMode: %s)", gameMode)

	// Configure server
	serverCfg := DefaultServerConfig()
	serverCfg.Memory = "512M"
	serverCfg.MinFreeMemoryMB = 256
	serverCfg.Version = mcVersion
	serverCfg.GameMode = gameMode
	serverCfg.Difficulty = difficulty
	serverCfg.PullImage = false
	serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", testName, mcVersion)
	RequireIntegrationEnv(t, serverCfg)

	// Optionally copy protocol dumper mod for packet debugging
	// Set ENABLE_PROTOCOL_DUMPER=1 to enable
	if os.Getenv("ENABLE_PROTOCOL_DUMPER") == "1" {
		modDir := filepath.Join(serverCfg.CacheDir, "mods")
		os.MkdirAll(modDir, 0755)
		err = copyFile(t, filepath.Join(cwd, "..", "..", "mc-protocol-dumper", "build", "libs", "protocol-dumper-0.0.3-fabric.jar"), filepath.Join(modDir, "protocol-dumper-0.0.3-fabric.jar"))
		if err != nil {
			t.Logf("Warning: failed to copy protocol dumper mod: %v", err)
		} else {
			t.Log("Protocol dumper mod enabled")
		}
	}

	// Start server
	inst, err := framework.StartServer(ctx, serverCfg)
	require.NoError(t, err, "start server")
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = framework.StopServer(stopCtx, inst, true)
	})
	t.Logf("server started: %s:%d", inst.Server.Host, inst.Server.HostServerPort)

	// Setup agent logging
	require.NoError(t, framework.setupAgentLogging(), "setup agent logging")
	t.Cleanup(func() { framework.CloseAgentLog() })

	// Spawn agent
	addr := fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort)
	botName := "StandaloneBot" // Must be <= 16 chars
	agentCfg := AgentConfig{
		Name:           botName,
		ServerAddress:  addr,
		Version:        serverCfg.Version,
		EnableCamAgent: true, // Enable camera agent to observe main agent
	}
	if enableReplay {
		agentCfg.EnableReplay = true
		agentCfg.ReplayOutput = normalizeReplayOutput(serverCfg.Version, fmt.Sprintf("%s_%s_%s.mcpr", testName, serverCfg.Version, time.Now().Format("20060102_150405")), agentCfg.Name)
		require.NoError(t, os.MkdirAll(filepath.Dir(agentCfg.ReplayOutput), 0755), "create replay directory")
	}

	// Version handler is auto-detected by the framework

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")
	t.Cleanup(func() {
		if agent != nil {
			// CRITICAL: Stop agent first to finalize replay recordings
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := agent.Stop(stopCtx); err != nil {
				t.Logf("Warning: agent stop failed: %v", err)
			}
			stopCancel()

			// Then close bot client
			if agent.BotClient() != nil {
				_ = agent.BotClient().Close()
			}
		}
	})

	// Load container type registry from Minecraft data
	// Must be done after agent is created, as the agent is what downloads the data
	registryPath := filepath.Join(cwd, ".agent", "cache", "downloads", serverCfg.Version)
	if err := mcscreen.LoadContainerTypesFromRegistry(registryPath); err != nil {
		t.Logf("Warning: failed to load container registry from %s: %v (using hardcoded values)", registryPath, err)
	}

	// Get components
	screenMgr := agent.ScreenManager()

	botClient := agent.BotClient()
	require.NotNil(t, botClient, "bot client should be available")

	// Wait for player to be online
	require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second),
		"agent never appeared in server player list")

	// Get spawn point
	playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
	require.NoError(t, err, "get player position")
	spawnPoint := models.V3{
		X: playerPos.X,
		Y: playerPos.Y,
		Z: playerPos.Z,
	}
	t.Logf("spawn point: %+v", spawnPoint)

	// Create container helpers
	itemUsage := items.NewItemUsage(botClient.Conn(), agent.Config.PacketMgr)
	// Set version-specific container handler
	if agent.Config.VersionHandler != nil {
		itemUsage.SetContainerHandler(agent.Config.VersionHandler.Play().Containers())
	}
	invMgr := items.NewInventoryManager(screenMgr)
	invMgr.SetWaitForUpdates(false)
	containerHelper := items.NewContainerHelper(itemUsage, invMgr, screenMgr, botClient, agent.Config.PacketMgr)

	// Wire container helper to agent (wraps items.ContainerHelper with adapter)
	agent.Agent.SetContainerHelper(containerHelper)

	// Place the container for this test (only for block-based containers)
	var containerPos models.V3
	if placeBlock {
		containerName := testName // e.g., "chest", "furnace", etc.
		containerBlockType := getContainerBlockType(containerName)
		containerX := int(math.Floor(spawnPoint.X)) + 5
		containerY := int(math.Floor(spawnPoint.Y))
		containerZ := int(math.Floor(spawnPoint.Z))

		t.Logf("spawn point: %+v", spawnPoint)
		t.Logf("calculated container block coords: X=%d Y=%d Z=%d", containerX, containerY, containerZ)

		// Ensure stable footing and clear line of sight to avoid "flying" kicks in survival.
		platformY := containerY - 1
		fillX1, fillX2 := containerX-3, containerX+3
		fillZ1, fillZ2 := containerZ-3, containerZ+3
		airTopY := containerY + 3

		_, err = inst.RCON.Exec(ctx, fmt.Sprintf(
			"fill %d %d %d %d %d %d minecraft:stone",
			fillX1, platformY, fillZ1, fillX2, platformY, fillZ2,
		))
		require.NoError(t, err, "create platform under container")

		_, err = inst.RCON.Exec(ctx, fmt.Sprintf(
			"fill %d %d %d %d %d %d minecraft:air",
			fillX1, containerY, fillZ1, fillX2, airTopY, fillZ2,
		))
		require.NoError(t, err, "clear space around container")

		// cmd := fmt.Sprintf("setblock %d %d %d minecraft:%s", containerX, containerY, containerZ, containerBlockType)
		// resp, err := inst.RCON.Exec(ctx, cmd)
		// require.NoError(t, err, "place %s", containerName)
		// t.Logf("setblock response: %s", resp)
		// t.Logf("placed %s at (%d, %d, %d)", containerName, containerX, containerY, containerZ)

		_, err = PlaceBlockAndWait(ctx, inst.RCON, agent, models.V3{
			X: float64(containerX),
			Y: float64(containerY),
			Z: float64(containerZ),
		}, "minecraft:"+containerBlockType, containerBlockType, 30*time.Second)
		require.NoError(t, err, "re-place %s after clearing space", containerName)

		// Calculate container position (center of block)
		containerPos = models.V3{
			X: float64(containerX) + 0.5,
			Y: float64(containerY),
			Z: float64(containerZ) + 0.5,
		}
		t.Logf("calculated container center pos: %+v", containerPos)

	} else {
		// For entity tests, use spawn point as placeholder (entity will be spawned by test)
		containerPos = spawnPoint
		t.Logf("entity test - using spawn point as placeholder: %+v", containerPos)
	}

	// Wait for chunks to load
	time.Sleep(3 * time.Second)

	if enableReplay {
		t.Logf("replay enabled: %s", agentCfg.ReplayOutput)
	}

	return &StandaloneTestEnv{
		Inst:            inst,
		Agent:           agent,
		ContainerHelper: containerHelper,
		ScreenMgr:       screenMgr,
		Ctx:             ctx,
		Cancel:          cancel,
		ContainerPos:    containerPos,
		BotName:         botName,
		Version:         serverCfg.Version,
	}
}

// getContainerBlockType returns the block type string for a container test name
func getContainerBlockType(testName string) string {
	blockTypes := map[string]string{
		"chest":             "chest",
		"barrel":            "barrel",
		"furnace":           "furnace",
		"blast_furnace":     "blast_furnace",
		"smoker":            "smoker",
		"dispenser":         "dispenser",
		"dropper":           "dropper",
		"hopper":            "hopper",
		"shulker_box":       "shulker_box[facing=up]",
		"anvil":             "anvil",
		"crafting_table":    "crafting_table",
		"grindstone":        "grindstone",
		"smithing_table":    "smithing_table",
		"crafter":           "crafter",
		"brewing_stand":     "brewing_stand",
		"enchanting_table":  "enchanting_table",
		"loom":              "loom",
		"cartography_table": "cartography_table",
		"stonecutter":       "stonecutter",
		"lectern":           "lectern",
		"beacon":            "beacon",
	}

	if blockType, ok := blockTypes[testName]; ok {
		return blockType
	}
	return testName // fallback to test name
}

// // getContainerPosition returns the position for the placed container
// func getContainerPosition(inst *TestInstance, ctx context.Context, botName string) (models.V3, error) {
// 	playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
// 	if err != nil {
// 		return models.V3{}, err
// 	}

// 	return models.V3{
// 		X: playerPos.X + 5 + 0.5,
// 		Y: playerPos.Y,
// 		Z: playerPos.Z + 0.5,
// 	}, nil
// }

// // lookAtContainer calculates and sends rotation to look at a container
// // DEPRECATED: This function is no longer needed - agent.OpenContainer() handles rotation automatically
// func lookAtContainer(env *StandaloneTestEnv, containerPos models.V3) error {
// 	// Get bot's current position
// 	playerPos, err := GetPlayerPosition(env.Ctx, env.Inst.RCON, env.BotName)
// 	if err != nil {
// 		return err
// 	}

// 	// Calculate look angles from bot's eyes to container center
// 	// Standard player eye height is 1.62 blocks above feet
// 	botEyeY := playerPos.Y + 1.62
// 	containerCenterY := containerPos.Y + 0.5 // Center of block

// 	dx := containerPos.X - playerPos.X
// 	dy := containerCenterY - botEyeY
// 	dz := containerPos.Z - playerPos.Z

// 	// Calculate horizontal distance
// 	horizontalDist := math.Sqrt(dx*dx + dz*dz)

// 	// Calculate yaw (rotation around Y axis)
// 	// Yaw 0 is south (+Z), 90 is west (-X), 180 is north (-Z), 270 is east (+X)
// 	yaw := float32(math.Atan2(-dx, dz) * 180 / math.Pi)

// 	// Calculate pitch (rotation around X axis)
// 	// Pitch -90 is up, 0 is level, 90 is down
// 	pitch := float32(-math.Atan2(dy, horizontalDist) * 180 / math.Pi)

// 	// Send rotation packet
// 	return movement.SendRotation(env.Agent.BotClient(), env.Agent.Config.PacketMgr, yaw, pitch, true)
// }

// TestChest_Standalone runs the chest test with its own server
func TestChest_Standalone(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "chest", tt.MCVersion)
			defer env.Cancel()

			// Teleport near chest
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Open chest using agent (handles rotation and continuous position packets automatically)
			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceEast, 5*time.Second)
			require.NoError(t, err, "open chest")
			t.Logf("chest opened with window ID: %d", windowID)

			// Verify it's a chest
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "chest window should exist")

			chest, ok := screen.(*mcscreen.Chest)
			require.True(t, ok, "screen should be a Chest")
			require.Equal(t, 3, chest.Rows, "should be single chest (3 rows)")

			// Close chest using agent
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close chest")

			t.Log("✓ Chest standalone test passed")
		})
	}
}

// TestBarrel_Standalone runs the barrel test with its own server
func TestBarrel_Standalone(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "barrel", tt.MCVersion)
			defer env.Cancel()

			// Teleport near barrel
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Open barrel using agent (handles rotation and continuous position packets automatically)
			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceEast, 5*time.Second)
			require.NoError(t, err, "open barrel")
			t.Logf("barrel opened with window ID: %d", windowID)

			// Verify it's a chest-type container (barrels use same type as chests)
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "barrel window should exist")

			chest, ok := screen.(*mcscreen.Chest)
			require.True(t, ok, "barrel should use Chest container type")
			require.Equal(t, 63, len(chest.Slots), "barrel should have 63 total slots")

			// Close barrel using agent
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close barrel")

			t.Log("✓ Barrel standalone test passed")
		})
	}
}

// TestFurnace_Standalone runs the furnace test with its own server
func TestFurnace_Standalone(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "furnace", tt.MCVersion)
			defer env.Cancel()

			// Teleport near furnace
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			// Open furnace using agent (handles rotation and continuous position packets automatically)
			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceNorth, 5*time.Second)
			require.NoError(t, err, "open furnace")
			t.Logf("furnace opened with window ID: %d", windowID)

			// Verify it's a GenericContainer (type 14)
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "furnace window should exist")

			genericContainer, ok := screen.(*mcscreen.GenericContainer)
			require.True(t, ok, "furnace should be a GenericContainer")
			require.Equal(t, int32(14), genericContainer.Type, "should be type 14 (furnace)")
			require.Equal(t, 3, genericContainer.ContainerSlots, "furnace should have 3 container slots")

			// Close furnace using agent
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close furnace")

			t.Log("✓ Furnace standalone test passed")
		})
	}
}

// TestShulkerBox_Standalone runs the shulker box test with its own server
func TestShulkerBox_Standalone(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "shulker_box", tt.MCVersion)
			defer env.Cancel()

			// Teleport near shulker box
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			cmd = fmt.Sprintf("data get block %.1f %.1f %.1f", env.ContainerPos.X, env.ContainerPos.Y, env.ContainerPos.Z)
			resp, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			t.Logf("shulker box block data: %s", resp)

			// Open shulker box using agent (handles rotation and continuous position packets automatically)
			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceUp, 5*time.Second)
			require.NoError(t, err, "open shulker box")
			t.Logf("shulker box opened with window ID: %d", windowID)

			// Verify it's a GenericContainer (type 20)
			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "shulker box window should exist")

			genericContainer, ok := screen.(*mcscreen.GenericContainer)
			require.True(t, ok, "shulker box should be a GenericContainer")
			require.Equal(t, int32(20), genericContainer.Type, "should be type 20 (shulker_box)")
			require.Equal(t, 63, len(genericContainer.Slots), "shulker box should have 63 total slots")
			require.Equal(t, 27, genericContainer.ContainerSlots, "shulker box should have 27 container slots")

			// Close shulker box using agent
			err = env.Agent.Agent.CloseContainer()
			require.NoError(t, err, "close shulker box")

			t.Log("✓ Shulker box standalone test passed")
		})
	}
}
