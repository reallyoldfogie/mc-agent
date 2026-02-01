package agent

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"

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
			Priority: 0,
			F:        a.onSoundPacket,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundAddEntity"),
			Priority: 0,
			F:        a.onAddEntity,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundMoveEntityPosRot"),
			Priority: 0,
			F:        a.onMoveEntityPosRot,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundMoveEntityPos"),
			Priority: 0,
			F:        a.onMoveEntityPos,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundTeleportEntity"),
			Priority: 0,
			F:        a.onTeleportEntity,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundRemoveEntities"),
			Priority: 0,
			F:        a.onRemoveEntities,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundLogin"),
			Priority: 100,
			F:        a.onLogin,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundPosition"),
			Priority: 63,
			F:        a.onClientboundPosition,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundPlayerInfo"),
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
			Priority: 0,
			F:        a.onUpdateViewDistance,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundSetSimulationDistance"),
			Priority: 0,
			F:        a.onSimulationDistance,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundDeclareRecipes"),
			Priority: 0,
			F:        a.ParseUpdateRecipesPacket,
		},
		{
			ID:       a.packetMgr.GetClientboundPacketID("ClientboundDisconnect"),
			Priority: 0,
			F:        a.onDisconnect2,
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
	pkt, err := a.packetMgr.GetClientboundPacketByID(a.packetMgr.GetClientboundPacketID("ClientboundDisconnect"))
	if err != nil {
		return err
	}
	err = pkt.Scan(p)
	if err != nil {
		return err
	}

	name := ""
	if a.client != nil {
		name = a.client.Name()
	}

	fields := pkt.GetFields()
	if reasonField, ok := fields["Reason"]; ok {
		reason, ok := reasonField.(pk.String)
		if ok {
			log.Printf("[Agent %s] Disconnected from server: %s", name, string(reason))
		} else {
			log.Printf("[Agent %s] Disconnected from server.", name)
		}
	} else {
		log.Printf("[Agent %s] Disconnected from server.", name)
	}
	return nil
}

// onAddEntity tracks new or respawned entities.
func (a *agent) onAddEntity(p pk.Packet) error {
	var entityID, entityType int32
	var uuid [16]byte
	var x, y, z float64
	var yaw, pitch int8

	// Use version handler if available, otherwise fall back to manual parsing
	if a.versionHandler != nil {
		var err error
		entityID, entityType, uuid, x, y, z, yaw, pitch, err = a.versionHandler.Play().Entities().ParseAddEntity(p)
		if err != nil {
			return nil // ignore malformed packets
		}
	} else {
		var (
			EntityID   pk.VarInt
			EntityUUID pk.UUID
			EntityType pk.VarInt
			X, Y, Z    pk.Double
			Velocity   LpVec3 // Velocity moved BEFORE pitch/yaw in 1.21.5+
			Pitch      pk.Angle
			Yaw        pk.Angle
			HeadYaw    pk.Angle
			Data       pk.VarInt
		)
		// Fixed field order for 1.21.5+: EntityID, UUID, Type, X, Y, Z, Velocity(LpVec3), Pitch, Yaw, HeadYaw, Data
		if err := p.Scan(&EntityID, &EntityUUID, &EntityType, &X, &Y, &Z, &Velocity, &Pitch, &Yaw, &HeadYaw, &Data); err != nil {
			return nil // ignore malformed packets here
		}
		entityID = int32(EntityID)
		entityType = int32(EntityType)
		copy(uuid[:], EntityUUID[:])
		x, y, z = float64(X), float64(Y), float64(Z)
		yaw, pitch = int8(Yaw), int8(Pitch)
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		e.EntityType = entityType
		e.UUID = uuid
		e.X, e.Y, e.Z = x, y, z
		e.Yaw, e.Pitch = yaw, pitch
		e.Removed = false
	} else {
		a.entities[entityID] = &trackedEntity{
			EntityID:   entityID,
			EntityType: entityType,
			UUID:       uuid,
			X:          x,
			Y:          y,
			Z:          z,
			Yaw:        yaw,
			Pitch:      pitch,
			Removed:    false,
		}
	}
	a.entitiesMu.Unlock()
	if a.moveMirror != nil && entityID == a.GetEntityID() {
		a.moveMirror.SetEntityType(entityType)
	}
	return nil
}

// onMoveEntityPosRot updates incremental position and rotation.
func (a *agent) onMoveEntityPosRot(p pk.Packet) error {
	var entityID int32
	var dx, dy, dz int16
	var yaw, pitch int8

	// Use version handler if available, otherwise fall back to manual parsing
	if a.versionHandler != nil {
		var onGround bool
		var err error
		entityID, dx, dy, dz, yaw, pitch, onGround, err = a.versionHandler.Play().Entities().ParseMoveEntityPosRot(p)
		if err != nil {
			return nil // ignore malformed packets
		}
		_ = onGround
	} else {
		var (
			EntityID   pk.VarInt
			DX, DY, DZ pk.Short
			Yaw, Pitch pk.Angle
			OnGround   pk.Boolean
		)
		if err := p.Scan(&EntityID, &DX, &DY, &DZ, &Yaw, &Pitch, &OnGround); err != nil {
			return nil
		}
		entityID = int32(EntityID)
		dx, dy, dz = int16(DX), int16(DY), int16(DZ)
		yaw, pitch = int8(Yaw), int8(Pitch)
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		e.X += float64(dx) / (128 * 32)
		e.Y += float64(dy) / (128 * 32)
		e.Z += float64(dz) / (128 * 32)
		e.Yaw, e.Pitch = yaw, pitch
		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onMoveEntityPos updates incremental position without rotation.
func (a *agent) onMoveEntityPos(p pk.Packet) error {
	var entityID int32
	var dx, dy, dz int16

	// Use version handler if available, otherwise fall back to manual parsing
	if a.versionHandler != nil {
		var onGround bool
		var err error
		entityID, dx, dy, dz, onGround, err = a.versionHandler.Play().Entities().ParseMoveEntityPos(p)
		if err != nil {
			return nil // ignore malformed packets
		}
		_ = onGround
	} else {
		var (
			EntityID   pk.VarInt
			DX, DY, DZ pk.Short
			OnGround   pk.Boolean
		)
		if err := p.Scan(&EntityID, &DX, &DY, &DZ, &OnGround); err != nil {
			return nil
		}
		entityID = int32(EntityID)
		dx, dy, dz = int16(DX), int16(DY), int16(DZ)
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		e.X += float64(dx) / (128 * 32)
		e.Y += float64(dy) / (128 * 32)
		e.Z += float64(dz) / (128 * 32)
		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onTeleportEntity handles absolute teleports.
func (a *agent) onTeleportEntity(p pk.Packet) error {
	var entityID int32
	var x, y, z float64
	var yaw, pitch int8

	// Use version handler if available, otherwise fall back to manual parsing
	if a.versionHandler != nil {
		var onGround bool
		var err error
		entityID, x, y, z, yaw, pitch, onGround, err = a.versionHandler.Play().Entities().ParseTeleportEntity(p)
		if err != nil {
			return nil // ignore malformed packets
		}
		_ = onGround
	} else {
		var (
			EntityID   pk.VarInt
			X, Y, Z    pk.Double
			Yaw, Pitch pk.Angle
			OnGround   pk.Boolean
		)
		if err := p.Scan(&EntityID, &X, &Y, &Z, &Yaw, &Pitch, &OnGround); err != nil {
			return nil
		}
		entityID = int32(EntityID)
		x, y, z = float64(X), float64(Y), float64(Z)
		yaw, pitch = int8(Yaw), int8(Pitch)
	}

	a.entitiesMu.Lock()
	if e, ok := a.entities[entityID]; ok {
		e.X, e.Y, e.Z = x, y, z
		e.Yaw, e.Pitch = yaw, pitch
		if e.Removed {
			e.Removed = false
		}
	}
	a.entitiesMu.Unlock()
	return nil
}

// onRemoveEntities marks entities as softly removed, allowing grace period before purge.
func (a *agent) onRemoveEntities(p pk.Packet) error {
	var entityIDs []int32

	// Use version handler if available, otherwise fall back to manual parsing
	if a.versionHandler != nil {
		var err error
		entityIDs, err = a.versionHandler.Play().Entities().ParseRemoveEntities(p)
		if err != nil {
			return nil // ignore malformed packets
		}
	} else {
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
		entityIDs = make([]int32, len(ids))
		for i, id := range ids {
			entityIDs[i] = int32(id)
		}
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
	return nil
}

// onLogin captures the bot's entity ID.
func (a *agent) onLogin(p pk.Packet) error {
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
func (a *agent) onClientboundPosition(p pk.Packet) error {
	var (
		TeleportID int32
		X, Y, Z    float64
		Yaw, Pitch float32
		Flags      int32
	)

	// Use version-specific parsing if available (packet format varies between versions)
	if a.versionHandler != nil {
		var err error
		TeleportID, X, Y, Z, Yaw, Pitch, Flags, err = a.versionHandler.Play().Movement().ParsePlayerPosition(p)
		if err != nil {
			return nil // Silently ignore parse errors
		}
	} else {
		// Fallback to hardcoded 1.21.5+ format parsing
		var (
			teleportID pk.VarInt
			x, y, z    pk.Double
			dx, dy, dz pk.Double
			yaw, pitch pk.Float
			flags      pk.VarInt
		)
		if err := p.Scan(&teleportID, &x, &y, &z, &dx, &dy, &dz, &yaw, &pitch, &flags); err != nil {
			return nil
		}
		TeleportID = int32(teleportID)
		X, Y, Z = float64(x), float64(y), float64(z)
		Yaw, Pitch = float32(yaw), float32(pitch)
		Flags = int32(flags)
		_ = dx
		_ = dy
		_ = dz // deltas currently unused
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
	var viewDistance pk.VarInt
	if err := p.Scan(&viewDistance); err != nil {
		return nil
	}
	// Currently just logging for awareness
	// Could be used to update client state if needed
	log.Printf("view distance: %d", int32(viewDistance))

	return nil
}

// onSimulationDistance handles server-sent simulation distance updates.
func (a *agent) onSimulationDistance(p pk.Packet) error {
	var simulationDistance pk.VarInt
	if err := p.Scan(&simulationDistance); err != nil {
		return nil
	}
	// Currently just logging for awareness
	// Could be used to update client state if needed
	log.Printf("simulation distance: %d", int32(simulationDistance))

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
