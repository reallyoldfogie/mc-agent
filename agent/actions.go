package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// MoveForward moves the bot forward based on current yaw using manual input control.
// Uses frame-by-frame control with the physics executor to move the specified distance.
// When sneaking, respects edge prevention naturally through physics constraints.
// Returns success when the movement completes or limited progress indicates edge prevention.
func (a *agent) MoveForward(ctx context.Context, dist float64) error {
	if a.moveExec == nil {
		return errors.New("movement executor not available")
	}

	// MoveForward requires an executor with manual movement control support
	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor doesn't support manual mode (required for MoveForward)")
	}

	x, y, z, yaw, _, ok := a.GetPosition()
	if !ok {
		return errors.New("position not initialized")
	}

	// Calculate throttle as world-space direction vector based on current yaw
	// Throttle represents absolute world direction, not player-relative WASD
	// Forward movement in world coordinates at current yaw angle
	// Negative distance means backward movement
	yawRad := float64(yaw) * math.Pi / 180
	direction := 1.0
	if dist < 0 {
		direction = -1.0
	}
	throttleX := direction * (-math.Sin(yawRad))
	throttleZ := direction * math.Cos(yawRad)

	// Store starting position for progress detection
	startPos := models.V3{X: x, Y: y, Z: z}

	// Calculate movement time based on speed and distance
	// Walk speed: ~0.215 blocks/tick at max
	// Sneak speed: 30% of walk speed = ~0.065 blocks/tick
	speedFactor := 1.0
	if a.moveExec.IsSneaking() {
		speedFactor = physics.SneakMultiplier // 0.3
	}
	baseSpeed := 0.215 // blocks per tick
	actualSpeed := baseSpeed * speedFactor
	tickCount := math.Abs(dist) / actualSpeed
	// Add 50% buffer for acceleration ramp-up
	tickCount *= 1.5

	// Enter manual mode for direct control
	if err := manual.EnterManualMode(); err != nil {
		return fmt.Errorf("failed to enter manual mode: %w", err)
	}
	defer manual.ExitManualMode()
	log.Printf("[MoveForward] Entered manual mode, currentYaw=%.2f, targetDist=%.2f", yaw, dist)

	// Set up movement with calculated throttle direction
	if err := manual.SetManualThrottle(throttleX, throttleZ); err != nil {
		return fmt.Errorf("failed to set throttle: %w", err)
	}
	log.Printf("[MoveForward] Throttle set to X=%.4f, Z=%.4f (forward direction at yaw=%.2f)", throttleX, throttleZ, yaw)

	// Keep current yaw (we're moving forward, not turning)
	if err := manual.SetManualRotation(math.NaN(), math.NaN()); err != nil {
		return fmt.Errorf("failed to set rotation: %w", err)
	}
	log.Printf("[MoveForward] Yaw maintained at current direction (%.2f)", yaw)

	// Execute movement for the calculated duration with context awareness
	tickInterval := 50 * time.Millisecond
	targetDuration := time.Duration(int64(tickCount*50)) * time.Millisecond
	startTime := time.Now()
	lastProgressTime := startTime
	lastProgressPos := startPos

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		elapsed := time.Since(startTime)

		// Check if we've reached the target distance
		currentX, _, currentZ, _, _, ok := a.GetPosition()
		if ok {
			currentPos := models.V3{X: currentX, Y: 0, Z: currentZ}
			distMoved := math.Sqrt((currentX-startPos.X)*(currentX-startPos.X) +
				(currentZ-startPos.Z)*(currentZ-startPos.Z))

			// Check for progress
			progress := currentPos.DistanceTo(lastProgressPos)

			if progress > 0.05 {
				lastProgressPos = currentPos
				lastProgressTime = time.Now()
			}

			// Success: we've moved the requested distance (within tolerance)
			if distMoved >= math.Abs(dist)*0.95 {
				log.Printf("[MoveForward] Movement complete: %.2f/%.2f blocks", distMoved, dist)
				return nil
			}

			// Edge prevention: very limited movement despite time elapsed
			if elapsed > time.Duration(int64(tickCount*25))*time.Millisecond && distMoved < 0.2 {
				log.Printf("[MoveForward] Movement blocked by edge prevention after %.2f blocks", distMoved)
				return nil // Return success - edge prevention is working
			}

			// Stuck detection: no progress for extended time
			if time.Since(lastProgressTime) > 5*time.Second {
				log.Printf("[MoveForward] No progress for 5 seconds, stopping after %.2f/%.2f blocks", distMoved, dist)
				return fmt.Errorf("movement stalled: only moved %.2f of %.2f blocks", distMoved, dist)
			}
		}

		// Timeout: exceeded reasonable movement time
		if elapsed > targetDuration+10*time.Second {
			if ok {
				currentX, _, currentZ, _, _, _ := a.GetPosition()
				distMoved := math.Sqrt((currentX-startPos.X)*(currentX-startPos.X) +
					(currentZ-startPos.Z)*(currentZ-startPos.Z))
				log.Printf("[MoveForward] Movement timeout after %.2f/%.2f blocks", distMoved, dist)
			}
			return errors.New("movement timed out")
		}

		// Sleep before next tick check
		if err := sleepWithContext(ctx, tickInterval); err != nil {
			return err
		}
	}
}

// MoveUp moves the bot vertically by distance.
func (a *agent) MoveUp(ctx context.Context, dist float64) error {
	if a.moveExec == nil {
		return errors.New("movement executor not available")
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return errors.New("position not initialized")
	}
	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond
	steps := int(math.Ceil(math.Abs(dist) / stepSize))
	for i := 0; i < steps; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		prog := float64(i+1) / float64(steps)
		if prog > 1 {
			prog = 1
		}
		ny := y + dist*prog
		if err := a.moveExec.SendPosition(x, ny, z, true); err != nil {
			return err
		}
		if err := sleepWithContext(ctx, stepDelay); err != nil {
			return err
		}
	}
	return nil
}

// stabilizeSneaking
// This prevents the server from applying gravity before the sneak takes effect.
func (a *agent) stabilizeSneaking(ctx context.Context) error {
	if a.moveExec == nil {
		return errors.New("movement executor not available")
	}

	// Start sneaking
	if err := a.moveExec.StartSneaking(); err != nil {
		return err
	}

	// Get current position
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return nil // Position not available, but sneak is started
	}

	// Continue sending position updates while sneaking to stabilize
	const stepDelay = 50 * time.Millisecond
	const stabilizationTicks = 10
	for i := 0; i < stabilizationTicks; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := a.moveExec.SendPosition(x, y, z, true); err != nil {
			return err
		}
		if err := sleepWithContext(ctx, stepDelay); err != nil {
			return err
		}
	}

	return nil
}

// MoveToAndSneak navigates to a position using pathfinding and then starts sneaking.
// This is useful for ending movement on ladders or edges where sneaking holds position.
func (a *agent) MoveToAndSneak(ctx context.Context, tx, ty, tz float64) error {
	if err := a.MoveTo(ctx, tx, ty, tz, false); err != nil {
		return err
	}
	return a.stabilizeSneaking(ctx)
}

// LineToAndSneak moves in a straight line to a position and then starts sneaking.
// This is useful for ending movement on ladders or edges where sneaking holds position.
func (a *agent) LineToAndSneak(ctx context.Context, tx, ty, tz float64) error {
	if err := a.LineTo(ctx, tx, ty, tz, false); err != nil {
		return err
	}
	return a.stabilizeSneaking(ctx)
}

// MoveUpAndSneak moves the bot vertically and then starts sneaking to hold position.
// This is useful for climbing ladders where the agent needs to stay at the destination.
// The sneak is started before the final position update and maintained with continued
// position packets to prevent falling.
func (a *agent) MoveUpAndSneak(ctx context.Context, dist float64) error {
	if a.moveExec == nil {
		return errors.New("movement executor not available")
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return errors.New("position not initialized")
	}

	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond
	steps := int(math.Ceil(math.Abs(dist) / stepSize))
	targetY := y + dist

	// Move up to the destination
	for i := range steps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		prog := float64(i+1) / float64(steps)
		if prog > 1 {
			prog = 1
		}
		ny := y + dist*prog

		// Start sneaking a few steps before the end to ensure we're sneaking
		// when we reach the destination
		if i >= steps-3 && !a.moveExec.IsSneaking() {
			if err := a.moveExec.StartSneaking(); err != nil {
				return err
			}
		}

		if err := a.moveExec.SendPosition(x, ny, z, true); err != nil {
			return err
		}
		if err := sleepWithContext(ctx, stepDelay); err != nil {
			return err
		}
	}

	// Continue sending position updates while sneaking to stabilize
	// This prevents the server from applying gravity before the sneak takes effect
	// The executor automatically sends sneak packets with each position update when sneaking
	const stabilizationTicks = 20
	for i := 0; i < stabilizationTicks; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := a.moveExec.SendPosition(x, targetY, z, true); err != nil {
			return err
		}
		if err := sleepWithContext(ctx, stepDelay); err != nil {
			return err
		}
	}

	return nil
}

// StopSneaking stops the sneaking state.
func (a *agent) StopSneaking() error {
	if a.moveExec != nil {
		return a.moveExec.StopSneaking()
	}
	return nil
}

// StartSneaking starts sneaking.
func (a *agent) StartSneaking() error {
	if a.moveExec != nil {
		return a.moveExec.StartSneaking()
	}
	return nil
}

// FindPath computes a path without moving.
func (a *agent) FindPath(ctx context.Context, tx, ty, tz float64) error {
	if a.pathfind == nil {
		return errors.New("pathfinding not available")
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return errors.New("position not initialized")
	}
	start := models.V3{X: x, Y: y, Z: z}
	goal := models.V3{X: tx, Y: ty, Z: tz}
	maxSteps := int(start.DistanceTo(goal) * 150)
	if maxSteps < 10000 {
		maxSteps = 10000
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	path, err := a.pathfind.FindPath(ctx, start, goal, maxSteps)
	if err != nil {
		return err
	}
	if !path.Found {
		return errors.New("no path found")
	}
	return nil
}

// LookAt rotates the bot's head to face a target position (head only, body stays in place).
func (a *agent) LookAt(ctx context.Context, x, y, z float64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if a.moveExec == nil {
		return errors.New("movement executor not available")
	}
	return a.moveExec.LookAt(x, y, z, true)
}

// TurnTowards rotates the bot's body to face a target position (entire body turns).
func (a *agent) TurnTowards(ctx context.Context, x, y, z float64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if a.moveExec == nil {
		return errors.New("movement executor not available")
	}

	// Get current position
	botX, botY, botZ, _, _, initialized := a.GetPosition()
	if !initialized {
		return errors.New("bot position not initialized")
	}

	// Use physics package to calculate yaw consistently with arrow firing
	// CRITICAL: Use arrow spawn height (1.52), not eye height (1.62)
	// This ensures pitch calculation matches physics system expectations
	botOrigin := models.V3{X: botX, Y: botY + a.getEyeHeight() - .1, Z: botZ}
	targetPos := models.V3{X: x, Y: y, Z: z}

	// Calculate yaw using physics formula: atan2(dZ, dX) - 90
	// This matches YawForStartTarget and ensures consistency
	yaw := float32(physics.YawForStartTarget(botOrigin, targetPos))

	// Calculate pitch based on arrow spawn height (1.52, not 1.62)
	// Arrow spawns at: eye - 0.1 = (standing height 1.62) - 0.1 = 1.52
	dy := y - (botY + a.getEyeHeight() - .1)
	dx := x - botX
	dz := z - botZ
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	pitch := float32(-math.Atan2(dy, horizontalDist) * 180 / math.Pi)

	// Send position and rotation to update body orientation
	return a.moveExec.SendPositionAndRotation(botX, botY, botZ, yaw, pitch, true)
}

// Follow starts following a target player by name.
func (a *agent) Follow(ctx context.Context, target string) error {
	_ = ctx
	a.followingMu.RLock()
	fm := a.followMgr
	a.followingMu.RUnlock()
	if fm == nil {
		return errors.New("follow system not available")
	}
	return fm.Start(target)
}

// StopFollow stops any active follow behavior.
func (a *agent) StopFollow(ctx context.Context) error {
	_ = ctx
	a.followingMu.RLock()
	fm := a.followMgr
	a.followingMu.RUnlock()
	if fm == nil {
		return errors.New("follow system not available")
	}
	if !fm.IsActive() {
		return nil
	}
	return fm.Stop()
}

// FollowStatus returns the follow manager status string.
func (a *agent) FollowStatus(ctx context.Context) string {
	select {
	case <-ctx.Done():
		return "Context cancelled"
	default:
	}
	a.followingMu.RLock()
	fm := a.followMgr
	a.followingMu.RUnlock()
	if fm == nil {
		return "Follow system not available"
	}
	return fm.GetStatus()
}

// ChatEvents exposes chat message events.
func (a *agent) ChatEvents(ctx context.Context) <-chan string {
	// Context parameter allows caller to respect cancellation
	// ChatEvents itself returns a channel; the caller is responsible for
	// listening on both the channel and ctx.Done()
	return a.chatEvents
}

// MoveTo performs pathfinding-based movement to the target.
func (a *agent) MoveTo(ctx context.Context, tx, ty, tz float64, notifyChat bool) error {
	if a.pathfind == nil {
		return errors.New("pathfinding not available")
	}

	if a.moveExec == nil {
		return errors.New("movement executor not available")
	}

	x, y, z, ok := a.GetPositionSimple()
	if !ok {
		return errors.New("position not initialized")
	}

	finalGoal := models.V3{
		X: math.Floor(tx),
		Y: math.Floor(ty),
		Z: math.Floor(tz),
	}
	currentPos := models.V3{
		X: math.Floor(x),
		Y: math.Floor(y),
		Z: math.Floor(z),
	}

	if currentPos == finalGoal {
		if notifyChat {
			_ = a.SendChat("Already at target position")
		}
		log.Printf("[MoveTo] Already at target position")
		return nil
	}

	if notifyChat {
		_ = a.SendChat(fmt.Sprintf("Navigating from (%.0f, %.0f, %.0f) to (%.0f, %.0f, %.0f)", x, y, z, finalGoal.X, finalGoal.Y, finalGoal.Z))
	}

	log.Printf("[MoveTo] Pathfinding from (%.0f, %.0f, %.0f) to (%.0f, %.0f, %.0f)",
		currentPos.X, currentPos.Y, currentPos.Z, finalGoal.X, finalGoal.Y, finalGoal.Z)

	// Use HPA* to find and follow the full path directly
	if err := a.pathfindAndFollow(ctx, currentPos, finalGoal); err != nil {
		if notifyChat {
			_ = a.SendChat(fmt.Sprintf("pathfindAndFollow - Pathfinding failed: %v", err.Error()))
			return fmt.Errorf("pathfinding failed: %w", err)
		}
	}

	if notifyChat {
		_ = a.SendChat(fmt.Sprintf("Arrived at (%.0f, %.0f, %.0f)", finalGoal.X, finalGoal.Y, finalGoal.Z))
	}
	return nil

}

// LineTo performs straight-line movement (for flat worlds only).
func (a *agent) LineTo(ctx context.Context, tx, ty, tz float64, notifyChat bool) error {
	if a.moveExec == nil {
		return errors.New("movement executor not available")
	}

	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return errors.New("position not initialized")
	}
	dx, dy, dz := tx-x, ty-y, tz-z
	total := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if total == 0 {
		return nil
	}

	if err := a.moveExec.LookAt(tx, ty, tz, true); err != nil {
		return err
	}

	const stepSize = 0.2
	const stepDelay = 50 * time.Millisecond
	steps := int(math.Ceil(total / stepSize))
	if steps == 0 {
		return nil
	}
	for i := range steps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		prog := float64(i+1) / float64(steps)
		if prog > 1 {
			prog = 1
		}
		nx, ny, nz := x+dx*prog, y+dy*prog, z+dz*prog
		if err := a.moveExec.SendPosition(nx, ny, nz, true); err != nil {
			return err
		}
		if err := sleepWithContext(ctx, stepDelay); err != nil {
			return err
		}
	}

	if notifyChat {
		_ = a.SendChat(fmt.Sprintf("Arrived at (%.2f, %.2f, %.2f)", tx, ty, tz))
	}
	return nil
}

// blockInteractionSetup performs the common setup for block interactions.
// Returns the hit point, block coordinates, and cursor coordinates relative to the block.
// Returns an error if the context is cancelled or line of sight cannot be established.
func (a *agent) blockInteractionSetup(ctx context.Context, x, y, z float64) (hitX, hitY, hitZ, blockX, blockY, blockZ, cursorX, cursorY, cursorZ float32, err error) {
	if ctx.Err() != nil {
		err = ctx.Err()
		return
	}

	canAccess, hitPtX, hitPtY, hitPtZ, err := a.hasLineOfSightForAccess(ctx, x, y, z)
	if err != nil {
		return
	}
	if !canAccess {
		err = errors.New("no line of sight to block")
		return
	}

	// Calculate block coordinates (floored from world position)
	blkX := math.Floor(x)
	blkY := math.Floor(y)
	blkZ := math.Floor(z)

	// Calculate cursor coordinates relative to block (0-1 within the block)
	crsX := float32(clampFloat64(hitPtX-blkX, 0, 1))
	crsY := float32(clampFloat64(hitPtY-blkY, 0, 1))
	crsZ := float32(clampFloat64(hitPtZ-blkZ, 0, 1))

	return float32(hitPtX), float32(hitPtY), float32(hitPtZ),
		float32(blkX), float32(blkY), float32(blkZ),
		crsX, crsY, crsZ, nil
}

// OpenContainerAt opens a container at the specified position.
func (a *agent) OpenContainerAt(ctx context.Context, x, y, z float64, face models.BlockFace, timeout time.Duration) (byte, error) {
	_, _, _, _, _, _, cursorX, cursorY, cursorZ, err := a.blockInteractionSetup(ctx, x, y, z)
	if err != nil {
		return 0, err
	}
	return a.OpenContainer(models.V3{X: x, Y: y, Z: z}, models.BlockFace(face), timeout, cursorX, cursorY, cursorZ)
}

// UseItemOnBlock uses the held item on a block.
func (a *agent) UseItemOnBlock(ctx context.Context, x, y, z float64, face models.BlockFace, hand models.Hand) error {
	_, _, _, _, _, _, cursorX, cursorY, cursorZ, err := a.blockInteractionSetup(ctx, x, y, z)
	if err != nil {
		return err
	}
	usage, err := a.itemUsageOrCreate()
	if err != nil {
		return err
	}
	return usage.UseItemOnBlockWithCursor(models.V3{X: x, Y: y, Z: z}, face, hand, cursorX, cursorY, cursorZ)
}

// UseItemOnEntity uses the held item on an entity.
func (a *agent) UseItemOnEntity(ctx context.Context, entityID int32, hand models.Hand, sneaking bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	usage, err := a.itemUsageOrCreate()
	if err != nil {
		return err
	}
	return usage.UseItemOnEntity(entityID, hand, sneaking)
}

// HasLineOfSight checks if the agent can see the target position.
func (a *agent) HasLineOfSight(ctx context.Context, tx, ty, tz float64) (bool, error) {
	world := a.GetWorld()
	if world == nil {
		return false, errors.New("world not available")
	}
	if a.blockMgr == nil {
		return false, errors.New("block manager not available")
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return false, errors.New("position not initialized")
	}
	ox := x
	oy := y + a.getEyeHeight()
	oz := z
	dx := tx - ox
	dy := ty - oy
	dz := tz - oz
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist == 0 {
		return true, nil
	}
	dirX := dx / dist
	dirY := dy / dist
	dirZ := dz / dist

	ix := math.Floor(ox)
	iy := math.Floor(oy)
	iz := math.Floor(oz)

	tMaxX, tDeltaX := initialRayStep(ox, dirX, int(ix))
	tMaxY, tDeltaY := initialRayStep(oy, dirY, int(iy))
	tMaxZ, tDeltaZ := initialRayStep(oz, dirZ, int(iz))

	maxSteps := int(dist*3) + 8
	for stepCount := 0; stepCount < maxSteps; stepCount++ {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if stepCount > 0 {
			blocked, err := a.blockOccludesRay(ctx, int(ix), int(iy), int(iz), ox, oy, oz, dirX, dirY, dirZ, dist)
			if err != nil {
				return false, err
			}
			if blocked {
				return false, nil
			}
		}

		nextT := minFloat64(tMaxX, tMaxY, tMaxZ)
		if nextT > dist {
			break
		}
		if tMaxX <= tMaxY && tMaxX <= tMaxZ {
			ix += stepSign(dirX)
			tMaxX += tDeltaX
		} else if tMaxY <= tMaxX && tMaxY <= tMaxZ {
			iy += stepSign(dirY)
			tMaxY += tDeltaY
		} else {
			iz += stepSign(dirZ)
			tMaxZ += tDeltaZ
		}
	}

	return true, nil
}

// FindVisibleEntity returns the nearest visible entity of the given type.
func (a *agent) FindVisibleEntity(ctx context.Context, entityTypeID int32, maxDistance float64) (int32, float64, float64, float64, bool, error) {
	entities := a.GetTrackedEntities()
	if len(entities) == 0 {
		return 0, 0, 0, 0, false, nil
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return 0, 0, 0, 0, false, errors.New("position not initialized")
	}
	bestDist := math.MaxFloat64
	var bestID int32
	var bestX, bestY, bestZ float64
	for _, ent := range entities {
		if ent.Removed || ent.EntityType != entityTypeID {
			continue
		}
		d := distance3D(x, y, z, ent.X, ent.Y, ent.Z)
		if maxDistance > 0 && d > maxDistance {
			continue
		}
		visible, err := a.HasLineOfSight(ctx, ent.X, ent.Y, ent.Z)
		if err != nil {
			return 0, 0, 0, 0, false, err
		}
		if !visible {
			continue
		}
		if d < bestDist {
			bestDist = d
			bestID = ent.EntityID
			bestX, bestY, bestZ = ent.X, ent.Y, ent.Z
		}
	}
	if bestDist == math.MaxFloat64 {
		return 0, 0, 0, 0, false, nil
	}
	return bestID, bestX, bestY, bestZ, true, nil
}

// FindVisibleBlock searches for the nearest visible block by name.
func (a *agent) FindVisibleBlock(ctx context.Context, blockName string, maxDistance int) (float64, float64, float64, bool, error) {
	world := a.GetWorld()
	if world == nil {
		return 0, 0, 0, false, errors.New("world not available")
	}
	if a.blockMgr == nil {
		return 0, 0, 0, false, errors.New("block manager not available")
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return 0, 0, 0, false, errors.New("position not initialized")
	}
	if maxDistance <= 0 {
		maxDistance = 8
	}
	targetName := strings.ToLower(blockName)
	bestDist := math.MaxFloat64
	var bestX, bestY, bestZ float64
	for dx := -maxDistance; dx <= maxDistance; dx++ {
		for dy := -maxDistance; dy <= maxDistance; dy++ {
			for dz := -maxDistance; dz <= maxDistance; dz++ {
				if ctx.Err() != nil {
					return 0, 0, 0, false, ctx.Err()
				}
				cx := math.Floor(x) + float64(dx)
				cy := math.Floor(y) + float64(dy)
				cz := math.Floor(z) + float64(dz)
				stateID, loaded := world.GetBlockAt(cx, cy, cz)
				if !loaded {
					continue
				}
				if stateID == 0 {
					continue
				}
				if blockID, ok := a.blockMgr.BlockIDByStateID(stateID); ok {
					if block, ok := a.blockMgr.GetByID(blockID); ok {
						if strings.ToLower(block.Name) != targetName {
							continue
						}
					}
				}

				visible, _, _, _, err := a.hasLineOfSightForAccess(ctx, cx, cy, cz)
				if err != nil {
					return 0, 0, 0, false, err
				}
				if !visible {
					continue
				}
				d := distance3D(x, y, z, cx+0.5, cy+0.5, cz+0.5)
				if d < bestDist {
					bestDist = d
					bestX, bestY, bestZ = cx, cy, cz
				}
			}
		}
	}
	if bestDist == math.MaxFloat64 {
		return 0, 0, 0, false, nil
	}
	return bestX, bestY, bestZ, true, nil
}

// FindLineOfSightAccessPoint returns a visible point on the target block for access interactions.
func (a *agent) FindLineOfSightAccessPoint(ctx context.Context, x, y, z float64) (float64, float64, float64, bool, error) {
	blockX := math.Floor(x)
	blockY := math.Floor(y)
	blockZ := math.Floor(z)
	visible, hitX, hitY, hitZ, err := a.hasLineOfSightForAccess(ctx, blockX, blockY, blockZ)
	if err != nil {
		return 0, 0, 0, false, err
	}
	if !visible {
		return 0, 0, 0, false, nil
	}
	return hitX, hitY, hitZ, true, nil
}

func (a *agent) MineBlockAt(ctx context.Context, blockPos models.V3, face models.BlockFace) error {
	usage, err := a.itemUsageOrCreate()
	if err != nil {
		return err
	}
	return usage.UseItemOnBlock(blockPos, face, models.MainHand)
}

func (a *agent) itemUsageOrCreate() (*items.ItemUsage, error) {
	if a.itemUsage != nil {
		return a.itemUsage, nil
	}
	if a.client == nil || a.packetMgr == nil {
		return nil, errors.New("item usage not available")
	}
	a.itemUsage = items.NewItemUsage(a.client.Conn(), a.packetMgr)
	// Set version-specific handlers if available
	if a.versionHandler != nil {
		a.itemUsage.SetContainerHandler(a.versionHandler.Play().Containers())
		a.itemUsage.SetActionHandler(a.versionHandler.Play().Actions())
		a.itemUsage.SetEntityHandler(a.versionHandler.Play().Entities())
	}
	return a.itemUsage, nil
}

func isAirBlockName(name string) bool {
	switch strings.ToLower(name) {
	case "minecraft:air", "minecraft:cave_air", "minecraft:void_air":
		return true
	default:
		return false
	}
}

func isSeeThroughBlockName(name string) bool {
	n := strings.ToLower(name)
	if strings.Contains(n, "glass") {
		return true
	}

	if strings.Contains(n, "leaves") || strings.Contains(n, "leaf") {
		return true
	}

	if n == "minecraft:iron_bars" {
		return true
	}

	if strings.Contains(n, "torch") {
		return true
	}

	if strings.Contains(n, "flower") {
		return true
	}

	if strings.Contains(n, "sapling") {
		return true
	}

	if strings.Contains(n, "fence") {
		return true
	}
	return false
}

func isOpenPassThroughBlock(name string, props map[string]string) bool {
	if props == nil {
		return false
	}
	if props["open"] != "true" {
		return false
	}
	n := strings.ToLower(name)
	if strings.Contains(n, "door") {
		return true
	}
	if strings.Contains(n, "trapdoor") {
		return true
	}
	if strings.Contains(n, "gate") {
		return true
	}
	return false
}

func distance3D(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	dz := z1 - z2
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func (a *agent) hasLineOfSightForAccess(ctx context.Context, targetX, targetY, targetZ float64) (bool, float64, float64, float64, error) {
	world := a.GetWorld()
	if world == nil {
		return false, 0, 0, 0, errors.New("world not available")
	}
	if a.blockMgr == nil {
		return false, 0, 0, 0, errors.New("block manager not available")
	}
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return false, 0, 0, 0, errors.New("position not initialized")
	}
	ox := x
	oy := y + a.getEyeHeight()
	oz := z
	// Convert target position to block coordinates using floor division to handle
	// both integer and float inputs correctly. This ensures that:
	// - Integer block coordinates (5, 3, 7) work correctly
	// - Float center coordinates (5.5, 3.5, 7.5) map to the correct block
	blockX := int(math.Floor(targetX))
	blockY := int(math.Floor(targetY))
	blockZ := int(math.Floor(targetZ))
	stateID, loaded := world.GetBlockAt(float64(blockX)+0.5, float64(blockY)+0.5, float64(blockZ)+0.5)
	if !loaded {
		return false, 0, 0, 0, fmt.Errorf("chunk not loaded at target position")
	}

	// If target is air (stateID == 0), check LOS directly to the target point
	// This allows firing projectiles at empty space as long as there's line of sight
	if stateID == 0 {
		visible, err := a.hasLineOfSightForAccessToPoint(ctx, targetX, targetY, targetZ, ox, oy, oz, targetX, targetY, targetZ)
		if err != nil {
			return false, 0, 0, 0, err
		}
		if visible {
			return true, targetX, targetY, targetZ, nil
		}
		a.logLineOfSightFailure(ctx, ox, oy, oz, blockX, blockY, blockZ)
		return false, 0, 0, 0, nil
	}

	points := a.blockSurfaceSamplePoints(stateID, blockX, blockY, blockZ)
	if len(points) == 0 {
		a.logLineOfSightFailure(ctx, ox, oy, oz, blockX, blockY, blockZ)
		return false, 0, 0, 0, nil
	}
	for _, pt := range points {
		visible, err := a.hasLineOfSightForAccessToPoint(ctx, targetX, targetY, targetZ, ox, oy, oz, pt.X, pt.Y, pt.Z)
		if err != nil {
			return false, 0, 0, 0, err
		}
		if visible {
			return true, pt.X, pt.Y, pt.Z, nil
		}
	}
	a.logLineOfSightFailure(ctx, ox, oy, oz, blockX, blockY, blockZ)
	return false, 0, 0, 0, nil
}

func (a *agent) logLineOfSightFailure(ctx context.Context, ox, oy, oz float64, targetX, targetY, targetZ int) {
	const extraBlocks = 2
	blocks := a.collectLineOfSightBlocks(ctx, ox, oy, oz, targetX, targetY, targetZ, extraBlocks)
	log.Printf("[LOS] failed from (%.2f, %.2f, %.2f) to block (%d, %d, %d); listing %d blocks (+%d past target)", ox, oy, oz, targetX, targetY, targetZ, len(blocks), extraBlocks)
	for _, block := range blocks {
		log.Printf("[LOS]   (%d, %d, %d) %s", block.x, block.y, block.z, block.name)
	}
	line := utils.Line(models.V3{X: ox, Y: oy, Z: oz}, models.V3{X: float64(targetX) + 0.5, Y: float64(targetY) + 0.5, Z: float64(targetZ) + 0.5})
	log.Printf("[LOS] line points: %v", line)
}

type losBlock struct {
	x    int
	y    int
	z    int
	name string
}

func (a *agent) collectLineOfSightBlocks(ctx context.Context, ox, oy, oz float64, targetX, targetY, targetZ int, extraBlocks int) []losBlock {
	tx := float64(targetX) + 0.5
	ty := float64(targetY) + 0.5
	tz := float64(targetZ) + 0.5
	dx := tx - ox
	dy := ty - oy
	dz := tz - oz
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist == 0 {
		return []losBlock{{x: targetX, y: targetY, z: targetZ, name: a.BlockNameAt(targetX, targetY, targetZ)}}
	}
	dirX := dx / dist
	dirY := dy / dist
	dirZ := dz / dist

	ix := int(math.Floor(ox))
	iy := int(math.Floor(oy))
	iz := int(math.Floor(oz))

	tMaxX, tDeltaX := initialRayStep(ox, dirX, ix)
	tMaxY, tDeltaY := initialRayStep(oy, dirY, iy)
	tMaxZ, tDeltaZ := initialRayStep(oz, dirZ, iz)

	blocks := make([]losBlock, 0, int(dist)+extraBlocks+4)
	reachedTarget := false
	remainingExtra := 0
	maxSteps := int(dist*3) + extraBlocks + 16

	for range maxSteps {
		if ctx.Err() != nil {
			break
		}
		blocks = append(blocks, losBlock{x: ix, y: iy, z: iz, name: a.BlockNameAt(ix, iy, iz)})
		if ix == targetX && iy == targetY && iz == targetZ && !reachedTarget {
			reachedTarget = true
			remainingExtra = extraBlocks
			if remainingExtra == 0 {
				break
			}
		} else if reachedTarget {
			remainingExtra--
			if remainingExtra <= 0 {
				break
			}
		}

		nextT := minFloat64(tMaxX, tMaxY, tMaxZ)
		if nextT > dist && !reachedTarget {
			break
		}
		if tMaxX <= tMaxY && tMaxX <= tMaxZ {
			ix += int(stepSign(dirX))
			tMaxX += tDeltaX
		} else if tMaxY <= tMaxX && tMaxY <= tMaxZ {
			iy += int(stepSign(dirY))
			tMaxY += tDeltaY
		} else {
			iz += int(stepSign(dirZ))
			tMaxZ += tDeltaZ
		}
	}

	return blocks
}

func (a *agent) BlockNameAt(ix, iy, iz int) string {
	world := a.GetWorld()
	if world == nil || a.blockMgr == nil {
		return "unknown"
	}
	stateID, loaded := world.GetBlockAt(float64(ix)+0.5, float64(iy)+0.5, float64(iz)+0.5)
	if !loaded {
		return fmt.Sprintf("<chunk not loaded>(%d,%d,%d)", ix, iy, iz)
	}
	if stateID == 0 {
		return "minecraft:air"
	}
	if blockID, ok := a.blockMgr.BlockIDByStateID(stateID); ok {
		if block, ok := a.blockMgr.GetByID(blockID); ok {
			return block.Name
		}
	}
	return fmt.Sprintf("unknown(%d %d %d stateID=%d)", ix, iy, iz, stateID)
}

func (a *agent) hasLineOfSightForAccessToPoint(ctx context.Context, targetX, targetY, targetZ float64, ox, oy, oz, tx, ty, tz float64) (bool, error) {
	dx := tx - ox
	dy := ty - oy
	dz := tz - oz
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist == 0 {
		return true, nil
	}
	// Targets within 1 block are always considered visible
	// This avoids precision issues with raycasting at sub-block distances
	if dist < 1.0 {
		return true, nil
	}
	dirX := dx / dist
	dirY := dy / dist
	dirZ := dz / dist

	// Check if agent and target are in the same block
	agentBlockX := int(math.Floor(ox))
	agentBlockY := int(math.Floor(oy))
	agentBlockZ := int(math.Floor(oz))
	targetBlockX := int(math.Floor(targetX))
	targetBlockY := int(math.Floor(targetY))
	targetBlockZ := int(math.Floor(targetZ))
	if agentBlockX == targetBlockX && agentBlockY == targetBlockY && agentBlockZ == targetBlockZ {
		return true, nil // Same block, always visible
	}

	ix := math.Floor(ox)
	iy := math.Floor(oy)
	iz := math.Floor(oz)

	tMaxX, tDeltaX := initialRayStep(ox, dirX, int(ix))
	tMaxY, tDeltaY := initialRayStep(oy, dirY, int(iy))
	tMaxZ, tDeltaZ := initialRayStep(oz, dirZ, int(iz))

	maxSteps := int(dist*3) + 8
	for stepCount := range maxSteps {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if ix == targetX && iy == targetY && iz == targetZ {
			return true, nil
		}
		if stepCount > 0 {
			blocked, err := a.blockOccludesRayAccess(ctx, ix, iy, iz, ox, oy, oz, dirX, dirY, dirZ, dist)
			if err != nil {
				return false, err
			}
			if blocked {
				return false, nil
			}
		}

		nextT := minFloat64(tMaxX, tMaxY, tMaxZ)
		if nextT > dist {
			break
		}
		if tMaxX <= tMaxY && tMaxX <= tMaxZ {
			ix += stepSign(dirX)
			tMaxX += tDeltaX
		} else if tMaxY <= tMaxX && tMaxY <= tMaxZ {
			iy += stepSign(dirY)
			tMaxY += tDeltaY
		} else {
			iz += stepSign(dirZ)
			tMaxZ += tDeltaZ
		}
	}

	return true, nil
}

func (a *agent) blockSurfaceSamplePoints(stateID uint32, blockX, blockY, blockZ int) []models.V3 {
	boxes := []models.AABB{}
	if a.shapeMgr != nil {
		boxes = a.shapeMgr.GetCollisionBoxes(stateID, blockX, blockY, blockZ)
	}
	if len(boxes) == 0 {
		boxes = []models.AABB{
			models.NewAABB(
				float64(blockX),
				float64(blockY),
				float64(blockZ),
				float64(blockX+1),
				float64(blockY+1),
				float64(blockZ+1),
			),
		}
	}
	const samplesPerAxis = 3
	points := make([]models.V3, 0, len(boxes)*samplesPerAxis*samplesPerAxis*6)
	for _, box := range boxes {
		points = append(points, surfacePointsForBox(box, samplesPerAxis)...)
	}
	return points
}

func surfacePointsForBox(box models.AABB, samples int) []models.V3 {
	xVals := sampleAxis(box.X.Min, box.X.Max, samples)
	yVals := sampleAxis(box.Y.Min, box.Y.Max, samples)
	zVals := sampleAxis(box.Z.Min, box.Z.Max, samples)

	points := make([]models.V3, 0, samples*samples*6)
	for _, y := range yVals {
		for _, z := range zVals {
			points = append(points, models.V3{X: box.X.Min, Y: y, Z: z})
			points = append(points, models.V3{X: box.X.Max, Y: y, Z: z})
		}
	}
	for _, x := range xVals {
		for _, z := range zVals {
			points = append(points, models.V3{X: x, Y: box.Y.Min, Z: z})
			points = append(points, models.V3{X: x, Y: box.Y.Max, Z: z})
		}
	}
	for _, x := range xVals {
		for _, y := range yVals {
			points = append(points, models.V3{X: x, Y: y, Z: box.Z.Min})
			points = append(points, models.V3{X: x, Y: y, Z: box.Z.Max})
		}
	}
	return points
}

func sampleAxis(min, max float64, samples int) []float64 {
	if samples <= 1 || max-min <= 1e-9 {
		return []float64{(min + max) / 2}
	}
	span := max - min
	eps := math.Min(0.001, span/4)
	step := span / float64(samples-1)
	values := make([]float64, samples)
	for i := 0; i < samples; i++ {
		v := min + step*float64(i)
		if v < min+eps {
			v = min + eps
		}
		if v > max-eps {
			v = max - eps
		}
		values[i] = v
	}
	return values
}

func (a *agent) blockOccludesRayAccess(_ context.Context, ix, iy, iz float64, ox, oy, oz, dx, dy, dz, maxDist float64) (bool, error) {
	world := a.GetWorld()
	stateID, loaded := world.GetBlockAt(float64(ix)+0.5, float64(iy)+0.5, float64(iz)+0.5)
	if !loaded {
		return false, fmt.Errorf("chunk not loaded at position (%d, %d, %d)", int(ix), int(iy), int(iz))
	}
	if stateID == 0 {
		return false, nil
	}
	if blockID, ok := a.blockMgr.BlockIDByStateID(stateID); ok {
		if block, ok := a.blockMgr.GetByID(blockID); ok {
			blockName := block.Name
			if isAirBlockName(blockName) {
				return false, nil
			}

			props := map[string]string{}
			if a.stateProps != nil {
				props = a.stateProps.GetProperties(stateID)
			}
			if a.shapeMgr == nil {
				if isOpenPassThroughBlock(blockName, props) {
					return false, nil
				}
				return true, nil
			}
		}
	}
	boxes := a.shapeMgr.GetCollisionBoxes(stateID, int(ix), int(iy), int(iz))
	if len(boxes) == 0 {
		return false, nil
	}
	for _, box := range boxes {
		if rayIntersectsAABB(ox, oy, oz, dx, dy, dz, maxDist, box) {
			return true, nil
		}
	}
	return false, nil
}

func (a *agent) blockOccludesRay(_ context.Context, ix, iy, iz int, ox, oy, oz, dx, dy, dz, maxDist float64) (bool, error) {
	world := a.GetWorld()
	stateID, loaded := world.GetBlockAt(float64(ix)+0.5, float64(iy)+0.5, float64(iz)+0.5)
	if !loaded {
		return false, fmt.Errorf("chunk not loaded at position (%d, %d, %d)", ix, iy, iz)
	}
	if stateID == 0 {
		return false, nil
	}
	if blockID, ok := a.blockMgr.BlockIDByStateID(stateID); ok {
		if block, ok := a.blockMgr.GetByID(blockID); ok {
			blockName := block.Name
			if isAirBlockName(blockName) {
				return false, nil
			}
			if isSeeThroughBlockName(blockName) {
				return false, nil
			}

			props := map[string]string{}
			if a.stateProps != nil {
				props = a.stateProps.GetProperties(stateID)
			}
			if a.shapeMgr == nil {
				if isOpenPassThroughBlock(blockName, props) {
					return false, nil
				}
				return true, nil
			}
		}
	}

	boxes := a.shapeMgr.GetCollisionBoxes(stateID, ix, iy, iz)
	if len(boxes) == 0 {
		return false, nil
	}
	for _, box := range boxes {
		if rayIntersectsAABB(ox, oy, oz, dx, dy, dz, maxDist, box) {
			return true, nil
		}
	}
	return false, nil
}

func rayIntersectsAABB(ox, oy, oz, dx, dy, dz, maxDist float64, box models.AABB) bool {
	tmin := 0.0
	tmax := maxDist

	if !raySlab(ox, dx, box.X.Min, box.X.Max, &tmin, &tmax) {
		return false
	}
	if !raySlab(oy, dy, box.Y.Min, box.Y.Max, &tmin, &tmax) {
		return false
	}
	if !raySlab(oz, dz, box.Z.Min, box.Z.Max, &tmin, &tmax) {
		return false
	}
	// Check if the intersection is ahead of the ray
	// If tmin <= 0, the ray starts inside or at the block boundary, so it's not blocking
	// If tmin > 0, the block is ahead of the ray starting point and blocks the line of sight
	const epsilon = 0.001
	return tmax >= tmin && tmin > epsilon
}

func raySlab(origin, dir, min, max float64, tmin, tmax *float64) bool {
	if math.Abs(dir) < 1e-9 {
		return origin >= min && origin <= max
	}
	inv := 1.0 / dir
	t1 := (min - origin) * inv
	t2 := (max - origin) * inv
	if t1 > t2 {
		t1, t2 = t2, t1
	}
	if t1 > *tmin {
		*tmin = t1
	}
	if t2 < *tmax {
		*tmax = t2
	}
	return *tmax >= *tmin
}

func initialRayStep(origin, dir float64, cell int) (tMax, tDelta float64) {
	if dir > 0 {
		next := float64(cell+1) - origin
		tMax = next / dir
		tDelta = 1.0 / dir
		return tMax, tDelta
	}
	if dir < 0 {
		next := float64(cell) - origin
		tMax = next / dir
		tDelta = -1.0 / dir
		return tMax, tDelta
	}
	return math.Inf(1), math.Inf(1)
}

func stepSign(v float64) float64 {
	if v > 0 {
		return float64(1)
	}
	if v < 0 {
		return float64(-1)
	}
	return float64(0)
}

func minFloat64(a, b, c float64) float64 {
	if a <= b && a <= c {
		return a
	}
	if b <= a && b <= c {
		return b
	}
	return c
}

func clampFloat64(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
