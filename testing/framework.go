package testing

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"
	"github.com/reallyoldfogie/mc-agent/agent"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/utils"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
)

// Framework orchestrates integration tests for Minecraft agents.
// It manages server lifecycle, agent spawning, and test execution.
type Framework struct {
	serverMgr      testenv.Manager
	instances      map[string]*TestInstance // keyed by test name
	agentLogFile   *os.File                 // Log file for all agents
	agentLogWriter io.Writer                // MultiWriter for agents (file + stdout)
	logSetupMu     sync.Mutex               // Mutex for log setup
}

// TestInstance represents a complete test environment with server and agents.
type TestInstance struct {
	Server *testenv.Instance
	RCON   testenv.RCONHelper
	Agents []*ManagedAgent
	mu     sync.RWMutex
}

// ManagedAgent wraps an agent instance with lifecycle tracking.
type ManagedAgent struct {
	Name   string
	Agent  agent.Agent
	Config agent.Config
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
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

// WorldGenType specifies the terrain generation mode for tests.
type WorldGenType string

const (
	WorldGenRandom     WorldGenType = "default"    // Random terrain
	WorldGenFlat       WorldGenType = "flat"       // Flat world
	WorldGenControlled WorldGenType = "controlled" // Flat with programmatic obstacles
)

// ServerConfig contains configuration for a test server instance.
type ServerConfig struct {
	Version              string
	Difficulty           string       // peaceful, easy, normal, hard
	GameMode             string       // creative, survival, adventure, spectator
	WorldGen             WorldGenType // World generation type (default: random)
	PullImage            bool
	Memory               string // e.g. "1G", "2G" - Java heap size (default: 1G for tests)
	CacheDir             string // Cache directory for server JARs (default: ~/.cache/mc-agent-test)
	ExtraEnv             map[string]string
	StartTimeout         time.Duration // default 10 minutes
	SkipMemoryCheck      bool          // Skip memory availability check (not recommended)
	MinFreeMemoryMB      int           // Minimum free memory required in MB (default: 2048)
	EstimatedMemoryMB    int           // Estimated memory for this server in MB (auto-calculated from Memory field)
	MemoryCheckRetries   int           // Number of times to retry memory check (default: 3)
	MemoryCheckRetryWait time.Duration // Wait between retries (default: 5 seconds)
}

// DefaultServerConfig returns a sensible default configuration for tests.
// Uses reduced memory (1G) to prevent OOM on systems with limited RAM.
// Uses random terrain for full integration testing.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Version:              "1.21.5",
		Difficulty:           "peaceful",
		GameMode:             "creative",
		WorldGen:             WorldGenRandom, // Random terrain for realistic testing
		PullImage:            false,          // set to true to pull latest image
		Memory:               "1G",           // Reduced from default 2G to prevent OOM
		StartTimeout:         10 * time.Minute,
		SkipMemoryCheck:      false,           // Always check memory availability
		MinFreeMemoryMB:      1024,            //2048,           // Require 2GB free memory minimum
		EstimatedMemoryMB:    0,               // Auto-calculated from Memory field
		MemoryCheckRetries:   3,               // Retry 3 times before failing
		MemoryCheckRetryWait: 5 * time.Second, // Wait 5 seconds between retries
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
	if cfg.Difficulty != "" {
		extraEnv["DIFFICULTY"] = cfg.Difficulty
	}
	if cfg.GameMode != "" {
		extraEnv["MODE"] = cfg.GameMode
	}
	// Apply flat-world configuration if requested
	if cfg.WorldGen == WorldGenFlat || cfg.WorldGen == WorldGenControlled {
		// Use the flat world preset: "minecraft:flat"
		extraEnv["LEVEL_TYPE"] = "flat"
		// Flat world preset: grass block at y=64 for testing
		extraEnv["GENERATOR_SETTINGS"] = "{\"layers\":[{\"block\":\"minecraft:grass_block\",\"height\":1},{\"block\":\"minecraft:dirt\",\"height\":3},{\"block\":\"minecraft:stone\",\"height\":60}],\"biome\":\"minecraft:plains\"}"
	}
	// Set memory limit to reduce OOM risk
	if cfg.Memory != "" {
		extraEnv["MEMORY"] = cfg.Memory
	}
	// Optimize JVM for container environments
	extraEnv["JVM_XX_OPTS"] = "-XX:+UseContainerSupport -XX:MaxRAMPercentage=80.0"

	// Set up cache directory for server JARs to avoid repeated downloads
	// Use version-specific directory so the container can reuse downloaded JARs
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			// Use version-specific subdirectory - the container will use this as /data
			// and will find any existing minecraft_server.{VERSION}.jar
			cacheDir = filepath.Join(homeDir, ".cache", "mc-agent-test", cfg.Version)
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
		DataDir:    cacheDir, // Use persistent cache for server JARs
		ExtraEnv:   extraEnv,
	}

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

	testInst := &TestInstance{
		Server: inst,
		RCON:   helper,
		Agents: []*ManagedAgent{},
	}

	return testInst, nil
}

// StopServer stops a test server and cleans up resources.
// Captures server logs before stopping for offline review.
func (f *Framework) StopServer(ctx context.Context, inst *TestInstance, remove bool) error {
	if inst == nil {
		return nil
	}

	// Stop all agents first (CRITICAL: must close before server to finalize replays)
	inst.mu.Lock()
	agents := make([]*ManagedAgent, len(inst.Agents))
	copy(agents, inst.Agents)
	inst.mu.Unlock()

	for _, agent := range agents {
		// Use short timeout for agent stop to prevent hanging
		agentCtx, agentCancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := agent.Stop(agentCtx); err != nil {
			fmt.Printf("WARNING: Agent %s failed to stop cleanly: %v\n", agent.Name, err)
		}
		agentCancel()
	}

	// Small delay to ensure agents are fully cleaned up
	time.Sleep(500 * time.Millisecond)

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
				return fmt.Errorf("stop server: %w", err)
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

	// Create logs directory
	logsDir := "./logs/agents"
	if err := ensureDir(logsDir); err != nil {
		return fmt.Errorf("create logs directory: %w", err)
	}

	// Create log file with timestamp
	logFile := fmt.Sprintf("%s/agents_%s.log", logsDir, time.Now().Format("20060102_150405"))
	file, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}

	// Create MultiWriter to write to both file and stdout
	multiWriter := io.MultiWriter(file, os.Stdout)

	// Set global log output
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	// Store for cleanup
	f.agentLogFile = file
	f.agentLogWriter = multiWriter

	fmt.Printf("Agent logging enabled: %s (also to console)\n", logFile)
	return nil
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
	EnablePathfinding bool
	EnableFollowing   bool
	EnableReplay      bool
	ReplayOutput      string
	SkinCacheDir      string
	SkinNetEnabled    bool
}

// DefaultAgentConfig returns a sensible default configuration for test agents.
func DefaultAgentConfig(name, serverAddress, version string) AgentConfig {
	return AgentConfig{
		Name:              name,
		ServerAddress:     serverAddress,
		Version:           version,
		MCDataGenPath:     "", // Empty = use default (downloads if needed)
		MCProtocolGoPath:  "", // Not needed for tests
		EnablePathfinding: true,
		EnableFollowing:   true,
		EnableReplay:      false,
		SkinCacheDir:      "skins",
		SkinNetEnabled:    false,
	}
}

// SpawnAgent creates and starts a new agent connected to the test server.
func (f *Framework) SpawnAgent(ctx context.Context, inst *TestInstance, cfg AgentConfig) (*ManagedAgent, error) {
	// Setup agent logging (redirects log package to file + stdout)
	// This is done once globally for all agents
	if err := f.setupAgentLogging(); err != nil {
		return nil, fmt.Errorf("setup agent logging: %w", err)
	}

	// Get packet manager for version
	packetMgr := mc_versions.GetPacketMgrForVersion(cfg.Version)
	if packetMgr == nil {
		return nil, fmt.Errorf("no packet manager found for version %s", cfg.Version)
	}

	// Get block manager for version
	blockMgr := mc_versions.GetBlockMgrForVersion(cfg.Version)

	// Get sound manager for version
	soundMgr := mc_versions.GetSoundMgrForVersion(cfg.Version)

	// Create bot client (required for agent to actually connect)
	botClient := bot.NewClient(packetMgr)
	botClient.Auth = bot.Auth{
		Name: cfg.Name,
		UUID: "", // Offline mode - server generates UUID
		AsTk: "", // Offline mode - no access token
	}

	// Create skin provider
	skinProvider := agent.NewSkinFetcher(agent.SkinFetcherConfig{
		AllowNetwork: cfg.SkinNetEnabled,
		CacheRoot:    cfg.SkinCacheDir,
		HTTPClient:   &http.Client{Timeout: 3 * time.Second},
	})

	// Get actual protocol version from server via version negotiation
	_, protocolVersion, err := utils.CheckServerVersion(cfg.ServerAddress, 0)
	if err != nil {
		return nil, fmt.Errorf("check server version: %w", err)
	}

	// Build agent configuration
	agentCfg := agent.Config{
		Address:           cfg.ServerAddress,
		Version:           cfg.Version,
		ProtocolVersion:   protocolVersion,
		Auth:              agent.Auth{Name: cfg.Name, UUID: "", AsTk: ""},
		PacketMgr:         packetMgr,
		BlockMgr:          blockMgr,
		SoundMgr:          soundMgr,
		Client:            agent.NewClientFromBot(botClient), // CRITICAL: Must provide client!
		MCDataGenPath:     cfg.MCDataGenPath,
		MCProtocolGoPath:  cfg.MCProtocolGoPath,
		EnablePathfinding: cfg.EnablePathfinding,
		EnableFollowing:   cfg.EnableFollowing,
		EnableReplay:      cfg.EnableReplay,
		ReplayOutput:      cfg.ReplayOutput,
		SkinProvider:      skinProvider,
	}

	// Create agent
	agent, err := agent.New(agentCfg)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	// Create managed agent with lifecycle context
	agentCtx, agentCancel := context.WithCancel(ctx)
	managed := &ManagedAgent{
		Name:   cfg.Name,
		Agent:  agent,
		Config: agentCfg,
		ctx:    agentCtx,
		cancel: agentCancel,
		done:   make(chan struct{}),
	}

	// Initialize agent
	if err := agent.Init(agentCtx); err != nil {
		agentCancel()
		return nil, fmt.Errorf("init agent: %w", err)
	}

	// Wire up subsystems (required for agents to function properly)
	if err := wireAgentSubsystems(agent, botClient, packetMgr, blockMgr, cfg); err != nil {
		agentCancel()
		return nil, fmt.Errorf("wire subsystems: %w", err)
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
	inst.mu.Lock()
	inst.Agents = append(inst.Agents, managed)
	inst.mu.Unlock()

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
	return nil
}

// Position represents a 3D position in the Minecraft world.
type Position struct {
	X, Y, Z float64
}

// Distance calculates Euclidean distance between two positions.
func (p Position) Distance(other Position) float64 {
	dx := p.X - other.X
	dy := p.Y - other.Y
	dz := p.Z - other.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// PositionTracker monitors agent positions using RCON queries.
type PositionTracker struct {
	inst         *TestInstance
	pollInterval time.Duration
	mu           sync.RWMutex
	positions    map[string]Position   // keyed by agent name
	history      map[string][]Position // position history for each agent
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
		positions:    make(map[string]Position),
		history:      make(map[string][]Position),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Start begins tracking agent positions.
func (pt *PositionTracker) Start(ctx context.Context) {
	go pt.track(ctx)
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
			pt.updatePositions(ctx)
		}
	}
}

// updatePositions queries RCON for all agent positions.
func (pt *PositionTracker) updatePositions(ctx context.Context) {
	pt.inst.mu.RLock()
	agents := make([]*ManagedAgent, len(pt.inst.Agents))
	copy(agents, pt.inst.Agents)
	pt.inst.mu.RUnlock()

	for _, agent := range agents {
		x, y, z, err := pt.inst.RCON.GetEntityPos(ctx, agent.Name)
		if err != nil {
			// Agent may not be in world yet; skip
			continue
		}

		pos := Position{X: x, Y: y, Z: z}
		pt.mu.Lock()
		pt.positions[agent.Name] = pos
		pt.history[agent.Name] = append(pt.history[agent.Name], pos)
		pt.mu.Unlock()
	}
}

// GetPosition returns the last known position of an agent.
func (pt *PositionTracker) GetPosition(name string) (Position, bool) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	pos, ok := pt.positions[name]
	return pos, ok
}

// WaitForPosition waits until an agent reaches a target position within tolerance.
func (pt *PositionTracker) WaitForPosition(ctx context.Context, name string, target Position, tolerance float64, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pt.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s to reach position", name)
		case <-ticker.C:
			pos, ok := pt.GetPosition(name)
			if !ok {
				continue
			}
			if pos.Distance(target) <= tolerance {
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
func (pt *PositionTracker) GetPositionHistory(name string) []Position {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	history := pt.history[name]
	// Return a copy to prevent external modification
	result := make([]Position, len(history))
	copy(result, history)
	return result
}

// AnalyzeMovementProgress analyzes whether an agent made meaningful progress toward a target.
// Returns:
// - totalDistance: total distance traveled
// - progressToward: net progress toward target (can be negative if moved away)
// - madeProgress: true if agent moved meaningfully toward target
func (pt *PositionTracker) AnalyzeMovementProgress(name string, start, target Position, minProgress float64) (totalDistance, progressToward float64, madeProgress bool) {
	history := pt.GetPositionHistory(name)
	if len(history) == 0 {
		return 0, 0, false
	}

	// Calculate total distance traveled
	for i := 1; i < len(history); i++ {
		totalDistance += history[i-1].Distance(history[i])
	}

	// Calculate net progress toward target
	startDist := start.Distance(target)
	finalPos := history[len(history)-1]
	finalDist := finalPos.Distance(target)
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

	// Create logs directory if it doesn't exist
	logsDir := "./logs/servers"
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
		Tail:       "1000",
	}

	reader, err := f.serverMgr.Logs(ctx, inst.ID, opts)
	if err != nil {
		return fmt.Errorf("fetch logs: %w", err)
	}
	defer reader.Close()

	// Write logs to file
	outFile, err := createFile(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer outFile.Close()

	if _, err := copyWithTimeout(outFile, reader, 10*time.Second); err != nil {
		return fmt.Errorf("write logs: %w", err)
	}

	fmt.Printf("Server logs saved to: %s\n", logFile)
	return nil
}

// captureAgentOutput captures agent output to a file for offline review.
func (f *Framework) captureAgentOutput(agent *ManagedAgent, output string) error {
	if agent == nil || output == "" {
		return nil
	}

	// Create logs directory if it doesn't exist
	logsDir := "./logs/agents"
	if err := ensureDir(logsDir); err != nil {
		return fmt.Errorf("create logs directory: %w", err)
	}

	// Generate log filename with timestamp and agent name
	logFile := fmt.Sprintf("%s/%s_%s.log", logsDir, agent.Name, time.Now().Format("20060102_150405"))

	// Write output to file
	outFile, err := createFile(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer outFile.Close()

	if _, err := outFile.WriteString(output); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	fmt.Printf("Agent output saved to: %s\n", logFile)
	return nil
}

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
		n, err := io.Copy(dst, src)
		ch <- result{n, err}
	}()

	select {
	case res := <-ch:
		return res.n, res.err
	case <-time.After(timeout):
		return 0, fmt.Errorf("copy timeout after %v", timeout)
	}
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
