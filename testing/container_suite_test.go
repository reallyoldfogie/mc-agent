package testing

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/suite"
)

// ContainerTestSuite provides a shared test environment for all container tests
type ContainerTestSuite struct {
	suite.Suite

	minecraftVersion string

	ctx       context.Context
	cancel    context.CancelFunc
	framework *Framework
	inst      *TestInstance
	agent     *ManagedAgent

	// Shared resources
	screenMgr       mcscreen.Manager
	itemUsage       *items.ItemUsage
	invMgr          models.InventoryManager
	containerHelper *items.ContainerHelper

	// Container positions (pre-placed in world)
	containers map[string]models.V3

	// Spawn point for teleporting back
	spawnPoint models.V3

	currentTestStartTime time.Time
	memProfilePath       string
	memProfileInterval   time.Duration
	memProfileStop       chan struct{}
	memProfileDone       chan struct{}
}

// SetupSuite runs once before all tests in the suite
func (s *ContainerTestSuite) SetupSuite() {
	var err error
	if s.minecraftVersion == "" {
		s.minecraftVersion = "1.21.5"
	}
	s.T().Logf("setting up test suite for Minecraft %s...", s.minecraftVersion)

	// Enable debug logging for container clicks and screen close
	os.Setenv("MC_AGENT_CLICK_DEBUG_PATH", "testing/logs/container_debug.log")

	// Create context with long timeout for entire suite
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Minute)

	// Get working directory
	cwd, err := os.Getwd()
	s.Require().NoError(err, "get current working directory")

	// Create framework
	s.framework, err = NewFramework()
	s.Require().NoError(err, "create framework")
	s.T().Log("framework initialized")

	// Configure server
	serverCfg := DefaultServerConfig()
	serverCfg.Memory = "1024M" // More memory for complex world
	serverCfg.MinFreeMemoryMB = 512
	serverCfg.Version = s.minecraftVersion
	serverCfg.GameMode = "survival"
	serverCfg.WorldGen = WorldGenFlat
	serverCfg.ExtraEnv = map[string]string{
		"FORCE_GAMEMODE":      "true",
		"VIEW_DISTANCE":       "6",
		"SIMULATION_DISTANCE": "4",
		"SPAWN_PROTECTION":    "0",
	}
	serverCfg.PullImage = false
	serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "ContainerTestSuite", s.minecraftVersion)
	RequireIntegrationEnv(s.T(), serverCfg)

	// Optional memory profile output path (file or directory).
	s.memProfilePath = os.Getenv("MC_AGENT_MEM_PROFILE")
	if s.memProfilePath != "" {
		s.memProfileInterval = 2 * time.Minute
		if intervalRaw := strings.TrimSpace(os.Getenv("MC_AGENT_MEM_PROFILE_INTERVAL")); intervalRaw != "" {
			interval, err := time.ParseDuration(intervalRaw)
			if err != nil {
				s.T().Logf("memory profiling: invalid interval %q, disabling periodic profiling", intervalRaw)
				s.memProfileInterval = 0
			} else {
				s.memProfileInterval = interval
			}
		}
		s.T().Logf("memory profiling enabled: %s (interval: %s)", s.memProfilePath, s.memProfileInterval)
		s.startMemProfiler()
	}

	// Start server
	s.inst, err = s.framework.StartServer(s.ctx, serverCfg)
	s.Require().NoError(err, "start server")
	s.T().Logf("server started: %s:%d", s.inst.Server.Host, s.inst.Server.HostServerPort)

	// Setup agent logging
	s.Require().NoError(s.framework.setupAgentLogging(), "setup agent logging")

	// Spawn a setup agent to build the world and capture spawn point.
	addr := fmt.Sprintf("%s:%d", s.inst.Server.Host, s.inst.Server.HostServerPort)
	botName := "ContainerBot"
	agentCfg := AgentConfig{
		Name:          botName,
		ServerAddress: addr,
		Version:       serverCfg.Version,
	}

	s.agent, err = s.framework.SpawnAgent(s.ctx, s.inst, agentCfg)
	s.Require().NoError(err, "spawn agent")

	// Wait for player to be online
	s.Require().True(waitForPlayerOnline(s.ctx, s.inst.RCON, botName, 30*time.Second),
		"agent never appeared in server player list")

	// Get spawn point
	playerPos, err := GetPlayerPosition(s.ctx, s.inst.RCON, botName)
	s.Require().NoError(err, "get player position")
	s.spawnPoint = models.V3{
		X: playerPos.X,
		Y: playerPos.Y,
		Z: playerPos.Z,
	}
	if serverCfg.WorldGen == WorldGenFlat {
		// Flat world default ground is at Y=64, keep agent's feet on ground.
		s.spawnPoint.Y = 65
	}
	s.T().Logf("spawn point: %+v", s.spawnPoint)

	// Build the test world with all containers
	s.T().Log("building test world with all container types...")
	s.containers = s.buildTestWorld()
	s.T().Logf("test world built with %d containers", len(s.containers))

	// Disconnect setup agent to ensure each test starts with a fresh agent.
	s.stopAgent(s.agent)
	s.agent = nil
	s.screenMgr = nil
	s.itemUsage = nil
	s.invMgr = nil
	s.containerHelper = nil

	if memStatsEnabled() {
		logMemStats("ContainerTestSuite setup complete")
	}
}

func memStatsEnabled() bool {
	val := strings.TrimSpace(os.Getenv("MC_AGENT_MEM_STATS"))
	if val == "" {
		return false
	}
	switch strings.ToLower(val) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func logMemStats(label string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	rssMB, vmsMB := readRSSStatsMB()
	log.Printf("[MemStats] %s: HeapAlloc=%.1fMB TotalAlloc=%.1fMB Sys=%.1fMB RSS=%.1fMB VMS=%.1fMB NumGC=%d",
		label,
		float64(ms.HeapAlloc)/1024/1024,
		float64(ms.TotalAlloc)/1024/1024,
		float64(ms.Sys)/1024/1024,
		rssMB,
		vmsMB,
		ms.NumGC,
	)
}

func readRSSStatsMB() (rssMB float64, vmsMB float64) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			rssMB = parseKBLineMB(line)
		} else if strings.HasPrefix(line, "VmSize:") {
			vmsMB = parseKBLineMB(line)
		}
	}
	return rssMB, vmsMB
}

func parseKBLineMB(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	kb, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0
	}
	return kb / 1024
}

// TearDownSuite runs once after all tests in the suite
func (s *ContainerTestSuite) TearDownSuite() {
	s.T().Log("tearing down test suite...")

	s.stopMemProfiler()
	s.writeMemProfile()

	// Close any remaining agent
	s.stopAgent(s.agent)

	// Close agent logging
	if s.framework != nil {
		s.framework.CloseAgentLog()
	}

	// Stop server
	if s.inst != nil && s.framework != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = s.framework.StopServer(stopCtx, s.inst, true)
	}

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	s.T().Log("test suite teardown complete")
}

func (s *ContainerTestSuite) startMemProfiler() {
	if s.memProfilePath == "" || s.memProfileInterval <= 0 {
		return
	}
	s.memProfileStop = make(chan struct{})
	s.memProfileDone = make(chan struct{})
	go func() {
		s.writeMemProfile()
		ticker := time.NewTicker(s.memProfileInterval)
		defer ticker.Stop()
		defer close(s.memProfileDone)
		for {
			select {
			case <-ticker.C:
				s.writeMemProfile()
			case <-s.memProfileStop:
				return
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *ContainerTestSuite) stopMemProfiler() {
	if s.memProfileStop == nil {
		return
	}
	close(s.memProfileStop)
	<-s.memProfileDone
	s.memProfileStop = nil
	s.memProfileDone = nil
}

func (s *ContainerTestSuite) writeMemProfile() {
	if s.memProfilePath == "" {
		return
	}

	path := s.memProfileOutputPath()

	f, err := os.Create(path)
	if err != nil {
		s.T().Logf("memory profile: failed to create %s: %v", path, err)
		return
	}
	defer f.Close()

	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		s.T().Logf("memory profile: failed to write %s: %v", path, err)
		return
	}
	s.T().Logf("memory profile written: %s", path)
}

func (s *ContainerTestSuite) memProfileOutputPath() string {
	path := s.memProfilePath
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return filepath.Join(path, fmt.Sprintf("heap_%s.pprof", time.Now().Format("20060102_150405")))
	}
	if s.memProfileInterval > 0 {
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)
		if ext == "" {
			ext = ".pprof"
		}
		return filepath.Join(dir, fmt.Sprintf("%s_%s%s", name, time.Now().Format("20060102_150405"), ext))
	}
	return path
}

// SetupTest runs before each test
func (s *ContainerTestSuite) SetupTest() {
	// Spawn a fresh agent for this test.
	addr := fmt.Sprintf("%s:%d", s.inst.Server.Host, s.inst.Server.HostServerPort)
	agentCfg := AgentConfig{
		Name:          "ContainerBot",
		ServerAddress: addr,
		Version:       s.inst.Server.Version,
	}

	var err error
	s.agent, err = s.framework.SpawnAgent(s.ctx, s.inst, agentCfg)
	s.Require().NoError(err, "spawn agent")

	// Get shared resources for this agent.
	s.screenMgr = s.agent.ScreenManager()
	s.Require().NotNil(s.screenMgr, "screen manager should be available")

	botClient := s.agent.BotClient()
	s.Require().NotNil(botClient, "bot client should be available")

	s.itemUsage = items.NewItemUsage(botClient.Conn(), s.agent.Config.PacketMgr)
	// Set version-specific container handler
	if s.agent.Config.VersionHandler != nil {
		s.itemUsage.SetContainerHandler(s.agent.Config.VersionHandler.Play().Containers())
	}
	s.invMgr = items.NewInventoryManager(s.screenMgr)
	s.invMgr.SetWaitForUpdates(false) // Use workaround for ServerUpdateVersion issue
	s.containerHelper = items.NewContainerHelper(s.itemUsage, s.invMgr, s.screenMgr, botClient, s.agent.Config.PacketMgr)
	s.agent.Agent.SetContainerHelper(s.containerHelper)

	// Ensure agent is online before issuing RCON commands.
	if !WaitForPlayerOnline(s.ctx, s.inst.RCON, "ContainerBot", 30*time.Second) {
		if listResp, err := s.inst.RCON.Exec(s.ctx, "list"); err == nil {
			s.T().Logf("player list: %s", listResp)
		}
		select {
		case <-s.agent.Agent.Done():
			s.T().Log("agent context closed before SetupTest")
		default:
		}
		s.Require().Fail("ContainerBot not online before SetupTest")
	}

	// Teleport agent back to spawn
	teleportCmd := fmt.Sprintf("tp ContainerBot %.1f %.1f %.1f", s.spawnPoint.X, s.spawnPoint.Y, s.spawnPoint.Z)
	_, err = s.inst.RCON.Exec(s.ctx, teleportCmd)
	s.Require().NoError(err, "teleport to spawn")
	time.Sleep(200 * time.Millisecond)

	// Clear agent inventory
	_, err = s.inst.RCON.Exec(s.ctx, "clear ContainerBot")
	s.Require().NoError(err, "clear inventory")
	// Wait longer for inventory clear to propagate to client
	time.Sleep(500 * time.Millisecond)
	s.currentTestStartTime = time.Now()
}

// TearDownTest runs after each test
func (s *ContainerTestSuite) TearDownTest() {
	// Log current screen count before close
	if s.screenMgr != nil {
		s.T().Logf("Screens before close: %d screens: %v", len(s.screenMgr.Screens()), getScreenIDs(s.screenMgr.Screens()))
	}

	// Close any open containers
	if s.containerHelper != nil {
		_ = s.containerHelper.CloseContainer()
	}

	// Wait to ensure window is fully closed on server and client before next test
	// This prevents "accessing containers too quickly" and hitting window ID limits
	time.Sleep(1 * time.Second)

	// Log screen count after close
	if s.screenMgr != nil {
		s.T().Logf("Screens after close: %d screens: %v", len(s.screenMgr.Screens()), getScreenIDs(s.screenMgr.Screens()))
	}

	s.stopAgent(s.agent)

	for time.Since(s.currentTestStartTime) < 10*time.Second {
		// Ensure at least 10 seconds between tests to avoid server issues
		time.Sleep(100 * time.Millisecond)
	}
	s.agent = nil
	s.screenMgr = nil
	s.itemUsage = nil
	s.invMgr = nil
	s.containerHelper = nil
}

func (s *ContainerTestSuite) stopAgent(agent *ManagedAgent) {
	if agent == nil {
		return
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := agent.Stop(stopCtx); err != nil {
		s.T().Logf("warning: failed to stop agent %s cleanly: %v", agent.Name, err)
	}
	stopCancel()
	s.removeAgent(agent)
}

func (s *ContainerTestSuite) removeAgent(agent *ManagedAgent) {
	if s.inst == nil || agent == nil {
		return
	}
	s.inst.mu.Lock()
	defer s.inst.mu.Unlock()
	for i, existing := range s.inst.Agents {
		if existing == agent {
			s.inst.Agents = append(s.inst.Agents[:i], s.inst.Agents[i+1:]...)
			return
		}
	}
}

// Helper to get screen IDs for logging
func getScreenIDs(screens map[int]mcscreen.Container) []int {
	ids := make([]int, 0, len(screens))
	for id := range screens {
		ids = append(ids, id)
	}
	return ids
}

// buildTestWorld creates all containers in the test world
// Returns a map of container names to positions
func (s *ContainerTestSuite) buildTestWorld() map[string]models.V3 {
	containers := make(map[string]models.V3)

	// Base Y level (ground level)
	baseX := int(math.Floor(s.spawnPoint.X))
	baseY := math.Floor(s.spawnPoint.Y)
	baseZ := int(math.Floor(s.spawnPoint.Z))
	if s.spawnPoint.Y >= 65 {
		// For flat worlds, ground is at Y=64.
		baseY = 64
	}

	// Ensure ground is clear
	s.T().Log("clearing ground...")
	// for x := -20; x <= 20; x++ {
	// 	for z := -20; z <= 20; z++ {
	// 		// Clear blocks above spawn
	// 		for y := 0; y <= 5; y++ {
	// 			cmd := fmt.Sprintf("setblock %d %d %d air",
	// 				baseX+x, int(baseY)+y, baseZ+z)
	// response, err := s.inst.RCON.Exec(s.ctx, cmd)
	// s.T().Logf("[buildTestWorld] setblock response => '%s' (%#v)", response, err)
	// 		}
	// 	}
	// }
	groundY := int(baseY)
	floorCmd := fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
		baseX-20, groundY, baseZ-20,
		baseX+20, groundY, baseZ+20)
	response, err := s.inst.RCON.Exec(s.ctx, floorCmd)
	s.T().Logf("[buildTestWorld] floor fill response => '%s' (%#v)", response, err)
	clearCmd := fmt.Sprintf("fill %d %d %d %d %d %d air",
		baseX-20, groundY+1, baseZ-20,
		baseX+20, groundY+6, baseZ+20)
	response, err = s.inst.RCON.Exec(s.ctx, clearCmd)
	s.T().Logf("[buildTestWorld] clear fill response => '%s' (%#v)", response, err)
	time.Sleep(3 * time.Second)

	baseY = float64(groundY + 1)
	s.spawnPoint.Y = baseY

	// Place containers according to the layout plan
	s.T().Log("placing containers...")

	// Helper function to place a container
	placeContainer := func(name, blockType string, offsetX, offsetY, offsetZ int) {
		pos := models.V3{
			X: float64(baseX + offsetX),
			Y: baseY + float64(offsetY),
			Z: float64(baseZ + offsetZ),
		}
		_, err := PlaceBlockAndWait(s.ctx, s.inst.RCON, s.agent, pos, "minecraft:"+blockType, blockType, 10*time.Second)
		if err != nil {
			s.T().Logf("WARNING: failed to place %s: %v", name, err)
			return
		}
		containers[name] = pos
		s.T().Logf("  placed %s at (%.0f, %.0f, %.0f)", name, pos.X, pos.Y, pos.Z)
	}

	// East side: Crafting interfaces
	placeContainer("crafting_table", "crafting_table", 5, 0, 0)
	placeContainer("crafter", "crafter", 5, 0, 2)
	placeContainer("dispenser", "dispenser", 5, 0, 4)
	placeContainer("dropper", "dropper", 5, 0, 6)

	// West side: Furnaces
	placeContainer("furnace", "furnace", -5, 0, 0)
	placeContainer("blast_furnace", "blast_furnace", -5, 0, 2)
	placeContainer("smoker", "smoker", -5, 0, 4)

	// North side: Utility blocks
	placeContainer("hopper", "hopper", 0, 0, -5)
	placeContainer("grindstone", "grindstone", 2, 0, -5)
	placeContainer("stonecutter", "stonecutter", 4, 0, -5)
	placeContainer("anvil", "anvil", 6, 0, -5)

	// South side: Special containers
	placeContainer("enchanting_table", "enchanting_table", 0, 0, 5)
	placeContainer("loom", "loom", 2, 0, 5)
	placeContainer("cartography_table", "cartography_table", 4, 0, 5)
	placeContainer("smithing_table", "smithing_table", 6, 0, 5)
	placeContainer("brewing_stand", "brewing_stand", 8, 0, 5)

	// Special cases
	placeContainer("lectern", "lectern", 0, 0, 6)
	placeContainer("barrel", "barrel", 2, 0, 6)
	placeContainer("shulker_box", "shulker_box[facing=up]", 4, 0, 6)

	// Chest variants
	placeContainer("chest", "chest", 0, 0, 2)
	// Double chest (two chests side by side)
	placeContainer("double_chest_1", "chest[facing=north,type=left]", -2, 0, 2)
	placeContainer("double_chest_2", "chest[facing=north,type=right]", -1, 0, 2)

	// Beacon requires pyramid base
	s.T().Log("placing beacon with pyramid...")
	beaconX := baseX + 10
	beaconY := int(baseY)
	beaconZ := baseZ
	// Place iron block pyramid base (3x3)
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			_, _ = s.inst.RCON.Exec(s.ctx, fmt.Sprintf("setblock %d %d %d iron_block",
				beaconX+dx, beaconY-1, beaconZ+dz))
		}
	}
	// Place beacon on top
	_, _ = s.inst.RCON.Exec(s.ctx, fmt.Sprintf("setblock %d %d %d beacon",
		beaconX, beaconY, beaconZ))
	containers["beacon"] = models.V3{X: float64(beaconX), Y: float64(beaconY), Z: float64(beaconZ)}

	// Wait for blocks to be sent to client
	s.T().Log("waiting for blocks to load on client...")
	time.Sleep(3 * time.Second)

	return containers
}

// Helper method to get a container position by name
func (s *ContainerTestSuite) getContainer(name string) models.V3 {
	pos, ok := s.containers[name]
	s.Require().True(ok, "container %s should exist in test world", name)
	return pos
}

// Helper method to teleport near a container and wait for block load
func (s *ContainerTestSuite) teleportToContainer(name string) models.V3 {
	pos := s.getContainer(name)

	// Teleport player next to container
	teleportCmd := fmt.Sprintf("tp ContainerBot %.1f %.1f %.1f facing %.1f %.1f %.1f",
		pos.X-2, pos.Y, pos.Z, pos.X, pos.Y, pos.Z)
	_, err := s.inst.RCON.Exec(s.ctx, teleportCmd)
	s.Require().NoError(err, "teleport to container")

	// Wait for block to be visible
	time.Sleep(500 * time.Millisecond)
	_, err = WaitForBlockState(s.ctx, s.agent, pos, "", 5*time.Second)
	s.Require().NoError(err, "wait for container block to load")

	return pos
}

// Helper method to open a container with retry logic
func (s *ContainerTestSuite) openContainer(pos models.V3, face items.BlockFace) byte {
	windowID, err := OpenContainerWithLOS(s.ctx, s.agent.Agent, pos, face, 5*time.Second)
	s.Require().NoError(err, "open container at (%.0f, %.0f, %.0f)", pos.X, pos.Y, pos.Z)
	return windowID
}

// TestContainerSuite runs the entire suite
func TestContainerSuite(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			testSuite := new(ContainerTestSuite)
			testSuite.minecraftVersion = tt.MCVersion
			suite.Run(t, testSuite)
		})
		// suite.Run(t, new(ContainerTestSuite))
	}
}
