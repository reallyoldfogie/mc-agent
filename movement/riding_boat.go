package movement

import (
	"fmt"
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// handleRidingModeBoat runs one client-authoritative boat tick.
//
// The physics model mirrors vanilla AbstractBoatEntity (extractedSrc
// 1.21.10/net/minecraft/entity/vehicle/AbstractBoatEntity.java:534-608):
//
//	// updateVelocity (drag + gravity)
//	 this.setVelocity(vec3d.x * f, vec3d.y + d, vec3d.z * f);
//	 this.yawVelocity *= f;
//	 // updatePaddles (input → yawVelocity & forward thrust)
//	 if (this.pressingLeft)  this.yawVelocity--;
//	 if (this.pressingRight) this.yawVelocity++;
//	 if (this.pressingForward) f += 0.04F;
//	 if (this.pressingBack)    f -= 0.005F;
//	 this.setYaw(this.getYaw() + this.yawVelocity);
//	 this.setVelocity(this.getVelocity().add(
//	     MathHelper.sin(-this.getYaw()*π/180) * f, 0,
//	     MathHelper.cos(this.getYaw()*π/180) * f));
//
// Left/right rotate the boat via angular velocity (boatYawVelocity).
// Forward/backward thrust is projected through the boat's current yaw.
// The boat cannot strafe — it can only accelerate along its heading.
func (pe *PhysicsMovementExecutor) handleRidingModeBoat(
	versionHandler models.VersionHandler,
	forward, backward, left, right, sneak bool,
) {
	// --- Send vehicle input packet (SteerVehicle on 1.21.1, PlayerInput on 1.21.2+) ---
	if err := versionHandler.Play().Movement().SendVehicleInput(
		pe.movementPacketSender.client.Conn(),
		forward, backward, left, right, false, sneak,
	); err != nil {
		log.Printf("[handleRidingMode] Failed to send boat vehicle input packet: %v", err)
	}

	// --- Send paddle-state packet for animation/sound parity ---
	leftPaddle := false
	rightPaddle := false
	if forward {
		leftPaddle = true
		rightPaddle = true
	} else if right {
		leftPaddle = true
	} else if left {
		rightPaddle = true
	}
	if err := versionHandler.Play().Movement().SendBoatPaddleState(
		pe.movementPacketSender.client.Conn(),
		leftPaddle, rightPaddle,
	); err != nil {
		log.Printf("[handleRidingMode] Failed to send boat paddle state packet: %v", err)
	}

	// Hold lock for entire read-compute-write cycle
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	// --- Boat physics (vanilla tick order) ---
	currentPos, yaw, pitch, _ := pe.physicsState.GetPosition()
	blockBelowBoat := pe.getBlockBelowBoat(currentPos.X, currentPos.Y, currentPos.Z)
	velMultiplier, gravityVal := pe.getBoatPhysicsValues(currentPos.X, currentPos.Y, currentPos.Z, blockBelowBoat)

	// Read current velocity
	ridingVelX := pe.ridingVelX
	ridingVelZ := pe.ridingVelZ
	boatYawVelocity := pe.boatYawVelocity

	// (1) Drag: velocity *= multiplier, yawVelocity *= multiplier
	ridingVelX *= velMultiplier
	ridingVelZ *= velMultiplier
	boatYawVelocity *= velMultiplier

	// (2) Input → angular acceleration: left/right modify yawVelocity ±1 deg/tick
	if left {
		boatYawVelocity--
	}
	if right {
		boatYawVelocity++
	}

	// (3) Input → thrust magnitude along heading
	var thrustSpeed float64
	if forward {
		thrustSpeed += physics.BoatForwardAcceleration // +0.04
	}
	if backward {
		thrustSpeed -= physics.BoatBackwardAcceleration // -0.005
	}

	// (4) Apply rotation: yaw += yawVelocity
	yaw += boatYawVelocity

	// (5) Project thrust through yaw into world-space velocity
	yawRad := yaw * math.Pi / 180.0
	ridingVelX += math.Sin(-yawRad) * thrustSpeed
	ridingVelZ += math.Cos(yawRad) * thrustSpeed

	// (6) Vanilla Entity.resetVelocityIfSmall: clamp tiny velocities to zero
	if math.Abs(ridingVelX) < physics.ResetVelocity {
		ridingVelX = 0
	}
	if math.Abs(ridingVelZ) < physics.ResetVelocity {
		ridingVelZ = 0
	}
	if math.Abs(boatYawVelocity) < physics.ResetVelocity {
		boatYawVelocity = 0
	}

	// Vertical: gravity unless floating in water (server maintains buoyancy).
	var velocityY float64
	if pe.shapeProvider != nil && pe.shapeProvider.IsWater(blockBelowBoat) {
		velocityY = 0.0
	} else {
		velocityY = gravityVal
	}

	newX := currentPos.X + ridingVelX
	newY := currentPos.Y + velocityY
	newZ := currentPos.Z + ridingVelZ
	onGround := false

	// Update shared velocity state (still holding lock)
	pe.ridingVelX = ridingVelX
	pe.ridingVelZ = ridingVelZ
	pe.boatYawVelocity = boatYawVelocity
	pe.lastVelMultiplier = velMultiplier

	log.Printf("[handleRidingMode] Boat physics: surface=%s drag=%.3f yaw=%.1f yawVel=%.2f thrust=%.4f vel=(%.4f,%.4f) pos=(%.2f,%.2f,%.2f)",
		pe.getBlockNameForBoat(blockBelowBoat), velMultiplier, yaw, boatYawVelocity, thrustSpeed, ridingVelX, ridingVelZ, newX, newY, newZ)

	newPos := models.V3{X: newX, Y: newY, Z: newZ}

	pe.physicsState.SetPosition(newPos, yaw, pitch, onGround)
	pe.movementPacketSender.setBotPosition(newPos, yaw, pitch)

	if err := versionHandler.Play().Movement().SendMoveVehicle(
		pe.movementPacketSender.client.Conn(),
		newX, newY, newZ,
		yaw, pitch,
		onGround,
	); err != nil {
		log.Printf("[handleRidingMode] Failed to send vehicle move packet: %v", err)
	}
}

// getBlockBelowBoat returns the block state directly below the boat's current position.
// This is used to determine what surface the boat is on.
func (pe *PhysicsMovementExecutor) getBlockBelowBoat(boatX, boatY, boatZ float64) uint32 {
	if pe.world == nil {
		return 0 // Air - assume boat is in water if we can't check
	}

	// Check block directly below boat (round down Y coordinate)
	blockX := int(math.Floor(boatX))
	blockY := int(math.Floor(boatY - 0.1)) // Slightly below to get the surface block
	blockZ := int(math.Floor(boatZ))

	blockState, loaded := pe.world.GetBlockStatus(blockX, blockY, blockZ)
	if !loaded {
		return 0 // Not loaded, assume air
	}
	return blockState
}

// getBoatPhysicsValues returns the velocity multiplier and gravity for the boat
// based on its current environment (water, flowing water, or land with different surfaces).
// Per Minecraft 1.21.10 source: AbstractBoatEntity.java
func (pe *PhysicsMovementExecutor) getBoatPhysicsValues(boatX, boatY, boatZ float64, blockBelowBoat uint32) (float64, float64) {
	if pe.shapeProvider == nil {
		// Fallback: assume boat is in water
		return physics.BoatInWaterVelocityMultiplier, physics.BoatInWaterGravity
	}

	// Check if boat is in water
	isWater := pe.shapeProvider.IsWater(blockBelowBoat)
	if isWater {
		// Boat is in water - check if it's under flowing water (current affects gravity)
		isFlowing := pe.shapeProvider.GetWaterFlowSpeed(blockBelowBoat) > 0.0
		if isFlowing {
			// Under flowing water: reduced gravity due to strong current
			return physics.BoatUnderFlowingWaterVelocityMultiplier, physics.BoatUnderFlowingWaterGravity
		}

		// Check if boat is fully submerged.
		blockAbove := int(math.Floor(boatY)) + 1
		blockX := int(math.Floor(boatX))
		blockZ := int(math.Floor(boatZ))
		blockAboveState, loaded := pe.world.GetBlockStatus(blockX, blockAbove, blockZ)
		if loaded && pe.shapeProvider.IsWater(blockAboveState) {
			// Fully submerged: much slower movement
			return physics.BoatUnderWaterVelocityMultiplier, physics.BoatUnderWaterGravity
		}

		// Standard water (not flowing, not submerged)
		return physics.BoatInWaterVelocityMultiplier, physics.BoatInWaterGravity
	}

	// Boat is on land - check block type for slipperiness
	blockName := pe.shapeProvider.BlockName(blockBelowBoat)
	switch blockName {
	case "minecraft:blue_ice":
		return physics.BoatOnLandBlueIceVelocityMultiplier, physics.BoatOnLandGravity
	case "minecraft:ice", "minecraft:packed_ice":
		return physics.BoatOnLandIceVelocityMultiplier, physics.BoatOnLandGravity
	default:
		// Standard land surface
		return physics.BoatOnLandStandardVelocityMultiplier, physics.BoatOnLandGravity
	}
}

// getBlockNameForBoat returns a human-readable name for the boat's surface for logging.
func (pe *PhysicsMovementExecutor) getBlockNameForBoat(blockState uint32) string {
	if pe.shapeProvider == nil {
		return "unknown"
	}

	if pe.shapeProvider.IsWater(blockState) {
		flowSpeed := pe.shapeProvider.GetWaterFlowSpeed(blockState)
		if flowSpeed > 0.0 {
			return fmt.Sprintf("flowing_water(%.2f)", flowSpeed)
		}
		return "water"
	}

	blockName := pe.shapeProvider.BlockName(blockState)
	if blockName == "" {
		return "air"
	}
	return blockName
}
