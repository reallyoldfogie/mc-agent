package movement

import (
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// handleRidingModeHappyGhast runs one tick for a ridden happy ghast pilot
// (1.21.6+). Mirrors the Java call chain (LivingEntity.travelControlled):
//
//	getControlledMovementInput() → raw (sideways, vertical, forward) input,
//	    scaled by 3.9*flying_speed, NOT length-clamped here
//	tickControlled()             → ease ghast yaw toward pilot yaw (0.08),
//	    pitch = pilot pitch * 0.5
//	setMovementSpeed / travel()  → speed = flying_speed * 5/3
//	travelFlying() → updateVelocity(): movementInputToVelocity (length-clamps
//	    here) → velocity += accel; move by velocity; velocity *= 0.91
//
// Notably absent from travelFlying's non-water/lava branch: any gravity
// term. Flight is genuinely zero-gravity, confirmed by its absence in the
// decompiled source, not inferred from observed behavior — see
// PHASE_6_PLAN.md §3.1.
//
// This handler assumes the ghast is not "staying still"
// (models.EntityMetadataKeyHappyGhastStayingStill) — that case has no
// controlling passenger at all in vanilla and is routed to
// handleRidingModePassiveRider by the dispatch in physics_executor.go before
// this handler is ever called, the same way an unsaddled horse is.
func (pe *PhysicsMovementExecutor) handleRidingModeHappyGhast(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	log.Printf("[handleRidingModeHappyGhast] SendVehicleInput(<conn>, forward: %t, backward: %t, left: %t, right: %t, jump: %t, sneak: %t)", forward, backward, left, right, jump, sneak)
	sendRidingInput(pe, versionHandler, forward, backward, left, right, jump, sneak)

	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	currentPos, stateYaw, statePitch, _ := pe.physicsState.GetPosition()

	agentYaw := inputs.Yaw
	if math.IsNaN(agentYaw) {
		agentYaw = stateYaw
	}
	agentPitch := inputs.Pitch
	if math.IsNaN(agentPitch) {
		agentPitch = statePitch
	}

	ghastState := pe.happyGhastState
	if ghastState == nil {
		ghastState = models.NewHappyGhastState(agentYaw)
		pe.happyGhastState = ghastState
	}

	// canMoveVoluntarily(): rotation updates still happen even when movement
	// is suppressed (e.g. by a future levitation effect), matching the
	// strider handler's identical guard.
	if !canMoveVoluntarily() {
		newPos := currentPos
		applyRidingTickState(pe, newPos, agentYaw, agentPitch, true)
		return ridingTickResult{
			NewPos:      newPos,
			OnGround:    true,
			Sneak:       sneak,
			StateYaw:    agentYaw,
			StatePitch:  agentPitch,
			PacketYaw:   ghastState.GetVehicleYaw(),
			PacketPitch: agentPitch * physics.HappyGhastPitchFactor,
		}
	}

	// getAttributeValue(FLYING_SPEED) — not MOVEMENT_SPEED, which this
	// handler never reads.
	flyingSpeed := resolveMountMovementSpeed(entityGetter, mountedEntityID, "generic.flying_speed", physics.HappyGhastDefaultFlyingSpeed)

	// getControlledMovementInput(): computed BEFORE tickControlled eases the
	// yaw, using the pilot's current-tick pitch — matches Java's exact call
	// order in travelControlled.
	movementInput := happyGhastControlledMovementInput(inputs.ThrottleX, inputs.ThrottleZ, agentPitch, jump, flyingSpeed)

	// tickControlled(): ease the ghast's own yaw toward the pilot's look yaw
	// and set its pitch to half the pilot's pitch (for the VehicleMove
	// packet only, same as nautilus).
	vehicleYaw := ghastState.EaseYawToward(agentYaw)
	vehiclePitch := agentPitch * physics.HappyGhastPitchFactor

	// travel() → travelFlying() → updateVelocity(): accumulate the scaled,
	// yaw-rotated input onto the existing velocity, move by the full
	// velocity, then apply drag. No gravity term in this branch (§3.1).
	travelSpeed := flyingSpeed * physics.HappyGhastTravelSpeedFactor
	accel := movementInputToVelocity(movementInput, travelSpeed, vehicleYaw)
	prevVel := models.V3{X: pe.ridingVelX, Y: pe.ridingVelY, Z: pe.ridingVelZ}
	velAfterAccel := prevVel.Add(accel)
	newPos := currentPos.Add(velAfterAccel)
	newVel := velAfterAccel.Mul(physics.HappyGhastFlightDrag)

	// Zero tiny velocities on all axes (Java Entity.resetVelocityIfSmall: 0.003).
	if math.Abs(newVel.X) < physics.ResetVelocity {
		newVel.X = 0
	}
	if math.Abs(newVel.Y) < physics.ResetVelocity {
		newVel.Y = 0
	}
	if math.Abs(newVel.Z) < physics.ResetVelocity {
		newVel.Z = 0
	}

	// Java's Entity.move() always resolves against world collision
	// regardless of flight status — a happy ghast cannot fly through walls
	// or terrain even though it has no gravity and no resting-on-ground
	// concept of its own.
	collisionPos, collisionVel, collisionOnGround, _, _ := resolveEntityCollision(pe, newPos, newVel, physics.HappyGhastWidth, physics.HappyGhastHeight)
	newPos = collisionPos
	newVel = collisionVel

	pe.ridingVelX = newVel.X
	pe.ridingVelY = newVel.Y
	pe.ridingVelZ = newVel.Z
	pe.lastVelMultiplier = physics.HappyGhastFlightDrag

	log.Printf("[handleRidingModeHappyGhast] flyingSpeed=%.4f vehYaw=%.1f agentYaw=%.1f agentPitch=%.1f input=(%.3f,%.3f,%.3f) vel=(%.4f,%.4f,%.4f) newPos=(%.2f,%.2f,%.2f)",
		flyingSpeed, vehicleYaw, agentYaw, agentPitch, movementInput.X, movementInput.Y, movementInput.Z,
		newVel.X, newVel.Y, newVel.Z, newPos.X, newPos.Y, newPos.Z)

	applyRidingTickState(pe, newPos, agentYaw, agentPitch, collisionOnGround)
	return ridingTickResult{
		NewPos:      newPos,
		OnGround:    collisionOnGround,
		Sneak:       sneak,
		StateYaw:    agentYaw,
		StatePitch:  agentPitch,
		PacketYaw:   vehicleYaw,
		PacketPitch: vehiclePitch,
	}
}
