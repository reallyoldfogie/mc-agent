package agent

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"
)

// Minecraft physics constants
const (
	Gravity = 0.05
	Drag    = 0.99
	// InitialSpeed = 3.1 // Approximated max charge speed
	TickRate = 20 // Minecraft runs at 20 ticks per second
)

// Point represents a 3D coordinate
type Point struct {
	X, Y, Z float64
}

// FireBow fires a bow with default hold time using version-specific handlers
// Deprecated: Use FireBowAt for targeted firing
func (a *agent) FireBow() error {
	if a.client == nil || a.versionHandler == nil {
		return fmt.Errorf("client or version handler not ready")
	}

	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return fmt.Errorf("action handler not available")
	}

	// Get next sequence number
	sequence := a.getNextSequence()

	// Use main hand then simulate action
	yaw, pitch := a.getRotation()
	if err := actions.SendUseItem(a.client.Conn(), 0, sequence, yaw, pitch); err != nil {
		return fmt.Errorf("send use item: %w", err)
	}

	// Hold for some ticks, then shoot
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = actions.SendPlayerAction(a.client.Conn(), 0, 0, 0, 0, 0, a.getNextSequence())
			time.Sleep(bowHoldSleep)
		}
		_ = actions.SendPlayerAction(a.client.Conn(), 5, 0, 0, 0, 0, a.getNextSequence())
	}()

	return nil
}

// cmdFireBow via UseItem + PlayerAction (legacy chat command)
func (a *agent) cmdFireBow() {
	_ = a.FireBow() // Delegate to the main implementation
}

// FireBowAt fires a bow at a specific target position using version-specific handlers
func (a *agent) FireBowAt(x, y, z float64) error {
	if a.client == nil || a.versionHandler == nil {
		return fmt.Errorf("client or version handler not ready")
	}

	if a.moveExec == nil {
		return fmt.Errorf("movement executor not available")
	}

	if err := a.TurnTowards(context.Background(), x, y, z); err != nil {
		return fmt.Errorf("turn towards target: %w", err)
	}

	time.Sleep(200 * time.Millisecond) // Small delay to ensure rotation is processed

	botX, botY, botZ, _, _, initialized := a.GetPosition()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}
	yaw, pitch, powerFactor := CalculateAiming(Point{X: botX, Y: botY, Z: botZ}, Point{X: x, Y: y, Z: z})

	a.setPosition(botX, botY, botZ, float32(yaw), float32(pitch))

	// Visualize trajectory with particles for debugging (uses RCON if available)
	a.visualizeArrowTrajectory(botX, botY, botZ, yaw, pitch, powerFactor, x, y, z)

	// Map the power factor back to a hold duration for your bot's action system (e.g., 0.1 to 1.0 seconds)
	holdDurationSeconds := powerFactor * 1.0
	holdDuration := durationFromSeconds(holdDurationSeconds)

	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return fmt.Errorf("action handler not available")
	}

	// Get sequence and use action handler
	sequence := a.getNextSequence()
	if err := actions.SendUseItem(a.client.Conn(), 0, sequence, float32(yaw), float32(pitch)); err != nil {
		return fmt.Errorf("send use item: %w", err)
	}

	// Hold for some ticks, then shoot
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = actions.SendPlayerAction(a.client.Conn(), 0, 0, 0, 0, 0, a.getNextSequence())
			time.Sleep(holdDuration / time.Duration(bowHoldIterations))
		}
		_ = actions.SendPlayerAction(a.client.Conn(), 5, 0, 0, 0, 0, a.getNextSequence())
	}()

	return nil
}

// cmdFireBowAt via UseItem + PlayerAction (legacy chat command)
func (a *agent) cmdFireBowAt(x, y, z float64) {
	if err := a.FireBowAt(x, y, z); err != nil {
		_ = a.SendChat("Fire bow at error: " + err.Error())
	}
}

func durationFromSeconds(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// calculateAiming calculates the required pitch (in degrees) to hit a target.
func CalculateAiming(botPos, targetPos Point) (yaw, pitch, power float64) {
	// Note: Server spawns arrows at approximately Y = botY + 1.5, not at eye level (botY + 1.62)
	// This is the actual arrow spawn position observed from server behavior
	const arrowSpawnHeight = 1.5
	dx := targetPos.X - botPos.X
	dy := targetPos.Y - (botPos.Y + arrowSpawnHeight) // Adjust for actual arrow spawn height
	dz := targetPos.Z - botPos.Z

	horizontalDistance := math.Sqrt(dx*dx + dz*dz)

	// Yaw calculation: atan2(dx, -dz) points north/south based on Z, east/west based on X
	// In Minecraft: +X is East, +Z is South, Yaw 0 is South, 90 is West, -90 is East, 180/-180 is North
	yaw = math.Atan2(dx, -dz) * 180 / math.Pi

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
	log.Printf("[findBestShot] horizontalDist=%.2f, verticalDist=%.2f", horizontalDist, verticalDist)

	// Search from high power to low power to prefer higher power when accuracy is similar
	for power := maxPower; power >= minPower; power -= powerStep {
		for pitch := minPitch; pitch <= maxPitch; pitch += pitchStep {
			pitchRad := pitch * math.Pi / 180.0

			// Pass the current power factor to the simulator
			hitX, hitY, hit := simulateArrow(pitchRad, power, horizontalDist, verticalDist)

			if hit {
				// Calculate error based on how close we are to the *exact* target XZ coordinates
				// We simplify by comparing the final horizontal distance to the target's horizontal distance
				error := math.Abs(hitX - horizontalDist)

				// Prefer solutions with lower absolute pitch (more direct shots) when errors are nearly equal
				// Use small epsilon for floating point comparison
				const epsilon = 0.01
				if error < minError-epsilon {
					// Clearly better error
					minError = error
					bestPitch = pitch
					bestPower = power
					log.Printf("  [findBestShot] Better match: pitch=%.1f, power=%.2f, hitX=%.2f, hitY=%.2f, error=%.2f", pitch, power, hitX, hitY, error)
				} else if math.Abs(error-minError) < epsilon && (math.Abs(pitch) < math.Abs(bestPitch) || math.Abs(bestPitch) == 0) {
					// Errors are similar, prefer lower angle (or higher power if same angle already selected)
					if math.Abs(pitch) < math.Abs(bestPitch) || (math.Abs(pitch) == math.Abs(bestPitch) && power > bestPower) {
						minError = error
						bestPitch = pitch
						bestPower = power
						log.Printf("  [findBestShot] Better match (lower angle or higher power): pitch=%.1f, power=%.2f, hitX=%.2f, hitY=%.2f, error=%.2f", pitch, power, hitX, hitY, error)
					}
				}
			}
		}
	}
	log.Printf("[findBestShot] Final: pitch=%.1f, power=%.2f, minError=%.2f", bestPitch, bestPower, minError)
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

	// Calculate distance to target to estimate simulation time needed
	maxTicks := 200 + int(targetHorizontalDist*20) // ~20 ticks per block of horizontal distance

	for range maxTicks {
		posX += velXZ
		posY += velY

		// Apply drag first, then gravity
		velXZ *= Drag
		velY *= Drag
		velY -= Gravity

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

// visualizeArrowTrajectory displays the arrow's path using particles via RCON
func (a *agent) visualizeArrowTrajectory(botX, botY, botZ, yaw, pitch, power, targetX, targetY, targetZ float64) {
	// Skip if we don't have RCON access (e.g., in non-testing scenarios)
	if a.cfg.RCON == nil {
		log.Printf("[visualizeArrowTrajectory] RCON not available, skipping visualization")
		return
	}

	log.Printf("[visualizeArrowTrajectory] Starting visualization %.2f %.2f %.2f => %.2f %.2f %.2f", botX, botY, botZ, targetX, targetY, targetZ)

	pitchRad := pitch * math.Pi / 180.0
	yawRad := yaw * math.Pi / 180.0

	// Calculate initial speed based on power
	initialSpeed := 3.1 * power

	// Calculate velocity components
	velY := math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	// Calculate direction in XZ plane from yaw
	velX := math.Sin(yawRad) * velXZ
	velZ := math.Cos(yawRad) * velXZ

	// Calculate distance to target to estimate simulation time needed
	horizontalDist := math.Sqrt((targetX-botX)*(targetX-botX) + (targetZ-botZ)*(targetZ-botZ))

	// Arrow starts at server's actual spawn height (not eye level)
	const arrowSpawnHeight = 1.5
	arrowX := botX
	arrowY := botY + arrowSpawnHeight
	arrowZ := botZ

	// Debug logging
	log.Printf("[visualizeArrowTrajectory] Velocity calculation: yaw=%.2f°, pitch=%.2f°, power=%.2f", yaw, pitch, power)
	log.Printf("[visualizeArrowTrajectory] Initial velocity: velX=%.3f, velY=%.3f, velZ=%.3f, velXZ=%.3f, initialSpeed=%.3f", velX, velY, velZ, velXZ, initialSpeed)
	log.Printf("[visualizeArrowTrajectory] Arrow spawn: (%.2f, %.2f, %.2f)", arrowX, arrowY, arrowZ)
	log.Printf("[visualizeArrowTrajectory] Target: (%.2f, %.2f, %.2f), horizontalDist=%.2f", targetX, targetY, targetZ, horizontalDist)
	maxTicks := 200 + int(horizontalDist*20) // ~20 ticks per block of horizontal distance

	// Simulate trajectory and mark with particles
	var positions []string
	for step := 0; step < maxTicks; step += 5 { // Sample every 5 ticks for visualization
		// Update position
		arrowX += velX
		arrowY += velY
		arrowZ += velZ

		// Apply drag and gravity
		velX *= Drag
		velZ *= Drag
		velY *= Drag
		velY -= Gravity

		// Capture all trajectory points
		positions = append(positions, fmt.Sprintf("/particle flame %.2f %.2f %.2f 0 0 0 0 0", arrowX, arrowY, arrowZ))

		// Stop if arrow goes way below the target
		if arrowY < targetY-20 {
			break
		}
	}

	// Send particle commands via RCON to visualize the path
	log.Printf("[visualizeArrowTrajectory] Sending %d position markers via RCON", len(positions))
	ctx := context.Background()
	for i, cmd := range positions {
		// if i%5 == 0 { // Sample particles to avoid spam
		log.Printf("[visualizeArrowTrajectory] Particle %d: %s", i, cmd)
		_, _ = a.cfg.RCON.Exec(ctx, cmd)
		time.Sleep(5 * time.Millisecond) // Small delay to avoid rate limiting
		// }
	}

	// Also mark the target with a different particle
	targetCmd := fmt.Sprintf("particle end_rod %.2f %.2f %.2f 0 0 0 0 1", targetX, targetY, targetZ)
	log.Printf("[visualizeArrowTrajectory] Target marker: %s", targetCmd)
	_, _ = a.cfg.RCON.Exec(ctx, targetCmd)
}
