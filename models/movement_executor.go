package models

// MovementExecutor handles sending movement packets to the server.
type MovementExecutor interface {
	SendPosition(x, y, z float64, onGround bool) error
	SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error
	SendRotation(yaw, pitch float32, onGround bool) error
	MoveTowards(targetX, targetY, targetZ float64, distance float64, onGround bool) (newX, newY, newZ float64, err error)
	LookAt(targetX, targetY, targetZ float64, onGround bool) error
	StartSprinting() error
	StopSprinting() error
	IsSprinting() bool
	StartSneaking() error
	StopSneaking() error
	IsSneaking() bool

	SetVelocity(x, y, z float64) error

	// SetMounted transitions the executor to mounted/riding mode.
	// The executor should switch to sending vehicle movement packets (ServerboundMoveVehicle).
	// vehicleEntityID: the entity ID of the vehicle being ridden
	SetMounted(vehicleEntityID int32) error

	// SetDismounted transitions the executor back from mounted mode to normal movement mode.
	// The executor should switch back to sending player movement packets (ServerboundMovePlayerPos, etc).
	SetDismounted() error

	SetTelemetryRecorder(recorder MovementTelemetryRecorder)

	SetMovementHandler(MovementHandler)
}

// ManualMovementExecutor is an optional interface for executors that support frame-by-frame physics
// simulation control. This allows commands like MoveForward to provide precise directional input
// each tick rather than teleporting to a target position.
type ManualMovementExecutor interface {
	// EnterManualMode enables frame-by-frame physics simulation control.
	// In manual mode, the executor processes SetManualThrottle and SetManualRotation commands
	// each tick instead of executing queued movement commands.
	EnterManualMode() error

	// ExitManualMode disables frame-by-frame physics simulation control and returns to normal operation.
	ExitManualMode() error

	// SetManualThrottle sets the directional input for the current and next frames.
	// westEastThrottle: positive = east, negative = west
	// northSouthThrottle: positive = south, negative = north
	// These values are world-space directions, not player-relative WASD inputs.
	SetManualThrottle(westEastThrottle, northSouthThrottle float64) error

	// SetManualRotation sets the yaw and pitch for the current and next frames.
	// Use math.NaN() to maintain the current rotation value without changing it.
	SetManualRotation(yaw, pitch float64) error
}

// RidingPhysicsInspector provides read-only access to the riding (mounted
// vehicle) physics state for testing and diagnostics. Both the movement
// executor and the agent implement this interface so tests can inspect the
// actual simulation values rather than maintaining a parallel simulator.
type RidingPhysicsInspector interface {
	// GetRidingVelocity returns the current horizontal riding velocity
	// (blocks/tick). Returns (0, 0) when not mounted.
	GetRidingVelocity() (velX, velZ float64)

	// GetRidingDragMultiplier returns the per-tick velocity multiplier
	// used on the most recent riding tick. The value depends on the surface
	// under the boat (water=0.9, land=0.6, ice=0.98, etc.) or the mount
	// type (horse=0.9). Returns 0 before the first riding tick.
	GetRidingDragMultiplier() float64
}

// ManualMovement provides passthrough access to manual movement control on the agent.
// These methods delegate to the underlying movement executor if it supports ManualMovementExecutor.
type ManualMovement interface {
	// EnterManualMode enables frame-by-frame physics simulation control on the agent.
	EnterManualMode() error

	// ExitManualMode disables frame-by-frame physics simulation control on the agent.
	ExitManualMode() error

	// SetManualThrottle sets directional input for movement.
	SetManualThrottle(westEastThrottle, northSouthThrottle float64) error

	// SetManualRotation sets rotation input (yaw/pitch).
	SetManualRotation(yaw, pitch float64) error
}
