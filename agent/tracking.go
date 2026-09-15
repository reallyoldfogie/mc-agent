package agent

import (
	"fmt"
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
	// Entity attributes (health, speed, etc.)
	Attributes map[string]models.AttributeValue // Entity attribute base values + modifiers (e.g. "generic.movement_speed"); see GetEntityAttribute for the computed final value
	// Equipment holds the entity's last-seen equipment, keyed by slot, as
	// reported by ClientboundEntityEquipment. Used to tell whether a mount is
	// saddled (1.21.5+, where the saddle became a real equipment slot).
	//
	// Caveat: the version handlers currently leave EquipmentEntry.Item.ItemID
	// at 0 with a TODO, so only occupancy (Count/Present) is trustworthy here,
	// not the item identity. That is enough for saddle detection because the
	// slot itself carries the meaning, but any check that needs to know *which*
	// item is equipped must wait for the parsers to extract the item ID.
	Equipment map[models.EquipmentSlotType]models.InventorySlot
	// Inventory is a cached snapshot of the entity's own container contents
	// (donkey/mule/llama chest, chest boat, chest minecart). Nil until the
	// container has been opened at least once. See models.EntityInventory for
	// the staleness contract.
	Inventory *models.EntityInventory
	// Effects holds the entity's last-seen active status effects, keyed by
	// the full minecraft:mob_effect registry name (e.g.
	// "minecraft:slow_falling"). Populated from ClientboundEntityEffect,
	// cleared per-key on ClientboundRemoveEntityEffect. Nil until the first
	// effect packet for this entity arrives.
	Effects map[string]models.ActiveEffect
	// Boat-specific metadata (only populated for boat/chest-boat entity types)
	BoatVariant     models.BoatVariant // Wood type (oak, spruce, birch, etc.)
	BoatPaddleLeft  bool               // Left paddle turning
	BoatPaddleRight bool               // Right paddle turning
	// Pose tracks the entity's last-seen EntityPose, sourced from the Pose
	// metadata field (handler type 21). PoseName is the lowercased Java enum
	// name (e.g. "standing", "sitting"); Pose is the raw wire ordinal.
	// HasPose is false until the first pose update arrives.
	Pose     int32
	PoseName string
	HasPose  bool
	// HorseFlags is the AbstractHorseEntity flags byte (metadata key 17),
	// carrying tamed/saddled/bred/eating/angry bits. Only populated for
	// saddleable mounts, and only meaningful before 1.21.5 where it is the
	// client's only source of saddled state.
	//
	// HasHorseFlags stays false until an update arrives. Because the server only
	// transmits tracked data that differs from its default, an all-zero flags
	// byte is never sent — so "not seen" and "no flags set" are the same thing,
	// which is exactly how the vanilla client reads it.
	HorseFlags    uint8
	HasHorseFlags bool

	// HappyGhastStayingStill mirrors HappyGhastEntity's STAYING_STILL tracked
	// boolean (metadata key 18). True whenever the ghast has no controlling
	// passenger at all — see models.EntityMetadataKeyHappyGhastStayingStill.
	// HasHappyGhastStayingStill stays false until the server's first update
	// for this field arrives.
	HappyGhastStayingStill    bool
	HasHappyGhastStayingStill bool

	// IsBaby mirrors PassiveEntity's CHILD tracked boolean (metadata key 16,
	// see models.EntityMetadataKeyPassiveChild). Currently only captured for
	// happy ghasts (gated the same way HappyGhastStayingStill is), where it
	// determines standable-surface eligibility: HappyGhastEntity.isCollidable
	// requires !isBaby(). Like HorseFlags, absence of a positive signal is
	// the answer — vanilla's default is "not baby" and an unchanged tracked
	// value is never sent — so HasIsBaby staying false is read as adult, not
	// unknown.
	IsBaby    bool
	HasIsBaby bool

	// FireworkShooterEntityID mirrors FireworkRocketEntity's SHOOTER_ENTITY_ID
	// tracked field (metadata key 9, see
	// models.EntityMetadataKeyFireworkShooterEntityID) — only populated for
	// firework_rocket entity types. Wire encoding is "0=absent, N=entity ID
	// N-1" (see HandlerOptionalInt), already decoded by the time it lands
	// here. HasFireworkShooterEntityID stays false until the first update
	// arrives, matching HorseFlags/IsBaby's "not seen yet" convention.
	FireworkShooterEntityID    int32
	HasFireworkShooterEntityID bool

	// AirSupply mirrors LivingEntity's AIR tracked int (metadata key 8, see
	// models.EntityMetadataKeyAirSupply) - ticks of air remaining while
	// submerged, used to avoid planning/executing a swim route that would
	// drown the bot (see docs/plans/WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item
	// 6). HasAirSupply stays false until the first update arrives; unlike
	// HorseFlags/IsBaby, full air (300) IS a valid, frequently-sent value (the
	// server does resync it), so "not seen" and "full air" are genuinely
	// different here and must not be conflated.
	AirSupply    int32
	HasAirSupply bool
}

// GetPosition returns the current bot position and rotation.
// When mounted, returns a.posX/Y/Z which is updated every tick by the physics
// executor (handleRidingMode → setBotPosition). The entity tracker is NOT used
// while riding because the server typically does not echo the vehicle's position
// back to the rider, making it stale within a tick or two.
func (a *agent) GetPosition() (pos models.V3, yaw, pitch float64, initialized bool) {
	a.posMu.RLock()
	defer a.posMu.RUnlock()
	// Debug, not Info (a.logf): this is by far the single hottest logging
	// call site in the whole codebase — GetPosition is called many times
	// per tick from all over (movement, physics, rlenv's Step/observation
	// building, ...). Logged unconditionally at Info, it alone accounted
	// for over a third of a live RL training run's runaway log volume
	// (2026-09-09) — see utils.VerboseLoggingEnabled.
	a.log().Debug(fmt.Sprintf("[GetPosition %s] Bot position  (%.2f, %.2f, %.2f) with rotation (yaw=%.1f, pitch=%.1f)", a.cfg.Name, a.posX, a.posY, a.posZ, a.posYaw, a.posPitch))

	return models.V3{X: a.posX, Y: a.posY, Z: a.posZ}, a.posYaw, a.posPitch, a.posInitialized
}

// setPosition updates the bot position and rotation.
func (a *agent) setPosition(pos models.V3, yaw, pitch float64) {
	a.posMu.Lock()
	defer a.posMu.Unlock()

	a.posX, a.posY, a.posZ = pos.X, pos.Y, pos.Z
	a.posYaw, a.posPitch = yaw, pitch
	a.posInitialized = true
	// Debug, not Info — see GetPosition's own doc comment; setPosition is
	// called at least once per physics tick during any movement.
	a.log().Debug(fmt.Sprintf("[setPosition %s] Updated bot position to %s with rotation (yaw=%.1f, pitch=%.1f)", a.cfg.Name, pos, yaw, pitch))
}

// GetPositionSimple returns bot position without rotation.
// When mounted, returns a.posX/Y/Z which is updated every tick by the physics
// executor (handleRidingMode → setBotPosition). The entity tracker is NOT used
// while riding because the server typically does not echo the vehicle's position
// back to the rider, making it stale within a tick or two.
func (a *agent) GetPositionSimple() (pos models.V3, initialized bool) {
	a.posMu.RLock()
	defer a.posMu.RUnlock()
	// Debug, not Info — see GetPosition's own doc comment.
	a.log().Debug(fmt.Sprintf("[GetPositionSimple %s] Bot position  (%.2f, %.2f, %.2f)", a.cfg.Name, a.posX, a.posY, a.posZ))

	return models.V3{X: a.posX, Y: a.posY, Z: a.posZ}, a.posInitialized
}

// Health returns the most recently received health/food/saturation values
// (see setHealth), and whether any HealthChange event has been received yet
// (false before the first one, e.g. before the player subsystem finishes
// joining). Satisfies models.HealthProvider.
func (a *agent) Health() (health float32, food int32, saturation float32, known bool) {
	a.healthMu.RLock()
	defer a.healthMu.RUnlock()
	return a.health, a.food, a.foodSaturation, a.healthInitialized
}

// setHealth records the latest health/food/saturation values from a
// HealthChange event (see HandleHealthChange), mirroring setPosition's
// pattern for the equivalent position state.
func (a *agent) setHealth(health float32, food int32, saturation float32) {
	a.healthMu.Lock()
	a.health, a.food, a.foodSaturation = health, food, saturation
	a.healthInitialized = true
	a.healthMu.Unlock()
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

// GetPlayerAbilities returns the bot's own last-known PlayerAbilities.
// initialized is false until the first clientbound Abilities packet (sent
// at login) has arrived.
func (a *agent) GetPlayerAbilities() (abilities models.PlayerAbilities, initialized bool) {
	a.abilitiesMu.RLock()
	defer a.abilitiesMu.RUnlock()
	return a.abilities, a.abilitiesInitialized
}

// setPlayerAbilities updates the bot's own tracked PlayerAbilities.
func (a *agent) setPlayerAbilities(abilities models.PlayerAbilities) {
	a.abilitiesMu.Lock()
	a.abilities = abilities
	a.abilitiesInitialized = true
	a.abilitiesMu.Unlock()
}

// GetGameMode returns the bot's own current game mode. initialized is false
// until the Login packet has been processed.
func (a *agent) GetGameMode() (gameMode models.GameMode, initialized bool) {
	a.gameModeMu.RLock()
	defer a.gameModeMu.RUnlock()
	return a.gameMode, a.gameModeInitialized
}

// setGameMode updates the bot's own tracked game mode.
func (a *agent) setGameMode(gameMode models.GameMode) {
	a.gameModeMu.Lock()
	a.gameMode = gameMode
	a.gameModeInitialized = true
	a.gameModeMu.Unlock()
}

// setMountedEntity sets the mounted vehicle entity ID and our seat index within
// that vehicle's passenger list. Pass entityID -1 (and index -1) to indicate
// dismounted. Both are written under one lock so a reader can never observe a
// mounted entity paired with a stale seat index.
func (a *agent) setMountedEntity(entityID int32, passengerIndex int) {
	a.mountedEntityMu.Lock()
	defer a.mountedEntityMu.Unlock()
	a.mountedEntityID = entityID
	a.mountedPassengerIndex = passengerIndex
}

// GetMountedPassengerIndex returns our index in the mounted vehicle's passenger
// list, or -1 when not mounted. Index 0 is the controlling passenger.
func (a *agent) GetMountedPassengerIndex() int {
	a.mountedEntityMu.RLock()
	defer a.mountedEntityMu.RUnlock()
	if a.mountedEntityID == -1 {
		return -1
	}
	return a.mountedPassengerIndex
}

// getMountedEntityID returns the currently mounted entity ID, or -1 if not mounted.
func (a *agent) getMountedEntityID() int32 {
	a.mountedEntityMu.RLock()
	defer a.mountedEntityMu.RUnlock()
	return a.mountedEntityID
}

// IsMounted returns true if the agent is currently riding a vehicle/mount.
func (a *agent) IsMounted() bool {
	a.mountedEntityMu.RLock()
	defer a.mountedEntityMu.RUnlock()
	return a.mountedEntityID != -1
}

// GetRidingVelocity returns the current horizontal riding velocity
// (blocks/tick). Returns (0, 0) when not mounted or when the executor
// does not support RidingPhysicsInspector.
func (a *agent) GetRidingVelocity() (float64, float64) {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()
	if inspector, ok := a.moveExec.(models.RidingPhysicsInspector); ok {
		return inspector.GetRidingVelocity()
	}
	return 0, 0
}

// GetRidingDragMultiplier returns the per-tick velocity multiplier used on
// the most recent riding tick. Returns 0 when the executor does not
// support RidingPhysicsInspector.
func (a *agent) GetRidingDragMultiplier() float64 {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()
	if inspector, ok := a.moveExec.(models.RidingPhysicsInspector); ok {
		return inspector.GetRidingDragMultiplier()
	}
	return 0
}

// GetCamelState returns the current camel state if mounted on a camel,
// or (nil, false) if not mounted or mounted on a different vehicle type.
func (a *agent) GetCamelState() (*models.CamelState, bool) {
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()
	if inspector, ok := a.moveExec.(models.RidingPhysicsInspector); ok {
		return inspector.GetCamelState()
	}
	return nil, false
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
	purgedEntityIDs := make([]int32, 0)
	a.entitiesMu.Lock()
	for id, e := range a.entities {
		if e.Removed && now.Sub(e.RemovedAt) > EntityRemovalGracePeriod {
			delete(a.entities, id)
			purgedEntityIDs = append(purgedEntityIDs, id)
		}
	}
	a.entitiesMu.Unlock()

	// Drop any container-window mapping for entities that no longer exist, so a
	// recycled window ID cannot be attributed to a dead entity.
	a.forgetEntityWindows(purgedEntityIDs)

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
				a.logf("[cleanupRemovedEntities] Fired pending projectile callbacks with fallback position (server didn't respond within 2s): projectileID=%d, type=%s, count=%d, clientPos=(%.2f, %.2f, %.2f) after %.1fs",
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
			a.logf("[cleanupRemovedEntities] Fired projectile callbacks due to callback timeout: projectileID=%d, type=%s, hitType=%v, count=%d, pos=(%.2f, %.2f, %.2f) after %.1fs",
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
func (a *agent) GetTrackedEntities() map[int32]models.TrackedEntityInfo {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	out := make(map[int32]models.TrackedEntityInfo, len(a.entities))
	for id, e := range a.entities {
		out[id] = models.TrackedEntityInfo{
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
			Pose:       e.Pose,
			PoseName:   e.PoseName,
			HasPose:    e.HasPose,
		}
	}
	return out
}

// FindNearestEntityByType finds the nearest entity of a specific type to a position.
// Returns entityID, distance, and whether found.
// Excludes entities marked as Removed. See models.Agent.FindNearestEntityByType's
// doc comment for honorPerceptionEffects' contract.
func (a *agent) FindNearestEntityByType(entityType int32, x, y, z float64, honorPerceptionEffects bool) (int32, float64, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	maxDist := math.Inf(1)
	if honorPerceptionEffects {
		maxDist = a.perceptionRadiusCap(maxDist)
	}

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

		if dist > maxDist {
			continue
		}

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
	pos, initialized := a.GetPositionSimple()
	botX, botY, botZ := pos.X, pos.Y, pos.Z
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
	return a.versionHandler.Play().Movement().SendRotation(a.client.Conn(), yaw, pitch, true)
}

// FacePosition makes the bot look at a specific position.
func (a *agent) FacePosition(targetX, targetY, targetZ float64) error {
	if a.versionHandler == nil || a.client == nil {
		return fmt.Errorf("version handler or client not initialized")
	}

	// Get current bot position
	pos, initialized := a.GetPositionSimple()
	botX, botY, botZ := pos.X, pos.Y, pos.Z
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
	return a.versionHandler.Play().Movement().SendRotation(a.client.Conn(), yaw, pitch, true)
}
