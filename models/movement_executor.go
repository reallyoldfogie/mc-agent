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

	SetTelemetryRecorder(recorder MovementTelemetryRecorder)
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
