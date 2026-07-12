package models

import (
	"math"
	"sync"
)

// CamelPose represents the camel's current pose state.
// Mirrors Java CamelEntity's use of EntityPose.SITTING / EntityPose.STANDING.
type CamelPose int

const (
	CamelPoseStanding CamelPose = iota
	CamelPoseSitting
)

// Camel physics constants matching Java CamelEntity (1.21.10/1.21.11).
// Source: net/minecraft/entity/passive/CamelEntity.java
const (
	// CamelDashCooldownTicks is the total dash cooldown in ticks (55 ticks = 2.75s).
	// Java: this.dashCooldown = 55 in jump()
	CamelDashCooldownTicks = 55

	// CamelDashEndThresholdTicks is the tick threshold below which dashing ends
	// when the entity is on ground or in fluid.
	// Java: if (this.isDashing() && this.dashCooldown < 50 && ...)
	CamelDashEndThresholdTicks = 50

	// CamelSitTransitionTicks is how many ticks the sit-down transition takes.
	// Java: isChangingPose() uses 40 when isSitting()
	CamelSitTransitionTicks = 40

	// CamelStandTransitionTicks is how many ticks the stand-up transition takes.
	// Java: isChangingPose() uses 52 when !isSitting()
	CamelStandTransitionTicks = 52

	// CamelDashHorizontalFactor scales the horizontal velocity impulse during a dash.
	// Java: 22.2222F * strength * movementSpeed * velocityMultiplier
	CamelDashHorizontalFactor = 22.2222

	// CamelDashVerticalFactor scales the vertical velocity impulse during a dash.
	// Java: 1.4285F * strength * jumpVelocity
	CamelDashVerticalFactor = 1.4285

	// CamelDefaultMovementSpeed is the camel's base movement_speed attribute.
	// Java: createCamelAttributes() .add(EntityAttributes.MOVEMENT_SPEED, 0.09F)
	CamelDefaultMovementSpeed = 0.09

	// CamelSprintBonus is the speed bonus when the rider is sprinting and dash cooldown is 0.
	// Java: getSaddledSpeed() adds 0.1F when controllingPlayer.isSprinting() && getJumpCooldown() == 0
	CamelSprintBonus = 0.1

	// CamelJumpStrength is the base jump velocity for camels.
	// Java: createCamelAttributes() .add(EntityAttributes.JUMP_STRENGTH, 0.42F)
	CamelJumpStrength = 0.42

	// CamelStepHeight is the camel's step-up height in blocks.
	// Java: createCamelAttributes() .add(EntityAttributes.STEP_HEIGHT, 1.5)
	CamelStepHeight = 1.5

	// CamelBabyScale is the scale factor for baby camels.
	// Java: getScaleFactor() returns 0.45F when isBaby()
	CamelBabyScale = 0.45

	// CamelHuskChargingSpeedMultiplier is the rider charging speed multiplier for CamelHusk (1.21.11+).
	// Java: CamelHuskEntity.getRiderChargingSpeedMultiplier() returns 4.0F
	CamelHuskChargingSpeedMultiplier = 4.0

	// Camel hitbox dimensions (blocks), matching the vanilla CamelEntity. The
	// client's ground/edge detection and collision must use these so they agree
	// with the server's authoritative entity; under-modeling the width makes a
	// ridden camel flip to airborne at a ledge before the server does and stall
	// at the edge instead of walking off. Only adults are rideable, and a sitting
	// camel is stationary, so the mounted-movement path uses the adult standing
	// box; the remaining values are defined for correctness and future use.
	CamelWidthAdult          = 1.7
	CamelHeightAdultStanding = 2.375
	CamelHeightAdultSitting  = 0.945
	CamelWidthBaby           = 0.85
	CamelHeightBabyStanding  = 1.1875
	CamelHeightBabySitting   = 0.4725
)

// CamelState tracks the server-side camel pose and dash state.
// This mirrors the TrackedData fields from Java CamelEntity:
//   - DASHING (Boolean)
//   - LAST_POSE_TICK (Long)
//
// The physics executor maintains a CamelState when mounted on a camel.
// Field access is synchronized with mu to prevent race conditions when
// the physics tick loop writes while tests/other code reads.
type CamelState struct {
	mu sync.RWMutex

	// Pose is the camel's current pose (standing or sitting).
	Pose CamelPose

	// LastPoseTick mirrors Java's LAST_POSE_TICK tracked data.
	// Negative = sitting (negate to get the world time when sitting started).
	// Positive = standing (the world time when standing started).
	// Zero or large-past = idle/no transition in progress.
	LastPoseTick int64

	// DashCooldownTicks counts down from CamelDashCooldownTicks (55) to 0.
	// While > 0, dashing is on cooldown and new dashes cannot start.
	DashCooldownTicks int

	// Dashing is true while the camel is in its dash lunge.
	// Java: this.dataTracker.get(DASHING)
	Dashing bool

	// JumpChargeStrength accumulates while the jump key is held (0–100 ticks).
	// Converted to a 0.4–1.0 strength float via ClampJumpStrength().
	JumpChargeTicks int

	// IsCharging is true while the jump key is held down, accumulating charge.
	IsCharging bool

	// IsCamelHusk distinguishes CamelHuskEntity (1.21.11+) from regular CamelEntity.
	// Affects charging speed multiplier and food items.
	IsCamelHusk bool
}

// NewCamelState creates a CamelState initialized to standing with no active dash.
// worldTime is the current server world time in ticks, used to set LastPoseTick
// so that the camel starts in a fully-transitioned standing pose.
func NewCamelState(worldTime int64, isCamelHusk bool) *CamelState {
	return &CamelState{
		Pose: CamelPoseStanding,
		// Java initLastPoseTick: Math.max(0, time - 52 - 1) — ensures isChangingPose() is false.
		LastPoseTick: max(0, worldTime-CamelStandTransitionTicks-1),
		IsCamelHusk:  isCamelHusk,
	}
}

// IsSitting returns true if the camel is in the sitting pose.
// Java: isSitting() { return this.dataTracker.get(LAST_POSE_TICK) < 0L; }
func (cs *CamelState) IsSitting() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.Pose == CamelPoseSitting
}

// TimeSinceLastPoseTick returns how many ticks have elapsed since the last pose change.
// Java: getTimeSinceLastPoseTick() { return worldTime - Math.abs(LAST_POSE_TICK); }
func (cs *CamelState) TimeSinceLastPoseTick(worldTime int64) int64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	absLastPose := cs.LastPoseTick
	if absLastPose < 0 {
		absLastPose = -absLastPose
	}
	return worldTime - absLastPose
}

// IsChangingPose returns true if the camel is in the middle of a sit/stand transition.
// Java: isChangingPose() { long l = getTimeSinceLastPoseTick(); return l < (isSitting() ? 40 : 52); }
func (cs *CamelState) IsChangingPose(worldTime int64) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	absLastPose := cs.LastPoseTick
	if absLastPose < 0 {
		absLastPose = -absLastPose
	}
	elapsed := worldTime - absLastPose
	if cs.Pose == CamelPoseSitting {
		return elapsed < CamelSitTransitionTicks
	}
	return elapsed < CamelStandTransitionTicks
}

// IsStationary returns true if the camel cannot move (sitting or mid-transition).
// Java: isStationary() { return isSitting() || isChangingPose(); }
func (cs *CamelState) IsStationary(worldTime int64) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.Pose == CamelPoseSitting {
		return true
	}
	absLastPose := cs.LastPoseTick
	if absLastPose < 0 {
		absLastPose = -absLastPose
	}
	elapsed := worldTime - absLastPose
	if cs.Pose == CamelPoseSitting {
		return elapsed < CamelSitTransitionTicks
	}
	return elapsed < CamelStandTransitionTicks
}

// StartSitting transitions the camel from standing to sitting.
// No-op if already sitting.
// Java: startSitting() — sets pose=SITTING, LastPoseTick = -worldTime
func (cs *CamelState) StartSitting(worldTime int64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.Pose == CamelPoseSitting {
		return
	}
	cs.Pose = CamelPoseSitting
	cs.LastPoseTick = -worldTime
}

// StartStanding transitions the camel from sitting to standing.
// No-op if already standing.
// Java: startStanding() — sets pose=STANDING, LastPoseTick = worldTime
func (cs *CamelState) StartStanding(worldTime int64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.Pose != CamelPoseSitting {
		return
	}
	cs.Pose = CamelPoseStanding
	cs.LastPoseTick = worldTime
}

// SetStanding immediately sets the camel to a fully-transitioned standing pose.
// Used for instant transitions (e.g., on damage, on water contact).
// Java: setStanding() — sets pose=STANDING, initLastPoseTick(worldTime)
func (cs *CamelState) SetStanding(worldTime int64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Pose = CamelPoseStanding
	// Java initLastPoseTick: Math.max(0, time - 52 - 1)
	cs.LastPoseTick = max(0, worldTime-CamelStandTransitionTicks-1)
}

// ClampJumpStrength converts a charge percentage (0–100) to a strength float (0.4–1.0).
// Java JumpingMount.clampJumpStrength:
//
//	strength >= 90 ? 1.0F : 0.4F + 0.4F * strength / 90.0F
func ClampJumpStrength(chargeTicks int) float64 {
	if chargeTicks >= 90 {
		return 1.0
	}
	return 0.4 + 0.4*float64(chargeTicks)/90.0
}

// MountJumpStrength mirrors the Java client charge ramp from
// ClientPlayerEntity.tickMovement() that drives the jump-bar while the jump key
// is held on a JumpingMount (horse/camel):
//
//	this.mountJumpTicks++;
//	if (this.mountJumpTicks < 10) {
//	    this.mountJumpStrength = this.mountJumpTicks * 0.1F;
//	} else {
//	    this.mountJumpStrength = 0.8F + 2.0F / (this.mountJumpTicks - 9) * 0.1F;
//	}
//
// The ramp reaches 1.0 at tick 10 and then decays back toward 0.8 as the charge
// is held longer. chargeTicks is the number of ticks the jump key has been held
// (already scaled by the mount's charging speed multiplier, e.g. 4x for a camel
// husk). The returned float is the 0..1 strength the client would send to the
// server; callers should floor(mountJumpStrength*100) and feed that int to
// ClampJumpStrength to match the server-side clamp exactly.
func MountJumpStrength(chargeTicks int) float64 {
	if chargeTicks < 10 {
		return float64(chargeTicks) * 0.1
	}
	return 0.8 + 2.0/float64(chargeTicks-9)*0.1
}

// TickDashCooldown decrements the dash cooldown by one tick.
// Returns true if the cooldown just reached zero this tick (the "dash ready" signal).
func (cs *CamelState) TickDashCooldown() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.DashCooldownTicks <= 0 {
		return false
	}
	cs.DashCooldownTicks--
	return cs.DashCooldownTicks == 0
}

// CanStartDash returns true if a new dash can be initiated.
// Java: setJumpStrength() requires hasSaddleEquipped() && dashCooldown <= 0 && isOnGround()
// Saddle is implied (we're riding), so we check cooldown and ground.
func (cs *CamelState) CanStartDash(isOnGround bool) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.DashCooldownTicks <= 0 && isOnGround
}

// ApplyDash sets the dash state after a dash impulse is applied.
func (cs *CamelState) ApplyDash() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.DashCooldownTicks = CamelDashCooldownTicks
	cs.Dashing = true
	cs.IsCharging = false
	cs.JumpChargeTicks = 0
}

// ShouldEndDash returns true if the dashing flag should be cleared.
// Java: isDashing() && dashCooldown < 50 && (isOnGround || isInFluid || hasVehicle)
func (cs *CamelState) ShouldEndDash(isOnGround, isInFluid bool) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.Dashing && cs.DashCooldownTicks < CamelDashEndThresholdTicks && (isOnGround || isInFluid)
}

// GetChargingSpeedMultiplier returns the rider charging speed multiplier.
// Regular CamelEntity: 1.0 (default from MobEntity)
// CamelHuskEntity: 4.0
func (cs *CamelState) GetChargingSpeedMultiplier() float64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.IsCamelHusk {
		return CamelHuskChargingSpeedMultiplier
	}
	return 1.0
}

// EndDash clears the dashing flag.
func (cs *CamelState) EndDash() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Dashing = false
}

// GetDashCooldownTicks returns the current dash cooldown ticks.
func (cs *CamelState) GetDashCooldownTicks() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.DashCooldownTicks
}

// SetCharging sets the charging state.
func (cs *CamelState) SetCharging(charging bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.IsCharging = charging
}

// SetJumpChargeTicks sets the jump charge ticks.
func (cs *CamelState) SetJumpChargeTicks(ticks int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.JumpChargeTicks = ticks
}

// AddJumpChargeTicks adds to the jump charge ticks.
func (cs *CamelState) AddJumpChargeTicks(delta int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.JumpChargeTicks += delta
}

// GetJumpChargeTicks returns the current jump charge ticks.
func (cs *CamelState) GetJumpChargeTicks() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.JumpChargeTicks
}

// GetIsCharging returns whether the camel is currently charging.
func (cs *CamelState) GetIsCharging() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.IsCharging
}

// GetIsCamelHusk returns whether the camel is a camel husk.
func (cs *CamelState) GetIsCamelHusk() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.IsCamelHusk
}

// DashImpulse computes the velocity impulse vector for a camel dash.
// Returns (deltaVelX, deltaVelY, deltaVelZ) to be added to the entity's velocity.
//
// Java CamelEntity.jump(float strength, Vec3d movementInput):
//
//	horizontal = rotationVector.normalize() * 22.2222 * strength * movementSpeed * velocityMultiplier
//	vertical   = 1.4285 * strength * jumpVelocity
func DashImpulse(yawDegrees, strength, movementSpeed, velocityMultiplier float64) (deltaVelX, deltaVelY, deltaVelZ float64) {
	yawRad := yawDegrees * math.Pi / 180.0
	// Java: getRotationVector().multiply(1,0,1).normalize() gives the horizontal facing direction
	facingX := -math.Sin(yawRad)
	facingZ := math.Cos(yawRad)

	horizontalMagnitude := CamelDashHorizontalFactor * strength * movementSpeed * velocityMultiplier
	verticalMagnitude := CamelDashVerticalFactor * strength * CamelJumpStrength

	deltaVelX = facingX * horizontalMagnitude
	deltaVelY = verticalMagnitude
	deltaVelZ = facingZ * horizontalMagnitude
	return
}
