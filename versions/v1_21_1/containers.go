// Package v1_21_5 provides version-specific packet handling for Minecraft 1.21.4.
package v1_21_1

import (
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/basetypes"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.1/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// containerHandler implements common.ContainerHandler for 1.21.4.
type containerHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendContainerClick sends a container click packet.
// This is a complex packet with changed slots and cursor item tracking.
func (c *containerHandler) SendContainerClick(conn common.PacketWriter, windowID int8, stateID, slot int32, button int8, mode int32, changedSlots map[int16]common.Slot, carriedItem common.Slot) error {
	pkt := sb.NewWindowClick()
	pkt.WindowId = basetypes.ContainerID(windowID)
	pkt.StateId = pk.VarInt(stateID)
	pkt.Slot = pk.Short(slot)
	pkt.MouseButton = pk.Byte(button)
	pkt.Mode = pk.VarInt(mode)

	// Convert changed slots to protocol format
	// Note: In 1.21.4, Slot structure uses ItemCount + UnnamedType0003 switch field
	changedSlotsArr := make([]sb.WindowClickChangedSlotsArrayType, 0, len(changedSlots))
	for slotNum, slotData := range changedSlots {
		entry := sb.WindowClickChangedSlotsArrayType{
			Location: pk.Short(slotNum),
		}
		// Set the item data - in 1.21.4, Slot uses ItemCount and UnnamedType0003
		if slotData.Present {
			entry.Item.ItemCount = pk.VarInt(slotData.Count)
			// UnnamedType0003 would need proper initialization for full support
			// This is a simplified version - full implementation would require
			// proper handling of item components
		} else {
			entry.Item.ItemCount = pk.VarInt(0)
		}
		changedSlotsArr = append(changedSlotsArr, entry)
	}
	pkt.ChangedSlots.Set(&changedSlotsArr)

	// Set cursor item - same Slot structure
	if carriedItem.Present {
		pkt.CursorItem.ItemCount = pk.VarInt(carriedItem.Count)
		// UnnamedType0003 would need proper initialization
	} else {
		pkt.CursorItem.ItemCount = pk.VarInt(0)
	}

	return conn.WritePacket(pkt.Marshal())
}

// SendContainerClose sends a container close packet.
func (c *containerHandler) SendContainerClose(conn common.PacketWriter, windowID int8) error {
	pkt := sb.NewCloseWindow()
	pkt.WindowId = basetypes.ContainerID(windowID)

	return conn.WritePacket(pkt.Marshal())
}

// SendSetCreativeModeSlot sends a creative mode slot update.
func (c *containerHandler) SendSetCreativeModeSlot(conn common.PacketWriter, slot int16, item common.Slot) error {
	pkt := sb.NewSetCreativeSlot()
	pkt.Slot = pk.Short(slot)

	// Set the untrusted slot data
	if item.Present {
		pkt.Item.ItemCount = pk.VarInt(item.Count)
		// The UnnamedType0001 field handles the item ID and components
		// This is complex and requires proper NBT encoding
	} else {
		pkt.Item.ItemCount = pk.VarInt(0)
	}

	return conn.WritePacket(pkt.Marshal())
}

// SendPickItem sends a pick item packet (for creative mode).
// Note: In 1.21.4, the PickItem packet takes a block position, not a slot.
// This implementation is a stub - the interface may need to be updated.
func (c *containerHandler) SendPickItem(conn common.PacketWriter, slot int32) error {
	// The 1.21.4 protocol changed PickItem to PickItemFromBlock which takes a position
	// instead of a slot. This interface needs to be redesigned for 1.21.4+.
	return common.ErrPacketSend{PacketName: "PickItem", Cause: nil}
}

// SendSetCarriedItem sends a held item change packet.
func (c *containerHandler) SendSetCarriedItem(conn common.PacketWriter, slot int16) error {
	pkt := sb.NewHeldItemSlot()
	pkt.SlotId = pk.Short(slot)

	return conn.WritePacket(pkt.Marshal())
}

// SendUseItemOn sends a use item on block packet (right-click on block).
// This is used for opening containers, placing blocks, and interacting with blocks.
// Note: 1.21.1 does NOT have the WorldBorderHit field (added in 1.21.2/protocol 768).
func (c *containerHandler) SendUseItemOn(conn common.PacketWriter, hand int32, x, y, z int, face int32, cursorX, cursorY, cursorZ float32, insideBlock bool, sequence int32) error {
	pkt := sb.NewBlockPlace()
	pkt.Hand = pk.VarInt(hand)
	pkt.Location = basetypes.Position{X: int64(x), Y: int64(y), Z: int64(z)}
	pkt.Direction = pk.VarInt(face)
	pkt.CursorX = pk.Float(cursorX)
	pkt.CursorY = pk.Float(cursorY)
	pkt.CursorZ = pk.Float(cursorZ)
	pkt.InsideBlock = pk.Boolean(insideBlock)
	// Note: 1.21.1 does NOT have WorldBorderHit field
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
func (c *containerHandler) ParseContainerSetContent(p pk.Packet) (windowID int8, stateID int32, slots []common.Slot, carriedItem common.Slot, err error) {
	pkt := cb.NewWindowItems()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, nil, common.Slot{}, common.ErrPacketParse{PacketName: "WindowItems", Cause: err}
	}

	windowID = int8(pkt.WindowId)
	stateID = int32(pkt.StateId)

	// Convert items array
	items := pkt.Items.Get()
	slots = make([]common.Slot, len(*items))
	for i, item := range *items {
		slots[i] = convertSlotFromProtocol(item)
	}

	// Convert carried item
	carriedItem = convertSlotFromProtocol(pkt.CarriedItem)

	return windowID, stateID, slots, carriedItem, nil
}

// ParseContainerSetSlot parses a slot update packet.
func (c *containerHandler) ParseContainerSetSlot(p pk.Packet) (windowID int8, stateID int32, slot int16, item common.Slot, err error) {
	pkt := cb.NewSetSlot()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, common.Slot{}, common.ErrPacketParse{PacketName: "SetSlot", Cause: err}
	}

	windowID = int8(pkt.WindowId)
	stateID = int32(pkt.StateId)
	slot = int16(pkt.Slot)
	item = convertSlotFromProtocol(pkt.Item)

	return windowID, stateID, slot, item, nil
}

// ParseHeldItemSlot parses a held item slot packet.
// In 1.21.1, the slot field is i8 (byte).
func (h *containerHandler) ParseHeldItemSlot(p pk.Packet) (int16, error) {
	pkt := cb.NewHeldItemSlot()
	if err := pkt.Scan(p); err != nil {
		return 0, common.ErrPacketParse{PacketName: "HeldItemSlot", Cause: err}
	}

	// In 1.21.1, Slot is pk.Byte (int8)
	return int16(pkt.Slot), nil
}

// convertSlotFromProtocol converts a protocol Slot to common.Slot.
func convertSlotFromProtocol(slot basetypes.Slot) common.Slot {
	if slot.ItemCount <= 0 {
		return common.Slot{Present: false}
	}

	// The slot structure in 1.21.4 uses ItemCount > 0 to indicate presence
	// and has a complex switch for item details
	result := common.Slot{
		Present: true,
		Count:   int32(slot.ItemCount),
	}

	// The item ID is stored in a switch field that's harder to access directly
	// For now, we mark it as present with the count
	// TODO: Extract itemId from the switch field properly

	return result
}
