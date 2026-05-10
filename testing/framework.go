package testing

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/reallyoldfogie/mc-agent/agent"
	_ "github.com/reallyoldfogie/mc-agent/handler_versions" // Import to register version handlers
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	agentutils "github.com/reallyoldfogie/mc-agent/utils"

	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/reallyoldfogie/mc-bot-go/utils"

	"github.com/reallyoldfogie/mc-client-test-go/testenv"

	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
)

// Framework orchestrates integration tests for Minecraft agents.
// It manages server lifecycle, agent spawning, and test execution.
type Framework struct {
	serverMgr        testenv.Manager
	instances        map[string]*TestInstance // keyed by test name
	agentLogFile     *os.File                 // Log file for all agents
	agentLogWriter   io.Writer                // MultiWriter for agents (file + stdout)
	agentLogFilename string                   // full path to log file
	logSetupMu       sync.Mutex               // Mutex for log setup
}

// TestInstance represents a complete test environment with server and agents.
type TestInstance struct {
	Server       *testenv.Instance
	RCON         testenv.RCONHelper
	Agents       []*ManagedAgent
	AgentLogFile string // Path to the agent log file
	mu           sync.RWMutex
}

// ManagedAgent wraps an agent instance with lifecycle tracking.
type ManagedAgent struct {
	Name      string
	Agent     models.Agent
	Config    models.AgentConfig
	Cam       *ManagedAgent
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	mu        sync.Mutex
	botClient bot.Client
}

func (ma *ManagedAgent) BotClient() bot.Client {
	return ma.botClient
}

// ScreenManager returns the concrete screen manager for tests that need direct access.
// For most tests, use the agent's ScreenOperations interface methods instead.
func (ma *ManagedAgent) ScreenManager() screen.Manager {
	// Get the screen manager from the agent
	sm := ma.Agent.GetScreenManager()
	if sm == nil {
		return nil
	}
	return sm.(screen.Manager)
}

// GetTrackedEntities returns all tracked entities from the agent
func (ma *ManagedAgent) GetTrackedEntities() map[int32]agent.TrackedEntityInfo {
	return ma.Agent.GetTrackedEntities()
}

// FindNearestEntityByType finds the nearest entity of a specific type to a position
func (ma *ManagedAgent) FindNearestEntityByType(entityType int32, x, y, z float64) (int32, float64, bool) {
	return ma.Agent.FindNearestEntityByType(entityType, x, y, z)
}

// EquipItemByName equips an item from the hotbar by name
func (ma *ManagedAgent) EquipItemByName(ctx context.Context, itemName string) error {
	// Get the concrete agent implementation
	if agentImpl, ok := ma.Agent.(interface {
		EquipItemByName(ctx context.Context, itemName string) error
	}); ok {
		return agentImpl.EquipItemByName(ctx, itemName)
	}
	return fmt.Errorf("agent does not support EquipItemByName")
}

// FaceEntity makes the bot look at a target entity
func (ma *ManagedAgent) FaceEntity(entityID int32) error {
	// Get the concrete agent implementation
	if agentImpl, ok := ma.Agent.(interface {
		FaceEntity(entityID int32) error
	}); ok {
		return agentImpl.FaceEntity(entityID)
	}
	return fmt.Errorf("agent does not support FaceEntity")
}

// LookAt makes the bot look at a target position (head only)
func (ma *ManagedAgent) LookAt(ctx context.Context, x, y, z float64) error {
	return ma.Agent.LookAt(ctx, x, y, z)
}

// TurnTowards rotates the bot's body to face a target position
func (ma *ManagedAgent) TurnTowards(ctx context.Context, x, y, z float64) error {
	return ma.Agent.TurnTowards(ctx, x, y, z)
}

// NewFramework creates a new testing framework.
func NewFramework() (*Framework, error) {
	mgr, err := testenv.NewManager()
	if err != nil {
		return nil, fmt.Errorf("create server manager: %w", err)
	}
	return &Framework{
		serverMgr: mgr,
		instances: make(map[string]*TestInstance),
	}, nil
}

// clearServerWorldData removes persisted world data from a cached server dir.
// This keeps downloaded JARs while ensuring tests start from a clean world.
func clearServerWorldData(cacheDir string) {
	if cacheDir == "" || os.Getenv("TEST_KEEP_SERVER_DATA") != "" {
		return
	}

	worldDirs := []string{"world", "world_nether", "world_the_end"}
	for _, dir := range worldDirs {
		path := filepath.Join(cacheDir, dir)
		log.Printf("deleting %s", path)
		if err := os.RemoveAll(path); err != nil {
			log.Printf("[Framework.StartServer] failed to remove world data %s: %v", path, err)
		}
	}
}

// getModsDir returns the path to the mods directory for a given Minecraft version,
// or empty string if the directory doesn't exist.
// Looks in: mods/v{version} relative to current working directory.
// Version format: "1.21.5" -> looks for "mods/v1_21_5"
func getModsDir(version string) string {
	if version == "" {
		return ""
	}

	// Convert version format: 1.21.5 -> v1_21_5
	modsPath := filepath.Join("mods", "v"+strings.ReplaceAll(version, ".", "_"))

	// Check if directory exists and convert to absolute path for Docker binding
	if _, err := os.Stat(modsPath); err == nil {
		if abs, err := filepath.Abs(modsPath); err == nil {
			return abs
		}
	}

	return ""
}

// getConfigDir returns the path to the config directory for a given Minecraft version,
// or empty string if the directory doesn't exist.
// Looks in: configs/v{version} relative to current working directory (tests run from testing dir).
// Version format: "1.21.5" -> looks for "configs/v1_21_5"
func getConfigDir(version string) string {
	if version == "" {
		return ""
	}

	// Convert version format: 1.21.5 -> v1_21_5
	configPath := filepath.Join("configs", "v"+strings.ReplaceAll(version, ".", "_"))

	// Check if directory exists and convert to absolute path for Docker binding
	if _, err := os.Stat(configPath); err == nil {
		if abs, err := filepath.Abs(configPath); err == nil {
			return abs
		}
	}

	return ""
}

// getOutputDir returns the path to the output directory for a given Minecraft version,
// or empty string if the directory doesn't exist.
// Looks in: outputs/v{version} relative to current working directory (tests run from testing dir).
// Version format: "1.21.5" -> looks for "outputs/v1_21_5"
func getOutputDir(version string) string {
	if version == "" {
		return ""
	}

	// Convert version format: 1.21.5 -> v1_21_5
	outputPath := filepath.Join("outputs", "v"+strings.ReplaceAll(version, ".", "_"))

	// Check if directory exists and convert to absolute path for Docker binding
	if _, err := os.Stat(outputPath); err == nil {
		if abs, err := filepath.Abs(outputPath); err == nil {
			return abs
		}
	}

	return ""
}

// WorldGenType specifies the terrain generation mode for tests.
type WorldGenType string

func (w WorldGenType) String() string {
	return string(w)
}

const (
	WorldGenRandom     WorldGenType = "default"    // Random terrain
	WorldGenFlat       WorldGenType = "flat"       // Flat world
	WorldGenControlled WorldGenType = "controlled" // Flat with programmatic obstacles
)

type Difficulty string

func (d Difficulty) String() string {
	return string(d)
}

const (
	DifficultyPeaceful Difficulty = "peaceful"
	DifficultyEasy     Difficulty = "easy"
	DifficultyNormal   Difficulty = "normal"
	DifficultyHard     Difficulty = "hard"
)

type GameMode string

func (g GameMode) String() string {
	return string(g)
}

const (
	GameModeCreative  GameMode = "creative"
	GameModeSurvival  GameMode = "survival"
	GameModeAdventure GameMode = "adventure"
	GameModeSpectator GameMode = "spectator"
)

// ServerConfig contains configuration for a test server instance.
type ServerConfig struct {
	Version              string
	Difficulty           Difficulty   // peaceful, easy, normal, hard
	GameMode             GameMode     // creative, survival, adventure, spectator
	WorldGen             WorldGenType // World generation type (default: random)
	PullImage            bool
	Memory               string // e.g. "1G", "2G" - Java heap size (default: 512M for tests)
	CacheDir             string // Cache directory for server JARs (default: ~/.cache/mc-agent-test)
	ExtraEnv             map[string]string
	StartTimeout         time.Duration // default 10 minutes
	SkipMemoryCheck      bool          // Skip memory availability check (not recommended)
	MinFreeMemoryMB      int           // Minimum free memory required in MB (default: 256)
	EstimatedMemoryMB    int           // Estimated memory for this server in MB (auto-calculated from Memory field)
	MemoryCheckRetries   int           // Number of times to retry memory check (default: 5)
	MemoryCheckRetryWait time.Duration // Wait between retries (default: 15 seconds)
	MountDirs            []string      // directories to be mounted to container
}

// DefaultServerConfig returns a sensible default configuration for tests.
// Uses reduced memory (512M) to prevent OOM on systems with limited RAM.
// Uses random terrain for full integration testing.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Version:              "1.21.5",
		Difficulty:           "peaceful",
		GameMode:             "survival",
		WorldGen:             WorldGenRandom, // Random terrain for realistic testing
		PullImage:            false,          // set to true to pull latest image
		Memory:               "512M",         // 512MB for Minecraft server
		StartTimeout:         10 * time.Minute,
		SkipMemoryCheck:      false,            // Always check memory availability
		MinFreeMemoryMB:      256,              // Required free memory buffer
		EstimatedMemoryMB:    0,                // Auto-calculated from Memory field
		MemoryCheckRetries:   5,                // Retry 5 times before failing
		MemoryCheckRetryWait: 15 * time.Second, // Wait 5 seconds between retries
	}
}

// FlatWorldServerConfig returns configuration for flat-world testing.
// Use this for movement commands without pathfinding (moveTo, moveForward).
// Ensures terrain is flat and predictable.
func FlatWorldServerConfig() ServerConfig {
	cfg := DefaultServerConfig()
	cfg.WorldGen = WorldGenFlat
	return cfg
}

// ControlledTerrainServerConfig returns configuration for obstacle testing.
// Use this to programmatically place blocks and create reproducible scenarios.
func ControlledTerrainServerConfig() ServerConfig {
	cfg := DefaultServerConfig()
	cfg.WorldGen = WorldGenControlled
	return cfg
}

// StartServer starts a test Minecraft server and waits for it to be ready.
func (f *Framework) StartServer(ctx context.Context, cfg ServerConfig) (*TestInstance, error) {
	if cfg.StartTimeout == 0 {
		cfg.StartTimeout = 10 * time.Minute
	}

	// Override: if running against remote host, skip memory check
	if host := os.Getenv(client.EnvOverrideHost); host != "" {
		if host != "127.0.0.1" {
			cfg.SkipMemoryCheck = true
		}
	}

	// Check memory availability before starting server (unless skipped)
	if !cfg.SkipMemoryCheck {
		if err := f.checkMemoryAvailability(cfg); err != nil {
			return nil, fmt.Errorf("insufficient memory: %w", err)
		}
	}

	startCtx, cancel := context.WithTimeout(ctx, cfg.StartTimeout)
	defer cancel()

	extraEnv := cfg.ExtraEnv
	if extraEnv == nil {
		extraEnv = make(map[string]string)
	}
	if _, ok := extraEnv["VIEW_DISTANCE"]; !ok {
		extraEnv["VIEW_DISTANCE"] = "6"
	}
	if _, ok := extraEnv["SIMULATION_DISTANCE"]; !ok {
		extraEnv["SIMULATION_DISTANCE"] = "4"
	}
	if cfg.Difficulty != "" {
		extraEnv["DIFFICULTY"] = cfg.Difficulty.String()
	}
	if cfg.GameMode != "" {
		extraEnv["MODE"] = cfg.GameMode.String()
	}

	extraEnv["TYPE"] = "FABRIC" // Use Fabric for tests

	// Apply flat-world configuration if requested
	if cfg.WorldGen == WorldGenFlat || cfg.WorldGen == WorldGenControlled {
		// Use the flat world preset: "minecraft:flat"
		extraEnv["LEVEL_TYPE"] = "flat"
		// Flat world preset: grass block at y=64 for testing
		extraEnv["GENERATOR_SETTINGS"] = `{
			"layers":[
				{
					"block":"minecraft:bedrock",
					"height":1
				},
				{
					"block":"minecraft:stone",
					"height":59
				},
				{
					"block":
					"minecraft:dirt",
					"height":3
				},			
				{
					"block":"minecraft:grass_block",
					"height":1
				}
			],
			"biome":"minecraft:plains"
		}`

		// Control structure generation for flat worlds via environment variable
		// Set MC_AGENT_GENERATE_STRUCTURES=false to disable villages, temples, etc.
		// Defaults to false (no structures) for predictable flat world testing
		if _, ok := extraEnv["GENERATE_STRUCTURES"]; !ok {
			if genStructures := os.Getenv("MC_AGENT_GENERATE_STRUCTURES"); genStructures != "" {
				extraEnv["GENERATE_STRUCTURES"] = genStructures
			} else {
				// Default: disable structures for predictable testing in flat worlds
				extraEnv["GENERATE_STRUCTURES"] = "false"
			}
		}
	}
	// Set memory limit to reduce OOM risk
	if cfg.Memory != "" {
		extraEnv["MEMORY"] = cfg.Memory
	}
	// Optimize JVM for container environments
	// extraEnv["JVM_XX_OPTS"] = "-XX:+UseContainerSupport -XX:MaxRAMPercentage=80.0"

	// if _, ok := os.LookupEnv("JVM_OPTS"); ok {
	// 	extraEnv["JVM_OPTS"] += " -Dfabric.development=true -Dlog4j2.configurationFile=/data/log4j2.xml"
	// 	// extraEnv["JVM_OPTS"] += " -Dfabric.development=true"
	// } else {
	// 	extraEnv["JVM_OPTS"] = "-Dfabric.development=true -Dlog4j2.configurationFile=/data/log4j2.xml"
	// 	// extraEnv["JVM_OPTS"] = "-Dfabric.development=true"
	// }

	// extraEnv["LOG_LEVEL"] = "debug" // Enable debug logging for tests
	// extraEnv["LOG_CONSOLE_FORMAT"] = "[%d{yyyy-MM-dd HH:mm:ss.SSS}] [%t/%level]: %msg%n"
	// extraEnv["LOG_FILE_FORMAT"] = "[%d{yyyy-MM-dd HH:mm:ss.SSS}] [%t/%level]: %msg%n"
	// extraEnv["LOG_TERMINAL_FORMAT"] = "[%d{yyyy-MM-dd HH:mm:ss.SSS}] [%t/%level]: %msg%n"

	// Set up cache directory for server JARs to avoid repeated downloads
	// Use version-specific directory so the container can reuse downloaded JARs
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		baseCacheDir, err := agentutils.FindOrCreateCacheDir()
		if err == nil {
			// Use version-specific subdirectory - the container will use this as /data
			// and will find any existing minecraft_server.{VERSION}.jar
			cacheDir = filepath.Join(baseCacheDir, "mc-agent-test", cfg.Version)
			// Create cache directory if it doesn't exist
			if err := os.MkdirAll(cacheDir, 0755); err != nil {
				// If we can't create cache, fall back to no caching
				log.Printf("[Framework.StartServer] failed to create cache directory %s: %s", cacheDir, err.Error())
				cacheDir = ""
			}
		}
	}

	serverCfg := testenv.ServerConfig{
		Version:    cfg.Version,
		PullImage:  cfg.PullImage,
		OnlineMode: false, // offline mode for tests
		NamePrefix: "mc-agent-test-",
		DataDir:    cacheDir,                  // Use persistent cache for server JARs
		ModsDir:    getModsDir(cfg.Version),   // Load mods if they exist for this version
		ConfigDir:  getConfigDir(cfg.Version), // Load configs if they exist for this version
		OutputDir:  getOutputDir(cfg.Version), // Load output dir if it exists for this version
		ExtraEnv:   extraEnv,
	}

	if cfg.MountDirs != nil {
		serverCfg.MountDirs = cfg.MountDirs
	}

	clearServerWorldData(cacheDir)

	inst, err := f.serverMgr.Start(startCtx, serverCfg)
	if err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}

	if err := f.serverMgr.WaitReady(startCtx, inst); err != nil {
		// Try to stop the server on error
		_ = f.serverMgr.Stop(context.Background(), inst, true)
		return nil, fmt.Errorf("wait for server ready: %w", err)
	}

	rcon, err := f.serverMgr.RCONClient(startCtx, inst)
	if err != nil {
		_ = f.serverMgr.Stop(context.Background(), inst, true)
		return nil, fmt.Errorf("create RCON client: %w", err)
	}

	helper := testenv.NewRCONHelper(rcon)

	// Initialize world state with sensible defaults for testing
	_, err = helper.ExecuteMany(startCtx,
		helper.SetTime(startCtx, "day"),
		helper.SetWeather(startCtx, "clear"),
		helper.SetGamerule(startCtx, "doDaylightCycle", "false"),
		helper.SetGamerule(startCtx, "doWeatherCycle", "false"),
		helper.SetGamerule(startCtx, "doMobSpawning", "false"),
	)
	if err != nil {
		_ = f.serverMgr.Stop(context.Background(), inst, true)
		return nil, fmt.Errorf("initialize world state: %w", err)
	}

	// Setup agent logging if not already done
	if err := f.setupAgentLogging(); err != nil {
		_ = f.serverMgr.Stop(context.Background(), inst, true)
		return nil, fmt.Errorf("setup agent logging: %w", err)
	}

	testInst := &TestInstance{
		Server:       inst,
		RCON:         helper,
		Agents:       []*ManagedAgent{},
		AgentLogFile: f.GetAgentLogFilename(),
	}

	return testInst, nil
}

// StopServer stops a test server and cleans up resources.
// Captures server logs before stopping for offline review.
// StopServerWithTimeout stops the server with a strict timeout.
// Wraps the ENTIRE StopServer cleanup (agent stop + server stop) with an enforced timeout.
// If cleanup takes too long, it returns anyway to prevent test hangs.
func (f *Framework) StopServerWithTimeout(inst *TestInstance, remove bool, timeout time.Duration) error {
	errCh := make(chan error, 1)

	go func() {
		// Run the entire StopServer in a goroutine with enforced timeout
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		errCh <- f.StopServer(ctx, inst, remove)
	}()

	select {
	case err := <-errCh:
		// Cleanup completed in time
		if err != nil {
			fmt.Printf("WARNING: StopServer returned error: %v\n", err)
		}
		return nil // Always return nil - cleanup completed or timed out, either way we continue
	case <-time.After(timeout + 500*time.Millisecond):
		// Cleanup exceeded timeout - don't wait forever
		fmt.Printf("WARNING: Server cleanup exceeded %v timeout, forcing continuation\n", timeout)
		return nil // Return nil anyway - test must continue
	}
}

func (f *Framework) StopServer(ctx context.Context, inst *TestInstance, remove bool) error {
	if inst == nil {
		return nil
	}

	// NOTE: Agents are independent of servers and must be stopped by tests explicitly.
	// This framework's StopServer only handles the server lifecycle, not agents.

	// Close RCON connection
	if inst.RCON != nil {
		_ = inst.RCON.Close()
	}

	// Capture server logs before stopping (for debugging)
	if inst.Server != nil {
		if err := f.captureServerLogs(ctx, inst.Server); err != nil {
			fmt.Printf("WARNING: Failed to capture server logs: %v\n", err)
			// Continue with shutdown even if log capture fails
		}

		if os.Getenv("TEST_KEEP_SERVER") == "" { // Only stop/remove if TEST_KEEP_SERVER is not set
			// Stop and optionally remove the container
			if err := f.serverMgr.Stop(ctx, inst.Server, remove); err != nil {
				fmt.Printf("WARNING: Failed to stop server: %v\n", err)
				// Don't return error - cleanup is best-effort
			}
		}
	}

	return nil
}

// setupAgentLogging configures the global log package to write to both file and stdout.
// This is called once when the first agent is spawned.
// All subsequent agents will use the same logging configuration.
func (f *Framework) setupAgentLogging() error {
	f.logSetupMu.Lock()
	defer f.logSetupMu.Unlock()

	// Already setup?
	if f.agentLogWriter != nil {
		return nil
	}

	// Get centralized cache directory
	cacheDir, err := agentutils.FindOrCreateCacheDir()
	if err != nil {
		return fmt.Errorf("find cache directory: %w", err)
	}

	// Create logs directory in cache
	logsDir := filepath.Join(cacheDir, "logs", "agents")
	if err := ensureDir(logsDir); err != nil {
		return fmt.Errorf("create logs directory: %w", err)
	}

	// Create log file with timestamp
	logFile := filepath.Join(logsDir, fmt.Sprintf("agents_%s.log", time.Now().Format("20060102_150405")))
	fullFileName, _ := filepath.Abs(logFile)
	file, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}

	// log before setting log.SetOutput, so the log file location is captured to the console.
	log.Printf("Agent logging enabled: %s (%s)\n", logFile, fullFileName)

	f.agentLogFilename = fullFileName

	// Set global log output to file to avoid buffering logs in test output.
	log.SetOutput(file)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	// Store for cleanup
	f.agentLogFile = file
	f.agentLogWriter = file

	return nil
}

func (f *Framework) GetAgentLogFilename() string {
	return f.agentLogFilename
}

// CloseAgentLog closes the agent log file.
// Should be called during test cleanup.
func (f *Framework) CloseAgentLog() {
	f.logSetupMu.Lock()
	defer f.logSetupMu.Unlock()

	if f.agentLogFile != nil {
		f.agentLogFile.Close()
		f.agentLogFile = nil
		f.agentLogWriter = nil
		// Restore log to stdout only
		log.SetOutput(os.Stdout)
	}
}

// AgentConfig contains configuration for spawning a test agent.
type AgentConfig struct {
	Name              string
	ServerAddress     string
	Version           string
	MCDataGenPath     string
	MCProtocolGoPath  string
	RegistriesPath    string // Path to registries.json directory (contains data_generator/reports/registries.json)
	EnableReplay      bool
	ReplayOutput      string
	SkinCacheDir      string
	SkinNetEnabled    bool
	HPADebugPathBlock string                // Explicit block name to use (expects <color>_stained_glass)
	HPADebugPathColor string                // Color name to use when block is not specified
	VersionHandler    models.VersionHandler // Optional: version-specific packet handler (overrides auto-detection)

	EnableCamAgent bool // Whether to spawn a companion cam agent
}

// DefaultAgentConfig returns a sensible default configuration for test agents.
func DefaultAgentConfig(name, serverAddress, version string) AgentConfig {
	return AgentConfig{
		Name:              validateAgentName(name),
		ServerAddress:     serverAddress,
		Version:           version,
		MCDataGenPath:     "", // Empty = use default (downloads if needed)
		MCProtocolGoPath:  "", // Not needed for tests
		EnableReplay:      false,
		SkinCacheDir:      "skins",
		SkinNetEnabled:    false,
		HPADebugPathBlock: "",
		HPADebugPathColor: "",
		EnableCamAgent:    true,
	}
}

// SpawnAgent creates and starts a new agent connected to the test server.
// It also spawns a companion recording agent (<name>Cam) before the main agent.
func (f *Framework) SpawnAgent(ctx context.Context, inst *TestInstance, cfg AgentConfig) (*ManagedAgent, error) {
	camCfg := cfg
	camCfg.Name = camAgentName(cfg.Name)
	camCfg.EnableReplay = true
	if cfg.ReplayOutput != "" {
		camCfg.ReplayOutput = camReplayOutput(cfg.ReplayOutput, camCfg.Name)
	} else {
		camCfg.ReplayOutput = ""
	}

	managed, err := f.spawnAgentInternal(ctx, inst, cfg, true)
	if err != nil {
		// _ = cam.Stop(context.Background())
		return nil, err
	}

	if cfg.EnableCamAgent {
		cam, err := f.spawnAgentInternal(ctx, inst, camCfg, false)
		if err != nil {
			return nil, fmt.Errorf("spawn cam agent %s: %w", camCfg.Name, err)
		}
		managed.Cam = cam
	}
	return managed, nil
}

func validateAgentName(name string) string {
	const maxLen = 16
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	return name
}
func camAgentName(base string) string {
	const suffix = "Cam"
	const maxLen = 16
	if len(base)+len(suffix) <= maxLen {
		return base + suffix
	}
	trim := max(maxLen-len(suffix), 0)
	if len(base) > trim {
		base = base[:trim]
	}
	return base + suffix
}

func camReplayOutput(base, name string) string {
	if base == "" {
		cacheDir, err := agentutils.FindOrCreateCacheDir()
		if err != nil {
			panic(fmt.Sprintf("find cache directory: %v", err))
		}
		return filepath.Join(cacheDir, "replays", fmt.Sprintf("%s_cam.mcpr", name))
	}
	const suffix = ".mcpr"
	if before, ok := strings.CutSuffix(base, suffix); ok {
		return before + "_cam" + suffix
	}
	return base + "_cam"
}

func normalizeReplayOutput(version, output, name string) string {
	if output == "" {
		output = fmt.Sprintf("%s_%s.mcpr", name, time.Now().Format("20060102_150405"))
	}
	cacheDir, err := agentutils.FindOrCreateCacheDir()
	if err != nil {
		panic(fmt.Sprintf("find cache directory: %v", err))
	}
	base := filepath.Base(output)
	return filepath.Join(cacheDir, "replays", version, base)
}

func (f *Framework) spawnAgentInternal(ctx context.Context, inst *TestInstance, cfg AgentConfig, addToInstance bool) (*ManagedAgent, error) {
	// Setup agent logging (redirects log package to file + stdout)
	// This is done once globally for all agents
	if err := f.setupAgentLogging(); err != nil {
		return nil, fmt.Errorf("setup agent logging: %w", err)
	}

	// Get actual protocol version from server via version negotiation
	mcVersion, protocolVersion, err := utils.CheckServerVersion(cfg.ServerAddress, 0)
	if err != nil {
		return nil, fmt.Errorf("check server version: %w", err)
	}

	if mcVersion != cfg.Version {
		log.Printf("[WARN][%s] Server version %s differs from agent config version %s", cfg.Name, mcVersion, cfg.Version)
	}

	cfg.EnableReplay = true
	cfg.ReplayOutput = normalizeReplayOutput(mcVersion, cfg.ReplayOutput, cfg.Name)
	if err := os.MkdirAll(filepath.Dir(cfg.ReplayOutput), 0755); err != nil {
		return nil, fmt.Errorf("create replay directory: %w", err)
	}
	log.Printf("[%s] Replay enabled: %s", cfg.Name, cfg.ReplayOutput)

	// Get packet manager for version
	packetMgr := mc_versions.GetPacketMgrForVersion(mcVersion)
	if packetMgr == nil {
		return nil, fmt.Errorf("no packet manager found for version %s", mcVersion)
	}

	// Setup packet logging (raw packet captures for debugging)
	cacheDir, err := agentutils.FindOrCreateCacheDir()
	if err != nil {
		return nil, fmt.Errorf("find cache directory: %w", err)
	}
	packetLogsDir := filepath.Join(cacheDir, "logs", "packets")
	_ = os.MkdirAll(packetLogsDir, 0760)
	receiverLog := &lumberjack.Logger{
		Filename:   filepath.Join(packetLogsDir, fmt.Sprintf("%s_%s.log", cfg.Name, time.Now().Format("20060102_150405"))),
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     28, // days
		Compress:   true,
		LocalTime:  true,
	}

	// Get block manager for version
	blockMgr := mc_versions.GetBlockMgrForVersion(mcVersion)

	// Get sound manager for version
	soundMgr := mc_versions.GetSoundMgrForVersion(mcVersion)

	// Get version handler (either from config or auto-detect)
	var versionHandler models.VersionHandler
	if cfg.VersionHandler != nil {
		versionHandler = cfg.VersionHandler
		log.Printf("[%s] Using provided version handler for %s", cfg.Name, mcVersion)
	} else {
		// Auto-detect version handler (required)
		vh, err := common.GetVersionHandler(mcVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to get version handler for %s: %w", mcVersion, err)
		}
		if vh == nil {
			return nil, fmt.Errorf("no version handler available for %s (supported: %v)", mcVersion, common.SupportedVersions())
		}
		versionHandler = vh
		log.Printf("[%s] Auto-detected version handler for %s", cfg.Name, mcVersion)
	}

	// Create bot client (required for agent to actually connect)
	botClient := bot.NewClient(packetMgr)
	botClient.SetAuth(bot.Auth{
		Name:        cfg.Name,
		UUID:        "", // Offline mode - server generates UUID
		AccessToken: "", // Offline mode - no access token
	})

	// Create skin provider
	skinProvider := agent.NewSkinFetcher(agent.SkinFetcherConfig{
		AllowNetwork: cfg.SkinNetEnabled,
		CacheRoot:    cfg.SkinCacheDir,
		HTTPClient:   &http.Client{Timeout: 3 * time.Second},
	})

	// Set RegistriesPath with fallback to test download cache if not explicitly set
	registriesPath := cfg.RegistriesPath
	if registriesPath == "" {
		// Fallback for tests that don't explicitly set RegistriesPath
		downloadCacheDir, err := agentutils.FindOrCreateCacheDir()
		if err == nil {
			registriesPath = filepath.Join(downloadCacheDir, "downloads", mcVersion)
		}
	}

	// Build agent configuration
	agentCfg := models.AgentConfig{
		Name:              cfg.Name,
		Address:           cfg.ServerAddress,
		Version:           mcVersion,
		ProtocolVersion:   protocolVersion,
		Auth:              models.Auth{Name: cfg.Name, UUID: "", AccessToken: ""},
		PacketMgr:         packetMgr,
		BlockMgr:          blockMgr,
		SoundMgr:          soundMgr,
		VersionHandler:    versionHandler, // Enable version-specific packet handling
		Client:            botClient,      // CRITICAL: Must provide client!
		MCDataGenPath:     cfg.MCDataGenPath,
		MCProtocolGoPath:  cfg.MCProtocolGoPath,
		RegistriesPath:    registriesPath, // Path to registries.json
		EnableReplay:      cfg.EnableReplay,
		ReplayOutput:      cfg.ReplayOutput,
		SkinProvider:      skinProvider,
		RCON:              inst.RCON,   // Pass RCON for debug visualization
		LogWriter:         receiverLog, // Raw packet logging for debugging
		HPADebugPathBlock: cfg.HPADebugPathBlock,
		HPADebugPathColor: cfg.HPADebugPathColor,
	}

	// Create agent
	agent, err := agent.New(agentCfg)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	// Create managed agent with lifecycle context
	agentCtx, agentCancel := context.WithCancel(ctx)

	// Initialize agent (connects to server, goes through config phase, loads registries)
	// Config packets will overwrite any registries loaded from file above
	if err := agent.Init(agentCtx); err != nil {
		agentCancel()
		return nil, fmt.Errorf("init agent: %w", err)
	}

	// Wire up minimal test adapters (subsystems are created automatically in agent.Init)
	wireAgentSubsystems(agent)

	managed := &ManagedAgent{
		Name:      cfg.Name,
		Agent:     agent,
		Config:    agentCfg,
		ctx:       agentCtx,
		cancel:    agentCancel,
		done:      make(chan struct{}),
		botClient: botClient,
	}

	// Start agent
	if err := agent.Start(agentCtx); err != nil {
		agentCancel()
		return nil, fmt.Errorf("start agent: %w", err)
	}

	// Start monitoring goroutine
	go func() {
		defer close(managed.done)
		select {
		case <-agentCtx.Done():
			// Agent stopped via context cancellation
		case <-agent.Done():
			// Agent stopped internally
		}
	}()

	// Add to test instance
	if addToInstance {
		inst.mu.Lock()
		inst.Agents = append(inst.Agents, managed)
		inst.mu.Unlock()
	}

	return managed, nil
}

// Stop gracefully stops a managed agent.
func (ma *ManagedAgent) Stop(ctx context.Context) error {
	ma.mu.Lock()
	if ma.cancel != nil {
		ma.cancel()
	}
	ma.mu.Unlock()

	// Wait for agent to finish with timeout
	select {
	case <-ma.done:
		// Agent stopped gracefully
	case <-ctx.Done():
		// Timeout waiting for graceful shutdown, but continue to close
	}

	// CRITICAL: Always call Close() to finalize replay recordings and cleanup resources
	// Use a fresh context with timeout to ensure Close() completes even if ctx is done
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer closeCancel()

	if err := ma.Agent.Close(closeCtx); err != nil {
		fmt.Printf("WARNING: Agent %s failed to close cleanly: %v\n", ma.Name, err)
		return err
	}

	if ma.Cam != nil {
		camCtx, camCancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := ma.Cam.Stop(camCtx); err != nil {
			fmt.Printf("WARNING: Cam agent %s failed to stop cleanly: %v\n", ma.Cam.Name, err)
		}
		camCancel()
	}
	return nil
}

// PositionTracker monitors agent positions using RCON queries.
type PositionTracker struct {
	inst         *TestInstance
	pollInterval time.Duration
	mu           sync.RWMutex
	positions    map[string]models.V3   // keyed by agent name
	history      map[string][]models.V3 // position history for each agent
	stopCh       chan struct{}
	doneCh       chan struct{}
}

// NewPositionTracker creates a position tracker with the specified poll interval.
func NewPositionTracker(inst *TestInstance, pollInterval time.Duration) *PositionTracker {
	if pollInterval == 0 {
		pollInterval = 500 * time.Millisecond
	}
	return &PositionTracker{
		inst:         inst,
		pollInterval: pollInterval,
		positions:    make(map[string]models.V3),
		history:      make(map[string][]models.V3),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Start begins tracking agent positions.
func (pt *PositionTracker) Start(ctx context.Context) *PositionTracker {
	go pt.track(ctx)
	return pt
}

// Stop halts position tracking.
func (pt *PositionTracker) Stop() {
	close(pt.stopCh)
	<-pt.doneCh
}

// track is the main tracking loop.
func (pt *PositionTracker) track(ctx context.Context) {
	defer close(pt.doneCh)
	ticker := time.NewTicker(pt.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pt.stopCh:
			return
		case <-ticker.C:
			pt.updatePositions()
		}
	}
}

// updatePositions queries RCON for all agent positions.
// Uses a short timeout for RCON queries to prevent the tracker from hanging
// if RCON becomes unresponsive.
func (pt *PositionTracker) updatePositions() {
	pt.inst.mu.RLock()
	agents := make([]*ManagedAgent, len(pt.inst.Agents))
	copy(agents, pt.inst.Agents)
	pt.inst.mu.RUnlock()

	// Use a 5-second timeout per RCON query to prevent indefinite hanging
	rconCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, agent := range agents {
		x, y, z, err := pt.inst.RCON.GetEntityPos(rconCtx, agent.Name)
		if err != nil {
			// Agent may not be in world yet, RCON timeout, or other error; skip
			continue
		}

		pos := models.V3{X: x, Y: y, Z: z}
		pt.mu.Lock()
		pt.positions[agent.Name] = pos
		pt.history[agent.Name] = append(pt.history[agent.Name], pos)
		pt.mu.Unlock()
	}
}

// GetPosition returns the last known position of an agent.
func (pt *PositionTracker) GetPosition(name string) (models.V3, bool) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	pos, ok := pt.positions[name]
	return pos, ok
}

// WaitForPosition waits until an agent reaches a target position within tolerance.
// Uses an isolated timeout to prevent cascading from parent context deadlines,
// while still respecting parent context cancellation.
func (pt *PositionTracker) WaitForPosition(ctx context.Context, name string, target models.V3, tolerance float64, timeout time.Duration) error {
	startTime := time.Now()
	fmt.Printf("[WaitForPosition] START: %s, timeout=%v, deadline=%v\n", name, timeout, startTime.Add(timeout))

	// Create an isolated timeout context so WaitForPosition's timeout is independent
	// of the parent context's deadline (which might be much longer, like 15 minutes).
	timeoutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(pt.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			fmt.Printf("[WaitForPosition] TIMEOUT: %s after %v\n", name, time.Since(startTime))
			return fmt.Errorf("PositionTracker: timeout waiting for %s to reach position", name)
		case <-ctx.Done():
			// Parent context cancelled or timed out - exit immediately
			fmt.Printf("[WaitForPosition] PARENT_CANCELLED: %s after %v\n", name, time.Since(startTime))
			return fmt.Errorf("PositionTracker: parent context cancelled while waiting for %s", name)
		case <-ticker.C:
			pos, ok := pt.GetPosition(name)
			if !ok {
				fmt.Printf("[PositionTracker] No position for %s yet (elapsed: %v)\n", name, time.Since(startTime))
				continue
			}
			dist := pos.DistanceTo(target)
			fmt.Printf("[PositionTracker] %s at %.2f, %.2f, %.2f (dist=%.2f, tolerance=%.2f, elapsed=%v)\n",
				name, pos.X, pos.Y, pos.Z, dist, tolerance, time.Since(startTime))
			if dist <= tolerance {
				fmt.Printf("[PositionTracker] REACHED: %s after %v\n", name, time.Since(startTime))
				return nil
			}
		}
	}
}

// MovementSpeed represents typical Minecraft movement speeds (blocks/second).
const (
	WalkingSpeed   = 4.317 // blocks/second
	SprintingSpeed = 5.612 // blocks/second
	FlyingSpeed    = 10.92 // blocks/second (creative)
)

// CalculateMovementTimeout calculates a reasonable timeout for movement.
// It uses walking speed + 50% buffer for pathfinding overhead.
func CalculateMovementTimeout(distance float64) time.Duration {
	baseTime := distance / WalkingSpeed
	bufferedTime := baseTime * 1.5 // 50% buffer
	return time.Duration(bufferedTime * float64(time.Second))
}

// GetPositionHistory returns the position history for an agent.
func (pt *PositionTracker) GetPositionHistory(name string) []models.V3 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	history := pt.history[name]
	// Return a copy to prevent external modification
	result := make([]models.V3, len(history))
	copy(result, history)
	return result
}

// AnalyzeMovementProgress analyzes whether an agent made meaningful progress toward a target.
// Returns:
// - totalDistance: total distance traveled
// - progressToward: net progress toward target (can be negative if moved away)
// - madeProgress: true if agent moved meaningfully toward target
func (pt *PositionTracker) AnalyzeMovementProgress(name string, start, target models.V3, minProgress float64) (totalDistance, progressToward float64, madeProgress bool) {
	history := pt.GetPositionHistory(name)
	if len(history) == 0 {
		return 0, 0, false
	}

	// Calculate total distance traveled
	for i := 1; i < len(history); i++ {
		totalDistance += history[i-1].DistanceTo(history[i])
	}

	// Calculate net progress toward target
	startDist := start.DistanceTo(target)
	finalPos := history[len(history)-1]
	finalDist := finalPos.DistanceTo(target)
	progressToward = startDist - finalDist

	// Agent made progress if it moved at least minProgress blocks toward target
	madeProgress = progressToward >= minProgress
	return totalDistance, progressToward, madeProgress
}

// captureServerLogs captures server logs to a file for offline review.
func (f *Framework) captureServerLogs(ctx context.Context, inst *testenv.Instance) error {
	if inst == nil {
		return nil
	}

	// Get centralized cache directory
	cacheDir, err := agentutils.FindOrCreateCacheDir()
	if err != nil {
		return fmt.Errorf("find cache directory: %w", err)
	}

	// Create logs directory if it doesn't exist
	logsDir := filepath.Join(cacheDir, "logs", "servers")
	if err := ensureDir(logsDir); err != nil {
		return fmt.Errorf("create logs directory: %w", err)
	}

	// Generate log filename with timestamp and container name
	logFile := fmt.Sprintf("%s/%s_%s.log", logsDir, inst.Name, time.Now().Format("20060102_150405"))

	// Fetch container logs (last 1000 lines)
	opts := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
		Timestamps: true,
		// Tail:       "1000",
	}

	containerReader, err := f.serverMgr.Logs(ctx, inst.ID, opts)
	if err != nil {
		return fmt.Errorf("fetch logs: %w", err)
	}
	defer containerReader.Close()

	// containerReader is a multiplexed stream, we need to seperate and remerge the stream to remove multiplexing metadata
	var reader io.Reader
	mergedStdErr := &bytes.Buffer{}

	_, err = stdcopy.StdCopy(mergedStdErr, mergedStdErr, containerReader)
	if err != nil {
		reader = containerReader
	} else {
		reader = mergedStdErr
	}

	// Write logs to file
	outFile, err := createFile(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer outFile.Close()

	if _, err := copyWithTimeout(outFile, reader, 10*time.Second); err != nil {
		return fmt.Errorf("write logs: %w", err)
	}

	absLogFile, _ := filepath.Abs(logFile)
	fmt.Printf("Server logs saved to: %s (%s)\n", logFile, absLogFile)
	return nil
}

// captureAgentOutput captures agent output to a file for offline review.
// func (f *Framework) captureAgentOutput(agent *ManagedAgent, output string) error {
// 	if agent == nil || output == "" {
// 		return nil
// 	}

// 	// Create logs directory if it doesn't exist
// 	logsDir := "./logs/agents"
// 	if err := ensureDir(logsDir); err != nil {
// 		return fmt.Errorf("create logs directory: %w", err)
// 	}

// 	// Generate log filename with timestamp and agent name
// 	logFile := fmt.Sprintf("%s/%s_%s.log", logsDir, agent.Name, time.Now().Format("20060102_150405"))

// 	// Write output to file
// 	outFile, err := createFile(logFile)
// 	if err != nil {
// 		return fmt.Errorf("create log file: %w", err)
// 	}
// 	defer outFile.Close()

// 	if _, err := outFile.WriteString(output); err != nil {
// 		return fmt.Errorf("write output: %w", err)
// 	}

// 	fmt.Printf("Agent output saved to: %s\n", logFile)
// 	return nil
// }

// Helper functions

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func createFile(path string) (*os.File, error) {
	return os.Create(path)
}

func copyWithTimeout(dst io.Writer, src io.Reader, timeout time.Duration) (int64, error) {
	type result struct {
		n   int64
		err error
	}
	ch := make(chan result, 1)
	go func() {
		// use our own copy to filter out control characters (for some reason the server logs have a bunch of control characters in them)
		n, err := copyFileStreaming(dst, src)
		ch <- result{n, err}
	}()

	select {
	case res := <-ch:
		return res.n, res.err
	case <-time.After(timeout):
		return 0, fmt.Errorf("copy timeout after %v", timeout)
	}
}

// cleanControlChars filters out all non-printable Unicode characters from a string.
func cleanControlChars(input string) string {
	return strings.Map(func(r rune) rune {
		// unicode.IsPrint returns true if the rune is a printable character
		// (letters, numbers, punctuation, symbols, and spaces).
		if unicode.IsPrint(r) {
			return r
		}

		// replace with a space.  We could also return -1 to drop the rune completely
		return ' '
	}, input)
}

func copyFileStreaming(dst io.Writer, src io.Reader) (int64, error) {
	var n int64

	writer := bufio.NewWriter(dst)
	defer writer.Flush() // Ensure all buffered writes are committed

	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		line := scanner.Text()
		cleanedLine := cleanControlChars(line)
		nn, err := writer.WriteString(cleanedLine + "\n") // Add newline back if needed
		if err != nil {
			return n, fmt.Errorf("failed to write line to destination file: %w", err)
		}
		n += int64(nn)
	}

	if err := scanner.Err(); err != nil {
		return n, fmt.Errorf("error reading from source file: %w", err)
	}

	return n, nil
}

// Memory checking functions

// checkMemoryAvailability verifies sufficient memory is available before starting a server.
// Retries multiple times with delays to allow memory to be freed up.
func (f *Framework) checkMemoryAvailability(cfg ServerConfig) error {
	// Calculate estimated memory needed
	estimatedMB := cfg.EstimatedMemoryMB
	if estimatedMB == 0 {
		estimatedMB = parseMemoryString(cfg.Memory)
		if estimatedMB == 0 {
			estimatedMB = 1536 // Default estimate: 1.5GB (1G heap + overhead)
		}
	}

	// Determine minimum free memory requirement
	minFreeMB := cfg.MinFreeMemoryMB
	if minFreeMB == 0 {
		minFreeMB = 2048 // Default: 2GB minimum
	}

	// Calculate total required (estimated + minimum free as buffer)
	totalRequiredMB := estimatedMB + minFreeMB

	// Determine retry settings
	retries := cfg.MemoryCheckRetries
	if retries == 0 {
		retries = 3 // Default: 3 retries
	}
	retryWait := cfg.MemoryCheckRetryWait
	if retryWait == 0 {
		retryWait = 5 * time.Second // Default: 5 seconds
	}

	// Try multiple times to check for available memory
	for attempt := 1; attempt <= retries; attempt++ {
		// Get available memory
		availableMB, err := getAvailableMemoryMB()
		if err != nil {
			fmt.Printf("WARNING: Failed to check memory availability: %v\n", err)
			// Don't fail - continue but warn
			return nil
		}

		// Check if sufficient memory available
		if availableMB >= totalRequiredMB {
			// Success!
			fmt.Printf("Memory check: %d MB available, %d MB required (server: %d MB + buffer: %d MB) - OK\n",
				availableMB, totalRequiredMB, estimatedMB, minFreeMB)
			return nil
		}

		// Insufficient memory
		if attempt < retries {
			// Not the last attempt - wait and retry
			fmt.Printf("Memory check (attempt %d/%d): %d MB available, %d MB required - waiting %v for memory to free up...\n",
				attempt, retries, availableMB, totalRequiredMB, retryWait)
			time.Sleep(retryWait)
		} else {
			// Last attempt failed - return error with memory report
			memReport := getMemoryUsageReport()
			return fmt.Errorf(
				"insufficient memory: need %d MB (server: %d MB + buffer: %d MB), have %d MB available\n"+
					"Tried %d times over %v.\n"+
					"Suggestions:\n"+
					"  - Close other applications to free memory\n"+
					"  - Reduce server memory (cfg.Memory)\n"+
					"  - Wait for other containers to finish\n"+
					"  - Run tests sequentially instead of parallel\n"+
					"%s",
				totalRequiredMB, estimatedMB, minFreeMB, availableMB,
				retries, time.Duration(retries-1)*retryWait,
				memReport,
			)
		}
	}

	return nil
}

// getAvailableMemoryMB returns available system memory in megabytes.
func getAvailableMemoryMB() (int, error) {
	// Read /proc/meminfo
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer file.Close()

	var memAvailableKB int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memAvailableKB, err = strconv.Atoi(fields[1])
				if err != nil {
					return 0, fmt.Errorf("parse MemAvailable: %w", err)
				}
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read /proc/meminfo: %w", err)
	}

	if memAvailableKB == 0 {
		return 0, fmt.Errorf("MemAvailable not found in /proc/meminfo")
	}

	// Convert KB to MB
	return memAvailableKB / 1024, nil
}

// parseMemoryString parses memory strings like "1G", "2G", "512M" into MB.
func parseMemoryString(mem string) int {
	if mem == "" {
		return 0
	}

	mem = strings.ToUpper(strings.TrimSpace(mem))

	// Extract number and unit
	var value int
	var unit string

	if strings.HasSuffix(mem, "G") {
		value, _ = strconv.Atoi(strings.TrimSuffix(mem, "G"))
		unit = "G"
	} else if strings.HasSuffix(mem, "M") {
		value, _ = strconv.Atoi(strings.TrimSuffix(mem, "M"))
		unit = "M"
	} else {
		// Try to parse as number (assume MB)
		value, _ = strconv.Atoi(mem)
		unit = "M"
	}

	// Convert to MB and add overhead estimate
	switch unit {
	case "G":
		// Java heap + container overhead ~= heap * 1.5
		return value * 1024 * 3 / 2
	case "M":
		// Add 50% overhead
		return value * 3 / 2
	default:
		return value
	}
}

// getMemoryUsageReport returns a report of top memory consumers.
func getMemoryUsageReport() string {
	var report strings.Builder
	report.WriteString("\n=== Memory Usage Report ===\n")

	// Get Docker container memory usage
	cmd := exec.Command("docker", "stats", "--no-stream", "--format", "table {{.Name}}\t{{.MemUsage}}")
	output, err := cmd.CombinedOutput()
	if err == nil && len(output) > 0 {
		report.WriteString("\nDocker Containers:\n")
		report.WriteString(string(output))
	}

	// Get top memory-consuming processes
	cmd = exec.Command("ps", "aux", "--sort=-%mem")
	output, err = cmd.CombinedOutput()
	if err == nil && len(output) > 0 {
		lines := strings.Split(string(output), "\n")
		report.WriteString("\nTop Memory-Consuming Processes:\n")
		// Show header + top 10 processes
		maxLines := 11
		if len(lines) < maxLines {
			maxLines = len(lines)
		}
		for i := 0; i < maxLines; i++ {
			if lines[i] != "" {
				report.WriteString(lines[i] + "\n")
			}
		}
	}

	return report.String()
}
