package movement

import (
	"log"
	"math"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// minecartRailInfo holds information about a rail block beneath a minecart.
type minecartRailInfo struct {
	found       bool
	shape       string
	isPowered   bool // block is a powered rail
	isEnergized bool // powered rail has redstone power
	railBlockY  int  // integer Y of the rail block
}

// handleRidingModeMinecart runs one client-authoritative minecart tick.
// Rails constrain the minecart's movement direction. The client computes position
// locally and reports it to the server via SendMoveVehicle.
func (pe *PhysicsMovementExecutor) handleRidingModeMinecart(
	versionHandler models.VersionHandler,
	forward, backward, sneak bool,
) {
	// (1) Send vehicle input (forward/backward affect momentum; left/right unused on rails)
	if err := versionHandler.Play().Movement().SendVehicleInput(
		pe.movementPacketSender.client.Conn(),
		forward, backward, false, false, false, sneak,
	); err != nil {
		log.Printf("[handleRidingModeMinecart] SendVehicleInput error: %v", err)
	}

	// Hold lock for entire read-compute-write cycle
	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	currentPos, yaw, pitch, _ := pe.physicsState.GetPosition()

	// Sanitize NaN yaw/pitch that may come from uninitialized physics state
	if math.IsNaN(yaw) {
		log.Printf("[handleRidingModeMinecart] WARNING: yaw is NaN, defaulting to 0.")
		yaw = 0
	}
	if math.IsNaN(pitch) {
		log.Printf("[handleRidingModeMinecart] WARNING: pitch is NaN, defaulting to 0.")
		pitch = 0
	}

	// (2) Detect rail
	rail := pe.detectRailBelow(currentPos.X, currentPos.Y, currentPos.Z)

	// Read current velocity
	ridingVelX := pe.ridingVelX
	ridingVelZ := pe.ridingVelZ

	var newX, newY, newZ float64
	var newVelX, newVelZ float64
	var newVelMultiplier float64

	// Detect water at the minecart's position.
	inWater := false
	if pe.world != nil && pe.shapeProvider != nil {
		blockBelow := pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z)
		inWater = pe.shapeProvider.IsWater(blockBelow)
	}

	if !rail.found {
		// Off-rail: apply gravity accumulation and conditional drag.
		gravityDelta := physics.MinecartFallGravity
		if inWater {
			gravityDelta = physics.MinecartWaterGravity
		}
		pe.ridingVelY += gravityDelta

		offRailDrag := physics.MinecartOffRailAirDrag
		if math.Abs(pe.ridingVelY) < 0.01 {
			offRailDrag = physics.MinecartOffRailDrag
		}

		newVelX = ridingVelX * offRailDrag
		newVelZ = ridingVelZ * offRailDrag
		newX = currentPos.X + newVelX
		newY = currentPos.Y + pe.ridingVelY
		newZ = currentPos.Z + newVelZ
		newVelMultiplier = offRailDrag
	} else {
		// On rail: reset Y velocity
		pe.ridingVelY = 0
		dirX, dirZ, isSloped := railShapeDirection(rail.shape)
		railBlockX := int(math.Floor(currentPos.X))
		railBlockZ := int(math.Floor(currentPos.Z))

		dot := ridingVelX*dirX + ridingVelZ*dirZ
		speed := math.Sqrt(ridingVelX*ridingVelX + ridingVelZ*ridingVelZ)
		if dot < 0 {
			speed = -speed
		}

		// Player input: nudge to break static inertia
		unpoweredBraking := rail.isPowered && !rail.isEnergized
		speedSq := ridingVelX*ridingVelX + ridingVelZ*ridingVelZ
		if speedSq < physics.MinecartNudgeSpeedThreshold {
			var inputDirX, inputDirZ float64
			if forward || backward {
				yawRad := yaw * math.Pi / 180.0
				sinYaw := math.Sin(yawRad)
				cosYaw := math.Cos(yawRad)
				var forwardInput float64
				if forward {
					forwardInput = 1.0
				} else {
					forwardInput = -1.0
				}
				inputDirX = -sinYaw * forwardInput
				inputDirZ = cosYaw * forwardInput
			}
			if inputDirX != 0 || inputDirZ != 0 {
				ridingVelX += inputDirX * physics.MinecartNudgeImpulse
				ridingVelZ += inputDirZ * physics.MinecartNudgeImpulse
				unpoweredBraking = false
				speed = math.Sqrt(ridingVelX*ridingVelX + ridingVelZ*ridingVelZ)
				newDot := ridingVelX*dirX + ridingVelZ*dirZ
				if newDot < 0 {
					speed = -speed
				}
			}
		}

		// Slope gravity
		slopeGravity := physics.MinecartSlopeGravity
		if inWater {
			slopeGravity *= physics.MinecartWaterSlopeGravityFactor
		}
		if isSloped {
			speed -= slopeGravity
		}

		// Unpowered braking
		if unpoweredBraking {
			absSpeed := math.Abs(speed)
			if absSpeed < physics.MinecartUnpoweredBrakeThreshold {
				speed = 0
			} else {
				speed *= 0.5
			}
		}

		// Powered rail boost
		if rail.isPowered && rail.isEnergized {
			if speed > 0 {
				speed += physics.MinecartPoweredRailBoost
			} else if speed < 0 {
				speed -= physics.MinecartPoweredRailBoost
			} else {
				speed = physics.MinecartPoweredRailBoost
			}
		}

		// Cap speed
		maxSpeed := physics.MinecartMaxSpeed
		if inWater {
			maxSpeed = physics.MinecartWaterMaxSpeed
		}
		if speed > maxSpeed {
			speed = maxSpeed
		} else if speed < -maxSpeed {
			speed = -maxSpeed
		}

		// Project speed back onto rail direction
		newVelX = dirX * speed
		newVelZ = dirZ * speed

		// Compute position with passenger multiplier
		newX = currentPos.X + newVelX*physics.MinecartPassengerSpeedMultiplier
		newZ = currentPos.Z + newVelZ*physics.MinecartPassengerSpeedMultiplier

		// Apply drag AFTER position computation
		newVelX *= physics.MinecartRailDrag
		newVelZ *= physics.MinecartRailDrag
		newVelMultiplier = physics.MinecartRailDrag
		if inWater {
			newVelX *= physics.MinecartWaterDragMultiplier
			newVelZ *= physics.MinecartWaterDragMultiplier
		}

		// Compute Y from rail geometry
		newY = railYAtPosition(rail.shape, newX, newZ, railBlockX, rail.railBlockY, railBlockZ)
	}

	// Update shared velocity state
	pe.ridingVelX = newVelX
	pe.ridingVelZ = newVelZ
	pe.lastVelMultiplier = newVelMultiplier

	log.Printf("[handleRidingModeMinecart] rail=%v shape=%s isSlope=%v vel=(%.4f,%.4f) pos=(%.3f,%.3f,%.3f)",
		rail.found, rail.shape, rail.found && isSlope(rail.shape), newVelX, newVelZ, newX, newY, newZ)

	newPos := models.V3{X: newX, Y: newY, Z: newZ}
	pe.physicsState.SetPosition(newPos, yaw, pitch, rail.found)
	pe.movementPacketSender.setBotPosition(newPos, yaw, pitch)

	// Final NaN safety check
	sendYaw := yaw
	sendPitch := pitch
	if math.IsNaN(sendYaw) {
		sendYaw = 0
	}
	if math.IsNaN(sendPitch) {
		sendPitch = 0
	}

	if err := versionHandler.Play().Movement().SendMoveVehicle(
		pe.movementPacketSender.client.Conn(),
		newX, newY, newZ,
		sendYaw, sendPitch,
		rail.found,
	); err != nil {
		log.Printf("[handleRidingModeMinecart] SendMoveVehicle error: %v", err)
	}
}

// detectRailBelow checks for rail blocks beneath the minecart and returns rail info.
func (pe *PhysicsMovementExecutor) detectRailBelow(x, y, z float64) minecartRailInfo {
	if pe.world == nil || pe.shapeProvider == nil {
		return minecartRailInfo{}
	}
	bx := int(math.Floor(x))
	bz := int(math.Floor(z))
	for _, by := range []int{int(math.Floor(y)), int(math.Floor(y)) - 1} {
		stateID, loaded := pe.world.GetBlockStatus(bx, by, bz)
		if !loaded {
			continue
		}
		name := pe.shapeProvider.BlockName(stateID)
		fullName := pe.shapeProvider.FullBlockName(stateID)
		props := pe.shapeProvider.GetBlockProperties(stateID)

		isRail := false
		railType := ""
		switch name {
		case "minecraft:rail",
			"minecraft:powered_rail",
			"minecraft:detector_rail",
			"minecraft:activator_rail":
			isRail = true
			railType = name
		default:
			if name == "" && fullName != "" {
				for _, rt := range []string{"minecraft:rail", "minecraft:powered_rail", "minecraft:detector_rail", "minecraft:activator_rail"} {
					if strings.HasPrefix(fullName, rt) {
						isRail = true
						railType = rt
						break
					}
				}
			}
		}

		if isRail {
			isPowered := railType == "minecraft:powered_rail"
			return minecartRailInfo{
				found:       true,
				shape:       props["shape"],
				isPowered:   isPowered,
				isEnergized: isPowered && props["powered"] == "true",
				railBlockY:  by,
			}
		}
	}
	return minecartRailInfo{}
}

// railShapeDirection returns the horizontal direction (dx, dz) and vertical
// slope flag for a given rail shape.
func railShapeDirection(shape string) (dx, dz float64, isSlope bool) {
	switch shape {
	case "north_south":
		return 0, 1, false
	case "east_west":
		return 1, 0, false
	case "ascending_north":
		return 0, -1, true
	case "ascending_south":
		return 0, 1, true
	case "ascending_east":
		return 1, 0, true
	case "ascending_west":
		return -1, 0, true
	case "south_east":
		inv := 1.0 / math.Sqrt2
		return inv, inv, false
	case "south_west":
		inv := 1.0 / math.Sqrt2
		return -inv, inv, false
	case "north_east":
		inv := 1.0 / math.Sqrt2
		return inv, -inv, false
	case "north_west":
		inv := 1.0 / math.Sqrt2
		return -inv, -inv, false
	}
	return 0, 0, false
}

// railYAtPosition returns the exact Y the minecart should be for a given position.
func railYAtPosition(shape string, posX, posZ float64, railBlockX, railBlockY, railBlockZ int) float64 {
	centerX := float64(railBlockX) + 0.5
	centerZ := float64(railBlockZ) + 0.5
	base := float64(railBlockY) + physics.MinecartRailYOffset
	switch shape {
	case "ascending_north":
		t := (centerZ + 0.5 - posZ)
		return base + clamp01(t)
	case "ascending_south":
		t := (posZ - (centerZ - 0.5))
		return base + clamp01(t)
	case "ascending_east":
		t := (posX - (centerX - 0.5))
		return base + clamp01(t)
	case "ascending_west":
		t := (centerX + 0.5 - posX)
		return base + clamp01(t)
	}
	return base
}

// clamp01 clamps a value to [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// isSlope returns true if the rail shape involves ascending/descending.
func isSlope(shape string) bool {
	switch shape {
	case "ascending_north", "ascending_south", "ascending_east", "ascending_west":
		return true
	}
	return false
}
