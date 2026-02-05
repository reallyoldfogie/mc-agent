package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/basic"
)

const (
	minHotbarSlot = 0
	maxHotbarSlot = 8
)

// SelectHotbarSlot switches the active hotbar slot and waits for server ack.
func (a *agent) SelectHotbarSlot(ctx context.Context, slot int) error {
	if slot < minHotbarSlot || slot > maxHotbarSlot {
		return fmt.Errorf("invalid hotbar slot %d (expected %d-%d)", slot, minHotbarSlot, maxHotbarSlot)
	}
	if a.packetMgr == nil || a.client == nil {
		return ErrInvalidConfig("packet manager or client not initialized")
	}
	if a.heldSlotUpdates == nil {
		return ErrInvalidConfig("held slot tracking not initialized")
	}

	a.heldSlotMu.RLock()
	currentSet := a.heldSlotSet
	current := a.heldSlot
	a.heldSlotMu.RUnlock()
	if currentSet && current == int16(slot) {
		a.logHotbarSelection("already selected", slot, current)
		return nil
	}

	a.logHotbarSelection("selecting", slot, current)
	packetID := a.packetMgr.GetServerboundPacketID("ServerboundHeldItemSlot")
	pkt, err := a.packetMgr.GetServerboundPacketByID(packetID)
	if err != nil {
		return fmt.Errorf("create held item slot packet: %w", err)
	}
	pkt.SetFields(map[string]pk.FieldEncoder{
		"SlotId": pk.Short(slot),
	})
	if err := a.client.Conn().WritePacket(pkt.Marshal()); err != nil {
		return fmt.Errorf("send held item slot packet: %w", err)
	}

	for {
		a.heldSlotMu.RLock()
		currentSet = a.heldSlotSet
		current = a.heldSlot
		a.heldSlotMu.RUnlock()
		if currentSet && current == int16(slot) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case ackSlot := <-a.heldSlotUpdates:
			if ackSlot == int16(slot) {
				a.logHotbarAck(slot, ackSlot)
				return nil
			}
		}
	}
}

func (a *agent) logHotbarSelection(action string, targetSlot int, currentSlot int16) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		log.Printf("[Agent %s] Hotbar %s: target=%d current=%d", a.client.Name(), action, targetSlot, currentSlot)
		return
	}

	currentName, currentCount := a.resolveHotbarSlot(slots, itemMgr, int(currentSlot))
	targetName, targetCount := a.resolveHotbarSlot(slots, itemMgr, targetSlot)
	log.Printf("[Agent %s] Hotbar %s: target=%d (%s x%d) current=%d (%s x%d)",
		a.client.Name(), action, targetSlot, targetName, targetCount, currentSlot, currentName, currentCount)
	a.logPlayerInventory(action)
}

func (a *agent) logHotbarAck(targetSlot int, ackSlot int16) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		log.Printf("[Agent %s] Hotbar ack: target=%d ack=%d", a.client.Name(), targetSlot, ackSlot)
		return
	}
	ackName, ackCount := a.resolveHotbarSlot(slots, itemMgr, int(ackSlot))
	log.Printf("[Agent %s] Hotbar ack: target=%d ack=%d (%s x%d)", a.client.Name(), targetSlot, ackSlot, ackName, ackCount)
}

func (a *agent) logPlayerInventory(context string) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		log.Printf("[Agent %s] Inventory (%s): slot resolver not ready", a.client.Name(), context)
		return
	}

	var b strings.Builder
	b.WriteString("[Agent")
	b.WriteString(a.client.Name())
	b.WriteString("] Inventory ")
	b.WriteString(context)
	b.WriteString(":")
	for i := range 46 {
		name, count := a.resolveInventorySlot(slots, itemMgr, i)
		fmt.Fprintf(&b, " %d=%s x%d", i, name, count)
	}
	log.Print(b.String())
}

func (a *agent) getSlotInfoDeps() (models.SlotResolver, models.ItemManager) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.slots, a.itemMgr
}

func (a *agent) resolveHotbarSlot(slots models.SlotResolver, itemMgr models.ItemManager, slot int) (string, int) {
	if slot < minHotbarSlot || slot > maxHotbarSlot {
		return "invalid", 0
	}
	// Player inventory hotbar slots are indexes 36-44.
	itemID, count, ok := slots.ResolveSlot(-2, 36+slot)
	if !ok {
		return "minecraft:air", 0
	}
	name := itemMgr.GetItemNameByID(itemID)
	if name == "" {
		name = fmt.Sprintf("item_%d", itemID)
	}
	return name, count
}

func (a *agent) resolveInventorySlot(slots models.SlotResolver, itemMgr models.ItemManager, index int) (string, int) {
	itemID, count, ok := slots.ResolveSlot(-2, index)
	if !ok {
		return "minecraft:air", 0
	}
	name := itemMgr.GetItemNameByID(itemID)
	if name == "" {
		name = fmt.Sprintf("item_%d", itemID)
	}
	return name, count
}

// FindHotbarSlotWithItem finds a hotbar slot containing an item by name.
// Returns the slot index (0-8) and true if found, or -1 and false if not found.
func (a *agent) FindHotbarSlotWithItem(ctx context.Context, itemName string) (int, bool) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		return -1, false
	}

	for slot := minHotbarSlot; slot <= maxHotbarSlot; slot++ {
		slotName, _ := a.resolveHotbarSlot(slots, itemMgr, slot)
		if slotName == itemName {
			return slot, true
		}
	}
	return -1, false
}

// EquipItemByName finds and equips an item from the hotbar by name.
// Waits for server acknowledgment before returning.
func (a *agent) EquipItemByName(ctx context.Context, itemName string) error {
	slot, found := a.FindHotbarSlotWithItem(ctx, itemName)
	if !found {
		return fmt.Errorf("item %s not found in hotbar", itemName)
	}
	log.Printf("[Agent %s] Found %s in hotbar slot %d, equipping...", a.client.Name(), itemName, slot)
	return a.SelectHotbarSlot(ctx, slot)
}

func (a *agent) initClientInformationHandler(settings basic.Settings) {
	if a.client == nil || a.packetMgr == nil {
		return
	}

	// Add a high-priority handler to send version-specific client information
	// This runs BEFORE the default mc-bot-go handler (priority 0)
	packetID := a.packetMgr.GetClientboundPacketID("ClientboundLogin")
	a.client.Events().AddListener(bot.PacketHandler{
		ID:       packetID,
		Priority: -10, // Higher priority (runs before default handler at priority 0)
		F: func(p pk.Packet) error {
			if a.versionHandler == nil {
				return nil // silently ignore if no version handler
			}

			// Send minecraft:brand custom payload first using version handler
			if err := a.versionHandler.Play().SendCustomPayload(a.client.Conn(), "minecraft:brand", settings.Brand); err != nil {
				return err
			}

			// Send client information using version handler
			info := common.ClientInfo{
				Locale:              settings.Locale,
				ViewDistance:        int8(settings.ViewDistance),
				ChatMode:            int32(settings.ChatMode),
				ChatColors:          settings.ChatColors,
				DisplayedSkinParts:  settings.DisplayedSkinParts,
				MainHand:            int32(settings.MainHand),
				EnableTextFiltering: settings.EnableTextFiltering,
				AllowServerListings: settings.AllowListing,
			}
			if err := a.versionHandler.Play().SendClientInformation(a.client.Conn(), info); err != nil {
				return err
			}
			return nil
		},
	})
}

func (a *agent) initHeldSlotTracking() {
	if a.client == nil || a.packetMgr == nil || a.heldSlotUpdates != nil {
		return
	}

	a.heldSlotUpdates = make(chan int16, 16)
	packetID := a.packetMgr.GetClientboundPacketID("ClientboundHeldItemSlot")
	a.client.Events().AddListener(bot.PacketHandler{
		ID:       packetID,
		Priority: 64,
		F: func(p pk.Packet) error {
			if a.versionHandler == nil {
				return nil // silently ignore if no version handler
			}

			slot, err := a.versionHandler.Play().Containers().ParseHeldItemSlot(p)
			if err != nil {
				return nil // ignore malformed packets
			}

			a.heldSlotMu.Lock()
			a.heldSlot = slot
			a.heldSlotSet = true
			a.heldSlotMu.Unlock()

			select {
			case a.heldSlotUpdates <- slot:
			default:
			}
			return nil
		},
	})
}
