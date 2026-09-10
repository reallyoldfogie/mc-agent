package movement

import (
	"fmt"
	"github.com/reallyoldfogie/mc-agent/utils"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
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
) ridingTickResult {
	// --- Send vehicle input packet (SteerVehicle on 1.21.1, PlayerInput on 1.21.2+) ---
	if err := versionHandler.Play().Movement().SendVehicleInput(
		pe.movementPacketSender.client.Conn(),
		forward, backward, left, right, false, sneak,
	); err != nil {
		utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingMode] Failed to send boat vehicle input packet: %v", err))
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
		utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingMode] Failed to send boat paddle state packet: %v", err))
	}

	// Hold lock for entire read-compute-write cycle
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	// --- Boat physics (vanilla tick order) ---
	currentPos, yaw, pitch, _ := pe.physicsState.GetPosition()
	blockBelowBoat := pe.getBlockBelowBoat(currentPos.X, currentPos.Y, currentPos.Z)
	surface := pe.getBoatSurface(currentPos.X, currentPos.Y, currentPos.Z, blockBelowBoat)
	velMultiplier, gravityVal := boatSurfacePhysics(surface)

	// Read current velocity
	ridingVelX := pe.ridingVelX
	ridingVelZ := pe.ridingVelZ
	boatYawVelocity := pe.boatYawVelocity

	// (1) Drag: velocity *= multiplier, yawVelocity *= multiplier
	ridingVelX *= velMultiplier
	ridingVelZ *= velMultiplier
	boatYawVelocity *= velMultiplier

	// (2) Input → angular acceleration: left/right modify yawVelocity ±1 deg/tick
	boatYawVelocity += boatYawAcceleration(left, right)

	// (3) Input → thrust magnitude along heading
	thrustSpeed := boatThrustSpeed(forward, backward)

	// (4) Apply rotation: yaw += yawVelocity
	yaw += boatYawVelocity

	// (5) Project thrust through yaw into world-space velocity
	yawRad := yaw * math.Pi / 180.0
	ridingVelX += math.Sin(-yawRad) * thrustSpeed
	ridingVelZ += math.Cos(yawRad) * thrustSpeed

	// (6) Vanilla Entity.resetVelocityIfSmall: clamp tiny velocities to zero
	ridingVelX = zeroTinyVelocity(ridingVelX)
	ridingVelZ = zeroTinyVelocity(ridingVelZ)
	boatYawVelocity = zeroTinyVelocity(boatYawVelocity)

	// Vertical: gravity unless floating in water (server maintains buoyancy).
	var velocityY float64
	if pe.shapeProvider != nil && pe.shapeProvider.IsWater(blockBelowBoat) {
		velocityY = 0.0
	} else {
		velocityY = gravityVal
	}

	// Position computation via collision detection
	// Boat dimensions: 1.4 wide × 0.6 tall
	moveVel := models.V3{X: ridingVelX, Y: velocityY, Z: ridingVelZ}
	newPos, correctedVel, _, _, _ := resolveEntityCollision(pe, currentPos, moveVel, 1.4, 0.6)
	onGround := false

	// Update shared velocity state (still holding lock)
	pe.ridingVelX = correctedVel.X
	pe.ridingVelZ = correctedVel.Z
	pe.boatYawVelocity = boatYawVelocity
	pe.lastVelMultiplier = velMultiplier

	utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingMode] Boat physics: surface=%s block=%s drag=%.3f yaw=%.1f yawVel=%.2f thrust=%.4f vel=(%.4f,%.4f) pos=(%.2f,%.2f,%.2f)",
		surface, pe.getBlockNameForBoat(blockBelowBoat), velMultiplier, yaw, boatYawVelocity, thrustSpeed, correctedVel.X, correctedVel.Z, newPos.X, newPos.Y, newPos.Z))

	// Update physicsState inside the lock before returning so sendRidingMove
	// (called outside the lock) cannot race with a concurrent TurnTowards.
	applyRidingTickState(pe, newPos, yaw, pitch, onGround)
	return ridingTickResult{
		NewPos:      newPos,
		OnGround:    onGround,
		Sneak:       sneak,
		StateYaw:    yaw,
		StatePitch:  pitch,
		PacketYaw:   yaw,
		PacketPitch: pitch,
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

// getBoatSurface resolves the world around the boat into a boatSurface profile.
// It performs the block lookups; the surface-selection rules and the resulting
// drag/gravity values live in the pure helpers in riding_physics.go.
// Per Minecraft 1.21.10 source: AbstractBoatEntity.java
func (pe *PhysicsMovementExecutor) getBoatSurface(boatX, boatY, boatZ float64, blockBelowBoat uint32) boatSurface {
	if pe.shapeProvider == nil {
		// Fallback: assume boat is in water
		return boatSurfaceWater
	}

	isWater := pe.shapeProvider.IsWater(blockBelowBoat)
	isFlowing := isWater && pe.shapeProvider.GetWaterFlowSpeed(blockBelowBoat) > 0.0

	// A boat counts as submerged only when there is also water directly above it.
	isSubmerged := false
	if isWater && pe.world != nil {
		blockX := int(math.Floor(boatX))
		blockAbove := int(math.Floor(boatY)) + 1
		blockZ := int(math.Floor(boatZ))
		if blockAboveState, loaded := pe.world.GetBlockStatus(blockX, blockAbove, blockZ); loaded {
			isSubmerged = pe.shapeProvider.IsWater(blockAboveState)
		}
	}

	// The block name only matters on land, where it selects the ice variants.
	blockName := ""
	if !isWater {
		blockName = pe.shapeProvider.BlockName(blockBelowBoat)
	}

	return selectBoatSurface(isWater, isFlowing, isSubmerged, blockName)
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
