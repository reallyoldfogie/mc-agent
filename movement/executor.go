package movement

import (
	"fmt"
	"log"
	"math"

	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// movementExecutor implements MovementExecutor
type movementExecutor struct {
	client    bot.Client
	packetMgr protocol_models.PacketMgr
	// Reference to bot position tracking (will be passed from main)
	getBotPosition func() (x, y, z float64, yaw, pitch float32, initialized bool)
	setBotPosition func(x, y, z float64, yaw, pitch float32)
	// Reference to bot entity ID (needed for sprint/sneak commands)
	getBotEntityID func() int32
	// Track sprint/sneak state to avoid redundant packets
	isSprinting bool
	isSneaking  bool
	// Optional callback for packet interception (e.g., replay mirror)
	onPacketSent func(pkt interface{})
}

// NewMovementExecutor creates a new MovementExecutor
func NewMovementExecutor(
	client bot.Client,
	packetMgr protocol_models.PacketMgr,
	getBotPos func() (float64, float64, float64, float32, float32, bool),
	setBotPos func(float64, float64, float64, float32, float32),
	getBotEntityID func() int32,
) MovementExecutor {
	return &movementExecutor{
		client:         client,
		packetMgr:      packetMgr,
		getBotPosition: getBotPos,
		setBotPosition: setBotPos,
		getBotEntityID: getBotEntityID,
		isSprinting:    false,
		isSneaking:     false,
		onPacketSent:   nil,
	}
}

// SetPacketCallback sets an optional callback that will be invoked with each packet before it's sent.
// This is useful for replay mirroring or packet logging.
func (me *movementExecutor) SetPacketCallback(callback func(pkt interface{})) {
	me.onPacketSent = callback
}

func (me *movementExecutor) IsSneaking() bool {
	return me.isSneaking
}

func (me *movementExecutor) IsSprinting() bool {
	return me.isSprinting
}

// SendPosition sends a position update packet
func (me *movementExecutor) SendPosition(x, y, z float64, onGround bool) error {
	// Skip packet sending if client is nil (test mode)
	var err error
	if me.client != nil {
		err = SendPositionWithCallback(me.client, me.packetMgr, x, y, z, onGround, me.onPacketSent)
		// If sneaking, also send the sneak command to maintain sneak state
		if err == nil && me.isSneaking {
			entityID := me.getBotEntityID()
			_ = SendStartSneaking(me.client, me.packetMgr, entityID)
		}
	}

	if err == nil {
		// Update tracked position (keep existing rotation)
		_, _, _, yaw, pitch, _ := me.getBotPosition()
		me.setBotPosition(x, y, z, yaw, pitch)
	}
	return err
}

// SendPositionAndRotation sends a combined position and rotation update packet
func (me *movementExecutor) SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error {
	// Skip packet sending if client is nil (test mode)
	var err error
	if me.client != nil {
		log.Printf("[YAW DEBUG %s] executor.SendPositionAndRotation received: pos=(%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f", me.client.Name(), x, y, z, yaw, pitch)
		err = SendPositionAndRotationWithCallback(me.client, me.packetMgr, x, y, z, yaw, pitch, onGround, me.onPacketSent)
		// If sneaking, also send the sneak command to maintain sneak state
		if err == nil && me.isSneaking {
			entityID := me.getBotEntityID()
			_ = SendStartSneaking(me.client, me.packetMgr, entityID)
		}
	}

	if err == nil {
		// Update tracked position and rotation
		if me.client != nil {
			log.Printf("[YAW DEBUG %s] executor.SendPositionAndRotation calling setBotPosition: pos=(%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f", me.client.Name(), x, y, z, yaw, pitch)
		}
		me.setBotPosition(x, y, z, yaw, pitch)
		if me.client != nil {
			log.Printf("[YAW DEBUG %s] executor.SendPositionAndRotation setBotPosition done", me.client.Name())
		}
	}
	return err
}

// SendRotation sends a rotation update packet
func (me *movementExecutor) SendRotation(yaw, pitch float32, onGround bool) error {
	// Skip packet sending if client is nil (test mode)
	var err error
	if me.client != nil {
		err = SendRotationWithCallback(me.client, me.packetMgr, yaw, pitch, onGround, me.onPacketSent)
	}

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

	// If we're already at target horizontally, handle vertical movement
	if horizontalDist <= 0.01 {
		if math.Abs(dy) > 0.01 {
			// Move vertically to match path step (world integration complete)
			err = me.SendPositionAndRotation(botX, targetY, botZ, yaw, pitch, onGround)
			return botX, targetY, botZ, err
		}
		return botX, botY, botZ, nil
	}

	// Normalize horizontal direction vector
	dxNorm := dx / horizontalDist
	dzNorm := dz / horizontalDist

	// Calculate movement distance (don't overshoot target horizontally)
	moveDistance := math.Min(distance, horizontalDist)

	// Calculate new position (include vertical movement from pathfinding)
	newX = botX + dxNorm*moveDistance

	// Smart physics for Y movement:
	// - Upward or nearly level: move to target Y immediately
	// - Downward: apply controlled descent (gravity simulation)
	if dy > 0 || math.Abs(dy) < 0.1 {
		// Going up or nearly level - move to target Y
		newY = targetY
	} else {
		// Going down - apply controlled descent (max 0.5 blocks per movement tick)
		fallAmount := min(-dy, 0.5)
		newY = max(targetY, botY-fallAmount)
	}

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

// StartSprinting sends a command to start sprinting
func (me *movementExecutor) StartSprinting() error {
	if me.isSprinting {
		return nil // Already sprinting, no need to send packet
	}

	entityID := me.getBotEntityID()
	err := SendStartSprinting(me.client, me.packetMgr, entityID)
	if err == nil {
		me.isSprinting = true
	}
	return err
}

// StopSprinting sends a command to stop sprinting
func (me *movementExecutor) StopSprinting() error {
	if !me.isSprinting {
		return nil // Not sprinting, no need to send packet
	}

	entityID := me.getBotEntityID()
	err := SendStopSprinting(me.client, me.packetMgr, entityID)
	if err == nil {
		me.isSprinting = false
	}
	return err
}

// StartSneaking sends a command to start sneaking
func (me *movementExecutor) StartSneaking() error {
	if me.isSneaking {
		return nil // Already sneaking, no need to send packet
	}

	entityID := me.getBotEntityID()
	err := SendStartSneaking(me.client, me.packetMgr, entityID)
	if err == nil {
		me.isSneaking = true
	}
	return err
}

// StopSneaking sends a command to stop sneaking
func (me *movementExecutor) StopSneaking() error {
	if !me.isSneaking {
		return nil // Not sneaking, no need to send packet
	}

	entityID := me.getBotEntityID()
	err := SendStopSneaking(me.client, me.packetMgr, entityID)
	if err == nil {
		me.isSneaking = false
	}
	return err
}
