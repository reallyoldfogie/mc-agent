package agent

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// handlers returns set of packet handlers needed.
func (a *agent) handlers() []bot.PacketHandler {
	if a.packetMgr == nil {
		return nil
	}

	handlers := []bot.PacketHandler{
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSound"),
			Name:     "ClientboundSound",
			Priority: 0,
			F:        a.onSoundPacket,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundAddEntity"),
			Name:     "ClientboundAddEntity",
			Priority: 0,
			F:        a.onAddEntity,
		},
		// MoveEntityPosRot (packet ID 47/0x2F) - entity position + rotation update
		// Aliases: "ClientboundEntityMoveLook" in 1.21.8+ (same packet ID)
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundMoveEntityPosRot"),
			Name:     "ClientboundMoveEntityPosRot",
			Priority: 0,
			F:        a.onMoveEntityPosRot,
		},
		// MoveEntityPos (packet ID 46/0x2E) - entity relative position update only
		// Aliases: "ClientboundRelEntityMove" in 1.21.8+ (same packet ID)
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundMoveEntityPos"),
			Name:     "ClientboundMoveEntityPos",
			Priority: 0,
			F:        a.onMoveEntityPos,
		},
		// SyncEntityPosition (packet ID 31/0x1F) - absolute position sync
		// Available in all 1.21+ versions, provides absolute position with implicit velocity
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSyncEntityPosition"),
			Name:     "ClientboundSyncEntityPosition",
			Priority: 0,
			F:        a.onSyncEntityPosition,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundTeleportEntity"),
			Name:     "ClientboundTeleportEntity",
			Priority: 0,
			F:        a.onTeleportEntity,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundRemoveEntities"),
			Name:     "ClientboundRemoveEntities",
			Priority: 0,
			F:        a.onRemoveEntities,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundEntityMetadata"),
			Name:     "ClientboundEntityMetadata",
			Priority: 0,
			F:        a.onSetEntityMetadata,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundEntityVelocity"),
			Name:     "ClientboundEntityVelocity",
			Priority: 0,
			F:        a.onEntityVelocityUpdate,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundEntityStatus"),
			Name:     "ClientboundEntityStatus",
			Priority: 0,
			F:        a.onEntityStatus,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundGameEvent"),
			Name:     "ClientboundGameEvent",
			Priority: 0,
			F:        a.onGameEvent,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundEntityUpdateAttributes"),
			Name:     "ClientboundEntityUpdateAttributes",
			Priority: 0,
			F:        a.onEntityUpdateAttributes,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundEntityEquipment"),
			Name:     "ClientboundEntityEquipment",
			Priority: 0,
			F:        a.onEntityEquipment,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundEntityHeadRotation"),
			Name:     "ClientboundEntityHeadRotation",
			Priority: 0,
			F:        a.onEntityHeadRotation,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundEntityLook"),
			Name:     "ClientboundEntityLook",
			Priority: 0,
			F:        a.onEntityLook,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundLogin"),
			Name:     "ClientboundLogin",
			Priority: 100,
			F:        a.onLogin,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundPosition"),
			Name:     "ClientboundPosition",
			Priority: 63,
			F:        a.onClientboundPosition,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundPlayerInfo"),
			Name:     "ClientboundPlayerInfo",
			Priority: 90,
			F: func(p pk.Packet) error {
				if a.moveMirror != nil {
					a.moveMirror.HandlePlayerInfo(p)
				}
				return nil
			},
		},
		{
			ID:       a.packetMgr.GetClientboundConfigPacketID("ClientboundConfigFinishConfiguration"),
			Name:     "ClientboundConfigFinishConfiguration",
			Priority: 95,
			F: func(p pk.Packet) error {
				if a.moveMirror != nil {
					a.moveMirror.NotifyLoginSeen()
				}
				return nil
			},
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSetChunkCacheRadius"),
			Name:     "ClientboundSetChunkCacheRadius",
			Priority: 0,
			F:        a.onUpdateViewDistance,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSetSimulationDistance"),
			Name:     "ClientboundSetSimulationDistance",
			Priority: 0,
			F:        a.onSimulationDistance,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundDeclareRecipes"),
			Name:     "ClientboundDeclareRecipes",
			Priority: 0,
			F:        a.ParseUpdateRecipesPacket,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundDisconnect"),
			Name:     "ClientboundDisconnect",
			Priority: 0,
			F:        a.onDisconnect2,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSetSlot"),
			Name:     "ClientboundSetSlot",
			Priority: 0,
			F:        a.onSetSlot,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundContainerSetContent"),
			Name:     "ClientboundContainerSetContent",
			Priority: 0,
			F:        a.onWindowItems,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSetEquipment"),
			Name:     "ClientboundSetEquipment",
			Priority: 0,
			F:        a.onSetEquipment,
		},
	}
	// Include config-phase registry capture
	handlers = append(handlers, a.registryHandlers()...)

	// Include world packet handlers when using mc-agent world with version handler
	handlers = append(handlers, a.worldPacketHandlers()...)

	return handlers
}

// onDisconnect handles cleanup on disconnect packet.
func (a *agent) onDisconnect2(p pk.Packet) error {
	a.setEntityID(-1)

	name := ""
	if a.client != nil {
		name = a.client.Name()
	}

	if a.versionHandler == nil {
		log.Printf("[Agent %s] Disconnected from server.", name)
		return nil
	}

	reason, err := a.versionHandler.Play().ParseDisconnect(p)
	if err != nil {
		log.Printf("[Agent %s] Disconnected from server.", name)
		return nil
	}

	log.Printf("[Agent %s] Disconnected from server: %s", name, reason)
	return nil
}

// onAddEntity tracks new or respawned entities.
func (a *agent) onAddEntity(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, entityType, _, uuid, x, y, z, yaw, pitch, velX, velY, velZ, err := a.versionHandler.Play().Entities().ParseAddEntity(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	// Debug logging: Log entity spawns with type name lookup
	var entityTypeName string
	reg := a.GetRegistry("minecraft:entity_type")
	if reg != nil && reg.IsReady() {
		if name, ok := reg.GetNameByID(entityType); ok {
			entityTypeName = name
		}
	}
	if entityTypeName == "minecraft:arrow" {
		log.Printf("[onAddEntity] ARROW SPAWN: entityID=%d, pos=(%.2f, %.2f, %.2f), yaw=%d, pitch=%d, vel=(%.4f, %.4f, %.4f)", entityID, x, y, z, yaw, pitch, velX, velY, velZ)
	} else if entityTypeName != "" {
		log.Printf("[onAddEntity] Entity spawn: entityID=%d, type=%s, pos=(%.2f, %.2f, %.2f), yaw=%d, pitch=%d, vel=(%.4f, %.4f, %.4f)", entityID, entityTypeName, x, y, z, yaw, pitch, velX, velY, velZ)
	}

	a.entitiesMu.Lock()
	now := time.Now()
	if e, ok := a.entities[entityID]; ok {
		e.EntityType = entityType
		e.UUID = uuid
		e.X, e.Y, e.Z = x, y, z
		e.Yaw, e.Pitch = yaw, pitch
		e.VelX, e.VelY, e.VelZ = velX, velY, velZ // Initial velocity from spawn packet
		e.Removed = false
		e.LastMetadataUpdate = now // Initialize velocity timestamp
		e.LastPositionUpdate = now // Initialize position timestamp
	} else {
		a.entities[entityID] = &trackedEntity{
			EntityID:           entityID,
			EntityType:         entityType,
			UUID:               uuid,
			X:                  x,
			Y:                  y,
			Z:                  z,
			Yaw:                yaw,
			Pitch:              pitch,
			VelX:               velX, // Initial velocity from spawn packet
			VelY:               velY,
			VelZ:               velZ,
			Removed:            false,
			LastMetadataUpdate: now, // Initialize velocity timestamp
			LastPositionUpdate: now, // Initialize position timestamp
		}
	}
	a.entitiesMu.Unlock()

	// Register entity in the metadata handler's entity registry
	if a.entityRegistry != nil {
		var entityTypeStr common.EntityType
		reg := a.GetRegistry("minecraft:entity_type")
		if reg != nil && reg.IsReady() {
			if name, ok := reg.GetNameByID(entityType); ok {
				// Map registry name to EntityType (e.g., "minecraft:player" -> EntityTypePlayer)
				entityTypeStr = common.EntityType(name)
			}
		}
		if entityTypeStr == "" {
			entityTypeStr = common.EntityTypeUnknown
		}
		a.entityRegistry.RegisterEntity(entityID, entityTypeStr)
		log.Printf("[onAddEntity] Registered entity %d as type %s in metadata handler", entityID, entityTypeStr)
	}

	if a.moveMirror != nil && entityID == a.GetEntityID() {
		a.moveMirror.SetEntityType(entityType)
	}

	// Check if this is a projectile with a pending callback
	if reg != nil && reg.IsReady() {
		if name, ok := reg.GetNameByID(entityType); ok {
			if projType, isProjectile := projectileTypeFromEntityName(name); isProjectile {
				// Look for matching pending callback
				a.pendingProjectilesMu.Lock()
				for i, pending := range a.pendingProjectiles {
					if pending.projectileType == projType {
						// Found matching projectile - move to active tracking
						a.activeProjectilesMu.Lock()
						now := time.Now()
						spawnPos := models.V3{X: x, Y: y, Z: z}
						spawnVel := models.V3{X: velX, Y: velY, Z: velZ}
						a.activeProjectiles[entityID] = &activeProjectileInfo{
							projectileType: projType,
							firedAt:        now,
							callbacks:      pending.callbacks,
							callbacksFired: false,
							// Initialize position history with spawn position
							lastServerPos:     spawnPos,
							currentServerPos:  spawnPos,
							lastServerTime:    time.Time{}, // Zero time initially
							currentServerTime: now,
							interpolatedPos:   spawnPos,
							// Store spawn data for velocity-based interpolation before position updates
							spawnPos:      spawnPos,
							spawnTime:     now,
							spawnVelocity: spawnVel,
						}
						a.activeProjectilesMu.Unlock()

						// Remove from pending queue
						a.pendingProjectiles = append(a.pendingProjectiles[:i], a.pendingProjectiles[i+1:]...)
						log.Printf("[onAddEntity] Matched projectile %s (entityID=%d) to pending callbacks (count=%d)", name, entityID, len(pending.callbacks))
						break
					}
				}
				a.pendingProjectilesMu.Unlock()
			}
		}
	}

	return nil
}

// onMoveEntityPosRot updates incremental position and rotation.
func (a *agent) onMoveEntityPosRot(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, dx, dy, dz, yaw, pitch, _, err := a.versionHandler.Play().Entities().ParseMoveEntityPosRot(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	a.entitiesMu.Lock()
	now := time.Now()
	if e, ok := a.entities[entityID]; ok {
		oldX, oldY, oldZ := e.X, e.Y, e.Z
		// Minecraft encodes position deltas as fixed-point: divide by (128*32=4096) to convert to block units
		e.X += float64(dx) / (128 * 32)
		e.Y += float64(dy) / (128 * 32)
		e.Z += float64(dz) / (128 * 32)
		e.Yaw, e.Pitch = yaw, pitch
		// Only update position timestamp if position actually changed (non-zero deltas)
		if dx != 0 || dy != 0 || dz != 0 {
			// Store position history for interpolation
			e.lastServerX, e.lastServerY, e.lastServerZ = oldX, oldY, oldZ
			e.lastServerUpdateTime = e.currentServerUpdateTime
			e.currentServerUpdateTime = now
			e.LastPositionUpdate = now
		}

		// Update position history in active projectiles (for render loop interpolation)
		// Update even for zero-delta packets - they still represent a server position confirmation
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			// Only update if there's an actual position change
			if dx != 0 || dy != 0 || dz != 0 {
				projInfo.lastServerPos = models.V3{X: oldX, Y: oldY, Z: oldZ}
				projInfo.lastServerTime = projInfo.currentServerTime
				projInfo.currentServerPos = models.V3{X: e.X, Y: e.Y, Z: e.Z}
				projInfo.currentServerTime = now
				log.Printf("[onMoveEntityPosRot] PROJECTILE: entityID=%d, oldPos=(%.2f,%.2f,%.2f), delta=(%.4f,%.4f,%.4f), newPos=(%.2f,%.2f,%.2f), yaw=%d, pitch=%d",
					entityID, oldX, oldY, oldZ, float64(dx)/(128*32), float64(dy)/(128*32), float64(dz)/(128*32), e.X, e.Y, e.Z, yaw, pitch)
			} else {
				// Zero delta packet - update currentServerTime to indicate we received a position confirmation
				// but keep the position unchanged
				projInfo.currentServerTime = now
				log.Printf("[onMoveEntityPosRot] PROJECTILE: entityID=%d, no position delta, updated time", entityID)
			}
		}
		a.activeProjectilesMu.Unlock()

		// Debug logging for arrows
		if reg := a.GetRegistry("minecraft:entity_type"); reg != nil && reg.IsReady() {
			if name, ok := reg.GetNameByID(e.EntityType); ok && (dx != 0 || dy != 0 || dz != 0) {
				// Minecraft encodes position deltas as fixed-point: divide by (128*32=4096) to convert to block units
				deltaX := float64(dx) / (128 * 32)
				deltaY := float64(dy) / (128 * 32)
				deltaZ := float64(dz) / (128 * 32)
				newX := e.X
				newY := e.Y
				newZ := e.Z
				// Also calculate velocity from position change (velocity = delta / tick)
				log.Printf("[onMoveEntityPosRot] %s: entityID=%d, oldPos=(%.2f, %.2f, %.2f), delta=(%.4f, %.4f, %.4f), newPos=(%.2f, %.2f, %.2f), vel/tick=(%.4f, %.4f, %.4f), yaw=%d, pitch=%d",
					name, entityID, oldX, oldY, oldZ, deltaX, deltaY, deltaZ, newX, newY, newZ, deltaX, deltaY, deltaZ, yaw, pitch)
			}
		}

		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onMoveEntityPos updates incremental position without rotation.
func (a *agent) onMoveEntityPos(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, dx, dy, dz, _, err := a.versionHandler.Play().Entities().ParseMoveEntityPos(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	a.entitiesMu.Lock()
	now := time.Now()
	if e, ok := a.entities[entityID]; ok {
		oldX, oldY, oldZ := e.X, e.Y, e.Z
		// Minecraft encodes position deltas as fixed-point: divide by (128*32=4096) to convert to block units
		e.X += float64(dx) / (128 * 32)
		e.Y += float64(dy) / (128 * 32)
		e.Z += float64(dz) / (128 * 32)
		// Only update position timestamp if position actually changed (non-zero deltas)
		if dx != 0 || dy != 0 || dz != 0 {
			// Store position history for interpolation
			e.lastServerX, e.lastServerY, e.lastServerZ = oldX, oldY, oldZ
			e.lastServerUpdateTime = e.currentServerUpdateTime
			e.currentServerUpdateTime = now
			e.LastPositionUpdate = now
		}

		// Update position history in active projectiles (for render loop interpolation)
		// Update even for zero-delta packets - they still represent a server position confirmation
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			// Only update if there's an actual position change
			if dx != 0 || dy != 0 || dz != 0 {
				projInfo.lastServerPos = models.V3{X: oldX, Y: oldY, Z: oldZ}
				projInfo.lastServerTime = projInfo.currentServerTime
				projInfo.currentServerPos = models.V3{X: e.X, Y: e.Y, Z: e.Z}
				projInfo.currentServerTime = now
				log.Printf("[onMoveEntityPos] PROJECTILE: entityID=%d, oldPos=(%.2f,%.2f,%.2f), delta=(%.4f,%.4f,%.4f), newPos=(%.2f,%.2f,%.2f)",
					entityID, oldX, oldY, oldZ, float64(dx)/(128*32), float64(dy)/(128*32), float64(dz)/(128*32), e.X, e.Y, e.Z)
			} else {
				// Zero delta packet - update currentServerTime to indicate we received a position confirmation
				// but keep the position unchanged
				projInfo.currentServerTime = now
				log.Printf("[onMoveEntityPos] PROJECTILE: entityID=%d, no position delta, updated time", entityID)
			}
		}
		a.activeProjectilesMu.Unlock()

		if e.Removed {
			e.Removed = false
		}
	} else {
		// Entity not tracked - this might be a projectile that spawned but wasn't matched yet
		a.activeProjectilesMu.Lock()
		if _, exists := a.activeProjectiles[entityID]; exists {
			log.Printf("[onMoveEntityPos] PROJECTILE entityID=%d not in entities map yet! delta=(%.4f,%.4f,%.4f)",
				entityID, float64(dx)/(128*32), float64(dy)/(128*32), float64(dz)/(128*32))
		}
		a.activeProjectilesMu.Unlock()
	}
	a.entitiesMu.Unlock()
	return nil
}

// onSyncEntityPosition handles absolute position sync packets.
// This packet provides absolute coordinates and is used for explicit position synchronization.
func (a *agent) onSyncEntityPosition(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, x, y, z, dx, dy, dz, yaw, pitch, onGround, err := a.versionHandler.Play().Entities().ParseSyncEntityPosition(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	a.entitiesMu.Lock()
	now := time.Now()
	if e, ok := a.entities[entityID]; ok {
		// Debug logging for arrows
		if reg := a.GetRegistry("minecraft:entity_type"); reg != nil && reg.IsReady() {
			if name, ok := reg.GetNameByID(e.EntityType); ok && name == "minecraft:arrow" {
				log.Printf("[onSyncEntityPosition] ARROW: entityID=%d, oldPos=(%.2f, %.2f, %.2f), newPos=(%.2f, %.2f, %.2f), vel=(%.4f, %.4f, %.4f), ground=%v",
					entityID, e.X, e.Y, e.Z, x, y, z, dx, dy, dz, onGround)
			}
		}
		// Store position history for interpolation
		e.lastServerX, e.lastServerY, e.lastServerZ = e.X, e.Y, e.Z
		e.lastServerUpdateTime = e.currentServerUpdateTime
		e.X, e.Y, e.Z = x, y, z
		e.Yaw, e.Pitch = yaw, pitch
		e.currentServerUpdateTime = now
		e.LastPositionUpdate = now // Track when position was last updated

		// Update position history in active projectiles (for render loop interpolation)
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			projInfo.lastServerPos = models.V3{X: e.lastServerX, Y: e.lastServerY, Z: e.lastServerZ}
			projInfo.lastServerTime = e.lastServerUpdateTime
			projInfo.currentServerPos = models.V3{X: x, Y: y, Z: z}
			projInfo.currentServerTime = now
			log.Printf("[onSyncEntityPosition] PROJECTILE: entityID=%d, oldPos=(%.2f,%.2f,%.2f), newPos=(%.2f,%.2f,%.2f)",
				entityID, e.lastServerX, e.lastServerY, e.lastServerZ, x, y, z)

			// Check if this projectile has a pending callback waiting for server position confirmation
			if projInfo.pendingCallbackFire && !projInfo.callbacksFired && len(projInfo.callbacks) > 0 {
				// Use server's authoritative position instead of client prediction
				serverPos := models.V3{X: x, Y: y, Z: z}
				flightTime := now.Sub(projInfo.spawnTime).Seconds()

				evt := models.ProjectileHitEvent{
					HitType:        projInfo.pendingHitType,
					ProjectileType: projInfo.projectileType,
					Position:       serverPos,
				}

				for _, cb := range projInfo.callbacks {
					go cb(evt) // Fire asynchronously
				}

				projInfo.callbacksFired = true
				log.Printf("[onSyncEntityPosition] Fired pending projectile callbacks with server position: projectileID=%d, type=%s, hitType=%v, count=%d, flightTime=%.3fs, clientPred=(%.2f, %.2f, %.2f), serverPos=(%.2f, %.2f, %.2f), delta=%.2f blocks",
					entityID, projInfo.projectileType, projInfo.pendingHitType, len(projInfo.callbacks), flightTime,
					projInfo.pendingHitPos.X, projInfo.pendingHitPos.Y, projInfo.pendingHitPos.Z,
					serverPos.X, serverPos.Y, serverPos.Z,
					projInfo.pendingHitPos.DistanceTo(serverPos))
			}
		}
		a.activeProjectilesMu.Unlock()
		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onTeleportEntity handles absolute teleports.
func (a *agent) onTeleportEntity(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, x, y, z, yaw, pitch, _, err := a.versionHandler.Play().Entities().ParseTeleportEntity(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	a.entitiesMu.Lock()
	now := time.Now()
	if e, ok := a.entities[entityID]; ok {
		// Debug logging for arrows
		if reg := a.GetRegistry("minecraft:entity_type"); reg != nil && reg.IsReady() {
			if name, ok := reg.GetNameByID(e.EntityType); ok && name == "minecraft:arrow" {
				log.Printf("[onTeleportEntity] ARROW: entityID=%d, oldPos=(%.2f, %.2f, %.2f), newPos=(%.2f, %.2f, %.2f), yaw=%d, pitch=%d",
					entityID, e.X, e.Y, e.Z, x, y, z, yaw, pitch)
			}
		}
		// Store position history for interpolation
		e.lastServerX, e.lastServerY, e.lastServerZ = e.X, e.Y, e.Z
		e.lastServerUpdateTime = e.currentServerUpdateTime
		e.X, e.Y, e.Z = x, y, z
		e.currentServerUpdateTime = now
		e.Yaw, e.Pitch = yaw, pitch
		e.LastPositionUpdate = now // Track when position was last updated

		// Update position history in active projectiles (for render loop interpolation)
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			projInfo.lastServerPos = models.V3{X: e.lastServerX, Y: e.lastServerY, Z: e.lastServerZ}
			projInfo.lastServerTime = e.lastServerUpdateTime
			projInfo.currentServerPos = models.V3{X: x, Y: y, Z: z}
			projInfo.currentServerTime = now
		}
		a.activeProjectilesMu.Unlock()
		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onEntityVelocityUpdate handles velocity changes for entities in flight.
// For projectiles, velocity updates allow tracking movement when position packets have zero deltas.
func (a *agent) onEntityVelocityUpdate(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, velX, velY, velZ, err := a.versionHandler.Play().Entities().ParseEntityVelocityUpdate(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		// Check if this is a tracked projectile
		a.activeProjectilesMu.Lock()
		isProjectile := a.activeProjectiles[entityID] != nil
		a.activeProjectilesMu.Unlock()

		if isProjectile {
			log.Printf("[onEntityVelocityUpdate] PROJECTILE: entityID=%d, velocity=(%.4f, %.4f, %.4f)",
				entityID, velX, velY, velZ)
		}

		// Store velocity and update timestamp for interpolation
		e.VelX = velX
		e.VelY = velY
		e.VelZ = velZ
		e.LastMetadataUpdate = time.Now() // Record when velocity was updated for interpolation
	}
	a.entitiesMu.Unlock()
	return nil
}

// onRemoveEntities marks entities as softly removed, allowing grace period before purge.
// Also fires projectile hit callbacks if a tracked projectile was removed.
func (a *agent) onRemoveEntities(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityIDs, err := a.versionHandler.Play().Entities().ParseRemoveEntities(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	now := time.Now()
	a.entitiesMu.Lock()
	for _, id := range entityIDs {
		if e, ok := a.entities[id]; ok {
			e.Removed = true
			e.RemovedAt = now
		}
	}
	a.entitiesMu.Unlock()

	// Check if any removed entities are tracked projectiles and fire callbacks
	a.activeProjectilesMu.Lock()
	for _, id := range entityIDs {
		if projInfo, exists := a.activeProjectiles[id]; exists && len(projInfo.callbacks) > 0 && !projInfo.callbacksFired {
			// Determine hit type based on projectile type and state
			hitType := models.ProjectileHitUnknown
			if projInfo.projectileType.IsPersistent() {
				// For persistent projectiles (arrows, tridents), check isInGround flag
				if projInfo.isInGround {
					// Arrow is stuck in block
					// If shake > 0, this is a pickup (arrow hasn't stopped shaking yet)
					// If shake == 0, arrow has finished shaking and despawned
					if projInfo.shake > 0 {
						// Arrow is being picked up while still in block
						hitType = models.ProjectileHitBlock
						log.Printf("[onRemoveEntities] Arrow removal with shake=%d indicates PICKUP (not despawn)", projInfo.shake)
					} else {
						// Arrow has stopped shaking and is being despawned due to timeout
						hitType = models.ProjectileHitBlock
						log.Printf("[onRemoveEntities] Arrow removal with shake=0 after IN_GROUND indicates DESPAWN/TIMEOUT")
					}
				} else {
					hitType = models.ProjectileHitEntity
				}
			}
			// For non-persistent projectiles (snowball, egg, ender pearl, potion):
			// We can't distinguish block vs entity hits, so use ProjectileHitUnknown

			// Get projectile position: use different strategies depending on what data we have
			var pos models.V3
			var positionSource string

			a.entitiesMu.RLock()
			e, entityExists := a.entities[id]
			a.entitiesMu.RUnlock()

			// Prefer most recent server position data if available
			if !projInfo.currentServerTime.IsZero() {
				// We have received position updates from the server
				pos = projInfo.currentServerPos
				positionSource = "latest server position"
				log.Printf("[onRemoveEntities] Projectile entityID=%d type=%s using %s: (%.2f, %.2f, %.2f) (spawn was %.2f,%.2f,%.2f)", id, projInfo.projectileType, positionSource, pos.X, pos.Y, pos.Z, projInfo.spawnPos.X, projInfo.spawnPos.Y, projInfo.spawnPos.Z)
			} else if projInfo.interpolatedPos != (models.V3{}) {
				// Use render loop's interpolated position if available
				pos = projInfo.interpolatedPos
				positionSource = "render loop interpolated"
				log.Printf("[onRemoveEntities] Projectile entityID=%d type=%s using %s: (%.2f, %.2f, %.2f) (spawn was %.2f,%.2f,%.2f)", id, projInfo.projectileType, positionSource, pos.X, pos.Y, pos.Z, projInfo.spawnPos.X, projInfo.spawnPos.Y, projInfo.spawnPos.Z)
				// Calculate time in flight
				flightTime := now.Sub(projInfo.spawnTime).Seconds()
				if flightTime > 0 {
					// Use discrete physics simulation to estimate final position
					pos = a.InterpolateProjectilePosition(projInfo, flightTime)
					positionSource = fmt.Sprintf("velocity-based estimation (%.3fs flight)", flightTime)
					log.Printf("[onRemoveEntities] Projectile entityID=%d type=%s using %s: (%.2f, %.2f, %.2f)", id, projInfo.projectileType, positionSource, pos.X, pos.Y, pos.Z)
				} else {
					// Very short flight time, use spawn position
					pos = projInfo.spawnPos
					positionSource = "spawn position (very short flight)"
					log.Printf("[onRemoveEntities] Projectile entityID=%d type=%s using %s: (%.2f, %.2f, %.2f)", id, projInfo.projectileType, positionSource, pos.X, pos.Y, pos.Z)
				}
			} else if projInfo.lastServerTime.IsZero() && entityExists {
				// Persistent projectile or entity exists but no position data, use entity position
				pos = models.V3{X: e.X, Y: e.Y, Z: e.Z}
				positionSource = "last tracked entity position"
				log.Printf("[onRemoveEntities] Projectile entityID=%d type=%s using %s: (%.2f, %.2f, %.2f) (spawn was %.2f,%.2f,%.2f)", id, projInfo.projectileType, positionSource, pos.X, pos.Y, pos.Z, projInfo.spawnPos.X, projInfo.spawnPos.Y, projInfo.spawnPos.Z)
			} else {
				positionSource = "interpolated position (fallback)"
				log.Printf("[onRemoveEntities] Projectile entityID=%d type=%s using %s: (%.2f, %.2f, %.2f)", id, projInfo.projectileType, positionSource, pos.X, pos.Y, pos.Z)
			}

			// Fire all callbacks
			evt := models.ProjectileHitEvent{
				HitType:        hitType,
				ProjectileType: projInfo.projectileType,
				Position:       pos,
			}
			for _, cb := range projInfo.callbacks {
				go cb(evt) // Fire asynchronously to not block handler
			}

			log.Printf("[onRemoveEntities] Fired projectile hit callbacks: type=%s, hitType=%v, count=%d, pos=(%.2f, %.2f, %.2f). Entity tracked pos: X=%.2f, Y=%.2f, Z=%.2f",
				projInfo.projectileType, hitType, len(projInfo.callbacks), pos.X, pos.Y, pos.Z, pos.X, pos.Y, pos.Z)

			// Remove from active tracking
			delete(a.activeProjectiles, id)
		}
	}
	a.activeProjectilesMu.Unlock()

	// Clean up entity registry
	if a.entityRegistry != nil {
		a.entityRegistry.RemoveEntities(entityIDs)
		log.Printf("[onRemoveEntities] Cleaned up %d entities from metadata handler registry", len(entityIDs))
	}

	return nil
}

// onSetEntityMetadata updates entity health and other metadata.
// For arrows, detects block hits via isInGround flag and can fire callbacks early.
func (a *agent) onSetEntityMetadata(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, entries, err := a.versionHandler.Play().Entities().ParseSetEntityMetadata(p)
	if err != nil {
		log.Printf("[Agent %s][onSetEntityMetadata] entityID=%d", a.client.Name(), entityID)
		return nil // ignore malformed packets
	}

	if projInfo, exists := a.activeProjectiles[entityID]; exists {

		log.Printf("[Agent %s][onSetEntityMetadata] %s entityID=%d %s)", a.client.Name(), projInfo.projectileType.String(), entityID, spew.Sdump(projInfo))
	}

	// Process metadata through the handler system if available
	var metadataResults []common.MetadataProcessResult
	if a.metadataHandler != nil {
		for _, entry := range entries {
			// Process each metadata entry through the handler
			result, err := a.metadataHandler.HandleMetadata(entityID, entry)
			if err != nil {
				log.Printf("[Agent %s][onSetEntityMetadata] Warning: failed to process metadata for entity %d: %v", a.client.Name(), entityID, err)
				continue
			}
			metadataResults = append(metadataResults, result)
		}
	}

	// Extract metadata we care about
	var health float32 = -1.0    // -1 indicates not set
	var maxHealth float32 = 20.0 // Default max health
	var isInGround bool = false  // Arrow projectile state
	var velocity *[3]float64     // Entity velocity from metadata
	var shake int8 = 0           // Shake animation counter
	var criticalHit bool = false // Critical hit flag
	var pierceLevel int8 = 0     // Piercing level
	var potionColor int32 = -1   // Potion color (-1 = no potion)

	for _, result := range metadataResults {
		// Apply health from metadata result
		if result.HasHealth {
			health = result.Health
			maxHealth = result.MaxHealth
		}
		// Apply isInGround from metadata result
		if result.IsInGround {
			isInGround = true
		}
		// Apply velocity from metadata result
		if result.HasVelocity && result.Velocity != nil {
			velocity = result.Velocity
		}
		// Apply projectile-specific metadata
		if result.HasShake {
			shake = result.Shake
		}
		if result.HasCritical {
			criticalHit = result.CriticalHit
		}
		if result.HasPierce {
			pierceLevel = result.PierceLevel
		}
		if result.HasColor {
			potionColor = result.PotionColor
		}
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		// Only update if health was actually provided in metadata
		if health >= 0 {
			log.Printf("[Agent %s] [ParseSetEntityMetadata] Entity %d health updated: %.1f / %.1f", a.client.Name(), entityID, health, maxHealth)
			e.Health = health
		}
		e.MaxHealth = maxHealth
		// Apply velocity from metadata if present
		if velocity != nil {
			e.VelX = velocity[0]
			e.VelY = velocity[1]
			e.VelZ = velocity[2]
			log.Printf("[Agent %s] [ParseSetEntityMetadata] Entity %d velocity updated: (%.4f, %.4f, %.4f)", a.client.Name(), entityID, velocity[0], velocity[1], velocity[2])
		}
		// Store projectile-specific metadata
		e.shake = shake
		e.criticalHit = criticalHit
		e.pierceLevel = pierceLevel
		e.potionColor = potionColor
		// Update last metadata update timestamp
		e.LastMetadataUpdate = time.Now()
	}
	a.entitiesMu.Unlock()

	// Track isInGround state for arrows (used to distinguish block vs entity hits)
	a.activeProjectilesMu.Lock()
	if projInfo, exists := a.activeProjectiles[entityID]; exists && projInfo.projectileType.IsPersistent() {
		prevInGround := projInfo.isInGround
		projInfo.isInGround = isInGround
		// Store projectile-specific metadata in active projectiles
		projInfo.shake = shake
		projInfo.criticalHit = criticalHit
		projInfo.pierceLevel = pierceLevel
		projInfo.potionColor = potionColor
		log.Printf("[Agent %s][onSetEntityMetadata] %s entityID=%d isInGround=%v (was %v), shake=%d, critical=%v, pierce=%d, color=%d", a.client.Name(), projInfo.projectileType.String(), entityID, isInGround, prevInGround, shake, criticalHit, pierceLevel, potionColor)

		// If arrow just hit a block (isInGround transitioned from false to true), fire block hit callbacks early
		if isInGround && !prevInGround && !projInfo.callbacksFired && len(projInfo.callbacks) > 0 {
			// Get arrow position from render loop's continuous interpolation
			pos := projInfo.interpolatedPos
			flightTime := time.Since(projInfo.spawnTime).Seconds()
			log.Printf("[onSetEntityMetadata] Block hit detected: entityID=%d, flightTime=%.3fs, spawn=(%.2f,%.2f,%.2f), interpolated=(%.2f,%.2f,%.2f), spawnVel=(%.4f,%.4f,%.4f)",
				entityID, flightTime,
				projInfo.spawnPos.X, projInfo.spawnPos.Y, projInfo.spawnPos.Z,
				pos.X, pos.Y, pos.Z,
				projInfo.spawnVelocity.X, projInfo.spawnVelocity.Y, projInfo.spawnVelocity.Z)

			// Fire all block hit callbacks immediately
			evt := models.ProjectileHitEvent{
				HitType:        models.ProjectileHitBlock,
				ProjectileType: projInfo.projectileType,
				Position:       pos,
			}
			for _, cb := range projInfo.callbacks {
				go cb(evt) // Fire asynchronously
			}

			log.Printf("[Agent %s][onSetEntityMetadata] Fired early ProjectileHitBlock callbacks: entityID=%d, count=%d, pos=(%.2f, %.2f, %.2f)",
				a.client.Name(), entityID, len(projInfo.callbacks), pos.X, pos.Y, pos.Z)

			// Mark callbacks as already fired so we don't fire them again in onRemoveEntities
			projInfo.callbacksFired = true
		}
	}
	a.activeProjectilesMu.Unlock()

	return nil
}

// onEntityStatus handles entity status effects and animations.
// For arrows, detects potion effect expiration.
func (a *agent) onEntityStatus(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, eventID, err := a.versionHandler.Play().Entities().ParseEntityEvent(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	// Check for potion effect expiration (status byte = 0)
	if eventID == 0 {
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			log.Printf("[onEntityStatus] Potion effect expiration for %s entityID=%d", projInfo.projectileType.String(), entityID)
		}
		a.activeProjectilesMu.Unlock()
	}

	// Log entity status for debugging
	log.Printf("[onEntityStatus] Entity %d status event: %d", entityID, eventID)
	return nil
}

// onGameEvent handles game events like projectile impacts.
func (a *agent) onGameEvent(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	eventType, x, y, z, value, err := a.versionHandler.Play().ParseGameEvent(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	// Log game event for debugging
	log.Printf("[onGameEvent] Event type: %d, Position: (%.2f, %.2f, %.2f), Value: %.1f", eventType, x, y, z, value)

	// Check for PROJECTILE_LAND event (game event type 3)
	if eventType == 3 {
		log.Printf("[onGameEvent] PROJECTILE_LAND detected at (%.2f, %.2f, %.2f)", x, y, z)
	}

	return nil
}

// onEntityUpdateAttributes handles entity attribute updates (health, speed, etc.).
// For now, we log these but don't need to take action.
func (a *agent) onEntityUpdateAttributes(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	// Note: This packet is complex and contains multiple attributes.
	// For now, we'll just log that we received it and not parse the contents.
	log.Printf("[onEntityUpdateAttributes] Received entity attributes update")
	return nil
}

// onEntityEquipment handles entity equipment changes (held items, armor).
func (a *agent) onEntityEquipment(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, equipment, err := a.versionHandler.Play().Entities().ParseEntityEquipment(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	// Log all equipment updates in this packet
	for _, eq := range equipment {
		log.Printf("[onEntityEquipment] Entity %d equipment slot %d: itemID=%d, count=%d", entityID, eq.Slot, eq.Item.ItemID, eq.Item.Count)
	}
	return nil
}

// onEntityHeadRotation handles entity head rotation updates.
func (a *agent) onEntityHeadRotation(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, headYaw, err := a.versionHandler.Play().Entities().ParseEntityHeadRotation(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		e.HeadYaw = headYaw
	}
	a.entitiesMu.Unlock()

	log.Printf("[onEntityHeadRotation] Entity %d head yaw: %d", entityID, headYaw)
	return nil
}

// onEntityLook handles entity look packets (rotation-only updates).
func (a *agent) onEntityLook(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, yaw, pitch, onGround, err := a.versionHandler.Play().Entities().ParseEntityLook(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		e.Yaw, e.Pitch = yaw, pitch
		e.OnGround = onGround
	}
	a.entitiesMu.Unlock()

	log.Printf("[onEntityLook] Entity %d look: yaw=%d, pitch=%d, onGround=%v", entityID, yaw, pitch, onGround)
	return nil
}

// onSetSlot handles inventory slot changes.
func (a *agent) onSetSlot(p pk.Packet) error {
	// Packets are automatically recorded by the bot client's replay recorder
	return nil
}

// onWindowItems handles container/window inventory updates.
func (a *agent) onWindowItems(p pk.Packet) error {
	// Packets are automatically recorded by the bot client's replay recorder
	return nil
}

// onSetEquipment handles entity equipment changes (held items, armor).
func (a *agent) onSetEquipment(p pk.Packet) error {
	// Packets are automatically recorded by the bot client's replay recorder
	return nil
}

// onLogin captures the bot's entity ID.
func (a *agent) onLogin(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	entityID, err := a.versionHandler.Play().ParseLogin(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	a.setEntityID(entityID)
	if a.rec != nil {
		// Set selfId to -1 to match ReplayMod's standard behavior.
		// This indicates no special camera entity - all players render normally.
		a.rec.SetSelfID(-1)
	}
	if a.moveMirror != nil {
		var id [16]byte
		if parsed, err := uuid.Parse(a.cfg.Auth.UUID); err == nil {
			copy(id[:], parsed[:])
		}
		a.moveMirror.SetEntityMeta(entityID, a.cfg.Auth.Name, id)
		// Entity type is set via registry callback during configuration phase
	}
	return nil
}

// onClientboundPosition updates absolute position and applies rotation flags.
func (a *agent) onClientboundPosition(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	TeleportID, X, Y, Z, Yaw, Pitch, Flags, err := a.versionHandler.Play().Movement().ParsePlayerPosition(p)
	if err != nil {
		return nil // Silently ignore parse errors
	}

	if a.moveMirror != nil {
		// First play-state packet; safe to allow replay mirror emissions now.
		a.moveMirror.NotifyLoginSeen()
	}

	// Absolute base position
	a.posMu.Lock()
	a.posX, a.posY, a.posZ = X, Y, Z
	// Rotation may be relative per flags
	if Flags&0x08 != 0 {
		a.posYaw += Yaw
	} else {
		a.posYaw = Yaw
	}
	if Flags&0x10 != 0 {
		a.posPitch += Pitch
	} else {
		a.posPitch = Pitch
	}
	a.posInitialized = true

	// Notify replay mirror of position update by creating a synthetic serverbound packet
	// This is critical for bot visibility in replays - the mirror needs movement packets
	// to trigger entity spawning via emitTeleport()
	if a.moveMirror != nil && a.packetMgr != nil {
		syntheticPacket := pk.Marshal(
			int32(a.packetMgr.GetServerboundPacketID("ServerboundMovePlayerPosRot")),
			pk.Double(a.posX),
			pk.Double(a.posY),
			pk.Double(a.posZ),
			pk.Float(a.posYaw),
			pk.Float(a.posPitch),
			pk.Boolean(true), // onGround
		)
		a.moveMirror.HandleServerbound(syntheticPacket)
	}
	if syncer, ok := a.moveExec.(interface {
		SyncWithServer(x, y, z float64, yaw, pitch float32, onGround bool)
	}); ok {
		syncer.SyncWithServer(a.posX, a.posY, a.posZ, a.posYaw, a.posPitch, true)
	}

	// Accept teleport BEFORE unlocking mutex.
	// This prevents a race condition where the physics executor starts sending
	// movement packets before the teleport confirmation is sent, which causes
	// "Invalid move player packet received" errors on the server.
	// Prefer auto-created player, fall back to injected teleport
	var t TeleportAccepter = a.player
	if t == nil {
		t = a.teleport
	}
	if t != nil {
		_ = t.AcceptTeleportation(pk.VarInt(TeleportID))
	}

	a.posMu.Unlock()
	return nil
}

// onUpdateViewDistance handles server-sent view distance updates.
func (a *agent) onUpdateViewDistance(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	viewDistance, err := a.versionHandler.Play().ParseViewDistance(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	// Currently just logging for awareness
	// Could be used to update client state if needed
	log.Printf("view distance: %d", viewDistance)
	return nil
}

// onSimulationDistance handles server-sent simulation distance updates.
func (a *agent) onSimulationDistance(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // silently ignore if no version handler
	}

	simulationDistance, err := a.versionHandler.Play().ParseSimulationDistance(p)
	if err != nil {
		return nil // ignore malformed packets
	}

	// Currently just logging for awareness
	// Could be used to update client state if needed
	log.Printf("simulation distance: %d", simulationDistance)
	return nil
}

// ParseUpdateRecipesPacket handles the ClientboundUpdateRecipes packet (also known as DeclaredRecipes).
// Protocol 1.21.5 (770) format: Property Sets + Stonecutter SingleInputSet entries
func (a *agent) ParseUpdateRecipesPacket(p pk.Packet) error {
	log.Printf("[recipes %s] === Starting Update Recipes Packet Parse ===", a.client.Name())

	log.Printf("[recipes %s] Packet Length: %d bytes", a.client.Name(), len(p.Data))
	log.Printf("[recipes %s] Packet ID: %d", a.client.Name(), p.ID)
	log.Printf("[recipes %s] Raw Packet Data: [%x]", a.client.Name(), p.Data)

	// Create a streaming reader over packet data
	r := bytes.NewReader(p.Data)
	// Parse Property Sets
	var payload UpdateRecipesPayload
	var numPropertySets pk.VarInt
	if _, err := numPropertySets.ReadFrom(r); err != nil {
		log.Printf("[recipes %s] ERROR: failed to read property set count: %v", a.client.Name(), err)
		return nil
	}
	log.Printf("[recipes %s] Property Sets Count: %d", a.client.Name(), numPropertySets)

	for i := 0; i < int(numPropertySets); i++ {
		var propertySetID pk.Identifier
		if _, err := propertySetID.ReadFrom(r); err != nil {
			log.Printf("[recipes %s] ERROR: failed to read property set ID at index %d: %v", a.client.Name(), i, err)
			return nil
		}

		var numItems pk.VarInt
		if _, err := numItems.ReadFrom(r); err != nil {
			log.Printf("[recipes %s] ERROR: failed to read item count for property set %s: %v", a.client.Name(), propertySetID, err)
			return nil
		}

		log.Printf("[recipes %s] Property Set %d: ID=%s, Items Count=%d", a.client.Name(), i+1, propertySetID, numItems)

		items := make([]int32, int(numItems))
		for j := 0; j < int(numItems); j++ {
			var itemID pk.VarInt
			if _, err := itemID.ReadFrom(r); err != nil {
				log.Printf("[recipes %s] ERROR: failed to read item ID at index %d for property set %s: %v", a.client.Name(), j, propertySetID, err)
				return nil
			}
			items[j] = int32(itemID)
		}
		log.Printf("[recipes %s]   Items: %v", a.client.Name(), items)
		payload.PropertySets = append(payload.PropertySets, PropertySet{ID: fmt.Sprintf("%s", propertySetID), Items: items})
	}

	// Parse Stonecutter entries (format: IDSet + SlotDisplay)
	var numStonecutterEntries pk.VarInt
	if _, err := numStonecutterEntries.ReadFrom(r); err != nil {
		log.Printf("[recipes %s] ERROR: failed to read stonecutter entry count: %v", a.client.Name(), err)
		return nil
	}
	log.Printf("[recipes %s] Stonecutter Entries Count: %d", a.client.Name(), numStonecutterEntries)

	for i := 0; i < int(numStonecutterEntries); i++ {
		log.Printf("[recipes %s] Stonecutter Entry %d:", a.client.Name(), i+1)

		// Parse IDSet (ingredients)
		idSet, err := scanIDSet(r)
		if err != nil {
			log.Printf("[recipes %s] ERROR: parsing IDSet for stonecutter entry %d: %v", a.client.Name(), i+1, err)
			return nil
		}
		log.Printf("[recipes %s]   Ingredients IDSet mode=%d, ids=%v", a.client.Name(), idSet.Mode, idSet.IDs)

		// Parse SlotDisplay (result)
		result, err := a.parseSlotDisplay(r, 1)
		if err != nil {
			log.Printf("[recipes %s] ERROR: parsing slot display for stonecutter entry %d: %v", a.client.Name(), i+1, err)
			return nil
		}

		// For compatibility, convert OLD format to our internal structure
		// Input: create a composite SlotDisplay from the IDSet
		// Results: single result from the parsed SlotDisplay
		var input SlotDisplay
		switch idSet.Mode {
		case IDSetEmpty:
			input = SlotDisplay{Type: SlotDisplayTypeEmpty}
		case IDSetSingle:
			if len(idSet.IDs) > 0 {
				input = SlotDisplay{Type: SlotDisplayTypeItem, Item: &SlotDisplayItem{ItemID: idSet.IDs[0]}}
			}
		case IDSetList:
			// Create composite with multiple item options
			options := make([]SlotDisplay, len(idSet.IDs))
			for idx, itemID := range idSet.IDs {
				options[idx] = SlotDisplay{Type: SlotDisplayTypeItem, Item: &SlotDisplayItem{ItemID: itemID}}
			}
			input = SlotDisplay{Type: SlotDisplayTypeComposite, Composite: options}
		}

		payload.StonecutterEntries = append(payload.StonecutterEntries, StonecutterEntry{
			Input:   input,
			Results: []SlotDisplay{result},
		})
	}

	log.Printf("[recipes %s] === Finished Update Recipes Packet Parse ===", a.client.Name())
	// Store payload
	a.recipesMu.Lock()
	a.lastUpdateRecipes = &payload
	a.recipesMu.Unlock()
	return nil
}

// parseSlotDisplay reads and logs a Slot Display structure recursively.
// Depth controls indentation for nested structures.
func (a *agent) parseSlotDisplay(r *bytes.Reader, depth int) (SlotDisplay, error) {
	indent := strings.Repeat("  ", depth)
	var slotDisplayType pk.VarInt
	if _, err := slotDisplayType.ReadFrom(r); err != nil {
		return SlotDisplay{}, err
	}
	log.Printf("[recipes %s]%sSlot Display Type: %d", a.client.Name(), indent, slotDisplayType)

	switch int(slotDisplayType) {
	case 0:
		log.Printf("[recipes %s]%s(empty)", a.client.Name(), indent)
		return SlotDisplay{Type: SlotDisplayTypeEmpty}, nil
	case 1:
		log.Printf("[recipes %s]%s(any_fuel)", a.client.Name(), indent)
		return SlotDisplay{Type: SlotDisplayTypeAnyFuel}, nil
	case 2:
		// minecraft:item -> item registry VarInt
		var itemID pk.VarInt
		if _, err := itemID.ReadFrom(r); err != nil {
			return SlotDisplay{}, err
		}
		log.Printf("[recipes %s]%sitem: id=%d", a.client.Name(), indent, itemID)
		return SlotDisplay{Type: SlotDisplayTypeItem, Item: &SlotDisplayItem{ItemID: int32(itemID)}}, nil
	case 3:
		// minecraft:item_stack -> Slot
		var s mcscreen.Slot
		if _, err := s.ReadFrom(r); err != nil {
			return SlotDisplay{}, err
		}
		if s.Count <= 0 {
			log.Printf("[recipes %s]%sitem_stack: empty", a.client.Name(), indent)
			return SlotDisplay{Type: SlotDisplayTypeItemStack, ItemStack: &SlotDisplayItemStack{ItemID: 0, Count: 0}}, nil
		}
		log.Printf("[recipes %s]%sitem_stack: id=%d count=%d", a.client.Name(), indent, s.ID, s.Count)
		return SlotDisplay{Type: SlotDisplayTypeItemStack, ItemStack: &SlotDisplayItemStack{ItemID: int32(s.ID), Count: int32(s.Count)}}, nil
	case 4:
		// minecraft:tag -> Identifier
		var tag pk.Identifier
		if _, err := tag.ReadFrom(r); err != nil {
			return SlotDisplay{}, err
		}
		str := fmt.Sprintf("%s", tag)
		log.Printf("[recipes %s]%stag: %s", a.client.Name(), indent, str)
		return SlotDisplay{Type: SlotDisplayTypeTag, Tag: &str}, nil
	case 5:
		// minecraft:smithing_trim -> Base SlotDisplay, Material SlotDisplay, Pattern VarInt
		log.Printf("[recipes %s]%ssmithing_trim:", a.client.Name(), indent)
		base, err := a.parseSlotDisplay(r, depth+1)
		if err != nil {
			return SlotDisplay{}, err
		}
		material, err := a.parseSlotDisplay(r, depth+1)
		if err != nil {
			return SlotDisplay{}, err
		}
		var pattern pk.VarInt
		if _, err := pattern.ReadFrom(r); err != nil {
			return SlotDisplay{}, err
		}
		log.Printf("[recipes %s]%s  pattern_id=%d", a.client.Name(), indent, pattern)
		return SlotDisplay{Type: SlotDisplayTypeSmithingTrim, SmithingTrim: &SlotDisplaySmithingTrim{Base: base, Material: material, Pattern: int32(pattern)}}, nil
	case 6:
		// minecraft:with_remainder -> Ingredient SlotDisplay, Remainder SlotDisplay
		log.Printf("[recipes %s]%swith_remainder:", a.client.Name(), indent)
		ing, err := a.parseSlotDisplay(r, depth+1)
		if err != nil {
			return SlotDisplay{}, err
		}
		rem, err := a.parseSlotDisplay(r, depth+1)
		if err != nil {
			return SlotDisplay{}, err
		}
		return SlotDisplay{Type: SlotDisplayTypeWithRemainder, WithRemainder: &SlotDisplayWithRemainder{Ingredient: ing, Remainder: rem}}, nil
	case 7:
		// minecraft:composite -> VarInt count + that many SlotDisplays
		var count pk.VarInt
		if _, err := count.ReadFrom(r); err != nil {
			return SlotDisplay{}, err
		}
		log.Printf("[recipes %s]%scomposite: options=%d", a.client.Name(), indent, count)
		options := make([]SlotDisplay, int(count))
		for i := 0; i < int(count); i++ {
			opt, err := a.parseSlotDisplay(r, depth+1)
			if err != nil {
				return SlotDisplay{}, err
			}
			options[i] = opt
		}
		return SlotDisplay{Type: SlotDisplayTypeComposite, Composite: options}, nil
	default:
		log.Printf("[recipes %s]%sunknown slot display type: %d", a.client.Name(), indent, slotDisplayType)
		return SlotDisplay{Type: SlotDisplayType(slotDisplayType)}, nil
	}
}

// worldPacketHandlers returns packet handlers for world packets when using mc-agent world.
// These handlers use the version handler to parse packets and feed data to the mc-agent world manager.
func (a *agent) worldPacketHandlers() []bot.PacketHandler {
	// Only register handlers when using mc-agent world with version handler
	if a.versionHandler == nil || a.mcAgentWorld == nil {
		log.Printf("[Agent %s] worldPacketHandlers: NOT registering (versionHandler=%v, mcAgentWorld=%v)",
			a.client.Name(), a.versionHandler != nil, a.mcAgentWorld != nil)
		return nil
	}

	log.Printf("[Agent %s] worldPacketHandlers: Registering world packet handlers", a.client.Name())
	worldHandler := a.versionHandler.Play().World()

	chunkPacketID := a.packetMgr.GetClientboundPacketID("ClientboundLevelChunkWithLight")
	blockUpdateID := a.packetMgr.GetClientboundPacketID("ClientboundBlockUpdate")
	log.Printf("[Agent %s] Registering chunk handler for packet ID %d", a.client.Name(), chunkPacketID)
	log.Printf("[Agent %s] Registering block update handler for packet ID %d", a.client.Name(), blockUpdateID)

	handlers := []bot.PacketHandler{
		{
			ID:       chunkPacketID,
			Priority: 50, // Higher priority than other handlers to process chunk data first
			F: func(p pk.Packet) error {
				log.Printf("[Agent %s] Received ClientboundLevelChunkWithLight packet", a.client.Name())
				chunkX, chunkZ, data, err := worldHandler.ParseChunkData(p)
				if err != nil {
					log.Printf("[Agent %s] Warning: failed to parse chunk data: %v", a.client.Name(), err)
					return nil // Don't fail on parse errors
				}
				log.Printf("[Agent %s] Loaded chunk at (%d, %d), data size: %d", a.client.Name(), chunkX, chunkZ, len(data))
				return a.mcAgentWorld.HandleChunkLoad(chunkX, chunkZ, data)
			},
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundForgetLevelChunk"),
			Priority: 50,
			F: func(p pk.Packet) error {
				chunkX, chunkZ, err := worldHandler.ParseUnloadChunk(p)
				if err != nil {
					log.Printf("[Agent %s] Warning: failed to parse unload chunk: %v", a.client.Name(), err)
					return nil
				}
				return a.mcAgentWorld.HandleChunkUnload(chunkX, chunkZ)
			},
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundBlockUpdate"),
			Priority: 50,
			F: func(p pk.Packet) error {
				x, y, z, blockStateID, err := worldHandler.ParseBlockUpdate(p)
				if err != nil {
					log.Printf("[Agent %s] Warning: failed to parse blocks update: %v", a.client.Name(), err)
					return nil // Ignore parse errors
				}
				log.Printf("[Agent %s] Block update at (%d, %d, %d) -> state %d", a.client.Name(), x, y, z, blockStateID)
				a.mcAgentWorld.HandleBlockUpdate(x, y, z, blockStateID)
				return nil
			},
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSectionBlocksUpdate"),
			Priority: 50,
			F: func(p pk.Packet) error {
				_, blocks, err := worldHandler.ParseSectionBlocksUpdate(p)
				if err != nil {
					log.Printf("[Agent %s] Warning: failed to parse section blocks update: %v", a.client.Name(), err)
					return nil // Ignore parse errors
				}
				a.mcAgentWorld.HandleSectionBlocksUpdate(blocks)
				return nil
			},
		},
		{
			// ChunkBatchFinished: Server signals end of a chunk batch, client must acknowledge
			// to receive more chunks. Required in 1.20.2+ or server stops sending chunks.
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundChunkBatchFinished"),
			Priority: 50,
			F: func(p pk.Packet) error {
				// Increment batch counter
				a.chunkBatchCount++

				// Send acknowledgment immediately
				if err := worldHandler.SendChunkBatchReceived(a.client.Conn(), a.chunkBatchCount); err != nil {
					log.Printf("[Agent %s] Error sending chunk batch acknowledgement: %v", a.client.Name(), err)
					return nil // Don't fail on ack errors
				}

				// Log progress periodically
				if int(a.chunkBatchCount)%10 == 0 || int(a.chunkBatchCount) <= 3 {
					log.Printf("[Agent %s] Acknowledged %d chunk batches", a.client.Name(), int(a.chunkBatchCount))
				}
				return nil
			},
		},
	}

	log.Printf("[Agent %s] worldPacketHandlers: Returning %d world packet handlers", a.client.Name(), len(handlers))
	return handlers
}
