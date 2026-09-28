package models

import (
	"context"
	"errors"
	"math"
	"sort"
)

// Vanilla item-pickup geometry, from the 1.21.5 sources
// (net/minecraft/entity/player/PlayerEntity.java's tickMovement and
// entity/ItemEntity.java): a player collects an item entity when the
// player's bounding box, expanded by 1.0 horizontally and 0.5 vertically,
// intersects the item's own box - and only once the item's pickup delay has
// run out. Block drops (Block.dropStack) get the default delay of
// PickupDelayTicks, so a freshly broken block's drop cannot be collected for
// half a second of server time (an eighth of a second at 80 ticks/second).
const (
	// PickupDelayTicks is how many server ticks a newly dropped block item
	// cannot be collected (ItemEntity.setToDefaultPickupDelay).
	PickupDelayTicks = 10

	pickupExpandXZ = 1.0
	pickupExpandY  = 0.5
	playerHalfW    = 0.3
	playerHeight   = 1.8
	itemHalfW      = 0.125
	itemHeight     = 0.25

	// pickupCandidateReach is how far (per axis, cell centre to item) a
	// standing cell may be from a drop and still be offered as a place to
	// collect it from. Well inside the true limit (1.0 + 0.3 + 0.125 = 1.425)
	// so a walk that lands a few tenths of a block short still collects.
	pickupCandidateReach = 1.0
)

// ItemInPickupRange reports whether a player standing at player (feet
// position) would collect an item at item (its position, the bottom of its
// box), delay aside: the exact box test vanilla runs every tick.
func ItemInPickupRange(player, item V3) bool {
	reachXZ := playerHalfW + pickupExpandXZ + itemHalfW
	if math.Abs(item.X-player.X) >= reachXZ || math.Abs(item.Z-player.Z) >= reachXZ {
		return false
	}
	// Player box: [feet-0.5, feet+1.8+0.5]; item box: [y, y+0.25].
	return item.Y+itemHeight > player.Y-pickupExpandY && item.Y < player.Y+playerHeight+pickupExpandY
}

// ErrNoPickupPosition is returned by TryPickupPositions when no walkable
// cell within reach of the item exists.
var ErrNoPickupPosition = errors.New("no walkable position within pickup range of the item")

// PickupPositionAgent is the minimal capability TryPickupPositions needs.
type PickupPositionAgent interface {
	GetWorld() World
	BlockShapeManager() BlockShapeManager
}

// pickupCandidates returns the standing positions (cell centre, feet level)
// around item that are within pickup range, nearest horizontally first,
// then preferring the same level. Walkability is not checked here.
func pickupCandidates(item V3) []V3 {
	baseX := math.Floor(item.X)
	baseY := math.Floor(item.Y + 1e-6)
	baseZ := math.Floor(item.Z)
	type cand struct {
		pos  V3
		dxz  float64
		dLvl float64
	}
	var cands []cand
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			for dy := -1; dy <= 1; dy++ {
				pos := V3{X: baseX + float64(dx) + 0.5, Y: baseY + float64(dy), Z: baseZ + float64(dz) + 0.5}
				if math.Abs(pos.X-item.X) > pickupCandidateReach || math.Abs(pos.Z-item.Z) > pickupCandidateReach {
					continue
				}
				if !ItemInPickupRange(pos, item) {
					continue
				}
				cands = append(cands, cand{pos: pos, dxz: math.Hypot(pos.X-item.X, pos.Z-item.Z), dLvl: math.Abs(float64(dy))})
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].dxz != cands[j].dxz {
			return cands[i].dxz < cands[j].dxz
		}
		return cands[i].dLvl < cands[j].dLvl
	})
	out := make([]V3, len(cands))
	for i, c := range cands {
		out[i] = c.pos
	}
	return out
}

// TryPickupPositions offers every walkable standing position within pickup
// range of item, nearest first, to try until one returns nil - the same
// "try every candidate, fail only when all have" rule as TryInteractPositions,
// applied to collecting a drop. Walking to the drop's own cell fails exactly
// when it matters (a log still standing above a broken one roofs the cell the
// item lies in; a drop can also land in a one-block hole), whereas some cell
// beside it is nearly always reachable and is just as good: vanilla collects
// from up to a block away. Returns ErrNoPickupPosition if nothing qualifies,
// the last error from try if every candidate failed, or ctx's error.
func TryPickupPositions(ctx context.Context, agent PickupPositionAgent, item V3, try func(pos V3) error) error {
	world := agent.GetWorld()
	shapeMgr := agent.BlockShapeManager()
	if world == nil || shapeMgr == nil {
		return ErrNoPickupPosition
	}
	var lastErr error
	for _, pos := range pickupCandidates(item) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !IsWalkablePosition(world, shapeMgr, pos) {
			continue
		}
		if lastErr = try(pos); lastErr == nil {
			return nil
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if lastErr != nil {
		return lastErr
	}
	return ErrNoPickupPosition
}

// ItemCollector is the optional capability of collecting nearby dropped
// items by getting within pickup range of them (see
// agent.CollectNearbyItems). Callers type-assert for it and fall back to
// walking to the item's own position when an agent lacks it.
type ItemCollector interface {
	// CollectNearbyItems collects the visible dropped items within
	// maxDistance, nearest first, returning how many it collected. Finding
	// none is (0, nil).
	CollectNearbyItems(ctx context.Context, maxDistance float64) (int, error)
}
