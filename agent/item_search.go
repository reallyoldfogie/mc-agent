package agent

import (
	"context"
	"fmt"
	"strings"

	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// FindSlotWith searches a screen window for the first slot containing itemName.
// windowID uses the same conventions as SlotResolver: -2 = player inventory, >=0 = container.
// Returns the slot index in that window, or found=false if not present.
func (a *agent) FindSlotWith(ctx context.Context, itemName string, windowID int) (int, bool, error) {
	if ctx.Err() != nil {
		return -1, false, ctx.Err()
	}
	if a.itemMgr == nil {
		return -1, false, ErrInvalidConfig("item manager not initialized")
	}

	normalized := normalizeItemName(itemName)
	if normalized == "" {
		return -1, false, fmt.Errorf("item name is empty")
	}

	var screenSlots []mcscreen.Slot
	switch {
	case windowID == -2:
		inv := a.GetInventory()
		if inv == nil {
			return -1, false, fmt.Errorf("inventory not available")
		}
		if inventory, ok := inv.(*mcscreen.Inventory); ok {
			screenSlots = inventory.Slots[:]
		}
	case windowID >= 0:
		screen := a.GetScreen(windowID)
		if screen == nil {
			return -1, false, fmt.Errorf("screen %d not available", windowID)
		}
		switch s := screen.(type) {
		case *mcscreen.GenericContainer:
			screenSlots = s.Slots
		case *mcscreen.Chest:
			screenSlots = s.Slots
		case *mcscreen.HorseContainer:
			screenSlots = s.Slots
		case *mcscreen.Inventory:
			screenSlots = s.Slots[:]
		}
	default:
		return -1, false, fmt.Errorf("unsupported windowID %d", windowID)
	}

	if len(screenSlots) == 0 {
		return -1, false, fmt.Errorf("no slots available for windowID %d", windowID)
	}

	matchEmpty := normalized == "minecraft:air"
	for idx, slot := range screenSlots {
		if ctx.Err() != nil {
			return -1, false, ctx.Err()
		}

		if slot.Count <= 0 {
			if matchEmpty {
				return idx, true, nil
			}
			continue
		}

		itemNameForSlot := normalizeItemName(a.itemMgr.GetItemNameByID(int(slot.ID)))
		if itemNameForSlot == normalized {
			return idx, true, nil
		}
	}

	return -1, false, nil
}

func normalizeItemName(name string) string {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "minecraft:") {
		return trimmed
	}
	return "minecraft:" + trimmed
}
