// # go:build legacy

package movement

import (
	"fmt"
	"log"
	"math"

	versions_common "github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// movementPacketSender implements a base MovementExecutor
// It does not implement a full MovementExecutor (some methods are no-ops)
type movementPacketSender struct {
	client    bot.Client
	packetMgr protocol_models.PacketMgr
	// Reference to bot position tracking (will be passed from main)
	getBotPosition func() (x, y, z float64, yaw, pitch float64, initialized bool)
	setBotPosition func(x, y, z float64, yaw, pitch float64)
	// Reference to bot entity ID (needed for sprint/sneak commands)
	getBotEntityID func() int32
	// Track sprint/sneak state to avoid redundant packets
	isSprinting bool
	isSneaking  bool
	// Optional callback for packet interception (e.g., replay mirror)
	onPacketSent func(pkt any)
	// Optional version-specific movement handler. When set, uses version-aware packet
	// construction instead of the generic packets.go functions.
	movementHandler models.MovementHandler
}

// NewBaseMovementExecutor creates a new MovementExecutor
func newMovementPacketSender(
	client bot.Client,
	packetMgr protocol_models.PacketMgr,
	getBotPos func() (float64, float64, float64, float64, float64, bool),
	setBotPos func(float64, float64, float64, float64, float64),
	getBotEntityID func() int32,
) *movementPacketSender {
	return &movementPacketSender{
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
func (me *movementPacketSender) SetPacketCallback(callback func(pkt any)) {
	me.onPacketSent = callback
}

// SetMovementHandler sets an optional version-specific movement handler.
// When set, the executor will use version-aware packet construction instead of
// the generic packets.go functions.
func (me *movementPacketSender) SetMovementHandler(handler models.MovementHandler) {
	me.movementHandler = handler
}

func (me *movementPacketSender) IsSneaking() bool {
	return me.isSneaking
}

func (me *movementPacketSender) IsSprinting() bool {
	return me.isSprinting
}

// SendPosition sends a position update packet
func (me *movementPacketSender) SendPosition(x, y, z float64, onGround bool) error {
	// Skip packet sending if client is nil (test mode)
	var err error
	if me.client != nil {
		// Use version-specific handler if available
		if me.movementHandler != nil {
			err = me.movementHandler.SendPosition(me.client.Conn(), x, y, z, onGround)

			// If sneaking, also send the sneak command to maintain sneak state
			if err == nil && me.isSneaking {
				entityID := me.getBotEntityID()
				_ = me.movementHandler.SendPlayerCommand(me.client.Conn(), entityID, versions_common.ActionStartSneaking)

			}
		} else {
			return versions_common.ErrHandlerNotSet{HandlerName: "MovementHandler"}
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
func (me *movementPacketSender) SendPositionAndRotation(x, y, z float64, yaw, pitch float64, onGround bool) error {
	// Skip packet sending if client is nil (test mode)
	var err error
	if me.client != nil {
		log.Printf("[YAW DEBUG %s] executor.SendPositionAndRotation received: pos=(%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f", me.client.Name(), x, y, z, yaw, pitch)
		// Use version-specific handler if available, otherwise fall back to generic packets
		if me.movementHandler != nil {
			err = me.movementHandler.SendPositionAndRotation(me.client.Conn(), x, y, z, yaw, pitch, onGround)

			// If sneaking, also send the sneak command to maintain sneak state
			if me.isSneaking {
				entityID := me.getBotEntityID()
				_ = me.movementHandler.SendPlayerCommand(me.client.Conn(), entityID, versions_common.ActionStartSneaking)

			}
		} else {
			return versions_common.ErrHandlerNotSet{HandlerName: "MovementHandler"}
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
func (me *movementPacketSender) SendRotation(yaw, pitch float64, onGround bool) error {
	// Skip packet sending if client is nil (test mode)
	var err error
	if me.client != nil {
		// Use version-specific handler if available, otherwise fall back to generic packets
		if me.movementHandler != nil {
			err = me.movementHandler.SendRotation(me.client.Conn(), yaw, pitch, onGround)
		} else {
			return versions_common.ErrHandlerNotSet{HandlerName: "MovementHandler"}
		}
	}

	if err == nil {
		// Update tracked rotation (keep existing position)
		x, y, z, _, _, _ := me.getBotPosition()
		me.setBotPosition(x, y, z, yaw, pitch)
	}
	return err
}

// MoveTowards moves the bot towards target coordinates by a given distance
func (me *movementPacketSender) MoveTowards(targetX, targetY, targetZ float64, distance float64, onGround bool) (newX, newY, newZ float64, err error) {
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
func (me *movementPacketSender) LookAt(targetX, targetY, targetZ float64, onGround bool) error {
	// Get current position
	botX, botY, botZ, _, _, initialized := me.getBotPosition()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}

	// Calculate look angles from bot's eyes to target
	// Player eye height depends on sneak state
	var eyeHeight float64
	if me.isSneaking {
		eyeHeight = models.PlayerEyeHeightSneaking // Sneaking eye height
	} else {
		eyeHeight = models.PlayerEyeHeight // Standing eye height
	}
	yaw, pitch := calculateLookAngles(botX, botY+eyeHeight, botZ, targetX, targetY+eyeHeight, targetZ)

	// Send rotation update
	return me.SendRotation(yaw, pitch, onGround)
}

// calculateLookAngles calculates the yaw and pitch needed to look from one position to another
func calculateLookAngles(fromX, fromY, fromZ, toX, toY, toZ float64) (yaw, pitch float64) {
	dx := toX - fromX
	dy := toY - fromY
	dz := toZ - fromZ

	// Calculate horizontal distance
	horizontalDist := math.Sqrt(dx*dx + dz*dz)

	// Calculate yaw (rotation around Y axis)
	// NOTE: Trajectory is simulated in local space with Z=forward, X=0
	// Yaw formula must convert from world delta to firing direction
	// atan2(-dx, dz) accounts for the coordinate system rotation
	yaw = math.Atan2(-dx, dz) * 180 / math.Pi

	// Calculate pitch (rotation around X axis)
	// Pitch -90 is straight up, 0 is level, 90 is straight down
	pitch = -math.Atan2(dy, horizontalDist) * 180 / math.Pi

	return yaw, pitch
}

// StartSprinting sends a command to start sprinting
func (me *movementPacketSender) StartSprinting() error {
	if me.isSprinting {
		return nil // Already sprinting, no need to send packet
	}

	if me.client == nil {
		me.isSprinting = true // Mark as sprinting even in test mode
		return nil            // No-op in test mode
	}

	entityID := me.getBotEntityID()
	var err error
	if me.movementHandler != nil {
		err = me.movementHandler.SendPlayerCommand(me.client.Conn(), entityID, versions_common.ActionStartSprinting)
	} else {
		return versions_common.ErrHandlerNotSet{HandlerName: "MovementHandler"}
	}
	if err == nil {
		me.isSprinting = true
	}
	return err
}

// StopSprinting sends a command to stop sprinting
func (me *movementPacketSender) StopSprinting() error {
	if !me.isSprinting {
		return nil // Not sprinting, no need to send packet
	}

	if me.client == nil {
		me.isSprinting = false // Mark as not sprinting even in test mode
		return nil             // No-op in test mode
	}

	entityID := me.getBotEntityID()
	var err error
	if me.movementHandler != nil {
		err = me.movementHandler.SendPlayerCommand(me.client.Conn(), entityID, versions_common.ActionStopSprinting)
	} else {
		return versions_common.ErrHandlerNotSet{HandlerName: "MovementHandler"}
	}
	if err == nil {
		me.isSprinting = false
	}
	return err
}

// StartSneaking sends a command to start sneaking
func (me *movementPacketSender) StartSneaking() error {
	if me.isSneaking {
		return nil // Already sneaking, no need to send packet
	}

	if me.client == nil {
		me.isSneaking = true // Mark as sneaking even in test mode
		return nil           // No-op in test mode
	}

	entityID := me.getBotEntityID()
	var err error
	if me.movementHandler != nil {
		err = me.movementHandler.SendPlayerCommand(me.client.Conn(), entityID, versions_common.ActionStartSneaking)
	} else {
		return versions_common.ErrHandlerNotSet{HandlerName: "MovementHandler"}
	}
	if err == nil {
		me.isSneaking = true
	}
	return err
}

// StopSneaking sends a command to stop sneaking
func (me *movementPacketSender) StopSneaking() error {
	if !me.isSneaking {
		return nil // Not sneaking, no need to send packet
	}

	if me.client == nil {
		me.isSneaking = false // Mark as not sneaking even in test mode
		return nil            // No-op in test mode
	}

	entityID := me.getBotEntityID()
	var err error
	if me.movementHandler != nil {
		err = me.movementHandler.SendPlayerCommand(me.client.Conn(), entityID, versions_common.ActionStopSneaking)
	} else {
		return versions_common.ErrHandlerNotSet{HandlerName: "MovementHandler"}
	}
	if err == nil {
		me.isSneaking = false
	}
	return err
}
