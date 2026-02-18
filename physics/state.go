package physics

import (
	"log"
	"math"
	"os"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Inputs is an alias to models.Inputs for backward compatibility.
// Use models.Inputs for new code to avoid import cycles.
type Inputs = models.Inputs

// State tracks the physics state of a player entity.
// This includes position, velocity, rotation, and ground contact flags.
type state struct {
	// Position and velocity
	Pos models.V3 // Player position (feet level)
	Vel models.V3 // Player velocity (blocks per tick)

	// Rotation
	yaw   float64 // Horizontal look direction (degrees, 0=south, 90=west, 180=north, 270=east)
	pitch float64 // Vertical look direction (degrees, -90=up, 0=forward, 90=down)

	// State flags
	onGround   bool // True if player is standing on solid ground
	isSneaking bool // True if player is sneaking (affects hitbox and edge behavior)
	collision  struct {
		vertical   bool // True if vertical (Y) velocity was clamped by collision
		horizontal bool // True if horizontal (X/Z) velocity was clamped by collision
	}

	// Internal state
	tick     uint32 // Current tick number
	lastJump uint32 // Tick when player last jumped (for cooldown)

	// Entity dimensions (constant for players)
	width     float64 // Collision box width (X/Z)
	height    float64 // Collision box height (Y)
	eyeHeight float64 // Eye level offset from feet

	// Block shape provider for collision detection
	shapeProvider BlockShapeProvider
}

// NewState creates a new physics state with default player dimensions.
func NewState(shapeProvider BlockShapeProvider) models.PhysicsState {
	return &state{
		width:         PlayerWidth,
		height:        PlayerHeight,
		eyeHeight:     PlayerEyeHeight,
		shapeProvider: shapeProvider,
	}
}

// SetPosition updates the player's position and rotation (for server corrections).
// This resets velocity and collision flags, as the server has teleported the player.
func (s *state) SetPosition(pos models.V3, yaw, pitch float64, onGround bool) {
	// Calculate delta for debugging (server corrections should be rare)
	deltaX := pos.X - s.Pos.X
	deltaY := pos.Y - s.Pos.Y
	deltaZ := pos.Z - s.Pos.Z

	if deltaX != 0 || deltaY != 0 || deltaZ != 0 {
		log.Printf("[Physics] Server position correction: Δ(%.3f, %.3f, %.3f) velY=%.3f\n",
			deltaX, deltaY, deltaZ, s.Vel.Y)
	}

	s.Pos = pos
	s.yaw = yaw
	s.pitch = pitch
	s.Vel = models.V3{X: 0, Y: 0, Z: 0} // Reset velocity
	s.onGround = onGround
	s.collision.vertical = false
	s.collision.horizontal = false
}

// GetPosition returns the current position and rotation.
func (s *state) GetPosition() (pos models.V3, yaw, pitch float64, onGround bool) {
	return s.Pos, s.yaw, s.pitch, s.onGround
}

// GetVelocity returns the current velocity.
func (s *state) GetVelocity() models.V3 {
	return s.Vel
}

// Position returns the current position.
func (s *state) Position() models.V3 {
	return s.Pos
}

// Velocity returns the current velocity.
func (s *state) Velocity() models.V3 {
	return s.Vel
}

// Yaw returns the current yaw.
func (s *state) Yaw() float64 {
	return s.yaw
}

// Pitch returns the current pitch.
func (s *state) Pitch() float64 {
	return s.pitch
}

// OnGround reports whether the player is on ground.
func (s *state) OnGround() bool {
	return s.onGround
}

// IsSneaking reports whether the player is sneaking.
func (s *state) IsSneaking() bool {
	return s.isSneaking
}

// GetDimensions returns the collision dimensions for the player.
func (s *state) GetDimensions() (width, height, eyeHeight float64) {
	return s.width, s.height, s.eyeHeight
}

// SetPositionSimple updates the position without changing rotation or ground status.
func (s *state) SetPositionSimple(pos models.V3) {
	s.Pos = pos
}

// SetYaw updates the yaw without changing position or pitch.
func (s *state) SetYaw(yaw float64) {
	s.yaw = yaw
}

// SetPitch updates the pitch without changing position or yaw.
func (s *state) SetPitch(pitch float64) {
	s.pitch = pitch
}

// SetVelocity updates the velocity.
func (s *state) SetVelocity(vel models.V3) {
	s.Vel = vel
}

// SetOnGround updates the grounded flag.
func (s *state) SetOnGround(onGround bool) {
	s.onGround = onGround
}

// SetSneaking updates the sneaking flag.
func (s *state) SetSneaking(sneaking bool) {
	s.isSneaking = sneaking
}

// GetAABB returns the player's current axis-aligned bounding box.
// Height changes based on sneaking state: 1.8 blocks normally, 1.5 blocks when sneaking.
func (s *state) GetAABB() AABB {
	height := s.height
	if s.isSneaking {
		height = PlayerHeightSneaking // 1.5 blocks when sneaking
	}

	return AABB{
		X: MinMax{Min: s.Pos.X - s.width/2, Max: s.Pos.X + s.width/2},
		Y: MinMax{Min: s.Pos.Y, Max: s.Pos.Y + height},
		Z: MinMax{Min: s.Pos.Z - s.width/2, Max: s.Pos.Z + s.width/2},
	}
}

// Tick advances the physics simulation by one tick (50ms).
// This applies inputs, updates velocity (gravity, drag, etc.), and moves the player
// with collision detection and resolution.
func (s *state) Tick(input Inputs, w World) error {
	s.tick++

	// Update sneaking state from inputs
	s.isSneaking = input.Sneak

	// Calculate ground-based inertia and acceleration
	inertiaFactor := Inertia
	accelFactor := Acceleration

	// Check block below player for slipperiness (ice, slime, etc.)
	if s.onGround {
		blockBelow, _ := w.GetBlockStatus(
			int(math.Floor(s.Pos.X)),
			int(math.Floor(s.Pos.Y))-1,
			int(math.Floor(s.Pos.Z)),
		)

		// For now, use default slipperiness for all blocks
		// TODO: Query block-specific slipperiness from shape provider
		if !s.shapeProvider.IsPassable(blockBelow) {
			inertiaFactor *= Slipperiness
			accelFactor = 0.1 * (0.1627714 / (inertiaFactor * inertiaFactor * inertiaFactor))
		}
	}

	// Update velocity based on inputs
	s.tickVelocity(input, inertiaFactor, accelFactor, w)

	// Update position with collision detection
	s.tickPosition(w)

	// Check if player is on a ladder/vine and should climb
	blockAtPlayer, _ := w.GetBlockStatus(
		int(math.Floor(s.Pos.X)),
		int(math.Floor(s.Pos.Y)),
		int(math.Floor(s.Pos.Z)),
	)
	isClimbable := s.shapeProvider.IsClimbable(blockAtPlayer)

	if isClimbable && input.ClimbDirection != 0 {
		// Apply vertical velocity based on climb direction
		// +1.0 = climb up, -1.0 = descend, 0.0 = don't climb (let gravity work)
		s.Vel.Y = LadderClimbSpeed * input.ClimbDirection
	}

	// Apply gravity
	s.Vel.Y -= Gravity

	// Apply drag (air resistance)
	s.Vel.Y *= Drag
	s.Vel.X *= inertiaFactor
	s.Vel.Z *= inertiaFactor

	return nil
}

// tickVelocity updates velocity based on player inputs.
func (s *state) tickVelocity(input Inputs, inertia, acceleration float64, w World) {
	// Deadzone: Reset very small velocities to zero (prevents floating point drift)
	if math.Abs(s.Vel.X) < ResetVelocity {
		s.Vel.X = 0
	}
	if math.Abs(s.Vel.Y) < ResetVelocity {
		s.Vel.Y = 0
	}
	if math.Abs(s.Vel.Z) < ResetVelocity {
		s.Vel.Z = 0
	}

	// Apply look inputs (yaw/pitch with rate limiting)
	s.applyLookInputs(input)

	// Apply movement inputs (throttle, jump)
	s.applyMovementInputs(input, acceleration)

	// Check if player is on a ladder (limits velocity)
	blockAtPlayer, _ := w.GetBlockStatus(
		int(math.Floor(s.Pos.X)),
		int(math.Floor(s.Pos.Y)),
		int(math.Floor(s.Pos.Z)),
	)
	if s.shapeProvider.IsClimbable(blockAtPlayer) {
		// Clamp all velocity components to ladder max speed
		s.Vel.X = clamp(s.Vel.X, -LadderMaxSpeed, LadderMaxSpeed)
		if s.isSneaking {
			// When sneaking on a ladder, prevent descent (vanilla Minecraft behavior)
			s.Vel.Y = clamp(s.Vel.Y, 0, LadderMaxSpeed)
		} else {
			s.Vel.Y = clamp(s.Vel.Y, -LadderMaxSpeed, LadderMaxSpeed)
		}
		s.Vel.Z = clamp(s.Vel.Z, -LadderMaxSpeed, LadderMaxSpeed)
	}
}

// applyLookInputs updates yaw and pitch with rate limiting (anti-cheat compliance).
func (s *state) applyLookInputs(input Inputs) {
	// Update yaw with rate limiting
	if !math.IsNaN(input.Yaw) {
		deltaYaw := NormalizeAngle(input.Yaw - s.yaw)
		deltaYaw = clamp(deltaYaw, -MaxYawChange, MaxYawChange)
		s.yaw += deltaYaw
	}

	// Update pitch with rate limiting
	deltaPitch := input.Pitch - s.pitch
	deltaPitch = clamp(deltaPitch, -MaxPitchChange, MaxPitchChange)
	s.pitch += deltaPitch
}

// applyMovementInputs updates velocity based on throttle and jump inputs.
func (s *state) applyMovementInputs(input Inputs, acceleration float64) {
	// Handle jump (with cooldown)
	if input.Jump && s.tick >= s.lastJump+MinJumpTicks && s.onGround {
		s.lastJump = s.tick
		s.Vel.Y = JumpVelocity
	}

	// Calculate throttle magnitude
	speed := math.Sqrt(input.ThrottleX*input.ThrottleX + input.ThrottleZ*input.ThrottleZ)
	if speed < 0.01 {
		return // No movement input
	}

	// Normalize and scale by acceleration
	speed = acceleration / math.Max(speed, 1.0)
	throttleX := input.ThrottleX * speed
	throttleZ := input.ThrottleZ * speed

	// Apply sprint/sneak multipliers
	if input.Sprint {
		throttleX *= SprintMultiplier
		throttleZ *= SprintMultiplier
	} else if input.Sneak {
		throttleX *= SneakMultiplier
		throttleZ *= SneakMultiplier
	}

	// Throttle is in world-absolute coordinates (direction vector)
	// NOT player-relative WASD controls
	// Add directly to velocity (acceleration)
	s.Vel.X += throttleX
	s.Vel.Z += throttleZ
}

// tickPosition updates position with collision detection and step-up mechanics.
func (s *state) tickPosition(w World) {
	// Get player bounding box
	playerBB := s.GetAABB()

	// Compute collision with YXZ order (Y first, then X, then Z)
	newPlayerBB, newVel := s.computeCollisionYXZ(playerBB, s.Vel, w)

	// Check if step-up is possible
	// Step-up is attempted if:
	// 1. Player is on ground OR
	// 2. Vertical velocity was clamped downward (hit ground this tick)
	if s.onGround || (s.Vel.Y != newVel.Y && s.Vel.Y < 0) {
		// Try step-up: can player climb a small obstacle?
		// IMPORTANT: Use newPlayerBB (post-collision position), not playerBB (pre-collision)
		// This ensures step-up starts from where the agent actually is after collision
		stepUpBB, stepUpVel := s.tryStepUp(newPlayerBB, s.Vel, w)

		// Compare horizontal movement distance
		// Use step-up if it moved further horizontally
		oldDist := newVel.X*newVel.X + newVel.Z*newVel.Z
		newDist := stepUpVel.X*stepUpVel.X + stepUpVel.Z*stepUpVel.Z

		if os.Getenv("DEBUG_STEP_UP") != "" {
			log.Printf("[StepUp] Pos=(%.2f,%.2f,%.2f) Vel=(%.3f,%.3f,%.3f) OldVel=(%.3f,%.3f,%.3f) NewVel=(%.3f,%.3f,%.3f)\n",
				s.Pos.X, s.Pos.Y, s.Pos.Z,
				s.Vel.X, s.Vel.Y, s.Vel.Z,
				newVel.X, newVel.Y, newVel.Z,
				stepUpVel.X, stepUpVel.Y, stepUpVel.Z)
			log.Printf("[StepUp] oldDist=%.4f newDist=%.4f stepUpVel.Y=%.4f threshold=%.4f\n",
				oldDist, newDist, stepUpVel.Y, -StepHeight+0.000002)
		}

		// Use step-up if:
		// 1. It moved further horizontally, AND
		// 2. Final Y offset is near zero (actually on ground after step)
		if newDist > oldDist && stepUpVel.Y > -StepHeight+0.000002 {
			if os.Getenv("DEBUG_STEP_UP") != "" {
				log.Printf("[StepUp] USING STEP-UP\n")
			}
			newPlayerBB = stepUpBB
			newVel = stepUpVel
		} else if os.Getenv("DEBUG_STEP_UP") != "" {
			log.Printf("[StepUp] NOT using step-up (failed conditions)\n")
		}
	}

	// Update collision flags BEFORE edge prevention (needed to determine onGround status)
	s.collision.horizontal = newVel.X != s.Vel.X || newVel.Z != s.Vel.Z
	s.collision.vertical = newVel.Y != s.Vel.Y
	s.onGround = s.collision.vertical && s.Vel.Y < 0

	// Edge prevention when sneaking (MUST happen BEFORE position update)
	// When sneaking on ground, prevent movement that would cause walking off block edges
	if s.isSneaking && s.onGround {
		// Calculate new position from the bounding box (collision already applied)
		newPosX := newPlayerBB.X.Min + s.width/2
		newPosY := newPlayerBB.Y.Min
		newPosZ := newPlayerBB.Z.Min + s.width/2

		// DEBUG
		if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
			log.Printf("[EdgePrev] Sneak=%v OnGround=%v NewPos=(%.3f,%.3f,%.3f)\n",
				s.isSneaking, s.onGround, newPosX, newPosY, newPosZ)
		}

		// Check all four corners of the player's hitbox at the new position
		// This matches vanilla Minecraft behavior
		halfWidth := s.width / 2
		corners := []models.V3{
			{X: newPosX + halfWidth, Y: newPosY, Z: newPosZ + halfWidth}, // +X +Z
			{X: newPosX + halfWidth, Y: newPosY, Z: newPosZ - halfWidth}, // +X -Z
			{X: newPosX - halfWidth, Y: newPosY, Z: newPosZ + halfWidth}, // -X +Z
			{X: newPosX - halfWidth, Y: newPosY, Z: newPosZ - halfWidth}, // -X -Z
		}

		// // If any corner would be over an edge (no ground support), prevent that movement
		// hasGroundSupport := true
		// for i, corner := range corners {
		// 	support := s.hasGroundSupportAt(corner, w)
		// 	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		// 		log.Printf("[EdgePrev]   Corner %d (%.3f,%.3f,%.3f) support=%v\n",
		// 			i, corner.X, corner.Y, corner.Z, support)
		// 	}
		// 	if !support {
		// 		hasGroundSupport = false
		// 		break
		// 	}
		// }

		// At least one corner must not be over an edge (no ground support), otherwise prevent that movement
		hasGroundSupport := false
		for i, corner := range corners {
			support := s.hasGroundSupportAt(corner, w)
			if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
				log.Printf("[EdgePrev]   Corner %d (%.3f,%.3f,%.3f) support=%v\n",
					i, corner.X, corner.Y, corner.Z, support)
			}
			if support {
				hasGroundSupport = true
				break
			}
		}

		// If no ground support at new position, revert to old position and stop movement
		if !hasGroundSupport {
			if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
				log.Printf("[EdgePrev] PREVENTING MOVEMENT - no ground support\n")
			}
			newPlayerBB = playerBB // Revert to original position before movement
			newVel.X = 0           // Zero horizontal velocity
			newVel.Z = 0
		}
	}

	// Extract position from bounding box (center of X/Z, min of Y)
	// This MUST happen AFTER edge prevention to respect position reverts
	s.Pos.X = newPlayerBB.X.Min + s.width/2
	s.Pos.Y = newPlayerBB.Y.Min
	s.Pos.Z = newPlayerBB.Z.Min + s.width/2

	// Update velocity
	s.Vel = newVel
}

// tryStepUp attempts to step up a small obstacle (max StepHeight).
// Returns the resulting bounding box and velocity if step-up succeeds.
func (s *state) tryStepUp(playerBB AABB, vel models.V3, w World) (AABB, models.V3) {
	// Query collision boxes in the step-up range
	queryBB := playerBB.Offset(vel.X, StepHeight, vel.Z)
	surroundings := s.getSurroundingBoxes(queryBB, w)

	if os.Getenv("DEBUG_STEP_UP") != "" && len(surroundings) > 0 {
		log.Printf("[tryStepUp] Found %d collision boxes in query range\n", len(surroundings))
		for i, box := range surroundings {
			log.Printf("  Box %d: X[%.2f-%.2f] Y[%.2f-%.2f] Z[%.2f-%.2f] BlockID=%d\n",
				i, box.X.Min, box.X.Max, box.Y.Min, box.Y.Max, box.Z.Min, box.Z.Max, box.BlockID)
		}
	}

	outVel := vel
	outVel.Y = StepHeight

	// Step 1: Move up to step height
	for _, box := range surroundings {
		outVel.Y = box.YOffset(playerBB, outVel.Y)
	}
	playerBB = playerBB.Offset(0, outVel.Y, 0)

	// Step 2: Move horizontally (X)
	for _, box := range surroundings {
		outVel.X = box.XOffset(playerBB, outVel.X)
	}
	playerBB = playerBB.Offset(outVel.X, 0, 0)

	// Step 3: Move horizontally (Z)
	for _, box := range surroundings {
		outVel.Z = box.ZOffset(playerBB, outVel.Z)
	}
	playerBB = playerBB.Offset(0, 0, outVel.Z)

	// Step 4: Move back down to ground
	outVel.Y = -StepHeight // Try to descend back to ground
	for _, box := range surroundings {
		outVel.Y = box.YOffset(playerBB, outVel.Y)
	}
	playerBB = playerBB.Offset(0, outVel.Y, 0)

	return playerBB, outVel
}

// computeCollisionYXZ performs collision detection and resolution in YXZ order.
// Returns the resulting bounding box and clamped velocity.
func (s *state) computeCollisionYXZ(playerBB AABB, vel models.V3, w World) (AABB, models.V3) {
	// Query collision boxes in the movement range
	queryBB := playerBB.Offset(vel.X, vel.Y, vel.Z)
	surroundings := s.getSurroundingBoxes(queryBB, w)

	outVel := vel

	// Y-axis collision (vertical)
	for _, box := range surroundings {
		outVel.Y = box.YOffset(playerBB, outVel.Y)
	}
	playerBB = playerBB.Offset(0, outVel.Y, 0)

	// X-axis collision (horizontal)
	for _, box := range surroundings {
		outVel.X = box.XOffset(playerBB, outVel.X)
	}
	playerBB = playerBB.Offset(outVel.X, 0, 0)

	// Z-axis collision (horizontal)
	for _, box := range surroundings {
		outVel.Z = box.ZOffset(playerBB, outVel.Z)
	}
	playerBB = playerBB.Offset(0, 0, outVel.Z)

	return playerBB, outVel
}

// getSurroundingBoxes queries all collision boxes in the given AABB range.
// Returns a slice of AABBs representing solid blocks that could collide with the player.
func (s *state) getSurroundingBoxes(queryBB AABB, w World) []AABB {
	// Expand query range by 1 block in each direction (safety margin)
	minX := int(math.Floor(queryBB.X.Min))
	maxX := int(math.Floor(queryBB.X.Max)) + 1
	minY := int(math.Floor(queryBB.Y.Min)) - 1
	maxY := int(math.Floor(queryBB.Y.Max)) + 1
	minZ := int(math.Floor(queryBB.Z.Min))
	maxZ := int(math.Floor(queryBB.Z.Max)) + 1

	// Pre-allocate with estimated capacity (reduce allocations)
	dx, dy, dz := maxX-minX, maxY-minY, maxZ-minZ
	capacity := dx * dy * dz * 2 // *2 for blocks with multiple collision boxes
	boxes := make([]AABB, 0, capacity)

	// Iterate through all blocks in range
	for y := minY; y < maxY; y++ {
		for z := minZ; z < maxZ; z++ {
			for x := minX; x < maxX; x++ {
				blockStateID, _ := w.GetBlockStatus(x, y, z)

				// Skip passable blocks (air, water, etc.)
				if s.shapeProvider.IsPassable(blockStateID) {
					continue
				}

				// Get collision boxes for this block
				blockBoxes := s.shapeProvider.GetCollisionBoxes(blockStateID, x, y, z)
				boxes = append(boxes, blockBoxes...)
			}
		}
	}

	return boxes
}

// AtLookTarget returns true if the player's current look direction matches
// the target yaw and pitch within a small tolerance.
func (s *state) AtLookTarget(targetYaw, targetPitch float64) bool {
	deltaYaw := math.Abs(NormalizeAngle(targetYaw - s.yaw))
	deltaPitch := math.Abs(targetPitch - s.pitch)
	return deltaYaw <= 0.8 && deltaPitch <= 1.1
}

// PredictMovement simulates N ticks ahead without modifying the current state.
// Returns a slice of predicted states, one for each tick simulated.
// This is useful for path validation and trajectory prediction.
func (s *state) PredictMovement(inputs []Inputs, maxTicks int, w World) []models.PhysicsState {
	// Create a copy of current state for simulation
	predicted := *s

	// Limit prediction to maxTicks or length of inputs
	ticks := maxTicks
	if len(inputs) < ticks {
		ticks = len(inputs)
	}

	// Pre-allocate result slice
	results := make([]models.PhysicsState, 0, ticks)

	// Simulate each tick
	for i := 0; i < ticks; i++ {
		input := inputs[i]
		predicted.Tick(input, w)
		snapshot := predicted
		results = append(results, &snapshot)
	}

	return results
}

// PredictPosition performs a fast position-only prediction without full physics.
// This simulates free fall or ballistic motion for 'ticks' iterations.
// Useful for quick fall distance calculations. This is an approximate prediction
// that doesn't account for full collision physics.
func (s *state) PredictPosition(vel models.V3, ticks int, w World) models.V3 {
	pos := s.Pos
	currentVel := vel

	for i := 0; i < ticks; i++ {
		// Apply gravity and drag first
		currentVel.Y -= Gravity
		currentVel.Y *= Drag
		currentVel.X *= Inertia
		currentVel.Z *= Inertia

		// Move
		pos.X += currentVel.X
		pos.Y += currentVel.Y
		pos.Z += currentVel.Z

		// Simple ground check: if falling and close to a solid block, stop
		if currentVel.Y < 0 {
			// Check the block at the player's feet level
			feetBlockY := int(math.Floor(pos.Y))
			blockBelow, _ := w.GetBlockStatus(
				int(math.Floor(pos.X)),
				feetBlockY,
				int(math.Floor(pos.Z)),
			)

			// If there's a solid block at feet level, snap to top of that block
			if !s.shapeProvider.IsPassable(blockBelow) {
				pos.Y = float64(feetBlockY + 1)
				currentVel.Y = 0
				break
			}
		}
	}

	return pos
}

// WillCollide performs a quick collision check to see if moving to targetPos would collide.
// This is a simplified check that doesn't account for full physics simulation.
// Returns true if collision is detected, false otherwise.
func (s *state) WillCollide(targetPos models.V3, w World) bool {
	// Create AABB at target position
	height := s.height
	if s.isSneaking {
		height = PlayerHeightSneaking
	}

	targetBB := AABB{
		X: MinMax{Min: targetPos.X - s.width/2, Max: targetPos.X + s.width/2},
		Y: MinMax{Min: targetPos.Y, Max: targetPos.Y + height},
		Z: MinMax{Min: targetPos.Z - s.width/2, Max: targetPos.Z + s.width/2},
	}

	// Get collision boxes at target
	surroundings := s.getSurroundingBoxes(targetBB, w)

	// Check if any collision boxes intersect with target AABB
	for _, box := range surroundings {
		if targetBB.Intersects(box) {
			return true
		}
	}

	return false
}

// HasGroundSupportAt exposes ground support checks for callers needing edge detection.
func (s *state) HasGroundSupportAt(pos models.V3, w World) bool {
	return s.hasGroundSupportAt(pos, w)
}

// Helper functions

// GetSurroundingBoxes exposes collision box lookup for callers that need it.
func (s *state) GetSurroundingBoxes(queryBB AABB, w World) []AABB {
	return s.getSurroundingBoxes(queryBB, w)
}

// hasGroundSupportAt checks if there's a solid block below the given position.
// This is used for edge prevention when sneaking - prevents walking off edges.
// In Minecraft, sneaking prevents walking off edges by checking if corners would
// extend beyond supported blocks. A corner is supported if:
// 1. It's safely within a block (not too close to edges), OR
// 2. It's near an edge but there's an adjacent block providing support
func (s *state) hasGroundSupportAt(pos models.V3, w World) bool {
	// Check for solid block directly below
	checkY := int(math.Floor(pos.Y - 0.05))
	checkX := int(math.Floor(pos.X))
	checkZ := int(math.Floor(pos.Z))

	blockID, _ := w.GetBlockStatus(checkX, checkY, checkZ)
	isPassable := s.shapeProvider.IsPassable(blockID)

	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		log.Printf("[EdgePrev]     Checking block at (%d,%d,%d) = %d, passable=%v\n",
			checkX, checkY, checkZ, blockID, isPassable)
	}

	// If no solid block, no support
	if isPassable {
		return false
	}

	// Check if position is close to block edges
	// When sneaking, corners close to edges need adjacent blocks for support
	xOffset := pos.X - float64(checkX)
	zOffset := pos.Z - float64(checkZ)

	const edgeMargin = 0.4 // If within 0.4 of an edge, check for adjacent support

	// Determine if we're close to far edges (approaching 1.0)
	nearXEdge := xOffset > (1.0 - edgeMargin) // > 0.6, close to X edge at 1.0
	nearZEdge := zOffset > (1.0 - edgeMargin) // > 0.6, close to Z edge at 1.0

	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		log.Printf("[EdgePrev]     Position offsets: X=%.3f, Z=%.3f (edge margin=%.1f)\n",
			xOffset, zOffset, edgeMargin)
		log.Printf("[EdgePrev]     Near edges: X=%v, Z=%v\n", nearXEdge, nearZEdge)
	}

	// If close to X edge, check for adjacent block in +X direction
	if nearXEdge {
		adjacentXBlock, _ := w.GetBlockStatus(checkX+1, checkY, checkZ)
		hasXSupport := !s.shapeProvider.IsPassable(adjacentXBlock)
		if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
			log.Printf("[EdgePrev]     Near X edge, checking block at (%d,%d,%d) = %d, supported=%v\n",
				checkX+1, checkY, checkZ, adjacentXBlock, hasXSupport)
		}
		if !hasXSupport {
			return false // No support in X direction
		}
	}

	// If close to Z edge, check for adjacent block in +Z direction
	if nearZEdge {
		adjacentZBlock, _ := w.GetBlockStatus(checkX, checkY, checkZ+1)
		hasZSupport := !s.shapeProvider.IsPassable(adjacentZBlock)
		if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
			log.Printf("[EdgePrev]     Near Z edge, checking block at (%d,%d,%d) = %d, supported=%v\n",
				checkX, checkY, checkZ+1, adjacentZBlock, hasZSupport)
		}
		if !hasZSupport {
			return false // No support in Z direction
		}
	}

	// Position is either safely within block or has adjacent support
	return true
}

// clamp restricts a value to the range [min, max].
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
