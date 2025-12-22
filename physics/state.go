package physics

import (
	"fmt"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// World represents a provider of block information for physics simulation.
// Implementations should provide efficient block lookups for collision detection.
type World interface {
	// GetBlockStatus returns the block state ID at the given integer coordinates
	GetBlockStatus(x, y, z int) int32
}

// BlockShapeProvider provides block shape data for collision detection.
// This interface allows physics to query collision boxes without hardcoding blocks.
type BlockShapeProvider interface {
	// GetCollisionBoxes returns all collision boxes for a block state at the given position
	// Returns empty slice for passable blocks (air, water, etc.)
	GetCollisionBoxes(blockStateID int32, x, y, z int) []AABB

	// IsPassable returns true if entities can move through this block
	IsPassable(blockStateID int32) bool

	// IsClimbable returns true if this block can be climbed (ladder, vine)
	IsClimbable(blockStateID int32) bool
}

// Inputs is an alias to models.Inputs for backward compatibility.
// Use models.Inputs for new code to avoid import cycles.
type Inputs = models.Inputs

// State tracks the physics state of a player entity.
// This includes position, velocity, rotation, and ground contact flags.
type State struct {
	// Position and velocity
	Pos V3 // Player position (feet level)
	Vel V3 // Player velocity (blocks per tick)

	// Rotation
	Yaw   float64 // Horizontal look direction (degrees, 0=south, 90=west, 180=north, 270=east)
	Pitch float64 // Vertical look direction (degrees, -90=up, 0=forward, 90=down)

	// State flags
	onGround  bool // True if player is standing on solid ground
	collision struct {
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
func NewState(shapeProvider BlockShapeProvider) *State {
	return &State{
		width:         PlayerWidth,
		height:        PlayerHeight,
		eyeHeight:     PlayerEyeHeight,
		shapeProvider: shapeProvider,
	}
}

// SetPosition updates the player's position and rotation (for server corrections).
// This resets velocity and collision flags, as the server has teleported the player.
func (s *State) SetPosition(pos V3, yaw, pitch float64, onGround bool) {
	// Calculate delta for debugging (server corrections should be rare)
	deltaX := pos.X - s.Pos.X
	deltaY := pos.Y - s.Pos.Y
	deltaZ := pos.Z - s.Pos.Z

	if deltaX != 0 || deltaY != 0 || deltaZ != 0 {
		fmt.Printf("[Physics] Server position correction: Δ(%.3f, %.3f, %.3f) velY=%.3f\n",
			deltaX, deltaY, deltaZ, s.Vel.Y)
	}

	s.Pos = pos
	s.Yaw = yaw
	s.Pitch = pitch
	s.Vel = V3{X: 0, Y: 0, Z: 0} // Reset velocity
	s.onGround = onGround
	s.collision.vertical = false
	s.collision.horizontal = false
}

// GetPosition returns the current position and rotation.
func (s *State) GetPosition() (pos V3, yaw, pitch float64, onGround bool) {
	return s.Pos, s.Yaw, s.Pitch, s.onGround
}

// GetVelocity returns the current velocity.
func (s *State) GetVelocity() V3 {
	return s.Vel
}

// GetAABB returns the player's current axis-aligned bounding box.
func (s *State) GetAABB() AABB {
	return AABB{
		X: MinMax{Min: s.Pos.X - s.width/2, Max: s.Pos.X + s.width/2},
		Y: MinMax{Min: s.Pos.Y, Max: s.Pos.Y + s.height},
		Z: MinMax{Min: s.Pos.Z - s.width/2, Max: s.Pos.Z + s.width/2},
	}
}

// Tick advances the physics simulation by one tick (50ms).
// This applies inputs, updates velocity (gravity, drag, etc.), and moves the player
// with collision detection and resolution.
func (s *State) Tick(input Inputs, w World) error {
	s.tick++

	// Calculate ground-based inertia and acceleration
	inertiaFactor := Inertia
	accelFactor := Acceleration

	// Check block below player for slipperiness (ice, slime, etc.)
	if s.onGround {
		blockBelow := w.GetBlockStatus(
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

	// Check if player is on a ladder and colliding horizontally
	blockAtPlayer := w.GetBlockStatus(
		int(math.Floor(s.Pos.X)),
		int(math.Floor(s.Pos.Y)),
		int(math.Floor(s.Pos.Z)),
	)
	if s.shapeProvider.IsClimbable(blockAtPlayer) && s.collision.horizontal {
		// When on ladder and moving into it, apply upward velocity
		s.Vel.Y = LadderClimbSpeed
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
func (s *State) tickVelocity(input Inputs, inertia, acceleration float64, w World) {
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
	blockAtPlayer := w.GetBlockStatus(
		int(math.Floor(s.Pos.X)),
		int(math.Floor(s.Pos.Y)),
		int(math.Floor(s.Pos.Z)),
	)
	if s.shapeProvider.IsClimbable(blockAtPlayer) {
		// Clamp all velocity components to ladder max speed
		s.Vel.X = clamp(s.Vel.X, -LadderMaxSpeed, LadderMaxSpeed)
		s.Vel.Y = clamp(s.Vel.Y, -LadderMaxSpeed, LadderMaxSpeed)
		s.Vel.Z = clamp(s.Vel.Z, -LadderMaxSpeed, LadderMaxSpeed)
	}
}

// applyLookInputs updates yaw and pitch with rate limiting (anti-cheat compliance).
func (s *State) applyLookInputs(input Inputs) {
	// Update yaw with rate limiting
	if !math.IsNaN(input.Yaw) {
		deltaYaw := normalizeYawDelta(input.Yaw - s.Yaw)
		deltaYaw = clamp(deltaYaw, -MaxYawChange, MaxYawChange)
		s.Yaw += deltaYaw
	}

	// Update pitch with rate limiting
	deltaPitch := input.Pitch - s.Pitch
	deltaPitch = clamp(deltaPitch, -MaxPitchChange, MaxPitchChange)
	s.Pitch += deltaPitch
}

// applyMovementInputs updates velocity based on throttle and jump inputs.
func (s *State) applyMovementInputs(input Inputs, acceleration float64) {
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

	// Add to velocity (acceleration)
	s.Vel.X += throttleX
	s.Vel.Z += throttleZ
}

// tickPosition updates position with collision detection and step-up mechanics.
func (s *State) tickPosition(w World) {
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
		stepUpBB, stepUpVel := s.tryStepUp(playerBB, s.Vel, w)

		// Compare horizontal movement distance
		// Use step-up if it moved further horizontally
		oldDist := newVel.X*newVel.X + newVel.Z*newVel.Z
		newDist := stepUpVel.X*stepUpVel.X + stepUpVel.Z*stepUpVel.Z

		// Use step-up if:
		// 1. It moved further horizontally, AND
		// 2. Final Y offset is near zero (actually on ground after step)
		if newDist > oldDist && stepUpVel.Y > -StepHeight+0.000002 {
			newPlayerBB = stepUpBB
			newVel = stepUpVel
		}
	}

	// Extract position from bounding box (center of X/Z, min of Y)
	s.Pos.X = newPlayerBB.X.Min + s.width/2
	s.Pos.Y = newPlayerBB.Y.Min
	s.Pos.Z = newPlayerBB.Z.Min + s.width/2

	// Update collision flags
	s.collision.horizontal = newVel.X != s.Vel.X || newVel.Z != s.Vel.Z
	s.collision.vertical = newVel.Y != s.Vel.Y
	s.onGround = s.collision.vertical && s.Vel.Y < 0

	// Update velocity
	s.Vel = newVel
}

// tryStepUp attempts to step up a small obstacle (max StepHeight).
// Returns the resulting bounding box and velocity if step-up succeeds.
func (s *State) tryStepUp(playerBB AABB, vel V3, w World) (AABB, V3) {
	// Query collision boxes in the step-up range
	queryBB := playerBB.Offset(vel.X, StepHeight, vel.Z)
	surroundings := s.getSurroundingBoxes(queryBB, w)

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
func (s *State) computeCollisionYXZ(playerBB AABB, vel V3, w World) (AABB, V3) {
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
func (s *State) getSurroundingBoxes(queryBB AABB, w World) []AABB {
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
				blockStateID := w.GetBlockStatus(x, y, z)

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
func (s *State) AtLookTarget(targetYaw, targetPitch float64) bool {
	deltaYaw := math.Abs(normalizeYawDelta(targetYaw - s.Yaw))
	deltaPitch := math.Abs(targetPitch - s.Pitch)
	return deltaYaw <= 0.8 && deltaPitch <= 1.1
}

// Helper functions

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

// normalizeYawDelta normalizes a yaw angle delta to the range [-180, 180].
func normalizeYawDelta(delta float64) float64 {
	delta = math.Mod(delta, 360)
	if delta > 180 {
		delta -= 360
	} else if delta < -180 {
		delta += 360
	}
	return delta
}
