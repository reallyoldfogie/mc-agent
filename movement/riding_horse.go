package movement

import (
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// handleRidingModeHorse runs one tick for a ridden AbstractHorseEntity-based mob
// (horse, donkey, mule). Also serves as the fallback for any unrecognized rideable entity.
//
// This mirrors Java's LivingEntity.travelControlled() with yaw-rotation steering,
// slipperiness-based friction, and version-gated water sinking/floating.
// Does NOT contain any camel-specific logic — no dash, no pose state machine.
func (pe *PhysicsMovementExecutor) handleRidingModeHorse(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) {
	// (1) Send PlayerInput
	log.Printf("[handleRidingModeHorse] SendVehicleInput(<conn>, forward: %t, backward: %t, left: %t, right: %t, jump: %t, sneak: %t)", forward, backward, left, right, jump, sneak)
	sendRidingInput(pe, versionHandler, forward, backward, left, right, jump, sneak)

	// Hold lock for entire read-compute-write cycle
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	currentPos, yaw, pitch, _ := pe.physicsState.GetPosition()

	// (2) Yaw-rotation steering
	yaw -= inputs.ThrottleX * horseTurnDegsPerTick

	// (3) Water/land physics
	waterParams := computeRidingWaterPhysics(pe, versionHandler, currentPos)
	var velocityDrag float64
	var gravityDelta float64

	if waterParams.IsInWater {
		velocityDrag = waterParams.VelocityDrag
		gravityDelta = waterParams.GravityDelta
	} else {
		// On land: slipperiness-based friction
		blockSlipperiness := physics.GetBlockSlipperiness(waterParams.BlockBelowEntity)
		velocityDrag = physics.HorseLandFriction(blockSlipperiness)
		gravityDelta = 0.0
		pe.ridingVelY = 0.0
	}

	// (4) Movement speed from entity attribute
	movementAcceleration := 0.225 // Default: vanilla horse movement speed
	if entityGetter != nil {
		if movementSpeed, ok := entityGetter.GetEntityAttribute(mountedEntityID, "generic.movement_speed"); ok {
			movementAcceleration = movementSpeed
		}
	}

	// (5) Velocity computation
	ridingVelZ := pe.ridingVelZ*velocityDrag + inputs.ThrottleZ*movementAcceleration
	ridingVelX := 0.0 // Horses don't strafe; ThrottleX is steering
	ridingVelY := pe.ridingVelY + gravityDelta

	if math.Abs(ridingVelZ) < physics.ResetVelocity {
		ridingVelZ = 0
	}

	// (6) Position computation
	yawRad := yaw * math.Pi / 180.0
	newX := currentPos.X + (-math.Sin(yawRad) * ridingVelZ)
	newY := currentPos.Y + ridingVelY
	newZ := currentPos.Z + (math.Cos(yawRad) * ridingVelZ)
	onGround := true

	// Update shared velocity state
	pe.ridingVelX = ridingVelX
	pe.ridingVelZ = ridingVelZ
	pe.ridingVelY = ridingVelY
	pe.lastVelMultiplier = velocityDrag

	// Auto-dismount when submerged
	checkRiderHeadSubmerged(pe, mountedEntityID, waterParams, newX, newY, newZ)

	waterBehavior := "none"
	if waterParams.IsInWater {
		if waterParams.ShouldApplyOldSinkingBehavior {
			waterBehavior = "sink_pre1.21.11"
		} else {
			waterBehavior = "float_1.21.11+"
		}
	}

	log.Printf("[handleRidingModeHorse] physics: water=%v behavior=%s yaw=%.1f throttle=(%.2f,%.2f) accel=%.4f drag=%.3f velZ=%.4f velY=%.4f newPos=(%.2f,%.2f,%.2f)",
		waterParams.IsInWater, waterBehavior, yaw, inputs.ThrottleX, inputs.ThrottleZ, movementAcceleration, velocityDrag, ridingVelZ, ridingVelY, newX, newY, newZ)

	newPos := models.V3{X: newX, Y: newY, Z: newZ}
	sendVehicleMove(pe, versionHandler, newPos, yaw, pitch, onGround)
}
