package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	"github.com/reallyoldfogie/mc-agent/models"
)

// rememberedContainerOpenTimeout mirrors craftTableOpenTimeout (craft.go)
// — the same bound on how long to wait for the server to confirm a
// container actually opened.
const rememberedContainerOpenTimeout = 5 * time.Second

// MineRememberedBlock implements models.WorldMemoryAgent: looks up
// blockName's nearest remembered, unreserved, loose sighting
// (NearestRememberedBlock) and attempts to mine it there. found=false
// (not an error) if nothing is remembered for blockName.
//
// No pre-verification of the remembered position happens here —
// MineBlockAt already approaches it itself (models.ApproachBlock) and
// re-reads the live block state before doing anything, returning cleanly
// if it's already air (see that function's own "already air" comment),
// so a stale memory is harmless by construction, not something this
// needs to guard against separately. The face argument is ignored by
// MineBlockAt itself (same reasoning as its other callers — see
// agent.go's mine-callback setup) — there's no real direction to pick
// ahead of time.
func (a *agent) MineRememberedBlock(ctx context.Context, blockName string) (found bool, err error) {
	pos, found, err := a.NearestRememberedBlock(blockName)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if err := a.MineBlockAt(ctx, pos, models.FaceUp); err != nil {
		return true, fmt.Errorf("mine remembered %s at %v: %w", blockName, pos, err)
	}
	return true, nil
}

// FetchRememberedItem implements models.WorldMemoryAgent: looks up
// itemName's nearest remembered, unreserved container sighting,
// navigates there, opens the container, takes the item into inventory,
// and closes it again. found=false (not an error) if nothing is
// remembered for itemName.
//
// Re-scans the opened container's current contents for itemName rather
// than trusting the remembered slot index blindly — contents can shift
// between the sighting and now (another bot, or this one, moving
// things) — the same "verify on arrival" principle the plan's own Open
// Questions section recommends. A remembered container that no longer
// actually has the item is reported as a plain "not found there any
// more" error, not a silent no-op or a false positive — unlike mining a
// stale position (a harmless no-op either way), failing to fetch
// something the caller specifically asked for is worth surfacing.
func (a *agent) FetchRememberedItem(ctx context.Context, itemName string) (found bool, err error) {
	normalized := normalizeItemName(itemName)
	pos, _, found := a.nearestUnreservedSighting(normalized, memorystore.SightingQuery{ContainerOnly: true})
	if !found {
		return false, nil
	}

	if err := models.ApproachBlock(ctx, a, pos, models.ApproachOptions{Announce: true, RequireSight: true}); err != nil {
		return true, fmt.Errorf("move to remembered container for %s: %w", itemName, err)
	}

	windowID, err := a.OpenContainerAt(ctx, pos.X, pos.Y, pos.Z, models.FaceUp, rememberedContainerOpenTimeout)
	if err != nil {
		return true, fmt.Errorf("open remembered container for %s: %w", itemName, err)
	}
	defer func() { _ = a.CloseContainer() }()

	itemMgr := a.GetItemManager()
	if itemMgr == nil {
		return true, fmt.Errorf("fetch remembered %s: item manager not available", itemName)
	}

	chestSlot := -1
	for idx, slot := range a.ChestSlots(windowID) {
		if slot.Count <= 0 {
			continue
		}
		if normalizeItemName(itemMgr.GetItemNameByID(slot.ItemID)) == normalized {
			chestSlot = idx
			break
		}
	}
	if chestSlot < 0 {
		return true, fmt.Errorf("fetch remembered %s: no longer found in the remembered container", itemName)
	}

	if err := a.TakeItemFromChest(windowID, int16(chestSlot)); err != nil {
		return true, fmt.Errorf("take remembered %s: %w", itemName, err)
	}
	return true, nil
}
