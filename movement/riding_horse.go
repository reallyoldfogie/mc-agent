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
//
// Jump handling mirrors the vanilla client (ClientPlayerEntity.tickMovement +
// AbstractHorseEntity.tickControlled/jump). Horse jump motion is client-
// authoritative: the client arms jumpStrength by holding/releasing the jump key
// (the MountJumpStrength ramp), and tickControlled fires jump() on the next
// on-ground tick, setting velocity.y = JUMP_STRENGTH * strength. The resulting
// position is shipped to the server via VehicleMove; the START_RIDING_JUMP
// packet (sent via sendRidingInput's jump flag) only drives animation/anger.
func (pe *PhysicsMovementExecutor) handleRidingModeHorse(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	// (1) Send PlayerInput (carries the jump flag → START_RIDING_JUMP on release
	// in vanilla; the server uses it only for animation/anger).
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

	// An entity is airborne while it has upward Y velocity or is no longer
	// supported by ground below. Probing for ground support (instead of relying
	// only on prior velocity) lets an entity that walks off a ledge with zero
	// vertical velocity begin to fall. While airborne, gravity + air drag apply
	// (mirrors Java travelMidAir); a supported entity has its Y velocity zeroed
	// on landing below.
	grounded := ridingHasGroundSupport(pe, currentPos, 1.4, 1.6)
	ridingAirborne := !waterParams.IsInWater && (pe.ridingVelY > 0.001 || !grounded)

	// NOTE: Don't zero pe.ridingVelY on the ground branch. We haven't yet confirmed
	// via collision that we're actually on ground. When walking off a cliff,
	// ridingAirborne is initially false (velocity hasn't changed yet), so we'd
	// incorrectly zero Y velocity before gravity has a chance to apply. Instead,
	// zero it after collision confirms we're on solid ground (see landing below).
	surface := mountSurfaceGround
	switch {
	case waterParams.IsInWater:
		surface = mountSurfaceWater
	case ridingAirborne:
		surface = mountSurfaceAirborne
	}
	velocityDrag, gravityDelta := mountSurfacePhysics(
		surface,
		waterParams.VelocityDrag,
		waterParams.GravityDelta,
		physics.GetBlockSlipperiness(waterParams.BlockBelowEntity),
	)

	// (4) Movement speed from entity attribute
	movementAcceleration := 0.225 // Default: vanilla horse movement speed
	if entityGetter != nil {
		if movementSpeed, ok := entityGetter.GetEntityAttribute(mountedEntityID, "generic.movement_speed"); ok {
			movementAcceleration = movementSpeed
		}
	}

	// (5) Jump charge-on-hold / release-to-fire state machine.
	// Mirrors ClientPlayerEntity.tickMovement: rising edge starts charging, hold
	// accumulates ticks, release arms the strength (via the MountJumpStrength
	// ramp → floor(*100) → ClampJumpStrength). The armed strength is applied on
	// the next on-ground tick below, matching AbstractHorseEntity.tickControlled
	// (jumpStrength > 0 && isOnGround() && !isJumping() → jump()).
	if jump && !pe.lastRidingJumpState {
		pe.horseCharging = true
		pe.horseJumpChargeTicks = 0
		log.Printf("[handleRidingModeHorse] Jump charging started")
	}
	if pe.horseCharging {
		if jump {
			pe.horseJumpChargeTicks++
			if pe.horseJumpChargeTicks > mountJumpChargeCapTicks {
				pe.horseJumpChargeTicks = mountJumpChargeCapTicks
			}
		}
		// Fire on release (or when the charge saturates at the cap).
		if !jump || pe.horseJumpChargeTicks >= mountJumpChargeCapTicks {
			strengthPercent, strength := mountJumpChargeStrength(pe.horseJumpChargeTicks)
			pe.horsePendingJumpStrength = strength
			pe.horseCharging = false
			// Tell the server about the released jump so the server-side horse
			// jumps too (vanilla LocalPlayer.sendRidingJump on key release).
			sendRidingJumpCommand(pe, versionHandler, strengthPercent)
			log.Printf("[handleRidingModeHorse] Jump armed: chargeTicks=%d strengthPercent=%d strength=%.3f",
				pe.horseJumpChargeTicks, strengthPercent, pe.horsePendingJumpStrength)
		}
	}

	// (6) Apply an armed jump on the next on-ground tick (Java tickControlled):
	//   velocity.y = getJumpVelocity(strength) = JUMP_STRENGTH * strength
	// and, when moving forward, add the 0.4*strength forward boost (Java jump()).
	// Reset the armed strength after firing so it doesn't repeat.
	if pe.horsePendingJumpStrength > 0 && !waterParams.IsInWater && !ridingAirborne {
		jumpVel := physics.HorseBaseJumpStrength * pe.horsePendingJumpStrength
		pe.ridingVelY = jumpVel
		ridingAirborne = true
		// Recompute drag/gravity for the now-airborne entity.
		velocityDrag, gravityDelta = mountSurfacePhysics(mountSurfaceAirborne, 0, 0, 0)

		// Forward boost (Java: only when movementInput.z > 0). inputs.ThrottleZ
		// is the forward input magnitude; the boost is added to the scalar forward
		// velocity (ridingVelZ), which the move step below rotates by yaw.
		if inputs.ThrottleZ > 0 {
			boost := physics.HorseJumpForwardBoost * pe.horsePendingJumpStrength * inputs.ThrottleZ
			// Add to the existing horizontal velocity (Java adds to velocity).
			pe.ridingVelZ += boost
		}

		log.Printf("[handleRidingModeHorse] Jump fired: strength=%.3f jumpVel=%.4f forwardBoost=%.4f",
			pe.horsePendingJumpStrength, jumpVel, physics.HorseJumpForwardBoost*pe.horsePendingJumpStrength)
		pe.horsePendingJumpStrength = 0
	}

	// (7) Velocity computation. While airborne, movement input provides only
	// limited air control; applying full ground movement speed in the air would
	// let the horse accelerate to several times its ground speed and fly forward
	// while falling.
	inputAccel := mountInputAcceleration(movementAcceleration, ridingAirborne)
	ridingVelZ := mountForwardVelocity(pe.ridingVelZ, velocityDrag, inputs.ThrottleZ, inputAccel)
	ridingVelX := 0.0 // Horses don't strafe; ThrottleX is steering
	ridingVelY := mountVerticalVelocity(pe.ridingVelY, gravityDelta, ridingAirborne)

	// (8) Position computation via collision detection
	// Horse/donkey/mule dimensions: 1.4 wide × 1.6 tall
	moveVelX, moveVelZ := forwardVelocityToWorld(ridingVelZ, yaw)
	moveVel := models.V3{
		X: moveVelX,
		Y: ridingVelY,
		Z: moveVelZ,
	}
	newPos, correctedVel, collisionOnGround, _, _ := resolveEntityCollision(pe, currentPos, moveVel, 1.4, 1.6)
	// onGround reflects actual ground support: either collision stopped a
	// downward move this tick, or the ground-support probe found a block just
	// beneath us (e.g. while walking on flat ground with zero Y velocity).
	onGround := collisionOnGround || grounded

	// Landing: collision stopped a downward move, so we've settled onto solid
	// ground. Zero the accumulated fall velocity and record the new ground
	// level. Landing is driven purely by collision now — the old "fell back to
	// recorded groundY" check would halt a genuine cliff fall at the pre-fall
	// height instead of letting the entity drop.
	if collisionOnGround {
		correctedVel.Y = 0
		onGround = true
		pe.ridingGroundY = newPos.Y
		log.Printf("[handleRidingModeHorse] Landed at Y=%.2f", newPos.Y)
	}

	// Use corrected velocity for next tick (horizontal clamped by collision)
	ridingVelZ = math.Sqrt(correctedVel.X*correctedVel.X + correctedVel.Z*correctedVel.Z)
	ridingVelY = correctedVel.Y

	// Update shared velocity state
	pe.ridingVelX = ridingVelX
	pe.ridingVelZ = ridingVelZ
	pe.ridingVelY = ridingVelY
	pe.lastVelMultiplier = velocityDrag
	pe.lastRidingJumpState = jump

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

	log.Printf("[handleRidingModeHorse] physics: water=%v behavior=%s airborne=%v yaw=%.1f throttle=(%.2f,%.2f) accel=%.4f drag=%.3f velZ=%.4f velY=%.4f newPos=(%.2f,%.2f,%.2f)",
		waterParams.IsInWater, waterBehavior, ridingAirborne, yaw, inputs.ThrottleX, inputs.ThrottleZ, movementAcceleration, velocityDrag, ridingVelZ, ridingVelY, newPos.X, newPos.Y, newPos.Z)

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
