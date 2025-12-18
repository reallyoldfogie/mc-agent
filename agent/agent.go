package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/Tnze/go-mc/chat"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
	"github.com/reallyoldfogie/mc-agent/following"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
	"github.com/reallyoldfogie/mc-bot-go/bot/world"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"

	"github.com/reallyoldfogie/mc-replay-go/adapters"
	mcprpkg "github.com/reallyoldfogie/mc-replay-go/mcpr"
	"github.com/reallyoldfogie/mc-replay-go/mcpr/recorder"
)

// Entity cleanup tuning (can be adjusted later or surfaced via Config if needed).
const (
	EntityRemovalGracePeriod = 30 * time.Second
	EntityCleanupInterval    = 10 * time.Second
)

type Agent interface {
	AgentHandlers
	// Lifecycle
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	Close(ctx context.Context) error
	Done() <-chan struct{}

	// Recipes (Update Recipes packet)
	LastUpdateRecipes() (UpdateRecipesPayload, bool)
	ExportLastUpdateRecipesAsJSON(indent bool) (string, bool, error)
	ParseUpdateRecipesPacket(p pk.Packet) error

	// Dependency injection
	SetTeleportAccepter(t TeleportAccepter)
	SetChat(c Chat)
	SetPlayerUUIDResolver(f func(string) ([16]byte, error))
	SetPlayerNameResolver(f func([16]byte) (string, bool))
	SetItemManager(im ItemManager)
	SetSlotResolver(sr SlotResolver)

	// Movement/pathfinding injection
	SetMovementExecutor(m movementExec)
	SetPathFinder(pf pathFinder)

	// Position update (for movement executor wiring)
	UpdatePosition(x, y, z float64, yaw, pitch float32)
	GetPosition() (x, y, z float64, yaw, pitch float32, initialized bool)
	GetPositionSimple() (x, y, z float64, initialized bool)

	GetEntityID() int32
	GetTrackedEntitiesForFollowing() map[int32]*following.TrackedEntity
	// SetEntityID(id int32)
	// GetEntityByID(id int32) (*trackedEntity, bool)
	// GetAllEntities() map[int32]*trackedEntity

	// Player UUID resolution
	ResolvePlayerUUIDByName(name string) ([16]byte, error)

	// Follow manager injection
	SetFollowManager(f following.FollowManager)

	// SetTargetSelector injects a TargetSelector for use by other subsystems.
	SetTargetSelector(ts following.TargetSelector)

	SendChat(string) error

	// String returns a human-friendly description for logging.
	String() string
}

type AgentHandlers interface {
	HandleGameStart() error
	HandleDisconnect(reason chat.Message) error
	HandleHealthChange(health float32, food int32, saturation float32) error
	HandleDeath() error
	HandleTeleported(x, y, z float64, yaw, pitch float32, _ byte, teleportID int32) error
	HandleChunkLoad(world.ChunkPos) error
	HandleChunkUnload(world.ChunkPos) error
	OnSystemChat(message chat.Message, overlay bool) error
	OnPlayerChat(playerlist.PlayerInfo, chat.Message, bool) error
	OnDisguisedChat(chat.Message) error
	OnScreenSlotChange(id, index int) error
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
	client    Client
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

	// optional subsystems
	teleport TeleportAccepter
	chat     Chat
	moveExec movementExec
	pathfind pathFinder
	itemMgr  ItemManager
	slots    SlotResolver

	// helpers
	playerUUIDByName func(string) ([16]byte, error)

	// behaviors
	followMgr      following.FollowManager
	targetSelector following.TargetSelector

	// tracking loop state (for legacy parity)
	trackMu          sync.Mutex
	trackActive      bool
	trackStop        chan struct{}
	lastNoPlayersMsg time.Time
	// trackWG          sync.WaitGroup

	// player name/uuid resolvers
	nameByUUID func([16]byte) (string, bool)

	// replay
	rec *recorder.Recorder

	// recipes (last Update Recipes payload)
	recipesMu         sync.RWMutex
	lastUpdateRecipes *UpdateRecipesPayload
}

// New constructs an agent with the provided configuration.
func New(cfg Config) (Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	a := &agent{cfg: cfg, logw: cfg.LogWriter}
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

	// TODO: create the real client and subsystems when ported.
	// For now this is a placeholder; during migration we'll inject or construct concrete implementations.

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
				fileFormatVersion = mcprpkg.CurrentFileFormatVersion
			}
			gen := a.cfg.ReplayGenerator
			if gen == "" {
				gen = "mc-agent"
			}
			rec, err := recorder.NewFile(out, mcprpkg.Meta{
				Protocol:          int(a.cfg.ProtocolVersion),
				MCVersion:         a.cfg.Version,
				FileFormatVersion: fileFormatVersion,
				Generator:         gen,
				ServerName:        a.cfg.Address,
			})
			if err == nil {
				a.rec = rec
				a.moveMirror = NewReplayMovementMirror(rec, a.packetMgr, a.cfg.SkinProvider)
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
				a.client.Events().AddGeneric(PacketHandler{Priority: 0, F: adapters.PacketFunc(rec, bundleDelimiterID)})
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
		opts := JoinOptions{ProtocolVersion: a.cfg.ProtocolVersion}
		if a.rec != nil {
			opts.ReplayRecorder = a.rec
			opts.MovementMirror = a.moveMirror
			opts.SkinProvider = a.cfg.SkinProvider
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
			log.Printf("[Replay] Set recorder selfId to -1")
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
				log.Printf("[Replay] Initialized MovementMirror with name=%s uuid=%s (entityID will be set later)",
					a.cfg.Auth.Name, a.cfg.Auth.UUID)
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
			log.Printf("[agent] Game handling loop started")
			for {
				select {
				case <-ctx.Done():
					log.Printf("[agent] Game handling loop exiting: context cancelled")
					return
				default:
				}
				if err := a.client.HandleGame(ctx); err != nil {
					log.Printf("[agent] Game handling loop exiting: HandleGame returned error: %v", err)
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
		log.Printf("[agent.replay] closing recorder")
		if err := a.rec.Close(); err != nil {
			log.Printf("[agent.replay] ERROR closing recorder: %v", err)
			return fmt.Errorf("close recorder: %w", err)
		}
		log.Printf("[agent.replay] closed recorder successfully")
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
func (a *agent) SetFollowManager(f following.FollowManager) {
	a.mu.Lock()
	a.followMgr = f
	a.mu.Unlock()
}

// SetTargetSelector injects a TargetSelector for use by other subsystems.
func (a *agent) SetTargetSelector(ts following.TargetSelector) {
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
	a.nameByUUID = f
	a.mu.Unlock()
}

// SetItemManager injects an ItemManager for item name lookup.
func (a *agent) SetItemManager(im ItemManager) { a.mu.Lock(); a.itemMgr = im; a.mu.Unlock() }

// SetSlotResolver injects a SlotResolver to inspect current slot contents.
func (a *agent) SetSlotResolver(sr SlotResolver) { a.mu.Lock(); a.slots = sr; a.mu.Unlock() }

// Movement/pathfinding injection
type movementExec interface {
	SendPosition(x, y, z float64, onGround bool) error
	SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error
	SendRotation(yaw, pitch float32, onGround bool) error
	MoveTowards(targetX, targetY, targetZ float64, distance float64, onGround bool) (newX, newY, newZ float64, err error)
	LookAt(targetX, targetY, targetZ float64, onGround bool) error
}

type pathFinder interface {
	FindPath(start, goal pathfinding.V3, maxSteps int) (*pathfinding.Path, error)
}

func (a *agent) SetMovementExecutor(m movementExec) {
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
func (a *agent) SetPathFinder(pf pathFinder) { a.mu.Lock(); a.pathfind = pf; a.mu.Unlock() }

// Errors
type configError string

func (e configError) Error() string     { return string(e) }
func ErrInvalidConfig(msg string) error { return configError(msg) }

var ErrAlreadyInitialized = errors.New("agent: already initialized")

// String returns a human-friendly description for logging.
func (a *agent) String() string {
	return fmt.Sprintf("agent{addr=%s ver=%s}", a.cfg.Address, a.cfg.Version)
}
