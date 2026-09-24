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

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/suite"
)

// ContainerTestSuite covers all 16 container-type tests below (chest,
// barrel, furnace family, crafting family, utility blocks): one server per
// version, shared by every method, instead of the previous
// per-test-function StartServer/StopServer pattern (this file's own prior
// art, which already achieved the "one server, many tests" benefit via its
// own hand-rolled SetupSuite/TearDownSuite driver before VersionWorldSuite
// existed - this migration is pure consistency, not a boot-count win).
//
// Unlike most VersionWorldSuite-based suites, every method here shares ONE
// pre-built world: all 23 container types are placed once, in SetupSuite,
// around a single working-area origin (via a throwaway setup agent that
// disconnects once the build is done) - this preserves the original's own
// efficient one-time-build design rather than having each of the 16
// methods redundantly place its own single container. Each method then
// spawns its own uniquely-named agent near that shared origin
// (spawnContainerAgent) and teleports to whichever container it needs.
// Each method needing its own distinct agent name (VersionWorldSuite's
// usedNames uniqueness check requires it) also means no explicit
// inventory-clear step is needed between tests, unlike the original's
// SetupTest/TearDownTest: a fresh player identity has no prior inventory,
// whereas the original reused one "ContainerBot" identity across all 16
// tests and had to clear it every time.
type ContainerTestSuite struct {
	VersionWorldSuite

	// origin is the one working area every method's agent spawns near;
	// containers holds every placed container's name -> position, both set
	// once in SetupSuite.
	origin     models.V3
	containers map[string]models.V3

	// leader/screenMgr are the CURRENT test method's agent and screen
	// manager, set by spawnContainerAgent at each method's own start -
	// analogous to the original's own s.agent/s.screenMgr fields, just
	// refreshed per method instead of via testify's SetupTest hook.
	leader    *WorkingAreaAgent
	screenMgr mcscreen.Manager

	memProfilePath     string
	memProfileInterval time.Duration
	memProfileStop     chan struct{}
	memProfileDone     chan struct{}
}

func TestContainerSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ContainerTestSuite{}
		s.WorldGen = WorldGenFlat
		s.Memory = "1024M"
		s.MinFreeMemoryMB = 512
		s.ExtraEnv = map[string]string{
			"FORCE_GAMEMODE":      "true",
			"VIEW_DISTANCE":       "6",
			"SIMULATION_DISTANCE": "4",
			"SPAWN_PROTECTION":    "0",
		}
		return s
	})
}

// SetupSuite boots the shared server (via the embedded VersionWorldSuite),
// then builds the one shared container world every method in this suite
// uses.
func (s *ContainerTestSuite) SetupSuite() {
	s.VersionWorldSuite.SetupSuite()

	// Enable debug logging for container clicks and screen close.
	os.Setenv("MC_AGENT_CLICK_DEBUG_PATH", "testing/logs/container_debug.log")

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

	// Spawn a throwaway setup agent to build the world and capture the
	// working-area origin every real test method will spawn near.
	setupAgent, err := s.SpawnWorkingAreaAgent("ContainerSetupBot", "container_suite_setup")
	s.Require().NoError(err, "spawn setup agent")
	s.origin = setupAgent.Origin

	s.T().Log("building test world with all container types...")
	s.containers = s.buildTestWorld(setupAgent)
	s.T().Logf("test world built with %d containers", len(s.containers))

	// Disconnect the setup agent immediately - it has no further role, and
	// every real test method spawns its own fresh agent instead.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
	_ = setupAgent.Stop(stopCtx)
	stopCancel()

	if memStatsEnabled() {
		logMemStats("ContainerTestSuite setup complete")
	}
}

// TearDownSuite only handles memory-profiling cleanup - the shared
// server itself is stopped via VersionWorldSuite's own t.Cleanup-based
// teardown (teardownServer), not testify's TearDownSuite hook (see that
// method's own doc comment for why), so this must not touch the
// server/framework at all.
func (s *ContainerTestSuite) TearDownSuite() {
	s.stopMemProfiler()
	s.writeMemProfile()
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
			case <-s.Ctx.Done():
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

// spawnContainerAgent spawns a fresh agent named name, near this suite's
// one shared origin (built once in SetupSuite with every container type in
// reach), and stores it as s.leader/s.screenMgr for
// teleportToContainer/openContainer to use. Call this as the first line of
// every Test* method below - each needs its own distinct name
// (VersionWorldSuite's usedNames uniqueness check requires it).
func (s *ContainerTestSuite) spawnContainerAgent(name, replayPrefix string) {
	leader, err := s.SpawnAgentNear(name, replayPrefix, s.origin, 0, 0)
	s.Require().NoError(err, "spawn agent")
	s.leader = leader
	s.screenMgr = leader.ScreenManager()
}

// buildTestWorld creates all containers in the test world using setupAgent
// (SetupSuite's own throwaway agent - not s.leader, which doesn't exist
// yet at this point in the suite's lifecycle). Returns a map of container
// names to positions.
func (s *ContainerTestSuite) buildTestWorld(setupAgent *WorkingAreaAgent) map[string]models.V3 {
	containers := make(map[string]models.V3)

	// Base Y level (ground level)
	baseX := int(math.Floor(s.origin.X))
	baseY := math.Floor(s.origin.Y)
	baseZ := int(math.Floor(s.origin.Z))
	if s.origin.Y >= 65 {
		// For flat worlds, ground is at Y=64.
		baseY = 64
	}

	// Ensure ground is clear
	s.T().Log("clearing ground...")
	groundY := int(baseY)
	floorCmd := fmt.Sprintf("fill %d %d %d %d %d %d grass_block",
		baseX-20, groundY, baseZ-20,
		baseX+20, groundY, baseZ+20)
	response, err := s.Inst.RCON.Exec(s.Ctx, floorCmd)
	s.T().Logf("[buildTestWorld] floor fill response => '%s' (%#v)", response, err)
	clearCmd := fmt.Sprintf("fill %d %d %d %d %d %d air",
		baseX-20, groundY+1, baseZ-20,
		baseX+20, groundY+6, baseZ+20)
	response, err = s.Inst.RCON.Exec(s.Ctx, clearCmd)
	s.T().Logf("[buildTestWorld] clear fill response => '%s' (%#v)", response, err)
	time.Sleep(3 * time.Second)

	baseY = float64(groundY + 1)
	s.origin.Y = baseY

	// Place containers according to the layout plan
	s.T().Log("placing containers...")

	// Helper function to place a container
	placeContainer := func(name, blockType string, offsetX, offsetY, offsetZ int) {
		pos := models.V3{
			X: float64(baseX + offsetX),
			Y: baseY + float64(offsetY),
			Z: float64(baseZ + offsetZ),
		}
		_, err := PlaceBlockAndWait(s.Ctx, s.Inst.RCON, setupAgent.ManagedAgent, pos, "minecraft:"+blockType, blockType, 10*time.Second)
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
			_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d iron_block",
				beaconX+dx, beaconY-1, beaconZ+dz))
		}
	}
	// Place beacon on top
	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d beacon",
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
	teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f facing %.1f %.1f %.1f",
		s.leader.Name, pos.X-2, pos.Y, pos.Z, pos.X, pos.Y, pos.Z)
	_, err := s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	s.Require().NoError(err, "teleport to container")

	// Wait for block to be visible
	time.Sleep(500 * time.Millisecond)
	_, err = WaitForBlockState(s.Ctx, s.leader.ManagedAgent, pos, "", 5*time.Second)
	s.Require().NoError(err, "wait for container block to load")

	return pos
}

// Helper method to open a container with retry logic
func (s *ContainerTestSuite) openContainer(pos models.V3, face models.BlockFace) byte {
	windowID, err := OpenContainerWithLOS(s.Ctx, s.leader.Agent, pos, face, 5*time.Second)
	s.Require().NoError(err, "open container at (%.0f, %.0f, %.0f)", pos.X, pos.Y, pos.Z)
	return windowID
}

// Helper to get screen IDs for logging
func getScreenIDs(screens map[int]mcscreen.Container) []int {
	ids := make([]int, 0, len(screens))
	for id := range screens {
		ids = append(ids, id)
	}
	return ids
}
