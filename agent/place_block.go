package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

const (
	// placeVerifyWait is how long PlaceHeldBlock waits, after asking the
	// server to place a block, for it to show up in the bot's own world
	// view before calling that attempt a failure.
	placeVerifyWait = 1500 * time.Millisecond

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
	bx, by, bz := int(math.Floor(pos.X)), int(math.Floor(pos.Y)), int(math.Floor(pos.Z))

	want := normalizeItemName(itemName)
	var lastErr error
	tried := 0
	for _, c := range models.PlacementCandidates(bx, by, bz) {
		if tried >= placeMaxCells {
			break
		}
		if !placementCellUsable(world, shapeMgr, c) {
			continue
		}
		tried++
		for attempt := 0; attempt < placeAttemptsPerCell; attempt++ {
			if err := ctx.Err(); err != nil {
				return models.V3{}, err
			}
			if err := a.Equip(ctx, itemName); err != nil {
				lastErr = fmt.Errorf("equip %s: %w", itemName, err)
				continue
			}
			if err := a.UseItemOnBlock(ctx, c.Floor.X, c.Floor.Y, c.Floor.Z, models.FaceUp, models.MainHand); err != nil {
				lastErr = fmt.Errorf("use %s on (%v): %w", itemName, c.Floor, err)
				continue
			}
			if a.waitForBlock(ctx, int(c.Cell.X), int(c.Cell.Y), int(c.Cell.Z), want) {
				return c.Cell, nil
			}
			lastErr = fmt.Errorf("place %s at (%v): the block never appeared", itemName, c.Cell)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("place %s: no free cell with a floor beside the bot", itemName)
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
