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
}
