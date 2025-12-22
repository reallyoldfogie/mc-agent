package pathfinding

import (
	"math"
	"math/rand"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// PhysicsState is an alias to models.PhysicsState for convenience.
type PhysicsState = models.PhysicsState

// Inputs is an alias to models.Inputs for convenience.
type Inputs = models.Inputs

// InputGenerator converts path steps to physics inputs.
// This is the bridge between high-level pathfinding (discrete blocks)
// and low-level physics simulation (continuous movement with inputs).
type InputGenerator interface {
	// GenerateInputs converts a path step to player inputs for physics simulation.
	// currentState: Current physics state (position, velocity, etc.)
	// targetStep: The path step we're trying to reach
	// runTime: How long we've been executing this step
	GenerateInputs(currentState PhysicsState, targetStep PathStep, runTime time.Duration) Inputs

	// EstimateTicksRequired estimates how many ticks to complete a movement step.
	// This is used for path execution planning.
	EstimateTicksRequired(step PathStep, currentState PhysicsState) int
}

// DefaultInputGenerator is the standard implementation of InputGenerator.
// It uses vanilla Minecraft movement physics and timing.
type DefaultInputGenerator struct {
	// No configuration needed for now
}

// NewInputGenerator creates a new default input generator.
func NewInputGenerator() InputGenerator {
	return &DefaultInputGenerator{}
}

// GenerateInputs implements the InputGenerator interface.
// Based on phys archive bot/path/path.go:107-191 (Tile.Inputs() logic).
func (ig *DefaultInputGenerator) GenerateInputs(
	currentState PhysicsState,
	targetStep PathStep,
	runTime time.Duration,
) Inputs {
	// Get current position, rotation, and velocity
	pos, yaw, pitch, _ := currentState.GetPosition()
	vel := currentState.GetVelocity()

	// Calculate delta from current position to target
	deltaPos := V3Sub(targetStep.Position, pos)

	// Calculate desired yaw based on movement direction
	// atan2(-deltaPos.X, -deltaPos.Z) gives the direction to the target
	at := math.Atan2(-deltaPos.X, -deltaPos.Z)

	// Default inputs: throttle toward target, maintain current yaw
	out := Inputs{
		ThrottleX: math.Sin(at),
		ThrottleZ: math.Cos(at),
		Yaw:       yaw,
		Pitch:     pitch,
		Jump:      false,
		Sprint:    false, // TODO: Add sprint support based on movement type
		Sneak:     false, // TODO: Add sneak support for sneak movement types
	}

	// Add small random pitch variations (simulates natural head movement)
	if (rand.Int() % 14) == 0 {
		out.Pitch = pitch + float64((rand.Int()%4)-2)
	}

	// Movement-specific input adjustments
	switch targetStep.Movement {
	case Traverse, DiagonalTraverse:
		// Simple horizontal movement - default inputs are fine
		// No special logic needed

	case Ascend, DiagonalAscend:
		// Jump up one block
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		// Jump when we're close to the target and below it
		// dist2 < 1.75: Within jump range
		// deltaPos.Y < -0.81: Significantly below target (need to jump up)
		out.Jump = dist2 < 1.75 && deltaPos.Y < -0.81

		// Special handling for stairs/slabs would go here
		// For now, we'll use the simple jump logic
		// TODO: Detect stairs/slabs and use optimized stair-climbing logic

		// Turn off throttle if we're stuck (close, below target, no vertical velocity)
		if dist2 < 1 && deltaPos.Y < 0 && vel.Y == 0 {
			out.ThrottleX, out.ThrottleZ = 0, 0
		}

	case Descend:
		// Drop down (1-3 blocks)
		// Just walk forward and let gravity do the work
		// Default inputs are fine

	case Jump2:
		// Jump across 2-block gap
		// Timing is critical: jump when at specific distance
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		// Jump window: 1.5 to 1.78 blocks from target
		// This gives the optimal jump trajectory
		out.Jump = dist2 > 1.5 && dist2 < 1.78

		// TODO: Add sprint for longer gaps

	case Climb:
		// Climb ladder/vine
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		// If far from ladder and below target, approach the ladder first
		if dist2 > (0.8*0.8) && deltaPos.Y < 0 {
			// Approach phase: aim for the ladder block center
			// No special throttle adjustment needed
		} else {
			// Climbing phase: center on ladder
			// Aim for the center of the target block
			ladderCenterX := targetStep.Position.X + 0.5
			ladderCenterZ := targetStep.Position.Z + 0.5

			at = math.Atan2(-pos.X+ladderCenterX, -pos.Z+ladderCenterZ)
			out.ThrottleX = math.Sin(at)
			out.ThrottleZ = math.Cos(at)
		}

	case Swim:
		// Swimming in water (horizontal)
		// Similar to traverse but slower
		// Default inputs work, physics will handle water resistance

	case SwimUp:
		// Swimming up in water
		out.Jump = true // Jump button makes you swim up

	case SwimDown:
		// Swimming down in water
		// Sneak button makes you swim down
		out.Sneak = true
	}

	return out
}

// EstimateTicksRequired estimates ticks needed to complete a movement step.
// This is a rough estimate used for planning and timeout detection.
func (ig *DefaultInputGenerator) EstimateTicksRequired(
	step PathStep,
	currentState PhysicsState,
) int {
	// Get current position
	pos, _, _, _ := currentState.GetPosition()

	// Calculate distance to target
	dist := pos.DistanceTo(step.Position)

	// Estimate based on movement type
	// These are rough estimates based on vanilla Minecraft speeds
	switch step.Movement {
	case Traverse:
		// Walking speed: ~4.3 blocks/second = ~0.215 blocks/tick (at 20 TPS)
		// Add buffer for acceleration
		return int(dist/0.20) + 10

	case DiagonalTraverse:
		// Diagonal is sqrt(2) longer but same speed
		return int(dist/0.20) + 10

	case Ascend, DiagonalAscend:
		// Jump up takes ~10 ticks (0.5 seconds)
		// Plus horizontal travel time
		horizontalDist := math.Sqrt(
			(step.Position.X-pos.X)*(step.Position.X-pos.X) +
				(step.Position.Z-pos.Z)*(step.Position.Z-pos.Z),
		)
		return int(horizontalDist/0.20) + 15

	case Descend:
		// Falling: ~0.08 blocks/tick² (gravity)
		// For 1 block: ~5 ticks, 2 blocks: ~7 ticks, 3 blocks: ~9 ticks
		verticalDist := pos.Y - step.Position.Y
		fallTime := int(math.Sqrt(verticalDist/0.04)) + 5

		horizontalDist := math.Sqrt(
			(step.Position.X-pos.X)*(step.Position.X-pos.X) +
				(step.Position.Z-pos.Z)*(step.Position.Z-pos.Z),
		)
		horizontalTime := int(horizontalDist / 0.20)

		// Return max of fall time and horizontal time
		if fallTime > horizontalTime {
			return fallTime + 5
		}
		return horizontalTime + 5

	case Jump2:
		// 2-block gap jump takes ~20 ticks
		return 25

	case Climb:
		// Ladder climbing: ~0.15 blocks/tick (slower than walking)
		return int(dist/0.12) + 20

	case Swim:
		// Swimming: ~0.12 blocks/tick (even slower)
		return int(dist/0.10) + 15

	case SwimUp:
		// Swimming up: ~0.1 blocks/tick
		return int(dist/0.08) + 20

	case SwimDown:
		// Swimming down: ~0.15 blocks/tick (faster with sneak)
		return int(dist/0.12) + 15

	default:
		// Unknown movement type, use conservative estimate
		return int(dist/0.15) + 20
	}
}
