package physics

import "math"

// LimitRotation applies rate limits to yaw and pitch changes according to
// Minecraft's anti-cheat constraints (MaxYawChange and MaxPitchChange per tick).
// Returns the new yaw and pitch after limiting.
func LimitRotation(currentYaw, currentPitch, targetYaw, targetPitch float64) (newYaw, newPitch float64) {
	// Calculate yaw delta with wrapping
	deltaYaw := NormalizeAngle(targetYaw - currentYaw)
	deltaYaw = clamp(deltaYaw, -MaxYawChange, MaxYawChange)
	newYaw = currentYaw + deltaYaw

	// Calculate pitch delta (no wrapping needed for pitch)
	deltaPitch := targetPitch - currentPitch
	deltaPitch = clamp(deltaPitch, -MaxPitchChange, MaxPitchChange)
	newPitch = currentPitch + deltaPitch

	return newYaw, newPitch
}

// RotateToward gradually rotates from current to target angle, respecting maxDelta per step.
// This function handles angle wrapping (e.g., 359° → 1° takes shortest path).
// Returns the new angle after rotation.
func RotateToward(current, target, maxDelta float64) float64 {
	delta := NormalizeAngle(target - current)
	delta = clamp(delta, -maxDelta, maxDelta)
	return current + delta
}

// NormalizeAngle normalizes an angle to the range [-180, 180] degrees.
// This ensures we always take the shortest path when rotating.
// Example: NormalizeAngle(270) = -90 (shortest path from 0° to 270° is -90°)
func NormalizeAngle(angle float64) float64 {
	angle = math.Mod(angle, 360)
	if angle > 180 {
		angle -= 360
	} else if angle < -180 {
		angle += 360
	}
	return angle
}

// AngleDifference calculates the shortest angular distance between two angles.
// Always returns a positive value in the range [0, 180].
// Example: AngleDifference(10, 350) = 20 (not 340)
func AngleDifference(from, to float64) float64 {
	return math.Abs(NormalizeAngle(to - from))
}

// IsLookingAt checks if the current yaw/pitch is within tolerance of the target.
// Useful for determining if rotation is "close enough" to target.
func IsLookingAt(currentYaw, currentPitch, targetYaw, targetPitch, yawTolerance, pitchTolerance float64) bool {
	return AngleDifference(currentYaw, targetYaw) <= yawTolerance &&
		math.Abs(targetPitch-currentPitch) <= pitchTolerance
}

// TicksToRotate calculates how many ticks it will take to rotate from current to target
// yaw/pitch with rate limiting applied. Useful for predicting rotation time.
func TicksToRotate(currentYaw, currentPitch, targetYaw, targetPitch float64) int {
	// Calculate required yaw rotation
	yawDelta := AngleDifference(currentYaw, targetYaw)
	yawTicks := int(math.Ceil(yawDelta / MaxYawChange))

	// Calculate required pitch rotation
	pitchDelta := math.Abs(targetPitch - currentPitch)
	pitchTicks := int(math.Ceil(pitchDelta / MaxPitchChange))

	// Return the maximum (rotation finishes when both axes reach target)
	if yawTicks > pitchTicks {
		return yawTicks
	}
	return pitchTicks
}
