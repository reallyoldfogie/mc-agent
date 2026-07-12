package models

import (
	"math"
	"sync"
)

// Nautilus physics constants matching Java AbstractNautilusEntity (1.21.11).
// Source: net/minecraft/entity/passive/AbstractNautilusEntity.java
const (
	// NautilusDefaultMovementSpeed is the regular nautilus base movement_speed
	// attribute. Java: createNautilusAttributes() .add(MOVEMENT_SPEED, 1.0).
	NautilusDefaultMovementSpeed = 1.0

	// NautilusZombieMovementSpeed is the zombie nautilus base movement_speed
	// attribute. Java: createZombieNautilusAttributes() .add(MOVEMENT_SPEED, 1.1).
	NautilusZombieMovementSpeed = 1.1

	// NautilusDashCooldownTicks is the dash cooldown after a spear-charge dash.
	// Java AbstractNautilusEntity.dash(): this.jumpCooldown = 40.
	NautilusDashCooldownTicks = 40

	// NautilusDashActiveTicks is how long the dashing flag stays set after a dash.
	// Java tick(): clears dashing once jumpCooldown < 35, i.e. 5 ticks after the
	// 40-tick reset.
	NautilusDashActiveTicks = 5

	// NautilusDashChargeCapTicks caps the accumulated jump charge. ClampJumpStrength
	// saturates at 90, so 100 leaves a small margin and matches the camel charge cap.
	NautilusDashChargeCapTicks = 100

	// NautilusWaterDashFactor scales the dash impulse while touching water.
	// Java dash(): (this.isTouchingWater() ? 1.2F : 0.5F).
	NautilusWaterDashFactor = 1.2

	// NautilusLandDashFactor scales the dash impulse while on land.
	// Java dash(): (this.isTouchingWater() ? 1.2F : 0.5F).
	NautilusLandDashFactor = 0.5
)

// NautilusState tracks the client-predicted control state for a ridden nautilus
// or zombie nautilus. It mirrors the server-side fields that affect movement
// prediction:
//   - the entity's own yaw, which eases toward the rider's look yaw each tick
//     (Java tickControlled / getControlledRotation),
//   - the dash cooldown and dashing flag (DASHING tracked data + jumpCooldown),
//   - the jump-charge accumulator used to build dash strength.
//
// Field access is synchronized with mu so the physics tick loop can write while
// tests read.
type NautilusState struct {
	mu sync.RWMutex

	// VehicleYaw is the nautilus's own yaw in degrees. Java tickControlled eases
	// it toward the rider's look yaw by half the wrapped delta each tick.
	VehicleYaw float64

	// IsZombie selects the default movement_speed used when the server has not
	// sent generic.movement_speed: 1.0 for nautilus, 1.1 for zombie nautilus.
	IsZombie bool

	// DashCooldownTicks counts down from NautilusDashCooldownTicks (40) to 0.
	// New dashes cannot start while it is > 0.
	DashCooldownTicks int

	// Dashing mirrors the Java DASHING tracked data.
	Dashing bool

	// JumpChargeTicks accumulates while the jump key is held (0–100), converted
	// to a 0.4–1.0 strength via ClampJumpStrength.
	JumpChargeTicks int

	// IsCharging is true while the jump key is held, accumulating charge.
	IsCharging bool
}

// NewNautilusState creates a NautilusState seeded with the mount's current yaw.
// isZombie selects the zombie nautilus default movement speed.
func NewNautilusState(initialYaw float64, isZombie bool) *NautilusState {
	return &NautilusState{
		VehicleYaw: initialYaw,
		IsZombie:   isZombie,
	}
}

// DefaultMovementSpeed returns the base movement_speed attribute to use when the
// server has not sent generic.movement_speed for the mount.
func (ns *NautilusState) DefaultMovementSpeed() float64 {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	if ns.IsZombie {
		return NautilusZombieMovementSpeed
	}
	return NautilusDefaultMovementSpeed
}

// GetVehicleYaw returns the nautilus's current (eased) yaw in degrees.
func (ns *NautilusState) GetVehicleYaw() float64 {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.VehicleYaw
}

// EaseYawToward eases the vehicle yaw toward the rider's look yaw by half the
// wrapped delta and returns the updated yaw. Mirrors Java tickControlled:
//
//	f += MathHelper.wrapDegrees(riderYaw - f) * 0.5
func (ns *NautilusState) EaseYawToward(riderYaw float64) float64 {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	delta := wrapDegrees(riderYaw - ns.VehicleYaw)
	ns.VehicleYaw += delta * 0.5
	return ns.VehicleYaw
}

// CanStartDash reports whether a new dash can begin (cooldown elapsed). The
// saddle requirement is implied because the agent is already riding.
func (ns *NautilusState) CanStartDash() bool {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.DashCooldownTicks <= 0
}

// SetCharging sets whether the jump key is held and charging a dash.
func (ns *NautilusState) SetCharging(charging bool) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.IsCharging = charging
}

// GetIsCharging returns whether a dash is currently charging.
func (ns *NautilusState) GetIsCharging() bool {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.IsCharging
}

// SetJumpChargeTicks sets the accumulated jump-charge ticks.
func (ns *NautilusState) SetJumpChargeTicks(ticks int) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.JumpChargeTicks = ticks
}

// AddJumpChargeTicks adds to the accumulated jump-charge ticks, clamping to the
// charge cap.
func (ns *NautilusState) AddJumpChargeTicks(delta int) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.JumpChargeTicks += delta
	if ns.JumpChargeTicks > NautilusDashChargeCapTicks {
		ns.JumpChargeTicks = NautilusDashChargeCapTicks
	}
}

// GetJumpChargeTicks returns the accumulated jump-charge ticks.
func (ns *NautilusState) GetJumpChargeTicks() int {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.JumpChargeTicks
}

// ApplyDash records that a dash fired: it starts the cooldown, sets the dashing
// flag, and clears the charge accumulator.
func (ns *NautilusState) ApplyDash() {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.DashCooldownTicks = NautilusDashCooldownTicks
	ns.Dashing = true
	ns.IsCharging = false
	ns.JumpChargeTicks = 0
}

// TickDashCooldown advances the dash timers by one tick. It decrements the
// cooldown and clears the dashing flag once the active window
// (NautilusDashActiveTicks) has elapsed. Mirrors Java tick(), where the dashing
// flag clears a few ticks into the 40-tick jump cooldown; the exact tick is
// cosmetic (the flag does not affect movement prediction). Returns true if the
// cooldown just reached zero this tick.
func (ns *NautilusState) TickDashCooldown() bool {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if ns.DashCooldownTicks <= 0 {
		return false
	}
	ns.DashCooldownTicks--
	if ns.Dashing && ns.DashCooldownTicks <= NautilusDashCooldownTicks-NautilusDashActiveTicks {
		ns.Dashing = false
	}
	return ns.DashCooldownTicks == 0
}

// IsDashing returns whether the nautilus is in its dash lunge.
func (ns *NautilusState) IsDashing() bool {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.Dashing
}

// NautilusDashImpulse computes the velocity impulse for a nautilus spear-charge
// dash. Java AbstractNautilusEntity.dash():
//
//	velocity += controller.getRotationVector()
//	    * (isTouchingWater ? 1.2 : 0.5) * strength * movementSpeed * velocityMultiplier
//
// The rotation vector is the rider's full 3D look direction (yaw + pitch), so a
// dash while looking up/down carries a vertical component. Returns the per-axis
// velocity delta to add to the mount's velocity.
func NautilusDashImpulse(lookYawDeg, lookPitchDeg, strength, movementSpeed, velocityMultiplier float64, inWater bool) (deltaVelX, deltaVelY, deltaVelZ float64) {
	yawRad := lookYawDeg * math.Pi / 180.0
	pitchRad := lookPitchDeg * math.Pi / 180.0

	// Minecraft Entity.getRotationVector(pitch, yaw).
	lookX := -math.Cos(pitchRad) * math.Sin(yawRad)
	lookY := -math.Sin(pitchRad)
	lookZ := math.Cos(pitchRad) * math.Cos(yawRad)

	factor := NautilusLandDashFactor
	if inWater {
		factor = NautilusWaterDashFactor
	}
	magnitude := factor * strength * movementSpeed * velocityMultiplier

	deltaVelX = lookX * magnitude
	deltaVelY = lookY * magnitude
	deltaVelZ = lookZ * magnitude
	return
}

// wrapDegrees wraps an angle into [-180, 180), matching Java MathHelper.wrapDegrees.
func wrapDegrees(degrees float64) float64 {
	wrapped := math.Mod(degrees, 360.0)
	if wrapped >= 180.0 {
		wrapped -= 360.0
	}
	if wrapped < -180.0 {
		wrapped += 360.0
	}
	return wrapped
}
