package agent

import (
	"math"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
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

// fireBow via UseItem + PlayerAction
func (a *agent) cmdFireBow() {
	if a.client == nil || a.packetMgr == nil {
		_ = a.SendChat("Client not ready")
		return
	}

	// Use main hand then simulate action
	useID := a.packetMgr.GetServerboundPacketID("ServerboundUseItem")
	_ = a.client.Conn().WritePacket(pk.Marshal(useID, pk.VarInt(0), pk.VarInt(1), pk.Float(0), pk.Float(0)))

	actID := a.packetMgr.GetServerboundPacketID("ServerboundPlayerAction")

	// Hold for some ticks, then shoot
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = a.client.Conn().WritePacket(pk.Marshal(actID, pk.VarInt(0), pk.Position{X: 0, Y: 0, Z: 0}, pk.Byte(0), pk.VarInt(0)))
			time.Sleep(bowHoldSleep)
		}
		_ = a.client.Conn().WritePacket(pk.Marshal(actID, pk.VarInt(5), pk.Position{X: 0, Y: 0, Z: 0}, pk.Byte(0), pk.VarInt(0)))
	}()
}

// fireBowAt via UseItem + PlayerAction
func (a *agent) cmdFireBowAt(x, y, z float64) {
	if a.client == nil || a.packetMgr == nil {
		_ = a.SendChat("Client not ready")
		return
	}

	err := a.moveExec.LookAt(x, y, z, true)
	if err != nil {
		_ = a.SendChat("Failed to look at target: " + err.Error())
		return
	}

	botX, botY, botZ, _, _, initialized := a.GetPosition()
	if !initialized {
		_ = a.SendChat("Bot position not initialized")
		return
	}
	yaw, pitch, powerFactor := CalculateAiming(Point{X: botX, Y: botY, Z: botZ}, Point{X: x, Y: y, Z: z})

	a.setPosition(x, y, z, float32(yaw), float32(pitch))

	// Map the power factor back to a hold duration for your bot's action system (e.g., 0.1 to 1.0 seconds)
	holdDurationSeconds := powerFactor * 1.0
	holdDuration := durationFromSeconds(holdDurationSeconds)

	// Use main hand then simulate action
	useID := a.packetMgr.GetServerboundPacketID("ServerboundUseItem")
	_ = a.client.Conn().WritePacket(pk.Marshal(useID, pk.VarInt(0), pk.VarInt(1), pk.Float(0), pk.Float(0)))

	actID := a.packetMgr.GetServerboundPacketID("ServerboundPlayerAction")

	// Hold for some ticks, then shoot
	go func() {
		for i := 0; i < bowHoldIterations; i++ {
			_ = a.client.Conn().WritePacket(pk.Marshal(actID, pk.VarInt(0), pk.Position{X: 0, Y: 0, Z: 0}, pk.Byte(0), pk.VarInt(0)))
			time.Sleep(holdDuration / time.Duration(bowHoldIterations))
		}
		_ = a.client.Conn().WritePacket(pk.Marshal(actID, pk.VarInt(5), pk.Position{X: 0, Y: 0, Z: 0}, pk.Byte(0), pk.VarInt(0)))
	}()
}

func durationFromSeconds(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// calculateAiming calculates the required pitch (in degrees) to hit a target.
func CalculateAiming(botPos, targetPos Point) (yaw, pitch, power float64) {
	dx := targetPos.X - botPos.X
	dy := targetPos.Y - botPos.Y + 1.62 // Use bot eye height for origin
	dz := targetPos.Z - botPos.Z

	horizontalDistance := math.Sqrt(dx*dx + dz*dz)

	// Yaw calculation: Standard atan2 for horizontal direction
	yaw = math.Atan2(-dx, -dz) * 180 / math.Pi

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
