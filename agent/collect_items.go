package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

const (
	// collectItemsMax bounds how many drops one CollectNearbyItems call
	// chases, so a room full of items cannot hold a caller for long.
	collectItemsMax = 8

	// collectItemWait is how long CollectNearbyItems waits, once standing
	// in range, for one drop to be collected. The pickup delay itself is
	// only models.PickupDelayTicks server ticks (half a second at 20/s, an
	// eighth at this project's 80/s), so this is mostly slack for the
	// server's own tick and packet latency.
	collectItemWait = 1200 * time.Millisecond

	collectItemPoll = 50 * time.Millisecond

	// collectItemAppearWait is how long the first search waits for a drop
	// to show up in the bot's own entity tracking before concluding there
	// is none. A caller collecting right after a block broke can beat the
	// item's spawn packet to it (found live: 2 of 10 collects found nothing
	// and the drop was collected by the next call); waiting costs this long
	// only when there really is nothing to collect.
	collectItemAppearWait = 600 * time.Millisecond
)

// CollectNearbyItems walks to and collects the dropped items within
// maxDistance of the bot, nearest first, and returns how many were
// collected. Finding nothing to collect is not an error (0, nil); a drop it
// could not collect (unreachable, or never picked up) is skipped, not
// retried, and reported only if nothing at all could be collected.
//
// Vanilla collects any item whose box meets the player's box grown by a
// block sideways (models.ItemInPickupRange), once its pickup delay is over,
// so a drop only needs the bot near it, not on it. That matters: walking to
// the drop's exact cell fails whenever that cell is not walkable, which is
// the usual case for the first log of a tree (the logs above it roof the
// cell), and drops land up to a block and a half from the block they came
// from. This tries every standable cell within pickup range instead - see
// models.TryPickupPositions.
func (a *agent) CollectNearbyItems(ctx context.Context, maxDistance float64) (int, error) {
	skipped := map[int32]bool{}
	collected := 0
	var lastErr error
	for i := 0; i < collectItemsMax; i++ {
		id, item, ok, err := a.nearestVisibleItem(ctx, maxDistance, skipped)
		if err != nil {
			return collected, err
		}
		if !ok && i == 0 {
			id, item, ok, err = a.awaitVisibleItem(ctx, maxDistance, skipped)
			if err != nil {
				return collected, err
			}
		}
		if !ok {
			break
		}
		if err := a.collectItem(ctx, id, item); err != nil {
			lastErr = err
			skipped[id] = true
			continue
		}
		collected++
	}
	if collected == 0 && lastErr != nil {
		return 0, lastErr
	}
	return collected, nil
}

// nearestVisibleItem is FindNearestVisibleItem minus the entities in skip.
func (a *agent) nearestVisibleItem(ctx context.Context, maxDistance float64, skip map[int32]bool) (int32, models.V3, bool, error) {
	itemTypeID, ok := a.GetEntityTypeID("minecraft:item")
	if !ok {
		return 0, models.V3{}, false, fmt.Errorf("minecraft:item entity type not found in registry")
	}
	pos, _, _, ok := a.GetPosition()
	if !ok {
		return 0, models.V3{}, false, fmt.Errorf("position not initialized")
	}
	bestDist := math.MaxFloat64
	var bestID int32
	var best models.V3
	for _, ent := range a.GetTrackedEntities() {
		if ent.Removed || ent.EntityType != itemTypeID || skip[ent.EntityID] {
			continue
		}
		at := models.V3{X: ent.X, Y: ent.Y, Z: ent.Z}
		d := pos.DistanceTo(at)
		if maxDistance > 0 && d > maxDistance {
			continue
		}
		visible, err := a.HasLineOfSight(ctx, ent.X, ent.Y, ent.Z)
		if err != nil {
			return 0, models.V3{}, false, err
		}
		if !visible || d >= bestDist {
			continue
		}
		bestDist, bestID, best = d, ent.EntityID, at
	}
	return bestID, best, bestDist != math.MaxFloat64, nil
}

// awaitVisibleItem polls nearestVisibleItem for up to collectItemAppearWait.
func (a *agent) awaitVisibleItem(ctx context.Context, maxDistance float64, skip map[int32]bool) (int32, models.V3, bool, error) {
	deadline := time.Now().Add(collectItemAppearWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return 0, models.V3{}, false, ctx.Err()
		case <-time.After(collectItemPoll):
		}
		id, item, ok, err := a.nearestVisibleItem(ctx, maxDistance, skip)
		if err != nil || ok {
			return id, item, ok, err
		}
	}
	return 0, models.V3{}, false, nil
}

// itemGone reports whether the tracked item entity id no longer exists (it
// was collected, or despawned).
func (a *agent) itemGone(id int32) bool {
	for _, ent := range a.GetTrackedEntities() {
		if ent.EntityID == id {
			return ent.Removed
		}
	}
	return true
}

// collectItemAttempts is how many times collectItem re-reads the drop's
// position and tries again. A resting item's position in the bot's own
// entity tracking can be stale (a drop that slid after its last position
// packet), so a bot that believes it is in range may not be; each retry
// re-reads the position, and the later ones walk right onto the item.
const collectItemAttempts = 3

// trackedItem returns the current tracked position of item entity id, and
// false if it is gone.
func (a *agent) trackedItem(id int32) (models.V3, bool) {
	for _, ent := range a.GetTrackedEntities() {
		if ent.EntityID == id && !ent.Removed {
			return models.V3{X: ent.X, Y: ent.Y, Z: ent.Z}, true
		}
	}
	return models.V3{}, false
}

// collectItem gets the bot into pickup range of one drop and waits for it
// to be collected, re-reading the drop's position between attempts.
func (a *agent) collectItem(ctx context.Context, id int32, item models.V3) error {
	var lastErr error
	for attempt := 0; attempt < collectItemAttempts; attempt++ {
		cur, ok := a.trackedItem(id)
		if !ok {
			return nil
		}
		item = cur
		if lastErr = a.approachItem(ctx, id, item, attempt > 0); lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return lastErr
}

// approachItem gets in range of item and waits for it to be collected.
// exact walks onto the item's own position instead of to a cell beside it.
func (a *agent) approachItem(ctx context.Context, id int32, item models.V3, exact bool) error {
	if !exact {
		if pos, _, _, ok := a.GetPosition(); ok && models.ItemInPickupRange(pos, item) {
			return a.waitItemGone(ctx, id)
		}
		err := models.TryPickupPositions(ctx, a, item, func(p models.V3) error {
			if err := a.MoveTo(ctx, p.X, p.Y, p.Z, false); err != nil {
				return err
			}
			return a.waitItemGone(ctx, id)
		})
		if err != models.ErrNoPickupPosition {
			return err
		}
		// Nothing standable beside it (a drop in a hole, say): fall through
		// to walking onto the item itself.
	}
	if err := a.MoveTo(ctx, item.X, item.Y, item.Z, false); err != nil && !a.itemGone(id) {
		return err
	}
	return a.waitItemGone(ctx, id)
}

// waitItemGone polls until the item entity id is gone, up to collectItemWait.
func (a *agent) waitItemGone(ctx context.Context, id int32) error {
	deadline := time.Now().Add(collectItemWait)
	for {
		if a.itemGone(id) {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("item %d was not collected within %s of getting in range", id, collectItemWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(collectItemPoll):
		}
	}
}
