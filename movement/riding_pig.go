package movement

import (
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// handleRidingModePig runs one tick for a ridden pig.
//
// Pigs use carrot_on_a_stick for speed boost and direction control. Holding the
// item makes the rider the controlling passenger (getControllingPassenger);
// the sinusoidal speed boost is applied only while a SaddledComponent boost is
// active, which is armed by *using* (right-click) the carrot_on_a_stick via
// TriggerSaddleBoost. Movement is otherwise identical to horses:
// slipperiness-based friction, water sinking, and yaw-rotation steering.
func (pe *PhysicsMovementExecutor) handleRidingModePig(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	// (1) Send PlayerInput
	log.Printf("[handleRidingModePig] SendVehicleInput(<conn>, forward: %t, backward: %t, left: %t, right: %t, jump: %t, sneak: %t)", forward, backward, left, right, jump, sneak)
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

	// (4) Movement speed from entity attribute, with carrot boost
	// Java PigEntity.getSaddledSpeed():
	//
	//	return getAttributeValue(MOVEMENT_SPEED) * 0.225 * saddledComponent.getMovementSpeedMultiplier();
	//
	// The 0.225 saddled multiplier is always applied for a ridden pig. The sinusoidal
	// boost multiplier (1.0 + 1.15*sin(...)) comes from the SaddledComponent and is
	// triggered by *using* (right-click) the carrot_on_a_stick, not merely holding it.
	// Holding the item only makes the player the controlling passenger (handled
	// server-side via getControllingPassenger); it does not change speed.
	attributeSpeed := physics.PigBaseMovementSpeed // Default pig speed
	if entityGetter != nil {
		if movementSpeed, ok := entityGetter.GetEntityAttribute(mountedEntityID, "generic.movement_speed"); ok {
			attributeSpeed = movementSpeed
		}
	}
	movementAcceleration := attributeSpeed * physics.PigSaddledSpeedMultiplier

	// Apply the SaddledComponent sinusoidal boost when active. Mirrors Java
	// SaddledComponent.getMovementSpeedMultiplier():
	//
	//	1.0F + 1.15F * sin(boostedTime / boostTime * PI)
	// The boost is armed via TriggerSaddleBoost (carrot_on_a_stick use).
	carrotBoost := false
	boostMultiplier := 1.0
	if pe.saddleBoosted && pe.saddleBoostTotal > 0 {
		carrotBoost = true
		boostMultiplier = 1.0 + physics.PigBoostSinAmplitude*math.Sin(float64(pe.saddleBoostTime)/float64(pe.saddleBoostTotal)*math.Pi)
	}
	movementAcceleration *= boostMultiplier

	// Holding carrot_on_a_stick makes the player the controlling passenger, but
	// does not by itself change speed. We track it for logging/test visibility only.
	holdsCarrotOnAStick := false
	if entityGetter != nil {
		if heldItem, found := entityGetter.GetRiderHeldItem(); found && heldItem == "carrot_on_a_stick" {
			holdsCarrotOnAStick = true
		}
	}

	// (5) Velocity computation
	ridingVelZ := pe.ridingVelZ*velocityDrag + inputs.ThrottleZ*movementAcceleration
	ridingVelX := 0.0 // Pigs don't strafe; ThrottleX is steering
	ridingVelY := pe.ridingVelY + gravityDelta

	if math.Abs(ridingVelZ) < physics.ResetVelocity {
		ridingVelZ = 0
	}

	// (6) Position computation via collision detection
	// Pig dimensions: 0.9 wide × 0.9 tall
	yawRad := yaw * math.Pi / 180.0
	moveVel := models.V3{
		X: -math.Sin(yawRad) * ridingVelZ,
		Y: ridingVelY,
		Z: math.Cos(yawRad) * ridingVelZ,
	}
	newPos, correctedVel, collisionOnGround, _, _ := resolveEntityCollision(pe, currentPos, moveVel, 0.9, 0.9)
	onGround := collisionOnGround || (!waterParams.IsInWater && ridingVelY >= 0)

	// Use corrected velocity for next tick
	ridingVelZ = math.Sqrt(correctedVel.X*correctedVel.X + correctedVel.Z*correctedVel.Z)
	ridingVelY = correctedVel.Y

	// Update shared velocity state
	pe.ridingVelX = ridingVelX
	pe.ridingVelZ = ridingVelZ
	pe.ridingVelY = ridingVelY
	pe.lastVelMultiplier = velocityDrag

	// Auto-dismount when submerged
	checkRiderHeadSubmerged(pe, mountedEntityID, waterParams, newPos.X, newPos.Y, newPos.Z)

	waterBehavior := "none"
	if waterParams.IsInWater {
		if waterParams.ShouldApplyOldSinkingBehavior {
			waterBehavior = "sink_pre1.21.11"
		} else {
			waterBehavior = "float_1.21.11+"
		}
	}

	log.Printf("[handleRidingModePig] holds_carrot=%v boost_active=%v boostMul=%.3f accel=%.4f drag=%.3f velZ=%.4f velY=%.4f yaw=%.1f throttle=(%.2f,%.2f) water=%v behavior=%s newPos=(%.2f,%.2f,%.2f)",
		holdsCarrotOnAStick, carrotBoost, boostMultiplier, movementAcceleration, velocityDrag, ridingVelZ, ridingVelY, yaw, inputs.ThrottleX, inputs.ThrottleZ, waterParams.IsInWater, waterBehavior, newPos.X, newPos.Y, newPos.Z)

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
