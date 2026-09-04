package movement

import (
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// handleRidingModeCamel runs one tick for a ridden camel (or camel_husk).
//
// Implements the full Java CamelEntity behavior:
//   - Sitting/standing pose state machine with stationary checks
//   - Velocity-impulse dash (not teleport) with charge mechanic
//   - Tick-based dash cooldown with onDashReady callback
//   - Sprint speed bonus gated on dashCooldown == 0
//   - Slipperiness-based land friction (airborne: 0.91, grounded: 0.546)
//   - Sit-to-stand on water contact
//   - Airborne physics with gravity and landing detection
func (pe *PhysicsMovementExecutor) handleRidingModeCamel(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	// (1) Send PlayerInput
	log.Printf("[handleRidingModeCamel] SendVehicleInput(<conn>, forward: %t, backward: %t, left: %t, right: %t, jump: %t, sneak: %t)", forward, backward, left, right, jump, sneak)
	sendRidingInput(pe, versionHandler, forward, backward, left, right, jump, sneak)

	// Hold lock for entire read-compute-write cycle
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	currentPos, yaw, pitch, _ := pe.physicsState.GetPosition()
	camelSt := pe.camelState
	if camelSt == nil {
		// Safety: should never happen if dispatch is correct
		log.Printf("[handleRidingModeCamel] WARNING: camelState is nil, falling back to horse handler")
		pe.mountedEntityMu.Unlock()
		horseResult := pe.handleRidingModeHorse(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, entityGetter)
		pe.mountedEntityMu.Lock()
		return horseResult
	}

	// Get world time from server (synchronized via ClientboundUpdateTime packets)
	// Fall back to a default value if not yet initialized
	worldTime := int64(0)
	if pe.world != nil {
		if age, ok := pe.world.GetWorldAge(); ok {
			worldTime = age
		}
	}

	// --- Dash end check and cooldown tick (runs every tick) ---
	blockBelowForDash := pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z)
	isInFluid := pe.shapeProvider != nil && pe.shapeProvider.IsWater(blockBelowForDash)

	if camelSt.ShouldEndDash(true, isInFluid) {
		camelSt.EndDash()
	}
	if camelSt.TickDashCooldown() {
		log.Printf("[handleRidingModeCamel] Dash cooldown expired — ready for new dash")
		if pe.onDashReady != nil {
			pe.onDashReady()
		}
	}
	// Auto-stand in water
	if camelSt.IsSitting() && isInFluid {
		camelSt.SetStanding(worldTime)
		log.Printf("[handleRidingModeCamel] Auto-stood in water")
	}

	// --- Stationary check ---
	if camelSt.IsStationary(worldTime) {
		if forward && camelSt.IsSitting() && !camelSt.IsChangingPose(worldTime) {
			camelSt.StartStanding(worldTime)
			log.Printf("[handleRidingModeCamel] Started standing (rider forward input)")
		}

		pe.ridingVelX = 0
		pe.ridingVelZ = 0
		pe.lastRidingJumpState = jump

		log.Printf("[handleRidingModeCamel] Stationary: sitting=%v changingPose=%v pos=(%.2f,%.2f,%.2f)",
			camelSt.IsSitting(), camelSt.IsChangingPose(worldTime), currentPos.X, currentPos.Y, currentPos.Z)

		// Update physicsState inside the lock so a concurrent TurnTowards
		// cannot race with this write-back (see applyRidingTickState).
		applyRidingTickState(pe, currentPos, yaw, pitch, true)
		return ridingTickResult{
			NewPos:      currentPos,
			OnGround:    true,
			Sneak:       sneak,
			StateYaw:    yaw,
			StatePitch:  pitch,
			PacketYaw:   yaw,
			PacketPitch: pitch,
		}
	}

	// (2) Yaw-rotation steering
	yaw -= inputs.ThrottleX * horseTurnDegsPerTick

	// (3) Water/land/airborne physics
	waterParams := computeRidingWaterPhysics(pe, versionHandler, currentPos)

	// Camel hitbox must match the vanilla server so client-side ground/edge
	// detection agrees with it. A mounted camel is always an adult, and a sitting
	// camel is stationary (handled by the early return above), so the standing
	// box always applies here. Under-modeling the width previously made the
	// client flip to airborne at a platform edge before the server (which sees
	// the true 1.7-wide body) agreed, dropping forward accel to air-control and
	// stalling the mount at the edge instead of letting it walk off and fall.
	camelWidth := models.CamelWidthAdult
	camelHeight := models.CamelHeightAdultStanding

	// Probe for ground support so a camel that walks off a ledge with zero
	// vertical velocity begins to fall instead of hovering.
	grounded := ridingHasGroundSupport(pe, currentPos, camelWidth, camelHeight)
	ridingAirborne := !waterParams.IsInWater && (pe.ridingVelY > 0.001 || !grounded)

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

	// (4) Movement speed with camel sprint bonus
	movementAcceleration := resolveMountMovementSpeed(entityGetter, mountedEntityID, "generic.movement_speed", models.CamelDefaultMovementSpeed)
	if inputs.Sprint && camelSt.GetDashCooldownTicks() <= 0 {
		movementAcceleration += models.CamelSprintBonus
	}

	// (5) Velocity computation. While airborne, movement input provides only
	// limited air control; applying full ground movement speed in the air would
	// let the camel accelerate to several times its ground speed and fly forward
	// while falling.
	inputAccel := mountInputAcceleration(movementAcceleration, ridingAirborne)
	ridingVelZ := mountForwardVelocity(pe.ridingVelZ, velocityDrag, inputs.ThrottleZ, inputAccel)
	ridingVelX := 0.0
	ridingVelY := mountVerticalVelocity(pe.ridingVelY, gravityDelta, ridingAirborne)

	// (6) Dash charge mechanic
	if jump && !pe.lastRidingJumpState {
		if camelSt.CanStartDash(true) {
			camelSt.SetCharging(true)
			camelSt.SetJumpChargeTicks(0)
			log.Printf("[handleRidingModeCamel] Dash charging started")
		} else {
			log.Printf("[handleRidingModeCamel] Dash blocked: cooldown=%d", camelSt.GetDashCooldownTicks())
		}
	}

	if camelSt.GetIsCharging() {
		if jump {
			// Accumulate charge using the mount's charging speed multiplier.
			// Regular camel = 1.0x; CamelHusk = 4.0x (charges 4x faster).
			chargeMul := camelSt.GetChargingSpeedMultiplier()
			camelSt.AddJumpChargeTicks(int(chargeMul))
			if camelSt.GetJumpChargeTicks() > mountJumpChargeCapTicks {
				camelSt.SetJumpChargeTicks(mountJumpChargeCapTicks)
			}
		}

		if !jump || camelSt.GetJumpChargeTicks() >= mountJumpChargeCapTicks {
			jumpTicks := camelSt.GetJumpChargeTicks()
			// mountJumpChargeStrength mirrors the Java client charge ramp
			// (ClientPlayerEntity.tickMovement) rather than clamping raw ticks,
			// which would make the ramp ~10x too slow.
			strengthPercent, strength := mountJumpChargeStrength(jumpTicks)
			// Tell the server about the released jump so the server-side camel
			// dashes too (vanilla LocalPlayer.sendRidingJump on key release).
			sendRidingJumpCommand(pe, versionHandler, strengthPercent)
			newRidingVelZ, deltaVelY := camelDashForwardVelocity(pe.ridingVelZ, yaw, strength, movementAcceleration)

			ridingVelY += deltaVelY
			ridingVelZ = newRidingVelZ

			camelSt.ApplyDash()
			log.Printf("[handleRidingModeCamel] Dash applied: strength=%.3f charge=%d strengthPercent=%d deltaVelY=%.3f forwardVel=%.4f",
				strength, jumpTicks, strengthPercent, deltaVelY, ridingVelZ)
		}
	}

	pe.lastRidingJumpState = jump

	// (7) Position computation via collision detection
	// Camel dimensions: 1.7 wide × 2.375 tall (vanilla adult standing box)
	moveVelX, moveVelZ := forwardVelocityToWorld(ridingVelZ, yaw)
	moveVel := models.V3{
		X: moveVelX,
		Y: ridingVelY,
		Z: moveVelZ,
	}
	newPos, correctedVel, collisionOnGround, _, _ := resolveEntityCollision(pe, currentPos, moveVel, camelWidth, camelHeight)
	// onGround reflects actual ground support: either collision stopped a
	// downward move this tick, or the ground-support probe found a block just
	// beneath us (e.g. while walking on flat ground with zero Y velocity).
	onGround := collisionOnGround || grounded

	// Landing: collision stopped a downward move, so we've settled onto solid
	// ground. Zero the accumulated fall velocity and record the new ground
	// level. Landing is driven purely by collision now — the old "reached
	// recorded groundY" check (and the buggy newPos.Y = correctedVel.Y
	// reposition) would halt a genuine cliff fall at the pre-fall height.
	if collisionOnGround {
		correctedVel.Y = 0
		onGround = true
		pe.ridingGroundY = newPos.Y
		log.Printf("[handleRidingModeCamel] Landed at Y=%.2f", newPos.Y)
	}

	// Use corrected horizontal velocity for next tick
	ridingVelZ = math.Sqrt(correctedVel.X*correctedVel.X + correctedVel.Z*correctedVel.Z)
	ridingVelY = correctedVel.Y

	// Update shared velocity state
	pe.ridingVelX = ridingVelX
	pe.ridingVelZ = ridingVelZ
	pe.ridingVelY = ridingVelY
	pe.lastVelMultiplier = velocityDrag

	// Auto-dismount when submerged
	checkRiderHeadSubmerged(pe, mountedEntityID, waterParams, newPos.X, newPos.Y, newPos.Z)

	vehicleKind := "camel"
	if camelSt.GetIsCamelHusk() {
		vehicleKind = "camel_husk"
	}
	log.Printf("[handleRidingModeCamel] %s physics: water=%v yaw=%.1f throttle=(%.2f,%.2f) accel=%.4f drag=%.3f velZ=%.4f velY=%.4f airborne=%v newPos=(%.2f,%.2f,%.2f)",
		vehicleKind, waterParams.IsInWater, yaw, inputs.ThrottleX, inputs.ThrottleZ, movementAcceleration, velocityDrag, ridingVelZ, ridingVelY, ridingAirborne, newPos.X, newPos.Y, newPos.Z)

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

// camelDashForwardVelocity composes models.DashImpulse's raw impulse vector
// into the scalar forward velocity a dash actually applies, mirroring the
// handler's call site exactly: DashImpulse returns a delta in world axes
// (already yaw-rotated), which is re-projected onto the camel's
// forward-facing direction via a dot product with the facing unit vector,
// then added to baseForwardVelocity — the rider's forward velocity from
// *before* this tick's step-5 velocity update (pe.ridingVelZ at the time the
// dash fires), not the value computed earlier in the same tick. This
// pre-step-5 base is what the handler actually adds to; matching it here
// keeps this a faithful extraction rather than an approximation.
//
// The camel's own velocityMultiplier is always 1.0 at this call site
// (unlike the nautilus's dash, which varies), so it isn't a parameter here.
func camelDashForwardVelocity(baseForwardVelocity, yawDegrees, dashStrength, movementAcceleration float64) (newForwardVelocity, verticalDelta float64) {
	const velocityMultiplier = 1.0
	deltaVelX, deltaVelY, deltaVelZ := models.DashImpulse(yawDegrees, dashStrength, movementAcceleration, velocityMultiplier)

	yawRad := yawDegrees * math.Pi / 180.0
	forwardImpulse := -math.Sin(yawRad)*deltaVelX + math.Cos(yawRad)*deltaVelZ
	return baseForwardVelocity + forwardImpulse, deltaVelY
}
