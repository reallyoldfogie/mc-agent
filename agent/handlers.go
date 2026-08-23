package agent

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
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
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundDamageEvent"),
			Name:     "ClientboundDamageEvent",
			Priority: 0,
			F:        a.onDamageEvent,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSetPassengers"),
			Name:     "ClientboundSetPassengers",
			Priority: 0,
			F:        a.onSetPassengers,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundMoveVehicle"),
			Name:     "ClientboundMoveVehicle",
			Priority: 0,
			F:        a.onClientboundMoveVehicle,
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
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundEntityEffect"),
			Name:     "ClientboundEntityEffect",
			Priority: 0,
			F:        a.onEntityEffect,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundRemoveEntityEffect"),
			Name:     "ClientboundRemoveEntityEffect",
			Priority: 0,
			F:        a.onRemoveEntityEffect,
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
			F:        a.onUpdateRecipes,
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
		// NOTE: no separate ClientboundSetEquipment handler. That name is only an
		// alias for ClientboundEntityEquipment (both resolve to the same packet
		// ID), so registering it again would run two handlers for one packet.
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundUpdateTime"),
			Name:     "ClientboundUpdateTime",
			Priority: 0,
			F:        a.onUpdateTime,
		},
	}
	// NOTE: Config-phase handlers (registryHandlers, FinishConfiguration) must NOT
	// be registered here. The event system dispatches by numeric packet ID with no
	// phase awareness, and config-phase IDs collide with play-phase IDs (e.g.
	// config RegistryData=7 vs play TileEntityData=7). This caused play-phase
	// packets to be misinterpreted as registry data, overwriting loaded registries
	// with empty/corrupt entries.
	//
	// Config-phase processing is handled during joinConfiguration:
	//   - RegistryData → onRegistryDataCallback (populates a.registries)
	//   - FinishConfiguration → ack sent by bot client
	// NotifyLoginSeen is called from onClientboundPosition on the first play-phase
	// position packet, which is the correct timing for replay mirror emissions.

	// Include world packet handlers when using mc-agent world with version handler
	handlers = append(handlers, a.worldPacketHandlers()...)

	return handlers
}

// onDisconnect handles cleanup on disconnect packet.
func (a *agent) onDisconnect2(p pk.Packet) error {
	a.setEntityID(-1)

	name := ""
	if a.client != nil {
		name = a.cfg.Name
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
		return fmt.Errorf("missing version handler")
	}

	entityID, entityType, _, uuid, x, y, z, yaw, pitch, velX, velY, velZ, err := a.versionHandler.Play().Entities().ParseAddEntity(p)
	if err != nil {
		return err
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
		var entityTypeStr models.EntityType
		reg := a.GetRegistry("minecraft:entity_type")
		if reg != nil && reg.IsReady() {
			if name, ok := reg.GetNameByID(entityType); ok {
				// Map registry name to EntityType (e.g., "minecraft:player" -> EntityTypePlayer).
				// GetNameByID returns the full namespaced registry name; models.EntityType
				// constants are all bare (no "minecraft:" prefix, see entity_registry.go),
				// so the prefix has to be stripped here for this to ever match one — this
				// was previously cast through unstripped, so entityRegistry.GetEntityType()
				// never matched any EntityType constant for any entity.
				localName := name
				if idx := strings.IndexByte(localName, ':'); idx >= 0 {
					localName = localName[idx+1:]
				}
				entityTypeStr = models.EntityType(localName)
			}
		} else {
			log.Printf("[onAddEntity] Warning: Entity type registry not ready when registering entity %d. Type ID: %d", entityID, entityType)
		}
		if entityTypeStr == "" {
			entityTypeStr = models.EntityTypeUnknown
		}
		a.entityRegistry.RegisterEntity(entityID, entityTypeStr)
		log.Printf("[onAddEntity] Registered entity %d as type %s in metadata handler", entityID, entityTypeStr)
	} else {
		log.Printf("[onAddEntity] Warning: entityRegistry is nil, cannot register entity %d", entityID)
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
							// Target information for hit validation
							targetPos: pending.targetPos,
							hasTarget: pending.hasTarget,
							// Callback timeout tracking
							callbackRegisteredAt: now,
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
		return fmt.Errorf("missing version handler")
	}

	entityID, dx, dy, dz, yaw, pitch, _, err := a.versionHandler.Play().Entities().ParseMoveEntityPosRot(p)
	if err != nil {
		return err
	}

	log.Printf("[onMoveEntityPosRot][%s] Received pos/rot update for entity %d: delta=(%.4f, %.4f, %.4f), yaw=%d, pitch=%d",
		a.cfg.Name, entityID, float64(dx)/(128*32), float64(dy)/(128*32), float64(dz)/(128*32), yaw, pitch)

	// Snapshot entity data (MINIMAL LOCK SCOPE)
	now := time.Now()
	var oldX, oldY, oldZ, newX, newY, newZ float64
	var entityType int32
	var callbackPos *models.V3

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		oldX, oldY, oldZ = e.X, e.Y, e.Z
		// Minecraft encodes position deltas as fixed-point: divide by (128*32=4096) to convert to block units
		e.X += float64(dx) / (128 * 32)
		e.Y += float64(dy) / (128 * 32)
		e.Z += float64(dz) / (128 * 32)
		e.Yaw, e.Pitch = yaw, pitch
		newX, newY, newZ = e.X, e.Y, e.Z
		entityType = e.EntityType

		if dx != 0 || dy != 0 || dz != 0 {
			e.lastServerX, e.lastServerY, e.lastServerZ = oldX, oldY, oldZ
			e.lastServerUpdateTime = e.currentServerUpdateTime
			e.currentServerUpdateTime = now
			e.LastPositionUpdate = now
		}
		if e.Removed {
			e.Removed = false
		}
		callbackPos = &models.V3{X: newX, Y: newY, Z: newZ}
	}
	a.entitiesMu.Unlock()

	// All remaining work OUTSIDE the lock

	if callbackPos == nil {
		log.Printf("[onMoveEntityPosRot] Entity %d NOT found in map!", entityID)
		return nil
	}

	log.Printf("[onMoveEntityPosRot] Entity %d found in map: oldPos=(%.2f,%.2f,%.2f), delta=(%.4f,%.4f,%.4f), newPos=(%.2f,%.2f,%.2f)", entityID, oldX, oldY, oldZ, float64(dx)/(128*32), float64(dy)/(128*32), float64(dz)/(128*32), newX, newY, newZ)

	// Update active projectiles (separate lock, no entity lock held)
	if dx != 0 || dy != 0 || dz != 0 {
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			projInfo.lastServerPos = models.V3{X: oldX, Y: oldY, Z: oldZ}
			projInfo.lastServerTime = projInfo.currentServerTime
			projInfo.currentServerPos = models.V3{X: newX, Y: newY, Z: newZ}
			projInfo.currentServerTime = now
			projInfo.positionHistory = append(projInfo.positionHistory, projInfo.currentServerPos)
			log.Printf("[onMoveEntityPosRot] PROJECTILE: entityID=%d, oldPos=(%.2f,%.2f,%.2f), delta=(%.4f,%.4f,%.4f), newPos=(%.2f,%.2f,%.2f), yaw=%d, pitch=%d",
				entityID, oldX, oldY, oldZ, float64(dx)/(128*32), float64(dy)/(128*32), float64(dz)/(128*32), newX, newY, newZ, yaw, pitch)
		}
		a.activeProjectilesMu.Unlock()
	} else {
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			projInfo.currentServerTime = now
			log.Printf("[onMoveEntityPosRot] PROJECTILE: entityID=%d, no position delta, updated time", entityID)
		}
		a.activeProjectilesMu.Unlock()
	}

	// Debug logging (no locks held)
	if dx != 0 || dy != 0 || dz != 0 {
		if reg := a.GetRegistry("minecraft:entity_type"); reg != nil && reg.IsReady() {
			if name, ok := reg.GetNameByID(entityType); ok {
				deltaX := float64(dx) / (128 * 32)
				deltaY := float64(dy) / (128 * 32)
				deltaZ := float64(dz) / (128 * 32)
				log.Printf("[onMoveEntityPosRot] %s: entityID=%d, oldPos=(%.2f, %.2f, %.2f), delta=(%.4f, %.4f, %.4f), newPos=(%.2f, %.2f, %.2f), vel/tick=(%.4f, %.4f, %.4f), yaw=%d, pitch=%d",
					name, entityID, oldX, oldY, oldZ, deltaX, deltaY, deltaZ, newX, newY, newZ, deltaX, deltaY, deltaZ, yaw, pitch)
			}
		}
	}

	// Sync mounted entity position (NO LOCK HELD - can block on physics calls)
	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()
	if moveExec != nil {
		if isMountable, ok := moveExec.(interface{ GetMountedEntityID() int32 }); ok {
			if isMountable.GetMountedEntityID() == entityID {
				// Determine rotation type (minecart/boat are independent)
				isIndependentRotation := false
				if reg := a.GetRegistry("minecraft:entity_type"); reg != nil && reg.IsReady() {
					if name, ok := reg.GetNameByID(entityType); ok {
						isIndependentRotation = (name == "minecraft:minecart" || name == "minecraft:boat")
					}
				}

				if isIndependentRotation {
					if syncer, ok := moveExec.(interface {
						SyncMountedPosition(float64, float64, float64)
					}); ok {
						syncer.SyncMountedPosition(newX, newY, newZ)
						log.Printf("[onMoveEntityPosRot] Mounted minecart/boat %d position synced (rotation independent): (%.2f, %.2f, %.2f)", entityID, newX, newY, newZ)
					}
				} else {
					if syncer, ok := moveExec.(interface {
						SyncMountedPositionWithRotation(float64, float64, float64, float64, float64)
					}); ok {
						yawDegrees := float64(yaw) * 360.0 / 256.0
						pitchDegrees := float64(pitch) * 360.0 / 256.0
						syncer.SyncMountedPositionWithRotation(newX, newY, newZ, yawDegrees, pitchDegrees)
						log.Printf("[onMoveEntityPosRot] Mounted entity %d position synced: (%.2f, %.2f, %.2f) yaw=%.1f° pitch=%.1f°", entityID, newX, newY, newZ, yawDegrees, pitchDegrees)
					}
				}
			}
		}
	}

	// Call position update callbacks (outside all locks)
	log.Printf("[onMoveEntityPosRot] Calling position callbacks for entity %d at (%.2f, %.2f, %.2f)", entityID, callbackPos.X, callbackPos.Y, callbackPos.Z)
	a.callEntityPositionCallbacks(entityID, callbackPos.X, callbackPos.Y, callbackPos.Z)
	return nil
}

// onMoveEntityPos updates incremental position without rotation.
func (a *agent) onMoveEntityPos(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, dx, dy, dz, _, err := a.versionHandler.Play().Entities().ParseMoveEntityPos(p)
	if err != nil {
		return err
	}

	// Snapshot entity data (MINIMAL LOCK SCOPE)
	now := time.Now()
	var oldX, oldY, oldZ, newX, newY, newZ float64
	var callbackPos *models.V3

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		oldX, oldY, oldZ = e.X, e.Y, e.Z
		e.X += float64(dx) / (128 * 32)
		e.Y += float64(dy) / (128 * 32)
		e.Z += float64(dz) / (128 * 32)
		newX, newY, newZ = e.X, e.Y, e.Z

		if dx != 0 || dy != 0 || dz != 0 {
			e.lastServerX, e.lastServerY, e.lastServerZ = oldX, oldY, oldZ
			e.lastServerUpdateTime = e.currentServerUpdateTime
			e.currentServerUpdateTime = now
			e.LastPositionUpdate = now
		}
		if e.Removed {
			e.Removed = false
		}
		callbackPos = &models.V3{X: newX, Y: newY, Z: newZ}
	}
	a.entitiesMu.Unlock()

	// All remaining work OUTSIDE the lock

	// Update active projectiles (separate lock, no entity lock held)
	if dx != 0 || dy != 0 || dz != 0 {
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			projInfo.lastServerPos = models.V3{X: oldX, Y: oldY, Z: oldZ}
			projInfo.lastServerTime = projInfo.currentServerTime
			projInfo.currentServerPos = models.V3{X: newX, Y: newY, Z: newZ}
			projInfo.currentServerTime = now
			projInfo.positionHistory = append(projInfo.positionHistory, projInfo.currentServerPos)
			log.Printf("[onMoveEntityPos] PROJECTILE: entityID=%d, oldPos=(%.2f,%.2f,%.2f), delta=(%.4f,%.4f,%.4f), newPos=(%.2f,%.2f,%.2f)",
				entityID, oldX, oldY, oldZ, float64(dx)/(128*32), float64(dy)/(128*32), float64(dz)/(128*32), newX, newY, newZ)
		}
		a.activeProjectilesMu.Unlock()
	} else {
		a.activeProjectilesMu.Lock()
		if projInfo, exists := a.activeProjectiles[entityID]; exists {
			projInfo.currentServerTime = now
			log.Printf("[onMoveEntityPos] PROJECTILE: entityID=%d, no position delta, updated time", entityID)
		}
		a.activeProjectilesMu.Unlock()
	}

	// Handle projectile not in entities map
	if callbackPos == nil {
		a.activeProjectilesMu.Lock()
		if _, exists := a.activeProjectiles[entityID]; exists {
			log.Printf("[onMoveEntityPos] PROJECTILE entityID=%d not in entities map yet! delta=(%.4f,%.4f,%.4f)",
				entityID, float64(dx)/(128*32), float64(dy)/(128*32), float64(dz)/(128*32))
		}
		a.activeProjectilesMu.Unlock()
		return nil
	}

	// Sync mounted entity position (NO LOCK HELD - can block on physics calls)
	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()
	if moveExec != nil {
		if isMountable, ok := moveExec.(interface{ GetMountedEntityID() int32 }); ok {
			if isMountable.GetMountedEntityID() == entityID {
				if syncer, ok := moveExec.(interface {
					SyncMountedPosition(float64, float64, float64)
				}); ok {
					syncer.SyncMountedPosition(newX, newY, newZ)
					log.Printf("[onMoveEntityPos] Mounted entity %d position synced: (%.2f, %.2f, %.2f)", entityID, newX, newY, newZ)
				}
			}
		}
	}

	// Call position update callbacks (outside all locks)
	log.Printf("[onMoveEntityPos] Calling position callbacks for entity %d at (%.2f, %.2f, %.2f)", entityID, callbackPos.X, callbackPos.Y, callbackPos.Z)
	a.callEntityPositionCallbacks(entityID, callbackPos.X, callbackPos.Y, callbackPos.Z)
	return nil
}

// onSyncEntityPosition handles absolute position sync packets.
// This packet provides absolute coordinates and is used for explicit position synchronization.
func (a *agent) onSyncEntityPosition(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, x, y, z, dx, dy, dz, yaw, pitch, onGround, err := a.versionHandler.Play().Entities().ParseSyncEntityPosition(p)
	if err != nil {
		return err
	}

	log.Printf("[onSyncEntityPosition][%s] Received sync for entity %d: pos=(%.2f, %.2f, %.2f), vel=(%.4f, %.4f, %.4f), yaw=%d, pitch=%d, onGround=%v",
		a.cfg.Name, entityID, x, y, z, dx, dy, dz, yaw, pitch, onGround)

	// Snapshot entity data (MINIMAL LOCK SCOPE)
	now := time.Now()
	var oldX, oldY, oldZ, oldServerX, oldServerY, oldServerZ float64
	var oldLastServerUpdateTime time.Time
	var entityType int32
	var callbackPos *models.V3

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		oldX, oldY, oldZ = e.X, e.Y, e.Z
		oldServerX, oldServerY, oldServerZ = e.lastServerX, e.lastServerY, e.lastServerZ
		oldLastServerUpdateTime = e.lastServerUpdateTime
		entityType = e.EntityType

		e.lastServerX, e.lastServerY, e.lastServerZ = e.X, e.Y, e.Z
		e.lastServerUpdateTime = e.currentServerUpdateTime
		e.X, e.Y, e.Z = x, y, z
		e.Yaw, e.Pitch = yaw, pitch
		e.currentServerUpdateTime = now
		e.LastPositionUpdate = now

		if e.Removed {
			e.Removed = false
		}
		callbackPos = &models.V3{X: x, Y: y, Z: z}
	}
	a.entitiesMu.Unlock()

	// All remaining work OUTSIDE the lock

	// Debug logging (no locks held)
	if reg := a.GetRegistry("minecraft:entity_type"); reg != nil && reg.IsReady() {
		if name, ok := reg.GetNameByID(entityType); ok && name == "minecraft:arrow" {
			log.Printf("[onSyncEntityPosition] ARROW: entityID=%d, oldPos=(%.2f, %.2f, %.2f), newPos=(%.2f, %.2f, %.2f), vel=(%.4f, %.4f, %.4f), ground=%v",
				entityID, oldX, oldY, oldZ, x, y, z, dx, dy, dz, onGround)
		}
	}

	// Update active projectiles (separate lock, no entity lock held)
	a.activeProjectilesMu.Lock()
	if projInfo, exists := a.activeProjectiles[entityID]; exists {
		projInfo.lastServerPos = models.V3{X: oldServerX, Y: oldServerY, Z: oldServerZ}
		projInfo.lastServerTime = oldLastServerUpdateTime
		projInfo.currentServerPos = models.V3{X: x, Y: y, Z: z}
		projInfo.currentServerTime = now
		projInfo.positionHistory = append(projInfo.positionHistory, projInfo.currentServerPos)
		log.Printf("[onSyncEntityPosition] PROJECTILE: entityID=%d, oldPos=(%.2f,%.2f,%.2f), newPos=(%.2f,%.2f,%.2f)",
			entityID, oldServerX, oldServerY, oldServerZ, x, y, z)

		// Check if this projectile has a pending callback waiting for server position confirmation
		if projInfo.pendingCallbackFire && !projInfo.callbacksFired && len(projInfo.callbacks) > 0 {
			serverPos := models.V3{X: x, Y: y, Z: z}
			flightTime := now.Sub(projInfo.spawnTime).Seconds()

			hitResult := models.ProjectileResultBlock
			hitEntityID := int32(-1)
			if projInfo.pendingHitType == models.ProjectileHitEntity {
				hitResult = models.ProjectileResultEntity
				hitEntityID = entityID
			}
			evt := models.ProjectileHitEvent{
				ProjectileEntityID: entityID,
				HitType:            projInfo.pendingHitType,
				ProjectileType:     projInfo.projectileType,
				Position:           serverPos,
				FiredAt:            projInfo.firedAt,
				HitAt:              now,
				HitResult:          hitResult,
				HitEntityID:        hitEntityID,
			}

			for _, cb := range projInfo.callbacks {
				go cb(evt)
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

	// Call position update callbacks (outside all locks)
	if callbackPos != nil {
		a.callEntityPositionCallbacks(entityID, callbackPos.X, callbackPos.Y, callbackPos.Z)
	}
	return nil
}

// onTeleportEntity handles absolute teleports.
func (a *agent) onTeleportEntity(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, x, y, z, yaw, pitch, _, err := a.versionHandler.Play().Entities().ParseTeleportEntity(p)
	if err != nil {
		return err
	}

	// Snapshot entity data (MINIMAL LOCK SCOPE)
	now := time.Now()
	var oldX, oldY, oldZ, oldServerX, oldServerY, oldServerZ float64
	var oldLastServerUpdateTime time.Time
	var entityType int32
	var callbackPos *models.V3

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		oldX, oldY, oldZ = e.X, e.Y, e.Z
		oldServerX, oldServerY, oldServerZ = e.lastServerX, e.lastServerY, e.lastServerZ
		oldLastServerUpdateTime = e.lastServerUpdateTime
		entityType = e.EntityType

		e.lastServerX, e.lastServerY, e.lastServerZ = e.X, e.Y, e.Z
		e.lastServerUpdateTime = e.currentServerUpdateTime
		e.X, e.Y, e.Z = x, y, z
		e.currentServerUpdateTime = now
		e.Yaw, e.Pitch = yaw, pitch
		e.LastPositionUpdate = now

		if e.Removed {
			e.Removed = false
		}
		callbackPos = &models.V3{X: x, Y: y, Z: z}
	}
	a.entitiesMu.Unlock()

	// All remaining work OUTSIDE the lock

	// Debug logging (no locks held)
	if reg := a.GetRegistry("minecraft:entity_type"); reg != nil && reg.IsReady() {
		if name, ok := reg.GetNameByID(entityType); ok && name == "minecraft:arrow" {
			log.Printf("[onTeleportEntity] ARROW: entityID=%d, oldPos=(%.2f, %.2f, %.2f), newPos=(%.2f, %.2f, %.2f), yaw=%d, pitch=%d",
				entityID, oldX, oldY, oldZ, x, y, z, yaw, pitch)
		}
	}

	// Update active projectiles (separate lock, no entity lock held)
	a.activeProjectilesMu.Lock()
	if projInfo, exists := a.activeProjectiles[entityID]; exists {
		projInfo.lastServerPos = models.V3{X: oldServerX, Y: oldServerY, Z: oldServerZ}
		projInfo.lastServerTime = oldLastServerUpdateTime
		projInfo.currentServerPos = models.V3{X: x, Y: y, Z: z}
		projInfo.currentServerTime = now
	}
	a.activeProjectilesMu.Unlock()

	// Call position update callbacks (outside all locks)
	if callbackPos != nil {
		a.callEntityPositionCallbacks(entityID, callbackPos.X, callbackPos.Y, callbackPos.Z)
	}
	return nil
}

// onEntityVelocityUpdate handles velocity changes for entities in flight.
// For projectiles, velocity updates allow tracking movement when position packets have zero deltas.
func (a *agent) onEntityVelocityUpdate(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, velX, velY, velZ, err := a.versionHandler.Play().Entities().ParseEntityVelocityUpdate(p)
	if err != nil {
		return err
	}
	botEntityID := a.GetEntityID()
	if entityID == botEntityID {
		err = a.moveExec.SetVelocity(velX, velY, velZ)
		if err != nil {
			log.Printf("[onEntityVelocityUpdate] Error setting bot velocity: %v", err)
		} else {
			log.Printf("[onEntityVelocityUpdate] EntityID=%d velocity update applied to bot: (%.4f, %.4f, %.4f)", entityID, velX, velY, velZ)
		}
	}

	// Check if this entity is the mounted vehicle
	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()
	if moveExec != nil {
		if isMountable, ok := moveExec.(interface{ GetMountedEntityID() int32 }); ok {
			if isMountable.GetMountedEntityID() == entityID {
				if setter, ok := moveExec.(interface {
					SetMountedVelocity(float64, float64, float64)
				}); ok {
					setter.SetMountedVelocity(velX, velY, velZ)
					log.Printf("[onEntityVelocityUpdate] Mounted entity %d velocity synced: (%.4f, %.4f, %.4f)", entityID, velX, velY, velZ)
				}
			}
		}
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

// onDamageEvent handles ClientboundDamageEvent packets.
// When the agent is damaged, this calculates and applies knockback based on the damage source.
// Knockback direction is determined from either:
//  1. The tracked position of the attacking entity (sourceDirectID or sourceCauseID)
//  2. The explicit source position provided in the packet
//
// This complements onEntityVelocityUpdate which handles server-applied velocity (used for
// knockback enchantment levels > 0, explosions, etc.). The DamageEvent handler provides
// a fallback for cases where the server doesn't send a velocity packet (e.g. fist attacks
// with no knockback enchantment that still cause visual/physics knockback).
func (a *agent) onDamageEvent(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, sourceTypeID, sourceCauseID, sourceDirectID, sourceX, sourceY, sourceZ, hasSourcePosition, err :=
		a.versionHandler.Play().Entities().ParseDamageEvent(p)
	if err != nil {
		return err
	}

	// Only process knockback for the agent itself
	botEntityID := a.GetEntityID()
	if entityID != botEntityID {
		return nil
	}

	log.Printf("[onDamageEvent] Agent damaged: sourceType=%d causedBy=%d directBy=%d hasPos=%v pos=(%.2f,%.2f,%.2f)",
		sourceTypeID, sourceCauseID, sourceDirectID, hasSourcePosition, sourceX, sourceY, sourceZ)

	// Determine the attacker position for knockback direction.
	// Priority: sourceDirectID entity position > sourceCauseID entity position > explicit source position.
	var attackerX, attackerZ float64
	var hasAttackerPos bool

	// Try the direct source entity first (e.g. the arrow or the player hitting us)
	if sourceDirectID >= 0 {
		a.entitiesMu.RLock()
		if attackerEntity, ok := a.entities[sourceDirectID]; ok {
			attackerX = attackerEntity.X
			attackerZ = attackerEntity.Z
			hasAttackerPos = true
		}
		a.entitiesMu.RUnlock()
	}

	// Fall back to the cause entity (e.g. the player who shot the arrow)
	if !hasAttackerPos && sourceCauseID >= 0 {
		a.entitiesMu.RLock()
		if causeEntity, ok := a.entities[sourceCauseID]; ok {
			attackerX = causeEntity.X
			attackerZ = causeEntity.Z
			hasAttackerPos = true
		}
		a.entitiesMu.RUnlock()
	}

	// Fall back to the explicit source position from the packet
	if !hasAttackerPos && hasSourcePosition {
		attackerX = sourceX
		attackerZ = sourceZ
		hasAttackerPos = true
	}

	if !hasAttackerPos {
		log.Printf("[onDamageEvent] No attacker position available for knockback calculation")
		return nil
	}

	// Calculate knockback direction: attacker → agent (push away from attacker)
	agentPos, _, _, initialized := a.GetPosition()
	if !initialized {
		return nil
	}
	agentX, agentZ := agentPos.X, agentPos.Z

	dirX := agentX - attackerX
	dirZ := agentZ - attackerZ

	// Normalize the direction vector
	magnitude := math.Sqrt(dirX*dirX + dirZ*dirZ)
	if magnitude < 0.01 {
		// Attacker at same position — no meaningful knockback direction
		log.Printf("[onDamageEvent] Attacker too close for directional knockback (dist=%.4f)", magnitude)
		return nil
	}
	dirX /= magnitude
	dirZ /= magnitude

	// Apply knockback velocity (vanilla: 0.4 horizontal, 0.4 vertical)
	knockbackX := dirX * physics.KnockbackHorizontalStrength
	knockbackZ := dirZ * physics.KnockbackHorizontalStrength
	knockbackY := physics.KnockbackVerticalStrength

	if err := a.moveExec.SetVelocity(knockbackX, knockbackY, knockbackZ); err != nil {
		log.Printf("[onDamageEvent] Error applying knockback velocity: %v", err)
		return nil
	}

	log.Printf("[onDamageEvent] Applied knockback: vel=(%.4f, %.4f, %.4f) dir=(%.2f, %.2f) from attacker at (%.2f, %.2f)",
		knockbackX, knockbackY, knockbackZ, dirX, dirZ, attackerX, attackerZ)

	return nil
}

// onRemoveEntities marks entities as softly removed, allowing grace period before purge.
// Also fires projectile hit callbacks if a tracked projectile was removed.
func (a *agent) onRemoveEntities(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityIDs, err := a.versionHandler.Play().Entities().ParseRemoveEntities(p)
	if err != nil {
		return err
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

	// If the removed entity is the vehicle we are currently riding, trigger a
	// clean dismount. This mirrors the vanilla client's removedPlayerVehicleId
	// mechanism: the server may remove the vehicle entity and follow up with a
	// SetPassengers or teleport. Without this, the agent stays mounted
	// indefinitely on a vehicle that no longer exists.
	currentMount := a.getMountedEntityID()
	for _, id := range entityIDs {
		if id == currentMount {
			log.Printf("[onRemoveEntities] Mounted vehicle %d was removed; triggering clean dismount", id)
			a.setMountedEntity(-1, -1)
			a.movementMu.RLock()
			moveExec := a.moveExec
			a.movementMu.RUnlock()
			if moveExec != nil {
				if err := moveExec.SetDismounted(); err != nil {
					log.Printf("[onRemoveEntities] Error clearing mounted state: %v", err)
				}
			}
			break
		}
	}

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

				// For non-persistent projectiles (wind charges, snowballs, etc.), the server doesn't send
				// a position update at the exact impact point - it just removes the entity.
				// We need to estimate how far the projectile traveled from the last server position to impact.
				if !projInfo.projectileType.IsPersistent() {
					timeSinceLastPos := now.Sub(projInfo.currentServerTime).Seconds()
					if timeSinceLastPos > 0 {
						// Use physics simulation to estimate position at impact time
						estimatedPos := a.InterpolateProjectilePosition(projInfo, now.Sub(projInfo.spawnTime).Seconds())
						positionSource = fmt.Sprintf("server position + velocity estimate (%.3fs after last update)", timeSinceLastPos)
						log.Printf("[onRemoveEntities] Projectile entityID=%d type=%s non-persistent: server pos=(%.2f,%.2f,%.2f) @ %.3fs ago, estimated impact pos=(%.2f,%.2f,%.2f)",
							id, projInfo.projectileType, projInfo.currentServerPos.X, projInfo.currentServerPos.Y, projInfo.currentServerPos.Z,
							timeSinceLastPos, estimatedPos.X, estimatedPos.Y, estimatedPos.Z)
						pos = estimatedPos
					}
				}
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

			// Determine hit result type
			hitResult := models.ProjectileResultBlock
			hitEntityID := int32(-1)
			if hitType == models.ProjectileHitEntity {
				hitResult = models.ProjectileResultEntity
				// Try to infer which entity was hit by finding nearest entity to projectile's last position
				// Only consider entities that are close enough to have been hit
				const hitDetectionRadius = 2.0 // Entities within 2 blocks of projectile position
				nearestEntityID := int32(-1)
				nearestDistance := hitDetectionRadius

				a.entitiesMu.RLock()
				for eid, entity := range a.entities {
					// Skip if it's the projectile itself or if removed
					if eid == id || entity.Removed {
						continue
					}
					// Calculate distance from projectile position to entity
					dx := pos.X - entity.X
					dy := pos.Y - entity.Y
					dz := pos.Z - entity.Z
					dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
					if dist < nearestDistance {
						nearestDistance = dist
						nearestEntityID = eid
					}
				}
				a.entitiesMu.RUnlock()

				if nearestEntityID >= 0 {
					hitEntityID = nearestEntityID
					log.Printf("[onRemoveEntities] Inferred hit entity: entityID=%d at distance %.2f blocks from projectile position", nearestEntityID, nearestDistance)
				} else {
					log.Printf("[onRemoveEntities] Entity hit but no nearby entities found to infer target (within %.1f blocks)", hitDetectionRadius)
				}
			}

			// Compute hit validation fields
			var hitDistance float64
			var trajectoryHit, isValidHit bool

			if projInfo.hasTarget {
				hitDistance = pos.DistanceTo(projInfo.targetPos)
				radius := projInfo.projectileType.HitAcceptanceRadius()
				isDirectHit := hitDistance <= radius

				if !isDirectHit {
					var checkTraj []models.TrajectoryPoint
					if len(projInfo.positionHistory) >= 2 {
						// Use server-confirmed positions for persistent projectiles
						for _, p := range projInfo.positionHistory {
							checkTraj = append(checkTraj, models.TrajectoryPoint{Pos: p})
						}
					} else if !projInfo.spawnTime.IsZero() {
						// Simulate from spawn data for non-persistent projectiles
						flightTicks := int(now.Sub(projInfo.spawnTime).Seconds() * 20)
						if flightTicks > 0 {
							checkTraj = physics.SimulateProjectileTrajectory(
								projInfo.projectileType, projInfo.spawnPos, projInfo.spawnVelocity, flightTicks)
						}
					}
					if len(checkTraj) > 0 {
						trajectoryHit = physics.TrajectoryPassesThroughRadius(checkTraj, projInfo.targetPos, radius)
					}
				}

				isValidHit = isDirectHit || trajectoryHit
				log.Printf("[onRemoveEntities] Hit validation: type=%s, hitDist=%.2f, radius=%.2f, "+
					"directHit=%v, trajHit=%v, isValidHit=%v",
					projInfo.projectileType, hitDistance, radius, isDirectHit, trajectoryHit, isValidHit)
			}

			// Fire all callbacks
			evt := models.ProjectileHitEvent{
				ProjectileEntityID: id,
				HitType:            hitType,
				ProjectileType:     projInfo.projectileType,
				Position:           pos,
				FiredAt:            projInfo.firedAt,
				HitAt:              now,
				HitResult:          hitResult,
				HitEntityID:        hitEntityID,
				TargetPos:          projInfo.targetPos,
				TargetSet:          projInfo.hasTarget,
				HitDistance:        hitDistance,
				TrajectoryHit:      trajectoryHit,
				IsValidHit:         isValidHit,
			}
			for _, cb := range projInfo.callbacks {
				go cb(evt) // Fire asynchronously to not block handler
			}

			log.Printf("[onRemoveEntities] Fired projectile hit callbacks: type=%s, hitType=%v, hitResult=%s, count=%d, pos=(%.2f, %.2f, %.2f). Entity tracked pos: X=%.2f, Y=%.2f, Z=%.2f",
				projInfo.projectileType, hitType, hitResult, len(projInfo.callbacks), pos.X, pos.Y, pos.Z, pos.X, pos.Y, pos.Z)

			// Visualize actual entity trajectory
			// For persistent projectiles (arrows, tridents), use recorded server positions
			if len(projInfo.positionHistory) > 0 {
				history := make([]models.V3, len(projInfo.positionHistory))
				copy(history, projInfo.positionHistory)
				go a.visualizeActualEntityTrajectory(projInfo.projectileType, history)
			} else if !projInfo.projectileType.IsPersistent() && !projInfo.spawnTime.IsZero() {
				// For non-persistent projectiles (wind charges, snowballs, eggs, etc.),
				// simulate trajectory from actual spawn velocity
				flightTicks := int(now.Sub(projInfo.spawnTime).Seconds() * 20)
				if flightTicks > 0 {
					simPositions := physics.SimulateProjectileTrajectory(
						projInfo.projectileType, projInfo.spawnPos, projInfo.spawnVelocity, flightTicks)
					positions := make([]models.V3, len(simPositions))
					for i, pt := range simPositions {
						positions[i] = pt.Pos
					}
					go a.visualizeActualEntityTrajectory(projInfo.projectileType, positions)
				}
			}

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
		return fmt.Errorf("missing version handler")
	}

	entityID, entries, err := a.versionHandler.Play().Entities().ParseSetEntityMetadata(p)
	if err != nil {
		log.Printf("[Agent %s][onSetEntityMetadata] entityID=%d", a.cfg.Name, entityID)
		return err
	}

	if projInfo, exists := a.activeProjectiles[entityID]; exists {

		log.Printf("[Agent %s][onSetEntityMetadata] %s entityID=%d %s)", a.cfg.Name, projInfo.projectileType.String(), entityID, spew.Sdump(projInfo))
	}

	// Process metadata through the handler system if available
	var metadataResults []models.MetadataProcessResult
	if a.metadataHandler != nil {
		for _, entry := range entries {
			// Process each metadata entry through the handler
			result, err := a.metadataHandler.HandleMetadata(entityID, entry)
			if err != nil {
				log.Printf("[Agent %s][onSetEntityMetadata] Warning: failed to process metadata for entity %d: %v", a.cfg.Name, entityID, err)
				continue
			}
			metadataResults = append(metadataResults, result)
		}
	}

	// Extract metadata we care about
	health := float32(-1.0)    // -1 indicates not set
	maxHealth := float32(20.0) // Default max health
	isInGround := false        // Arrow projectile state
	var velocity *[3]float64   // Entity velocity from metadata
	shake := int8(0)           // Shake animation counter
	criticalHit := false       // Critical hit flag
	pierceLevel := int8(0)     // Piercing level
	potionColor := int32(-1)   // Potion color := no potion)
	hasPose := false           // Whether any result included a Pose entry
	poseOrdinal := int32(0)    // EntityPose wire varint
	poseName := ""             // Lowercased enum name (or "unknown_<n>")

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
		if result.HasPose {
			hasPose = true
			poseOrdinal = result.Pose
			poseName = result.PoseName
		}
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		// Only update if health was actually provided in metadata
		if health >= 0 {
			log.Printf("[Agent %s] [ParseSetEntityMetadata] Entity %d health updated: %.1f / %.1f", a.cfg.Name, entityID, health, maxHealth)
			e.Health = health
		}
		e.MaxHealth = maxHealth
		// Apply velocity from metadata if present
		if velocity != nil {
			e.VelX = velocity[0]
			e.VelY = velocity[1]
			e.VelZ = velocity[2]
			log.Printf("[Agent %s] [ParseSetEntityMetadata] Entity %d velocity updated: (%.4f, %.4f, %.4f)", a.cfg.Name, entityID, velocity[0], velocity[1], velocity[2])
		}
		// Store projectile-specific metadata
		e.shake = shake
		e.criticalHit = criticalHit
		e.pierceLevel = pierceLevel
		e.potionColor = potionColor

		// Persist pose if this update included a Pose entry. We keep both the
		// raw wire ordinal and the resolved name so consumers (e.g. the
		// mounted-camel logic) can match without re-looking up the registry.
		if hasPose {
			e.Pose = poseOrdinal
			e.PoseName = poseName
			e.HasPose = true
		}

		// Handle boat-specific metadata
		// Check if this is a boat entity type and update paddle/variant metadata
		for _, entry := range entries {
			switch int(entry.Key) {
			case int(models.EntityMetadataKeyBoatVariant):
				if variantVal, ok := entry.Value.(int32); ok {
					e.BoatVariant = models.BoatVariant(variantVal)
				}
			case int(models.EntityMetadataKeyBoatPaddleLeft):
				if paddleVal, ok := entry.Value.(bool); ok {
					e.BoatPaddleLeft = paddleVal
				}
			case int(models.EntityMetadataKeyBoatPaddleRight):
				if paddleVal, ok := entry.Value.(bool); ok {
					e.BoatPaddleRight = paddleVal
				}
			case int(models.EntityMetadataKeyPassiveChild):
				// Key 16 is CHILD on every PassiveEntity-chain mount, but is
				// only captured for happy ghasts right now (needed for the
				// standable-surface isBaby() gate) — gate the same way
				// STAYING_STILL is, so this doesn't misread key 16 on a
				// different entity type where it means something else, or
				// hasn't even been derived for.
				if !a.IsMountedEntityHappyGhast(e.EntityType) {
					continue
				}
				if babyVal, ok := entry.Value.(bool); ok {
					e.IsBaby = babyVal
					e.HasIsBaby = true
					log.Printf("[onSetEntityMetadata] Entity %d happy ghast is_baby=%v", entityID, babyVal)
				}
			case int(models.EntityMetadataKeyHappyGhastStayingStill):
				// Key 18 is only HappyGhastEntity's STAYING_STILL flag on
				// that entity type; other entity types register a different
				// (or no) field at this index, so it has to be gated the
				// same way HORSE_FLAGS is.
				if !a.IsMountedEntityHappyGhast(e.EntityType) {
					continue
				}
				if stillVal, ok := entry.Value.(bool); ok {
					e.HappyGhastStayingStill = stillVal
					e.HasHappyGhastStayingStill = true
					log.Printf("[onSetEntityMetadata] Entity %d happy ghast staying_still=%v", entityID, stillVal)
				}
			case int(models.EntityMetadataKeyHorseFlags):
				// Key 17 is only the horse flags byte on AbstractHorseEntity
				// subclasses; on a player it is the score VarInt, so the entity
				// type has to gate this or we would misread unrelated entities.
				if !a.isSaddleableMountType(e.EntityType) {
					continue
				}
				if flagsVal, ok := entry.Value.(*pk.Byte); ok && flagsVal != nil {
					e.HorseFlags = uint8(*flagsVal)
					e.HasHorseFlags = true
					log.Printf("[onSetEntityMetadata] Entity %d horse flags=0x%02X (saddled=%v tamed=%v)",
						entityID, e.HorseFlags,
						models.HorseFlagSaddled.IsSet(e.HorseFlags),
						models.HorseFlagTamed.IsSet(e.HorseFlags))
				}
			}
		}

		// Update last metadata update timestamp
		e.LastMetadataUpdate = time.Now()
	}
	a.entitiesMu.Unlock()

	// If this entity is the bot's mount and the executor cares about vehicle
	// pose changes (e.g. camel sit/stand), forward the update. The interface
	// assertion keeps this loose so non-physics executors silently skip.
	if hasPose && entityID == a.getMountedEntityID() && a.moveExec != nil {
		if notifier, ok := a.moveExec.(interface {
			NotifyVehiclePose(entityID int32, poseName string, ordinal int32)
		}); ok {
			notifier.NotifyVehiclePose(entityID, poseName, poseOrdinal)
		}
	}

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
		log.Printf("[Agent %s][onSetEntityMetadata] %s entityID=%d isInGround=%v (was %v), shake=%d, critical=%v, pierce=%d, color=%d", a.cfg.Name, projInfo.projectileType.String(), entityID, isInGround, prevInGround, shake, criticalHit, pierceLevel, potionColor)

		// If arrow just hit a block (isInGround transitioned from false to true), queue callback for server-authoritative position
		if isInGround && !prevInGround && !projInfo.callbacksFired && len(projInfo.callbacks) > 0 {
			// For persistent projectiles, queue callback instead of firing immediately with stale client prediction
			if projInfo.projectileType.IsPersistent() {
				// If server has already sent a position update, fire immediately with authoritative position
				if !projInfo.currentServerTime.IsZero() {
					pos := projInfo.currentServerPos
					flightTime := time.Since(projInfo.spawnTime).Seconds()
					log.Printf("[onSetEntityMetadata] Block hit: firing immediately with server position. entityID=%d, flightTime=%.3fs, serverPos=(%.2f,%.2f,%.2f)",
						entityID, flightTime, pos.X, pos.Y, pos.Z)

					evt := models.ProjectileHitEvent{
						ProjectileEntityID: entityID,
						HitType:            models.ProjectileHitBlock,
						ProjectileType:     projInfo.projectileType,
						Position:           pos,
						FiredAt:            projInfo.firedAt,
						HitAt:              time.Now(),
						HitResult:          models.ProjectileResultBlock,
						HitEntityID:        -1,
					}
					for _, cb := range projInfo.callbacks {
						go cb(evt) // Fire asynchronously
					}
					projInfo.callbacksFired = true
				} else {
					// No server position yet — queue and wait for onSyncEntityPosition
					projInfo.pendingCallbackFire = true
					projInfo.pendingHitType = models.ProjectileHitBlock
					projInfo.pendingHitPos = projInfo.interpolatedPos // fallback if server never responds
					projInfo.collisionDetectTime = time.Now()
					log.Printf("[onSetEntityMetadata] Block hit: QUEUED callback (waiting for server position). entityID=%d, clientPos=(%.2f,%.2f,%.2f)",
						entityID, projInfo.interpolatedPos.X, projInfo.interpolatedPos.Y, projInfo.interpolatedPos.Z)
				}
			} else {
				// Non-persistent projectiles: fire immediately with client prediction (no server position available)
				pos := projInfo.interpolatedPos
				flightTime := time.Since(projInfo.spawnTime).Seconds()
				log.Printf("[onSetEntityMetadata] Block hit (non-persistent): firing immediately. entityID=%d, flightTime=%.3fs, pos=(%.2f,%.2f,%.2f)",
					entityID, flightTime, pos.X, pos.Y, pos.Z)

				evt := models.ProjectileHitEvent{
					ProjectileEntityID: entityID,
					HitType:            models.ProjectileHitBlock,
					ProjectileType:     projInfo.projectileType,
					Position:           pos,
					FiredAt:            projInfo.firedAt,
					HitAt:              time.Now(),
					HitResult:          models.ProjectileResultBlock,
					HitEntityID:        -1,
				}
				for _, cb := range projInfo.callbacks {
					go cb(evt) // Fire asynchronously
				}
				projInfo.callbacksFired = true
			}
		}
	}
	a.activeProjectilesMu.Unlock()

	return nil
}

// onEntityStatus handles entity status effects and animations.
// For arrows, detects potion effect expiration.
func (a *agent) onEntityStatus(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, eventID, err := a.versionHandler.Play().Entities().ParseEntityEvent(p)
	if err != nil {
		return err
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
		return fmt.Errorf("missing version handler")
	}

	eventType, x, y, z, value, err := a.versionHandler.Play().ParseGameEvent(p)
	if err != nil {
		return err
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
func (a *agent) onEntityUpdateAttributes(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, attrs, err := a.versionHandler.Play().Entities().ParseEntityUpdateAttributes(p)
	if err != nil {
		return err
	}

	a.entitiesMu.Lock()
	if entity, ok := a.entities[entityID]; ok {
		if entity.Attributes == nil {
			entity.Attributes = make(map[string]models.AttributeValue)
		}
		for key, value := range attrs {
			entity.Attributes[key] = value
		}
		log.Printf("[onEntityUpdateAttributes] Updated attributes for entity %d: %v", entityID, attrs)
	}
	a.entitiesMu.Unlock()

	return nil
}

// onEntityEquipment handles entity equipment changes (held items, armor, saddle).
//
// The packet is also known as ClientboundSetEquipment; both names resolve to
// the same clientbound packet, so only one handler is registered for it.
//
// Equipment is recorded per entity so the riding dispatch can tell whether a
// mount is actually saddled. Vanilla only treats a rider as the controlling
// passenger when the mount is equipped (AbstractHorseEntity.isSaddled), and
// without this the executor drove unsaddled mounts as if they obeyed input.
func (a *agent) onEntityEquipment(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, equipment, err := a.versionHandler.Play().Entities().ParseEntityEquipment(p)
	if err != nil {
		return err
	}

	a.entitiesMu.Lock()
	if entity, ok := a.entities[entityID]; ok {
		if entity.Equipment == nil {
			entity.Equipment = make(map[models.EquipmentSlotType]models.InventorySlot)
		}
		for _, eq := range equipment {
			entity.Equipment[models.EquipmentSlotType(eq.InventorySlot)] = eq.Item
		}
	}
	a.entitiesMu.Unlock()

	for _, eq := range equipment {
		slot := models.EquipmentSlotType(eq.InventorySlot)
		log.Printf("[onEntityEquipment] Entity %d equipment slot %d (%s): itemID=%d, count=%d",
			entityID, eq.InventorySlot, slot, eq.Item.ItemID, eq.Item.Count)
	}
	return nil
}

// effectNameByID resolves a minecraft:mob_effect registry ID to its full
// namespaced name (e.g. "minecraft:slow_falling"), the same way
// GetEntityTypeID resolves entity-type registry IDs elsewhere. Returns
// ("", false) if the registry isn't loaded/ready yet or the ID isn't found —
// registry IDs are not guaranteed stable across versions, so this must never
// be replaced with a hardcoded numeric comparison.
func (a *agent) effectNameByID(effectID int32) (string, bool) {
	reg := a.GetRegistry("minecraft:mob_effect")
	if reg == nil || !reg.IsReady() {
		return "", false
	}
	return reg.GetNameByID(effectID)
}

// onEntityEffect handles a status effect being applied to (or updated on) an
// entity. Effects are recorded per-entity like Attributes/Equipment, and
// additionally cached separately for the agent's own entity (ownEffects),
// since the bot's own player entity is never present in a.entities — the
// server never sends us an AddEntity spawn packet for ourselves. See
// GetOwnActiveEffect, which is what movement/physics_executor.go's walking
// tick reads to apply Slow Falling/Levitation physics (Phase 4a).
func (a *agent) onEntityEffect(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, effectID, amplifier, durationTicks, ambient, showParticles, showIcon, err := a.versionHandler.Play().Entities().ParseEntityEffect(p)
	if err != nil {
		return err
	}

	effectName, ok := a.effectNameByID(effectID)
	if !ok {
		log.Printf("[onEntityEffect] entity=%d effectID=%d could not be resolved via minecraft:mob_effect registry (not ready yet?)", entityID, effectID)
		return nil
	}

	effect := models.ActiveEffect{
		Amplifier:     amplifier,
		DurationTicks: durationTicks,
		Ambient:       ambient,
		ShowParticles: showParticles,
		ShowIcon:      showIcon,
	}

	if entityID == a.GetEntityID() {
		a.ownEffectsMu.Lock()
		if a.ownEffects == nil {
			a.ownEffects = make(map[string]models.ActiveEffect)
		}
		a.ownEffects[effectName] = effect
		a.ownEffectsMu.Unlock()
	}

	a.entitiesMu.Lock()
	if entity, ok := a.entities[entityID]; ok {
		if entity.Effects == nil {
			entity.Effects = make(map[string]models.ActiveEffect)
		}
		entity.Effects[effectName] = effect
	}
	a.entitiesMu.Unlock()

	log.Printf("[onEntityEffect] entity=%d effect=%s amplifier=%d duration=%d ambient=%v particles=%v icon=%v",
		entityID, effectName, amplifier, durationTicks, ambient, showParticles, showIcon)
	return nil
}

// onRemoveEntityEffect handles a status effect being removed from an entity,
// clearing it from both a.entities and (if applicable) ownEffects — see
// onEntityEffect.
func (a *agent) onRemoveEntityEffect(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, effectID, err := a.versionHandler.Play().Entities().ParseRemoveEntityEffect(p)
	if err != nil {
		return err
	}

	effectName, ok := a.effectNameByID(effectID)
	if !ok {
		log.Printf("[onRemoveEntityEffect] entity=%d effectID=%d could not be resolved via minecraft:mob_effect registry (not ready yet?)", entityID, effectID)
		return nil
	}

	if entityID == a.GetEntityID() {
		a.ownEffectsMu.Lock()
		delete(a.ownEffects, effectName)
		a.ownEffectsMu.Unlock()
	}

	a.entitiesMu.Lock()
	if entity, ok := a.entities[entityID]; ok && entity.Effects != nil {
		delete(entity.Effects, effectName)
	}
	a.entitiesMu.Unlock()

	log.Printf("[onRemoveEntityEffect] entity=%d effect=%s", entityID, effectName)
	return nil
}

// onEntityHeadRotation handles entity head rotation updates.
func (a *agent) onEntityHeadRotation(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, headYaw, err := a.versionHandler.Play().Entities().ParseEntityHeadRotation(p)
	if err != nil {
		return err
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
		return fmt.Errorf("missing version handler")
	}

	entityID, yaw, pitch, onGround, err := a.versionHandler.Play().Entities().ParseEntityLook(p)
	if err != nil {
		return err
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
// When the offhand slot (index 45) changes, it emits an EntityEquipment packet
// to the replay mirror so the replay viewer can render the offhand item.
// When the currently-held hotbar slot changes (e.g., item consumed), it also
// re-emits the main hand equipment.
func (a *agent) onSetSlot(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil
	}

	windowID, _, slotIndex, item, err := a.versionHandler.Play().Containers().ParseContainerSetSlot(p)
	if err != nil {
		return nil
	}

	// Keep any tracked entity container snapshot in step with single-slot
	// changes. Negative window IDs are the player's own inventory views, never
	// an entity container.
	if windowID > 0 {
		a.updateEntityWindowSlot(byte(windowID), slotIndex, item)
	}

	// The remainder is replay-mirror bookkeeping only.
	if a.moveMirror == nil {
		return nil
	}

	// Offhand slot is index 45 in the player inventory (windowID 0 or -2)
	const offhandSlotIndex int16 = 45
	if slotIndex == offhandSlotIndex {
		a.moveMirror.EmitEquipment(a.GetEntityID(), models.OffHand, item.ItemID, item.Count)
	}

	// Hotbar slots are 36-44. If the currently-held slot's contents changed,
	// re-emit main hand equipment so the replay stays in sync (e.g., after
	// consuming food, placing blocks, or picking up items).
	const hotbarSlotStart int16 = 36
	const hotbarSlotEnd int16 = 44
	if slotIndex >= hotbarSlotStart && slotIndex <= hotbarSlotEnd {
		a.heldSlotMu.RLock()
		heldSlot := a.heldSlot
		heldSlotSet := a.heldSlotSet
		a.heldSlotMu.RUnlock()
		if heldSlotSet && slotIndex == hotbarSlotStart+heldSlot {
			a.moveMirror.EmitEquipment(a.GetEntityID(), models.MainHand, item.ItemID, item.Count)
		}
	}

	return nil
}

// onWindowItems handles container/window inventory updates (ClientboundContainerSetContent).
//
// The player-facing inventory state is owned by the screen manager inside
// mc-bot-go. What this handler adds is entity attribution: when the window
// belongs to an entity container we opened (donkey/mule/llama chest, chest
// boat, chest minecart), the contents are cached against that entity so they
// remain readable after the window closes.
func (a *agent) onWindowItems(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	windowID, _, slots, _, err := a.versionHandler.Play().Containers().ParseContainerSetContent(p)
	if err != nil {
		return err
	}

	log.Printf("[Agent %s] ClientboundContainerSetContent: windowID=%d slots=%d", a.cfg.Name, windowID, len(slots))

	// Negative window IDs address the player's own inventory views, which are
	// never an entity container.
	if windowID > 0 {
		a.storeEntityWindowContents(byte(windowID), slots)
	}

	return nil
}

// onLogin captures the bot's entity ID and resumes physics after respawn or reconnect.
func (a *agent) onLogin(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	entityID, err := a.versionHandler.Play().ParseLogin(p)
	if err != nil {
		return err
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
	// Resume position updates after respawn or reconnect.
	// If the agent was dead (isDead=true), this clears it so physics can send positions again.
	// Closes the race window between server respawn and NotifyRespawned() being called.
	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()
	if notifier, ok := moveExec.(interface{ NotifyRespawned() }); ok {
		notifier.NotifyRespawned()
	}
	return nil
}

// sendPlayerLoadedOnce sends ServerboundPlayerLoaded exactly once per connection
// lifecycle. Required for 1.21.4+ where the server keeps a per-player
// remainingLoadTicks=60 counter and silently drops interact/vehicle/use-entity
// packets while !PlayerEntity.isLoaded(). Vanilla clients send this packet once
// after finishing the world load. For 1.21.1–1.21.3 the version-specific
// implementation is a no-op (the packet doesn't exist server-side).
func (a *agent) sendPlayerLoadedOnce() {
	if a.versionHandler == nil || a.client == nil {
		log.Printf("[Agent %s][WARN] Version handler or client not available; cannot send PlayerLoaded", a.cfg.Name)
		return
	}

	lifecycle := a.versionHandler.Play().Lifecycle()
	if lifecycle == nil {
		log.Printf("[Agent %s][WARN] Lifecycle handler not available; cannot send PlayerLoaded", a.cfg.Name)
		return
	}

	a.playerLoadedSent.Do(func() {
		conn := a.client.Conn()
		if conn == nil {
			log.Printf("[Agent %s][WARN] Client connection not available; skipping SendPlayerLoaded (likely in tests)", a.cfg.Name)
			return
		}
		if err := lifecycle.SendPlayerLoaded(conn); err != nil {
			log.Printf("[Agent %s][ERROR] SendPlayerLoaded failed: %v", a.cfg.Name, err)
			return
		}
		log.Printf("[Agent %s] Sent ServerboundPlayerLoaded (post first-teleport-ack)", a.cfg.Name)
	})
}

// onClientboundPosition updates absolute position and applies rotation flags.
// While mounted on a vehicle, the vanilla client ignores the position part of
// PlayerPositionLookS2CPacket (ServerPlayerEntity.startRiding always calls
// requestTeleport, which the client must acknowledge to unblock VehicleMove
// processing). We replicate that: skip the position/physics update but still
// send TeleportConfirm so the server clears its pending-teleport gate.
func (a *agent) onClientboundPosition(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	TeleportID, X, Y, Z, Yaw, Pitch, Flags, err := a.versionHandler.Play().Movement().ParsePlayerPosition(p)
	if err != nil {
		return err
	}

	if a.moveMirror != nil {
		// First play-state packet; safe to allow replay mirror emissions now.
		a.moveMirror.NotifyLoginSeen()
	}

	// Snapshot moveExec and teleport before acquiring posMu to avoid nested lock dependencies
	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()

	a.fallbackHandlersMu.RLock()
	teleport := a.teleport
	a.fallbackHandlersMu.RUnlock()

	// While mounted, ignore the position payload but still acknowledge the teleport.
	// The server sends PlayerPositionLook immediately after startRiding() and waits
	// for TeleportConfirm before accepting VehicleMove packets. Applying the position
	// here would overwrite the vehicle-seeded physicsState with the player's passenger
	// offset, causing every subsequent VehicleMove to carry the wrong Y coordinate.
	//
	// NOTE on packet ordering: ServerPlayerEntity.startRiding() sends PlayerPositionLook
	// *before* EntityPassengersSetS2CPacket, so IsMounted() is still false when this
	// handler fires for the initial mount teleport. The non-mounted path below also sends
	// TeleportConfirm; this branch covers subsequent corrections while already riding.
	if a.IsMounted() {
		log.Printf("[onClientboundPosition] Mounted: skipping position update (%.2f,%.2f,%.2f), confirming teleportID=%d",
			X, Y, Z, TeleportID)
		t := a.player
		if t == nil {
			t = teleport
		}
		if t != nil {
			if err := t.AcceptTeleportation(pk.VarInt(TeleportID)); err != nil {
				log.Printf("[onClientboundPosition] Warning: failed to send teleport confirmation (mounted): %v", err)
			}
		} else {
			log.Printf("[onClientboundPosition] ERROR: no TeleportAccepter available to confirm teleport (mounted) ID=%d", TeleportID)
		}
		a.sendPlayerLoadedOnce()
		if notifier, ok := moveExec.(interface{ NotifyRespawned() }); ok {
			notifier.NotifyRespawned()
		}
		return nil
	}

	// Apply position and rotation with respect to relative flags.
	// Flag bits: 0x01=X relative, 0x02=Y relative, 0x04=Z relative, 0x08=Yaw relative, 0x10=Pitch relative
	a.posMu.Lock()
	if Flags&0x01 != 0 { // X relative
		a.posX += X
	} else {
		a.posX = X
	}
	if Flags&0x02 != 0 { // Y relative
		a.posY += Y
	} else {
		a.posY = Y
	}
	if Flags&0x04 != 0 { // Z relative
		a.posZ += Z
	} else {
		a.posZ = Z
	}
	if Flags&0x08 != 0 { // Yaw relative
		a.posYaw += Yaw
	} else {
		a.posYaw = Yaw
	}
	if Flags&0x10 != 0 { // Pitch relative
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
	if syncer, ok := moveExec.(interface {
		SyncWithServer(x, y, z float64, yaw, pitch float64, onGround bool)
	}); ok {
		syncer.SyncWithServer(a.posX, a.posY, a.posZ, a.posYaw, a.posPitch, true)
	}

	// Send teleport confirmation before resuming physics to prevent out-of-order packets.
	// Prefer auto-created player, fall back to injected teleport.
	// AcceptTeleportation handles its own connection internally (basic.Player uses p.c.Conn).
	t := a.player
	if t == nil {
		t = teleport
	}
	if t != nil {
		if err := t.AcceptTeleportation(pk.VarInt(TeleportID)); err != nil {
			log.Printf("[onClientboundPosition] Warning: failed to send teleport confirmation: %v", err)
		}
	} else {
		log.Printf("[onClientboundPosition] ERROR: no TeleportAccepter available to confirm teleport ID=%d", TeleportID)
	}
	a.sendPlayerLoadedOnce()
	if notifier, ok := moveExec.(interface{ NotifyRespawned() }); ok {
		notifier.NotifyRespawned()
	}

	a.posMu.Unlock()
	return nil
}

// onClientboundMoveVehicle handles the server-to-client vehicle position correction.
// When the server rejects a VehicleMove (e.g., because the position was wrong), it
// sends this packet with the authoritative vehicle position. The agent must update
// its physics state to the corrected position and reset riding velocity so the next
// VehicleMove starts from the server-approved location.
func (a *agent) onClientboundMoveVehicle(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	vx, vy, vz, vyaw, vpitch, err := a.versionHandler.Play().Movement().ParseClientboundMoveVehicle(p)
	if err != nil {
		log.Printf("[onClientboundMoveVehicle] ERROR parsing packet: %v", err)
		return err
	}

	log.Printf("[onClientboundMoveVehicle] Server correction: pos=(%.2f,%.2f,%.2f) yaw=%.2f pitch=%.2f",
		vx, vy, vz, vyaw, vpitch)

	if !a.IsMounted() {
		// Received while not mounted; could be a stale packet from a just-dismounted
		// vehicle. Update the entity tracker position if we have it.
		log.Printf("[onClientboundMoveVehicle] Not mounted, ignoring position correction")
		return nil
	}

	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()

	if syncer, ok := moveExec.(interface {
		SyncRidingPosition(x, y, z float64, yaw, pitch float64)
	}); ok {
		syncer.SyncRidingPosition(vx, vy, vz, vyaw, vpitch)
	}
	return nil
}

// onUpdateViewDistance handles server-sent view distance updates.
func (a *agent) onUpdateViewDistance(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	viewDistance, err := a.versionHandler.Play().ParseViewDistance(p)
	if err != nil {
		return err
	}

	// Currently just logging for awareness
	// Could be used to update client state if needed
	log.Printf("view distance: %d", viewDistance)
	return nil
}

// onSimulationDistance handles server-sent simulation distance updates.
func (a *agent) onSimulationDistance(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	simulationDistance, err := a.versionHandler.Play().ParseSimulationDistance(p)
	if err != nil {
		return err
	}

	// Currently just logging for awareness
	// Could be used to update client state if needed
	log.Printf("simulation distance: %d", simulationDistance)
	return nil
}

// onUpdateRecipes handles the ClientboundUpdateRecipes packet (also known as DeclareRecipes).
func (a *agent) onUpdateRecipes(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil
	}

	payload, err := a.versionHandler.Play().ParseUpdateRecipes(p)
	if err != nil {
		log.Printf("[Agent %s] Failed to parse UpdateRecipes: %v", a.cfg.Name, err)
		return nil // Don't fail on parse errors
	}

	if payload == nil {
		return nil // Not supported for this version
	}

	a.recipesMu.Lock()
	a.lastUpdateRecipes = payload
	a.recipesMu.Unlock()
	return nil
}

// onSetPassengers handles the ClientboundSetPassengers packet to track mount state.
func (a *agent) onSetPassengers(p pk.Packet) error {
	if a.versionHandler == nil {
		return fmt.Errorf("missing version handler")
	}

	vehicleID, passengerIDs, err := a.versionHandler.Play().Entities().ParseSetPassengers(p)
	if err != nil {
		log.Printf("[onSetPassengers] ERROR parsing packet: %v", err)
		return err
	}

	// Record our index in the passenger list, not merely our membership. The
	// list is ordered, and index 0 is the controlling passenger: vehicles that
	// seat several players (boat, camel, happy ghast) ignore input from anyone
	// else, so a later index means we are a passive rider and must not predict
	// the vehicle's movement.
	agentID := a.GetEntityID()
	passengerIndex := -1
	for idx, pid := range passengerIDs {
		if pid == agentID {
			passengerIndex = idx
			break
		}
	}
	isPassenger := passengerIndex >= 0

	currentMount := a.getMountedEntityID()
	log.Printf("[onSetPassengers] RECEIVED: vehicleID=%d isPassenger=%v passengerIndex=%d currentMount=%d passengerCount=%d", vehicleID, isPassenger, passengerIndex, currentMount, len(passengerIDs))

	// A seat change without a mount/dismount happens when another passenger
	// boards or leaves: e.g. the driver of a happy ghast dismounts and the
	// remaining riders rotate up, promoting one of them to controlling
	// passenger. Keep our recorded index current so the dispatch follows.
	if isPassenger && currentMount == vehicleID {
		a.setMountedEntity(vehicleID, passengerIndex)
	}

	if isPassenger && currentMount != vehicleID {
		// Agent just mounted a vehicle
		log.Printf("[onSetPassengers] Agent mounted entity %d at passenger index %d (vehicle with %d passengers)", vehicleID, passengerIndex, len(passengerIDs))
		a.setMountedEntity(vehicleID, passengerIndex)
		if a.moveExec != nil {
			if err := a.moveExec.SetMounted(vehicleID); err != nil {
				log.Printf("[onSetPassengers] Error setting movement executor mounted state: %v", err)
			}
			// Seed the executor with the mount's currently-tracked pose so
			// vehicle-specific state (e.g. CamelState) reflects whether the
			// camel is already sitting at mount time instead of defaulting
			// to standing.
			a.entitiesMu.RLock()
			ent, ok := a.entities[vehicleID]
			var seedPose int32
			var seedName string
			var hasSeed bool
			if ok && ent.HasPose {
				seedPose = ent.Pose
				seedName = ent.PoseName
				hasSeed = true
			}
			a.entitiesMu.RUnlock()
			if hasSeed {
				if notifier, ok := a.moveExec.(interface {
					NotifyVehiclePose(entityID int32, poseName string, ordinal int32)
				}); ok {
					notifier.NotifyVehiclePose(vehicleID, seedName, seedPose)
				}
			}
		}
	} else if !isPassenger && currentMount == vehicleID {
		// Agent just dismounted from the vehicle
		log.Printf("[onSetPassengers] Agent dismounted from entity %d", vehicleID)
		a.setMountedEntity(-1, -1)
		if a.moveExec != nil {
			if err := a.moveExec.SetDismounted(); err != nil {
				log.Printf("[onSetPassengers] Error setting movement executor dismounted state: %v", err)
			}
		}
	} else {
		log.Printf("[onSetPassengers] No mount state change (isPassenger=%v, currentMount=%d, vehicleID=%d)", isPassenger, currentMount, vehicleID)
	}

	return nil
}

// onUpdateTime handles the ClientboundUpdateTime packet.
// This packet contains the world age and time of day from the server.
// We extract the world age and store it for time-dependent mechanics like camel dash cooldowns.
func (a *agent) onUpdateTime(p pk.Packet) error {
	if a.versionHandler == nil {
		return nil // No version handler, skip
	}

	// Parse the UpdateTime packet using the version handler
	worldAge, timeOfDay, err := a.versionHandler.Play().World().ParseUpdateTime(p)
	if err != nil {
		log.Printf("[Agent %s] Failed to parse UpdateTime packet: %v", a.cfg.Name, err)
		return nil // Non-fatal: just log and continue
	}

	// Update world manager with server's world age and time of day
	if a.mcAgentWorld != nil {
		a.mcAgentWorld.SetWorldTime(worldAge, timeOfDay)
		log.Printf("[Agent %s] Updated world time: age=%d ticks (%.1f days), timeOfDay=%d",
			a.cfg.Name, worldAge, float64(worldAge)/24000.0, timeOfDay)
	}

	return nil
}

// worldPacketHandlers returns packet handlers for world packets when using mc-agent world.
// These handlers use the version handler to parse packets and feed data to the mc-agent world manager.
func (a *agent) worldPacketHandlers() []bot.PacketHandler {
	// Only register handlers when using mc-agent world with version handler
	if a.versionHandler == nil || a.mcAgentWorld == nil {
		log.Printf("[Agent %s] worldPacketHandlers: NOT registering (versionHandler=%v, mcAgentWorld=%v)",
			a.cfg.Name, a.versionHandler != nil, a.mcAgentWorld != nil)
		return nil
	}

	log.Printf("[Agent %s] worldPacketHandlers: Registering world packet handlers", a.cfg.Name)
	worldHandler := a.versionHandler.Play().World()

	chunkPacketID := a.packetMgr.GetClientboundPacketID("ClientboundMapChunk")
	blockUpdateID := a.packetMgr.GetClientboundPacketID("ClientboundBlockUpdate")
	log.Printf("[Agent %s] Registering chunk handler for packet ID %d", a.cfg.Name, chunkPacketID)
	log.Printf("[Agent %s] Registering block update handler for packet ID %d", a.cfg.Name, blockUpdateID)

	handlers := []bot.PacketHandler{
		{
			ID:       chunkPacketID,
			Priority: 50, // Higher priority than other handlers to process chunk data first
			F: func(p pk.Packet) error {
				log.Printf("[Agent %s] Received ClientboundLevelChunkWithLight packet", a.cfg.Name)
				chunkX, chunkZ, data, err := worldHandler.ParseChunkData(p)
				if err != nil {
					log.Printf("[Agent %s] Warning: failed to parse chunk data: %v", a.cfg.Name, err)
					return nil // Don't fail on parse errors
				}
				log.Printf("[Agent %s] Loaded chunk at (%d, %d), data size: %d", a.cfg.Name, chunkX, chunkZ, len(data))
				return a.mcAgentWorld.HandleChunkLoad(chunkX, chunkZ, data)
			},
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundForgetLevelChunk"),
			Priority: 50,
			F: func(p pk.Packet) error {
				chunkX, chunkZ, err := worldHandler.ParseUnloadChunk(p)
				if err != nil {
					log.Printf("[Agent %s] Warning: failed to parse unload chunk: %v", a.cfg.Name, err)
					return nil
				}
				return a.mcAgentWorld.HandleChunkUnload(chunkX, chunkZ)
			},
		},
		{
			ID:       blockUpdateID,
			Priority: 50,
			F: func(p pk.Packet) error {
				x, y, z, blockStateID, err := worldHandler.ParseBlockUpdate(p)
				if err != nil {
					log.Printf("[Agent %s] Warning: failed to parse blocks update: %v", a.cfg.Name, err)
					return nil // Ignore parse errors
				}
				log.Printf("[Agent %s] Block update at (%d, %d, %d) -> state %d", a.cfg.Name, x, y, z, blockStateID)
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
					log.Printf("[Agent %s] Warning: failed to parse section blocks update: %v", a.cfg.Name, err)
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
					log.Printf("[Agent %s] Error sending chunk batch acknowledgement: %v", a.cfg.Name, err)
					return nil // Don't fail on ack errors
				}

				// Log progress periodically
				if int(a.chunkBatchCount)%10 == 0 || int(a.chunkBatchCount) <= 3 {
					log.Printf("[Agent %s] Acknowledged %d chunk batches", a.cfg.Name, int(a.chunkBatchCount))
				}
				return nil
			},
		},
	}

	log.Printf("[Agent %s] worldPacketHandlers: Returning %d world packet handlers", a.cfg.Name, len(handlers))
	return handlers
}
