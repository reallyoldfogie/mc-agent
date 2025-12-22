package movement

import (
	"fmt"
	"log"
	"time"

	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// PhysicsMovementExecutor implements MovementExecutor using physics simulation.
// Instead of interpolating positions, it simulates realistic Minecraft physics
// with inputs (throttle, jump, sprint, sneak) and sends position updates at 20 TPS.
type PhysicsMovementExecutor struct {
	// Base executor for packet sending
	baseExecutor *movementExecutor

	// Physics simulation
	physicsState  *physics.State
	inputGen      pathfinding.InputGenerator
	world         physics.World
	shapeProvider physics.BlockShapeProvider

	// Timing
	tickRate time.Duration // Time per tick (default: 50ms for 20 TPS)

	// Server correction tracking
	predictionErrors []float64 // Recent prediction errors for logging
	maxErrorHistory  int        // Max errors to track (default: 100)
}

// NewPhysicsMovementExecutor creates a new physics-based movement executor.
func NewPhysicsMovementExecutor(
	client *bot.Client,
	packetMgr protocol_models.PacketMgr,
	getBotPos func() (float64, float64, float64, float32, float32, bool),
	setBotPos func(float64, float64, float64, float32, float32),
	getBotEntityID func() int32,
	world physics.World,
	shapeProvider physics.BlockShapeProvider,
) *PhysicsMovementExecutor {
	// Create base executor for packet sending
	baseExecutor := &movementExecutor{
		client:         client,
		packetMgr:      packetMgr,
		getBotPosition: getBotPos,
		setBotPosition: setBotPos,
		getBotEntityID: getBotEntityID,
		isSprinting:    false,
		isSneaking:     false,
		onPacketSent:   nil,
	}

	// Create physics state
	physicsState := physics.NewState(shapeProvider)

	// Initialize physics state from current bot position
	x, y, z, yaw, pitch, initialized := getBotPos()
	if initialized {
		physicsState.SetPosition(
			physics.V3{X: x, Y: y, Z: z},
			float64(yaw),
			float64(pitch),
			true, // Assume on ground initially
		)
	}

	return &PhysicsMovementExecutor{
		baseExecutor:     baseExecutor,
		physicsState:     physicsState,
		inputGen:         pathfinding.NewInputGenerator(),
		world:            world,
		shapeProvider:    shapeProvider,
		tickRate:         50 * time.Millisecond, // 20 TPS
		predictionErrors: make([]float64, 0, 100),
		maxErrorHistory:  100,
	}
}

// SetPacketCallback sets an optional callback for packet interception.
func (pe *PhysicsMovementExecutor) SetPacketCallback(callback func(pkt interface{})) {
	pe.baseExecutor.SetPacketCallback(callback)
}

// SendPosition sends a position update to the server.
func (pe *PhysicsMovementExecutor) SendPosition(x, y, z float64, onGround bool) error {
	return pe.baseExecutor.SendPosition(x, y, z, onGround)
}

// SendPositionAndRotation sends a combined position and rotation update.
func (pe *PhysicsMovementExecutor) SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error {
	return pe.baseExecutor.SendPositionAndRotation(x, y, z, yaw, pitch, onGround)
}

// SendRotation sends a rotation update.
func (pe *PhysicsMovementExecutor) SendRotation(yaw, pitch float32, onGround bool) error {
	return pe.baseExecutor.SendRotation(yaw, pitch, onGround)
}

// MoveTowards is not used by physics executor (use ExecutePath instead).
func (pe *PhysicsMovementExecutor) MoveTowards(targetX, targetY, targetZ float64, distance float64, onGround bool) (newX, newY, newZ float64, err error) {
	return 0, 0, 0, fmt.Errorf("MoveTowards not supported by physics executor, use ExecutePath instead")
}

// LookAt rotates the bot to look at target coordinates.
func (pe *PhysicsMovementExecutor) LookAt(targetX, targetY, targetZ float64, onGround bool) error {
	return pe.baseExecutor.LookAt(targetX, targetY, targetZ, onGround)
}

// StartSprinting sends a command to start sprinting.
func (pe *PhysicsMovementExecutor) StartSprinting() error {
	return pe.baseExecutor.StartSprinting()
}

// StopSprinting sends a command to stop sprinting.
func (pe *PhysicsMovementExecutor) StopSprinting() error {
	return pe.baseExecutor.StopSprinting()
}

// IsSprinting returns true if the bot is currently sprinting.
func (pe *PhysicsMovementExecutor) IsSprinting() bool {
	return pe.baseExecutor.IsSprinting()
}

// StartSneaking sends a command to start sneaking.
func (pe *PhysicsMovementExecutor) StartSneaking() error {
	return pe.baseExecutor.StartSneaking()
}

// StopSneaking sends a command to stop sneaking.
func (pe *PhysicsMovementExecutor) StopSneaking() error {
	return pe.baseExecutor.StopSneaking()
}

// IsSneaking returns true if the bot is currently sneaking.
func (pe *PhysicsMovementExecutor) IsSneaking() bool {
	return pe.baseExecutor.IsSneaking()
}

// SyncWithServer updates physics state from server position correction.
// This should be called when receiving ClientboundPosition packets from the server.
func (pe *PhysicsMovementExecutor) SyncWithServer(x, y, z float64, yaw, pitch float32, onGround bool) {
	// Calculate prediction error before syncing
	currentPos := pe.physicsState.Pos
	deltaX := x - currentPos.X
	deltaY := y - currentPos.Y
	deltaZ := z - currentPos.Z
	predictionError := deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ

	// Track prediction error
	pe.trackPredictionError(predictionError)

	// Sync physics state with server
	pe.physicsState.SetPosition(
		physics.V3{X: x, Y: y, Z: z},
		float64(yaw),
		float64(pitch),
		onGround,
	)

	log.Printf("[PhysicsExecutor] Server correction: Δ(%.3f, %.3f, %.3f) error²=%.6f",
		deltaX, deltaY, deltaZ, predictionError)
}

// trackPredictionError records a prediction error for monitoring.
func (pe *PhysicsMovementExecutor) trackPredictionError(error float64) {
	if len(pe.predictionErrors) >= pe.maxErrorHistory {
		// Remove oldest error
		pe.predictionErrors = pe.predictionErrors[1:]
	}
	pe.predictionErrors = append(pe.predictionErrors, error)
}

// GetAveragePredictionError returns the average squared prediction error.
func (pe *PhysicsMovementExecutor) GetAveragePredictionError() float64 {
	if len(pe.predictionErrors) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, err := range pe.predictionErrors {
		sum += err
	}
	return sum / float64(len(pe.predictionErrors))
}

// ExecuteStep executes a single path step using physics simulation.
// Returns true when the step is complete, false if more ticks are needed.
func (pe *PhysicsMovementExecutor) ExecuteStep(step pathfinding.PathStep, runTime time.Duration) (bool, error) {
	// Generate inputs for this step
	inputs := pe.inputGen.GenerateInputs(pe.physicsState, step, runTime)

	// Apply sprint/sneak state changes
	if inputs.Sprint && !pe.IsSprinting() {
		if err := pe.StartSprinting(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to start sprinting: %v", err)
		}
	} else if !inputs.Sprint && pe.IsSprinting() {
		if err := pe.StopSprinting(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to stop sprinting: %v", err)
		}
	}

	if inputs.Sneak && !pe.IsSneaking() {
		if err := pe.StartSneaking(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to start sneaking: %v", err)
		}
	} else if !inputs.Sneak && pe.IsSneaking() {
		if err := pe.StopSneaking(); err != nil {
			log.Printf("[PhysicsExecutor] Failed to stop sneaking: %v", err)
		}
	}

	// Tick physics simulation
	if err := pe.physicsState.Tick(inputs, pe.world); err != nil {
		return false, fmt.Errorf("physics tick failed: %w", err)
	}

	// Get updated position from physics
	pos, yaw, pitch, onGround := pe.physicsState.GetPosition()

	// Send position update to server
	err := pe.SendPositionAndRotation(
		pos.X, pos.Y, pos.Z,
		float32(yaw), float32(pitch),
		onGround,
	)
	if err != nil {
		return false, fmt.Errorf("failed to send position: %w", err)
	}

	// Check if step is complete
	complete := pathfinding.IsComplete(pos, step)
	return complete, nil
}

// ExecutePath executes a complete path using physics simulation.
// This is the main entry point for physics-based movement.
func (pe *PhysicsMovementExecutor) ExecutePath(path *pathfinding.Path) error {
	if !path.Found {
		return fmt.Errorf("path not found")
	}

	log.Printf("[PhysicsExecutor] Executing path: %d steps, cost=%.2f", len(path.Steps), path.TotalCost)

	// Execute each step in sequence
	for i, step := range path.Steps {
		log.Printf("[PhysicsExecutor] Step %d/%d: %s to (%.1f, %.1f, %.1f)",
			i+1, len(path.Steps), step.Movement, step.Position.X, step.Position.Y, step.Position.Z)

		// Track how long we've been executing this step
		stepStartTime := time.Now()
		maxStepDuration := 30 * time.Second // Timeout per step

		// Execute step until complete or timeout
		tickCount := 0
		for {
			runTime := time.Since(stepStartTime)

			// Check timeout
			if runTime > maxStepDuration {
				return fmt.Errorf("step %d timed out after %v (position: %.2f, %.2f, %.2f)",
					i+1, runTime, pe.physicsState.Pos.X, pe.physicsState.Pos.Y, pe.physicsState.Pos.Z)
			}

			// Execute one tick
			complete, err := pe.ExecuteStep(step, runTime)
			if err != nil {
				return fmt.Errorf("step %d failed: %w", i+1, err)
			}

			tickCount++

			// Check if step is complete
			if complete {
				log.Printf("[PhysicsExecutor] Step %d complete after %d ticks (%.1fs)",
					i+1, tickCount, runTime.Seconds())
				break
			}

			// Wait for next tick
			time.Sleep(pe.tickRate)
		}
	}

	log.Printf("[PhysicsExecutor] Path execution complete")
	return nil
}

// GetCurrentPosition returns the current physics state position.
func (pe *PhysicsMovementExecutor) GetCurrentPosition() (x, y, z float64) {
	pos, _, _, _ := pe.physicsState.GetPosition()
	return pos.X, pos.Y, pos.Z
}

// GetPhysicsState returns the internal physics state for advanced usage.
func (pe *PhysicsMovementExecutor) GetPhysicsState() *physics.State {
	return pe.physicsState
}
