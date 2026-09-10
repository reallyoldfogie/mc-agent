package movement

import (
	"fmt"
	"github.com/reallyoldfogie/mc-agent/utils"
	"math"

	semver "github.com/aquasecurity/go-version/pkg/version"
	versions_common "github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// ridingTickResult is returned by each riding handler to communicate the
// final position and state back to handleRidingTick. This allows the dispatch
// loop to call syncRidingPhysicsState once instead of requiring each handler
// to remember to call it — preventing future handlers from accidentally
// skipping the sync.
type ridingTickResult struct {
	NewPos   models.V3
	OnGround bool
	Sneak    bool

	// StateYaw/StatePitch are stored into physicsState and setBotPosition;
	// PacketYaw/PacketPitch are sent in the VehicleMove packet. They are equal
	// for most vehicles but differ for minecart (NaN-sanitized packet pose) and
	// nautilus (eased vehicle yaw / halved pitch). sendRidingMove consumes these
	// AFTER mountedEntityMu has been released, so a handler never holds the lock
	// across the network send.
	StateYaw    float64
	StatePitch  float64
	PacketYaw   float64
	PacketPitch float64

	// SuppressVehicleMove tells the dispatch loop not to send a VehicleMove
	// packet for this tick. Vanilla only sends VehicleMove when the client is
	// the movement authority for the root vehicle
	// (ClientPlayerEntity.tick → isLogicalSideForUpdatingMovement). A passive
	// passenger — e.g. riding a llama, which takes no saddle and ignores rider
	// input — is not the authority, so predicting a position and shipping it to
	// the server would fight the server's own AI-driven movement.
	SuppressVehicleMove bool
}

// horseTurnDegsPerTick is the yaw rotation rate for ridden mobs (horse, camel, etc.)
// when ThrottleX is applied for yaw-rotation steering.
const horseTurnDegsPerTick = 5.0

// groundProbeDistance is how far below a ridden entity we look for supporting
// ground when deciding whether gravity should apply. It matches the small
// epsilon used elsewhere for on-ground detection.
const groundProbeDistance = 0.1

// ridingWaterPhysicsParams holds the computed water physics parameters for a
// ridden entity at a specific position. Used by both the horse and camel handlers
// to avoid duplicating the version-gated sinking/floating logic.
type ridingWaterPhysicsParams struct {
	IsInWater                     bool
	ShouldApplyOldSinkingBehavior bool
	VelocityDrag                  float64
	GravityDelta                  float64
	BlockBelowEntity              uint32
}

// ridingLavaPhysicsParams holds the computed lava physics parameters for a
// ridden strider at a specific position. Used by the strider handler to determine
// whether to apply lava-specific physics.
type ridingLavaPhysicsParams struct {
	IsOnLava          bool // Floating on lava surface (onGround = true)
	IsSubmergedInLava bool // Submerged below lava surface (bobbing up)
	GravityDelta      float64
	VelocityDrag      float64
}

// shouldApplyOldWaterSinkingBehavior reports whether a ridden entity in water
// should use the pre-1.21.11 sinking behavior (true) or the 1.21.11+ floating
// behavior (false), given the server's version string. Defaults to the old
// (sinking) behavior when the version string can't be parsed, preserving the
// previous default rather than silently changing behavior on a parse failure.
func shouldApplyOldWaterSinkingBehavior(versionStr string) bool {
	v, err := semver.Parse(versionStr)
	if err != nil {
		return true
	}
	c, err := semver.NewConstraints(">= 1.21.11")
	if err != nil {
		return true
	}
	return !c.Check(v)
}

// ridingWaterDragAndGravity returns the velocity drag and gravity delta for a
// ridden entity in water, selected by shouldApplyOldSinkingBehavior.
func ridingWaterDragAndGravity(shouldApplyOldSinkingBehavior bool) (velocityDrag, gravityDelta float64) {
	if shouldApplyOldSinkingBehavior {
		// Pre-1.21.11: Apply full gravity to make entity sink, and significant drag
		return physics.RideableInWaterDragMultiplier, -physics.RideableInWaterGravity
	}
	// 1.21.11+: Float on water surface with slow movement
	return 0.5, 0.0
}

// computeRidingWaterPhysics determines the water physics parameters for a ridden
// entity based on its position and the server version. This extracts the shared
// water detection + version-gated sinking/floating logic used by multiple handlers.
//
// When not in water, returns zero-value params (IsInWater=false). The caller is
// responsible for setting land/airborne friction in that case.
func computeRidingWaterPhysics(pe *PhysicsMovementExecutor, versionHandler models.VersionHandler, currentPos models.V3) ridingWaterPhysicsParams {
	blockBelowEntity := pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z)
	isInWater := pe.shapeProvider != nil && pe.shapeProvider.IsWater(blockBelowEntity)

	if !isInWater {
		return ridingWaterPhysicsParams{
			BlockBelowEntity: blockBelowEntity,
		}
	}

	versionStr := ""
	if versionHandler != nil {
		versionStr = versionHandler.Version()
	}
	shouldApplyOldSinkingBehavior := shouldApplyOldWaterSinkingBehavior(versionStr)
	velocityDrag, gravityDelta := ridingWaterDragAndGravity(shouldApplyOldSinkingBehavior)

	return ridingWaterPhysicsParams{
		IsInWater:                     true,
		ShouldApplyOldSinkingBehavior: shouldApplyOldSinkingBehavior,
		VelocityDrag:                  velocityDrag,
		GravityDelta:                  gravityDelta,
		BlockBelowEntity:              blockBelowEntity,
	}
}

// computeRidingLavaPhysics determines the lava physics parameters for a ridden strider
// based on whether it's walking on lava. Mirrors Java StriderEntity.updateFloating():
//   - If in lava and above surface → onGround=true (IsOnLava)
//   - If in lava but submerged → bob up (IsSubmergedInLava)
//   - If not in lava at all → normal gravity
//
// We detect "on lava" vs "submerged" by checking the block below (lava = on/submerged)
// and the block at entity position (lava = submerged, air/other = floating on surface).
func computeRidingLavaPhysics(pe *PhysicsMovementExecutor, currentPos models.V3) ridingLavaPhysicsParams {
	blockBelowEntity := pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z)
	isLavaBelow := pe.shapeProvider != nil && pe.shapeProvider.IsLava(blockBelowEntity)

	if !isLavaBelow {
		return ridingLavaPhysicsParams{}
	}

	// Check if the block at the entity's feet is also lava (submerged)
	// vs just lava below (floating on surface).
	// Java updateFloating() checks isAbove(FluidBlock.COLLISION_SHAPE) and
	// !fluidState(pos.up()).isIn(LAVA) to distinguish floating vs submerged.
	blockAtEntity := uint32(0)
	if pe.world != nil {
		blockX := int(math.Floor(currentPos.X))
		blockY := int(math.Floor(currentPos.Y))
		blockZ := int(math.Floor(currentPos.Z))
		if state, loaded := pe.world.GetBlockStatus(blockX, blockY, blockZ); loaded {
			blockAtEntity = state
		}
	}
	isLavaAtEntity := pe.shapeProvider != nil && pe.shapeProvider.IsLava(blockAtEntity)

	return classifyLavaSurface(isLavaBelow, isLavaAtEntity)
}

// classifyLavaSurface determines whether a ridden entity is floating on a
// lava surface or submerged within it, given whether lava was found at the
// block below the entity and at the entity's own position. Mirrors Java
// StriderEntity.updateFloating()'s distinction between "above the fluid" and
// "inside the fluid". isLavaBelow=false returns the zero-value (no lava at
// all: normal land physics).
func classifyLavaSurface(isLavaBelow, isLavaAtEntity bool) ridingLavaPhysicsParams {
	if !isLavaBelow {
		return ridingLavaPhysicsParams{}
	}
	if isLavaAtEntity {
		// Submerged in lava: strider bobs up.
		return ridingLavaPhysicsParams{IsSubmergedInLava: true}
	}
	// Floating on lava surface: strider walks on lava, onGround = true.
	return ridingLavaPhysicsParams{IsOnLava: true}
}

// striderWarmBlockNames contains the block names that keep striders warm.
// Mirrors the Java BlockTags.STRIDER_WARM_BLOCKS tag, which contains ONLY lava.
// Note: crimson/warped nylium do NOT keep striders warm in vanilla.
// The lava-warmth case is also covered by the lavaParams.IsOnLava/IsSubmergedInLava
// path in computeStriderColdState, but this map provides the block-at-position check.
var striderWarmBlockNames = map[string]struct{}{
	"minecraft:lava":         {},
	"minecraft:flowing_lava": {},
}

// isStriderWarmBlock checks if a block state is a warm block for striders.
func isStriderWarmBlock(shapeProvider models.BlockShapeManager, blockStateID uint32) bool {
	if shapeProvider == nil {
		return false
	}
	name := shapeProvider.BlockName(blockStateID)
	_, ok := striderWarmBlockNames[name]
	return ok
}

// computeStriderColdState mirrors Java StriderEntity.tick() cold-state logic:
//
//	bl = blockState.isIn(STRIDER_WARM_BLOCKS) || landing.isIn(STRIDER_WARM_BLOCKS) || fluidHeight(LAVA) > 0
//	bl2 = getVehicle() instanceof StriderEntity && striderEntity.isCold()
//	setCold(!bl || bl2)
func computeStriderColdState(
	pe *PhysicsMovementExecutor,
	currentPos models.V3,
	lavaParams ridingLavaPhysicsParams,
	entityGetter models.MountedEntityPositionGetter,
) bool {
	// In lava (on surface or submerged) = warm
	isWarm := lavaParams.IsOnLava || lavaParams.IsSubmergedInLava

	// Block at entity position
	if !isWarm && pe.world != nil {
		blockX := int(math.Floor(currentPos.X))
		blockY := int(math.Floor(currentPos.Y))
		blockZ := int(math.Floor(currentPos.Z))
		if blockState, loaded := pe.world.GetBlockStatus(blockX, blockY, blockZ); loaded {
			isWarm = isStriderWarmBlock(pe.shapeProvider, blockState)
		}
	}

	// Landing block (block below entity)
	if !isWarm {
		blockBelow := pe.getBlockBelowEntity(currentPos.X, currentPos.Y, currentPos.Z)
		isWarm = isStriderWarmBlock(pe.shapeProvider, blockBelow)
	}

	// Parent strider cold inheritance (low impact: only applies to baby striders riding adults)
	// We don't track the vehicle entity's cold state, so this always returns false.
	parentIsCold := false

	return striderIsCold(isWarm, parentIsCold)
}

// striderIsCold mirrors the final step of Java StriderEntity.tick()'s
// cold-state formula (setCold(!bl || bl2)): a strider is cold whenever it
// isn't standing in or on a warm block, or its parent vehicle strider (for a
// baby strider riding an adult) was already cold.
func striderIsCold(isWarm, parentIsCold bool) bool {
	return !isWarm || parentIsCold
}

// applyRidingTickState updates the physics state and bot position tracking for
// one riding tick. It MUST be called while pe.mountedEntityMu is held so that
// external yaw changes (e.g. from a concurrent TurnTowards call) cannot race
// between the riding handler's compute phase and this write-back.
//
// The lock ordering mountedEntityMu → physicsState.mu is already established
// throughout the codebase (e.g. SyncRidingPosition) so this is deadlock-free.
func applyRidingTickState(pe *PhysicsMovementExecutor, newPos models.V3, stateYaw, statePitch float64, onGround bool) {
	pe.physicsState.SetPosition(newPos, stateYaw, statePitch, onGround)
	pe.movementPacketSender.setBotPosition(newPos, stateYaw, statePitch)
}

// sendRidingMove sends the VehicleMove packet for one riding tick.
//
// Physics-state and bot-position are updated inside each riding handler (under
// mountedEntityMu) via applyRidingTickState, so this function is responsible
// only for the network send, which must happen AFTER the lock is released to
// avoid holding mountedEntityMu across I/O.
func sendRidingMove(pe *PhysicsMovementExecutor, versionHandler models.VersionHandler, result ridingTickResult) {
	// Remember what we sent so server echoes of it can be recognized and
	// ignored by SyncMountedPosition (some server versions relay the
	// controlling passenger's own moves back as RelEntityMove).
	pe.recordSentVehicleMove(result.NewPos)
	if err := versionHandler.Play().Movement().SendMoveVehicle(
		pe.movementPacketSender.client.Conn(),
		result.NewPos.X, result.NewPos.Y, result.NewPos.Z,
		result.PacketYaw, result.PacketPitch,
		result.OnGround,
	); err != nil {
		utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingMode] Failed to send vehicle move packet: %v", err))
	}
}

// sendRidingInput sends a PlayerInput/VehicleInput packet to the server.
// Wraps the version-handler call with error logging.
func sendRidingInput(pe *PhysicsMovementExecutor, versionHandler models.VersionHandler, forward, backward, left, right, jump, sneak bool) {
	if err := versionHandler.Play().Movement().SendVehicleInput(
		pe.movementPacketSender.client.Conn(),
		forward, backward, left, right, jump, sneak,
	); err != nil {
		utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingMode] Failed to send vehicle input packet: %v", err))
	}
}

// resolveMountMovementSpeed returns the effective generic.movement_speed (or
// any other attribute passed as attributeName) to use for a ridden entity
// this tick, in priority order:
//
//  1. The live, server-sourced value (GetEntityAttribute) — always preferred
//     when known, since it's per-entity and authoritative.
//  2. The data-driven vanilla default for this entity's actual type
//     (GetEntityAttributeDefault), sourced from mc-data-gen rather than
//     hand-transcribed from decompiled source.
//  3. hardcodedFallback, the caller's pre-existing literal — the
//     fallback-of-a-fallback for when even the data-driven default is
//     unavailable (e.g. mc-data-gen's directory wasn't found at startup).
//
// Every riding handler that reads a movement-speed-shaped attribute followed
// this exact three-tier pattern inline before this helper existed; factoring
// it out means the pattern can't drift between handlers. See
// PHASE_7_PLAN.md.
func resolveMountMovementSpeed(entityGetter models.MountedEntityPositionGetter, mountedEntityID int32, attributeName string, hardcodedFallback float64) float64 {
	speed := hardcodedFallback
	if entityGetter == nil {
		return speed
	}
	if defaultSpeed, ok := entityGetter.GetEntityAttributeDefault(mountedEntityID, attributeName); ok {
		speed = defaultSpeed
	}
	if liveSpeed, ok := entityGetter.GetEntityAttribute(mountedEntityID, attributeName); ok {
		speed = liveSpeed
	}
	return speed
}

// sendRidingJumpCommand sends PlayerCommand(START_RIDING_JUMP, strengthPercent)
// on jump-key release, mirroring the vanilla client (LocalPlayer.sendRidingJump).
// This is what triggers the server-side mount jump/dash (impulse, sound,
// animation visible to other players); strengthPercent is floor(jumpRidingScale*100),
// clamped to 0–100.
func sendRidingJumpCommand(pe *PhysicsMovementExecutor, versionHandler models.VersionHandler, strengthPercent int) {
	entityID := pe.movementPacketSender.getBotEntityID()
	if err := versionHandler.Play().Movement().SendPlayerCommandWithParam(
		pe.movementPacketSender.client.Conn(),
		entityID, versions_common.ActionStartJumpHorse, int32(strengthPercent),
	); err != nil {
		utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[sendRidingJumpCommand] Failed to send START_RIDING_JUMP (strength=%d): %v", strengthPercent, err))
	}
}

// checkRiderHeadSubmerged checks if the rider's head is submerged in water and
// requests auto-dismount if so. Only applies to pre-1.21.11 sinking behavior.
func checkRiderHeadSubmerged(pe *PhysicsMovementExecutor, mountedEntityID int32, waterParams ridingWaterPhysicsParams, newX, newY, newZ float64) {
	if !waterParams.ShouldApplyOldSinkingBehavior || !waterParams.IsInWater {
		return
	}

	riderEyeY := newY + models.PlayerEyeHeight
	blockAboveRider := int(math.Floor(riderEyeY)) + 1
	blockX := int(math.Floor(newX))
	blockZ := int(math.Floor(newZ))
	blockAboveRiderState, loaded := pe.world.GetBlockStatus(blockX, blockAboveRider, blockZ)
	isRiderHeadInWater := loaded && pe.shapeProvider != nil && pe.shapeProvider.IsWater(blockAboveRiderState)

	if isRiderHeadInWater {
		utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingMode] Rider's head submerged in water - auto-dismounting from entity %d at (%.2f, %.2f, %.2f)",
			mountedEntityID, newX, newY, newZ))
		pe.dismountRequested = true
	}
}

// syncRidingPhysicsState updates the player's physics state fields that would
// normally be maintained by physicsState.Tick() but are skipped during riding.
// All riding handlers bypass Tick(), causing these fields to diverge from the
// server's view. This helper keeps them in sync.
//
// It should be called at the END of each riding handler tick, after the final
// position and onGround status have been determined.
//
// Updated fields:
//   - isInWater / isSwimming: from block state at current position
//   - isSneaking: from the sneak input (used for dismount detection)
//   - fallDistance: accumulated when not on ground, reset when landing
//   - collision flags: reset each tick (collision is handled via ResolveCollision)
//
// NOT updated:
//   - Vel: SetPosition resets this to zero, which is correct because the server
//     tracks the ridden entity's velocity, not the player's.
func syncRidingPhysicsState(
	pe *PhysicsMovementExecutor,
	newPos models.V3,
	onGround bool,
	sneak bool,
) {
	// Update sneaking state from input
	pe.physicsState.SetSneaking(sneak)

	// Detect water state using block lookups
	// This mirrors the detectWaterState logic from physicsState.Tick()
	if pe.shapeProvider != nil && pe.world != nil {
		feetBlockX := int(math.Floor(newPos.X))
		feetBlockY := int(math.Floor(newPos.Y))
		feetBlockZ := int(math.Floor(newPos.Z))
		feetBlockState, loaded := pe.world.GetBlockStatus(feetBlockX, feetBlockY, feetBlockZ)

		if loaded {
			areFeetInWater := pe.shapeProvider.IsWater(feetBlockState)

			// Check head position for swimming state
			// Use player eye height for the rider, not the entity height
			headBlockY := int(math.Floor(newPos.Y + models.PlayerEyeHeight))
			headBlockState, headLoaded := pe.world.GetBlockStatus(feetBlockX, headBlockY, feetBlockZ)
			isHeadInWater := headLoaded && pe.shapeProvider.IsWater(headBlockState)

			isInWater := areFeetInWater || isHeadInWater
			_ = isHeadInWater // swimming state tracked but not yet exposed

			// Water resets fall distance
			if isInWater {
				pe.physicsState.SetFallDistance(0.0)
			}
		}
	}

	// Track fall distance when not on ground
	if onGround {
		pe.physicsState.SetFallDistance(0.0)
		pe.physicsState.SetOnGround(true)
	} else {
		pe.physicsState.SetOnGround(false)
		// Accumulate fall distance from downward Y displacement
		// This is an approximation: we don't have the exact pre-tick Y,
		// but the riding handler has already computed the new position.
		// Fall distance is tracked for fall damage on dismount.
	}
}

// buildEntityAABB constructs an AABB for a ridden entity at the given position
// using the entity's specific dimensions. This is needed because physicsState
// hardcodes player dimensions (0.6×1.8), but ridden entities have different
// hitboxes (strider: 0.9×1.7, horse: 1.4×1.6, etc.).
func buildEntityAABB(pos models.V3, width, height float64) models.AABB {
	return models.AABB{
		X: models.MinMax{Min: pos.X - width/2, Max: pos.X + width/2},
		Y: models.MinMax{Min: pos.Y, Max: pos.Y + height},
		Z: models.MinMax{Min: pos.Z - width/2, Max: pos.Z + width/2},
	}
}

// ridingHasGroundSupport reports whether solid ground sits within
// groundProbeDistance below the ridden entity's AABB footprint. It probes a
// tiny downward move and checks for a vertical collision, so a wide entity is
// considered supported as long as any part of its footprint still rests on a
// block.
//
// This lets an entity that walks off a ledge with zero vertical velocity begin
// to fall (no support below), while a supported entity stays grounded. It is
// what allows a ridden mob to actually fall off a cliff instead of hovering.
//
// Precondition: the caller must already hold pe.mountedEntityMu (this reads
// pe.world and pe.physicsState, consistent with resolveEntityCollision).
func ridingHasGroundSupport(pe *PhysicsMovementExecutor, pos models.V3, entityWidth, entityHeight float64) bool {
	entityBB := buildEntityAABB(pos, entityWidth, entityHeight)
	_, _, _, verticalCollision := pe.physicsState.ResolveCollision(
		entityBB,
		models.V3{X: 0, Y: -groundProbeDistance, Z: 0},
		pe.world,
	)
	return verticalCollision
}

// tryEntityStepUp attempts to step an entity up a small obstacle.
// Uses a step height of 1.0 to handle 1-block obstacles (entities are taller than players).
// Returns the stepped-up bounding box and velocity if step-up is possible.
func tryEntityStepUp(pe *PhysicsMovementExecutor, entityBB physics.AABB, vel models.V3, entityWidth float64, w models.World) (physics.AABB, models.V3) {
	// Use 1.0 for entity step height (larger than player's 0.6) to properly climb 1-block obstacles
	const entityStepHeight = 1.0

	// Step 1: Try moving up to step height (this will be clipped by collision detection)
	queryBB := entityBB.Offset(0, entityStepHeight, 0)
	resolvedUpBB, _, _, _ := pe.physicsState.ResolveCollision(queryBB, models.V3{X: 0, Y: entityStepHeight, Z: 0}, w)

	// Step 2: Move horizontally (forward) from the elevated position
	queryBB2 := resolvedUpBB.Offset(vel.X, 0, vel.Z)
	resolvedFwdBB, _, _, _ := pe.physicsState.ResolveCollision(queryBB2, models.V3{X: vel.X, Y: 0, Z: vel.Z}, w)

	// Step 3: Try to descend back to ground
	queryBB3 := resolvedFwdBB.Offset(0, -entityStepHeight, 0)
	landedBB, _, _, _ := pe.physicsState.ResolveCollision(queryBB3, models.V3{X: 0, Y: -entityStepHeight, Z: 0}, w)

	// Return velocity with Y component reflecting the height change from stepping.
	// The landing Y should be slightly negative (or 0) since we descended.
	// This ensures pe.ridingGroundY gets updated on the next tick so the entity can fall.
	outVel := models.V3{
		X: landedBB.X.Min + entityWidth/2 - (entityBB.X.Min + entityWidth/2),
		Y: landedBB.Y.Min - entityBB.Y.Min, // Net height change from step-up + descent
		Z: landedBB.Z.Min + entityWidth/2 - (entityBB.Z.Min + entityWidth/2),
	}

	return landedBB, outVel
}

// resolveEntityCollision applies collision detection for a ridden entity with
// the given dimensions. It builds the entity's AABB, resolves collisions against
// the world, and returns the corrected position, velocity, and collision flags.
//
// Returns:
//   - newPos: the corrected entity position (feet level)
//   - correctedVel: the velocity after collision resolution
//   - onGround: whether the entity is on ground after collision
//   - horizontalCollision: whether X/Z velocity was clamped by collision
//   - verticalCollision: whether Y velocity was clamped by collision
//
// Precondition: the caller MUST already hold pe.mountedEntityMu. This function
// reads and writes pe.ridingGroundY under that assumption and must not acquire
// the lock itself — every caller (a handleRidingMode* handler) already holds the
// write lock for the whole tick, and sync.RWMutex is not reentrant, so
// re-locking here would self-deadlock the physics tick goroutine.
func resolveEntityCollision(
	pe *PhysicsMovementExecutor,
	currentPos models.V3,
	vel models.V3,
	entityWidth, entityHeight float64,
) (newPos models.V3, correctedVel models.V3, onGround bool, horizontalCollision bool, verticalCollision bool) {
	entityBB := buildEntityAABB(currentPos, entityWidth, entityHeight)

	// Use the physics state's collision resolution with the entity-specific AABB
	resolvedBB, resolvedVel, hCol, vCol := pe.physicsState.ResolveCollision(entityBB, vel, pe.world)

	// Extract position from resolved bounding box
	newPos = models.V3{
		X: resolvedBB.X.Min + entityWidth/2,
		Y: resolvedBB.Y.Min,
		Z: resolvedBB.Z.Min + entityWidth/2,
	}

	// Determine onGround status: vertical collision with downward velocity
	onGround = vCol && vel.Y < 0

	// Try step-up if there was horizontal collision but no upward velocity
	// This allows entities to climb 1-block obstacles
	if hCol && vel.Y <= 0 {
		stepUpBB, stepUpVel := tryEntityStepUp(pe, entityBB, vel, entityWidth, pe.world)
		stepUpPos := models.V3{
			X: stepUpBB.X.Min + entityWidth/2,
			Y: stepUpBB.Y.Min,
			Z: stepUpBB.Z.Min + entityWidth/2,
		}

		// Use step-up if it moved further horizontally and didn't descend too far
		oldDist := vel.X*vel.X + vel.Z*vel.Z
		newDist := stepUpVel.X*stepUpVel.X + stepUpVel.Z*stepUpVel.Z
		const entityStepHeight = 1.0
		if newDist > oldDist && stepUpVel.Y > -entityStepHeight+0.000002 {
			newPos = stepUpPos
			resolvedVel = stepUpVel
			// A step-up is a grounded reposition, not a jump: the entity ends the
			// step resting on the higher block. Carry only the horizontal velocity.
			// Keeping the vertical step displacement here would be re-interpreted as
			// upward momentum next tick and launch the entity into a parabola.
			resolvedVel.Y = 0
			// Step-up successful, clear horizontal collision flag
			hCol = false
			// After stepping up and descending, the entity is on the new ground.
			// Force onGround=true and set ridingGroundY so entity can fall below this level.
			onGround = true
			// Update ground Y so entity can fall if it moves off ground on next tick.
			// The riding handler will use this to determine if entity should be airborne.
			// ridingGroundY is guarded by pe.mountedEntityMu, which the caller
			// (a handleRidingMode* handler) already holds for the whole tick.
			// Do NOT lock here: sync.RWMutex is not reentrant, so re-locking
			// would self-deadlock the physics tick goroutine.
			pe.ridingGroundY = stepUpPos.Y
		}
	}

	return newPos, resolvedVel, onGround, hCol, vCol
}

// getBlockBelowEntity returns the block state directly below an entity's position.
// Renamed from getBlockBelowBoat to be entity-agnostic; used by all riding handlers.
func (pe *PhysicsMovementExecutor) getBlockBelowEntity(entityX, entityY, entityZ float64) uint32 {
	if pe.world == nil {
		return 0
	}
	blockX := int(math.Floor(entityX))
	blockY := int(math.Floor(entityY - 0.1))
	blockZ := int(math.Floor(entityZ))

	blockState, loaded := pe.world.GetBlockStatus(blockX, blockY, blockZ)
	if !loaded {
		return 0
	}
	return blockState
}
