package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"

	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/basic"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

const (
	minHotbarSlot = int16(0)
	maxHotbarSlot = int16(8)
)

// SelectHotbarSlot switches the active hotbar slot and waits for server ack.
func (a *agent) SelectHotbarSlot(ctx context.Context, slot int16) error {
	if slot < minHotbarSlot || slot > maxHotbarSlot {
		return fmt.Errorf("invalid hotbar slot %d (expected %d-%d)", slot, minHotbarSlot, maxHotbarSlot)
	}
	if a.packetMgr == nil || a.client == nil {
		return models.ErrInvalidConfig("packet manager or client not initialized")
	}
	if a.heldSlotUpdates == nil {
		return models.ErrInvalidConfig("held slot tracking not initialized")
	}

	a.heldSlotMu.RLock()
	currentSet := a.heldSlotSet
	current := a.heldSlot
	a.heldSlotMu.RUnlock()
	if currentSet && current == int16(slot) {
		a.logHotbarSelection("already selected", slot, current)
		a.emitHeldItemEquipment(slot)
		return nil
	}

	a.logHotbarSelection("selecting", slot, current)
	packetID := a.packetMgr.GetServerboundPacketID("ServerboundHeldItemSlot")
	pkt, err := a.packetMgr.GetServerboundPacketByID(packetID)
	if err != nil {
		return fmt.Errorf("create held item slot packet: %w", err)
	}
	slotId := pk.Short(slot)
	pkt.SetFields(map[string]pk.FieldEncoder{
		"SlotId": &slotId,
	})
	if err := a.client.Conn().WritePacket(pkt.Marshal()); err != nil {
		return fmt.Errorf("send held item slot packet: %w", err)
	}

	// ServerboundHeldItemSlot has no vanilla acknowledgment: unlike this
	// codebase's original assumption (this function used to block here
	// waiting for a ClientboundHeldItemSlot echo), the server does not
	// send that packet back to the client that requested the switch — it's
	// a server-initiated notification (e.g. another game mechanic swapping
	// what's in the player's hand), not an ack of the player's own request.
	// Waiting for it here meant every switch to a slot other than the
	// already-selected one blocked until ctx expired — confirmed live
	// while adding testing/mine_test.go. Update the local state
	// optimistically instead, the same way movement/rotation packets in
	// this codebase are fire-and-forget.
	a.heldSlotMu.Lock()
	a.heldSlot = slot
	a.heldSlotSet = true
	a.heldSlotMu.Unlock()
	a.emitHeldItemEquipment(slot)
	return nil
}

func (a *agent) SelectEmptyHotbarSlot(ctx context.Context) error {
	var emptySlot int16 = -1
	inventory := a.GetInventory()
	hotbarSlots := inventory.Hotbar()
	// for slot := mcscreen.HotbarSlotStart; slot < mcscreen.HotbarSlotEnd; slot++ {
	for idx, slot := range hotbarSlots {
		if slot.Count == 0 {
			emptySlot = int16(idx)
			break
		}
	}

	if emptySlot == -1 {
		return errors.New("no empty slot in player inventory")
	}

	return a.SelectHotbarSlot(ctx, emptySlot)
}

// SwapInventoryWithHotbar swaps an inventory slot with a hotbar slot by clicking with hotbar mode
// This is equivalent to pressing a number key while hovering over an inventory item
func (a *agent) SwapInventoryWithHotbar(ctx context.Context, inventorySlot, hotbarSlot int) error {
	if a.versionHandler == nil || a.client == nil {
		return fmt.Errorf("version handler or client not available")
	}

	if hotbarSlot < 0 || hotbarSlot > 8 {
		return fmt.Errorf("invalid hotbar slot %d (must be 0-8)", hotbarSlot)
	}

	// Send a container click packet with hotbar swap mode
	// windowID 0 = player inventory
	// mode 2 = hotbar swap
	// button = hotbar slot number (0-8)
	containerHandler := a.versionHandler.Play().Containers()
	if containerHandler == nil {
		return fmt.Errorf("container handler not available")
	}

	err := containerHandler.SendContainerClick(
		a.client.Conn(),
		0,                                    // windowID = player inventory
		0,                                    // stateID
		int32(inventorySlot),                 // slot to swap from
		int8(hotbarSlot),                     // button = hotbar slot to swap to
		2,                                    // mode = hotbar key press (swap mode)
		make(map[int16]models.InventorySlot), // let server respond with changes
		models.InventorySlot{},               // cursor item
	)

	if err != nil {
		return fmt.Errorf("error performing hotbar swap: %w", err)
	}

	// Wait for the swap to complete
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (a *agent) logHotbarSelection(action string, targetSlot int16, currentSlot int16) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		a.logf("[Agent %s] Hotbar %s: target=%d current=%d", a.cfg.Name, action, targetSlot, currentSlot)
		return
	}

	currentName, currentCount := a.resolveHotbarSlot(slots, itemMgr, currentSlot)
	targetName, targetCount := a.resolveHotbarSlot(slots, itemMgr, targetSlot)
	a.logf("[Agent %s] Hotbar %s: target=%d (%s x%d) current=%d (%s x%d)",
		a.cfg.Name, action, targetSlot, targetName, targetCount, currentSlot, currentName, currentCount)
	a.logPlayerInventory(action)
}

func (a *agent) logHotbarAck(targetSlot int16, ackSlot int16) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		a.logf("[Agent %s] Hotbar ack: target=%d ack=%d", a.cfg.Name, targetSlot, ackSlot)
		return
	}
	ackName, ackCount := a.resolveHotbarSlot(slots, itemMgr, ackSlot)
	a.logf("[Agent %s] Hotbar ack: target=%d ack=%d (%s x%d)", a.cfg.Name, targetSlot, ackSlot, ackName, ackCount)
}

func (a *agent) logPlayerInventory(context string) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		a.logf("[Agent %s] Inventory (%s): slot resolver not ready", a.cfg.Name, context)
		return
	}

	var b strings.Builder
	b.WriteString("[Agent")
	b.WriteString(a.cfg.Name)
	b.WriteString("] Inventory ")
	b.WriteString(context)
	b.WriteString(":")
	for i := range int16(46) {
		name, count := a.resolveInventorySlot(slots, itemMgr, i)
		fmt.Fprintf(&b, " %d=%s x%d", i, name, count)
	}
	a.logp(b.String())
}

// LogInventory writes the full player inventory contents to the given writer.
// Each non-empty slot is printed on its own line with slot index, item name, and count.
// Hotbar slots (36-44) are labeled separately for clarity.
func (a *agent) LogInventory(output io.Writer) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		fmt.Fprintf(output, "[Agent %s] Inventory: slot resolver not ready\n", a.cfg.Name)
		return
	}

	fmt.Fprintf(output, "[Agent %s] Inventory:\n", a.cfg.Name)
	for i := range int16(46) {
		name, count := a.resolveInventorySlot(slots, itemMgr, i)
		if name == "minecraft:air" || count == 0 {
			continue
		}
		label := "inv"
		if i >= 36 && i <= 44 {
			label = fmt.Sprintf("hotbar[%d]", i-36)
		}
		fmt.Fprintf(output, "  slot %2d (%s): %s x%d\n", i, label, name, count)
	}
}

func (a *agent) getSlotInfoDeps() (models.SlotResolver, models.ItemManager) {
	a.slotsMu.RLock()
	defer a.slotsMu.RUnlock()
	a.itemMgrMu.RLock()
	defer a.itemMgrMu.RUnlock()
	return a.slots, a.itemMgr
}

// emitHeldItemEquipment resolves the item in the given hotbar slot and emits
// an EntityEquipment packet to the replay mirror so the replay viewer can
// render the held item.
func (a *agent) emitHeldItemEquipment(slot int16) {
	if a.moveMirror == nil {
		return
	}
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		return
	}
	itemID, count, ok := slots.ResolveSlot(-2, mcscreen.HotbarSlotStart+slot)
	if !ok {
		itemID = 0
		count = 0
	}
	a.moveMirror.EmitEquipment(a.GetEntityID(), models.EquipmentSlotMainHand, int32(itemID), int32(count))
}

// resyncHandEquipment re-emits main-hand and off-hand equipment to the
// replay mirror based on current inventory state. Container-click
// operations (ShiftClickSlot, LeftClickSlot, etc.) apply their predicted
// result to the local screen state immediately, but vanilla servers often
// skip sending a redundant ClientboundContainerSetSlot correction for a
// prediction that was already correct — most commonly the *source* slot of
// a shift-click, which the server assumes the client already knows became
// empty. Since onSetSlot (the mirror's normal hook) only fires from
// incoming packets, a hand-slot item moved away by a locally-predicted
// click can end up never reported as cleared, leaving the replay showing
// an item still held after it was actually moved elsewhere (e.g. shift-
// clicking an item from the hotbar straight into an armor slot). Callers
// should call this after any click operation that could plausibly affect
// either hand slot, since it is a full re-read of current state rather
// than a delta and is safe to call unconditionally.
func (a *agent) resyncHandEquipment() {
	if a.moveMirror == nil {
		return
	}

	a.heldSlotMu.RLock()
	heldSlot := a.heldSlot
	heldSlotSet := a.heldSlotSet
	a.heldSlotMu.RUnlock()
	if heldSlotSet {
		a.emitHeldItemEquipment(heldSlot)
	}

	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		return
	}
	const offhandSlotIndex int16 = 45
	itemID, count, ok := slots.ResolveSlot(-2, offhandSlotIndex)
	if !ok {
		itemID, count = 0, 0
	}
	a.moveMirror.EmitEquipment(a.GetEntityID(), models.EquipmentSlotOffHand, int32(itemID), int32(count))
}

func (a *agent) resolveHotbarSlot(slots models.SlotResolver, itemMgr models.ItemManager, slot int16) (string, int) {
	if slot < minHotbarSlot || slot > maxHotbarSlot {
		return "invalid", 0
	}
	// Player inventory hotbar slots are indexes 36-44.
	itemID, count, ok := slots.ResolveSlot(-2, mcscreen.HotbarSlotStart+slot)
	if !ok {
		return "minecraft:air", 0
	}
	name := itemMgr.GetItemNameByID(itemID)
	if name == "" {
		// Unknown item ID - construct name but log it for debugging
		name = fmt.Sprintf("item_%d", itemID)
		if slot >= 0 && slot <= 8 && count > 0 {
			a.logf("[DEBUG] Hotbar slot %d: Unknown item ID %d (count=%d) - no registry entry", slot, itemID, count)
		}
	}
	return name, count
}

func (a *agent) resolveInventorySlot(slots models.SlotResolver, itemMgr models.ItemManager, index int16) (string, int) {
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
func (a *agent) FindHotbarSlotWithItem(ctx context.Context, itemName string) (int16, bool) {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		a.logf("[%s] FindHotbarSlotWithItem(%s): slots=%v, itemMgr=%v", a.cfg.Name, itemName, slots, itemMgr)
		return -1, false
	}

	var hotbarDebug []string
	for slot := minHotbarSlot; slot <= maxHotbarSlot; slot++ {
		slotName, count := a.resolveHotbarSlot(slots, itemMgr, slot)
		hotbarDebug = append(hotbarDebug, fmt.Sprintf("[%d]=%s(x%d)", slot, slotName, count))
		if slotName == itemName {
			return slot, true
		}
	}
	a.logf("[%s] FindHotbarSlotWithItem(%s): NOT FOUND. Hotbar contents: %v", a.cfg.Name, itemName, hotbarDebug)
	return -1, false
}

// SwitchToItem finds an item by name anywhere in the player's inventory and equips it.
// If the item is already in the hotbar, it selects that slot directly.
// If the item is in the main inventory, it swaps it into a hotbar slot and selects it.
// Returns (nil, true) if the item was found and equipped, (nil, false) if not found,
// or (err, false) on error.
func (a *agent) SwitchToItem(ctx context.Context, itemName string) (bool, error) {
	// Check hotbar first (fast path)
	hotbarSlot, found := a.FindHotbarSlotWithItem(ctx, itemName)
	if found {
		return true, a.SelectHotbarSlot(ctx, hotbarSlot)
	}

	// Search entire player inventory (windowID -2)
	slotIndex, found, err := a.FindSlotWith(ctx, itemName, -2)
	if err != nil {
		return false, fmt.Errorf("search inventory for %s: %w", itemName, err)
	}
	if !found {
		return false, nil
	}

	// Item is in main inventory — swap it into hotbar slot 0
	targetHotbar := 0
	if err := a.SwapInventoryWithHotbar(ctx, slotIndex, targetHotbar); err != nil {
		return false, fmt.Errorf("swap %s to hotbar: %w", itemName, err)
	}

	return true, a.SelectHotbarSlot(ctx, int16(targetHotbar))
}

// EquipItemByName finds and equips an item from the hotbar by name.
// Waits for server acknowledgment before returning.
func (a *agent) EquipItemByName(ctx context.Context, itemName string) error {
	slot, found := a.FindHotbarSlotWithItem(ctx, itemName)
	if !found {
		return fmt.Errorf("item %s not found in hotbar", itemName)
	}
	a.logf("[Agent %s] Found %s in hotbar slot %d, equipping...", a.cfg.Name, itemName, slot)
	return a.SelectHotbarSlot(ctx, slot)
}

// WaitForHotbarItem waits for a specific item to appear in the hotbar.
// This is useful after RCON commands that place items, as there may be inventory sync delays.
// Returns the slot index when found, or error if timeout/context cancelled.
func (a *agent) WaitForHotbarItem(ctx context.Context, itemName string, maxWaitMS int) (int16, error) {
	// Use a shorter check interval (50ms) with longer total timeout
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(time.Duration(maxWaitMS) * time.Millisecond)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-timeout.C:
			return -1, fmt.Errorf("timeout waiting for %s to appear in hotbar (waited %dms)", itemName, maxWaitMS)
		case <-ticker.C:
			slot, found := a.FindHotbarSlotWithItem(ctx, itemName)
			if found {
				a.logf("[Agent %s] Item %s appeared in hotbar slot %d", a.cfg.Name, itemName, slot)
				return slot, nil
			}
		}
	}
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
			info := models.ClientInfo{
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
