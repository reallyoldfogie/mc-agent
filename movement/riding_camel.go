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
) {
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
		pe.handleRidingModeHorse(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, entityGetter)
		pe.mountedEntityMu.Lock()
		return
	}

	// Get world time from server (synchronized via ClientboundUpdateTime packets)
	// Fall back to a default value if not yet initialized
	worldTime := int64(0)
	if pe.worldManager != nil {
		if age, ok := pe.worldManager.GetWorldAge(); ok {
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

		sendVehicleMove(pe, versionHandler, currentPos, yaw, pitch, true)
		log.Printf("[handleRidingModeCamel] Stationary: sitting=%v changingPose=%v pos=(%.2f,%.2f,%.2f)",
			camelSt.IsSitting(), camelSt.IsChangingPose(worldTime), currentPos.X, currentPos.Y, currentPos.Z)
		return
	}

	// (2) Yaw-rotation steering
	yaw -= inputs.ThrottleX * horseTurnDegsPerTick

	// (3) Water/land/airborne physics
	waterParams := computeRidingWaterPhysics(pe, versionHandler, currentPos)
	var velocityDrag float64
	var gravityDelta float64
	ridingAirborne := false

	if waterParams.IsInWater {
		velocityDrag = waterParams.VelocityDrag
		gravityDelta = waterParams.GravityDelta
	} else {
		ridingAirborne = pe.ridingVelY > 0.001 || (pe.ridingVelY < -0.001 && currentPos.Y > pe.ridingGroundY+0.01)
		if ridingAirborne {
			velocityDrag = physics.Inertia
			gravityDelta = -physics.Gravity
		} else {
			blockSlipperiness := physics.GetBlockSlipperiness(waterParams.BlockBelowEntity)
			velocityDrag = physics.HorseLandFriction(blockSlipperiness)
			gravityDelta = 0.0
			pe.ridingVelY = 0.0
			pe.ridingGroundY = currentPos.Y
		}
	}

	// (4) Movement speed with camel sprint bonus
	movementAcceleration := models.CamelDefaultMovementSpeed
	if entityGetter != nil {
		if movementSpeed, ok := entityGetter.GetEntityAttribute(mountedEntityID, "generic.movement_speed"); ok {
			movementAcceleration = movementSpeed
		}
	}
	if inputs.Sprint && camelSt.GetDashCooldownTicks() <= 0 {
		movementAcceleration += models.CamelSprintBonus
	}

	// (5) Velocity computation
	ridingVelZ := pe.ridingVelZ*velocityDrag + inputs.ThrottleZ*movementAcceleration
	ridingVelX := 0.0
	ridingVelY := pe.ridingVelY + gravityDelta
	if ridingAirborne {
		ridingVelY *= physics.Drag
	}
	if math.Abs(ridingVelZ) < physics.ResetVelocity {
		ridingVelZ = 0
	}

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
			chargeMul := camelSt.GetChargingSpeedMultiplier()
			camelSt.AddJumpChargeTicks(int(chargeMul))
			if camelSt.GetJumpChargeTicks() > 100 {
				camelSt.SetJumpChargeTicks(100)
			}
		}

		if !jump || camelSt.GetJumpChargeTicks() >= 100 {
			jumpTicks := camelSt.GetJumpChargeTicks()
			strength := models.ClampJumpStrength(jumpTicks)
			velocityMultiplier := 1.0
			deltaVelX, deltaVelY, deltaVelZ := models.DashImpulse(yaw, strength, movementAcceleration, velocityMultiplier)

			ridingVelY += deltaVelY
			yawRad := yaw * math.Pi / 180.0
			forwardImpulse := -math.Sin(yawRad)*deltaVelX + math.Cos(yawRad)*deltaVelZ
			ridingVelZ = pe.ridingVelZ + forwardImpulse

			camelSt.ApplyDash()
			log.Printf("[handleRidingModeCamel] Dash applied: strength=%.3f charge=%d impulse=(%.3f,%.3f,%.3f) forwardVel=%.4f",
				strength, jumpTicks, deltaVelX, deltaVelY, deltaVelZ, ridingVelZ)
		}
	}

	pe.lastRidingJumpState = jump

	// (7) Position computation
	yawRad := yaw * math.Pi / 180.0
	newX := currentPos.X + (-math.Sin(yawRad) * ridingVelZ)
	newY := currentPos.Y + ridingVelY
	newZ := currentPos.Z + (math.Cos(yawRad) * ridingVelZ)
	onGround := !ridingAirborne

	// Landing detection
	if ridingAirborne && ridingVelY < 0 && newY <= pe.ridingGroundY {
		newY = pe.ridingGroundY
		ridingVelY = 0
		onGround = true
		log.Printf("[handleRidingModeCamel] Landed at Y=%.2f (groundY=%.2f)", newY, pe.ridingGroundY)
	}

	// Update shared velocity state
	pe.ridingVelX = ridingVelX
	pe.ridingVelZ = ridingVelZ
	pe.ridingVelY = ridingVelY
	pe.lastVelMultiplier = velocityDrag

	// Auto-dismount when submerged
	checkRiderHeadSubmerged(pe, mountedEntityID, waterParams, newX, newY, newZ)

	vehicleKind := "camel"
	if camelSt.GetIsCamelHusk() {
		vehicleKind = "camel_husk"
	}
	log.Printf("[handleRidingModeCamel] %s physics: water=%v yaw=%.1f throttle=(%.2f,%.2f) accel=%.4f drag=%.3f velZ=%.4f velY=%.4f airborne=%v newPos=(%.2f,%.2f,%.2f)",
		vehicleKind, waterParams.IsInWater, yaw, inputs.ThrottleX, inputs.ThrottleZ, movementAcceleration, velocityDrag, ridingVelZ, ridingVelY, ridingAirborne, newX, newY, newZ)

	newPos := models.V3{X: newX, Y: newY, Z: newZ}
	sendVehicleMove(pe, versionHandler, newPos, yaw, pitch, onGround)
}
