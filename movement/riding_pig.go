package movement

import (
	"fmt"
	"github.com/reallyoldfogie/mc-agent/utils"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// handleRidingModePig runs one tick for a ridden pig.
//
// Mirrors the Java 1.21.11 call chain:
//
//	tickMovement() → travelControlled(player, vec3d2)
//	  → getControlledMovementInput(): returns (0,0,1) always — pig ignores player keys
//	  → tickControlled(): pig.yaw=player.yaw, pig.pitch=player.pitch*0.5, tick boost
//	  → setMovementSpeed(getSaddledSpeed): attribute*0.225*saddledComponent.getMovementSpeedMultiplier()
//	  → travel(vec3d) → travelMidAir (land/air) or travelInFluid (water)
//	floatIfRidden(): +0.04 Y/tick upward push when CAN_FLOAT_WHILE_RIDDEN and deep (1.21.11+)
//
// Steering: pig.yaw is set to the player's yaw via setRotation each tick.
//
//	Since the agent IS the controlling player, the pig's yaw is taken directly
//	from physicsState without any ThrottleX-based increment. The agent steers
//	by changing its own look direction (via TurnTowards) before each tick.
//
// Direction: getControlledMovementInput always returns (0,0,1). The `forward`
//
//	bool gates whether to apply that full-forward acceleration: true = full
//	speed, false = coast to a stop. `inputs` is not used for movement
//	computation — only `forward` and the yaw from physicsState matter.
//
// Controlling item: a rider only becomes the pig's controlling passenger while
//
//	holding carrot_on_a_stick (https://minecraft.wiki/w/Pig) — without it the
//	pig ignores rider input entirely, so accelFactor is also gated on
//	holdsCarrotOnAStick below, independent of `forward`.
//
// Velocity formula (Java travelMidAir, three-step):
//
//  1. updateVelocity(speedFactor, movementInput): vel += accel
//
//  2. move(SELF, vel): collision resolution
//
//  3. vel.xz *= slip*0.91  (post-move friction)
//
//     speedFactor = saddledSpeed*(0.216/slip³) on ground
//     = saddledSpeed*0.1           when airborne
func (pe *PhysicsMovementExecutor) handleRidingModePig(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	// inputs is not used for movement computation — getControlledMovementInput
	// always returns (0,0,1). The forward/backward/left/right booleans are used
	// only for the VehicleInput packet.
	_ = inputs

	// (1) Send PlayerInput
	utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingModePig] SendVehicleInput(<conn>, forward: %t, backward: %t, left: %t, right: %t, jump: %t, sneak: %t)", forward, backward, left, right, jump, sneak))
	sendRidingInput(pe, versionHandler, forward, backward, left, right, jump, sneak)

	// Hold lock for entire read-compute-write cycle
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	currentPos, yaw, pitch, _ := pe.physicsState.GetPosition()

	// (2) tickControlled: snap pitch to player pitch * 0.5.
	// Java: this.setRotation(controllingPlayer.getYaw(), controllingPlayer.getPitch() * 0.5F)
	// Yaw is already the agent's own yaw — no modification needed (pig.yaw = player.yaw).
	pitch = pitch * 0.5

	// (3) tickControlled: tick the SaddledComponent boost.
	// Java SaddledComponent.tickBoost(): if (boosted && boostedTime++ > boostTime) boosted = false
	// Post-increment: the condition tests the old boostedTime, so the boost runs one extra tick.
	if pe.saddleBoosted {
		oldBoostTime := pe.saddleBoostTime
		pe.saddleBoostTime++
		if oldBoostTime > pe.saddleBoostTotal {
			pe.saddleBoosted = false
		}
	}

	// (4) Water/land physics detection.
	waterParams := computeRidingWaterPhysics(pe, versionHandler, currentPos)

	// (5) Airborne detection (mirrors horse handler).
	// A pig is airborne when it has upward Y velocity or is no longer supported
	// by ground. Probing for support lets a pig that walks off a ledge with zero
	// Y velocity start to fall on the same tick, rather than hovering.
	grounded := ridingHasGroundSupport(pe, currentPos, 0.9, 0.9)
	ridingAirborne := !waterParams.IsInWater && (pe.ridingVelY > 0.001 || !grounded)

	// (6) Compute saddled movement speed.
	// Java: getSaddledSpeed = attribute * 0.225 * saddledComponent.getMovementSpeedMultiplier()
	attributeSpeed := resolveMountMovementSpeed(entityGetter, mountedEntityID, "generic.movement_speed", physics.PigBaseMovementSpeed)
	saddledSpeed := attributeSpeed * physics.PigSaddledSpeedMultiplier

	// Apply sinusoidal carrot_on_a_stick boost.
	// Java SaddledComponent.getMovementSpeedMultiplier(): 1.0 + 1.15*sin(boostedTime/boostTime*PI)
	carrotBoost := pe.saddleBoosted && pe.saddleBoostTotal > 0
	boostMultiplier := saddledBoostMultiplier(pe.saddleBoosted, pe.saddleBoostTime, pe.saddleBoostTotal, physics.PigBoostSinAmplitude)
	saddledSpeed *= boostMultiplier

	// Holding carrot_on_a_stick makes the player the controlling passenger — a
	// pig ignores steering input entirely without it (https://minecraft.wiki/w/Pig,
	// confirmed empirically in docs/PIG_MOUNT_SYNC_SNAPBACK_INVESTIGATION.md). Gates
	// accelFactor below; also used for the boost calculation and logging.
	holdsCarrotOnAStick := false
	if entityGetter != nil {
		if heldItem, found := entityGetter.GetRiderHeldItem(); found && heldItem == "carrot_on_a_stick" {
			holdsCarrotOnAStick = true
		}
	}

	// (7) Per-surface speedFactor and friction.
	//
	// Java travelMidAir:
	//   getMovementSpeed(slip): onGround → movementSpeed*(0.216/slip³)
	//                           airborne → getOffGroundSpeed() = movementSpeed*0.1
	//   friction (post-move):   slip * 0.91
	//
	// Java travelInWater:
	//   g = 0.02 base speed (pig has no WATER_MOVEMENT_EFFICIENCY attribute)
	//   horizontal drag = 0.8 per tick (f in travelInWater)
	var speedFactor, friction float64

	switch {
	case waterParams.IsInWater:
		speedFactor = physics.PigInWaterBaseSpeed
		friction = waterParams.VelocityDrag
	case ridingAirborne:
		// getOffGroundSpeed with controlling player: movementSpeed * 0.1
		speedFactor = travelMidAirSpeedFactor(saddledSpeed, airSlipperiness, false)
		friction = physics.Inertia // air: slip=1.0, friction = 1.0*0.91
	default:
		// On land
		blockSlipperiness := physics.GetBlockSlipperiness(waterParams.BlockBelowEntity)
		speedFactor = travelMidAirSpeedFactor(saddledSpeed, blockSlipperiness, true)
		friction = blockSlipperiness * physics.Inertia // slip * 0.91
		// Zero Y velocity on land; collision will keep it grounded.
		pe.ridingVelY = 0.0
	}

	// (8) getControlledMovementInput always returns (0,0,1).
	// `forward` gates acceleration: true = full speed; false = coast (no accel).
	// holdsCarrotOnAStick gates it too: a rider without the carrot is not the
	// controlling passenger in vanilla and cannot steer the pig at all.
	var accelFactor float64
	if forward && holdsCarrotOnAStick {
		accelFactor = speedFactor
	}

	accelX, accelZ := forwardVelocityToWorld(accelFactor, yaw)

	// updateVelocity(speedFactor, movementInput): velocity += accel  (Java step 1)
	velX := pe.ridingVelX + accelX
	velZ := pe.ridingVelZ + accelZ

	// Y velocity per surface state.
	var velY float64
	switch {
	case waterParams.IsInWater:
		velY = pe.ridingVelY + waterParams.GravityDelta
		// floatIfRidden: pigs are in CAN_FLOAT_WHILE_RIDDEN.
		// Java: if fluidHeight > swimHeight → vel.y += 0.04F
		// Applied for 1.21.11+ floating behavior (not the pre-1.21.11 sinking path).
		if !waterParams.ShouldApplyOldSinkingBehavior {
			velY += 0.04
		}
	case ridingAirborne:
		velY = pe.ridingVelY - physics.Gravity // -0.08 per tick
	default:
		velY = 0.0 // on ground
	}

	// (9) move(SELF, velocity) — collision detection.  (Java step 2)
	// Pig dimensions: 0.9 wide × 0.9 tall.
	moveVel := models.V3{X: velX, Y: velY, Z: velZ}
	newPos, correctedVel, collisionOnGround, _, _ := resolveEntityCollision(pe, currentPos, moveVel, 0.9, 0.9)
	onGround := collisionOnGround || grounded

	// Landing: vertical collision stopped a downward move.
	if collisionOnGround {
		correctedVel.Y = 0
		onGround = true
		pe.ridingGroundY = newPos.Y
	}

	// (10) Post-move friction: velocity.xz *= friction  (Java step 3)
	velX = correctedVel.X * friction
	velZ = correctedVel.Z * friction
	velY = correctedVel.Y
	if ridingAirborne {
		velY *= physics.Drag // 0.98 Y air drag (Java: d*h, h=0.98 for non-Flutterer)
	}

	velX = zeroTinyVelocity(velX)
	velZ = zeroTinyVelocity(velZ)

	pe.ridingVelX = velX
	pe.ridingVelZ = velZ
	pe.ridingVelY = velY
	pe.lastVelMultiplier = friction

	// Auto-dismount when submerged (pre-1.21.11 behavior).
	checkRiderHeadSubmerged(pe, mountedEntityID, waterParams, newPos.X, newPos.Y, newPos.Z)

	waterBehavior := "none"
	if waterParams.IsInWater {
		if waterParams.ShouldApplyOldSinkingBehavior {
			waterBehavior = "sink_pre1.21.11"
		} else {
			waterBehavior = "float_1.21.11+"
		}
	}

	utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingModePig] holds_carrot=%v boost_active=%v boostMul=%.3f accel=%.4f friction=%.3f velX=%.4f velZ=%.4f velY=%.4f yaw=%.1f pitch=%.1f forward=%v water=%v behavior=%s newPos=(%.2f,%.2f,%.2f)",
		holdsCarrotOnAStick, carrotBoost, boostMultiplier, accelFactor, friction, velX, velZ, velY, yaw, pitch, forward, waterParams.IsInWater, waterBehavior, newPos.X, newPos.Y, newPos.Z))

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
