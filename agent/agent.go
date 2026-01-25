package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/agent/plan"

	"github.com/reallyoldfogie/mc-agent/following"
	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/movement"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/physics"
	agentutils "github.com/reallyoldfogie/mc-agent/utils"

	"github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/basic"
	"github.com/reallyoldfogie/mc-bot-go/bot/msg"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/reallyoldfogie/mc-bot-go/bot/world"

	"github.com/reallyoldfogie/mc-client-test-go/testenv"

	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	protocol_utils "github.com/reallyoldfogie/mc-protocol-go/utils"

	"github.com/reallyoldfogie/mc-replay-go/adapters"
	"github.com/reallyoldfogie/mc-replay-go/mcpr"
	"github.com/reallyoldfogie/mc-replay-go/mcpr/recorder"
)

// Entity cleanup tuning (can be adjusted later or surfaced via Config if needed).
const (
	EntityRemovalGracePeriod = 30 * time.Second
	EntityCleanupInterval    = 10 * time.Second
)

// rconAdapter wraps testenv.RCONHelper to implement pathfinding.RCONSummoner
type rconAdapter struct {
	rcon testenv.RCONHelper
}

func (r *rconAdapter) SummonEntity(ctx context.Context, x, y, z float64, entityType, nbtData string) {
	result := r.rcon.SummonEntity(ctx, x, y, z, entityType, nbtData)
	// Fire and forget - we don't care about the result for visualization
	resp, _ := result.Exec(ctx)
	log.Printf("%s => %s", result.Cmd, resp)
}

// agent is the top-level orchestrator of bot/client, state, and event wiring.
type agent struct {
	cfg Config

	// lifecycle
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// logging
	logw io.Writer

	// dependencies (to be filled in during Init)
	client    bot.Client
	packetMgr protocol_models.PacketMgr
	blockMgr  mc_versions.BlockMgr
	soundMgr  mc_versions.SoundMgr

	// internal state placeholders (expanded during migration)
	// tracking
	posMu            sync.RWMutex
	posX, posY, posZ float64
	posYaw, posPitch float32
	posInitialized   bool

	entIDMu sync.RWMutex
	entID   int32

	entitiesMu sync.RWMutex
	entities   map[int32]*trackedEntity

	// e.g., player, playerList, world, movement, pathfinding, registries, etc.
	// registries
	regMu      sync.RWMutex
	registries map[RegistryID]CustomRegistry

	// replay helpers
	moveMirror MovementMirror

	// core subsystems (created automatically in Init) - using interfaces for separation of concerns
	player     TeleportAccepter       // Player subsystem (teleportation)
	worldMgr   World                  // World manager (block queries, pathfinding)
	chatMgr    Chat                   // Chat manager (message sending)
	screenMgr  models.ScreenSubsystem // Screen manager (inventory/containers)
	playerList playerlist.PlayerList  // Player list (online player tracking)

	// optional subsystems (for dependency injection override)
	teleport   TeleportAccepter // override player if needed
	chat       Chat             // override chatMgr if needed
	moveExec   MovementExecutor
	pathfind   pathfinding.PathFinder
	shapeMgr   pathfinding.BlockShapeManager
	stateProps *pathfinding.StatePropertyLoader
	itemMgr    ItemManager
	itemUsage  *items.ItemUsage
	slots      SlotResolver

	// HPA* dynamic world updates
	hpaUpdateHandler *pathfinding.WorldUpdateHandler

	// container system
	containerHelper ContainerHelper

	// position heartbeat (continuous position packets at 20 TPS)
	posHeartbeatMu     sync.Mutex
	posHeartbeatActive bool
	posHeartbeatStop   chan struct{}

	// helpers

	// player name/uuid resolvers
	playerNameByUUID func([16]byte) (string, bool)
	playerUUIDByName func(string) ([16]byte, error)

	// behaviors
	followMgr      models.FollowManager
	targetSelector models.TargetSelector

	// tracking loop state (for legacy parity)
	trackMu          sync.Mutex
	trackActive      bool
	trackStop        chan struct{}
	lastNoPlayersMsg time.Time
	// trackWG          sync.WaitGroup

	// replay
	rec *recorder.Recorder

	// clutch assist
	lastClutchAction time.Time

	// plan runner
	planRunner *plan.Runner

	// command registry
	commandRegistry models.ActionRegistry[actions.CommandAgent]

	// chat events
	chatEvents chan string

	// recipes (last Update Recipes payload)
	recipesMu         sync.RWMutex
	lastUpdateRecipes *UpdateRecipesPayload

	heldSlotMu      sync.RWMutex
	heldSlot        int16
	heldSlotSet     bool
	heldSlotUpdates chan int16
}

// New constructs an agent with the provided configuration.
func New(cfg Config) (models.Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var name string
	if cfg.Client != nil {
		name = cfg.Client.Name()
	}
	// Ensure RegistriesPath is set and data is available
	// If not set, defaults to ~/.cache/mc-agent/registries/{version}/
	// Downloads and generates registries.json if needed (thread-safe)
	if cfg.RegistriesPath == "" {
		resolvedPath, err := agentutils.EnsureRegistriesPath("", cfg.Version)
		if err != nil {
			// Non-fatal: log warning and continue without registries
			// They can still be loaded manually or from testing framework paths
			log.Printf("[Agent %s] Warning: failed to ensure registries path: %v", name, err)
			log.Printf("[Agent %s] Registries will need to be loaded manually via LoadEntityTypesFromRegistry()", name)
		} else {
			cfg.RegistriesPath = resolvedPath
			log.Printf("[Agent %s] RegistriesPath auto-configured: %s", name, resolvedPath)
		}
	}

	a := &agent{cfg: cfg, logw: cfg.LogWriter, chatEvents: make(chan string, 64)}
	a.commandRegistry = actions.NewRegistry()
	a.planRunner = plan.NewRunner(a)
	return a, nil
}

// Init prepares dependencies, managers and event wiring but does not connect.
func (a *agent) Init(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		return ErrAlreadyInitialized
	}
	a.ctx, a.cancel = context.WithCancel(ctx)

	// Derive managers if not provided
	if a.packetMgr == nil {
		a.packetMgr = a.cfg.PacketMgr
	}
	if a.blockMgr == nil {
		a.blockMgr = a.cfg.BlockMgr
	}
	if a.soundMgr == nil {
		a.soundMgr = a.cfg.SoundMgr
	}

	// Use prebuilt client when provided.
	if a.client == nil && a.cfg.Client != nil {
		a.client = a.cfg.Client
	}

	// Subsystems
	if a.cfg.Chat != nil {
		a.chat = a.cfg.Chat
	}

	// Initialize maps for tracking.
	a.entitiesMu.Lock()
	if a.entities == nil {
		a.entities = make(map[int32]*trackedEntity)
	}
	a.entitiesMu.Unlock()

	// Create core subsystems automatically (Player, World, Chat, Screen)
	// These are created if client and packetMgr are available
	if a.client != nil && a.packetMgr != nil {
		// Type assert to get the underlying bot.Client
		// if botAdapter, ok := a.client.(*botClientAdapter); ok {
		// 	botClient := botAdapter.BotClient()
		botClient := a.client

		// Create Player subsystem (handles health, respawn, teleportation)
		// Use concrete type for constructors, store as interface for separation of concerns
		customSettings := basic.DefaultSettings
		customSettings.ViewDistance = 32
		customSettings.Locale = "en_us"

		playerConcrete := basic.NewPlayer(botClient, customSettings, basic.EventsListener{
			GameStart:    a.HandleGameStart,
			Disconnect:   a.HandleDisconnect,
			HealthChange: a.HandleHealthChange,
			Death:        a.HandleDeath,
			Teleported:   a.HandleTeleported,
		}, a.packetMgr)
		a.player = playerConcrete // Assign concrete type to interface field
		log.Printf("[Agent %s] Player subsystem initialized", a.client.Name())

		// Create PlayerList (for player tracking, needed by msg.New constructor)
		playerList := playerlist.New(botClient, a.packetMgr)
		a.playerList = playerList // Store as interface{}
		log.Printf("[Agent %s] PlayerList initialized", a.client.Name())
		if a.playerUUIDByName == nil {
			a.playerUUIDByName = func(name string) ([16]byte, error) {
				players := playerList.Get()
				log.Printf("[Agent %s] ResolvePlayerUUID: looking for %q in list with %d players", a.client.Name(), name, len(players))
				for id, info := range players {
					log.Printf("[Agent %s] ResolvePlayerUUID: checking player %q (UUID: %s)", a.client.Name(), info.Name, id.String())
					if info.Name == name {
						var out [16]byte
						copy(out[:], id[:])
						log.Printf("[Agent %s] ResolvePlayerUUID: found %q -> %s", a.client.Name(), name, id.String())
						return out, nil
					}
				}
				log.Printf("[Agent %s] ResolvePlayerUUID: player %q not found in list", a.client.Name(), name)
				return [16]byte{}, fmt.Errorf("player %s not found", name)
			}
		}
		if a.playerNameByUUID == nil {
			a.playerNameByUUID = func(u [16]byte) (string, bool) {
				for id, info := range playerList.Get() {
					var cu [16]byte
					copy(cu[:], id[:])
					if cu == u {
						return info.Name, true
					}
				}
				return "", false
			}
		}

		// Create Chat Manager (constructor requires concrete types)
		chatMgrConcrete := msg.New(botClient, playerConcrete, playerList, msg.EventsHandler{
			SystemChat:        a.OnSystemChat,
			PlayerChatMessage: a.OnPlayerChat,
			DisguisedChat:     a.OnDisguisedChat,
		}, a.packetMgr)
		a.chatMgr = chatMgrConcrete // Assign concrete type to interface field
		log.Printf("[Agent %s] Chat manager initialized", a.client.Name())

		// Create World Manager (for chunk management, constructor requires concrete Player)
		worldInterface := world.NewWorld(botClient, playerConcrete, world.EventsListener{
			LoadChunk: func(pos world.ChunkPos) error {
				return a.HandleChunkLoad(models.ChunkPos{X: pos.X, Z: pos.Z})
			},
			UnloadChunk: func(pos world.ChunkPos) error {
				return a.HandleChunkUnload(models.ChunkPos{X: pos.X, Z: pos.Z})
			},
		}, a.packetMgr)
		a.worldMgr = worldInterface

		// Create Screen Manager (for inventory/containers)
		a.screenMgr = screen.NewManager(botClient, containerEvents{agent: a}, a.packetMgr)
		log.Printf("[Agent %s] Screen manager initialized", a.client.Name())

		a.initHeldSlotTracking()

		var shapeMgr pathfinding.BlockShapeManager
		var stateProps *pathfinding.StatePropertyLoader
		dataBasePath, err := agentutils.ResolveDataPath(a.cfg.MCDataGenPath, filepath.Join("data", "mc-data-gen-cache"), "")
		if err == nil {
			// Use new unified Minecraft data cache for block properties
			dataCache := agentutils.NewMinecraftDataCache(a.cfg.Version)
			if err := dataCache.EnsureDataGenerated(); err == nil {
				blocksJSONPath, err := dataCache.GetBlocksJSONPath()
				if err == nil {
					if spl, err := pathfinding.NewStatePropertyLoader(blocksJSONPath); err == nil {
						stateProps = spl
						log.Printf("[Agent %s] Loaded block state properties from cache", a.client.Name())
					} else {
						log.Printf("[Agent %s] Warning: failed to load state properties: %v", a.client.Name(), err)
					}
				} else {
					log.Printf("[Agent %s] Warning: failed to get blocks.json path: %v", a.client.Name(), err)
				}
			} else {
				log.Printf("[Agent %s] Warning: failed to generate Minecraft data cache: %v", a.client.Name(), err)
			}
			shapeMgr, err = pathfinding.NewBlockShapeManager(a.cfg.Version, dataBasePath, a.blockMgr, stateProps)
			if err != nil {
				log.Printf("[Agent %s] Warning: failed to create block shape manager: %v", a.client.Name(), err)
			}
		} else {
			log.Printf("[Agent %s] Warning: failed to resolve data path: %v", a.client.Name(), err)
		}

		// Create movement executor (core component needed for container interactions, looking, movement, etc.)
		// Default to physics executor (realistic movement), fallback to interpolation if prerequisites missing NOTE: 1-13-26 - fallback disabled
		executorType := movement.PhysicsExecutor
		execConfig := movement.ExecutorConfig{
			Client:         botClient,
			PacketMgr:      a.packetMgr,
			GetBotPos:      a.GetPosition,
			SetBotPos:      a.UpdatePosition,
			GetBotEntityID: a.GetEntityID,
		}

		// Check if physics executor can be used (requires world manager, shape data, block manager)
		if a.worldMgr == nil {
			log.Printf("[Agent %s] Warning: world manager unavailable, falling back to interpolation executor", a.client.Name())
			// executorType = movement.InterpolationExecutor
			return fmt.Errorf("[Agent %s] Missing world manager", a.client.Name())
		} else if shapeMgr == nil {
			log.Printf("[Agent %s] Warning: block shape data unavailable, falling back to interpolation executor", a.client.Name())
			// executorType = movement.InterpolationExecutor
			return fmt.Errorf("[Agent %s] Missing shape manager", a.client.Name())
		} else if a.blockMgr == nil {
			log.Printf("[Agent %s] Warning: block manager unavailable, falling back to interpolation executor", a.client.Name())
			// executorType = movement.InterpolationExecutor
			return fmt.Errorf("[Agent %s] Missing block manager", a.client.Name())
		} else {
			// Physics executor can be used
			execConfig.World = movement.NewPhysicsWorldAdapter(a.worldMgr)
			execConfig.ShapeProvider = shapeMgr
		}

		// Allow explicit override to interpolation via config flag
		a.moveExec = movement.NewExecutor(executorType, execConfig)
		log.Printf("[Agent %s] Movement executor initialized (%s)", a.client.Name(), executorType.String())
		a.shapeMgr = shapeMgr
		a.stateProps = stateProps

		if a.cfg.EnableClutchAssist {
			if clutchSetter, ok := a.moveExec.(interface {
				SetClutchCallback(func(plan physics.ClutchPlan))
			}); ok {
				usage := items.NewItemUsage(
					botClient.Conn(),
					a.packetMgr,
				)
				clutchSetter.SetClutchCallback(func(plan physics.ClutchPlan) {
					log.Printf("[Clutch] plan=%s fall=%.2f ticks=%d place=(%.0f,%.0f,%.0f)",
						plan.Type, plan.FallDistance, plan.TicksToImpact,
						plan.PlacePos.X, plan.PlacePos.Y, plan.PlacePos.Z)
					if plan.Type != physics.ClutchWaterBucket && plan.Type != physics.ClutchPowderSnow {
						return
					}
					a.ensureClutchItemManager()
					if !a.ensureClutchSafety(time.Now()) {
						return
					}
					a.handleClutchPlan(usage, plan)
				})
			} else {
				log.Printf("[Agent %s] Warning: clutch assist enabled but movement executor does not support clutch callbacks", a.client.Name())
			}
		}

		// Create pathfinder if data paths are configured
		// Create low-level A* pathfinder
		lowLevelPathfinder := pathfinding.NewAStarPathFinderWithConfig(a.worldMgr, shapeMgr, pathfinding.PathfinderConfig{
			GoalRadius: a.cfg.PathfinderGoalRadius,
		})

		// Set up stuck recovery callback for physics executor
		// This enables automatic re-pathfinding when the agent gets stuck
		if stuckRecoverySetter, ok := a.moveExec.(interface {
			SetStuckRecoveryCallback(movement.StuckRecoveryFn)
		}); ok {
			// Create a closure that captures the pathfinder
			stuckRecoverySetter.SetStuckRecoveryCallback(func(currentPos models.V3, goalPos models.V3) *pathfinding.Path {
				// CRITICAL: Floor positions to block coordinates for pathfinding.
				// The physics executor provides fractional positions (e.g., Y=7.50 on stairs),
				// but A* pathfinder works with integer block coordinates. Without flooring,
				// the pathfinder generates moves at fractional Y levels which fail ground checks.
				snappedStart := models.V3{
					X: math.Floor(currentPos.X),
					Y: math.Floor(currentPos.Y),
					Z: math.Floor(currentPos.Z),
				}
				snappedGoal := models.V3{
					X: math.Floor(goalPos.X),
					Y: math.Floor(goalPos.Y),
					Z: math.Floor(goalPos.Z),
				}

				log.Printf("[Agent %s] Stuck recovery: re-pathfinding from (%.2f, %.2f, %.2f) -> snapped (%.0f, %.0f, %.0f) to (%.0f, %.0f, %.0f)",
					a.client.Name(), currentPos.X, currentPos.Y, currentPos.Z,
					snappedStart.X, snappedStart.Y, snappedStart.Z,
					snappedGoal.X, snappedGoal.Y, snappedGoal.Z)

				// Try to pathfind from snapped position to goal
				distance := snappedStart.DistanceTo(snappedGoal)
				maxSteps := max(int(distance*150), 10000)

				path, err := lowLevelPathfinder.FindPath(snappedStart, snappedGoal, maxSteps)
				if err != nil {
					log.Printf("[Agent %s] Stuck recovery pathfinding failed: %v", a.client.Name(), err)
					return nil
				}

				if !path.Found || len(path.Steps) == 0 {
					log.Printf("[Agent %s] Stuck recovery: no path found from current position", a.client.Name())
					return nil
				}

				log.Printf("[Agent %s] Stuck recovery: found path with %d steps", a.client.Name(), len(path.Steps))
				return path
			})
			log.Printf("[Agent %s] Stuck recovery callback configured", a.client.Name())
		}

		// Wrap with HPA* for hierarchical pathfinding on long distances
		// Larger cluster size = fewer clusters, faster building (but more entrances per cluster)
		// 32x32x32 aligns with Minecraft chunks (16x16) and is power-of-2 for CPU efficiency
		clusterSize := 32 // 32x32x32 blocks per cluster (2x2 chunks horizontally)
		hpaPathfinder := pathfinding.NewHPAPathFinder(a.worldMgr, shapeMgr, lowLevelPathfinder, clusterSize)
		if limiter, ok := hpaPathfinder.(interface {
			SetEntranceLimits(maxCount int, maxCost float64)
		}); ok {
			//limiter.SetEntranceLimits(4, 10)
			limiter.SetEntranceLimits(50, 2000) // Allow paths up to 2000 cost to entrances (handles complex vertical terrain)
		}
		a.pathfind = hpaPathfinder

		// Create update handler for dynamic world changes
		// Need to extract builder from HPA pathfinder
		if hpaPF, ok := hpaPathfinder.(interface {
			GetBuilder() *pathfinding.HPABuilder
		}); ok {
			a.hpaUpdateHandler = pathfinding.NewWorldUpdateHandler(hpaPF.GetBuilder())
			// Enable batch mode by default - it auto-flushes periodically
			// This handles both initial load and dynamic chunk loading
			a.hpaUpdateHandler.EnableBatchMode()

			// Set up debug visualization if RCON is available
			if a.cfg.RCON != nil {
				adapter := &rconAdapter{rcon: a.cfg.RCON}
				debugViz := pathfinding.NewHPADebugVisualizer(adapter, pathfinding.HPADebugVisualizerConfig{
					PathBlock: a.cfg.HPADebugPathBlock,
					PathColor: a.cfg.HPADebugPathColor,
				})
				// Set visualizer on both pathfinder and builder
				if setter, ok := hpaPathfinder.(interface {
					SetDebugVisualizer(*pathfinding.HPADebugVisualizer)
				}); ok {
					setter.SetDebugVisualizer(debugViz)
				}
				hpaPF.GetBuilder().DebugViz = debugViz
			}

			log.Printf("[Agent %s] HPA* PathFinder initialized with update handler (batch mode: auto-flush)", a.client.Name())
		} else {
			log.Printf("[Agent %s] PathFinder initialized (HPA* without update handler)", a.client.Name())
		}

		if a.pathfind != nil && a.moveExec != nil {
			if a.targetSelector == nil {
				a.targetSelector = following.NewTargetSelector(
					a.GetTrackedEntitiesForFollowing,
					a.ResolvePlayerUUIDByName,
					a.GetPositionSimple,
				)
			}
			if a.followMgr == nil {
				followCfg := following.DefaultFollowConfig()
				followPathfinder := fallbackPathFinder{
					primary:  a.pathfind,
					fallback: lowLevelPathfinder,
				}
				a.followMgr = following.NewFollowManager(
					a.targetSelector,
					followPathfinder,
					a.moveExec,
					a.GetPosition,
					a.SendChat,
					followCfg,
					func() string { return a.client.Name() },
				)
			}
		}
	}

	// Register core packet handlers if a client is available.
	if a.client != nil && a.packetMgr != nil {
		for _, h := range a.handlers() {
			a.client.Events().AddListener(h)
		}
		// Register generic packet logger if provided
		if a.logw != nil {
			a.client.Events().AddGeneric(a.packetLogger())
		}

		// Optional: register ReplayMod recorder alongside logger
		if a.cfg.EnableReplay {
			out := a.cfg.ReplayOutput
			if out == "" {
				out = "session.mcpr"
			}
			fileFormatVersion := 13
			if a.cfg.ProtocolVersion >= 764 { // 1.20.2+ needs login+config phases in replay
				fileFormatVersion = mcpr.CurrentFileFormatVersion
			}
			gen := a.cfg.ReplayGenerator
			if gen == "" {
				gen = "mc-agent"
			}
			rec, err := recorder.NewFile(out, mcpr.Meta{
				Protocol:          int(a.cfg.ProtocolVersion),
				MCVersion:         a.cfg.Version,
				FileFormatVersion: fileFormatVersion,
				Generator:         gen,
				ServerName:        a.cfg.Address,
			})
			if err == nil {
				a.rec = rec
				a.moveMirror = NewReplayMovementMirror(rec, a.packetMgr, a.cfg.SkinProvider)
				// If movement executor was configured earlier, wire its packet callback now.
				if a.moveMirror != nil && a.moveExec != nil {
					type packetCallbackSetter interface {
						SetPacketCallback(func(interface{}))
					}
					if setter, ok := a.moveExec.(packetCallbackSetter); ok {
						setter.SetPacketCallback(func(pkt interface{}) {
							if pkPkt, okPkt := pkt.(pk.Packet); okPkt {
								a.moveMirror.HandleServerbound(pkPkt)
							}
						})
					}
				}
				// Pre-initialize entity type from protocol data (before login)
				// In Minecraft 1.21+, entity_type registry is not sent during configuration phase
				if a.moveMirror != nil && a.packetMgr != nil {
					playerEntityType := a.packetMgr.GetEntityTypeID("minecraft:player")
					if playerEntityType == -1 {
						log.Printf("[ReplayMirror] WARNING: minecraft:player not found in protocol data, using fallback")
						playerEntityType = 148 // Fallback for safety (1.21.5 value)
					}
					log.Printf("[ReplayMirror] Pre-initializing player entity type to %d (minecraft:player)", playerEntityType)
					a.moveMirror.SetEntityType(playerEntityType)
				}
				// Use bundle delimiter filtering to avoid recording unconsumed buffer data
				// Login phase packets (including Set Compression) are filtered at the bot client level
				bundleDelimiterID := int32(a.packetMgr.GetClientboundPacketID("ClientboundBundleDelimiter"))
				a.client.Events().AddGeneric(bot.PacketHandler{Priority: 0, F: adapters.PacketFunc(rec, bundleDelimiterID)})
			} else {
				// If recorder setup fails, continue without recording
			}
		}
	}

	return nil
}

// Start connects to the server and begins background tasks.
func (a *agent) Start(ctx context.Context) error {
	a.mu.Lock()
	baseCtx := a.ctx
	a.mu.Unlock()

	// download jars and generate reports if needed
	if err := a.downloadJarsAndGenerateReports(); err != nil {
		return fmt.Errorf("failed to download jars and generate reports: %w", err)
	}

	if baseCtx == nil {
		// Allow Start without explicit Init; initialize implicitly.
		if err := a.Init(ctx); err != nil {
			return err
		}
		a.mu.Lock()
		baseCtx = a.ctx
		a.mu.Unlock()
	}

	// Connect to the server using the underlying client if present.
	if a.client != nil {
		opts := bot.JoinOptions{ProtocolVersion: a.cfg.ProtocolVersion}
		if a.rec != nil {
			opts.ReplayRecorder = a.rec
			opts.MovementMirror = a.moveMirror
			// opts.SkinProvider = a.cfg.SkinProvider
		}
		// Set registry callback to handle entity types and other registry data
		opts.RegistryDataCallback = a.onRegistryDataCallback
		if err := a.client.JoinServerWithOptions(baseCtx, a.cfg.Address, opts); err != nil {
			return err
		}

		// After connection, initialize replay recording metadata
		// The LOGIN packet was received during JoinServerWithOptions, but handlers
		// registered via AddListener only fire during HandleGame. We need to set
		// entity metadata immediately so MovementMirror can emit synthetic packets.
		if a.rec != nil {
			// Set selfId to -1 to match ReplayMod's standard behavior
			// (all players visible, no special camera entity)
			a.rec.SetSelfID(-1)
			log.Printf("[Agent %s][Replay] Set recorder selfId to -1", a.client.Name())
		}

		if a.moveMirror != nil {
			// Get entity ID from agent state (will be set by onLogin handler later during HandleGame)
			// For now, we need to wait for the first position update to get the entity ID
			// The entity metadata will be set in onLogin and onClientboundPosition handlers
			var id [16]byte
			if a.cfg.Auth.UUID != "" {
				if parsed, err := uuid.Parse(a.cfg.Auth.UUID); err == nil {
					copy(id[:], parsed[:])
				}
			}

			// Note: entity ID not available yet - will be set by onLogin handler
			// during HandleGame when ClientboundLogin is processed
			if a.cfg.Auth.Name != "" {
				a.moveMirror.SetEntityMeta(0, a.cfg.Auth.Name, id)
				log.Printf("[Agent %s][Replay] Initialized MovementMirror with name=%s uuid=%s (entityID will be set later)",
					a.client.Name(), a.cfg.Auth.Name, a.cfg.Auth.UUID)
			}
		}
	}

	// Start background cleanup goroutine for entity tracking
	a.startEntityCleanup(baseCtx.Done())

	// Start stop file watcher if configured
	if a.cfg.StopFilePath != "" {
		a.startStopFileWatcher(a.cfg.StopFilePath, baseCtx.Done())
	}

	// Start game handling loop if client supports it
	if a.client != nil {
		a.wg.Add(1)
		go func(ctx context.Context) {
			defer a.wg.Done()
			log.Printf("[Agent %s] Game handling loop started", a.client.Name())
			for {
				select {
				case <-ctx.Done():
					log.Printf("[Agent %s] Game handling loop exiting: context cancelled", a.client.Name())
					return
				default:
				}
				if err := a.client.HandleGame(ctx); err != nil {
					log.Printf("[Agent %s] Game handling loop exiting: HandleGame returned error: %v", a.client.Name(), err)
					// Cancel the agent's context to signal shutdown
					a.mu.Lock()
					if a.cancel != nil {
						a.cancel()
					}
					a.mu.Unlock()
					return
				}
			}
		}(baseCtx)
	}

	// Start continuous physics executor or position heartbeat at 20 TPS
	// Physics executor: Simulates physics continuously (gravity, forces, movement)
	// Heartbeat fallback: For non-physics executors (InterpolationExecutor)
	// Both ensure server sees continuous packets to prevent kicks and enable container interactions
	if physicsExec, ok := a.moveExec.(*movement.PhysicsMovementExecutor); ok {
		log.Printf("[Agent %s] Starting continuous physics executor", a.client.Name())
		a.waitForGroundData(baseCtx, 5*time.Second)
		physicsExec.Start()
	} else {
		log.Printf("[Agent %s] Starting position heartbeat (non-physics executor)", a.client.Name())
		a.startPositionHeartbeat(20)
	}

	a.startInitialPlan()

	return nil
}

func (a *agent) waitForGroundData(ctx context.Context, timeout time.Duration) {
	if a.worldMgr == nil {
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()
	lastLogTime := startTime

	for {
		select {
		case <-waitCtx.Done():
			log.Printf("[Agent %s] Ground check timed out after %s; starting physics anyway", a.client.Name(), timeout)
			return
		case <-ticker.C:
			x, y, z, _, _, ok := a.GetPosition()
			if !ok {
				continue
			}

			// Log progress every second
			if time.Since(lastLogTime) >= 1*time.Second {
				log.Printf("[Agent %s] Waiting for chunks at (%.1f, %.1f, %.1f)...", a.client.Name(), x, y, z)
				lastLogTime = time.Now()
			}

			if a.hasLoadedGround(x, y, z) {
				elapsed := time.Since(startTime)
				log.Printf("[Agent %s] Ground chunks loaded after %v", a.client.Name(), elapsed)
				return
			}
		}
	}
}

func (a *agent) hasLoadedGround(x, y, z float64) bool {
	if a.worldMgr == nil {
		return false
	}

	floorX := math.Floor(x)
	floorY := math.Floor(y)
	floorZ := math.Floor(z)

	// Check blocks 1-4 below agent
	for depth := 1; depth <= 4; depth++ {
		stateID, loaded := a.worldMgr.GetBlockAt(floorX, floorY-float64(depth), floorZ)

		// If chunk not loaded, we need to keep waiting
		if !loaded {
			return false
		}

		// If we find a non-air block, ground is available
		if stateID != 0 {
			return true
		}
	}

	// All chunks loaded but only air found - this is also a valid state
	// (agent may be spawning in air, physics will handle falling)
	return true
}

func (a *agent) downloadJarsAndGenerateReports() error {
	cacheDir := filepath.Join(".", "data", "download-cache")
	versionCacheDir := filepath.Join(cacheDir, a.cfg.Version)
	expectedReportsDir := filepath.Join(versionCacheDir, "data_generator")

	name := ""
	if a.client != nil {
		name = a.client.Name()
	}

	// Check if reports already exist
	if info, err := os.Stat(expectedReportsDir); err == nil && info.IsDir() {
		log.Printf("[Agent %s] Reports already exist at %s, skipping generation", name, expectedReportsDir)
		return nil
	}

	// Download JARs (GetVersionFiles will skip if already downloaded)
	files, err := protocol_utils.GetVersionFiles(a.cfg.Version, cacheDir)
	if err != nil {
		return err
	}

	// Generate reports
	baseDir, err := protocol_utils.GenerateReports(versionCacheDir, files["server.jar"])
	if err != nil {
		return fmt.Errorf("failed to generate reports base dir: %w", err)
	}
	log.Printf("[Agent %s] Generated reports base dir at %s", name, baseDir)
	return nil
}

// Done returns a channel that's closed when the agent's internal context is cancelled.
// It returns nil if Init hasn't been called yet.
func (a *agent) Done() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return nil
	}
	return a.ctx.Done()
}

// Close gracefully shuts down the agent and its background tasks.
func (a *agent) Close(context.Context) error {
	// Stop physics executor or position heartbeat before cancelling context
	if physicsExec, ok := a.moveExec.(*movement.PhysicsMovementExecutor); ok {
		log.Printf("[Agent %s] Stopping continuous physics executor", a.client.Name())
		physicsExec.Stop()
	} else {
		a.stopPositionHeartbeat()
	}

	a.mu.Lock()
	if a.cancel == nil {
		a.mu.Unlock()
		return nil
	}
	a.cancel()
	a.cancel = nil
	a.mu.Unlock()
	a.wg.Wait()
	if a.rec != nil {
		log.Printf("[Agent %s] [Replay] closing recorder", a.client.Name())
		if err := a.rec.Close(); err != nil {
			log.Printf("[agent.replay] ERROR closing recorder: %v", err)
			return fmt.Errorf("close recorder: %w", err)
		}
		log.Printf("[Agent %s] [Replay] closed recorder successfully", a.client.Name())
		a.rec = nil
	}
	return nil
}

// LastUpdateRecipes returns a copy of the most recently received Update Recipes payload.
func (a *agent) LastUpdateRecipes() (UpdateRecipesPayload, bool) {
	a.recipesMu.RLock()
	defer a.recipesMu.RUnlock()
	if a.lastUpdateRecipes == nil {
		return UpdateRecipesPayload{}, false
	}
	// shallow copy is fine since slices will be copied by caller if needed
	return *a.lastUpdateRecipes, true
}

// SetTeleportAccepter injects a TeleportAccepter implementation (e.g., player subsystem).
func (a *agent) SetTeleportAccepter(t TeleportAccepter) { a.mu.Lock(); a.teleport = t; a.mu.Unlock() }

// SetChat injects a Chat implementation.
func (a *agent) SetChat(c Chat) { a.mu.Lock(); a.chat = c; a.mu.Unlock() }

// SetPlayerUUIDResolver injects a function to resolve player names to UUIDs.
func (a *agent) SetPlayerUUIDResolver(f func(string) ([16]byte, error)) {
	a.mu.Lock()
	a.playerUUIDByName = f
	a.mu.Unlock()
}

// UpdatePosition sets internal position; intended for movement executor wiring.
func (a *agent) UpdatePosition(x, y, z float64, yaw, pitch float32) {
	a.setPosition(x, y, z, yaw, pitch)
}

// SetFollowManager injects a follow manager implementation.
func (a *agent) SetFollowManager(f models.FollowManager) {
	a.mu.Lock()
	a.followMgr = f
	a.mu.Unlock()
}

// SetTargetSelector injects a TargetSelector for use by other subsystems.
func (a *agent) SetTargetSelector(ts models.TargetSelector) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// No-op if the same selector is already set
	if a.targetSelector == ts {
		return
	}
	a.targetSelector = ts
}

// ResolvePlayerUUIDByName resolves a player UUID using the injected resolver.
func (a *agent) ResolvePlayerUUIDByName(name string) ([16]byte, error) {
	a.mu.Lock()
	f := a.playerUUIDByName
	a.mu.Unlock()
	if f == nil {
		return [16]byte{}, ErrInvalidConfig("player UUID resolver not set")
	}
	return f(name)
}

// SetPlayerNameResolver injects resolver to map UUID->player name.
func (a *agent) SetPlayerNameResolver(f func([16]byte) (string, bool)) {
	a.mu.Lock()
	a.playerNameByUUID = f
	a.mu.Unlock()
}

// SetItemManager injects an ItemManager for item name lookup.
func (a *agent) SetItemManager(im ItemManager) { a.mu.Lock(); a.itemMgr = im; a.mu.Unlock() }

// SetSlotResolver injects a SlotResolver to inspect current slot contents.
func (a *agent) SetSlotResolver(sr SlotResolver) { a.mu.Lock(); a.slots = sr; a.mu.Unlock() }

// Movement/pathfinding injection
func (a *agent) SetMovementExecutor(m MovementExecutor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.moveExec = m

	// If we have a movement mirror and the executor supports packet callbacks,
	// wire them together so movement packets are mirrored to the replay
	if a.moveMirror != nil {
		// Try to set the packet callback (will only work if m has the method)
		type packetCallbackSetter interface {
			SetPacketCallback(func(interface{}))
		}
		if setter, ok := m.(packetCallbackSetter); ok {
			setter.SetPacketCallback(func(pkt interface{}) {
				// The packet from packet.Marshal is pk.Packet
				if pkPkt, okPkt := pkt.(pk.Packet); okPkt {
					a.moveMirror.HandleServerbound(pkPkt)
				}
			})
		}
	}
}

// MovementExecutorType reports the current movement executor flavor for diagnostics.
func (a *agent) MovementExecutorType() movement.ExecutorType {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.moveExec == nil {
		return movement.InterpolationExecutor
	}
	if _, ok := a.moveExec.(*movement.PhysicsMovementExecutor); ok {
		return movement.PhysicsExecutor
	}
	return movement.InterpolationExecutor
}
func (a *agent) SetPathFinder(pf models.PathFinder) { a.mu.Lock(); a.pathfind = pf; a.mu.Unlock() }

// SetTelemetryRecorder injects a telemetry recorder for movement testing.
// Only works with PhysicsMovementExecutor.
func (a *agent) SetTelemetryRecorder(recorder models.MovementTelemetryRecorder) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Try to set telemetry on physics executor
	if physicsExec, ok := a.moveExec.(*movement.PhysicsMovementExecutor); ok {
		physicsExec.SetTelemetryRecorder(recorder)
	}
}

// Errors
type configError string

func (e configError) Error() string     { return string(e) }
func ErrInvalidConfig(msg string) error { return configError(msg) }

var ErrAlreadyInitialized = errors.New("agent: already initialized")

// String returns a human-friendly description for logging.
func (a *agent) String() string {
	return fmt.Sprintf("agent{addr=%s ver=%s}", a.cfg.Address, a.cfg.Version)
}
