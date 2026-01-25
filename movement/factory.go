package movement

import (
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// ExecutorType specifies which movement executor implementation to use.
type ExecutorType int

const (
	// InterpolationExecutor uses simple position interpolation (default, legacy behavior)
	InterpolationExecutor ExecutorType = iota
	// PhysicsExecutor uses realistic physics simulation with inputs
	PhysicsExecutor
)

// String returns the name of the executor type.
func (et ExecutorType) String() string {
	switch et {
	case InterpolationExecutor:
		return "Interpolation"
	case PhysicsExecutor:
		return "Physics"
	default:
		return "Unknown"
	}
}

// ExecutorConfig holds configuration for creating a movement executor.
type ExecutorConfig struct {
	// Required for all executor types
	Client         bot.Client
	PacketMgr      protocol_models.PacketMgr
	GetBotPos      func() (float64, float64, float64, float32, float32, bool)
	SetBotPos      func(float64, float64, float64, float32, float32)
	GetBotEntityID func() int32

	// Required only for PhysicsExecutor
	World         physics.World
	ShapeProvider physics.BlockShapeProvider
}

// NewExecutor creates a movement executor of the specified type.
// For PhysicsExecutor, config.World and config.ShapeProvider must be provided.
func NewExecutor(executorType ExecutorType, config ExecutorConfig) MovementExecutor {
	// switch executorType {
	// case InterpolationExecutor:
	// 	return NewMovementExecutor(
	// 		config.Client,
	// 		config.PacketMgr,
	// 		config.GetBotPos,
	// 		config.SetBotPos,
	// 		config.GetBotEntityID,
	// 	)

	// case PhysicsExecutor:
	return NewPhysicsMovementExecutor(
		config.Client,
		config.PacketMgr,
		config.GetBotPos,
		config.SetBotPos,
		config.GetBotEntityID,
		config.World,
		config.ShapeProvider,
	)

	// default:
	// 	// Default to interpolation executor
	// 	return NewMovementExecutor(
	// 		config.Client,
	// 		config.PacketMgr,
	// 		config.GetBotPos,
	// 		config.SetBotPos,
	// 		config.GetBotEntityID,
	// 	)
	// }
}
