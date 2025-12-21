package agent

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// handlers returns set of packet handlers needed.
func (a *agent) handlers() []PacketHandler {
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
		{
			ID:       int32(a.packetMgr.GetClientboundPacketID("ClientboundDeclareRecipes")),
			Priority: 0,
			F:        a.ParseUpdateRecipesPacket,
		},
	}
	// Include config-phase registry capture
	handlers = append(handlers, a.registryHandlers()...)
	return handlers
}

// onAddEntity tracks new or respawned entities.
func (a *agent) onAddEntity(p pk.Packet) error {
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
func (a *agent) onMoveEntityPosRot(p pk.Packet) error {
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
func (a *agent) onMoveEntityPos(p pk.Packet) error {
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
func (a *agent) onTeleportEntity(p pk.Packet) error {
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
func (a *agent) onRemoveEntities(p pk.Packet) error {
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
		// Notify that LOGIN packet has been seen and recorded
		a.moveMirror.NotifyLoginSeen()
		// Entity type is set via registry callback during configuration phase
	}
	return nil
}

// onClientboundPosition updates absolute position and applies rotation flags.
func (a *agent) onClientboundPosition(p pk.Packet) error {
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
func (a *agent) onUpdateViewDistance(p pk.Packet) error {
	var viewDistance pk.VarInt
	if err := p.Scan(&viewDistance); err != nil {
		return nil
	}
	// Currently just logging for awareness
	// Could be used to update client state if needed
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
	return nil
}

// ParseUpdateRecipesPacket handles the ClientboundUpdateRecipes packet (also known as DeclaredRecipes).
// Protocol 1.21.5 (770) format: Property Sets + Stonecutter SingleInputSet entries
func (a *agent) ParseUpdateRecipesPacket(p pk.Packet) error {
	log.Printf("[recipes] === Starting Update Recipes Packet Parse ===")

	log.Printf("[recipes] Packet Length: %d bytes", len(p.Data))
	log.Printf("[recipes] Packet ID: %d", p.ID)
	log.Printf("[recipes] Raw Packet Data: [%x]", p.Data)

	// Create a streaming reader over packet data
	r := bytes.NewReader(p.Data)
	// Parse Property Sets
	var payload UpdateRecipesPayload
	var numPropertySets pk.VarInt
	if _, err := numPropertySets.ReadFrom(r); err != nil {
		log.Printf("[recipes] ERROR: failed to read property set count: %v", err)
		return nil
	}
	log.Printf("[recipes] Property Sets Count: %d", numPropertySets)

	for i := 0; i < int(numPropertySets); i++ {
		var propertySetID pk.Identifier
		if _, err := propertySetID.ReadFrom(r); err != nil {
			log.Printf("[recipes] ERROR: failed to read property set ID at index %d: %v", i, err)
			return nil
		}

		var numItems pk.VarInt
		if _, err := numItems.ReadFrom(r); err != nil {
			log.Printf("[recipes] ERROR: failed to read item count for property set %s: %v", propertySetID, err)
			return nil
		}

		log.Printf("[recipes] Property Set %d: ID=%s, Items Count=%d", i+1, propertySetID, numItems)

		items := make([]int32, int(numItems))
		for j := 0; j < int(numItems); j++ {
			var itemID pk.VarInt
			if _, err := itemID.ReadFrom(r); err != nil {
				log.Printf("[recipes] ERROR: failed to read item ID at index %d for property set %s: %v", j, propertySetID, err)
				return nil
			}
			items[j] = int32(itemID)
		}
		log.Printf("[recipes]   Items: %v", items)
		payload.PropertySets = append(payload.PropertySets, PropertySet{ID: fmt.Sprintf("%s", propertySetID), Items: items})
	}

// Parse Stonecutter entries (format: IDSet + SlotDisplay)
	var numStonecutterEntries pk.VarInt
	if _, err := numStonecutterEntries.ReadFrom(r); err != nil {
		log.Printf("[recipes] ERROR: failed to read stonecutter entry count: %v", err)
		return nil
	}
	log.Printf("[recipes] Stonecutter Entries Count: %d", numStonecutterEntries)

	for i := 0; i < int(numStonecutterEntries); i++ {
		log.Printf("[recipes] Stonecutter Entry %d:", i+1)

		// Parse IDSet (ingredients)
		idSet, err := scanIDSet(r)
		if err != nil {
			log.Printf("[recipes] ERROR: parsing IDSet for stonecutter entry %d: %v", i+1, err)
			return nil
		}
		log.Printf("[recipes]   Ingredients IDSet mode=%d, ids=%v", idSet.Mode, idSet.IDs)

		// Parse SlotDisplay (result)
		result, err := a.parseSlotDisplay(r, 1)
		if err != nil {
			log.Printf("[recipes] ERROR: parsing slot display for stonecutter entry %d: %v", i+1, err)
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

	log.Printf("[recipes] === Finished Update Recipes Packet Parse ===")
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
	log.Printf("[recipes]%sSlot Display Type: %d", indent, slotDisplayType)

	switch int(slotDisplayType) {
	case 0:
		log.Printf("[recipes]%s(empty)", indent)
		return SlotDisplay{Type: SlotDisplayTypeEmpty}, nil
	case 1:
		log.Printf("[recipes]%s(any_fuel)", indent)
		return SlotDisplay{Type: SlotDisplayTypeAnyFuel}, nil
	case 2:
		// minecraft:item -> item registry VarInt
		var itemID pk.VarInt
		if _, err := itemID.ReadFrom(r); err != nil {
			return SlotDisplay{}, err
		}
		log.Printf("[recipes]%sitem: id=%d", indent, itemID)
		return SlotDisplay{Type: SlotDisplayTypeItem, Item: &SlotDisplayItem{ItemID: int32(itemID)}}, nil
	case 3:
		// minecraft:item_stack -> Slot
		var s mcscreen.Slot
		if _, err := s.ReadFrom(r); err != nil {
			return SlotDisplay{}, err
		}
		if s.Count <= 0 {
			log.Printf("[recipes]%sitem_stack: empty", indent)
			return SlotDisplay{Type: SlotDisplayTypeItemStack, ItemStack: &SlotDisplayItemStack{ItemID: 0, Count: 0}}, nil
		}
		log.Printf("[recipes]%sitem_stack: id=%d count=%d", indent, s.ID, s.Count)
		return SlotDisplay{Type: SlotDisplayTypeItemStack, ItemStack: &SlotDisplayItemStack{ItemID: int32(s.ID), Count: int32(s.Count)}}, nil
	case 4:
		// minecraft:tag -> Identifier
		var tag pk.Identifier
		if _, err := tag.ReadFrom(r); err != nil {
			return SlotDisplay{}, err
		}
		str := fmt.Sprintf("%s", tag)
		log.Printf("[recipes]%stag: %s", indent, str)
		return SlotDisplay{Type: SlotDisplayTypeTag, Tag: &str}, nil
	case 5:
		// minecraft:smithing_trim -> Base SlotDisplay, Material SlotDisplay, Pattern VarInt
		log.Printf("[recipes]%ssmithing_trim:", indent)
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
		log.Printf("[recipes]%s  pattern_id=%d", indent, pattern)
		return SlotDisplay{Type: SlotDisplayTypeSmithingTrim, SmithingTrim: &SlotDisplaySmithingTrim{Base: base, Material: material, Pattern: int32(pattern)}}, nil
	case 6:
		// minecraft:with_remainder -> Ingredient SlotDisplay, Remainder SlotDisplay
		log.Printf("[recipes]%swith_remainder:", indent)
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
		log.Printf("[recipes]%scomposite: options=%d", indent, count)
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
		log.Printf("[recipes]%sunknown slot display type: %d", indent, slotDisplayType)
		return SlotDisplay{Type: SlotDisplayType(slotDisplayType)}, nil
	}
}
