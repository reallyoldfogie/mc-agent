package movement

import (
	"fmt"
	"math"

	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// MovementExecutor handles sending movement packets to the server
type MovementExecutor interface {
	// SendPosition sends a position update to the server (position only, no rotation)
	SendPosition(x, y, z float64, onGround bool) error

	// SendPositionAndRotation sends a combined position and rotation update
	SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error

	// SendRotation sends a rotation update (rotation only, no position)
	SendRotation(yaw, pitch float32, onGround bool) error

	// MoveTowards moves the bot towards target coordinates by a given distance
	// Returns the new position after moving
	MoveTowards(targetX, targetY, targetZ float64, distance float64, onGround bool) (newX, newY, newZ float64, err error)

	// LookAt rotates the bot to look at target coordinates
	LookAt(targetX, targetY, targetZ float64, onGround bool) error
}

// movementExecutor implements MovementExecutor
type movementExecutor struct {
	client    *bot.Client
	packetMgr protocol_models.PacketMgr
	// Reference to bot position tracking (will be passed from main)
	getBotPosition func() (x, y, z float64, yaw, pitch float32, initialized bool)
	setBotPosition func(x, y, z float64, yaw, pitch float32)
}

// NewMovementExecutor creates a new MovementExecutor
func NewMovementExecutor(
	client *bot.Client,
	packetMgr protocol_models.PacketMgr,
	getBotPos func() (float64, float64, float64, float32, float32, bool),
	setBotPos func(float64, float64, float64, float32, float32),
) MovementExecutor {
	return &movementExecutor{
		client:         client,
		packetMgr:      packetMgr,
		getBotPosition: getBotPos,
		setBotPosition: setBotPos,
	}
}

// SendPosition sends a position update packet
func (me *movementExecutor) SendPosition(x, y, z float64, onGround bool) error {
	err := SendPosition(me.client, me.packetMgr, x, y, z, onGround)
	if err == nil {
		// Update tracked position (keep existing rotation)
		_, _, _, yaw, pitch, _ := me.getBotPosition()
		me.setBotPosition(x, y, z, yaw, pitch)
	}
	return err
}

// SendPositionAndRotation sends a combined position and rotation update packet
func (me *movementExecutor) SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error {
	err := SendPositionAndRotation(me.client, me.packetMgr, x, y, z, yaw, pitch, onGround)
	if err == nil {
		// Update tracked position and rotation
		me.setBotPosition(x, y, z, yaw, pitch)
	}
	return err
}

// SendRotation sends a rotation update packet
func (me *movementExecutor) SendRotation(yaw, pitch float32, onGround bool) error {
	err := SendRotation(me.client, me.packetMgr, yaw, pitch, onGround)
	if err == nil {
		// Update tracked rotation (keep existing position)
		x, y, z, _, _, _ := me.getBotPosition()
		me.setBotPosition(x, y, z, yaw, pitch)
	}
	return err
}

// MoveTowards moves the bot towards target coordinates by a given distance
func (me *movementExecutor) MoveTowards(targetX, targetY, targetZ float64, distance float64, onGround bool) (newX, newY, newZ float64, err error) {
	// Get current position
	botX, botY, botZ, yaw, pitch, initialized := me.getBotPosition()
	if !initialized {
		return 0, 0, 0, fmt.Errorf("bot position not initialized")
	}

	// Calculate direction vector (horizontal only to prevent walking on air)
	dx := targetX - botX
	dy := targetY - botY
	dz := targetZ - botZ

	// Calculate horizontal distance (X-Z plane only)
	horizontalDist := math.Sqrt(dx*dx + dz*dz)

	// If we're already at target horizontally, don't move
	if horizontalDist <= 0.01 {
		// But we might need to adjust Y if there's a vertical difference
		if math.Abs(dy) > 0.01 {
			// For now, maintain current Y to prevent walking on air
			// TODO: When world integration is ready, check if we need to step up/down
			return botX, botY, botZ, nil
		}
		return botX, botY, botZ, nil
	}

	// Normalize horizontal direction vector
	dxNorm := dx / horizontalDist
	dzNorm := dz / horizontalDist

	// Calculate movement distance (don't overshoot target horizontally)
	moveDistance := math.Min(distance, horizontalDist)

	// Calculate new position (horizontal movement only)
	newX = botX + dxNorm*moveDistance
	newY = botY // Maintain current Y to avoid walking on air
	newZ = botZ + dzNorm*moveDistance

	// Keep existing rotation (yaw and pitch) - caller should set rotation via LookAt if needed
	// This allows the bot to look at target while walking towards a different position

	// Send position and rotation update (keeping current rotation)
	err = me.SendPositionAndRotation(newX, newY, newZ, yaw, pitch, onGround)
	return newX, newY, newZ, err
}

// LookAt rotates the bot to look at target coordinates
func (me *movementExecutor) LookAt(targetX, targetY, targetZ float64, onGround bool) error {
	// Get current position
	botX, botY, botZ, _, _, initialized := me.getBotPosition()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}

	// Calculate look angles from bot's eyes to target
	// Standard player eye height is 1.62 blocks above feet
	yaw, pitch := calculateLookAngles(botX, botY+1.62, botZ, targetX, targetY+1.62, targetZ)

	// Send rotation update
	return me.SendRotation(yaw, pitch, onGround)
}

// calculateLookAngles calculates the yaw and pitch needed to look from one position to another
func calculateLookAngles(fromX, fromY, fromZ, toX, toY, toZ float64) (yaw, pitch float32) {
	dx := toX - fromX
	dy := toY - fromY
	dz := toZ - fromZ

	// Calculate horizontal distance
	horizontalDist := math.Sqrt(dx*dx + dz*dz)

	// Calculate yaw (rotation around Y axis)
	// Yaw 0 is south (+Z), 90 is west (-X), 180 is north (-Z), 270 is east (+X)
	yaw = float32(math.Atan2(-dx, dz) * 180 / math.Pi)

	// Calculate pitch (rotation around X axis)
	// Pitch -90 is straight up, 0 is level, 90 is straight down
	pitch = float32(-math.Atan2(dy, horizontalDist) * 180 / math.Pi)

	return yaw, pitch
}
