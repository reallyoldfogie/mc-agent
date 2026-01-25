package models

import (
	"context"

	pk "github.com/Tnze/go-mc/net/packet"
)

// Agent is the primary interface for bot control and lifecycle management.
type Agent interface {
	AgentHandlers
	AgentActions
	ContainerOperations
	ChatOperations
	ScreenOperations
	WorldOperations

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
	SetMovementExecutor(m MovementExecutor)
	SetPathFinder(pf PathFinder)
	SetTelemetryRecorder(recorder MovementTelemetryRecorder)

	// Position update (for movement executor wiring)
	UpdatePosition(x, y, z float64, yaw, pitch float32)
	GetPosition() (x, y, z float64, yaw, pitch float32, initialized bool)
	GetPositionSimple() (x, y, z float64, initialized bool)

	GetEntityID() int32
	GetTrackedEntitiesForFollowing() map[int32]*TrackedEntity
	GetTrackedEntities() map[int32]TrackedEntityInfo
	FindNearestEntityByType(entityType int32, x, y, z float64) (entityID int32, distance float64, found bool)

	// Registry access (version-agnostic lookups)
	GetRegistry(id string) CustomRegistry
	GetEntityTypeID(entityName string) (int32, bool)
	LoadEntityTypesFromRegistry(dataPath string) error

	// Player UUID resolution
	ResolvePlayerUUIDByName(name string) ([16]byte, error)

	// Follow manager injection
	SetFollowManager(f FollowManager)

	// SetTargetSelector injects a TargetSelector for use by other subsystems.
	SetTargetSelector(ts TargetSelector)

	SendChat(string) error

	// Plan execution
	StartPlan(plan Plan) error
	StopPlan() error
	PlanStatus() PlanStatus
	PlanEvents() <-chan PlanEvent

	RegistryItemManager() ItemManager

	// String returns a human-friendly description for logging.
	String() string
}
