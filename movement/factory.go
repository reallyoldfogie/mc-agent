package movement

import (
	"context"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// ExecutorType specifies which movement executor implementation to use.
type ExecutorType int

const (
	UnknownExecutor ExecutorType = iota
	// PhysicsExecutor uses realistic physics simulation with inputs
	PhysicsExecutor
)

// String returns the name of the executor type.
func (et ExecutorType) String() string {
	switch et {
	case PhysicsExecutor:
		return "Physics"
	case UnknownExecutor:
		fallthrough
	default:
		return "Unknown"
	}
}

// ExecutorConfig holds configuration for creating a movement executor.
type ExecutorConfig struct {
	// Required for all executor types
	Client         bot.Client
	PacketMgr      protocol_models.PacketMgr
	GetBotPos      func() (models.V3, float64, float64, bool)
	SetBotPos      func(models.V3, float64, float64)
	GetBotEntityID func() int32
	Ctx            context.Context

	// Required only for PhysicsExecutor
	World          physics.World
	ShapeProvider  physics.BlockShapeProvider
	WorldManager   models.World // For accessing world time and other state
}

// NewExecutor creates a movement executor of the specified type.
// For PhysicsExecutor, config.World, config.ShapeProvider, and config.WorldManager must be provided.
func NewExecutor(executorType ExecutorType, config ExecutorConfig) MovementExecutor {
	return NewPhysicsMovementExecutor(
		config.Ctx,
		config.Client,
		config.PacketMgr,
		config.GetBotPos,
		config.SetBotPos,
		config.GetBotEntityID,
		config.World,
		config.ShapeProvider,
		config.WorldManager,
	)
}
