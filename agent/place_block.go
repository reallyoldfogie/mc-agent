package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

const (
	// placeVerifyWait is how long PlaceHeldBlock waits, after asking the
	// server to place a block, for it to show up in the bot's own world
	// view before calling that attempt a failure.
	//
	// 2026-10-02: raised 1.5s -> 5s, same false-negative-under-load reasoning
	// as craftConfirmTimeout/mineConfirmTimeout's bump to 10s (agent/craft.go,
	// agent/actions.go) — this was the shortest client-sync deadline in the
	// codebase. Kept below craft/mine's 10s rather than matched to it: unlike
	// a timed-out craft/mine (handled below by the 2-attempts/4-cells retry
	// wrapper regardless), a stuck PlaceHeldBlock already retries other cells
	// on failure, so a shorter deadline here still lets it move on and try
	// elsewhere rather than burning a full 10s per cell.
	placeVerifyWait = 5000 * time.Millisecond

	placePoll = 50 * time.Millisecond

	// placeAttemptsPerCell and placeMaxCells bound the retries: two tries
	// on a cell (the observed failure - a request the server ignored - is
	// usually transient), and at most this many distinct cells.
	placeAttemptsPerCell = 2
	placeMaxCells        = 4
)

// PlaceHeldBlock implements models.BlockPlacer. See that interface.
//
// It picks the nearest of models.PlacementCandidates whose cell is free
// (passable) with a solid, standable floor, equips the item, and right-clicks
// the floor's top face - the same sequence the Phase 0 spikes of
// docs/plans/12 showed placing a crafting table 19 times out of 20. The
// twentieth returned success and placed nothing, hence the verification.
// PlacementCandidates' second tier (one level down) is what finds a cell at
// all when the bot is standing on top of something rather than the floor -
// found live as chain_place/chain_use episodes reporting "no free cell"
// while standing a full block higher than the usual -60/-61 pair.
func (a *agent) PlaceHeldBlock(ctx context.Context, itemName string) (models.V3, error) {
	if a.InventoryCount(itemName) == 0 {
		return models.V3{}, fmt.Errorf("place %s: none in the inventory", itemName)
	}
	pos, _, _, ok := a.GetPosition()
	if !ok {
		return models.V3{}, fmt.Errorf("place %s: position not initialized", itemName)
	}
	world, shapeMgr := a.GetWorld(), a.BlockShapeManager()
	if world == nil || shapeMgr == nil {
		return models.V3{}, fmt.Errorf("place %s: world not available", itemName)
	}
	bx, by, bz := models.StandingCell(pos)

	want := normalizeItemName(itemName)
	var lastErr error
	tried, unusable := 0, 0
	for _, c := range models.PlacementCandidates(bx, by, bz) {
		if tried >= placeMaxCells {
			break
		}
		if !placementCellUsable(world, shapeMgr, c) {
			unusable++
			continue
		}
		tried++
		for attempt := 0; attempt < placeAttemptsPerCell; attempt++ {
			if err := ctx.Err(); err != nil {
				return models.V3{}, err
			}
			if err := a.equipAndConfirm(ctx, itemName, want); err != nil {
				lastErr = err
				continue
			}
			before := a.InventoryCount(itemName)
			if err := a.UseItemOnBlock(ctx, c.Floor.X, c.Floor.Y, c.Floor.Z, models.FaceUp, models.MainHand); err != nil {
				lastErr = fmt.Errorf("use %s on (%v): %w", itemName, c.Floor, err)
				continue
			}
			if a.waitForPlacement(ctx, c.Cell, want, itemName, before) {
				return c.Cell, nil
			}
			lastErr = fmt.Errorf("place %s at (%v): the block never appeared", itemName, c.Cell)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("place %s: no free cell with a floor beside the bot (standing in cell %d,%d,%d at y=%.4f; %d candidate cells unusable)", itemName, bx, by, bz, pos.Y, unusable)
	}
	return models.V3{}, lastErr
}

// placementCellUsable reports whether a block can go in c.Cell against
// c.Floor: the cell (and the one above it, so it doesn't wall the bot in) is
// passable and the floor is solid.
func placementCellUsable(world models.World, shapeMgr models.BlockShapeManager, c models.PlacementCell) bool {
	cellID, cellOK := world.GetBlockAt(c.Cell.X+0.5, c.Cell.Y+0.5, c.Cell.Z+0.5)
	aboveID, aboveOK := world.GetBlockAt(c.Cell.X+0.5, c.Cell.Y+1.5, c.Cell.Z+0.5)
	floorID, floorOK := world.GetBlockAt(c.Floor.X+0.5, c.Floor.Y+0.5, c.Floor.Z+0.5)
	if !cellOK || !aboveOK || !floorOK {
		return false
	}
	return shapeMgr.IsPassable(cellID) && shapeMgr.IsPassable(aboveID) && !shapeMgr.IsPassable(floorID)
}

// waitForBlock polls the bot's own world view until (x, y, z) shows want.
func (a *agent) waitForBlock(ctx context.Context, x, y, z int, want string) bool {
	deadline := time.Now().Add(placeVerifyWait)
	for {
		if a.BlockNameAt(x, y, z) == want {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(placePoll):
		}
	}
}

// heldItemName is the name of the item in the bot's selected hotbar slot
// ("minecraft:air" when empty, "" if it cannot be read).
func (a *agent) heldItemName() string {
	slots, itemMgr := a.getSlotInfoDeps()
	if slots == nil || itemMgr == nil {
		return ""
	}
	a.heldSlotMu.RLock()
	slot := a.heldSlot
	a.heldSlotMu.RUnlock()
	name, _ := a.resolveHotbarSlot(slots, itemMgr, slot)
	return name
}

const (
	// equipConfirmWait is how long PlaceHeldBlock waits, after Equip returns,
	// for the item to actually be the held one.
	equipConfirmWait = 800 * time.Millisecond
	equipAttempts    = 3
)

// equipAndConfirm equips itemName and waits until it is the item in hand,
// re-trying the equip when it is not. Equip returns success on paths that
// leave a different item selected (its hotbar swap can lose a race with the
// server rejecting a stale click and resyncing the window), and a
// right-click on the ground with the wrong item places *that* - found live:
// a log was placed as a block instead of the table, silently destroying the
// episode's ingredients.
func (a *agent) equipAndConfirm(ctx context.Context, itemName, want string) error {
	var lastErr error
	for attempt := 0; attempt < equipAttempts; attempt++ {
		if err := a.Equip(ctx, itemName); err != nil {
			lastErr = fmt.Errorf("equip %s: %w", itemName, err)
		} else {
			deadline := time.Now().Add(equipConfirmWait)
			for {
				if a.heldItemName() == want {
					return nil
				}
				if !time.Now().Before(deadline) {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(placePoll):
				}
			}
			lastErr = fmt.Errorf("equip %s: it never became the held item (holding %q)", itemName, a.heldItemName())
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		time.Sleep(150 * time.Millisecond) // let a window resync settle before retrying
	}
	return lastErr
}

// waitForPlacement waits until a placement has taken effect: the block shows
// in the bot's world view, or the item has left the inventory. The second is
// the server's own confirmation and can arrive well before the block update
// does; treating the block's absence as failure repeated the placement (and
// clicked the first block), leaving two tables where the caller knew of one.
func (a *agent) waitForPlacement(ctx context.Context, cell models.V3, want, itemName string, countBefore int) bool {
	deadline := time.Now().Add(placeVerifyWait)
	for {
		if placementTookEffect(a.BlockNameAt(int(cell.X), int(cell.Y), int(cell.Z)), want, a.InventoryCount(itemName), countBefore) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(placePoll):
		}
	}
}

// placementTookEffect is waitForPlacement's test: the cell shows the placed
// block, or fewer of the item are held than before the click.
func placementTookEffect(blockAtCell, want string, countNow, countBefore int) bool {
	return blockAtCell == want || countNow < countBefore
}
