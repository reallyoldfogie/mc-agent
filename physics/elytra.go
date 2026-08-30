package physics

import "math"

// This file holds the pure (dependency-free) elytra-gliding formulas, split
// out the same way physics/effects.go separates other status-effect math
// from state.go's tick loop — testable without a live server. Cited
// directly from decompiled net/minecraft/entity/LivingEntity.java
// (calcGlidingVelocity/canGlide/tickGliding, Yarn, 1.21.11) and
// net/minecraft/entity/Entity.java (getRotationVector), not the wiki, since
// the wiki does not spell out the exact per-axis blending coefficients.

// RotationVector returns the look-direction unit vector for the given yaw
// and pitch (in degrees, using this codebase's Minecraft-protocol
// convention — see physics/state.go's yaw/pitch field comments), mirroring
// Java Entity.getRotationVector(pitch, yaw):
//
//	f = pitch in radians; g = -yaw in radians
//	vector = (sin(g)*cos(f), -sin(f), cos(g)*cos(f))
//
// Cross-checked against utils.GetYawAndPitch's own delta-to-yaw/pitch
// formula (its algebraic inverse produces this same vector): that
// function's output is what's actually sent to real Minecraft servers via
// look/position packets throughout this codebase (follow, fireBowAt,
// startTracking), so its sign convention is the one confirmed correct
// against live servers, and it agrees with Java's raw formula here.
func RotationVector(yawDegrees, pitchDegrees float64) (x, y, z float64) {
	yawRad := yawDegrees * math.Pi / 180.0
	pitchRad := pitchDegrees * math.Pi / 180.0
	g := -yawRad
	x = math.Sin(g) * math.Cos(pitchRad)
	y = -math.Sin(pitchRad)
	z = math.Cos(g) * math.Cos(pitchRad)
	return x, y, z
}

// GlidingVelocity mirrors Java LivingEntity.calcGlidingVelocity(): the
// velocity update applied every tick while gliding on an elytra, entirely
// replacing normal gravity/drag (travelMidAir) for the duration of the
// glide. gravity should be the entity's EffectiveGravity for this tick —
// calcGlidingVelocity calls getEffectiveGravity() directly, so Slow
// Falling's cap still applies while gliding.
//
// The three conditional adjustments, in order:
//  1. Gravity, blended by look angle (GlideDiveGravityBlend): looking level
//     cuts effective gravity to 25%; looking straight up/down applies it in
//     full.
//  2. While descending (vy<0) and not looking straight up/down (d>0): pulls
//     velocity toward the look direction, scaled by how fast you're
//     falling — diving forward converts fall speed into forward speed.
//  3. While pitched upward (pitchDegrees<0) and d>0: an extra vertical
//     boost funded by trading away horizontal speed, letting a shallow
//     climb bleed off built-up speed to gain height.
//  4. Unconditionally (d>0): eases horizontal velocity 10%/tick toward the
//     look direction at the current horizontal speed.
//
// Finally, velocity is scaled by (GlideHorizontalDrag, GlideVerticalDrag,
// GlideHorizontalDrag).
func GlidingVelocity(vx, vy, vz, yawDegrees, pitchDegrees, gravity float64) (nx, ny, nz float64) {
	rx, _, rz := RotationVector(yawDegrees, pitchDegrees)
	pitchRad := pitchDegrees * math.Pi / 180.0

	d := math.Sqrt(rx*rx + rz*rz) // horizontal length of the look vector (0 only when looking straight up/down)
	e := math.Sqrt(vx*vx + vz*vz) // horizontal speed before any of this tick's adjustments
	h := math.Cos(pitchRad) * math.Cos(pitchRad)

	vy += gravity * (-1.0 + h*GlideDiveGravityBlend)

	if vy < 0 && d > 0 {
		i := vy * GlideDiveFactor * h
		vx += rx * i / d
		vy += i
		vz += rz * i / d
	}

	if pitchDegrees < 0 && d > 0 {
		i := e * -math.Sin(pitchRad) * GlideClimbFactor
		vx += -rx * i / d
		vy += i * GlideClimbVerticalMultiplier
		vz += -rz * i / d
	}

	if d > 0 {
		vx += (rx/d*e - vx) * GlideHorizontalEaseFactor
		vz += (rz/d*e - vz) * GlideHorizontalEaseFactor
	}

	return vx * GlideHorizontalDrag, vy * GlideVerticalDrag, vz * GlideHorizontalDrag
}

// CanGlide mirrors Java PlayerEntity.canGlide() wrapping
// LivingEntity.canGlide(): gliding requires being airborne (not on
// ground), unmounted, free of Levitation, not flying, and wearing an
// elytra. onGround/hasVehicle/hasLevitation/isFlying short-circuit the
// whole check the same way Java's early-returns do - PlayerEntity.
// canGlide() is literally `!abilities.flying && super.canGlide()`, so
// flying and gliding are mutually exclusive in vanilla (see
// PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4).
func CanGlide(onGround, hasVehicle, hasLevitation, isFlying, elytraEquipped bool) bool {
	if onGround || hasVehicle || hasLevitation || isFlying {
		return false
	}
	return elytraEquipped
}

// CanStartGliding mirrors Java PlayerEntity.checkGliding()'s additional
// gate on top of CanGlide: gliding cannot start while touching water (it
// can continue there once already started — vanilla only re-checks the
// water condition at the start-gliding transition, not every tick via
// canGlide/tickGliding).
func CanStartGliding(alreadyGliding, isTouchingWater bool, canGlide bool) bool {
	return !alreadyGliding && canGlide && !isTouchingWater
}

// FireworkBoostVelocity mirrors Java FireworkRocketEntity.tick()'s
// shooter-velocity nudge, applied every tick a firework rocket used while
// gliding remains alive and attached: velocity eases toward
// FireworkBoostTarget blocks/tick in the look direction, on all three axes
// (including vertical — pitching up while boosting gains real altitude, not
// just speed). This runs unconditionally each tick for the firework's
// lifetime, layered on top of whatever GlidingVelocity already computed
// that same tick — vanilla applies it as a separate, unconditional
// setVelocity call on the shooter, not a modification of the gliding
// formula itself.
func FireworkBoostVelocity(vx, vy, vz, yawDegrees, pitchDegrees float64) (nx, ny, nz float64) {
	rx, ry, rz := RotationVector(yawDegrees, pitchDegrees)
	nx = vx + rx*FireworkBoostBlend + (rx*FireworkBoostTarget-vx)*FireworkBoostEase
	ny = vy + ry*FireworkBoostBlend + (ry*FireworkBoostTarget-vy)*FireworkBoostEase
	nz = vz + rz*FireworkBoostBlend + (rz*FireworkBoostTarget-vz)*FireworkBoostEase
	return nx, ny, nz
}
