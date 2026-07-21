package movement

import (
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// nautilusGroundFrictionSpeedFactor is the constant from Java
// LivingEntity.getMovementSpeed(slipperiness): speed * (0.21600002 / slip^3)
// when on the ground. Kept as a named constant rather than a bare literal.
const nautilusGroundFrictionSpeedFactor = 0.21600002

// nautilusOffGroundSpeedFactor mirrors Java LivingEntity.getOffGroundSpeed():
// movementSpeed * 0.1 when the controlling passenger is a player.
const nautilusOffGroundSpeedFactor = 0.1

// nautilusDashVelocityMultiplier is the block-based velocity multiplier applied
// to a dash impulse. Java uses getVelocityMultiplier() (soul sand/honey slow);
// we assume 1.0, matching the other riding handlers.
const nautilusDashVelocityMultiplier = 1.0

// nautilusDashChargePerTick is how many charge ticks accumulate per tick while
// the jump key is held. Java's regular mounts use a charging multiplier of 1.
const nautilusDashChargePerTick = 1

// handleRidingModeNautilus runs one tick for a ridden Nautilus or ZombieNautilus
// (1.21.11+). It mirrors the Java AbstractNautilusEntity controlled-movement
// chain (LivingEntity.travelControlled):
//
//	getControlledMovementInput() → 3D pitch-based input (forward=cos, vertical=-sin)
//	tickControlled()             → ease vehicle yaw toward rider yaw, pitch*0.5, dash
//	setMovementSpeed(getSaddledSpeed())
//	travel() → travelInWater()   (in water) : vel += R_yaw(input*speed); move; vel*=0.9
//	         → travelMidAir()     (on land)  : slipperiness friction + gravity
//
// The rider (agent) look direction drives the movement input and the dash; the
// nautilus's own yaw lags behind it (eased) and is what the VehicleMove packet
// carries, with pitch halved. Position prediction is sent via VehicleMove while
// the rider's head look is sent separately by handleRidingTick.
func (pe *PhysicsMovementExecutor) handleRidingModeNautilus(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	// (1) Send PlayerInput
	log.Printf("[handleRidingModeNautilus] SendVehicleInput(<conn>, forward: %t, backward: %t, left: %t, right: %t, jump: %t, sneak: %t)", forward, backward, left, right, jump, sneak)
	sendRidingInput(pe, versionHandler, forward, backward, left, right, jump, sneak)

	// Hold lock for the entire read-compute-write cycle.
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	currentPos, stateYaw, statePitch, _ := pe.physicsState.GetPosition()

	// Rider (agent) look direction. In manual mode this comes from the inputs;
	// fall back to the physics state when a generator leaves them unset (NaN).
	agentYaw := inputs.Yaw
	if math.IsNaN(agentYaw) {
		agentYaw = stateYaw
	}
	agentPitch := inputs.Pitch
	if math.IsNaN(agentPitch) {
		agentPitch = statePitch
	}

	// nautilusState should always be set when dispatched here (SetMounted creates
	// it). Guard defensively in case of an out-of-band call.
	nautilusState := pe.nautilusState
	if nautilusState == nil {
		nautilusState = models.NewNautilusState(agentYaw, false)
		pe.nautilusState = nautilusState
	}

	// tick() dash-cooldown bookkeeping: decrement the cooldown and clear the
	// dashing flag once the active window elapses.
	if nautilusState.TickDashCooldown() && pe.onDashReady != nil {
		pe.onDashReady()
	}

	// tickControlled(): ease the nautilus yaw toward the rider's look yaw and set
	// the nautilus pitch to half the rider pitch (for the VehicleMove packet).
	vehicleYaw := nautilusState.EaseYawToward(agentYaw)
	vehiclePitch := agentPitch * 0.5

	// Water detection using isTouchingWater() semantics: the nautilus is in water
	// when its occupied block or the block just below it is water.
	belowBlock := pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z)
	occupantBlock := uint32(0)
	if pe.world != nil {
		blockX := int(math.Floor(currentPos.X))
		blockY := int(math.Floor(currentPos.Y))
		blockZ := int(math.Floor(currentPos.Z))
		if blockState, loaded := pe.world.GetBlockStatus(blockX, blockY, blockZ); loaded {
			occupantBlock = blockState
		}
	}
	inWater := pe.shapeProvider != nil && (pe.shapeProvider.IsWater(occupantBlock) || pe.shapeProvider.IsWater(belowBlock))

	// getSaddledSpeed(): movement_speed attribute when the server provides it,
	// otherwise the per-type default (1.0 nautilus / 1.1 zombie) from the state.
	movementSpeed := nautilusState.DefaultMovementSpeed()
	if entityGetter != nil {
		if attrSpeed, ok := entityGetter.GetEntityAttribute(mountedEntityID, "generic.movement_speed"); ok {
			movementSpeed = attrSpeed
		}
	}

	// getControlledMovementInput(): build the 3D pitch-based input (normalized).
	sideways, vertical, forwardComponent := nautilusMovementInput(inputs.ThrottleX, inputs.ThrottleZ, agentPitch)

	// tickControlled() dash: charge while jump is held, fire on release. The dash
	// impulse is added to the velocity BEFORE the travel accumulation, mirroring
	// Java (dash happens in tickControlled, before travel).
	prevVel := models.V3{X: pe.ridingVelX, Y: pe.ridingVelY, Z: pe.ridingVelZ}
	prevVel = applyNautilusDashCharge(nautilusState, jump, pe.lastRidingJumpState, agentYaw, agentPitch, movementSpeed, inWater, prevVel)
	pe.lastRidingJumpState = jump

	// travel(): water vs land path.
	var newPos, newVel models.V3
	var onGround bool
	var dragUsed float64
	if inWater {
		saddledSpeed := physics.NautilusInWaterSpeedMultiplier * movementSpeed
		newPos, newVel = nautilusWaterStep(currentPos, prevVel, sideways, vertical, forwardComponent, vehicleYaw, saddledSpeed)
		onGround = false
		dragUsed = physics.NautilusWaterDrag
	} else {
		saddledSpeed := physics.NautilusOnLandSpeedMultiplier * movementSpeed
		onGroundLand := pe.shapeProvider != nil && pe.shapeProvider.IsSolid(belowBlock)
		slipperiness := 1.0
		if onGroundLand {
			slipperiness = physics.GetBlockSlipperiness(belowBlock)
		}
		newPos, newVel, onGround = nautilusLandStep(currentPos, prevVel, sideways, vertical, forwardComponent, vehicleYaw, saddledSpeed, slipperiness, onGroundLand)
		dragUsed = slipperiness * physics.Inertia
	}

	// Zero tiny velocities on all axes (Java LivingEntity.tickMovement: 0.003).
	if math.Abs(newVel.X) < physics.ResetVelocity {
		newVel.X = 0
	}
	if math.Abs(newVel.Y) < physics.ResetVelocity {
		newVel.Y = 0
	}
	if math.Abs(newVel.Z) < physics.ResetVelocity {
		newVel.Z = 0
	}

	pe.ridingVelX = newVel.X
	pe.ridingVelY = newVel.Y
	pe.ridingVelZ = newVel.Z
	pe.lastVelMultiplier = dragUsed

	// Collision detection for the ridden nautilus
	// Nautilus dimensions: 1.4 wide × 1.6 tall (approximate from AbstractNautilusEntity)
	collisionPos, collisionVel, collisionOnGround, _, _ := resolveEntityCollision(pe, newPos, newVel, 1.4, 1.6)
	if !inWater {
		// Only use collision on land; in water the nautilus swims freely
		newPos = collisionPos
		newVel = collisionVel
		onGround = collisionOnGround
	}

	log.Printf("[handleRidingModeNautilus] inWater=%v ms=%.3f vehYaw=%.1f agentYaw=%.1f agentPitch=%.1f input=(%.2f,%.2f,%.2f) vel=(%.4f,%.4f,%.4f) dashing=%v newPos=(%.2f,%.2f,%.2f)",
		inWater, movementSpeed, vehicleYaw, agentYaw, agentPitch, sideways, vertical, forwardComponent,
		newVel.X, newVel.Y, newVel.Z, nautilusState.IsDashing(), newPos.X, newPos.Y, newPos.Z)

	// Update physicsState inside the lock before returning so sendRidingMove
	// (called outside the lock) cannot race with a concurrent TurnTowards.
	// StateYaw/Pitch store the RIDER look direction; vehicleYaw/Pitch go only
	// into the VehicleMove packet (the nautilus's own eased yaw and halved pitch).
	applyRidingTickState(pe, newPos, agentYaw, agentPitch, onGround)
	return ridingTickResult{
		NewPos:      newPos,
		OnGround:    onGround,
		Sneak:       sneak,
		StateYaw:    agentYaw,
		StatePitch:  agentPitch,
		PacketYaw:   vehicleYaw,
		PacketPitch: vehiclePitch,
	}
}

// nautilusMovementInput builds the controlled movement input vector
// (sideways, vertical, forward) for a ridden nautilus, mirroring Java
// AbstractNautilusEntity.getControlledMovementInput.
//
// The forward/vertical magnitude is derived from the rider's FULL look pitch
// (cos/sin) and is independent of how hard forward is pressed (Java only checks
// forwardSpeed != 0). A backward throttle reverses and halves both forward and
// vertical. The returned vector is normalized when its length exceeds 1 so a
// combined forward+strafe diagonal never exceeds unit input (Java
// movementInputToVelocity normalizes when lengthSquared > 1).
func nautilusMovementInput(throttleX, throttleZ, agentPitchDeg float64) (sideways, vertical, forward float64) {
	sideways = throttleX
	if throttleZ != 0 {
		pitchRad := agentPitchDeg * math.Pi / 180.0
		forwardComponent := math.Cos(pitchRad)
		verticalComponent := -math.Sin(pitchRad)
		if throttleZ < 0 {
			// Reverse at half speed (Java: i *= -0.5F; j *= -0.5F).
			forwardComponent *= -0.5
			verticalComponent *= -0.5
		}
		forward = forwardComponent
		vertical = verticalComponent
	}

	lengthSquared := sideways*sideways + vertical*vertical + forward*forward
	if lengthSquared > 1.0 {
		length := math.Sqrt(lengthSquared)
		sideways /= length
		vertical /= length
		forward /= length
	}
	return
}

// nautilusInputToVelocity scales the (already length-clamped) movement input by
// speed and rotates the X/Z plane by the entity yaw, mirroring Java
// Entity.movementInputToVelocity. The vertical (Y) component passes through.
func nautilusInputToVelocity(sideways, vertical, forward, yawDeg, speed float64) models.V3 {
	scaledX := sideways * speed
	scaledY := vertical * speed
	scaledZ := forward * speed

	yawRad := yawDeg * math.Pi / 180.0
	sinYaw := math.Sin(yawRad)
	cosYaw := math.Cos(yawRad)

	return models.V3{
		X: scaledX*cosYaw - scaledZ*sinYaw,
		Y: scaledY,
		Z: scaledX*sinYaw + scaledZ*cosYaw,
	}
}

// nautilusWaterStep runs one tick of the nautilus's overridden travelInWater:
//
//	velocity += movementInputToVelocity(input, saddledSpeed, vehicleYaw)
//	move(SELF, velocity)            // pos += velocity (pre-drag)
//	velocity *= 0.9                 // all axes, no gravity (neutral buoyancy)
//
// Returns the advanced position and the post-drag velocity to store for the
// next tick.
func nautilusWaterStep(pos, prevVel models.V3, sideways, vertical, forward, vehicleYawDeg, saddledSpeed float64) (newPos, newVel models.V3) {
	accel := nautilusInputToVelocity(sideways, vertical, forward, vehicleYawDeg, saddledSpeed)
	velAfterAccel := prevVel.Add(accel)
	newPos = pos.Add(velAfterAccel)
	newVel = velAfterAccel.Mul(physics.NautilusWaterDrag)
	return
}

// nautilusLandStep runs one tick of the standard LivingEntity.travelMidAir path
// (used when the nautilus is out of water):
//
//	slipperiness = onGround ? blockBelow.slipperiness : 1.0
//	friction     = slipperiness * 0.91
//	speedFactor  = onGround ? saddledSpeed * (0.21600002 / slip^3)
//	                        : saddledSpeed * 0.1   (off-ground, player passenger)
//	velocity += movementInputToVelocity(input, speedFactor, vehicleYaw)
//	move(SELF, velocity)                            // pos += velocity (pre-drag)
//	velocity.xz *= friction
//	velocity.y  = (velocity.y - gravity) * 0.98
func nautilusLandStep(pos, prevVel models.V3, sideways, vertical, forward, vehicleYawDeg, saddledSpeed, slipperiness float64, onGround bool) (newPos, newVel models.V3, resolvedOnGround bool) {
	friction := slipperiness * physics.Inertia

	var speedFactor float64
	if onGround {
		speedFactor = saddledSpeed * (nautilusGroundFrictionSpeedFactor / (slipperiness * slipperiness * slipperiness))
	} else {
		speedFactor = saddledSpeed * nautilusOffGroundSpeedFactor
	}

	accel := nautilusInputToVelocity(sideways, vertical, forward, vehicleYawDeg, speedFactor)
	velAfterAccel := prevVel.Add(accel)
	newPos = pos.Add(velAfterAccel)

	newVel = models.V3{
		X: velAfterAccel.X * friction,
		Y: (velAfterAccel.Y - physics.Gravity) * physics.Drag,
		Z: velAfterAccel.Z * friction,
	}
	resolvedOnGround = onGround
	return
}

// applyNautilusDashCharge runs the spear-charge dash state machine for one tick
// and returns the velocity with any fired dash impulse added. It mirrors Java's
// JumpingMount charge-on-hold / fire-on-release behavior: holding jump
// accumulates charge (gated on the dash cooldown), and releasing (or hitting the
// charge cap) fires a dash whose impulse follows the rider's full 3D look vector.
func applyNautilusDashCharge(state *models.NautilusState, jump, lastJump bool, agentYaw, agentPitch, movementSpeed float64, inWater bool, vel models.V3) models.V3 {
	// Rising edge: begin charging if the dash is off cooldown.
	if jump && !lastJump && state.CanStartDash() {
		state.SetCharging(true)
		state.SetJumpChargeTicks(0)
	}

	if !state.GetIsCharging() {
		return vel
	}

	if jump {
		state.AddJumpChargeTicks(nautilusDashChargePerTick)
	}

	// Fire on release or when the charge saturates.
	if !jump || state.GetJumpChargeTicks() >= models.NautilusDashChargeCapTicks {
		strength := models.ClampJumpStrength(state.GetJumpChargeTicks())
		deltaVelX, deltaVelY, deltaVelZ := models.NautilusDashImpulse(agentYaw, agentPitch, strength, movementSpeed, nautilusDashVelocityMultiplier, inWater)
		vel.X += deltaVelX
		vel.Y += deltaVelY
		vel.Z += deltaVelZ
		state.ApplyDash()
		log.Printf("[handleRidingModeNautilus] Dash fired: strength=%.3f impulse=(%.4f,%.4f,%.4f) inWater=%v", strength, deltaVelX, deltaVelY, deltaVelZ, inWater)
	}
	return vel
}
