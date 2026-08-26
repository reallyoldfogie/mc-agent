package physics

import (
	"log"
	"math"
	"os"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Inputs is an alias to models.Inputs for backward compatibility.
// Use models.Inputs for new code to avoid import cycles.
type Inputs = models.Inputs

// State tracks the physics state of a player entity.
// This includes position, velocity, rotation, and ground contact flags.
type state struct {
	mu sync.RWMutex // Protects all mutable fields for concurrent access

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

	// Water/swimming state (updated at the start of each Tick)
	isSwimming bool // True if player's head is submerged in water
	isInWater  bool // True if player's feet or head are in water

	// Internal state
	tick         uint32  // Current tick number
	lastJump     uint32  // Tick when player last jumped (for cooldown)
	fallDistance float64 // Current fall distance accumulated this fall (resets on landing)

	// Entity dimensions (constant for players)
	width     float64 // Collision box width (X/Z)
	height    float64 // Collision box height (Y)
	eyeHeight float64 // Eye level offset from feet

	// Block shape provider for collision detection
	shapeProvider BlockShapeProvider

	// activeEffects holds the walking player's own status effects relevant
	// to physics (Slow Falling, Levitation), set externally once per tick
	// via SetActiveEffects before Tick() runs.
	activeEffects models.ActiveEffects
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
	s.mu.Lock()
	defer s.mu.Unlock()
	// SetPosition is a low-level setter used both for genuine server corrections
	// and for the client's own per-tick riding updates (via sendRidingMove), so it
	// must not label every change a "server correction" — that mislabels routine
	// client updates and floods the log. Genuine corrections are logged with
	// accurate context by the executor (HandleServerCorrection / SyncRidingPosition
	// / SyncMountedPosition). Keep only an opt-in, neutral trace here.
	if os.Getenv("DEBUG_PHYSICS_POSITION") != "" {
		deltaX := pos.X - s.Pos.X
		deltaY := pos.Y - s.Pos.Y
		deltaZ := pos.Z - s.Pos.Z
		if deltaX != 0 || deltaY != 0 || deltaZ != 0 {
			log.Printf("[PhysicsState] SetPosition Δ(%.3f, %.3f, %.3f) velY=%.3f\n",
				deltaX, deltaY, deltaZ, s.Vel.Y)
		}
	}

	s.Pos = pos
	s.yaw = yaw
	s.pitch = pitch
	s.Vel = models.V3{X: 0, Y: 0, Z: 0} // Reset velocity
	s.onGround = onGround
	s.fallDistance = 0.0 // Reset fall distance on server correction
	s.collision.vertical = false
	s.collision.horizontal = false
}

func (s *state) UpdatePosition(pos models.V3, yaw, pitch float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Pos = pos
	s.yaw = yaw
	s.pitch = pitch
}

// GetPosition returns the current position and rotation.
func (s *state) GetPosition() (pos models.V3, yaw, pitch float64, onGround bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Pos, s.yaw, s.pitch, s.onGround
}

// GetPosition returns the current position and rotation.
func (s *state) GetPositionSimple() (pos models.V3, onGround bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Pos, s.onGround
}

// GetVelocity returns the current velocity.
func (s *state) GetVelocity() models.V3 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Vel
}

// Position returns the current position.
func (s *state) Position() models.V3 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Pos
}

// Velocity returns the current velocity.
func (s *state) Velocity() models.V3 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Vel
}

// Yaw returns the current yaw.
func (s *state) Yaw() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.yaw
}

// Pitch returns the current pitch.
func (s *state) Pitch() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pitch
}

// OnGround reports whether the player is on ground.
func (s *state) OnGround() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.onGround
}

// IsSneaking reports whether the player is sneaking.
func (s *state) IsSneaking() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isSneaking
}

// IsSwimming reports whether the player's head is submerged in water.
func (s *state) IsSwimming() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isSwimming
}

// IsInWater reports whether any part of the player (feet or head) is in water.
func (s *state) IsInWater() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isInWater
}

// FallDistance reports the current accumulated fall distance in blocks.
func (s *state) FallDistance() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fallDistance
}

// GetDimensions returns the collision dimensions for the player.
func (s *state) GetDimensions() (width, height, eyeHeight float64) {
	return s.width, s.height, s.eyeHeight
}

// SetPositionSimple updates the position without changing rotation or ground status.
func (s *state) SetPositionSimple(pos models.V3) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Pos = pos
	// Reset fall distance when position is manually set
	s.fallDistance = 0.0
}

// SetYaw updates the yaw without changing position or pitch.
func (s *state) SetYaw(yaw float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.yaw = yaw
}

// SetPitch updates the pitch without changing position or yaw.
func (s *state) SetPitch(pitch float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pitch = pitch
}

// SetVelocity updates the velocity.
func (s *state) SetVelocity(vel models.V3) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Vel = vel
}

// SetOnGround updates the grounded flag.
func (s *state) SetOnGround(onGround bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onGround = onGround
	// Reset fall distance when landing
	if onGround {
		s.fallDistance = 0.0
	}
}

// SetSneaking updates the sneaking flag.
func (s *state) SetSneaking(sneaking bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isSneaking = sneaking
}

// SetFallDistance updates the fall distance.
func (s *state) SetFallDistance(distance float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallDistance = distance
}

// SetActiveEffects updates the status-effect state Tick() consults for
// Slow Falling/Levitation. Callers should call this once per tick, before
// Tick(), the same way other externally-sourced per-tick state is synced in.
func (s *state) SetActiveEffects(effects models.ActiveEffects) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeEffects = effects
}

// getAABBUnsafe computes the AABB without acquiring the mutex.
// Must only be called while the write lock is already held (e.g., from within Tick()).
func (s *state) getAABBUnsafe() AABB {
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

// GetAABB returns the player's current axis-aligned bounding box.
// Height changes based on sneaking state: 1.8 blocks normally, 1.5 blocks when sneaking.
func (s *state) GetAABB() AABB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getAABBUnsafe()
}

// Tick advances the physics simulation by one tick (50ms).
// This applies inputs, updates velocity (gravity, drag, etc.), and moves the player
// with collision detection and resolution.
func (s *state) Tick(input Inputs, w World) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("[PhysicsState][Tick] Tick %d: Pos=(%.2f, %.2f, %.2f) Vel=(%.2f, %.2f, %.2f) Yaw=%.2f Pitch=%.2f onGround=%t sneaking=%t swimming=%t fallDistance=%.2f\n",
		s.tick, s.Pos.X, s.Pos.Y, s.Pos.Z, s.Vel.X, s.Vel.Y, s.Vel.Z, s.yaw, s.pitch, s.onGround, s.isSneaking, s.isSwimming, s.fallDistance)

	s.tick++

	// Update sneaking state from inputs
	s.isSneaking = input.Sneak

	// Detect water state FIRST so swim-up/down inputs work in applyMovementInputs
	s.detectWaterState(w)

	// Detect cobweb overlap at the pre-move position (see
	// isOverlappingCobweb's doc comment for why pre-move rather than
	// replicating vanilla's exact one-tick-delayed field).
	inCobweb := s.isOverlappingCobweb(w)

	// Reset fall distance when in water (water negates all fall damage).
	// Slow Falling and Levitation do the same — Java calls onLanding() every
	// tick while either is active (LivingEntity.tickMovement), which is what
	// actually negates their fall damage, not a special-cased damage formula.
	// Cobwebs do too (Entity.slowMovement/makeStuckInBlock also calls
	// onLanding()/resetFallDistance()).
	if s.isInWater || s.activeEffects.HasSlowFalling || s.activeEffects.HasLevitation || inCobweb {
		s.fallDistance = 0.0
	}

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

	// Speed/Slowness scale movement_speed itself (Java's
	// getMovementSpeed(slipperiness) multiplies the attribute's value into
	// every branch above, on-ice or not), so apply the multiplier last,
	// after accelFactor's base value is set by whichever branch ran.
	accelFactor *= EffectSpeedMultiplier(s.activeEffects.HasSpeed, s.activeEffects.SpeedAmplifier, s.activeEffects.HasSlowness, s.activeEffects.SlownessAmplifier)

	// Update velocity based on inputs (swim-up/down uses s.isInWater)
	s.tickVelocity(input, inertiaFactor, accelFactor, w)

	// Cobweb slowdown scales *this tick's* attempted movement (Java
	// Entity.move()/travel: `movement = movement.multiply(movementMultiplier)`
	// before collision resolution, then velocity is reset to zero
	// afterward — see the zero-out below). Weaving halves the severity
	// rather than bypassing it.
	if inCobweb {
		mx, my, mz := CobwebSlowdownMultiplier(s.activeEffects.HasWeaving)
		s.Vel.X *= mx
		s.Vel.Y *= my
		s.Vel.Z *= mz
	}

	// Update position with collision detection
	s.tickPosition(w)

	// Zero velocity after moving while in a cobweb, matching Java's
	// `this.setVelocity(Vec3d.ZERO)`/`setDeltaMovement(Vec3.ZERO)` — momentum
	// does not carry into the next tick.
	if inCobweb {
		s.Vel = models.V3{}
	}

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

	// Apply gravity, drag, and water flow based on water state
	s.applyEnvironmentForces(inertiaFactor, w)

	log.Printf("[PhysicsState][Tick] After physics: Pos=(%.2f, %.2f, %.2f) Vel=(%.2f, %.2f, %.2f) onGround=%t inWater=%t swimming=%t collision=(h=%t v=%t)\n",
		s.Pos.X, s.Pos.Y, s.Pos.Z, s.Vel.X, s.Vel.Y, s.Vel.Z, s.onGround, s.isInWater, s.isSwimming, s.collision.horizontal, s.collision.vertical)

	return nil
}

// detectWaterState checks if the player is in water and updates isInWater/isSwimming.
// Must only be called while the write lock is held.
func (s *state) detectWaterState(w World) {
	feetBlockX := int(math.Floor(s.Pos.X))
	feetBlockY := int(math.Floor(s.Pos.Y))
	feetBlockZ := int(math.Floor(s.Pos.Z))
	feetBlockState, _ := w.GetBlockStatus(feetBlockX, feetBlockY, feetBlockZ)

	headBlockX := int(math.Floor(s.Pos.X))
	headBlockY := int(math.Floor(s.Pos.Y + s.eyeHeight))
	headBlockZ := int(math.Floor(s.Pos.Z))
	headBlockState, _ := w.GetBlockStatus(headBlockX, headBlockY, headBlockZ)

	areFeetInWater := s.shapeProvider.IsWater(feetBlockState)
	isHeadInWater := s.shapeProvider.IsWater(headBlockState)

	s.isInWater = areFeetInWater || isHeadInWater
	s.isSwimming = isHeadInWater

	if os.Getenv("DEBUG_WATER_FLOW") != "" {
		log.Printf("[Water] Feet: (%d,%d,%d) stateID=%d isWater=%v (head=%v feet=%v), Head: (%d,%d,%d)\n",
			feetBlockX, feetBlockY, feetBlockZ, feetBlockState, s.isInWater, isHeadInWater, areFeetInWater,
			headBlockX, headBlockY, headBlockZ)
	}
}

// applyEnvironmentForces applies gravity, drag, and water flow based on current water state.
// Must only be called while the write lock is held.
func (s *state) applyEnvironmentForces(inertiaFactor float64, w World) {
	if s.isInWater {
		// Apply reduced gravity in water
		s.Vel.Y -= Gravity * WaterGravityFactor

		// Apply water drag (higher than air drag). Horizontal (X/Z) drag is
		// the effect-dependent axis (Dolphin's Grace overrides it); vertical
		// (Y) always uses the fixed WaterDrag baseline, matching Java
		// travelInWater's vec3d.multiply(f, 0.8F, f).
		horizontalDrag := HorizontalWaterDrag(s.activeEffects.HasDolphinsGrace)
		s.Vel.X *= horizontalDrag
		s.Vel.Y *= WaterDrag
		s.Vel.Z *= horizontalDrag

		// Apply water flow current
		s.applyWaterFlow(w)
	} else {
		// Normal physics (air). Levitation replaces gravity entirely with an
		// eased approach toward a fixed upward target velocity; Slow Falling
		// instead caps how much gravity is subtracted. Both are walking-only
		// for now — travelMidAir is the source function for this branch, and
		// vanilla's fluid movement (the s.isInWater branch above) is a
		// separate Java method this hasn't been researched against yet.
		switch {
		case s.activeEffects.HasLevitation:
			s.Vel.Y = LevitationVerticalVelocity(s.Vel.Y, s.activeEffects.LevitationAmplifier)
		default:
			s.Vel.Y -= EffectiveGravity(Gravity, s.Vel.Y, s.activeEffects.HasSlowFalling)
		}
		s.Vel.Y *= Drag
		s.Vel.X *= inertiaFactor
		s.Vel.Z *= inertiaFactor
	}
}

// applyWaterFlow accumulates water flow from multiple sample points around the player
// and applies the resulting velocity. Must only be called while the write lock is held.
func (s *state) applyWaterFlow(w World) {
	var accumulatedFlowDir models.V3
	var bestFlowSpeed float64

	feetBlockX := int(math.Floor(s.Pos.X))
	feetBlockY := int(math.Floor(s.Pos.Y))
	feetBlockZ := int(math.Floor(s.Pos.Z))
	feetBlockState, _ := w.GetBlockStatus(feetBlockX, feetBlockY, feetBlockZ)
	areFeetInWater := s.shapeProvider.IsWater(feetBlockState)
	isHeadInWater := s.isSwimming

	checkPoint := func(x, y, z int) {
		blockState, loaded := w.GetBlockStatus(x, y, z)
		if !loaded || !s.shapeProvider.IsWater(blockState) {
			return
		}
		flowDir := s.shapeProvider.GetWaterFlowDirection(x, y, z, w)
		flowSpeed := s.shapeProvider.GetWaterFlowSpeed(blockState)
		if flowDir.X != 0 || flowDir.Z != 0 {
			accumulatedFlowDir.X += flowDir.X * flowSpeed
			accumulatedFlowDir.Z += flowDir.Z * flowSpeed
			if flowSpeed > bestFlowSpeed {
				bestFlowSpeed = flowSpeed
			}
		}
	}

	if areFeetInWater {
		checkPoint(feetBlockX, feetBlockY, feetBlockZ)
	}

	// Check mid-body (center and 4 corners)
	midBodyY := int(math.Floor(s.Pos.Y + s.height*0.6))
	midCenterX := int(math.Floor(s.Pos.X))
	midCenterZ := int(math.Floor(s.Pos.Z))

	checkPoint(midCenterX, midBodyY, midCenterZ)

	cornerOffset := int(math.Ceil(s.width / 2))
	if cornerOffset < 1 {
		cornerOffset = 1
	}
	checkPoint(midCenterX+cornerOffset, midBodyY, midCenterZ+cornerOffset)
	checkPoint(midCenterX+cornerOffset, midBodyY, midCenterZ-cornerOffset)
	checkPoint(midCenterX-cornerOffset, midBodyY, midCenterZ+cornerOffset)
	checkPoint(midCenterX-cornerOffset, midBodyY, midCenterZ-cornerOffset)

	if isHeadInWater {
		checkPoint(int(math.Floor(s.Pos.X)), int(math.Floor(s.Pos.Y+s.eyeHeight)), int(math.Floor(s.Pos.Z)))
	}

	// Normalize accumulated flow
	var flowDir models.V3
	flowSpeed := bestFlowSpeed
	magnitude := accumulatedFlowDir.DistanceTo(models.V3{})
	if magnitude > 0.01 {
		flowDir = models.V3{
			X: accumulatedFlowDir.X / magnitude,
			Y: 0,
			Z: accumulatedFlowDir.Z / magnitude,
		}
	}

	if os.Getenv("DEBUG_WATER_FLOW") != "" {
		log.Printf("[Water] Flow: speed=%.3f dir=(%.2f,%.2f,%.2f) accum=(%.2f,%.2f)\n",
			flowSpeed, flowDir.X, flowDir.Y, flowDir.Z, accumulatedFlowDir.X, accumulatedFlowDir.Z)
	}

	if flowSpeed > 0 && flowDir.DistanceTo(models.V3{}) > 0.01 {
		flowVel := WaterFlowSpeedBase * flowSpeed
		s.Vel.X += flowDir.X * flowVel
		s.Vel.Z += flowDir.Z * flowVel
		if os.Getenv("DEBUG_WATER_FLOW") != "" {
			log.Printf("[Water] Applied flow: flowVel=%.3f velAfter=(%.3f,%.3f,%.3f)\n",
				flowVel, s.Vel.X, s.Vel.Y, s.Vel.Z)
		}
	} else if os.Getenv("DEBUG_WATER_FLOW") != "" {
		log.Printf("[Water] Flow NOT applied: flowSpeed=%.3f flowDist=%.3f\n", flowSpeed, flowDir.DistanceTo(models.V3{}))
	}
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
	// Handle jump / swim-up / swim-down
	if s.isInWater {
		// In water: jump input swims up, sneak input swims down (no cooldown)
		if input.Jump {
			s.Vel.Y += SwimUpVelocity
		}
		if input.Sneak {
			s.Vel.Y -= SwimDownVelocity
		}
	} else if input.Jump && s.tick >= s.lastJump+MinJumpTicks && s.onGround {
		// On ground: normal jump with cooldown
		s.lastJump = s.tick
		s.Vel.Y = JumpVelocity
		if s.activeEffects.HasJumpBoost {
			s.Vel.Y += JumpBoostVelocityBonus(s.activeEffects.JumpBoostAmplifier)
		}
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
	oldVelX := s.Vel.X
	oldVelZ := s.Vel.Z
	s.Vel.X += throttleX
	s.Vel.Z += throttleZ

	if os.Getenv("DEBUG_MANUAL_MOVEMENT") != "" {
		log.Printf("[DEBUG_MANUAL] throttle=(%.4f,%.4f) after accel scaling, sneak=%v speed=%.4f -> adjusted throttle=(%.4f,%.4f) velBefore=(%.4f,%.4f) velAfter=(%.4f,%.4f)",
			input.ThrottleX, input.ThrottleZ, input.Sneak, speed, throttleX, throttleZ,
			oldVelX, oldVelZ, s.Vel.X, s.Vel.Z)
	}
}

// tickPosition updates position with collision detection and step-up mechanics.
// Must only be called while the write lock is already held
func (s *state) tickPosition(w World) {
	// Get player bounding box
	playerBB := s.getAABBUnsafe()

	// Edge prevention when sneaking happens BEFORE collision detection
	// Implements Minecraft's adjustMovementForSneaking() algorithm
	// This reduces movement in 0.05 block increments when at edges
	// Guard matches Java Entity.move() → PlayerEntity.adjustMovementForSneaking():
	//   !flying && !(movement.y > 0) && clipAtLedge() && isStandingOnSurface(stepHeight)
	// The isSneaking and velY checks are here; isStandingOnSurface is inside the method.
	adjustedVel := s.Vel
	if s.isSneaking && !(s.Vel.Y > 0) {
		adjustedVel.X, adjustedVel.Z = s.adjustMovementForSneaking(playerBB, s.Vel.X, s.Vel.Z, w)
	}

	// Compute collision with YXZ order (Y first, then X, then Z) using adjusted velocity.
	// Unlike the generic computeCollisionYXZ (also used by ResolveCollision for
	// ridden-vehicle physics), the walking player's own tick additionally
	// collides against "standable" entities — e.g. a happy ghast while
	// staying still — so the player can rest on top of one instead of only
	// being pushed away from it. See computeCollisionYXZWithStandableEntities.
	newPlayerBB, newVel := s.computeCollisionYXZWithStandableEntities(playerBB, adjustedVel, w)

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

	// Check for entity collisions and apply separation/velocity from entity contact
	// Entity collisions happen after block collisions
	s.handleEntityCollisions(newPlayerBB, &newVel, w)

	// Update collision flags BEFORE edge prevention (needed to determine onGround status)
	s.collision.horizontal = newVel.X != s.Vel.X || newVel.Z != s.Vel.Z
	s.collision.vertical = newVel.Y != s.Vel.Y

	// Bot is on ground if: (1) it hit the ground via collision, OR (2) it's standing on a block with non-positive vertical velocity
	hasVerticalCollision := s.collision.vertical && s.Vel.Y < 0
	hasGroundBelow := s.hasGroundSupportAt(newPlayerBB.Center(), w)
	s.onGround = hasVerticalCollision || (hasGroundBelow && newVel.Y <= 0)

	// Update fall distance: accumulate distance fallen, reset on landing
	if s.onGround {
		s.fallDistance = 0.0
	} else if s.Vel.Y < 0 {
		// Accumulate downward velocity as fall distance
		s.fallDistance -= newVel.Y
	}

	// Extract position from bounding box (center of X/Z, min of Y)
	// This MUST happen AFTER edge prevention to respect position reverts
	oldX := s.Pos.X
	oldZ := s.Pos.Z
	s.Pos.X = newPlayerBB.X.Min + s.width/2
	s.Pos.Y = newPlayerBB.Y.Min
	s.Pos.Z = newPlayerBB.Z.Min + s.width/2

	if os.Getenv("DEBUG_MANUAL_MOVEMENT") != "" {
		log.Printf("[DEBUG_MANUAL_POS] Vel before collision=(%.4f,%.4f) after=(%.4f,%.4f) Pos before=(%.4f,%.4f) after=(%.4f,%.4f)",
			s.Vel.X, s.Vel.Z, newVel.X, newVel.Z, oldX, oldZ, s.Pos.X, s.Pos.Z)
	}

	// Update velocity
	s.Vel = newVel
}

// isStandingOnSurface matches Java's PlayerEntity.isStandingOnSurface(float stepHeight).
// Returns true when the player is on ground, or has fallen less than stepHeight
// and still has block support below.
func (s *state) isStandingOnSurface(playerBB AABB, stepHeight float64, w World) bool {
	return s.onGround || (s.fallDistance < stepHeight && !s.isSpaceAroundPlayerEmpty(playerBB, 0.0, 0.0, stepHeight-s.fallDistance, w))
}

// adjustMovementForSneaking implements Minecraft's three-phase sneaking edge prevention algorithm.
// Reduces movement in 0.05 block increments when approaching edges.
// Based on PlayerEntity.adjustMovementForSneaking() from Minecraft 1.21.10.
func (s *state) adjustMovementForSneaking(playerBB AABB, x, z float64, w World) (float64, float64) {
	const stepIncrement = 0.05
	stepHeight := StepHeight
	dX := x
	dZ := z

	// Matches Java guard: isStandingOnSurface(stepHeight)
	if !s.isStandingOnSurface(playerBB, stepHeight, w) {
		return dX, dZ
	}

	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		log.Printf("[SneakEdge] Starting adjustment: dX=%.3f, dZ=%.3f\n", dX, dZ)
	}

	// Phase 1: Reduce X-axis movement until space is clear
	// Loop continues WHILE space is empty (no support below)
	// Loop stops WHEN space is NOT empty (support detected)
	hX := math.Copysign(stepIncrement, dX)
	for ; dX != 0 && s.isSpaceAroundPlayerEmpty(playerBB, dX, 0, stepHeight, w); dX -= hX {
		if math.Abs(dX) <= stepIncrement {
			dX = 0
			break
		}
	}

	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		log.Printf("[SneakEdge] After X phase: dX=%.3f\n", dX)
	}

	// Phase 2: Reduce Z-axis movement until space is clear
	hZ := math.Copysign(stepIncrement, dZ)
	for dZ != 0 && s.isSpaceAroundPlayerEmpty(playerBB, 0, dZ, stepHeight, w) {
		if math.Abs(dZ) <= stepIncrement {
			dZ = 0
			break
		}
		dZ -= hZ
	}

	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		log.Printf("[SneakEdge] After Z phase: dZ=%.3f\n", dZ)
	}

	// Phase 3: Reduce diagonal movement (both axes) until space is clear
	for dX != 0 && dZ != 0 && s.isSpaceAroundPlayerEmpty(playerBB, dX, dZ, stepHeight, w) {
		if math.Abs(dX) <= stepIncrement {
			dX = 0
		} else {
			dX -= hX
		}

		if math.Abs(dZ) <= stepIncrement {
			dZ = 0
		} else {
			dZ -= hZ
		}
	}

	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		log.Printf("[SneakEdge] Final result: dX=%.3f, dZ=%.3f\n", dX, dZ)
	}

	return dX, dZ
}

// handleEntityCollisions applies a geometry-based horizontal separation impulse when
// another entity's AABB overlaps with the player.
//
// This mirrors vanilla Minecraft's Entity.pushAwayFrom() formula:
//
//	chebyshev = max(|dx|, |dz|)  (Chebyshev distance between centres)
//	sqrtF     = sqrt(chebyshev)
//	scale     = min(1/sqrtF, 1) * EntitySeparationForce
//	pushX     = (dx/sqrtF) * scale,  pushZ = (dz/sqrtF) * scale
//
// Key properties:
//   - Deep overlaps (chebyshev → 0) produce a tiny force, so a /tp to the operator's
//     exact position causes minimal drift rather than the runaway 7-block displacement
//     that occurred when entity *velocity* was copied each tick.
//   - Shallow overlaps (chebyshev near 0.6, edge of AABB contact) produce up to
//     EntitySeparationForce blocks/tick, giving visible bump response.
//   - No vertical (Y) component — gravity and block physics handle vertical separation.
//   - Entities whose centres are < 0.01 blocks apart receive no push (matches vanilla).
//
// Knockback and server-applied impulses are delivered via ClientboundEntityVelocity →
// onEntityVelocityUpdate → moveExec.SetVelocity and are unaffected by this function.
func (s *state) handleEntityCollisions(playerBB AABB, newVel *models.V3, w World) {
	entities := w.GetEntitiesInRange(playerBB)
	for _, entity := range entities {
		// Standable entities (e.g. a happy ghast while staying still) are
		// already handled as block-like obstacles by
		// computeCollisionYXZWithStandableEntities, which gives proper
		// vertical support in addition to horizontal clipping. Also running
		// this softer separation-impulse push on them would double-handle
		// the same contact and fight the harder collision's result — matches
		// vanilla, where a genuinely collidable entity is resolved by the
		// same box-adjustment algorithm as blocks and never additionally
		// nudged by the softer per-tick separation vanilla uses for merely
		// overlappable entities.
		if entity.Standable {
			continue
		}

		entityCenterX := (entity.AABB.X.Min + entity.AABB.X.Max) / 2
		entityCenterZ := (entity.AABB.Z.Min + entity.AABB.Z.Max) / 2
		playerCenterX := (playerBB.X.Min + playerBB.X.Max) / 2
		playerCenterZ := (playerBB.Z.Min + playerBB.Z.Max) / 2

		// Separation direction: entity centre → player centre
		dx := playerCenterX - entityCenterX
		dz := playerCenterZ - entityCenterZ

		chebyshev := math.Max(math.Abs(dx), math.Abs(dz))
		if chebyshev < 0.01 {
			// Entities at nearly the same position — no push (matches vanilla behaviour).
			continue
		}

		// Vanilla formula: scale by min(1/sqrt(chebyshev), 1) * EntitySeparationForce.
		sqrtF := math.Sqrt(chebyshev)
		scale := math.Min(1.0/sqrtF, 1.0) * EntitySeparationForce
		newVel.X += (dx / sqrtF) * scale
		newVel.Z += (dz / sqrtF) * scale
	}
}

// isSpaceAroundPlayerEmpty checks if there's space for the player to move.
// Tests a box from minY - stepHeight to minY with the given XZ offset.
// This implements the space check from PlayerEntity.isSpaceAroundPlayerEmpty().
func (s *state) isSpaceAroundPlayerEmpty(playerBB AABB, offsetX, offsetZ, stepHeight float64, w World) bool {
	if stepHeight == 0 {
		stepHeight = StepHeight // Use default player step height (0.6 blocks)
	}

	// Get player's current center position (feet level)
	playerCenterX := playerBB.X.Min + s.width/2
	playerCenterZ := playerBB.Z.Min + s.width/2

	// Create test box: same horizontal size as player, but check from (minY - stepHeight) to minY
	// This checks for blocks that would block movement to the new position
	testBB := AABB{
		X: MinMax{
			Min: playerBB.X.Min + 1e-7 + offsetX,
			Max: playerBB.X.Max - 1e-7 + offsetX,
		},
		Y: MinMax{
			Min: playerBB.Y.Min - stepHeight - 1e-7,
			Max: playerBB.Y.Min,
		},
		Z: MinMax{
			Min: playerBB.Z.Min + 1e-7 + offsetZ,
			Max: playerBB.Z.Max - 1e-7 + offsetZ,
		},
	}

	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		log.Printf("[SpaceCheck] Testing offset (%.3f, %.3f) from pos (%.2f, %.2f)\n",
			offsetX, offsetZ, playerCenterX, playerCenterZ)
		log.Printf("[SpaceCheck]   testBB: X[%.3f-%.3f] Y[%.3f-%.3f] Z[%.3f-%.3f]\n",
			testBB.X.Min, testBB.X.Max,
			testBB.Y.Min, testBB.Y.Max,
			testBB.Z.Min, testBB.Z.Max)
	}

	// Get all collision boxes in the test area
	collisionBoxes := s.getSurroundingBoxes(testBB, w)

	// If ANY solid block intersects the test box, space is NOT empty (i.e., supported)
	// Note: We intentionally do NOT skip the block directly under the current position.
	// Vanilla adjustMovementForSneaking() treats that block as valid support while moving within it.
	for _, box := range collisionBoxes {
		if testBB.Intersects(box) {
			if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
				log.Printf("[SpaceCheck]   SUPPORT/COLLISION with block X[%.2f-%.2f] Y[%.2f-%.2f] Z[%.2f-%.2f]\n",
					box.X.Min, box.X.Max, box.Y.Min, box.Y.Max, box.Z.Min, box.Z.Max)
			}
			return false // Space is NOT empty - there is support/collision
		}
	}

	if os.Getenv("DEBUG_SNEAK_EDGE") != "" {
		log.Printf("[SpaceCheck]   OK - space is empty\n")
	}
	return true // Space is empty
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

// ResolveCollision performs collision detection and resolution for an arbitrary AABB
// moving with the given velocity through the world. Returns the corrected AABB,
// corrected velocity, and whether horizontal/vertical collisions occurred.
// This is the exported version of computeCollisionYXZ for use by riding handlers
// that need collision detection for ridden entities with non-player dimensions.
func (s *state) ResolveCollision(entityBB AABB, vel models.V3, w World) (AABB, models.V3, bool, bool) {
	resultBB, resultVel := s.computeCollisionYXZ(entityBB, vel, w)
	horizontalCollision := resultVel.X != vel.X || resultVel.Z != vel.Z
	verticalCollision := resultVel.Y != vel.Y
	return resultBB, resultVel, horizontalCollision, verticalCollision
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

// isOverlappingCobweb reports whether the player's current bounding box
// intersects any cobweb block, mirroring Java's per-tick entityInside/
// onEntityCollision dispatch — hitbox overlap, not a single point sample,
// since a cobweb occupies the block's full outline shape even though it has
// no collision boxes of its own (getSurroundingBoxes already skips it as
// passable). Checked at the player's pre-move position, applied within the
// same tick, rather than replicating vanilla's exact one-tick-delayed
// movementMultiplier/stuckSpeedMultiplier field — the steady-state behavior
// while continuously overlapping is the same either way, and every other
// per-tick block check in this file (ladder, ice) already uses the
// pre-move position the same way. Must only be called while the write lock
// is held.
func (s *state) isOverlappingCobweb(w World) bool {
	bb := s.getAABBUnsafe()
	minX := int(math.Floor(bb.X.Min))
	maxX := int(math.Floor(bb.X.Max))
	minY := int(math.Floor(bb.Y.Min))
	maxY := int(math.Floor(bb.Y.Max))
	minZ := int(math.Floor(bb.Z.Min))
	maxZ := int(math.Floor(bb.Z.Max))

	for y := minY; y <= maxY; y++ {
		for z := minZ; z <= maxZ; z++ {
			for x := minX; x <= maxX; x++ {
				blockStateID, _ := w.GetBlockStatus(x, y, z)
				if s.shapeProvider.IsCobweb(blockStateID) {
					return true
				}
			}
		}
	}
	return false
}

// computeCollisionYXZWithStandableEntities is computeCollisionYXZ's walking-player
// counterpart: the same Y-then-X-then-Z box-adjustment sweep, but the box list
// also includes any "standable" entities in range (see models.EntityBounds.Standable) —
// a happy ghast while staying still, for example — merged in alongside the
// ordinary block boxes so the sweep gives them the same support-from-above
// treatment blocks already get, not just horizontal separation.
//
// This is deliberately NOT folded into computeCollisionYXZ/ResolveCollision.
// Those are also used by every riding handler's own collision resolution
// (via resolveEntityCollision), and neither function nor its callers carry
// the moving entity's own ID — so a ridden entity querying "standable
// entities near me" would have no way to exclude itself and could collide
// against its own bounding box. The walking player's own entity list
// (World.GetEntitiesInRange, backed by agent.GetEntitiesSnapshot) already
// excludes the player's own ID, so this variant is safe to use for that one
// call site without needing to thread an exclusion ID through the whole
// shared collision API for a feature that, for now, is scoped to a walking
// player standing on a stationary happy ghast — not vehicles resting on one
// another. See PHASE_6_PLAN.md §5.
func (s *state) computeCollisionYXZWithStandableEntities(playerBB AABB, vel models.V3, w World) (AABB, models.V3) {
	queryBB := playerBB.Offset(vel.X, vel.Y, vel.Z)
	surroundings := s.getSurroundingBoxes(queryBB, w)
	surroundings = append(surroundings, s.getStandableEntityBoxes(queryBB, w)...)

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

// getStandableEntityBoxes returns the AABBs of entities in range that are
// currently flagged Standable (see models.EntityBounds.Standable) — the
// entity-sourced counterpart to getSurroundingBoxes's block-sourced boxes.
func (s *state) getStandableEntityBoxes(queryBB AABB, w World) []AABB {
	entities := w.GetEntitiesInRange(queryBB)
	var boxes []AABB
	for _, entity := range entities {
		if entity.Standable {
			boxes = append(boxes, entity.AABB)
		}
	}
	return boxes
}

// IsLookingAtTarget returns true if the player's current look direction matches
// the target yaw and pitch within a small tolerance.
func (s *state) IsLookingAtTarget(targetYaw, targetPitch float64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	deltaYaw := math.Abs(NormalizeAngle(targetYaw - s.yaw))
	deltaPitch := math.Abs(targetPitch - s.pitch)
	return deltaYaw <= 0.8 && deltaPitch <= 1.1
}

// copyFieldsUnsafe copies all mutable fields into a new state without copying the mutex.
// Must only be called while the caller holds at least a read lock on the source state.
func (s *state) copyFieldsUnsafe() state {
	return state{
		Pos:           s.Pos,
		Vel:           s.Vel,
		yaw:           s.yaw,
		pitch:         s.pitch,
		onGround:      s.onGround,
		isSneaking:    s.isSneaking,
		isSwimming:    s.isSwimming,
		isInWater:     s.isInWater,
		collision:     s.collision,
		tick:          s.tick,
		lastJump:      s.lastJump,
		fallDistance:  s.fallDistance,
		width:         s.width,
		height:        s.height,
		eyeHeight:     s.eyeHeight,
		shapeProvider: s.shapeProvider,
	}
}

// PredictMovement simulates N ticks ahead without modifying the current state.
// Returns a slice of predicted states, one for each tick simulated.
// This is useful for path validation and trajectory prediction.
func (s *state) PredictMovement(inputs []Inputs, maxTicks int, w World) []models.PhysicsState {
	// Create a copy of the current state's fields for simulation.
	// Fields are copied explicitly to avoid copying the mutex.
	s.mu.RLock()
	predicted := s.copyFieldsUnsafe()
	s.mu.RUnlock()

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
		// Snapshot current fields without copying the mutex
		predicted.mu.RLock()
		snapshot := predicted.copyFieldsUnsafe()
		predicted.mu.RUnlock()
		results = append(results, &snapshot)
	}

	return results
}

// PredictPosition performs a fast position-only prediction without full physics.
// This simulates free fall or ballistic motion for 'ticks' iterations.
// Useful for quick fall distance calculations. This is an approximate prediction
// that doesn't account for full collision physics.
func (s *state) PredictPosition(vel models.V3, ticks int, w World) models.V3 {
	s.mu.RLock()
	pos := s.Pos
	s.mu.RUnlock()
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
	s.mu.RLock()
	height := s.height
	isSneaking := s.isSneaking
	width := s.width
	s.mu.RUnlock()

	// Create AABB at target position
	if isSneaking {
		height = PlayerHeightSneaking
	}

	targetBB := AABB{
		X: MinMax{Min: targetPos.X - width/2, Max: targetPos.X + width/2},
		Y: MinMax{Min: targetPos.Y, Max: targetPos.Y + height},
		Z: MinMax{Min: targetPos.Z - width/2, Max: targetPos.Z + width/2},
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
