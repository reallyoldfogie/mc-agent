package movement

import (
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// striderVelocityZeroThreshold mirrors the Java LivingEntity.tickMovement()
// threshold for zeroing tiny velocities on non-player entities.
// Java: if (Math.abs(vec3d.x) < 0.003) d = 0.0; (same for z and y).
const striderVelocityZeroThreshold = 0.003

// canMoveVoluntarily mirrors Java LivingEntity/MobEntity.canMoveVoluntarily().
// In vanilla, this returns false when the entity has a status effect that prevents
// voluntary movement (e.g. levitation). When false, the entity should not execute
// its normal travel/movement logic, though rotation updates still occur.
//
// Stub: returns true because the movement executor does not yet track status effects.
// When status effect data is wired into the executor, replace this with a real check.
func canMoveVoluntarily() bool {
	return true
}

// handleRidingModeStrider runs one tick for a ridden strider.
//
// Mirrors the Java 1.21.11 call chain:
//   tickMovement() → travelControlled(player, vec3d2)
//     → getControlledMovementInput() returns (0,0,1)  [always forward, ignores actual inputs]
//     → tickControlled(): snap yaw to player's yaw, pitch * 0.5, tick boost
//     → setMovementSpeed(getSaddledSpeed(player))
//     → travel(vec3d) → travelMidAir() (because canWalkOnFluid returns true for lava)
//   tick() → updateFloating(): set onGround when floating, or bob when submerged
//
// Note: The `inputs` parameter is ignored for movement computation because Java's
// getControlledMovementInput() always returns (0, 0, 1) regardless of the actual input.
// The forward/backward/left/right booleans are used only for the VehicleInput packet.
func (pe *PhysicsMovementExecutor) handleRidingModeStrider(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	// (1) Send PlayerInput
	log.Printf("[handleRidingModeStrider] SendVehicleInput(<conn>, forward: %t, backward: %t, left: %t, right: %t, jump: %t, sneak: %t)", forward, backward, left, right, jump, sneak)
	sendRidingInput(pe, versionHandler, forward, backward, left, right, jump, sneak)

	// Hold lock for entire read-compute-write cycle
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	currentPos, yaw, pitch, prevOnGround := pe.physicsState.GetPosition()

	// ── Ground support detection ──
	// Java LivingEntity.isOnGround() is true whenever the entity is supported by
	// a solid surface OR (for the strider) floating on lava. The previous
	// implementation derived isOnGround ONLY from lavaParams.IsOnLava, which
	// made a strider standing on land appear airborne every tick. That forced
	// the off-ground speed branch (getOffGroundSpeed = movementSpeed * 0.1),
	// collapsing per-tick acceleration to ~0.004 — below the 0.003 zeroing
	// threshold — so the strider crept at ~0.25 blocks over 3 seconds on BOTH
	// land and lava. Mirror Java by treating supported-on-solid (from the prior
	// tick's collision result, confirmed by a solid block below) and
	// floating-on-lava as on-ground sources.
	solidBlockBelow := pe.shapeProvider != nil && pe.shapeProvider.IsSolid(pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z))
	supportedOnGround := prevOnGround && solidBlockBelow

	// ── tickControlled: snap strider yaw/pitch to player's ──
	// Java: this.setRotation(controllingPlayer.getYaw(), controllingPlayer.getPitch() * 0.5F)
	// We use the agent's current yaw/pitch as the "player" values since we ARE the player.
	// Pitch is halved per Java.
	pitch = pitch * 0.5

	// ── tickControlled: tick the SaddledComponent boost ──
	// Java SaddledComponent.tickBoost(): if boosted && boostedTime++ > boostTime → boosted = false
	// Note: Java uses post-increment, so the condition checks the OLD value against boostTime.
	// This means the boost continues for one extra tick compared to a pre-increment check.
	if pe.saddleBoosted {
		oldBoostTime := pe.saddleBoostTime
		pe.saddleBoostTime++
		if oldBoostTime > pe.saddleBoostTotal {
			pe.saddleBoosted = false
		}
	}

	// ── Determine cold state ──
	// Java StriderEntity.tick():
	//   bl = blockState.isIn(BlockTags.STRIDER_WARM_BLOCKS) || landing.isIn(BlockTags.STRIDER_WARM_BLOCKS) || fluidHeight(LAVA) > 0
	//   bl2 = getVehicle() instanceof StriderEntity && striderEntity.isCold()
	//   setCold(!bl || bl2)
	lavaParams := computeRidingLavaPhysics(pe, currentPos)
	cold := computeStriderColdState(pe, currentPos, lavaParams, entityGetter)

	// ── canMoveVoluntarily guard ──
	// Java LivingEntity.tickMovement(): if (this.canMoveVoluntarily()) { this.travel(vec3d); }
	// When the entity is immobilized (e.g. by levitation), travel/movement is skipped.
	if !canMoveVoluntarily() {
		// Rotation updates still happen even when movement is suppressed.
		newPos := models.V3{X: currentPos.X, Y: currentPos.Y, Z: currentPos.Z}
		return ridingTickResult{
			NewPos:      newPos,
			OnGround:    true,
			Sneak:       sneak,
			StateYaw:    yaw,
			StatePitch:  pitch,
			PacketYaw:   yaw,
			PacketPitch: pitch,
		}
	}

	// ── getSaddledSpeed: compute the saddled movement speed ──
	// Java: getAttributeValue(MOVEMENT_SPEED) * (cold ? 0.35 : 0.55) * saddledComponent.getMovementSpeedMultiplier()
	attributeSpeed := physics.StriderBaseMovementSpeed
	if entityGetter != nil {
		if movementSpeed, ok := entityGetter.GetEntityAttribute(mountedEntityID, "generic.movement_speed"); ok {
			attributeSpeed = movementSpeed
		}
	}

	// Apply SUFFOCATING_MODIFIER when cold.
	// Java: ADD_MULTIPLIED_BASE means effective = base * (1 + modifier)
	if cold {
		attributeSpeed *= (1.0 + physics.StriderSuffocatingModifier)
	}

	// Apply cold/warm speed multiplier
	var speedMultiplier float64
	if cold {
		speedMultiplier = physics.StriderColdSpeedMultiplier
	} else {
		speedMultiplier = physics.StriderWarmSpeedMultiplier
	}

	// Apply SaddledComponent boost multiplier
	// Java: 1.0F + 1.15F * sin(boostedTime / boostTime * PI)
	boostMultiplier := 1.0
	if pe.saddleBoosted && pe.saddleBoostTotal > 0 {
		boostMultiplier = 1.0 + physics.StriderBoostSinAmplitude*math.Sin(float64(pe.saddleBoostTime)/float64(pe.saddleBoostTotal)*math.Pi)
	}

	saddledSpeed := attributeSpeed * speedMultiplier * boostMultiplier

	// ── travelMidAir physics ──
	// Java: slipperiness = onGround ? blockBelow.getSlipperiness() : 1.0
	//       friction = slipperiness * 0.91
	//       speedFactor = onGround ? movementSpeed * (0.216 / slip^3) : offGroundSpeed
	//       updateVelocity(speedFactor, input)  // adds to velocity
	//       move(SELF, velocity)                // collision detection
	//       velocity.xz *= friction             // post-movement drag
	//       velocity.y -= gravity               // gravity
	// Strider always uses travelMidAir physics because canWalkOnFluid(lava) returns true
	// in Java, which makes isTravellingInFluid() return false, bypassing travelInLava.
	// This is functionally correct even though the dispatch path differs from other lava entities.
	slipperiness := physics.DefaultSlipperiness
	if !lavaParams.IsOnLava {
		blockBelowEntity := pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z)
		slipperiness = physics.GetBlockSlipperiness(blockBelowEntity)
	}
	friction := slipperiness * physics.Inertia // slip * 0.91

// Java LivingEntity.getMovementSpeed():
//   onGround ? movementSpeed * (0.216 / slip³) : getOffGroundSpeed()
// Java LivingEntity.getOffGroundSpeed():
//   controllingPassenger instanceof Player ? movementSpeed * 0.1 : 0.02
// isOnGround mirrors Java LivingEntity.isOnGround(): supported on a solid surface
// OR (for the strider) floating on lava. The previous code derived isOnGround solely
// from lavaParams.IsOnLava, which made a strider standing on solid land appear
// airborne every tick and collapsed its speed to the 0.1 off-ground factor. Folding
// in supportedOnGround (prior-tick ground collision confirmed by a solid block
// below) lets a land-bound strider use the full on-ground speed factor so it
// actually moves — matching the cold/land movement Java produces.
var speedFactor float64
isOnGround := supportedOnGround || lavaParams.IsOnLava
if isOnGround {
	slipCubed := slipperiness * slipperiness * slipperiness
	speedFactor = saddledSpeed * (0.21600002 / slipCubed)
} else {
	speedFactor = saddledSpeed * 0.1
}

	// getControlledMovementInput always returns (0, 0, 1) — constant forward
	// movementInputToVelocity with input (0,0,1) and speed:
	//   result = (-speed * sin(yaw), 0, speed * cos(yaw))
	yawRad := yaw * math.Pi / 180.0
	accelX := -math.Sin(yawRad) * speedFactor
	accelZ := math.Cos(yawRad) * speedFactor

	// Compute pre-move velocity (updateVelocity: velocity += accel)
	velX := pe.ridingVelX + accelX
	velZ := pe.ridingVelZ + accelZ

	// Compute Y velocity based on lava state
	// Java: travelMidAir applies gravity before move(); updateFloating overrides after move()
	var velY float64
	switch {
	case lavaParams.IsOnLava:
		velY = 0.0
	case lavaParams.IsSubmergedInLava:
		velY = pe.ridingVelY*physics.StriderLavaBobDrag + physics.StriderLavaBobUpVelocity
	default:
		velY = pe.ridingVelY + physics.StriderOffLavaGravity
	}

	// move(SELF, velocity) — collision detection for the ridden entity
	// Strider dimensions: 0.9 wide × 1.7 tall
	moveVel := models.V3{X: velX, Y: velY, Z: velZ}
	newPos, correctedVel, collisionOnGround, _, _ := resolveEntityCollision(pe, currentPos, moveVel, 0.9, 1.7)

	// Post-movement friction: velocity.xz *= friction
	velX = correctedVel.X * friction
	velZ = correctedVel.Z * friction

	// Y-axis: apply drag and lava state overrides
	// Java: travelMidAir applies Y drag after move; updateFloating overrides for lava
	onGround := collisionOnGround
	switch {
	case lavaParams.IsOnLava:
		// Floating on lava surface overrides collision-based onGround
		velY = 0.0
		onGround = true
	case lavaParams.IsSubmergedInLava:
		// updateFloating: additional 0.5 drag on all axes (on top of friction)
		velX *= physics.StriderLavaBobDrag
		velZ *= physics.StriderLavaBobDrag
		velY = correctedVel.Y * physics.StriderLavaBobDrag
		onGround = false
	default:
		// Off lava: Y-velocity drag (0.98) - matches Java travelMidAir
		velY = correctedVel.Y * 0.98
	}

	// ── Velocity zeroing (Java LivingEntity.tickMovement) ──
	// Non-player entities: zero X/Z when abs < 0.003, zero Y when abs < 0.003
	if math.Abs(velX) < striderVelocityZeroThreshold {
		velX = 0
	}
	if math.Abs(velZ) < striderVelocityZeroThreshold {
		velZ = 0
	}
	if math.Abs(velY) < striderVelocityZeroThreshold {
		velY = 0
	}

	// Update shared velocity state
	pe.ridingVelX = velX
	pe.ridingVelZ = velZ
	pe.ridingVelY = velY
	pe.lastVelMultiplier = friction

	log.Printf("[handleRidingModeStrider] cold=%v boost=%.2f saddledSpeed=%.4f speedFactor=%.4f friction=%.3f vel=(%.4f,%.4f,%.4f) yaw=%.1f onLava=%v submerged=%v newPos=(%.2f,%.2f,%.2f)",
		cold, boostMultiplier, saddledSpeed, speedFactor, friction, velX, velY, velZ, yaw,
		lavaParams.IsOnLava, lavaParams.IsSubmergedInLava, newPos.X, newPos.Y, newPos.Z)

	// The VehicleMove send happens in handleRidingTick via sendRidingMove, after
	// mountedEntityMu is released. State and packet pose are identical for striders.
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
