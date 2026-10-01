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
		if err := ctx.Err(); err != nil {
			return models.V3{}, err
		}
		if err := a.placeAgainst(ctx, itemName, want, c.Floor, models.FaceUp, c.Cell); err != nil {
			lastErr = err
			continue
		}
		return c.Cell, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("place %s: no free cell with a floor beside the bot (standing in cell %d,%d,%d at y=%.4f; %d candidate cells unusable)", itemName, bx, by, bz, pos.Y, unusable)
	}
	return models.V3{}, lastErr
}

// placeAgainst attempts, up to placeAttemptsPerCell times, to place itemName
// into targetCell by right-clicking support's face face - the shared retry/
// verify core of both PlaceHeldBlock (clicking a floor beside the bot) and
// PlaceBlockAt (clicking whatever solid neighbor an arbitrary target cell
// has). want is itemName already run through normalizeItemName, passed in
// rather than recomputed so both callers normalize exactly once. Returns
// nil once waitForPlacement confirms the block appeared, or the last error
// encountered across all attempts.
func (a *agent) placeAgainst(ctx context.Context, itemName, want string, support models.V3, face models.BlockFace, targetCell models.V3) error {
	var lastErr error
	for attempt := 0; attempt < placeAttemptsPerCell; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := a.equipAndConfirm(ctx, itemName, want); err != nil {
			lastErr = err
			continue
		}
		before := a.InventoryCount(itemName)
		if err := a.UseItemOnBlock(ctx, support.X, support.Y, support.Z, face, models.MainHand); err != nil {
			lastErr = fmt.Errorf("use %s on (%v): %w", itemName, support, err)
			continue
		}
		if a.waitForPlacement(ctx, targetCell, want, itemName, before) {
			return nil
		}
		lastErr = fmt.Errorf("place %s at (%v): the block never appeared", itemName, targetCell)
	}
	return lastErr
}

// supportFaceOrder is the preference order PlaceBlockAt's findSupportFace
// checks target's six neighbors in: down first (how a player naturally
// builds - floor before walls), then the four horizontal neighbors, then
// up last (placing against a ceiling is the least natural choice and the
// worst for reach from a standing position).
var supportFaceOrder = []struct {
	dx, dy, dz int
	face       models.BlockFace
}{
	{0, -1, 0, models.FaceUp},    // support below target -> click its top face
	{0, 0, -1, models.FaceSouth}, // support north of target -> click its south face
	{0, 0, 1, models.FaceNorth},  // support south of target -> click its north face
	{1, 0, 0, models.FaceWest},   // support east of target -> click its west face
	{-1, 0, 0, models.FaceEast},  // support west of target -> click its east face
	{0, 1, 0, models.FaceDown},   // support above target -> click its bottom face
}

// blockReader is the minimal world-query surface findSupportFace needs -
// narrower than models.World (14 methods, most of them world-time/border/
// light queries irrelevant here) so it stays trivially fakeable in tests.
// Same reasoning as models.InteractPositionAgent's doc comment. A
// models.World value satisfies this automatically (Go interfaces are
// structural), so real callers (PlaceBlockAt, passing a.GetWorld()) need no
// adapter.
type blockReader interface {
	GetBlockAt(x, y, z float64) (stateID uint32, chunkLoaded bool)
}

// passabilityChecker is the single-method slice of models.BlockShapeManager
// findSupportFace needs - same narrowing rationale as blockReader.
type passabilityChecker interface {
	IsPassable(blockStateID uint32) bool
}

// findSupportFace looks at target's six neighbors, in supportFaceOrder, for
// the first solid one to place against, returning that neighbor's position
// and the face of it (facing target) to click. ok is false if every
// neighbor is passable - target is unsupported from every side (e.g. a
// floating/overhang cell a bottom-up build order hasn't reached support
// for yet), and PlaceBlockAt should fail that cell rather than guess.
func findSupportFace(world blockReader, shapeMgr passabilityChecker, target models.V3) (support models.V3, face models.BlockFace, ok bool) {
	tx, ty, tz := math.Floor(target.X), math.Floor(target.Y), math.Floor(target.Z)
	for _, o := range supportFaceOrder {
		nx, ny, nz := tx+float64(o.dx), ty+float64(o.dy), tz+float64(o.dz)
		id, loaded := world.GetBlockAt(nx+0.5, ny+0.5, nz+0.5)
		if !loaded || shapeMgr.IsPassable(id) {
			continue
		}
		return models.V3{X: nx, Y: ny, Z: nz}, o.face, true
	}
	return models.V3{}, 0, false
}

// PlaceBlockAt implements models.CommandAgent. See that interface's doc
// comment for the contract; this picks a support face via findSupportFace,
// then reuses models.TryInteractPositions (built for walking up to an
// existing solid block, but equally correct here - CanInteractFromPosition's
// underlying line-of-sight check already special-cases an air/empty target,
// see hasLineOfSightForAccessFrom in actions.go) to find a reachable
// standing position and place from there.
func (a *agent) PlaceBlockAt(ctx context.Context, target models.V3, itemName string) error {
	if a.InventoryCount(itemName) == 0 {
		return fmt.Errorf("place %s at (%v): none in the inventory", itemName, target)
	}
	world, shapeMgr := a.GetWorld(), a.BlockShapeManager()
	if world == nil || shapeMgr == nil {
		return fmt.Errorf("place %s at (%v): world not available", itemName, target)
	}

	support, face, ok := findSupportFace(world, shapeMgr, target)
	if !ok {
		return fmt.Errorf("place %s at (%v): no solid neighbor to place against", itemName, target)
	}

	want := normalizeItemName(itemName)
	err := models.TryInteractPositions(ctx, a, target, func(standAt models.V3) error {
		if err := a.MoveTo(ctx, standAt.X, standAt.Y, standAt.Z, false); err != nil {
			return err
		}
		return a.placeAgainst(ctx, itemName, want, support, face, target)
	})
	if err != nil {
		return fmt.Errorf("place %s at (%v): %w", itemName, target, err)
	}
	return nil
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
