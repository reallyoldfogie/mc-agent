package movement

import (
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// This file holds the pure (dependency-free) arithmetic behind the riding
// handlers. Keeping the formulas here — separate from the block lookups,
// packet sends and locking in the riding_*.go handlers — lets them be unit
// tested without a live Minecraft server or the agent/network stack.

// boatSurface identifies which vanilla AbstractBoatEntity surface profile
// applies to a ridden boat on the current tick.
type boatSurface int

const (
	// boatSurfaceWater is a boat floating on still water.
	boatSurfaceWater boatSurface = iota
	// boatSurfaceFlowingWater is a boat on water with a non-zero flow speed.
	boatSurfaceFlowingWater
	// boatSurfaceSubmerged is a boat with water both below and above it.
	boatSurfaceSubmerged
	// boatSurfaceLand is a boat resting on a normal (non-icy) block.
	boatSurfaceLand
	// boatSurfaceIce is a boat resting on ice or packed ice.
	boatSurfaceIce
	// boatSurfaceBlueIce is a boat resting on blue ice, the slipperiest surface.
	boatSurfaceBlueIce
)

// String returns a human-readable name for the surface, used in riding logs.
func (surface boatSurface) String() string {
	switch surface {
	case boatSurfaceWater:
		return "water"
	case boatSurfaceFlowingWater:
		return "flowing_water"
	case boatSurfaceSubmerged:
		return "submerged_water"
	case boatSurfaceLand:
		return "land"
	case boatSurfaceIce:
		return "ice"
	case boatSurfaceBlueIce:
		return "blue_ice"
	default:
		return "unknown"
	}
}

// selectBoatSurface picks the surface profile from already-resolved world
// queries. Mirrors the branch order in vanilla AbstractBoatEntity: flowing
// water wins over submersion, submersion wins over still water, and only when
// the boat is not in water at all does the block name matter.
func selectBoatSurface(isWater, isFlowing, isSubmerged bool, blockName string) boatSurface {
	if isWater {
		switch {
		case isFlowing:
			return boatSurfaceFlowingWater
		case isSubmerged:
			return boatSurfaceSubmerged
		default:
			return boatSurfaceWater
		}
	}

	switch blockName {
	case "minecraft:blue_ice":
		return boatSurfaceBlueIce
	case "minecraft:ice", "minecraft:packed_ice":
		return boatSurfaceIce
	default:
		return boatSurfaceLand
	}
}

// boatSurfacePhysics returns the per-tick horizontal velocity multiplier and
// the vertical gravity delta for a boat on the given surface.
func boatSurfacePhysics(surface boatSurface) (velocityMultiplier, gravity float64) {
	switch surface {
	case boatSurfaceFlowingWater:
		return physics.BoatUnderFlowingWaterVelocityMultiplier, physics.BoatUnderFlowingWaterGravity
	case boatSurfaceSubmerged:
		return physics.BoatUnderWaterVelocityMultiplier, physics.BoatUnderWaterGravity
	case boatSurfaceIce:
		return physics.BoatOnLandIceVelocityMultiplier, physics.BoatOnLandGravity
	case boatSurfaceBlueIce:
		return physics.BoatOnLandBlueIceVelocityMultiplier, physics.BoatOnLandGravity
	case boatSurfaceLand:
		return physics.BoatOnLandStandardVelocityMultiplier, physics.BoatOnLandGravity
	default:
		return physics.BoatInWaterVelocityMultiplier, physics.BoatInWaterGravity
	}
}

// boatThrustSpeed returns the scalar thrust applied along the boat's heading
// for the current paddle inputs. Vanilla uses asymmetric constants: forward
// adds 0.04 while backward only subtracts 0.005.
func boatThrustSpeed(forward, backward bool) float64 {
	thrustSpeed := 0.0
	if forward {
		thrustSpeed += physics.BoatForwardAcceleration
	}
	if backward {
		thrustSpeed -= physics.BoatBackwardAcceleration
	}
	return thrustSpeed
}

// boatYawAcceleration returns the per-tick change in the boat's angular
// velocity for the current steering inputs (±1 degree/tick, cancelling when
// both are held).
func boatYawAcceleration(left, right bool) float64 {
	yawAcceleration := 0.0
	if left {
		yawAcceleration--
	}
	if right {
		yawAcceleration++
	}
	return yawAcceleration
}

// zeroTinyVelocity mirrors vanilla Entity.resetVelocityIfSmall: components
// below the 0.003 threshold are snapped to zero so mounts come to a complete
// stop instead of drifting forever.
func zeroTinyVelocity(velocity float64) float64 {
	if math.Abs(velocity) < physics.ResetVelocity {
		return 0
	}
	return velocity
}

// forwardVelocityToWorld rotates a scalar forward velocity into a world-space
// X/Z pair using the mount's yaw. Horses, camels, pigs and striders all steer
// by facing direction and cannot strafe, so their entire horizontal velocity
// is this single scalar projected through yaw.
func forwardVelocityToWorld(forwardVelocity, yawDegrees float64) (velX, velZ float64) {
	yawRadians := yawDegrees * math.Pi / 180.0
	return -math.Sin(yawRadians) * forwardVelocity, math.Cos(yawRadians) * forwardVelocity
}

// mountSurface identifies which medium a ridden land mount (horse, donkey,
// mule, camel) is travelling through on the current tick.
type mountSurface int

const (
	// mountSurfaceGround is a mount supported by a solid block.
	mountSurfaceGround mountSurface = iota
	// mountSurfaceAirborne is a mount in free fall or rising from a jump.
	mountSurfaceAirborne
	// mountSurfaceWater is a mount whose body is in water.
	mountSurfaceWater
)

// mountSurfacePhysics returns the per-tick horizontal drag and the vertical
// gravity delta for a ridden land mount.
//
// The water values are supplied by the caller because they are version-gated
// (pre-1.21.11 mounts sink, later ones float), while the ground value derives
// from the block's slipperiness and the airborne values are vanilla constants.
func mountSurfacePhysics(
	surface mountSurface,
	waterVelocityDrag, waterGravityDelta, blockSlipperiness float64,
) (velocityDrag, gravityDelta float64) {
	switch surface {
	case mountSurfaceWater:
		return waterVelocityDrag, waterGravityDelta
	case mountSurfaceAirborne:
		return physics.Inertia, -physics.Gravity
	default:
		return physics.HorseLandFriction(blockSlipperiness), 0.0
	}
}

// mountInputAcceleration returns the horizontal acceleration applied per unit
// of forward throttle. Airborne mounts get only limited air control; without
// this cap a falling mount would accelerate to several times its ground speed.
func mountInputAcceleration(movementSpeed float64, airborne bool) float64 {
	if airborne {
		return physics.RidingAirborneAcceleration
	}
	return movementSpeed
}

// mountForwardVelocity advances the scalar forward velocity by one tick:
// the previous value decays by the surface drag, then the throttle input adds
// acceleration. Tiny results are snapped to zero.
func mountForwardVelocity(previousForwardVelocity, velocityDrag, throttleZ, inputAcceleration float64) float64 {
	return zeroTinyVelocity(previousForwardVelocity*velocityDrag + throttleZ*inputAcceleration)
}

// mountVerticalVelocity advances the vertical velocity by one tick. Airborne
// mounts additionally get the 0.98 air drag from vanilla travelMidAir.
func mountVerticalVelocity(previousVerticalVelocity, gravityDelta float64, airborne bool) float64 {
	verticalVelocity := previousVerticalVelocity + gravityDelta
	if airborne {
		verticalVelocity *= physics.Drag
	}
	return verticalVelocity
}

// mountJumpChargeCapTicks is the number of held ticks at which a mount's jump
// or dash charge saturates and fires on its own.
const mountJumpChargeCapTicks = 100

// mountJumpChargeStrength converts held charge ticks into the integer strength
// percent the client sends to the server and the clamped float strength the
// local prediction uses.
//
// This mirrors the two-step vanilla path rather than clamping the raw tick
// count: ClientPlayerEntity.tickMovement ramps mountJumpStrength to 1.0 by
// tick 10, sends floor(strength*100), and the server re-clamps that percent
// through JumpingMount.clampJumpStrength. Clamping the ticks directly makes
// the ramp roughly 10x too slow, so a multi-second hold never reaches full
// strength.
func mountJumpChargeStrength(chargeTicks int) (strengthPercent int, strength float64) {
	strengthPercent = int(math.Floor(models.MountJumpStrength(chargeTicks) * 100.0))
	if strengthPercent > 100 {
		strengthPercent = 100
	}
	return strengthPercent, models.ClampJumpStrength(strengthPercent)
}

// airSlipperiness is the slipperiness Java uses for an airborne entity, where
// there is no block below to sample: friction = 1.0 * 0.91.
const airSlipperiness = 1.0

// travelMidAirGroundSpeedFactor is the numerator in Java
// LivingEntity.getMovementSpeed(slipperiness): speed * (0.21600002 / slip³).
const travelMidAirGroundSpeedFactor = 0.21600002

// travelMidAirOffGroundSpeedFactor mirrors Java LivingEntity.getOffGroundSpeed():
// movementSpeed * 0.1 when the controlling passenger is a player.
const travelMidAirOffGroundSpeedFactor = 0.1

// travelMidAirSpeedFactor mirrors Java LivingEntity.getMovementSpeed(slipperiness).
// Grounded entities get a slipperiness-compensated boost so that the following
// friction multiply nets out to a constant top speed; airborne entities fall
// back to the reduced off-ground factor.
func travelMidAirSpeedFactor(movementSpeed, slipperiness float64, onGround bool) float64 {
	if !onGround {
		return movementSpeed * travelMidAirOffGroundSpeedFactor
	}
	slipperinessCubed := slipperiness * slipperiness * slipperiness
	return movementSpeed * (travelMidAirGroundSpeedFactor / slipperinessCubed)
}

// saddledBoostMultiplier mirrors Java SaddledComponent.getMovementSpeedMultiplier():
//
//	1.0 + amplitude * sin(boostTicks / boostTotalTicks * PI)
//
// It returns 1.0 (no boost) when no boost is active or the total duration is
// not yet known, so callers can multiply unconditionally.
func saddledBoostMultiplier(boosted bool, boostTicks, boostTotalTicks int, amplitude float64) float64 {
	if !boosted || boostTotalTicks <= 0 {
		return 1.0
	}
	return 1.0 + amplitude*math.Sin(float64(boostTicks)/float64(boostTotalTicks)*math.Pi)
}

// striderSaddledSpeed mirrors Java StriderEntity.getSaddledSpeed():
//
//	movementSpeed * (cold ? 0.35 : 0.55) * saddledComponent.getMovementSpeedMultiplier()
//
// A cold strider additionally carries the SUFFOCATING_MODIFIER on its
// movement_speed attribute, which is an ADD_MULTIPLIED_BASE operation and so
// scales the base by (1 + modifier) before the cold multiplier applies.
func striderSaddledSpeed(attributeSpeed float64, cold bool, boostMultiplier float64) float64 {
	speed := attributeSpeed
	coldMultiplier := physics.StriderWarmSpeedMultiplier
	if cold {
		speed *= 1.0 + physics.StriderSuffocatingModifier
		coldMultiplier = physics.StriderColdSpeedMultiplier
	}
	return speed * coldMultiplier * boostMultiplier
}
