package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

const (
	// boatPlacementDetectTimeout bounds how long PlaceAndMountBoat waits for
	// the placed item to actually produce a boat entity before giving up.
	boatPlacementDetectTimeout = 3 * time.Second
	boatPlacementPollInterval  = 50 * time.Millisecond
	// boatPlacementDetectRadiusSq is the squared search radius (2 blocks)
	// around the placement target for the newly-spawned boat entity.
	boatPlacementDetectRadiusSq = 4.0
)

// PlaceAndMountBoat finds a boat item in the hotbar, places it on the water
// at waterPos, waits for the resulting boat entity to spawn nearby, and
// mounts it. This is the on-foot -> place -> mount transition described in
// docs/plans/WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 8, built entirely
// from existing primitives (SwitchToSlot/UseItemOnBlock/MountEntity),
// modeled on handleClutchPlan's water-bucket placement flow.
//
// Like MountEntity/MountNearest, this does not wait for the mount
// interaction itself to be confirmed - that confirmation (ClientboundSetPassengers)
// is the movement executor's job, tracked the same way for every mount.
func (a *agent) PlaceAndMountBoat(ctx context.Context, waterPos models.V3) error {
	slot, itemName := a.findBoatItemSlot()
	if slot < 0 {
		return fmt.Errorf("no boat item found in hotbar")
	}

	usage, err := a.itemUsageOrCreate()
	if err != nil {
		return fmt.Errorf("place boat: %w", err)
	}

	if err := usage.SwitchToSlot(slot); err != nil {
		return fmt.Errorf("place boat: switch to slot %d (%s): %w", slot, itemName, err)
	}

	beforeIDs := a.snapshotEntityIDs()

	if err := usage.UseItemOnBlock(waterPos, models.FaceUp, models.MainHand); err != nil {
		return fmt.Errorf("place boat: UseItemOnBlock failed for %s: %w", itemName, err)
	}

	entityID, err := a.awaitPlacedBoat(ctx, waterPos, beforeIDs)
	if err != nil {
		return fmt.Errorf("place boat: %w", err)
	}

	a.logf("[PlaceAndMountBoat] Placed %s, detected entity %d, mounting", itemName, entityID)

	if err := a.MountEntity(ctx, entityID); err != nil {
		return fmt.Errorf("place boat: mount entity %d: %w", entityID, err)
	}

	return nil
}

// findBoatItemSlot scans the hotbar for any boat item. Unlike
// findClutchItemSlot's exact-name match, boat items vary by wood type (and
// the bamboo raft), so this matches by name shape instead of a fixed list.
func (a *agent) findBoatItemSlot() (int16, string) {
	a.slotsMu.RLock()
	defer a.slotsMu.RUnlock()
	a.itemMgrMu.RLock()
	defer a.itemMgrMu.RUnlock()

	if a.slots == nil || a.itemMgr == nil {
		return -1, ""
	}

	for i := mcscreen.HotbarSlotStart; i <= mcscreen.HotbarSlotEnd; i++ {
		itemID, _, ok := a.slots.ResolveSlot(-2, i)
		if !ok {
			continue
		}
		name := a.itemMgr.GetItemNameByID(itemID)
		if isBoatItemName(name) {
			return i - mcscreen.HotbarSlotStart, name
		}
	}
	return -1, ""
}

// isBoatItemName reports whether name (e.g. "minecraft:oak_boat",
// "minecraft:bamboo_chest_raft") names a boat-family item.
func isBoatItemName(name string) bool {
	return strings.HasSuffix(name, "_boat") || name == "minecraft:boat" ||
		strings.HasSuffix(name, "_raft") || name == "minecraft:bamboo_raft"
}

// snapshotEntityIDs returns the currently-tracked entity IDs, used by
// awaitPlacedBoat to distinguish a newly-placed boat from one that happened
// to already be nearby.
func (a *agent) snapshotEntityIDs() map[int32]bool {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	ids := make(map[int32]bool, len(a.entities))
	for id := range a.entities {
		ids[id] = true
	}
	return ids
}

// awaitPlacedBoat polls tracked entities for a new (not in before) boat or
// chest_boat entity within boatPlacementDetectRadiusSq of waterPos, bounded
// by boatPlacementDetectTimeout. Placement can fail in ways swimming can't
// (obstructed target, wrong face, no clear water), so this returning an
// error is an expected, real outcome the caller must handle - not something
// that "should never happen".
func (a *agent) awaitPlacedBoat(ctx context.Context, waterPos models.V3, before map[int32]bool) (int32, error) {
	deadline := time.Now().Add(boatPlacementDetectTimeout)
	for {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}

		if id, found := a.findNewNearbyBoat(waterPos, before); found {
			return id, nil
		}

		if time.Now().After(deadline) {
			return 0, fmt.Errorf("timed out waiting for placed boat entity to appear near (%.1f, %.1f, %.1f)",
				waterPos.X, waterPos.Y, waterPos.Z)
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(boatPlacementPollInterval):
		}
	}
}

func (a *agent) findNewNearbyBoat(waterPos models.V3, before map[int32]bool) (int32, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	if a.entityRegistry == nil {
		return 0, false
	}

	for id, e := range a.entities {
		if before[id] || e.Removed {
			continue
		}
		entityType := a.entityRegistry.GetEntityType(id)
		if entityType != models.EntityTypeBoat && entityType != models.EntityTypeChestBoat {
			continue
		}
		dx := e.X - waterPos.X
		dy := e.Y - waterPos.Y
		dz := e.Z - waterPos.Z
		if dx*dx+dy*dy+dz*dz <= boatPlacementDetectRadiusSq {
			return id, true
		}
	}
	return 0, false
}
