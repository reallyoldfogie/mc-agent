package movement

import (
	"log"
	"math"

	semver "github.com/aquasecurity/go-version/pkg/version"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// horseTurnDegsPerTick is the yaw rotation rate for ridden mobs (horse, camel, etc.)
// when ThrottleX is applied for yaw-rotation steering.
const horseTurnDegsPerTick = 5.0

// ridingWaterPhysicsParams holds the computed water physics parameters for a
// ridden entity at a specific position. Used by both the horse and camel handlers
// to avoid duplicating the version-gated sinking/floating logic.
type ridingWaterPhysicsParams struct {
	IsInWater                   bool
	ShouldApplyOldSinkingBehavior bool
	VelocityDrag                float64
	GravityDelta                float64
	BlockBelowEntity            uint32
}

// computeRidingWaterPhysics determines the water physics parameters for a ridden
// entity based on its position and the server version. This extracts the shared
// water detection + version-gated sinking/floating logic used by multiple handlers.
//
// When not in water, returns zero-value params (IsInWater=false). The caller is
// responsible for setting land/airborne friction in that case.
func computeRidingWaterPhysics(pe *PhysicsMovementExecutor, versionHandler models.VersionHandler, currentPos models.V3) ridingWaterPhysicsParams {
	blockBelowEntity := pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z)
	isInWater := pe.shapeProvider != nil && pe.shapeProvider.IsWater(blockBelowEntity)

	if !isInWater {
		return ridingWaterPhysicsParams{
			BlockBelowEntity: blockBelowEntity,
		}
	}

	// Determine version to decide sinking vs floating behavior
	shouldApplyOldSinkingBehavior := true
	if versionHandler != nil {
		versionStr := versionHandler.Version()
		if v, err := semver.Parse(versionStr); err == nil {
			if c, err := semver.NewConstraints(">= 1.21.11"); err == nil && c.Check(v) {
				shouldApplyOldSinkingBehavior = false
			}
		}
	}

	var velocityDrag, gravityDelta float64
	if shouldApplyOldSinkingBehavior {
		// Pre-1.21.11: Apply full gravity to make entity sink, and significant drag
		velocityDrag = physics.RideableInWaterDragMultiplier
		gravityDelta = -physics.RideableInWaterGravity
	} else {
		// 1.21.11+: Float on water surface with slow movement
		velocityDrag = 0.5
		gravityDelta = 0.0
	}

	return ridingWaterPhysicsParams{
		IsInWater:                     true,
		ShouldApplyOldSinkingBehavior: shouldApplyOldSinkingBehavior,
		VelocityDrag:                  velocityDrag,
		GravityDelta:                  gravityDelta,
		BlockBelowEntity:              blockBelowEntity,
	}
}

// sendVehicleMove updates the executor's physics state and bot position, then
// sends a VehicleMove packet to the server. This 6-line pattern is used by
// every riding handler.
func sendVehicleMove(pe *PhysicsMovementExecutor, versionHandler models.VersionHandler, newPos models.V3, yaw, pitch float64, onGround bool) {
	pe.physicsState.SetPosition(newPos, yaw, pitch, onGround)
	pe.movementPacketSender.setBotPosition(newPos, yaw, pitch)

	if err := versionHandler.Play().Movement().SendMoveVehicle(
		pe.movementPacketSender.client.Conn(),
		newPos.X, newPos.Y, newPos.Z,
		yaw, pitch,
		onGround,
	); err != nil {
		log.Printf("[handleRidingMode] Failed to send vehicle move packet: %v", err)
	}
}

// sendRidingInput sends a PlayerInput/VehicleInput packet to the server.
// Wraps the version-handler call with error logging.
func sendRidingInput(pe *PhysicsMovementExecutor, versionHandler models.VersionHandler, forward, backward, left, right, jump, sneak bool) {
	if err := versionHandler.Play().Movement().SendVehicleInput(
		pe.movementPacketSender.client.Conn(),
		forward, backward, left, right, jump, sneak,
	); err != nil {
		log.Printf("[handleRidingMode] Failed to send vehicle input packet: %v", err)
	}
}

// checkRiderHeadSubmerged checks if the rider's head is submerged in water and
// requests auto-dismount if so. Only applies to pre-1.21.11 sinking behavior.
func checkRiderHeadSubmerged(pe *PhysicsMovementExecutor, mountedEntityID int32, waterParams ridingWaterPhysicsParams, newX, newY, newZ float64) {
	if !waterParams.ShouldApplyOldSinkingBehavior || !waterParams.IsInWater {
		return
	}

	riderEyeY := newY + models.PlayerEyeHeight
	blockAboveRider := int(math.Floor(riderEyeY)) + 1
	blockX := int(math.Floor(newX))
	blockZ := int(math.Floor(newZ))
	blockAboveRiderState, loaded := pe.world.GetBlockStatus(blockX, blockAboveRider, blockZ)
	isRiderHeadInWater := loaded && pe.shapeProvider != nil && pe.shapeProvider.IsWater(blockAboveRiderState)

	if isRiderHeadInWater {
		log.Printf("[handleRidingMode] Rider's head submerged in water - auto-dismounting from entity %d at (%.2f, %.2f, %.2f)",
			mountedEntityID, newX, newY, newZ)
		pe.dismountRequested = true
	}
}

// getBlockBelowEntity returns the block state directly below an entity's position.
// Renamed from getBlockBelowBoat to be entity-agnostic; used by all riding handlers.
func (pe *PhysicsMovementExecutor) getBlockBelowEntity(entityX, entityY, entityZ float64) uint32 {
	if pe.world == nil {
		return 0
	}
	blockX := int(math.Floor(entityX))
	blockY := int(math.Floor(entityY - 0.1))
	blockZ := int(math.Floor(entityZ))

	blockState, loaded := pe.world.GetBlockStatus(blockX, blockY, blockZ)
	if !loaded {
		return 0
	}
	return blockState
}
