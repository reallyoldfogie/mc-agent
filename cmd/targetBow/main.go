package main

import (
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// The InitialSpeed in the simulation corresponds to a fully charged bow (approx. 1 second hold duration).
// This single power level often provides the best accuracy. If you need variable power, you would need to
// iterate through different InitialSpeed values (representing different hold durations) within your
// findBestPitch function as well.

// Minecraft physics constants
const (
	Gravity      = 0.05
	Drag         = 0.99
	InitialSpeed = 3.1 // Approximated max charge speed
	TickRate     = 20  // Minecraft runs at 20 ticks per second
)

// Point represents a 3D coordinate
type Point struct {
	X, Y, Z float64
}

// calculateAiming calculates the required pitch (in degrees) to hit a target.
func CalculateAiming(botPos, targetPos Point) (yaw, pitch, power float64) {
	const playerEyeHeight = models.PlayerEyeHeight
	dx := targetPos.X - botPos.X
	dy := targetPos.Y - botPos.Y + playerEyeHeight // Use bot eye height for origin
	dz := targetPos.Z - botPos.Z

	horizontalDistance := math.Sqrt(dx*dx + dz*dz)

	// Calculate yaw for horizontal rotation
	// NOTE: Trajectory is simulated in local space with Z=forward, X=0
	// Yaw formula must convert from world delta to firing direction
	// atan2(-dx, dz) accounts for the coordinate system rotation
	yaw = math.Atan2(-dx, dz) * 180 / math.Pi

	// Pitch calculation: Use the iterative solver from before, passing the correct distances
	// pitch = findBestPitch(horizontalDistance, dy)
	// Search for the best combination of power and pitch
	bestPitch, bestPower := findBestShot(horizontalDistance, dy)

	return yaw, bestPitch, bestPower
}

// findBestShot simulates arrow trajectories to find the optimal pitch and power.
func findBestShot(horizontalDist, verticalDist float64) (float64, float64) {
	const (
		minPitch  = -90.0
		maxPitch  = 90.0
		pitchStep = 0.1
		minPower  = 0.1 // Minimum useful power
		maxPower  = 1.0 // Full charge power (1 second hold)
		powerStep = 0.05
	)

	bestPitch := 0.0
	bestPower := 1.0
	minError := math.MaxFloat64

	for power := minPower; power <= maxPower; power += powerStep {
		for pitch := minPitch; pitch <= maxPitch; pitch += pitchStep {
			pitchRad := pitch * math.Pi / 180.0

			// Pass the current power factor to the simulator
			hitX, _ /*hitY*/, hit := simulateArrow(pitchRad, power, horizontalDist, verticalDist)

			if hit {
				// Calculate error based on how close we are to the *exact* target XZ coordinates
				// We simplify by comparing the final horizontal distance to the target's horizontal distance
				error := math.Abs(hitX - horizontalDist)

				if error < minError {
					minError = error
					bestPitch = pitch
					bestPower = power
				}
			}
		}
	}
	// If no perfect shot is found within constraints, return the closest approximation
	return bestPitch, bestPower
}

// simulateArrow runs the physics simulation for a given pitch, power, and target conditions.
func simulateArrow(pitchRad, powerFactor, targetHorizontalDist, targetVerticalDist float64) (float64, float64, bool) {
	// Calculate initial speed based on the power factor 'f'
	// The base max speed seems to be around 3.1, so InitialSpeed = 3.1 * powerFactor (f from the formula) is a good approximation
	initialSpeed := 3.1 * powerFactor

	velY := math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	posX, posY := 0.0, 0.0 // Simplified 2D plane simulation (horizontal distance, vertical position)

	for tick := 0; tick < 400; tick++ {
		posX += velXZ
		posY += velY

		velY -= Gravity
		velY *= Drag
		velXZ *= Drag

		// Check if the arrow is near the target's height
		if posY <= targetVerticalDist && posY >= targetVerticalDist-0.5 { // Check within a small vertical tolerance (0.5 blocks)
			// Check if it's within a reasonable horizontal range as well
			if math.Abs(posX) >= math.Abs(targetHorizontalDist)-0.5 && math.Abs(posX) <= math.Abs(targetHorizontalDist)+0.5 {
				return math.Abs(posX), posY, true
			}
		}

		// If the arrow goes way below the target height and past horizontal distance, stop simulation
		if posY < targetVerticalDist-10 || math.Abs(posX) > math.Abs(targetHorizontalDist)+20 {
			break // Optimization: stop if trajectory is clearly wrong
		}
	}
	return math.Abs(posX), posY, false
}

func main() {
	// Example usage
	botPosition := Point{X: 0, Y: 64, Z: 0}
	targetPosition := Point{X: 100, Y: -64, Z: 10}

	for targetPosition.Y = -64; targetPosition.Y <= 128; targetPosition.Y += 1 {
		yaw, pitchAngle, powerFactor := CalculateAiming(botPosition, targetPosition)

		// Map the power factor back to a hold duration for your bot's action system (e.g., 0.1 to 1.0 seconds)
		holdDurationSeconds := powerFactor * 1.0
		holdDuration := durationFromSeconds(holdDurationSeconds)

		// Use yaw, pitchAngle, and holdDuration to aim and shoot
		fmt.Println("Calculated Yaw:", yaw)
		fmt.Println("Calculated Pitch:", pitchAngle)
		fmt.Println("Calculated holdDuration to hit target:", holdDuration)
		fmt.Println("-----")
	}
}

func durationFromSeconds(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}
