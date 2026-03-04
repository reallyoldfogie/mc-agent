package agent

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// trackedEntity mirrors the runtime fields we need to follow players/entities.
type trackedEntity struct {
	EntityID           int32
	EntityType         int32
	UUID               [16]byte
	X, Y, Z            float64
	VelX, VelY, VelZ   float64 // Entity velocity (from velocity update packets)
	Yaw                int8
	Pitch              int8
	HeadYaw            int8    // Head rotation (separate from body)
	Health             float32 // Current health (0 = dead)
	MaxHealth          float32 // Maximum health (typically 20.0 for mobs)
	OnGround           bool    // Whether entity is on ground
	Removed            bool
	RemovedAt          time.Time
	LastMetadataUpdate time.Time // When velocity was last updated (for interpolation reference)
	LastPositionUpdate time.Time // When position was last updated (to check if position is current)
	shake              int8      // Shake animation counter (0-7) for projectiles
	criticalHit        bool      // Critical hit flag for projectiles
	pierceLevel        int8      // Piercing level for projectiles
	potionColor        int32     // Potion effect color for arrows (-1 = no potion)
	// Position interpolation for accurate projectile tracking
	lastServerX, lastServerY, lastServerZ float64   // Previous server position
	lastServerUpdateTime                  time.Time // When lastServer position was received
	currentServerUpdateTime               time.Time // When current (X, Y, Z) position was received
}

// TrackedEntityInfo exposes entity tracking data for external use (tests, following, etc.)
type TrackedEntityInfo = models.TrackedEntityInfo

// GetPosition returns the current bot position and rotation.
func (a *agent) GetPosition() (x, y, z float64, yaw, pitch float32, initialized bool) {
	a.posMu.RLock()
	defer a.posMu.RUnlock()
	return a.posX, a.posY, a.posZ, a.posYaw, a.posPitch, a.posInitialized
}

// setPosition updates the bot position and rotation.
func (a *agent) setPosition(x, y, z float64, yaw, pitch float32) {
	a.posMu.Lock()
	a.posX, a.posY, a.posZ = x, y, z
	a.posYaw, a.posPitch = yaw, pitch
	a.posInitialized = true
	a.posMu.Unlock()
}

// GetPositionSimple returns bot position without rotation.
func (a *agent) GetPositionSimple() (x, y, z float64, initialized bool) {
	a.posMu.RLock()
	defer a.posMu.RUnlock()
	return a.posX, a.posY, a.posZ, a.posInitialized
}

// GetEntityID returns the bot's entity ID.
func (a *agent) GetEntityID() int32 {
	a.entIDMu.RLock()
	defer a.entIDMu.RUnlock()
	return a.entID
}

// setEntityID sets the bot's entity ID.
func (a *agent) setEntityID(id int32) {
	a.entIDMu.Lock()
	a.entID = id
	a.entIDMu.Unlock()
}

// snapshotEntities returns a shallow copy of tracked entities for external consumption/tests.
func (a *agent) snapshotEntities() map[int32]trackedEntity {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	out := make(map[int32]trackedEntity, len(a.entities))
	for id, e := range a.entities {
		out[id] = *e
	}
	return out
}

// startEntityCleanup runs a periodic task to purge soft-deleted entities.
func (a *agent) startEntityCleanup(ctxDone <-chan struct{}) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(EntityCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctxDone:
				return
			case <-ticker.C:
				a.cleanupRemovedEntities()
			}
		}
	}()
}

func (a *agent) cleanupRemovedEntities() {
	now := time.Now()
	a.entitiesMu.Lock()
	for id, e := range a.entities {
		if e.Removed && now.Sub(e.RemovedAt) > EntityRemovalGracePeriod {
			delete(a.entities, id)
		}
	}
	a.entitiesMu.Unlock()

	// Check for expired projectiles and fire timeout callbacks
	a.activeProjectilesMu.Lock()
	for id, projInfo := range a.activeProjectiles {
		// Skip if callbacks already fired
		if projInfo.callbacksFired || len(projInfo.callbacks) == 0 {
			continue
		}

		// Handle pending callbacks that are waiting for server position
		if projInfo.pendingCallbackFire && !projInfo.callbacksFired && len(projInfo.callbacks) > 0 {
			// If server position hasn't arrived within 2 seconds after collision detection, fire with client prediction
			if now.Sub(projInfo.collisionDetectTime) > 2*time.Second {
				evt := models.ProjectileHitEvent{
					ProjectileEntityID: id,
					HitType:            projInfo.pendingHitType,
					ProjectileType:     projInfo.projectileType,
					Position:           projInfo.pendingHitPos, // Use client prediction as fallback
					FiredAt:            projInfo.firedAt,
					HitAt:              now,
					HitResult:          models.ProjectileResultTimeout, // Fired due to server position timeout
				}
				for _, cb := range projInfo.callbacks {
					go cb(evt) // Fire asynchronously
				}
				projInfo.callbacksFired = true
				log.Printf("[cleanupRemovedEntities] Fired pending projectile callbacks with fallback position (server didn't respond within 2s): projectileID=%d, type=%s, count=%d, clientPos=(%.2f, %.2f, %.2f) after %.1fs",
					id, projInfo.projectileType, len(projInfo.callbacks), projInfo.pendingHitPos.X, projInfo.pendingHitPos.Y, projInfo.pendingHitPos.Z, now.Sub(projInfo.collisionDetectTime).Seconds())
			}
		}

		// Handle projectiles with registered callbacks that have exceeded the timeout
		if !projInfo.callbackRegisteredAt.IsZero() && now.Sub(projInfo.callbackRegisteredAt) > projectileCallbackTimeout && !projInfo.callbacksFired {
			// Determine best-effort position
			var pos models.V3
			if !projInfo.currentServerTime.IsZero() {
				pos = projInfo.currentServerPos
			} else if projInfo.interpolatedPos != (models.V3{}) {
				pos = projInfo.interpolatedPos
			} else if projInfo.spawnPos != (models.V3{}) {
				pos = projInfo.spawnPos
			}

			// Determine hit type based on what we know
			hitType := models.ProjectileHitUnknown
			if projInfo.projectileType.IsPersistent() && projInfo.isInGround {
				hitType = models.ProjectileHitBlock
			}

			evt := models.ProjectileHitEvent{
				ProjectileEntityID: id,
				HitType:            hitType,
				ProjectileType:     projInfo.projectileType,
				Position:           pos,
				FiredAt:            projInfo.firedAt,
				HitAt:              now,
				HitResult:          models.ProjectileResultTimeout, // Fired due to callback timeout
			}
			for _, cb := range projInfo.callbacks {
				go cb(evt) // Fire asynchronously
			}
			projInfo.callbacksFired = true
			log.Printf("[cleanupRemovedEntities] Fired projectile callbacks due to callback timeout: projectileID=%d, type=%s, hitType=%v, count=%d, pos=(%.2f, %.2f, %.2f) after %.1fs",
				id, projInfo.projectileType, hitType, len(projInfo.callbacks), pos.X, pos.Y, pos.Z, now.Sub(projInfo.callbackRegisteredAt).Seconds())
			// Remove from active tracking
			delete(a.activeProjectiles, id)
		}
	}
	a.activeProjectilesMu.Unlock()
}

// getTrackedEntitiesForFollowing converts internal entities to following.TrackedEntity map.
func (a *agent) GetTrackedEntitiesForFollowing() map[int32]*models.TrackedEntity {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	out := make(map[int32]*models.TrackedEntity, len(a.entities))
	for id, e := range a.entities {
		out[id] = &models.TrackedEntity{UUID: e.UUID, X: e.X, Y: e.Y, Z: e.Z, Yaw: e.Yaw, Pitch: e.Pitch}
	}
	return out
}

// getInterpolatedPosition calculates entity position by interpolating between the last two server positions.
// This provides accurate position tracking for projectiles by linear interpolation rather than velocity extrapolation.
// Returns the interpolated X, Y, Z coordinates.
func (e *trackedEntity) getInterpolatedPosition() (x, y, z float64) {
	x, y, z = e.X, e.Y, e.Z

	// Timestamp-based linear interpolation between server positions
	// Only works if we have two position updates
	if e.lastServerUpdateTime.IsZero() || e.currentServerUpdateTime.IsZero() {
		// Fallback: use velocity extrapolation if we don't have position history
		if e.LastPositionUpdate.IsZero() || e.LastMetadataUpdate.IsZero() {
			return // No data, use raw position
		}

		// Only interpolate if position was updated AFTER velocity was last set
		if !e.LastPositionUpdate.After(e.LastMetadataUpdate) {
			return // Velocity changed more recently than position, use raw position
		}

		// Calculate time elapsed since last position update (in seconds)
		elapsed := time.Since(e.LastPositionUpdate).Seconds()
		if elapsed <= 0 {
			return // No time has passed since position update, use raw position
		}

		// Add velocity * elapsed time to current position
		x += e.VelX * elapsed
		y += e.VelY * elapsed
		z += e.VelZ * elapsed
		return
	}

	// Linear interpolation between last two server positions
	// Calculate progress from last update toward current update (clamped to 0-1)
	timeBetweenUpdates := e.currentServerUpdateTime.Sub(e.lastServerUpdateTime).Seconds()
	if timeBetweenUpdates <= 0 {
		return // No time has passed between updates, use current position
	}

	timeElapsedSinceLastUpdate := time.Since(e.lastServerUpdateTime).Seconds()
	if timeElapsedSinceLastUpdate <= 0 {
		return // We're before the last update somehow, use current position
	}

	// Calculate interpolation factor (0.0 = at last position, 1.0 = at current position, >1.0 = extrapolating)
	interpolationFactor := timeElapsedSinceLastUpdate / timeBetweenUpdates
	if interpolationFactor > 1.0 {
		interpolationFactor = 1.0 // Clamp to current position if we've gone past the update time
	}

	// Linear interpolation: lerp(lastPos, currentPos, factor)
	x = e.lastServerX + (e.X-e.lastServerX)*interpolationFactor
	y = e.lastServerY + (e.Y-e.lastServerY)*interpolationFactor
	z = e.lastServerZ + (e.Z-e.lastServerZ)*interpolationFactor

	return
}

// GetTrackedEntities returns a snapshot of all tracked entities for external consumption (tests, debugging, etc.)
func (a *agent) GetTrackedEntities() map[int32]TrackedEntityInfo {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	out := make(map[int32]TrackedEntityInfo, len(a.entities))
	for id, e := range a.entities {
		out[id] = TrackedEntityInfo{
			EntityID:   e.EntityID,
			EntityType: e.EntityType,
			UUID:       e.UUID,
			X:          e.X,
			Y:          e.Y,
			Z:          e.Z,
			Yaw:        e.Yaw,
			Pitch:      e.Pitch,
			Health:     e.Health,
			MaxHealth:  e.MaxHealth,
			Removed:    e.Removed,
		}
	}
	return out
}

// FindNearestEntityByType finds the nearest entity of a specific type to a position.
// Returns entityID, distance, and whether found.
// Excludes entities marked as Removed.
func (a *agent) FindNearestEntityByType(entityType int32, x, y, z float64) (int32, float64, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	var nearestID int32
	nearestDist := math.MaxFloat64
	found := false

	for id, e := range a.entities {
		if e.Removed || e.EntityType != entityType {
			continue
		}

		dx := e.X - x
		dy := e.Y - y
		dz := e.Z - z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if dist < nearestDist {
			nearestDist = dist
			nearestID = id
			found = true
		}
	}

	return nearestID, nearestDist, found
}

// FaceEntity makes the bot look at a target entity by calculating and sending rotation.
func (a *agent) FaceEntity(entityID int32) error {
	if a.versionHandler == nil || a.client == nil {
		return fmt.Errorf("version handler or client not initialized")
	}

	// Get current bot position
	botX, botY, botZ, initialized := a.GetPositionSimple()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}

	// Get target entity position
	a.entitiesMu.RLock()
	targetEnt, exists := a.entities[entityID]
	a.entitiesMu.RUnlock()

	if !exists {
		return fmt.Errorf("target entity %d not found", entityID)
	}

	// Calculate eye-level position
	botEyeY := botY + a.getEyeHeight()

	// Calculate deltas
	dx := targetEnt.X - botX
	dy := targetEnt.Y - botEyeY
	dz := targetEnt.Z - botZ

	// Calculate yaw (horizontal rotation)
	// Minecraft yaw uses: yaw = atan2(dx, dz) where both deltas have same sign
	// This gives: 0°=+Z(south), 90°=-X(west), ±180°=-Z(north), -90°=+X(east)
	yaw := math.Atan2(dx, dz) * 180 / math.Pi

	// Calculate pitch (vertical rotation)
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	pitch := math.Atan2(-dy, horizontalDist) * 180 / math.Pi

	// Send rotation packet
	return a.versionHandler.Play().Movement().SendRotation(a.client.Conn(), float32(yaw), float32(pitch), true)
}

// FacePosition makes the bot look at a specific position.
func (a *agent) FacePosition(targetX, targetY, targetZ float64) error {
	if a.versionHandler == nil || a.client == nil {
		return fmt.Errorf("version handler or client not initialized")
	}

	// Get current bot position
	botX, botY, botZ, initialized := a.GetPositionSimple()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}

	// Calculate eye-level position
	botEyeY := botY + a.getEyeHeight()

	// Calculate deltas
	dx := targetX - botX
	dy := targetY - botEyeY
	dz := targetZ - botZ

	// Calculate yaw (horizontal rotation)
	// Minecraft yaw uses: yaw = atan2(dx, dz) where both deltas have same sign
	// This gives: 0°=+Z(south), 90°=-X(west), ±180°=-Z(north), -90°=+X(east)
	yaw := math.Atan2(dx, dz) * 180 / math.Pi

	// Calculate pitch (vertical rotation)
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	pitch := math.Atan2(-dy, horizontalDist) * 180 / math.Pi

	// Send rotation packet
	return a.versionHandler.Play().Movement().SendRotation(a.client.Conn(), float32(yaw), float32(pitch), true)
}
