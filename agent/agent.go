package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/movement"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/physics"
	agentutils "github.com/reallyoldfogie/mc-agent/utils"
	mcworld "github.com/reallyoldfogie/mc-agent/world"

	"github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/basic"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	rof_utils "github.com/reallyoldfogie/mc-bot-go/utils"

	"github.com/reallyoldfogie/mc-client-test-go/testenv"

	protocol_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	protocol_utils "github.com/reallyoldfogie/mc-protocol-go/utils"

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
	rcon   testenv.RCONHelper
	logger *slog.Logger
}

func (r *rconAdapter) SummonEntity(ctx context.Context, x, y, z float64, entityType, nbtData string) {
	result := r.rcon.SummonEntity(ctx, x, y, z, entityType, nbtData)
	// Fire and forget - we don't care about the result for visualization
	resp, _ := result.Exec(ctx)
	safeLogger(r.logger).Info(fmt.Sprintf("%s => %s", result.Cmd, resp))
}

// agent is the top-level orchestrator of bot/client, state, and event wiring.
// pendingProjectileInfo represents a projectile waiting to spawn with an optional callback
type pendingProjectileInfo struct {
	projectileType models.ProjectileType
	callbacks      []models.ProjectileHitCallback // Multiple callbacks supported
	targetPos      models.V3                      // Intended target (copied to active info on spawn)
	hasTarget      bool                           // Whether a target was provided
}

// activeProjectileInfo tracks information about a fired projectile
type activeProjectileInfo struct {
	projectileType  models.ProjectileType
	firedAt         time.Time
	callbacks       []models.ProjectileHitCallback // Multiple callbacks supported
	callbacksFired  bool                           // Whether callbacks have already been fired (prevents double-firing)
	isInGround      bool                           // For arrows: whether they've hit a block
	shake           int8                           // Shake animation counter (0-7)
	criticalHit     bool                           // From CRITICAL_FLAG metadata
	pierceLevel     int8                           // From PIERCE_LEVEL metadata
	potionColor     int32                          // From COLOR metadata (-1 = no potion)
	soundEventCount int                            // Track sound events received
	// Position history for render loop interpolation
	lastServerPos     models.V3 // Previous position from server
	currentServerPos  models.V3 // Current position from server
	lastServerTime    time.Time // Timestamp of last position update
	currentServerTime time.Time // Timestamp of current position update
	interpolatedPos   models.V3 // Current interpolated position
	// Spawn data for pre-update interpolation
	spawnPos      models.V3 // Position when projectile spawned
	spawnTime     time.Time // Time when projectile spawned
	spawnVelocity models.V3 // Velocity from spawn packet
	// Target information for hit validation
	targetPos models.V3 // Intended target for hit validation
	hasTarget bool      // Whether a target was provided

	// Position history for trajectory visualization
	positionHistory []models.V3 // Server-confirmed positions during flight (persistent projectiles)

	// Server-authoritative position confirmation for persistent projectiles
	pendingCallbackFire bool                     // Whether callback should fire when server position arrives
	pendingHitType      models.ProjectileHitType // Hit type for pending callback
	pendingHitPos       models.V3                // Fallback position if server doesn't send one (client prediction)
	collisionDetectTime time.Time                // When collision was first detected (for timeout tracking)

	// Callback timeout tracking
	callbackRegisteredAt time.Time // When callbacks were registered (for timeout detection)
}

type agent struct {
	cfg models.AgentConfig

	// lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// fine-grained locks (replacing single a.mu for subsystems)
	// lifecycle management: ctx, cancel
	lifecycleMu sync.RWMutex
	// optional fallback handlers: teleport, chat
	fallbackHandlersMu sync.RWMutex
	// player name/UUID resolvers: playerUUIDByName, playerNameByUUID
	playerResolversMu sync.RWMutex
	// following subsystem: followMgr, targetSelector
	followingMu sync.RWMutex
	// movement subsystem: moveExec, pathfind, shapeMgr
	movementMu sync.RWMutex
	// slots subsystem: slots
	slotsMu sync.RWMutex
	// itemMgr
	itemMgrMu sync.RWMutex
	// container subsystem: containerHelper, screenMgr, worldMgr
	containerSubsystemMu sync.RWMutex

	// logging: logger/logWriter are created once in New and never reassigned,
	// so they're safe to read from any goroutine without a lock; loggingMu
	// only guards the logFile lifecycle (setupLogging/closeLogging).
	logger          *slog.Logger
	logWriter       *syncWriter
	loggingMu       sync.Mutex
	logFile         *os.File
	packetLogWriter io.Writer

	// dependencies (to be filled in during Init)
	client         bot.Client
	packetMgr      protocol_models.PacketMgr
	blockMgr       protocol_versions.BlockMgr
	soundMgr       protocol_versions.SoundMgr
	versionHandler models.VersionHandler // optional version-specific packet handler

	// internal state placeholders (expanded during migration)
	// tracking
	posMu            sync.RWMutex
	posX, posY, posZ float64
	posYaw, posPitch float64
	posInitialized   bool

	// sequenceMu guards the anti-cheat sequence counter used on outgoing
	// Player Action packets. Kept separate from posMu since it has nothing
	// to do with position.
	sequenceMu          sync.Mutex
	sequenceCounter     int32
	sequenceInitialized bool

	healthMu          sync.RWMutex
	health            float32
	food              int32
	foodSaturation    float32
	healthInitialized bool

	entIDMu sync.RWMutex
	entID   int32

	mountedEntityMu sync.RWMutex
	mountedEntityID int32 // -1 = not mounted
	// mountedPassengerIndex is our position in the vehicle's passenger list,
	// as ordered by ClientboundSetPassengers. Index 0 is the controlling
	// passenger (the driver); later indices are passive riders. -1 when not
	// mounted. Vehicles that seat more than one player (boat, camel, happy
	// ghast) only obey the passenger at index 0, so the riding dispatch needs
	// this to decide whether to predict movement or just follow the server.
	mountedPassengerIndex int

	entitiesMu sync.RWMutex
	entities   map[int32]*trackedEntity

	// abilitiesMu guards abilities/abilitiesInitialized.
	abilitiesMu sync.RWMutex
	// abilities is the bot's own last-known PlayerAbilities, as reported by
	// the server's clientbound Abilities packet (sent at login and
	// whenever it changes, e.g. a game mode switch).
	abilities            models.PlayerAbilities
	abilitiesInitialized bool

	// gameModeMu guards gameMode/gameModeInitialized.
	gameModeMu sync.RWMutex
	// gameMode is the bot's own current game mode, set from the Login
	// packet at join and kept current via ClientboundGameEvent's
	// GAME_MODE_CHANGED reason.
	gameMode            models.GameMode
	gameModeInitialized bool

	// camFollowMu guards camFollowCancel.
	camFollowMu sync.Mutex
	// camFollowCancel stops the currently active StartCamFollow loop, if
	// any. nil when not currently following.
	camFollowCancel context.CancelFunc

	// ownEffectsMu guards ownEffects.
	ownEffectsMu sync.RWMutex
	// ownEffects holds the agent's own player entity's active status
	// effects, keyed by full minecraft:mob_effect registry name. Tracked
	// separately from entities (which never contains the bot's own entity
	// — the server never sends us an AddEntity spawn packet for ourselves)
	// so GetOwnActiveEffect has somewhere to actually read from.
	ownEffects map[string]models.ActiveEffect

	// entityWindowsMu guards entityWindows.
	entityWindowsMu sync.RWMutex
	// entityWindows maps an open container window ID to the entity whose
	// container it is. The container packets only carry a window ID, so without
	// this the contents cannot be attributed to the entity they belong to.
	// Entries are added when an entity container is opened and removed when it
	// closes.
	entityWindows map[byte]int32
	// pendingEntityContainerID is the entity whose container we are in the middle
	// of opening, or -1 when no open is in flight.
	//
	// The window ID is only known once the server has assigned it, but the server
	// sends the window's contents as part of opening it — so the contents can
	// arrive before we could possibly have recorded the mapping. Noting the
	// entity up front lets a content packet for an as-yet-unmapped window bind
	// itself to the right entity instead of being dropped.
	pendingEntityContainerID int32

	// e.g., player, playerList, world, movement, pathfinding, registries, etc.
	// registries
	regMu      sync.RWMutex
	registries map[RegistryID]CustomRegistry

	// replay helpers
	moveMirror MovementMirror

	// core subsystems (created automatically in Init) - using interfaces for separation of concerns
	player       TeleportAccepter       // Player subsystem (teleportation)
	worldMgr     models.World           // World manager (block queries, pathfinding)
	mcAgentWorld *mcworld.Manager       // mc-agent world manager (when version handler is available)
	screenMgr    models.ScreenSubsystem // Screen manager (inventory/containers)
	playerList   playerlist.PlayerList  // Player list (online player tracking)

	// Chunk batching (1.20.2+): track number of batches received for acknowledgement
	chunkBatchCount float32

	// playerLoadedSent fires once-per-connection. The vanilla client sends
	// ServerboundPlayerLoaded after the world finishes loading; without it the
	// 1.21.4+ server silently drops interact/vehicle packets for the first 60
	// server ticks (PlayerEntity.isLoaded gate).
	playerLoadedSent sync.Once

	// optional subsystems (for dependency injection override)
	teleport   TeleportAccepter // override player if needed
	chat       Chat             // fallback chat for when version handler unavailable
	moveExec   models.MovementExecutor
	pathfind   models.PathFinder
	shapeMgr   models.BlockShapeManager
	stateProps *pathfinding.StatePropertyLoader
	itemMgr    ItemManager
	itemUsage  *items.ItemUsage
	slots      SlotResolver

	// HPA* dynamic world updates
	hpaUpdateHandler *pathfinding.WorldUpdateHandler

	// container/inventory system
	containerHelper ContainerHelper
	invMgr          models.InventoryManager

	// position heartbeat (continuous position packets at 20 TPS)
	posHeartbeatMu     sync.Mutex
	posHeartbeatActive bool
	posHeartbeatStop   chan struct{}

	// entity position update callbacks (for testing/monitoring)
	entityPosCallbacksMu sync.RWMutex
	entityPosCallbacks   []models.EntityPositionCallback

	// plugin-message (CustomPayload) receive callbacks
	pluginMsgCallbacksMu sync.RWMutex
	pluginMsgCallbacks   []models.PluginMessageCallback

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
	commandRegistry models.ActionRegistry[models.CommandAgent]

	// chat events
	chatEvents chan string

	// recipes (last Update Recipes payload)
	recipesMu         sync.RWMutex
	lastUpdateRecipes *UpdateRecipesPayload

	heldSlotMu      sync.RWMutex
	heldSlot        int16
	heldSlotSet     bool
	heldSlotUpdates chan int16

	// chatHandlersInitialized guards initChatCommandHandlers against the
	// Init() setup block running more than once per agent.
	chatHandlersInitialized bool

	// projectile hit callbacks and tracking
	pendingProjectilesMu sync.Mutex
	pendingProjectiles   []pendingProjectileInfo
	activeProjectilesMu  sync.Mutex
	activeProjectiles    map[int32]*activeProjectileInfo

	// entity metadata handling
	entityRegistry    *models.EntityRegistry
	metadataHandler   models.MetadataHandler
	poseRegistry      *models.EntityPoseRegistry
	attributeDefaults *models.EntityAttributeDefaultsRegistry

	// critical error handling
	criticalErrorMu sync.Mutex
	criticalError   error
}

// New constructs an agent with the provided configuration.
func New(cfg models.AgentConfig) (models.Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var name string
	if cfg.Name != "" {
		name = cfg.Name
	} else {
		if cfg.Client != nil {
			name = cfg.Client.Name()
		} else {
			name = "UnnamedAgent"
		}
	}

	// Created here (rather than in Init) so it's available for the
	// RegistriesPath warnings below, and stable for the agent's whole
	// lifetime: setupLogging/closeLogging only ever redirect its writer, so
	// nothing in the package needs to reload a.logger after Init or Close.
	logger, logWriter := newAgentLogger(name, cfg.LogLevel)

	// Ensure RegistriesPath is set and data is available
	// If not set, defaults to ~/.agent/cache/mc-agent/registries/{version}/
	// Downloads and generates registries.json if needed (thread-safe)
	if cfg.RegistriesPath == "" {
		resolvedPath, err := agentutils.EnsureRegistriesPath("", cfg.Version)
		if err != nil {
			// Non-fatal: log warning and continue without registries
			// They can still be loaded manually or from testing framework paths
			logger.Warn("failed to ensure registries path", "error", err)
			logger.Warn("registries will need to be loaded manually via LoadEntityTypesFromRegistry()")
		} else {
			cfg.RegistriesPath = resolvedPath
			logger.Info("RegistriesPath auto-configured", "path", resolvedPath)
		}
	}

	a := &agent{
		cfg:                      cfg,
		logger:                   logger,
		logWriter:                logWriter,
		packetLogWriter:          cfg.LogWriter,
		chatEvents:               make(chan string, 64),
		commandRegistry:          actions.NewRegistry(),
		pendingProjectiles:       []pendingProjectileInfo{},
		activeProjectiles:        map[int32]*activeProjectileInfo{},
		entityRegistry:           models.NewEntityRegistry(),
		mountedEntityID:          -1, // -1 indicates not mounted
		mountedPassengerIndex:    -1,
		pendingEntityContainerID: -1, // -1 indicates no container open in flight
	}
	a.planRunner = plan.NewRunner(a)

	return a, nil
}

// Init prepares dependencies, managers and event wiring but does not connect.
func (a *agent) Init(ctx context.Context) error {
	// Brief lock for double-init check only. Remaining Init body is single-threaded
	// (goroutines start in Start(), not here), so no locks needed for field writes.
	a.lifecycleMu.Lock()
	if a.cancel != nil {
		a.lifecycleMu.Unlock()
		return ErrAlreadyInitialized
	}
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.lifecycleMu.Unlock()

	// Open this agent's own log file and point its logger at file + stdout.
	if err := a.setupLogging(); err != nil {
		return fmt.Errorf("setup agent logging: %w", err)
	}

	// Resolve version and protocol (auto-detect from server if not specified)
	_, err := a.resolveVersionAndManagers()
	if err != nil {
		return err
	}

	// Ensure the version's data-generator reports (registries.json, etc.) exist
	// before trying to load registries from file below. downloadJarsAndGenerateReports
	// is idempotent (skips if the reports dir already exists), so this is a fast
	// no-op for any version that's been used before. Without this, a brand-new
	// version's first-ever Init() (called separately from Start(), which is the
	// common case in tests) would race ahead of report generation and silently
	// end up with no file-loaded registries (entity_type, block_entity_type,
	// etc.) for the entire agent lifetime.
	if err := a.downloadJarsAndGenerateReports(); err != nil {
		return fmt.Errorf("failed to download jars and generate reports: %w", err)
	}

	a.logf("[Agent %s] Trying to load registries from file (%s)", a.cfg.Name, a.cfg.RegistriesPath)
	// Load client-side registries from file if available
	// (server config packets will overwrite any registries loaded from file)
	if err := a.LoadRegistriesFromFile(a.cfg.RegistriesPath); err != nil {
		a.logf("[Agent %s][WARN] failed to load registries from file: %v (hardcoded IDs may be needed)", a.cfg.Name, err)
	}

	// Use prebuilt client when provided
	if a.client == nil && a.cfg.Client != nil {
		a.client = a.cfg.Client
	}

	if a.client == nil && a.packetMgr != nil {
		a.client = bot.NewClient(a.packetMgr)
		// Set auth on the newly created client if provided in config
		if a.cfg.Auth.Name != "" || a.cfg.Auth.UUID != "" {
			a.client.SetAuth(bot.Auth{
				AccessToken: a.cfg.Auth.AccessToken,
				Name:        a.cfg.Auth.Name,
				UUID:        a.cfg.Auth.UUID,
			})
			a.logf("[Agent] Created bot client for version %s (auth: %s)", a.cfg.Version, a.cfg.Auth.Name)
		} else {
			a.logf("[Agent] Created bot client for version %s (no auth configured)", a.cfg.Version)
		}
	}

	// Note: client can be nil for testing scenarios where dependencies are injected manually.
	// Subsystem creation is skipped if client is nil.

	// Set version handler on bot client immediately (before connection)
	if a.client != nil {
		a.logf("[Agent %s][DEBUG] Checking version handler: versionHandler=%v", a.cfg.Name, a.versionHandler != nil)

		if a.versionHandler != nil {
			type versionHandlerSetter interface {
				SetVersionHandler(bot.VersionHandler)
			}
			if vhSetter, ok := a.client.(versionHandlerSetter); ok {
				adapter := NewVersionHandlerAdapter(a.versionHandler, a.logger)
				vhSetter.SetVersionHandler(adapter)
				a.logf("[Agent %s] Version handler set for %s during Init", a.cfg.Name, a.versionHandler.Version())
			} else {
				a.logf("[Agent %s][DEBUG] Type assertion failed for SetVersionHandler", a.cfg.Name)
			}
		} else {
			a.logf("[Agent %s][DEBUG] Skipping version handler setup (versionHandler nil)", a.cfg.Name)
		}
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

	// Initialize entity metadata handler with entity registry.
	// Pose registry is injected later (after data path is resolved) via
	// the BasicMetadataProcessor.SetPoseRegistry hook below.
	if a.entityRegistry != nil {
		a.metadataHandler = models.NewBasicMetadataProcessor(a.entityRegistry, a.logger)
		a.logf("[Agent] Entity metadata handler initialized")
	}

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

		// Note: Teleported handler is NOT provided here because the agent's own
		// onClientboundPosition handler (in handlers.go) already handles position
		// parsing and teleport confirmation using version-specific parsing.
		// Providing Teleported here would cause duplicate TeleportConfirm packets
		// to be sent, which can cause issues with certain Minecraft versions.
		playerConcrete := basic.NewPlayer(botClient, customSettings, basic.EventsListener{
			GameStart:    a.HandleGameStart,
			Disconnect:   a.HandleDisconnect,
			HealthChange: a.HandleHealthChange,
			Death:        a.HandleDeath,
		}, a.packetMgr)
		a.player = playerConcrete // Assign concrete type to interface field
		a.logf("[Agent %s] Player subsystem initialized", a.cfg.Name)

		// Create PlayerList (for player tracking and UUID resolution)
		playerList := playerlist.New(botClient, a.packetMgr)
		a.playerList = playerList // Store as interface{}
		a.logf("[Agent %s] PlayerList initialized", a.cfg.Name)
		if a.playerUUIDByName == nil {
			a.playerUUIDByName = func(name string) ([16]byte, error) {
				players := playerList.Get()
				a.logf("[Agent %s] ResolvePlayerUUID: looking for %q in list with %d players", a.cfg.Name, name, len(players))
				for id, info := range players {
					a.logf("[Agent %s] ResolvePlayerUUID: checking player %q (UUID: %s)", a.cfg.Name, info.Name, id.String())
					if info.Name == name {
						var out [16]byte
						copy(out[:], id[:])
						a.logf("[Agent %s] ResolvePlayerUUID: found %q -> %s", a.cfg.Name, name, id.String())
						return out, nil
					}
				}
				a.logf("[Agent %s] ResolvePlayerUUID: player %q not found in list", a.cfg.Name, name)
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

		// Create World Manager (for chunk management). mc-agent's own world.Manager
		// is the sole implementation now (see docs/plans/WORLD_STRUCT_CONSOLIDATION.md):
		// it's a complete, version-aware reimplementation that already supersedes
		// mc-bot-go/bot/world's generic (non-version-aware) chunk parsing, and
		// NewManager already tolerates a nil versionHandler. Note: worldPacketHandlers()
		// still requires a real versionHandler to register any chunk/block/time
		// packet handlers at all, so a Client supplied without a VersionHandler
		// (the resolveVersionAndManagers escape hatch for tests using mock clients)
		// now yields a world manager that never receives chunk data — accepted,
		// since nothing in the current codebase exercises that combination.
		a.mcAgentWorld = mcworld.NewManager(a.versionHandler, mcworld.EventsListener{
			LoadChunk: func(pos mcworld.ChunkPos) error {
				return a.HandleChunkLoad(models.ChunkPos{X: pos.X, Z: pos.Z})
			},
			UnloadChunk: func(pos mcworld.ChunkPos) error {
				return a.HandleChunkUnload(models.ChunkPos{X: pos.X, Z: pos.Z})
			},
		}, a.logger)
		a.worldMgr = a.mcAgentWorld
		a.logf("[Agent %s] Using mc-agent world manager (versionHandler=%v)", a.cfg.Name, a.versionHandler != nil)

		// Create Screen Manager (for inventory/containers)
		a.screenMgr = screen.NewManager(botClient, containerEvents{agent: a}, a.packetMgr)
		a.logf("[Agent %s] Screen manager initialized", a.cfg.Name)

		// Initialize slot resolver so inventory can be queried
		a.slots = newScreenManagerSlotResolver(a)
		a.logf("[Agent %s] Slot resolver initialized from screen manager", a.cfg.Name)

		// Initialize item manager with a registry-backed default so item name
		// lookups work out-of-the-box. Callers that need custom behavior can
		// override this by injecting their own ItemManager via SetItemManager
		// after Init.
		a.itemMgrMu.Lock()
		if a.itemMgr == nil {
			a.itemMgr = registryItemManager{registryGetter: a.GetRegistry}
			a.logf("[Agent %s] Item manager initialized from registries", a.cfg.Name)
		}
		a.itemMgrMu.Unlock()

		a.initHeldSlotTracking()
		a.initClientInformationHandler(customSettings)
		a.initChatCommandHandlers()

		var shapeMgr models.BlockShapeManager
		var stateProps *pathfinding.StatePropertyLoader
		mcDataGenCacheDir := filepath.Join(".agent", "cache", "mc-data-gen")
		if resolvedCacheDir, cacheErr := agentutils.FindOrCreateCacheDir(); cacheErr == nil {
			mcDataGenCacheDir = filepath.Join(resolvedCacheDir, "mc-data-gen")
		}
		dataBasePath, err := agentutils.ResolveDataPath(a.cfg.MCDataGenPath, mcDataGenCacheDir, "")
		if err == nil {
			a.logf("[Agent %s] Resolved data path: %s", a.cfg.Name, dataBasePath)
			// Verify path exists and has version directory
			versionPath := filepath.Join(dataBasePath, a.cfg.Version)
			if info, err := os.Stat(versionPath); err == nil && info.IsDir() {
				a.logf("[Agent %s] Version directory exists: %s", a.cfg.Name, versionPath)
			} else {
				a.logf("[Agent %s] Warning: version directory missing: %s (error: %v)", a.cfg.Name, versionPath, err)
			}

			// Use new unified Minecraft data cache for block properties
			dataCache := agentutils.NewMinecraftDataCache(a.cfg.Version, a.logger)
			if err := dataCache.EnsureDataGenerated(); err == nil {
				blocksJSONPath, err := dataCache.GetBlocksJSONPath()
				if err == nil {
					if spl, err := pathfinding.NewStatePropertyLoader(blocksJSONPath); err == nil {
						stateProps = spl
						a.logf("[Agent %s] Loaded block state properties from cache", a.cfg.Name)
					} else {
						a.logf("[Agent %s] Warning: failed to load state properties: %v", a.cfg.Name, err)
					}
				} else {
					a.logf("[Agent %s] Warning: failed to get blocks.json path: %v", a.cfg.Name, err)
				}
			} else {
				a.logf("[Agent %s] Warning: failed to generate Minecraft data cache: %v", a.cfg.Name, err)
			}
			shapeMgr, err = pathfinding.NewBlockShapeManager(a.cfg.Version, dataBasePath, a.blockMgr, stateProps, a.logger)
			if err != nil {
				a.logf("[Agent %s] Warning: failed to create block shape manager: %v", a.cfg.Name, err)
			} else {
				a.logf("[Agent %s] Successfully created block shape manager", a.cfg.Name)
			}

			if poses, perr := models.LoadEntityPoseRegistry(dataBasePath, a.cfg.Version); perr == nil {
				a.poseRegistry = poses
				if poses.UsedFallback() {
					a.logf("[Agent %s] poses.json missing for %s, using built-in EntityPose fallback (%d entries)", a.cfg.Name, a.cfg.Version, poses.Count())
				} else {
					a.logf("[Agent %s] Loaded EntityPose registry: %d entries", a.cfg.Name, poses.Count())
				}
			} else {
				a.poseRegistry = models.NewEntityPoseRegistryFromFallback()
				a.logf("[Agent %s] Warning: failed to load poses.json (%v); using fallback EntityPose table", a.cfg.Name, perr)
			}
			if bmp, ok := a.metadataHandler.(*models.BasicMetadataProcessor); ok && a.poseRegistry != nil {
				bmp.SetPoseRegistry(a.poseRegistry)
			}

			if attrDefaults, aerr := models.LoadEntityAttributeDefaultsRegistry(dataBasePath, a.cfg.Version); aerr == nil {
				a.attributeDefaults = attrDefaults
				if attrDefaults.UsedFallback() {
					a.logf("[Agent %s] entity attribute data missing for %s, using built-in fallback defaults", a.cfg.Name, a.cfg.Version)
				} else {
					a.logf("[Agent %s] Loaded entity attribute defaults: %d entity types", a.cfg.Name, attrDefaults.Count())
				}
			} else {
				a.attributeDefaults = models.NewEntityAttributeDefaultsRegistryFromFallback()
				a.logf("[Agent %s] Warning: failed to load entity attribute defaults (%v); using fallback table", a.cfg.Name, aerr)
			}
		} else {
			a.logf("[Agent %s] Warning: failed to resolve data path: %v", a.cfg.Name, err)
		}

		// Create movement executor (core component needed for container interactions, looking, movement, etc.)
		// Default to physics executor (realistic movement)
		executorType := movement.PhysicsExecutor
		execConfig := movement.ExecutorConfig{
			Client:         botClient,
			PacketMgr:      a.packetMgr,
			GetBotPos:      a.getBotPositionLegacy,
			SetBotPos:      a.UpdatePosition,
			GetBotEntityID: a.GetEntityID,
			Ctx:            a.ctx,
			Logger:         a.logger,
		}

		// Check if physics executor can be used (requires world manager, shape data, block manager)
		if a.worldMgr == nil {
			a.logf("[Agent %s] Warning: world manager unavailable", a.cfg.Name)
			executorType = movement.UnknownExecutor
		} else if shapeMgr == nil {
			a.logf("[Agent %s] Warning: block shape data unavailable", a.cfg.Name)
			executorType = movement.UnknownExecutor
		} else if a.blockMgr == nil {
			a.logf("[Agent %s] Warning: block manager unavailable", a.cfg.Name)
			executorType = movement.UnknownExecutor
		} else {
			// Physics executor can be used
			a.mcAgentWorld.WithEntityProvider(a) // Agent implements models.EntityProvider
			execConfig.World = a.mcAgentWorld
			execConfig.ShapeProvider = shapeMgr
		}

		if executorType == movement.UnknownExecutor {
			panic(fmt.Sprintf("Agent %s missing required subsystems: worldMgr=%v, shapeMgr=%v, blockMgr=%v. Cannot initialize movement executor.", a.cfg.Name, a.worldMgr != nil, shapeMgr != nil, a.blockMgr != nil))
		}

		// Allow explicit override to interpolation via config flag
		a.moveExec = movement.NewExecutor(executorType, execConfig)
		a.logf("[Agent %s] Movement executor initialized (%s)", a.cfg.Name, executorType.String())
		a.shapeMgr = shapeMgr
		a.stateProps = stateProps

		// Wire up version handler to movement executor if available
		if a.versionHandler != nil {
			a.moveExec.SetMovementHandler(a.versionHandler.Play().Movement())
			a.logf("[Agent %s] Movement executor using version-specific handler for %s", a.cfg.Name, a.versionHandler.Version())
		}

		if a.cfg.EnableClutchAssist {
			if clutchSetter, ok := a.moveExec.(interface {
				SetClutchCallback(func(plan physics.ClutchPlan))
			}); ok {
				usage := items.NewItemUsage(
					botClient.Conn(),
					a.packetMgr,
					a.logger,
				)
				clutchSetter.SetClutchCallback(func(plan physics.ClutchPlan) {
					a.logf("[Clutch] plan=%s fall=%.2f ticks=%d place=(%.0f,%.0f,%.0f)",
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
				a.logf("[Agent %s] Warning: clutch assist enabled but movement executor does not support clutch callbacks", a.cfg.Name)
			}
		}

		// Set up mounted entity position getter and version handler for riding support
		if positionGetter, ok := a.moveExec.(interface {
			SetMountedEntityPositionGetter(models.MountedEntityPositionGetter)
		}); ok {
			positionGetter.SetMountedEntityPositionGetter(a)
			a.logf("[Agent %s] Movement executor configured for mounted entity position tracking", a.cfg.Name)
		}

		if versionHandlerSetter, ok := a.moveExec.(interface {
			SetVersionHandler(models.VersionHandler)
		}); ok && a.versionHandler != nil {
			versionHandlerSetter.SetVersionHandler(a.versionHandler)
			a.logf("[Agent %s] Movement executor configured with version handler for vehicle movement", a.cfg.Name)
		}

		// Create pathfinder if data paths are configured
		// Create low-level A* pathfinder
		lowLevelPathfinder := pathfinding.NewAStarPathFinderWithConfig(a.worldMgr, shapeMgr, pathfinding.PathfinderConfig{
			GoalRadius: a.cfg.PathfinderGoalRadius,
		}, a.logger)

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

				a.logf("[Agent %s] Stuck recovery: re-pathfinding from (%.2f, %.2f, %.2f) -> snapped (%.0f, %.0f, %.0f) to (%.0f, %.0f, %.0f)",
					a.cfg.Name, currentPos.X, currentPos.Y, currentPos.Z,
					snappedStart.X, snappedStart.Y, snappedStart.Z,
					snappedGoal.X, snappedGoal.Y, snappedGoal.Z)

				// Try to pathfind from snapped position to goal
				distance := snappedStart.DistanceTo(snappedGoal)
				maxSteps := max(int(distance*150), 10000)

				path, err := lowLevelPathfinder.FindPath(context.Background(), snappedStart, snappedGoal, maxSteps)
				if err != nil {
					a.logf("[Agent %s] Stuck recovery pathfinding failed: %v", a.cfg.Name, err)
					return nil
				}

				if !path.Found || len(path.Steps) == 0 {
					a.logf("[Agent %s] Stuck recovery: no path found from current position", a.cfg.Name)
					return nil
				}

				a.logf("[Agent %s] Stuck recovery: found path with %d steps", a.cfg.Name, len(path.Steps))
				return path
			})
			a.logf("[Agent %s] Stuck recovery callback configured", a.cfg.Name)
		}

		// Set up mount/dismount callbacks for vehicle pathfinding
		if mountCallbackSetter, ok := a.moveExec.(interface {
			SetMountCallbacks(func(context.Context, int32) error, func(context.Context) error)
		}); ok {
			mountCallbackSetter.SetMountCallbacks(
				func(ctx context.Context, entityID int32) error {
					return a.MountEntity(ctx, entityID)
				},
				func(ctx context.Context) error {
					return a.DismountEntity()
				},
			)
			a.logf("[Agent %s] Mount/dismount callbacks configured", a.cfg.Name)
		}

		// Wrap with HPA* for hierarchical pathfinding on long distances
		// Larger cluster size = fewer clusters, faster building (but more entrances per cluster)
		// 32x32x32 aligns with Minecraft chunks (16x16) and is power-of-2 for CPU efficiency
		clusterSize := 32 // 32x32x32 blocks per cluster (2x2 chunks horizontally)
		hpaPathfinder := pathfinding.NewHPAPathFinder(a.worldMgr, shapeMgr, lowLevelPathfinder, clusterSize, a.logger)
		if limiter, ok := hpaPathfinder.(interface {
			SetEntranceLimits(maxCount int, maxCost float64)
		}); ok {
			//limiter.SetEntranceLimits(4, 10)
			limiter.SetEntranceLimits(50, 2000) // Allow paths up to 2000 cost to entrances (handles complex vertical terrain)
		}

		// Wrap HPA pathfinder with vehicle-aware pathfinder
		vehicleAwarePathfinder := pathfinding.NewVehicleAwarePathFinder(
			hpaPathfinder,
			a,          // Agent implements VehicleProvider
			a.worldMgr, // World for terrain checking
			shapeMgr,   // Shape manager for vehicle movement validation
			32.0,       // Default search radius for vehicles
			a.logger,
		)
		a.pathfind = vehicleAwarePathfinder

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
				adapter := &rconAdapter{rcon: a.cfg.RCON, logger: a.logger}
				debugViz := pathfinding.NewHPADebugVisualizer(adapter, pathfinding.HPADebugVisualizerConfig{
					PathBlock: a.cfg.HPADebugPathBlock,
					PathColor: a.cfg.HPADebugPathColor,
				}, a.logger)
				// Set visualizer on both pathfinder and builder
				if setter, ok := hpaPathfinder.(interface {
					SetDebugVisualizer(*pathfinding.HPADebugVisualizer)
				}); ok {
					setter.SetDebugVisualizer(debugViz)
				}
				hpaPF.GetBuilder().DebugViz = debugViz
			}

			a.logf("[Agent %s] HPA* PathFinder initialized with update handler (batch mode: auto-flush)", a.cfg.Name)
		} else {
			a.logf("[Agent %s] PathFinder initialized (HPA* without update handler)", a.cfg.Name)
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
					a.getFollowBotPosition,
					a.SendChat,
					followCfg,
					func() string { return a.cfg.Name },
					a.logger,
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
		// Disable for now - the connection also logs packets using the same writer.
		// if a.logw != nil {
		// 	a.client.Events().AddGeneric(a.packetLogger())
		// }

		// Optional: register ReplayMod recorder alongside logger
		if a.cfg.EnableReplay {
			out := a.cfg.ReplayOutput
			if out == "" {
				out = "session.mcpr"
			}

			ensureDirectory(a.logger, out)

			a.logf("[Agent %s] Initializing replay recorder (output: %s)", a.cfg.Name, out)

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
				a.moveMirror = NewReplayMovementMirror(rec, a.packetMgr, a.versionHandler, a.cfg.SkinProvider, a.logger)
				// If movement executor was configured earlier, wire its packet callback now.
				if a.moveMirror != nil && a.moveExec != nil {
					type packetCallbackSetter interface {
						SetPacketCallback(func(interface{}))
					}
					if setter, ok := a.moveExec.(packetCallbackSetter); ok {
						setter.SetPacketCallback(func(pkt any) {
							if pkPkt, okPkt := pkt.(pk.Packet); okPkt {
								a.moveMirror.HandleServerbound(pkPkt)
							}
						})
					}
					// Wire position snapshot recording directly from physics
					// tick to the replay mirror, bypassing the serverbound
					// packet interception chain for reliable auto-camera
					// keyframe generation during normal movement.
					type positionSnapshotRecorder interface {
						RecordPositionSnapshot(x, y, z float64, yaw, pitch float64)
					}
					type positionUpdateCallbackSetter interface {
						SetPositionUpdateCallback(func(x, y, z float64, yaw, pitch float64))
					}
					if recorder, ok := a.moveMirror.(positionSnapshotRecorder); ok {
						if setter, ok := a.moveExec.(positionUpdateCallbackSetter); ok {
							setter.SetPositionUpdateCallback(recorder.RecordPositionSnapshot)
						}
					}
				}
				// Pre-initialize entity type from protocol data (before login)
				// In Minecraft 1.21+, entity_type registry is not sent during configuration phase
				if a.moveMirror != nil && a.packetMgr != nil {
					playerEntityType := a.packetMgr.GetEntityTypeID("minecraft:player")
					if playerEntityType == -1 {
						a.logf("[ReplayMirror] WARNING: minecraft:player not found in protocol data, using fallback")
						playerEntityType = 148 // Fallback for safety (1.21.5 value)
					}
					a.logf("[ReplayMirror] Pre-initializing player entity type to %d (minecraft:player)", playerEntityType)
					a.moveMirror.SetEntityType(playerEntityType)
				}
				// Use bundle delimiter filtering to avoid recording unconsumed buffer data
				// Login phase packets (including Set Compression) are filtered at the bot client level
				// bundleDelimiterID := int32(a.packetMgr.GetClientboundPacketID("ClientboundBundleDelimiter"))
				// DISABLED: Duplicate recording - packets already recorded by mc-bot-go/bot/replay.go
				// a.client.Events().AddGeneric(bot.PacketHandler{Priority: 0, F: adapters.PacketFunc(rec, bundleDelimiterID)})
			} else {
				// If recorder setup fails, continue without recording
				a.logf("[Agent %s][WARN] Failed to initialize replay recorder: %v (replay recording disabled)", a.cfg.Name, err)
			}
		} else {
			a.logf("[Agent %s] Replay recording disabled", a.cfg.Name)
		}
	}

	return nil
}

func ensureDirectory(logger *slog.Logger, path string) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Warn("failed to create directory for path", "path", path, "error", err)
	}
}

func (a *agent) BlockShapeManager() models.BlockShapeManager {
	return a.shapeMgr
}

func (a *agent) Config() models.AgentConfig {
	return a.cfg
}

// Logger returns this agent's own fielded logger. See models.Agent.Logger's
// doc comment for the same contract.
func (a *agent) Logger() *slog.Logger {
	return a.log()
}

// Start connects to the server and begins background tasks.
func (a *agent) Start(ctx context.Context) error {
	a.lifecycleMu.RLock()
	baseCtx := a.ctx
	a.lifecycleMu.RUnlock()

	if baseCtx == nil {
		// Allow Start without explicit Init; initialize implicitly.
		if err := a.Init(ctx); err != nil {
			return err
		}
		a.lifecycleMu.RLock()
		baseCtx = a.ctx
		a.lifecycleMu.RUnlock()
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

		// Enable bidirectional packet logging for debugging
		opts.PacketLogWriter = a.packetLogWriter
		if err := a.client.JoinServerWithOptions(baseCtx, a.cfg.Address, opts); err != nil {
			return err
		}

		// Initialize container helper now that client is connected
		// (needs established connection to send packets)
		a.initializeContainerHelper()

		// After connection, initialize replay recording metadata
		// The LOGIN packet was received during JoinServerWithOptions, but handlers
		// registered via AddListener only fire during HandleGame. We need to set
		// entity metadata immediately so MovementMirror can emit synthetic packets.
		if a.rec != nil {
			// Set selfId to -1 to match ReplayMod's standard behavior
			// (all players visible, no special camera entity)
			a.rec.SetSelfID(-1)
			a.logf("[Agent %s][Replay] Set recorder selfId to -1", a.cfg.Name)
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
				a.logf("[Agent %s][Replay] Initialized MovementMirror with name=%s uuid=%s (entityID will be set later)",
					a.cfg.Name, a.cfg.Auth.Name, a.cfg.Auth.UUID)
			}
		}
	} else {
		return fmt.Errorf("no client available to connect to server")
	}

	// Start background cleanup goroutine for entity tracking
	a.startEntityCleanup(baseCtx.Done())

	// Start render loop for projectile position interpolation
	a.startRenderLoop(baseCtx.Done())

	// Start stop file watcher if configured
	if a.cfg.StopFilePath != "" {
		a.startStopFileWatcher(a.cfg.StopFilePath, baseCtx.Done())
	}

	// Start game handling loop if client supports it
	if a.client != nil {
		a.wg.Add(1)
		go func(ctx context.Context) {
			defer a.wg.Done()
			a.logf("[Agent %s] Game handling loop started", a.cfg.Name)
			for {
				select {
				case <-ctx.Done():
					a.logf("[Agent %s] Game handling loop exiting: context cancelled", a.cfg.Name)
					return
				default:
				}
				if err := a.client.HandleGame(ctx); err != nil {
					a.logf("[Agent %s] Game handling loop exiting: HandleGame returned error: %v", a.cfg.Name, err)
					// Cancel the agent's context to signal shutdown
					a.lifecycleMu.Lock()
					if a.cancel != nil {
						a.cancel()
					}
					a.lifecycleMu.Unlock()
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
		a.logf("[Agent %s] Starting continuous physics executor", a.cfg.Name)
		a.waitForGroundData(baseCtx, 5*time.Second)
		physicsExec.Start()
	} else {
		a.logf("[Agent %s] Starting position heartbeat (non-physics executor)", a.cfg.Name)
		a.startPositionHeartbeat(20)
	}

	a.startInitialPlan()

	return nil
}

func (a *agent) GetPacketLogWriter() io.Writer {
	return a.packetLogWriter
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
			a.logf("[Agent %s] Ground check timed out after %s; starting physics anyway", a.cfg.Name, timeout)
			return
		case <-ticker.C:
			pos, _, _, ok := a.GetPosition()
			if !ok {
				continue
			}
			x, y, z := pos.X, pos.Y, pos.Z

			// Log progress every second
			if time.Since(lastLogTime) >= 1*time.Second {
				a.logf("[Agent %s] Waiting for chunks at (%.1f, %.1f, %.1f)...", a.cfg.Name, x, y, z)
				lastLogTime = time.Now()
			}

			if a.hasLoadedGround(x, y, z) {
				elapsed := time.Since(startTime)
				a.logf("[Agent %s] Ground chunks loaded after %v", a.cfg.Name, elapsed)
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
	baseCacheDir, err := agentutils.FindOrCreateCacheDir()
	if err != nil {
		return fmt.Errorf("find cache directory: %w", err)
	}
	cacheDir := filepath.Join(baseCacheDir, "downloads")
	versionCacheDir := filepath.Join(cacheDir, a.cfg.Version)
	expectedReportsDir := filepath.Join(versionCacheDir, "data_generator")

	name := ""
	if a.client != nil {
		name = a.cfg.Name
	}

	// Check if reports already exist
	if info, err := os.Stat(expectedReportsDir); err == nil && info.IsDir() {
		a.logf("[Agent %s] Reports already exist at %s, skipping generation", name, expectedReportsDir)
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
	a.logf("[Agent %s] Generated reports base dir at %s", name, baseDir)
	return nil
}

// SetCriticalError sets a critical error and cancels the agent's context.
// Only the first error is retained; subsequent calls are ignored.
func (a *agent) SetCriticalError(err error) {
	a.criticalErrorMu.Lock()
	defer a.criticalErrorMu.Unlock()
	if a.criticalError == nil && err != nil {
		a.criticalError = err
		// Cancel the agent's context to signal shutdown
		a.lifecycleMu.Lock()
		if a.cancel != nil {
			a.cancel()
		}
		a.lifecycleMu.Unlock()
	}
}

// CriticalError returns any critical error that occurred during agent execution.
func (a *agent) CriticalError() error {
	a.criticalErrorMu.Lock()
	defer a.criticalErrorMu.Unlock()
	return a.criticalError
}

// Done returns a channel that's closed when the agent's internal context is cancelled.
// It returns nil if Init hasn't been called yet.
func (a *agent) Done() <-chan struct{} {
	a.lifecycleMu.RLock()
	defer a.lifecycleMu.RUnlock()
	if a.ctx == nil {
		return nil
	}
	return a.ctx.Done()
}

// Close gracefully shuts down the agent and its background tasks.
func (a *agent) Close(ctx context.Context) error {
	defer a.closeLogging()

	// Stop physics executor or position heartbeat before cancelling context
	if physicsExec, ok := a.moveExec.(*movement.PhysicsMovementExecutor); ok {
		a.logf("[Agent %s] Stopping continuous physics executor", a.cfg.Name)
		physicsExec.Stop()
	} else {
		a.stopPositionHeartbeat()
	}

	a.lifecycleMu.Lock()
	if a.cancel == nil {
		a.lifecycleMu.Unlock()
		return nil
	}
	a.cancel()
	a.cancel = nil
	a.lifecycleMu.Unlock()
	a.wg.Wait()
	if a.rec != nil {
		// Generate auto-camera timeline before closing the recorder.
		autoCamera := a.cfg.ReplayAutoCamera == nil || *a.cfg.ReplayAutoCamera
		if autoCamera {
			a.writeReplayTimeline()
		}

		a.logf("[Agent %s] [Replay] closing recorder", a.cfg.Name)
		// Use a goroutine with timeout to prevent indefinite hang on slow recorder close
		recCloseDone := make(chan error, 1)
		go func() {
			recCloseDone <- a.rec.Close()
		}()

		select {
		case err := <-recCloseDone:
			if err != nil {
				a.logf("[agent.replay] ERROR closing recorder: %v", err)
				a.rec = nil
				return fmt.Errorf("close recorder: %w", err)
			}
			a.logf("[Agent %s] [Replay] closed recorder successfully", a.cfg.Name)
			a.rec = nil
		case <-ctx.Done():
			// Timeout or cancellation while closing recorder
			a.logf("[Agent %s] [Replay] recorder close timeout/cancelled (context: %v), continuing shutdown", a.cfg.Name, ctx.Err())
			// Note: recorder may still be open/writing, but we can't wait indefinitely
			// The file descriptor will be closed when the process exits
			a.rec = nil
		}
	}
	return nil
}

// writeReplayTimeline generates and writes an auto-camera timelines.json into
// the MCPR archive. It is called from Close() before the recorder is finalized.
// Errors are logged but non-fatal — the replay is still valid without timelines.
func (a *agent) writeReplayTimeline() {
	type snapshotProvider interface {
		PositionSnapshots() ([]positionSnapshot, time.Time)
	}
	provider, ok := a.moveMirror.(snapshotProvider)
	if !ok || a.rec == nil {
		return
	}
	snapshots, startTime := provider.PositionSnapshots()
	if len(snapshots) < 2 {
		a.logf("[Agent %s] [Replay] skipping timeline generation (only %d snapshots)", a.cfg.Name, len(snapshots))
		return
	}
	data, err := generateTimelinesJSON(snapshots, startTime)
	if err != nil {
		a.logf("[Agent %s] [Replay] failed to generate timelines.json: %v", a.cfg.Name, err)
		return
	}
	if data == nil {
		return
	}
	if err := a.rec.WriteExtraEntry("timelines.json", data); err != nil {
		a.logf("[Agent %s] [Replay] failed to write timelines.json: %v", a.cfg.Name, err)
		return
	}
	a.logf("[Agent %s] [Replay] wrote auto-camera timelines.json (%d keyframes)", a.cfg.Name, len(snapshots))
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

// SetLastUpdateRecipes sets the update recipes payload (for testing).
func (a *agent) SetLastUpdateRecipes(payload *UpdateRecipesPayload) {
	a.recipesMu.Lock()
	a.lastUpdateRecipes = payload
	a.recipesMu.Unlock()
}

// SetTeleportAccepter injects a TeleportAccepter implementation (e.g., player subsystem).
func (a *agent) SetTeleportAccepter(t TeleportAccepter) {
	a.fallbackHandlersMu.Lock()
	a.teleport = t
	a.fallbackHandlersMu.Unlock()
}

// SetChat injects a Chat implementation.
func (a *agent) SetChat(c Chat) {
	a.fallbackHandlersMu.Lock()
	a.chat = c
	a.fallbackHandlersMu.Unlock()
}

// SetPlayerUUIDResolver injects a function to resolve player names to UUIDs.
func (a *agent) SetPlayerUUIDResolver(f func(string) ([16]byte, error)) {
	a.playerResolversMu.Lock()
	a.playerUUIDByName = f
	a.playerResolversMu.Unlock()
}

// UpdatePosition sets internal position; intended for movement executor wiring.
func (a *agent) UpdatePosition(pos models.V3, yaw, pitch float64) {
	a.setPosition(pos, yaw, pitch)
}

// SetFollowManager injects a follow manager implementation.
func (a *agent) SetFollowManager(f models.FollowManager) {
	a.followingMu.Lock()
	a.followMgr = f
	a.followingMu.Unlock()
}

// SetTargetSelector injects a TargetSelector for use by other subsystems.
func (a *agent) SetTargetSelector(ts models.TargetSelector) {
	a.followingMu.Lock()
	defer a.followingMu.Unlock()
	// No-op if the same selector is already set
	if a.targetSelector == ts {
		return
	}
	a.targetSelector = ts
}

// ResolvePlayerUUIDByName resolves a player UUID using the injected resolver.
func (a *agent) ResolvePlayerUUIDByName(name string) ([16]byte, error) {
	a.playerResolversMu.RLock()
	f := a.playerUUIDByName
	a.playerResolversMu.RUnlock()
	if f == nil {
		return [16]byte{}, models.ErrInvalidConfig("player UUID resolver not set")
	}
	return f(name)
}

// SetPlayerNameResolver injects resolver to map UUID->player name.
func (a *agent) SetPlayerNameResolver(f func([16]byte) (string, bool)) {
	a.playerResolversMu.Lock()
	a.playerNameByUUID = f
	a.playerResolversMu.Unlock()
}

// SetItemManager injects an ItemManager for item name lookup.
func (a *agent) SetItemManager(im ItemManager) {
	a.itemMgrMu.Lock()
	a.itemMgr = im
	a.itemMgrMu.Unlock()
}

func (a *agent) GetItemManager() ItemManager {
	a.itemMgrMu.RLock()
	defer a.itemMgrMu.RUnlock()
	return a.itemMgr
}

// SetSlotResolver injects a SlotResolver to inspect current slot contents.
func (a *agent) SetSlotResolver(sr SlotResolver) {
	a.slotsMu.Lock()
	a.slots = sr
	a.slotsMu.Unlock()
}

// Movement/pathfinding injection
func (a *agent) SetMovementExecutor(m models.MovementExecutor) {
	a.movementMu.Lock()
	defer a.movementMu.Unlock()
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
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()
	if a.moveExec == nil {
		return movement.UnknownExecutor
	}
	if _, ok := a.moveExec.(*movement.PhysicsMovementExecutor); ok {
		return movement.PhysicsExecutor
	}
	return movement.UnknownExecutor
}
func (a *agent) SetPathFinder(pf models.PathFinder) {
	a.movementMu.Lock()
	a.pathfind = pf
	a.movementMu.Unlock()
}

// SetTelemetryRecorder injects a telemetry recorder for movement testing.
// Only works with PhysicsMovementExecutor.
func (a *agent) SetTelemetryRecorder(recorder models.MovementTelemetryRecorder) {
	a.movementMu.Lock()
	defer a.movementMu.Unlock()

	// Set telemetry on physics executor
	a.moveExec.SetTelemetryRecorder(recorder)
}

// Manual movement passthrough methods

// EnterManualMode enables frame-by-frame physics simulation control.
// Requires the movement executor to support ManualMovementExecutor.
func (a *agent) EnterManualMode() error {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return fmt.Errorf("movement executor not available")
	}

	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor does not support manual mode")
	}

	return manual.EnterManualMode()
}

// ExitManualMode disables frame-by-frame physics simulation control.
func (a *agent) ExitManualMode() error {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return fmt.Errorf("movement executor not available")
	}

	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor does not support manual mode")
	}

	return manual.ExitManualMode()
}

// SetManualThrottle sets directional input for manual movement.
// westEastThrottle: positive = east, negative = west
// northSouthThrottle: positive = south, negative = north
func (a *agent) SetManualThrottle(westEastThrottle, northSouthThrottle float64) error {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return fmt.Errorf("movement executor not available")
	}

	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor does not support manual mode")
	}

	return manual.SetManualThrottle(westEastThrottle, northSouthThrottle)
}

// SetManualRotation sets yaw and pitch for manual control.
// Use math.NaN() to maintain current rotation without changing it.
func (a *agent) SetManualRotation(yaw, pitch float64) error {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return fmt.Errorf("movement executor not available")
	}

	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor does not support manual mode")
	}

	return manual.SetManualRotation(yaw, pitch)
}

// GetVelocity returns the agent's current physics velocity in blocks/tick.
// ok is false when no movement executor is active.
func (a *agent) GetVelocity() (x, y, z float64, ok bool) {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return 0, 0, 0, false
	}

	x, y, z = a.moveExec.GetVelocity()
	return x, y, z, true
}

// IsGliding returns whether elytra-gliding physics are currently active.
// Returns false when no movement executor is active.
func (a *agent) IsGliding() bool {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return false
	}

	return a.moveExec.IsGliding()
}

// SetManualJump sets whether the jump button is pressed for manual control.
// For camels, holding jump charges the dash; releasing fires the impulse.
func (a *agent) SetManualJump(enabled bool) error {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return fmt.Errorf("movement executor not available")
	}

	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor does not support manual mode")
	}

	return manual.SetManualJump(enabled)
}

// SetManualSprint sets whether the sprint button is pressed for manual control.
func (a *agent) SetManualSprint(enabled bool) error {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return fmt.Errorf("movement executor not available")
	}

	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor does not support manual mode")
	}

	return manual.SetManualSprint(enabled)
}

// SetManualSneak sets whether the sneak button is pressed for manual control.
func (a *agent) SetManualSneak(enabled bool) error {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()

	if a.moveExec == nil {
		return fmt.Errorf("movement executor not available")
	}

	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor does not support manual mode")
	}

	return manual.SetManualSneak(enabled)
}

// errors

var ErrAlreadyInitialized = errors.New("agent: already initialized")

// resolveVersionAndManagers handles version auto-detection and manager derivation.
// It ensures all version-related configuration is consistent and complete.
//
// Resolution order:
// 1. Auto-detect version from server if Config.Version is empty
// 2. Resolve ProtocolVersion from Version if not specified
// 3. Validate consistency between specified components
// 4. Derive managers (PacketMgr, VersionHandler, etc.) if not provided
//
// Returns true if version was auto-detected from server (caller should create client).
// initializeContainerHelper creates and initializes the container helper
// This is called from Start() AFTER the client connects (needs established connection)
func (a *agent) initializeContainerHelper() {
	if a.screenMgr == nil || a.client == nil {
		if a.screenMgr == nil {
			a.logf("[Agent %s] Warning: Screen manager unavailable, skipping container helper initialization", a.cfg.Name)
		}
		if a.client == nil {
			a.logf("[Agent] Warning: Bot client unavailable, skipping container helper initialization")
		}
		return
	}

	// Now that connection is established, we can safely use a.client.Conn()
	itemUsage := items.NewItemUsage(a.client.Conn(), a.packetMgr, a.logger)

	// Set version-specific handlers
	if a.versionHandler != nil {
		// Set container handler (for UseItemOn packets)
		containerHandler := a.versionHandler.Play().Containers()
		itemUsage.SetContainerHandler(containerHandler)
		a.logf("[Agent %s] Container handler initialized for version %s", a.cfg.Name, a.cfg.Version)

		// Set action handler (for swing/interact packets)
		actionHandler := a.versionHandler.Play().Actions()
		itemUsage.SetActionHandler(actionHandler)
		a.logf("[Agent %s] Action handler initialized for version %s", a.cfg.Name, a.cfg.Version)

		// Set entity handler (for UseItemOnEntity/entity interaction packets)
		entityHandler := a.versionHandler.Play().Entities()
		itemUsage.SetEntityHandler(entityHandler)
		a.logf("[Agent %s] Entity handler initialized for version %s", a.cfg.Name, a.cfg.Version)
	}

	// Cast screen manager to concrete type
	screenMgr, ok := a.screenMgr.(screen.Manager)
	if !ok {
		a.logf("[Agent %s] Warning: Screen manager type assertion failed, skipping container helper initialization", a.cfg.Name)
		return
	}

	// Create inventory manager with screen manager
	invMgr := items.NewInventoryManager(screenMgr)
	invMgr.SetWaitForUpdates(false)
	a.invMgr = invMgr

	// Create container helper with all required dependencies
	containerHelper := items.NewContainerHelper(itemUsage, invMgr, screenMgr, a.client, a.packetMgr)

	// Assign directly (containerHelper is internally synchronized)
	a.containerHelper = containerHelper
	if containerHelper != nil {
		containerHelper.SetEntityIDProvider(a)
		containerHelper.SetEntityTypeProvider(a.entityRegistry)
		containerHelper.SetMountStateProvider(a)

		// Set movement handler (for SendPlayerCommand packets)
		if a.versionHandler != nil {
			movementHandler := a.versionHandler.Play().Movement()
			containerHelper.SetMovementHandler(movementHandler)
			a.logf("[Agent %s] Movement handler initialized on container helper for version %s", a.cfg.Name, a.cfg.Version)
		}
	}
	a.logf("[Agent %s] Container helper automatically initialized after connection", a.cfg.Name)
}

func (a *agent) resolveVersionAndManagers() (versionAutoDetected bool, err error) {
	name := a.cfg.Name
	if name == "" {
		name = "Agent"
	}

	// Step 1: Auto-detect version from server if not specified
	versionAutoDetected = false
	if a.cfg.Version == "" {
		if a.cfg.Address == "" {
			return false, models.ErrInvalidConfig("cannot auto-detect version: Address not set")
		}
		detectedVersion, detectedProtocol, err := rof_utils.CheckServerVersion(a.cfg.Address, 0)
		if err != nil {
			return false, fmt.Errorf("auto-detect version from %s: %w", a.cfg.Address, err)
		}
		a.cfg.Version = detectedVersion
		a.cfg.ProtocolVersion = detectedProtocol
		versionAutoDetected = true
		a.logf("[%s] Auto-detected server version %s (protocol %d)", name, detectedVersion, detectedProtocol)
	}

	// Step 2: Resolve ProtocolVersion from Version if not specified
	if a.cfg.ProtocolVersion == 0 {
		if proto, ok := protocol_versions.VersionProtocol[a.cfg.Version]; ok {
			a.cfg.ProtocolVersion = proto
			a.logf("[%s] Resolved protocol %d for version %s", name, proto, a.cfg.Version)
		} else {
			return false, models.ErrInvalidConfig(fmt.Sprintf("unknown version %q: cannot resolve protocol version", a.cfg.Version))
		}
	} else {
		// Validate that specified ProtocolVersion matches Version
		if expectedProto, ok := protocol_versions.VersionProtocol[a.cfg.Version]; ok {
			if a.cfg.ProtocolVersion != expectedProto {
				return false, models.ErrInvalidConfig(fmt.Sprintf(
					"version/protocol mismatch: version %s expects protocol %d, but ProtocolVersion is %d",
					a.cfg.Version, expectedProto, a.cfg.ProtocolVersion))
			}
		}
	}

	// Step 3: Validate VersionHandler consistency if provided
	if a.cfg.VersionHandler != nil {
		handlerVersion := a.cfg.VersionHandler.Version()
		if handlerVersion != a.cfg.Version {
			return false, models.ErrInvalidConfig(fmt.Sprintf(
				"VersionHandler mismatch: handler is for %s, but Config.Version is %s",
				handlerVersion, a.cfg.Version))
		}
		a.logf("[%s] Using provided VersionHandler for %s", name, handlerVersion)
	}

	// Step 4: Derive PacketMgr if not provided
	if a.cfg.PacketMgr == nil {
		a.cfg.PacketMgr = protocol_versions.GetPacketMgrForVersion(a.cfg.Version)
		if a.cfg.PacketMgr == nil {
			return false, models.ErrInvalidConfig(fmt.Sprintf("no PacketMgr available for version %s", a.cfg.Version))
		}
		a.logf("[%s] Derived PacketMgr for version %s", name, a.cfg.Version)
	}
	a.packetMgr = a.cfg.PacketMgr

	// Step 5: Derive VersionHandler if not provided
	if a.cfg.VersionHandler == nil {
		vh, err := common.GetVersionHandler(a.cfg.Version, a.logger)
		if err != nil {
			// VersionHandler is required unless a Client is already provided
			// (for testing scenarios where mock clients don't need version-specific handling)
			if a.cfg.Client == nil {
				return false, fmt.Errorf("get VersionHandler for %s: %w", a.cfg.Version, err)
			}
			a.logf("[%s] Warning: no VersionHandler for %s (using provided client without version-specific handling)", name, a.cfg.Version)
		} else if vh == nil {
			if a.cfg.Client == nil {
				return false, models.ErrInvalidConfig(fmt.Sprintf(
					"no VersionHandler available for version %s (supported: %v)",
					a.cfg.Version, common.SupportedVersions()))
			}
			a.logf("[%s] Warning: no VersionHandler for %s (using provided client without version-specific handling)", name, a.cfg.Version)
		} else {
			a.cfg.VersionHandler = vh
			a.logf("[%s] Derived VersionHandler for version %s", name, a.cfg.Version)
		}
	}
	a.versionHandler = a.cfg.VersionHandler

	// Validate protocol version matches between handler and PacketMgr
	if a.versionHandler != nil && a.cfg.PacketMgr != nil {
		handlerVersion := a.versionHandler.ProtocolVersion()
		packetMgrVersion := a.cfg.PacketMgr.VersionProtocol()
		if uint64(handlerVersion) != packetMgrVersion {
			panic(fmt.Sprintf(
				"[%s] FATAL: Protocol version mismatch for %s: handler has %d but PacketMgr has %d. "+
					"This indicates a misconfigured version handler. Handler ProtocolVersion constant must match the protocol version used by mc-protocol-go.",
				name, a.versionHandler.Version(), handlerVersion, packetMgrVersion))
		}
		a.logf("[%s] Verified protocol version %d for %s", name, handlerVersion, a.versionHandler.Version())
	}

	// Step 6: Derive optional managers (SoundMgr, BlockMgr) - best effort
	if a.cfg.SoundMgr == nil {
		a.cfg.SoundMgr = protocol_versions.GetSoundMgrForVersion(a.cfg.Version)
		if a.cfg.SoundMgr != nil {
			a.logf("[%s] Derived SoundMgr for version %s", name, a.cfg.Version)
		}
	}
	a.soundMgr = a.cfg.SoundMgr

	if a.cfg.BlockMgr == nil {
		a.cfg.BlockMgr = protocol_versions.GetBlockMgrForVersion(a.cfg.Version)
		if a.cfg.BlockMgr != nil {
			a.logf("[%s] Derived BlockMgr for version %s", name, a.cfg.Version)
		}
	}
	a.blockMgr = a.cfg.BlockMgr

	return versionAutoDetected, nil
}

// getNextSequence returns and increments this agent's anti-cheat sequence
// number. Each action that requires a sequence number should call this to
// get the next value.
func (a *agent) getNextSequence() int32 {
	a.sequenceMu.Lock()
	defer a.sequenceMu.Unlock()
	if !a.sequenceInitialized {
		a.sequenceCounter = 0
		a.sequenceInitialized = true
	}
	result := a.sequenceCounter
	a.sequenceCounter++
	return result
}

// getRotation returns the player's current yaw and pitch
func (a *agent) getRotation() (float64, float64) {
	a.posMu.RLock()
	defer a.posMu.RUnlock()
	return a.posYaw, a.posPitch
}

func (a *agent) getEyeHeight() float64 {
	a.posMu.RLock()
	defer a.posMu.RUnlock()
	if a.moveExec.IsSneaking() {
		return models.PlayerEyeHeightSneaking
	} else {
		return models.PlayerEyeHeight
	}
}

// RegisterEntityPositionCallback registers a callback to be called when entity positions update.
// Useful for testing and monitoring entity movement.
func (a *agent) RegisterEntityPositionCallback(cb models.EntityPositionCallback) {
	a.entityPosCallbacksMu.Lock()
	defer a.entityPosCallbacksMu.Unlock()
	a.entityPosCallbacks = append(a.entityPosCallbacks, cb)
}

// callEntityPositionCallbacks calls all registered entity position callbacks.
func (a *agent) callEntityPositionCallbacks(entityID int32, x, y, z float64) {
	a.entityPosCallbacksMu.RLock()
	callbacks := a.entityPosCallbacks
	a.entityPosCallbacksMu.RUnlock()

	if len(callbacks) > 0 {
		a.logf("[callEntityPositionCallbacks] Calling %d callbacks for entity %d at (%.2f, %.2f, %.2f)", len(callbacks), entityID, x, y, z)
	}
	for _, cb := range callbacks {
		cb(entityID, x, y, z)
	}
}

// RegisterPluginMessageCallback registers a callback to be called for every
// inbound CustomPayload (plugin message) packet, on any channel. Implements
// models.PluginMessaging.
func (a *agent) RegisterPluginMessageCallback(cb models.PluginMessageCallback) {
	a.pluginMsgCallbacksMu.Lock()
	defer a.pluginMsgCallbacksMu.Unlock()
	a.pluginMsgCallbacks = append(a.pluginMsgCallbacks, cb)
}

// callPluginMessageCallbacks calls all registered plugin-message callbacks.
func (a *agent) callPluginMessageCallbacks(channel string, data []byte) {
	a.pluginMsgCallbacksMu.RLock()
	callbacks := a.pluginMsgCallbacks
	a.pluginMsgCallbacksMu.RUnlock()

	for _, cb := range callbacks {
		cb(channel, data)
	}
}

// SendPluginMessage sends a serverbound CustomPayload (plugin message) packet
// on the given channel with raw, already-encoded payload bytes. Implements
// models.PluginMessaging.
func (a *agent) SendPluginMessage(channel string, data []byte) error {
	// versionHandler and client are no-lock fields (set-once in Init, read-only after)
	vh := a.versionHandler
	c := a.client
	if vh == nil || c == nil {
		return fmt.Errorf("SendPluginMessage: not connected")
	}
	conn := c.Conn()
	if conn == nil {
		return fmt.Errorf("SendPluginMessage: not connected")
	}
	return vh.Play().SendCustomPayloadRaw(conn, channel, data)
}

// getBotPositionLegacy returns bot position in legacy format for movement executor compatibility
func (a *agent) getBotPositionLegacy() (models.V3, float64, float64, bool) {
	return a.GetPosition()
}

// getFollowBotPosition returns bot position for follow manager
func (a *agent) getFollowBotPosition() (models.V3, float64, float64, bool) {
	pos, yaw, pitch, initialized := a.GetPosition()
	return pos, yaw, pitch, initialized
}

// String returns a human-friendly description for logging.
func (a *agent) String() string {
	return fmt.Sprintf("agent{addr=%s ver=%s}", a.cfg.Address, a.cfg.Version)
}
