// Package v1_21_9 provides version-specific packet handling for Minecraft 1.21.9.
package v1_21_9

import (
	"fmt"
	"io"
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.9/basetypes"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// componentTypeNameToID/componentTypeIDToName translate between 1.21.9's own
// real protocol numeric component-type IDs (basetypes.SlotComponentTypeMappings)
// and mc-bot-go's screen.SlotComponent.Type numeric ID space -- built once
// from this version's own mapping, not a shared pre/post-1.21.5 bucket (see
// docs/plans/SLOT_CODEC_IMPLEMENTATION_PLAN.md Phase 2).
var componentTypeNameToID, componentTypeIDToName = common.BuildSlotComponentTypeMaps(basetypes.SlotComponentTypeMappings)

// containerHandler implements common.ContainerHandler for 1.21.9.
type containerHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendContainerClick sends a container click packet.
// This is a complex packet with changed slots and cursor item tracking.
func (c *containerHandler) SendContainerClick(conn models.PacketWriter, windowID int8, stateID, slot int32, button int8, mode int32, changedSlots map[int16]models.InventorySlot, carriedItem models.InventorySlot) error {
	pkt := sb.NewWindowClick()
	pkt.WindowId = basetypes.ContainerID(windowID)
	pkt.StateId = pk.VarInt(stateID)
	pkt.Slot = pk.Short(slot)
	pkt.MouseButton = pk.Byte(button)
	pkt.Mode = pk.VarInt(mode)

	// Convert changed slots to protocol format
	// Note: The protocol uses a complex slot format with hashed components
	changedSlotsArr := make([]sb.WindowClickChangedSlotsArrayType, 0, len(changedSlots))
	for slotNum, slotData := range changedSlots {
		entry := sb.WindowClickChangedSlotsArrayType{
			Location: pk.Short(slotNum),
		}
		// Set the item data
		entry.Item.Has = pk.Boolean(slotData.Present)
		if slotData.Present {
			hashedSlot := &basetypes.HashedSlot{
				ItemId:    pk.VarInt(slotData.ItemID),
				ItemCount: pk.VarInt(slotData.Count),
			}
			entry.Item.Val = hashedSlot
		}
		changedSlotsArr = append(changedSlotsArr, entry)
	}
	pkt.ChangedSlots.Set(changedSlotsArr)

	// Set cursor item
	pkt.CursorItem.Has = pk.Boolean(carriedItem.Present)
	if carriedItem.Present {
		hashedSlot := &basetypes.HashedSlot{
			ItemId:    pk.VarInt(carriedItem.ItemID),
			ItemCount: pk.VarInt(carriedItem.Count),
		}
		pkt.CursorItem.Val = hashedSlot
	}

	return conn.WritePacket(pkt.Marshal())
}

// SendContainerClose sends a container close packet.
func (c *containerHandler) SendContainerClose(conn models.PacketWriter, windowID int8) error {
	pkt := sb.NewCloseWindow()
	pkt.WindowId = basetypes.ContainerID(windowID)

	return conn.WritePacket(pkt.Marshal())
}

// SendSetCreativeModeSlot sends a creative mode slot update.
func (c *containerHandler) SendSetCreativeModeSlot(conn models.PacketWriter, slot int16, item models.InventorySlot) error {
	pkt := sb.NewSetCreativeSlot()
	pkt.Slot = pk.Short(slot)

	if item.Present {
		pkt.Item.ItemCount = pk.VarInt(item.Count)
		pkt.Item.UnnamedType0001 = &basetypes.UntrustedSlotUnnamedType0001Default{
			ItemId:                pk.VarInt(item.ItemID),
			AddedComponentCount:   pk.VarInt(0),
			RemovedComponentCount: pk.VarInt(0),
		}
	} else {
		pkt.Item.ItemCount = pk.VarInt(0)
	}

	return conn.WritePacket(pkt.Marshal())
}

// SendPickItem
// Note: In 1.21.9, the PickItem packet takes a block position, not a slot.
// This implementation is a stub - the interface may need to be updated.
func (c *containerHandler) SendPickItem(conn models.PacketWriter, slot int32) error {
	// The 1.21.9 protocol changed PickItem to PickItemFromBlock which takes a position
	// instead of a slot. This interface needs to be redesigned for 1.21.9+.
	return common.ErrPacketSend{PacketName: "PickItem", Cause: nil}
}

// SendSetCarriedItem sends a held item change packet.
func (c *containerHandler) SendSetCarriedItem(conn models.PacketWriter, slot int16) error {
	pkt := sb.NewHeldItemSlot()
	pkt.SlotId = pk.Short(slot)

	return conn.WritePacket(pkt.Marshal())
}

// SendUseItemOn sends a use item on block packet (right-click on block).
// This is used for opening containers, placing blocks, and interacting with blocks.
// Note: 1.21.9+ includes the WorldBorderHit field (added in 1.21.2/protocol 768).
func (c *containerHandler) SendUseItemOn(conn models.PacketWriter, hand models.Hand, x, y, z int, face int32, cursorX, cursorY, cursorZ float32, insideBlock bool, sequence int32) error {
	pkt := sb.NewBlockPlace()
	pkt.Hand = pk.VarInt(hand)
	pkt.Location = basetypes.Position{X: int64(x), Y: int64(y), Z: int64(z)}
	pkt.Direction = pk.VarInt(face)
	pkt.CursorX = pk.Float(cursorX)
	pkt.CursorY = pk.Float(cursorY)
	pkt.CursorZ = pk.Float(cursorZ)
	pkt.InsideBlock = pk.Boolean(insideBlock)
	pkt.WorldBorderHit = pk.Boolean(false) // 1.21.9+ has this field
	pkt.Sequence = pk.VarInt(sequence)

	return conn.WritePacket(pkt.Marshal())
}

// ParseOpenScreen parses a container open packet.
func (c *containerHandler) ParseOpenScreen(p pk.Packet) (windowID int8, windowType int32, title string, err error) {
	pkt := cb.NewOpenWindow()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, "", common.ErrPacketParse{PacketName: "OpenWindow", Cause: err}
	}

	windowID = int8(pkt.WindowId)
	windowType = int32(pkt.InventoryType)
	// Extract title from NBT
	title = extractNBTString(pkt.WindowTitle)

	return windowID, windowType, title, nil
}

// ParseContainerSetContent parses a container content packet.
func (c *containerHandler) ParseContainerSetContent(p pk.Packet) (windowID int8, stateID int32, slots []models.InventorySlot, carriedItem models.InventorySlot, err error) {
	pkt := cb.NewWindowItems()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, nil, models.InventorySlot{}, common.ErrPacketParse{PacketName: "WindowItems", Cause: err}
	}

	windowID = int8(pkt.WindowId)
	stateID = int32(pkt.StateId)

	// Convert items array
	items := pkt.Items.Get()
	slots = make([]models.InventorySlot, len(items))
	for i, item := range items {
		slots[i] = convertSlotFromProtocol(item)
	}

	// Convert carried item
	carriedItem = convertSlotFromProtocol(pkt.CarriedItem)

	return windowID, stateID, slots, carriedItem, nil
}

// ParseContainerSetSlot parses a slot update packet.
func (c *containerHandler) ParseContainerSetSlot(p pk.Packet) (windowID int8, stateID int32, slot int16, item models.InventorySlot, err error) {
	pkt := cb.NewSetSlot()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, models.InventorySlot{}, common.ErrPacketParse{PacketName: "SetSlot", Cause: err}
	}

	windowID = int8(pkt.WindowId)
	stateID = int32(pkt.StateId)
	slot = int16(pkt.Slot)
	item = convertSlotFromProtocol(pkt.Item)

	return windowID, stateID, slot, item, nil
}

// slotItemID extracts the item ID from a protocol Slot's non-empty case.
// Returns 0 for an empty slot (ItemCount == 0, UnnamedType0001 is *models.Void).
func slotItemID(slot basetypes.Slot) int32 {
	if unnamed, ok := slot.UnnamedType0001.(*basetypes.SlotUnnamedType0001Default); ok {
		return int32(unnamed.ItemId)
	}
	return 0
}

// convertSlotFromProtocol converts a protocol Slot to models.InventorySlot.
func convertSlotFromProtocol(slot basetypes.Slot) models.InventorySlot {
	if slot.ItemCount <= 0 {
		return models.InventorySlot{Present: false}
	}

	// The slot structure in 1.21.9 uses ItemCount > 0 to indicate presence
	// and has a complex switch for item details
	return models.InventorySlot{
		Present: true,
		ItemID:  slotItemID(slot),
		Count:   int32(slot.ItemCount),
	}
}

// ParseHeldItemSlot parses a held item slot packet.
// In 1.21.9, the slot field is varint (int32).
func (c *containerHandler) ParseHeldItemSlot(p pk.Packet) (int16, error) {
	pkt := cb.NewHeldItemSlot()
	if err := pkt.Scan(p); err != nil {
		return 0, common.ErrPacketParse{PacketName: "HeldItemSlot", Cause: err}
	}

	// In 1.21.9, Slot is pk.VarInt (int32)
	return int16(pkt.Slot), nil
}

// DecodeSlot reads one full (non-hashed) Slot in 1.21.9's real wire format --
// satisfies bot/screen.SlotCodec via versionHandlerAdapter. Used by
// ClientboundContainerSetContent/SetSlot/SetPlayerInventory in every
// version, including this one (only ServerboundContainerClick's item
// encoding switches to HashedSlot from 1.21.5 on -- see
// SendContainerClickV2 below). See
// docs/plans/SLOT_CODEC_IMPLEMENTATION_PLAN.md Phase 2a.
func (c *containerHandler) DecodeSlot(r io.Reader) (screen.Slot, int64, error) {
	var wireSlot basetypes.Slot
	n, err := wireSlot.ReadFrom(r)
	if err != nil {
		return screen.Slot{}, n, err
	}
	if wireSlot.ItemCount <= 0 {
		return screen.Slot{}, n, nil
	}

	def, ok := wireSlot.UnnamedType0001.(*basetypes.SlotUnnamedType0001Default)
	if !ok {
		return screen.Slot{}, n, fmt.Errorf("v1_21_9: unexpected non-empty slot payload type %T", wireSlot.UnnamedType0001)
	}

	result := screen.Slot{ID: def.ItemId, Count: wireSlot.ItemCount}

	if components := def.Components.Get(); components != nil {
		for _, component := range *components {
			typeID, ok := componentTypeNameToID[component.Type.Value]
			if !ok {
				return screen.Slot{}, n, fmt.Errorf("v1_21_9: unknown slot component type %q", component.Type.Value)
			}
			result.Components = append(result.Components, screen.SlotComponent{Type: pk.VarInt(typeID), Data: component.Data})
		}
	}

	if removeComponents := def.RemoveComponents.Get(); removeComponents != nil {
		for _, rc := range *removeComponents {
			typeID, ok := componentTypeNameToID[rc.Type.Value]
			if !ok {
				return screen.Slot{}, n, fmt.Errorf("v1_21_9: unknown slot component type %q (remove)", rc.Type.Value)
			}
			result.RemoveComponents = append(result.RemoveComponents, pk.VarInt(typeID))
		}
	}

	return result, n, nil
}

// hashedSlotFromScreenSlot converts mc-bot-go's screen.Slot into 1.21.9's own
// HashedSlot wire format: a presence flag plus a component-hash-only
// summary (real component payloads never cross the wire in this direction
// from 1.21.5 on -- see basetypes.HashedSlot's protodef comment).
func hashedSlotFromScreenSlot(slot *screen.Slot) (protocol_models.Option[basetypes.HashedSlot], error) {
	if slot == nil || slot.Count <= 0 {
		return protocol_models.Option[basetypes.HashedSlot]{Has: false}, nil
	}

	hashed := basetypes.HashedSlot{ItemId: slot.ID, ItemCount: slot.Count}

	components := make([]basetypes.HashedSlotComponentsArrayType, 0, len(slot.Components))
	for _, component := range slot.Components {
		name, ok := componentTypeIDToName[int32(component.Type)]
		if !ok {
			return protocol_models.Option[basetypes.HashedSlot]{}, fmt.Errorf("v1_21_9: unknown slot component type ID %d", component.Type)
		}
		hashValue, err := common.ComponentHash(component.Data)
		if err != nil {
			return protocol_models.Option[basetypes.HashedSlot]{}, err
		}
		components = append(components, basetypes.HashedSlotComponentsArrayType{
			Type: basetypes.SlotComponentType{Value: name},
			Hash: pk.Int(hashValue),
		})
	}
	hashed.Components.Set(components)

	removeComponents := make([]basetypes.HashedSlotRemoveComponentsArrayType, 0, len(slot.RemoveComponents))
	for _, componentType := range slot.RemoveComponents {
		name, ok := componentTypeIDToName[int32(componentType)]
		if !ok {
			return protocol_models.Option[basetypes.HashedSlot]{}, fmt.Errorf("v1_21_9: unknown slot component type ID %d (remove)", componentType)
		}
		removeComponents = append(removeComponents, basetypes.HashedSlotRemoveComponentsArrayType{
			Type: basetypes.SlotComponentType{Value: name},
		})
	}
	hashed.RemoveComponents.Set(removeComponents)

	return protocol_models.Option[basetypes.HashedSlot]{Has: true, Val: &hashed}, nil
}

// SendContainerClickV2 builds and writes a complete ServerboundContainerClick
// packet in 1.21.9's real wire format (HashedSlot item encoding, VarInt
// window ID), using mc-bot-go's Slot type directly so item components
// round-trip (unlike SendContainerClick/InventorySlot above, which drops
// every component -- see docs/plans/SLOT_CODEC_IMPLEMENTATION_PLAN.md's
// Background). Satisfies bot/screen.SlotCodec's SendContainerClick via
// versionHandlerAdapter. See
// docs/plans/SLOT_CODEC_IMPLEMENTATION_PLAN.md Phase 2b.
func (c *containerHandler) SendContainerClickV2(conn bot.PacketWriter, windowID int, stateID int32, slot int16, button byte, mode int32, changedSlots screen.ChangedSlots, cursor *screen.Slot) error {
	pkt := sb.NewWindowClick()
	pkt.SetPacketID(int32(c.packetMgr.GetServerboundPacketID("ServerboundContainerClick")))
	pkt.WindowId = basetypes.ContainerID(windowID)
	pkt.StateId = pk.VarInt(stateID)
	pkt.Slot = pk.Short(slot)
	pkt.MouseButton = pk.Byte(button)
	pkt.Mode = pk.VarInt(mode)

	changedSlotsArr := make([]sb.WindowClickChangedSlotsArrayType, 0, len(changedSlots))
	for location, slotData := range changedSlots {
		item, err := hashedSlotFromScreenSlot(slotData)
		if err != nil {
			return err
		}
		changedSlotsArr = append(changedSlotsArr, sb.WindowClickChangedSlotsArrayType{
			Location: pk.Short(location),
			Item:     item,
		})
	}
	pkt.ChangedSlots.Set(changedSlotsArr)

	cursorItem, err := hashedSlotFromScreenSlot(cursor)
	if err != nil {
		return err
	}
	pkt.CursorItem = cursorItem

	return conn.WritePacket(pkt.Marshal())
}

// SendContainerButtonClick sends a container button click packet.
func (c *containerHandler) SendContainerButtonClick(conn models.PacketWriter, windowID int8, buttonID int8) error {
	pkt := sb.NewEnchantItem()
	pkt.WindowId = basetypes.ContainerID(windowID)
	pkt.Enchantment = pk.VarInt(buttonID)

	log.Printf("[v1.21.9 Container] SendContainerButtonClick: windowID=%d buttonID=%d", windowID, buttonID)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "EnchantItem", Cause: err}
	}
	return nil
}
