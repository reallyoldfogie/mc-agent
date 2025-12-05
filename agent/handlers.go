package agent

import (
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// coreHandlers returns the minimal set of packet handlers needed for tracking.
func (a *Agent) coreHandlers() []PacketHandler {
	if a.packetMgr == nil {
		return nil
	}
	handlers := []PacketHandler{
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundSound")),
			Priority: 0,
			F:        a.onSoundPacket,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundAddEntity")),
			Priority: 0,
			F:        a.onAddEntity,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundMoveEntityPosRot")),
			Priority: 0,
			F:        a.onMoveEntityPosRot,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundMoveEntityPos")),
			Priority: 0,
			F:        a.onMoveEntityPos,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundTeleportEntity")),
			Priority: 0,
			F:        a.onTeleportEntity,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundRemoveEntities")),
			Priority: 0,
			F:        a.onRemoveEntities,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundLogin")),
			Priority: 100,
			F:        a.onLogin,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundPosition")),
			Priority: 63,
			F:        a.onClientboundPosition,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundPlayerInfo")),
			Priority: 90,
			F: func(p pk.Packet) error {
				if a.moveMirror != nil {
					a.moveMirror.HandlePlayerInfo(p)
				}
				return nil
			},
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundSetChunkCacheRadius")),
			Priority: 0,
			F:        a.onUpdateViewDistance,
		},
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundSetSimulationDistance")),
			Priority: 0,
			F:        a.onSimulationDistance,
		},
	}
	// Include config-phase registry capture
	handlers = append(handlers, a.registryHandlers()...)
	return handlers
}

// onAddEntity tracks new or respawned entities.
func (a *Agent) onAddEntity(p pk.Packet) error {
	var (
		EntityID   pk.VarInt
		EntityUUID pk.UUID
		EntityType pk.VarInt
		X, Y, Z    pk.Double
		Pitch      pk.Angle
		Yaw        pk.Angle
		HeadYaw    pk.Angle
		Data       pk.VarInt
		VelX       pk.Short
		VelY       pk.Short
		VelZ       pk.Short
	)
	if err := p.Scan(&EntityID, &EntityUUID, &EntityType, &X, &Y, &Z, &Pitch, &Yaw, &HeadYaw, &Data, &VelX, &VelY, &VelZ); err != nil {
		return nil // ignore malformed packets here
	}

	var uuid [16]byte
	copy(uuid[:], EntityUUID[:])

	a.entitiesMu.Lock()
	if e, ok := a.entities[int32(EntityID)]; ok {
		e.EntityType = int32(EntityType)
		e.UUID = uuid
		e.X, e.Y, e.Z = float64(X), float64(Y), float64(Z)
		e.Yaw, e.Pitch = int8(Yaw), int8(Pitch)
		e.Removed = false
	} else {
		a.entities[int32(EntityID)] = &trackedEntity{
			EntityID:   int32(EntityID),
			EntityType: int32(EntityType),
			UUID:       uuid,
			X:          float64(X),
			Y:          float64(Y),
			Z:          float64(Z),
			Yaw:        int8(Yaw),
			Pitch:      int8(Pitch),
			Removed:    false,
		}
	}
	a.entitiesMu.Unlock()
	if a.moveMirror != nil && int32(EntityID) == a.GetEntityID() {
		a.moveMirror.SetEntityType(int32(EntityType))
	}
	return nil
}

// onMoveEntityPosRot updates incremental position and rotation.
func (a *Agent) onMoveEntityPosRot(p pk.Packet) error {
	var (
		EntityID   pk.VarInt
		DX, DY, DZ pk.Short
		Yaw, Pitch pk.Angle
		OnGround   pk.Boolean
	)
	if err := p.Scan(&EntityID, &DX, &DY, &DZ, &Yaw, &Pitch, &OnGround); err != nil {
		return nil
	}
	a.entitiesMu.Lock()
	if e, ok := a.entities[int32(EntityID)]; ok {
		e.X += float64(DX) / (128 * 32)
		e.Y += float64(DY) / (128 * 32)
		e.Z += float64(DZ) / (128 * 32)
		e.Yaw, e.Pitch = int8(Yaw), int8(Pitch)
		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onMoveEntityPos updates incremental position without rotation.
func (a *Agent) onMoveEntityPos(p pk.Packet) error {
	var (
		EntityID   pk.VarInt
		DX, DY, DZ pk.Short
		OnGround   pk.Boolean
	)
	if err := p.Scan(&EntityID, &DX, &DY, &DZ, &OnGround); err != nil {
		return nil
	}
	a.entitiesMu.Lock()
	if e, ok := a.entities[int32(EntityID)]; ok {
		e.X += float64(DX) / (128 * 32)
		e.Y += float64(DY) / (128 * 32)
		e.Z += float64(DZ) / (128 * 32)
		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onTeleportEntity handles absolute teleports.
func (a *Agent) onTeleportEntity(p pk.Packet) error {
	var (
		EntityID   pk.VarInt
		X, Y, Z    pk.Double
		Yaw, Pitch pk.Angle
		OnGround   pk.Boolean
	)
	if err := p.Scan(&EntityID, &X, &Y, &Z, &Yaw, &Pitch, &OnGround); err != nil {
		return nil
	}
	a.entitiesMu.Lock()
	if e, ok := a.entities[int32(EntityID)]; ok {
		e.X, e.Y, e.Z = float64(X), float64(Y), float64(Z)
		e.Yaw, e.Pitch = int8(Yaw), int8(Pitch)
		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onRemoveEntities marks entities as softly removed, allowing grace period before purge.
func (a *Agent) onRemoveEntities(p pk.Packet) error {
	var count pk.VarInt
	if err := p.Scan(&count); err != nil {
		return nil
	}
	ids := make([]pk.VarInt, int(count))
	for i := 0; i < int(count); i++ {
		if err := p.Scan(&ids[i]); err != nil {
			return nil
		}
	}
	now := time.Now()
	a.entitiesMu.Lock()
	for _, id := range ids {
		if e, ok := a.entities[int32(id)]; ok {
			e.Removed = true
			e.RemovedAt = now
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onLogin captures the bot's entity ID.
func (a *Agent) onLogin(p pk.Packet) error {
	var entityID pk.Int
	if err := p.Scan(&entityID); err != nil {
		return nil
	}
	a.setEntityID(int32(entityID))
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
		a.moveMirror.SetEntityMeta(int32(entityID), a.cfg.Auth.Name, id)
		// Entity type is set via registry callback during configuration phase
	}
	return nil
}

// onClientboundPosition updates absolute position and applies rotation flags.
func (a *Agent) onClientboundPosition(p pk.Packet) error {
	var (
		TeleportID pk.VarInt
		X, Y, Z    pk.Double
		DX, DY, DZ pk.Double
		Yaw, Pitch pk.Float
		Flags      pk.VarInt
	)
	if err := p.Scan(&TeleportID, &X, &Y, &Z, &DX, &DY, &DZ, &Yaw, &Pitch, &Flags); err != nil {
		return nil
	}

	// Absolute base position
	a.posMu.Lock()
	a.posX, a.posY, a.posZ = float64(X), float64(Y), float64(Z)
	// Rotation may be relative per flags
	if Flags&0x08 != 0 {
		a.posYaw += float32(Yaw)
	} else {
		a.posYaw = float32(Yaw)
	}
	if Flags&0x10 != 0 {
		a.posPitch += float32(Pitch)
	} else {
		a.posPitch = float32(Pitch)
	}
	a.posInitialized = true
	a.posMu.Unlock()

	// Accept teleport when possible
	if a.teleport != nil {
		_ = a.teleport.AcceptTeleportation(TeleportID)
	}
	_ = DX
	_ = DY
	_ = DZ // deltas currently unused
	return nil
}

// onUpdateViewDistance handles server-sent view distance updates.
func (a *Agent) onUpdateViewDistance(p pk.Packet) error {
	var viewDistance pk.VarInt
	if err := p.Scan(&viewDistance); err != nil {
		return nil
	}
	// Currently just logging for awareness
	// Could be used to update client state if needed
	return nil
}

// onSimulationDistance handles server-sent simulation distance updates.
func (a *Agent) onSimulationDistance(p pk.Packet) error {
	var simulationDistance pk.VarInt
	if err := p.Scan(&simulationDistance); err != nil {
		return nil
	}
	// Currently just logging for awareness
	// Could be used to update client state if needed
	return nil
}
