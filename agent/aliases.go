package agent

import "github.com/reallyoldfogie/mc-agent/models"

// Type aliases to models to keep agent package usage stable.
type (
	// Agent          = models.Agent
	AgentHandlers  = models.AgentHandlers
	Chat           = models.Chat
	ChatOperations = models.ChatOperations
	// Client              = models.Client
	ContainerAccess     = models.ContainerAccess
	ContainerHelper     = models.ContainerHelper
	ContainerOperations = models.ContainerOperations
	CustomRegistry      = models.CustomRegistry
	EntityIDProvider    = models.EntityIDProvider
	// EventBus            = models.EventBus
	ItemManager = models.ItemManager
	// JoinOptions         = models.JoinOptions
	// MovementExecutor = models.MovementExecutor
	MovementMirror = models.MovementMirror
	MovementAgent  = models.MovementAgent
	PacketBuilder  = models.PacketBuilder
	// PacketHandler       = models.PacketHandler
	PacketManager  = models.PacketManager
	PacketRecorder = models.PacketRecorder
	// PlayerList       = models.PlayerList
	Respawner        = models.Respawner
	Screen           = models.Screen
	ScreenAccess     = models.ScreenAccess
	ScreenOperations = models.ScreenOperations
	// ScreenSubsystem  = models.ScreenSubsystem
	// SkinProvider        = models.SkinProvider
	Slot             = models.Slot
	SlotResolver     = models.SlotResolver
	TargetSelector   = models.TargetSelector
	TeleportAccepter = models.TeleportAccepter
	TrackedEntity    = models.TrackedEntity
	// World            = models.World
	WorldManager    = models.WorldManager
	WorldOperations = models.WorldOperations
)
