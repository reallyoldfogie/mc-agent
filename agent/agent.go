package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/pathfinding"
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

// Agent is the top-level orchestrator of bot/client, state, and event wiring.
type Agent struct {
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
	followMgr interface {
		IsActive() bool
		Start(string) error
		Stop() error
		GetStatus() string
	}

	// tracking loop state (for legacy parity)
	trackMu          sync.Mutex
	trackActive      bool
	trackStop        chan struct{}
	lastNoPlayersMsg time.Time
	trackWG          sync.WaitGroup

	// player name/uuid resolvers
	nameByUUID func([16]byte) (string, bool)

	// replay
	rec *recorder.Recorder
}

// New constructs an Agent with the provided configuration.
func New(cfg Config) (*Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	a := &Agent{cfg: cfg, logw: cfg.LogWriter}
	return a, nil
}

// Init prepares dependencies, managers and event wiring but does not connect.
func (a *Agent) Init(ctx context.Context) error {
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
		for _, h := range a.coreHandlers() {
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
				a.client.Events().AddGeneric(PacketHandler{Priority: 0, F: adapters.PacketFunc(rec)})
			} else {
				// If recorder setup fails, continue without recording
			}
		}
	}

	return nil
}

// Start connects to the server and begins background tasks.
func (a *Agent) Start(ctx context.Context) error {
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
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if err := a.client.HandleGame(ctx); err != nil {
					// Stop on disconnect or error; in future we can classify errors
					return
				}
			}
		}(baseCtx)
	}

	return nil
}

// Done returns a channel that's closed when the agent's internal context is cancelled.
// It returns nil if Init hasn't been called yet.
func (a *Agent) Done() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx == nil {
		return nil
	}
	return a.ctx.Done()
}

// Close gracefully shuts down the agent and its background tasks.
func (a *Agent) Close(context.Context) error {
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
		_ = a.rec.Close()
		log.Printf("[agent.replay] closed recorder")
		a.rec = nil
	}
	return nil
}

// SetTeleportAccepter injects a TeleportAccepter implementation (e.g., player subsystem).
func (a *Agent) SetTeleportAccepter(t TeleportAccepter) { a.mu.Lock(); a.teleport = t; a.mu.Unlock() }

// SetChat injects a Chat implementation.
func (a *Agent) SetChat(c Chat) { a.mu.Lock(); a.chat = c; a.mu.Unlock() }

// SetPlayerUUIDResolver injects a function to resolve player names to UUIDs.
func (a *Agent) SetPlayerUUIDResolver(f func(string) ([16]byte, error)) {
	a.mu.Lock()
	a.playerUUIDByName = f
	a.mu.Unlock()
}

// UpdatePosition sets internal position; intended for movement executor wiring.
func (a *Agent) UpdatePosition(x, y, z float64, yaw, pitch float32) {
	a.setPosition(x, y, z, yaw, pitch)
}

// SetFollowManager injects a follow manager implementation.
func (a *Agent) SetFollowManager(f interface {
	IsActive() bool
	Start(string) error
	Stop() error
	GetStatus() string
}) {
	a.mu.Lock()
	a.followMgr = f
	a.mu.Unlock()
}

// ResolvePlayerUUIDByName resolves a player UUID using the injected resolver.
func (a *Agent) ResolvePlayerUUIDByName(name string) ([16]byte, error) {
	a.mu.Lock()
	f := a.playerUUIDByName
	a.mu.Unlock()
	if f == nil {
		return [16]byte{}, ErrInvalidConfig("player UUID resolver not set")
	}
	return f(name)
}

// SetPlayerNameResolver injects resolver to map UUID->player name.
func (a *Agent) SetPlayerNameResolver(f func([16]byte) (string, bool)) {
	a.mu.Lock()
	a.nameByUUID = f
	a.mu.Unlock()
}

// SetItemManager injects an ItemManager for item name lookup.
func (a *Agent) SetItemManager(im ItemManager) { a.mu.Lock(); a.itemMgr = im; a.mu.Unlock() }

// SetSlotResolver injects a SlotResolver to inspect current slot contents.
func (a *Agent) SetSlotResolver(sr SlotResolver) { a.mu.Lock(); a.slots = sr; a.mu.Unlock() }

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

func (a *Agent) SetMovementExecutor(m movementExec) { a.mu.Lock(); a.moveExec = m; a.mu.Unlock() }
func (a *Agent) SetPathFinder(pf pathFinder)        { a.mu.Lock(); a.pathfind = pf; a.mu.Unlock() }

// Errors
type configError string

func (e configError) Error() string     { return string(e) }
func ErrInvalidConfig(msg string) error { return configError(msg) }

var ErrAlreadyInitialized = errors.New("agent: already initialized")

// String returns a human-friendly description for logging.
func (a *Agent) String() string {
	return fmt.Sprintf("Agent{addr=%s ver=%s}", a.cfg.Address, a.cfg.Version)
}
