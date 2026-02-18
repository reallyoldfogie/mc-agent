package agent

import (
	"log"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

const (
	// RenderLoopInterval is the tick interval for the render loop (50ms = 20 ticks/sec)
	RenderLoopInterval = 50 * time.Millisecond
)

// startRenderLoop spawns a goroutine that continuously interpolates projectile positions at 20 ticks per second.
// This mimics the Minecraft client's render loop, allowing accurate position tracking during flight.
func (a *agent) startRenderLoop(ctxDone <-chan struct{}) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(RenderLoopInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctxDone:
				return
			case <-ticker.C:
				a.renderTick()
			}
		}
	}()
}

// renderTick executes one iteration of the render loop.
// For each active projectile, it:
// 1. Updates interpolated position based on server position history
// 2. Detects collisions using the interpolated position
// 3. Fires hit callbacks when collisions are detected
func (a *agent) renderTick() {
	a.activeProjectilesMu.Lock()
	defer a.activeProjectilesMu.Unlock()

	now := time.Now()

	for entityID, projInfo := range a.activeProjectiles {
		// Skip if already collided
		if projInfo.callbacksFired {
			log.Printf("[renderTick] Projectile %d: skipping (callbacks already fired)", entityID)
			continue
		}
		// Skip if we don't have enough position history to interpolate
		if projInfo.currentServerTime.IsZero() {
			log.Printf("[renderTick] Projectile %d: skipping (no position history, currentServerTime=zero)", entityID)
			continue
		}

		// Determine which interpolation method to use
		useVelocityInterpolation := projInfo.lastServerTime.IsZero()

		if useVelocityInterpolation {
			// No real position updates received yet - use velocity-based interpolation from spawn
			timeElapsedSinceSpawn := now.Sub(projInfo.spawnTime).Seconds()
			if timeElapsedSinceSpawn < 0 {
				// Time went backwards, use spawn position
				projInfo.interpolatedPos = projInfo.spawnPos
				log.Printf("[renderTick] Projectile %d: time went backwards, using spawn pos", entityID)
				continue
			}

			// Use discrete physics simulation to predict projectile position
			// This matches Minecraft's tick-based physics engine
			// Now includes sub-tick interpolation for smooth motion before first tick boundary
			projInfo.interpolatedPos = a.InterpolateProjectilePosition(projInfo, timeElapsedSinceSpawn)

			// Calculate tick information for logging
			totalTicks := timeElapsedSinceSpawn * 20.0
			fullTicks := int(totalTicks)
			subTickFraction := totalTicks - float64(fullTicks)

			if subTickFraction > 0 {
				log.Printf("[renderTick] Projectile %d velocity interpolated: spawn=(%.2f,%.2f,%.2f) → (%.4f, %.4f, %.4f) elapsed=%.3fs, ticks=%.2f (full=%d, sub=%.2f), spawnVel=(%.4f, %.4f, %.4f)",
					entityID, projInfo.spawnPos.X, projInfo.spawnPos.Y, projInfo.spawnPos.Z,
					projInfo.interpolatedPos.X, projInfo.interpolatedPos.Y, projInfo.interpolatedPos.Z, timeElapsedSinceSpawn,
					totalTicks, fullTicks, subTickFraction,
					projInfo.spawnVelocity.X, projInfo.spawnVelocity.Y, projInfo.spawnVelocity.Z)
			} else {
				log.Printf("[renderTick] Projectile %d velocity interpolated: spawn=(%.2f,%.2f,%.2f) → (%.4f, %.4f, %.4f) elapsed=%.3fs, ticks=%d, spawnVel=(%.4f, %.4f, %.4f)",
					entityID, projInfo.spawnPos.X, projInfo.spawnPos.Y, projInfo.spawnPos.Z,
					projInfo.interpolatedPos.X, projInfo.interpolatedPos.Y, projInfo.interpolatedPos.Z, timeElapsedSinceSpawn,
					fullTicks,
					projInfo.spawnVelocity.X, projInfo.spawnVelocity.Y, projInfo.spawnVelocity.Z)
			}
		} else {
			// Skip collision detection until we've received at least one position update
			// (lastServerTime should differ from currentServerTime after first update)
			if projInfo.lastServerTime.Equal(projInfo.currentServerTime) {
				log.Printf("[renderTick] Projectile %d: waiting for first position delta (lastServerTime == currentServerTime)", entityID)
				continue
			}

			// Calculate interpolation factor for server position updates
			timeBetweenUpdates := projInfo.currentServerTime.Sub(projInfo.lastServerTime).Seconds()
			if timeBetweenUpdates <= 0 {
				// No time has passed between updates, use current position
				projInfo.interpolatedPos = projInfo.currentServerPos
				continue
			}

			timeElapsedSinceLastUpdate := now.Sub(projInfo.lastServerTime).Seconds()
			if timeElapsedSinceLastUpdate < 0 {
				// Time went backwards somehow, use current position
				projInfo.interpolatedPos = projInfo.currentServerPos
				continue
			}

			// Calculate interpolation factor (0.0 = at last position, 1.0 = at current position)
			interpolationFactor := timeElapsedSinceLastUpdate / timeBetweenUpdates
			if interpolationFactor > 1.0 {
				interpolationFactor = 1.0 // Clamp to current position if we've gone past the update time
			}

			// Linear interpolation: lerp(lastPos, currentPos, factor)
			projInfo.interpolatedPos.X = projInfo.lastServerPos.X + (projInfo.currentServerPos.X-projInfo.lastServerPos.X)*interpolationFactor
			projInfo.interpolatedPos.Y = projInfo.lastServerPos.Y + (projInfo.currentServerPos.Y-projInfo.lastServerPos.Y)*interpolationFactor
			projInfo.interpolatedPos.Z = projInfo.lastServerPos.Z + (projInfo.currentServerPos.Z-projInfo.lastServerPos.Z)*interpolationFactor

			log.Printf("[renderTick] Projectile %d server interpolated: (%.4f, %.4f, %.4f) factor=%.3f",
				entityID, projInfo.interpolatedPos.X, projInfo.interpolatedPos.Y, projInfo.interpolatedPos.Z, interpolationFactor)
		}

		// For non-persistent projectiles (snowball, egg, etc.), only fire callbacks when removed by server
		// Don't do collision detection in render loop since we don't get position updates
		if !projInfo.projectileType.IsPersistent() {
			// Skip collision detection for non-persistent projectiles
			// They will be handled by onRemoveEntities when server removes them
			continue
		} else {
			// For persistent projectiles (arrows, tridents), try to distinguish hit type
			// Check for block collisions at interpolated position
			if a.checkBlockCollision(projInfo.interpolatedPos) {
				a.fireProjectileCollisionCallback(entityID, projInfo, models.ProjectileHitBlock)
				continue
			}

			// Check for entity collisions at interpolated position
			if hitEntityID, ok := a.checkEntityCollision(entityID, projInfo.interpolatedPos); ok {
				a.fireProjectileCollisionCallback(entityID, projInfo, models.ProjectileHitEntity)
				log.Printf("[renderTick] Projectile %d hit entity %d at (%.4f, %.4f, %.4f)",
					entityID, hitEntityID, projInfo.interpolatedPos.X, projInfo.interpolatedPos.Y, projInfo.interpolatedPos.Z)
			}
		}
	}
}

// checkBlockCollision checks if the interpolated position has collided with a solid block.
// Returns true if a collision is detected, false otherwise.
func (a *agent) checkBlockCollision(pos models.V3) bool {
	if a.worldMgr == nil || a.shapeMgr == nil {
		return false // Can't check without world or shape manager
	}

	// Get block state ID
	stateID, loaded := a.worldMgr.GetBlockAt(pos.X, pos.Y, pos.Z)
	if !loaded {
		// Chunk not loaded, assume no collision
		return false
	}

	// Air blocks (stateID 0) have no collision
	if stateID == 0 {
		return false
	}

	// Check if block is solid (has collision shapes)
	if a.shapeMgr.IsSolid(stateID) {
		log.Printf("[checkBlockCollision] Hit solid block at (%.4f, %.4f, %.4f) stateID=%d",
			pos.X, pos.Y, pos.Z, stateID)
		return true
	}

	return false
}

// checkEntityCollision checks if the projectile at the given position would collide with any entity.
// Returns the entity ID of the hit entity (if any) and a boolean indicating if a collision occurred.
func (a *agent) checkEntityCollision(projectileID int32, pos models.V3) (int32, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	const collisionRadius = 0.6 // Collision radius to match entity bounding box (0.6 x 0.6 x 1.8)

	// Check all tracked entities for collision
	for entityID, entity := range a.entities {
		// Don't collide with self or removed entities
		if entityID == projectileID || entity.Removed {
			continue
		}

		// Calculate distance between projectile and entity
		dx := pos.X - entity.X
		dy := pos.Y - entity.Y
		dz := pos.Z - entity.Z
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

		// Simple sphere collision: check if distance is within collision radius
		// Most entities have a bounding box of 0.6 x 1.8, but for projectiles we use a simple sphere
		if distance < collisionRadius {
			log.Printf("[checkEntityCollision] Projectile at (%.4f, %.4f, %.4f) collided with entity %d at (%.4f, %.4f, %.4f) distance=%.4f",
				pos.X, pos.Y, pos.Z, entityID, entity.X, entity.Y, entity.Z, distance)
			return entityID, true
		}
	}

	return 0, false
}

// fireProjectileCollisionCallback handles collision callbacks for projectiles.
// For persistent projectiles (arrows, tridents), queues callback for server-authoritative position.
// For non-persistent projectiles, fires immediately with client prediction.
func (a *agent) fireProjectileCollisionCallback(projectileID int32, projInfo *activeProjectileInfo, hitType models.ProjectileHitType) {
	if projInfo.callbacksFired || len(projInfo.callbacks) == 0 {
		return // Already fired or no callbacks registered
	}

	// For persistent projectiles (arrows, tridents), queue callback for server position confirmation
	// instead of firing immediately with client prediction
	if projInfo.projectileType.IsPersistent() {
		projInfo.pendingCallbackFire = true
		projInfo.pendingHitType = hitType
		projInfo.pendingHitPos = projInfo.interpolatedPos
		projInfo.collisionDetectTime = time.Now()
		log.Printf("[fireProjectileCollisionCallback] QUEUED callback (waiting for server position): projectileID=%d, type=%s, hitType=%v, count=%d, clientPos=(%.2f, %.2f, %.2f)",
			projectileID, projInfo.projectileType, hitType, len(projInfo.callbacks), projInfo.interpolatedPos.X, projInfo.interpolatedPos.Y, projInfo.interpolatedPos.Z)
		return
	}

	// For non-persistent projectiles (snowball, egg, etc.), fire immediately with client prediction
	// We don't get server position updates for these, so client prediction is all we have
	evt := models.ProjectileHitEvent{
		HitType:        hitType,
		ProjectileType: projInfo.projectileType,
		Position:       projInfo.interpolatedPos,
	}
	for _, cb := range projInfo.callbacks {
		go cb(evt) // Fire asynchronously
	}

	projInfo.callbacksFired = true
	log.Printf("[fireProjectileCollisionCallback] Fired projectile collision callbacks (non-persistent): projectileID=%d, type=%s, hitType=%v, count=%d, pos=(%.2f, %.2f, %.2f)",
		projectileID, projInfo.projectileType, hitType, len(projInfo.callbacks), projInfo.interpolatedPos.X, projInfo.interpolatedPos.Y, projInfo.interpolatedPos.Z)
}
