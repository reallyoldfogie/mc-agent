package models

import (
	"context"
	"io"
)

// Agent is the primary interface for bot control and lifecycle management.
type Agent interface {
	AgentHandlers
	AgentActions
	CommandAgent
	PlanAgent
	ContainerOperations
	ChatOperations
	ScreenOperations
	WorldOperations
	ManualMovement
	EntityCallbackRegistry

	// Lifecycle
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	Close(ctx context.Context) error
	Done() <-chan struct{}
	SetCriticalError(err error)
	CriticalError() error

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

	// Position update (for movement executor wiring)
	UpdatePosition(x, y, z float64, yaw, pitch float64)
	GetPosition() (x, y, z float64, yaw, pitch float64, initialized bool)

	GetEntityID() int32
	GetTrackedEntitiesForFollowing() map[int32]*TrackedEntity
	GetTrackedEntities() map[int32]TrackedEntityInfo
	FindNearestEntityByType(entityType int32, x, y, z float64) (entityID int32, distance float64, found bool)

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
