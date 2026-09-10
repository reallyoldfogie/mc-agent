package models

import (
	"context"
	"io"
	"log/slog"
)

// Agent is the primary interface for bot control and lifecycle management.
type Agent interface {
	AgentHandlers
	AgentActions
	CommandAgent
	PlanAgent
	InventoryManager
	ContainerOperations
	ChatOperations
	ScreenOperations
	WorldOperations
	ManualMovement
	EntityCallbackRegistry
	MountState
	PluginMessaging
	ActionRegistrar

	// Lifecycle
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	Close(ctx context.Context) error
	Done() <-chan struct{}
	SetCriticalError(err error)
	CriticalError() error

	// Logger returns this agent's own fielded logger. Every line it writes
	// is tagged with this agent's identity, so callers driving multiple
	// concurrent agents (e.g. the RL environment or LLM executor) can log
	// through it instead of a shared package-level logger.
	Logger() *slog.Logger

	// Recipes (Update Recipes packet)
	LastUpdateRecipes() (UpdateRecipesPayload, bool)
	SetLastUpdateRecipes(payload *UpdateRecipesPayload)
	ExportLastUpdateRecipesAsJSON(indent bool) (string, bool, error)

	// Dependency injection
	SetTeleportAccepter(t TeleportAccepter)
	SetChat(c Chat)
	SetPlayerUUIDResolver(f func(string) ([16]byte, error))
	SetPlayerNameResolver(f func([16]byte) (string, bool))
	SetItemManager(im ItemManager)
	GetItemManager() ItemManager
	SetSlotResolver(sr SlotResolver)

	// Movement/pathfinding injection
	SetMovementExecutor(m MovementExecutor)
	SetPathFinder(pf PathFinder)
	SetTelemetryRecorder(recorder MovementTelemetryRecorder)

	GetEntityID() int32
	GetTrackedEntitiesForFollowing() map[int32]*TrackedEntity
	GetTrackedEntities() map[int32]TrackedEntityInfo
	// FindNearestEntityByType finds the nearest entity of a given type to a
	// position. honorPerceptionEffects, when true, additionally excludes
	// candidates beyond the agent's own effective vision range under
	// Blindness/Darkness (see physics.PerceptionRadiusCap) — real
	// agent decision-making that should behave as if the effect matters
	// passes true; callers that need deterministic results regardless of
	// incidental effect state (e.g. test setup locating a just-spawned
	// entity) pass false.
	FindNearestEntityByType(entityType int32, x, y, z float64, honorPerceptionEffects bool) (entityID int32, distance float64, found bool)

	// Registry access (version-agnostic lookups)
	GetRegistry(id string) CustomRegistry
	LoadRegistriesFromFile(dataPath string) error

	// Player UUID resolution
	ResolvePlayerUUIDByName(name string) ([16]byte, error)

	// Follow manager injection
	SetFollowManager(f FollowManager)

	// SetTargetSelector injects a TargetSelector for use by other subsystems.
	SetTargetSelector(ts TargetSelector)

	// Plan execution
	StartPlan(plan Plan) error
	PlanEvents() <-chan PlanEvent

	RegistryItemManager() ItemManager

	// String returns a human-friendly description for logging.
	String() string

	GetPacketLogWriter() io.Writer //TODO: use to log start/stop of projectiles

	Config() AgentConfig
	BlockShapeManager() BlockShapeManager
}
