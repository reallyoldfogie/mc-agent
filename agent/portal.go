package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// portalBlockNames are the three block types that teleport a player on
// contact: a built nether portal, the end's exit/access portal, and an end
// gateway (used both by the dragon-fight exit and by far-distance end
// islands). Finding/building a portal from scratch is out of scope -
// UsePortal assumes one of these already exists in the world.
var portalBlockNames = []string{
	"minecraft:nether_portal",
	"minecraft:end_portal",
	"minecraft:end_gateway",
}

// usePortalDimensionChangeTimeout bounds how long UsePortal waits, after
// arriving at the portal block, for the dimension to actually change. A
// Nether portal requires a few ticks of continuous contact before the
// server initiates the teleport (vanilla's "portal time"); an End
// portal/gateway teleports near-instantly. This is generous enough to
// cover either, plus the Respawn/chunk-load sequence that follows -
// confirm the exact lower bound against a live capture if this ever needs
// tightening.
var usePortalDimensionChangeTimeout = 15 * time.Second

// FindNearestPortal searches for the nearest block matching any of
// portalBlockNames within maxDistance, by calling FindVisibleBlock once
// per name and keeping the closest result - a Nether portal and an End
// portal are never the same search, and there's no existing multi-name
// variant of FindVisibleBlock to share one shell-scan across all three
// names in a single pass. Exposed (not just an internal helper of
// UsePortal) so a caller that dispatches movement far more often than a
// human types a chat command (rlenv) can resolve a target once and pass
// exact coordinates to UsePortalAt, the same FindNearestBed/SleepInBed
// split uses.
func (a *agent) FindNearestPortal(ctx context.Context, maxDistance int) (models.V3, bool, error) {
	pos, _, _, posOK := a.GetPosition()
	bestDistSq := math.MaxFloat64
	var best models.V3
	var found bool

	for _, name := range portalBlockNames {
		px, py, pz, ok, ferr := a.FindVisibleBlock(ctx, name, maxDistance)
		if ferr != nil {
			return models.V3{}, false, ferr
		}
		if !ok {
			continue
		}

		// Without a known position there's nothing to compare distances
		// against - keep whichever name matched first.
		if !posOK {
			if !found {
				best, found = models.V3{X: px, Y: py, Z: pz}, true
			}
			continue
		}

		dx, dy, dz := px-pos.X, py-pos.Y, pz-pos.Z
		distSq := dx*dx + dy*dy + dz*dz
		if !found || distSq < bestDistSq {
			found = true
			bestDistSq = distSq
			best = models.V3{X: px, Y: py, Z: pz}
		}
	}
	return best, found, nil
}

// UsePortal walks to the nearest nether_portal/end_portal/end_gateway
// block within maxDistance blocks and waits for the dimension to actually
// change, rather than treating "arrived at the portal's coordinates" as
// done — see models.CommandAgent's doc comment for the full contract.
func (a *agent) UsePortal(ctx context.Context, maxDistance int) error {
	if maxDistance <= 0 {
		maxDistance = 16
	}

	pos, found, err := a.FindNearestPortal(ctx, maxDistance)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no portal block found within %d blocks", maxDistance)
	}
	return a.UsePortalAt(ctx, pos)
}

// UsePortalAt walks to the exact portal block at pos and waits for the
// dimension to actually change — the exact-coordinate counterpart to
// UsePortal's by-radius search, for a caller (rlenv) that already resolved
// a target via FindNearestPortal and wants to avoid a second,
// independently-radius'd search inside the dispatched action, the same
// FindNearestBed/SleepInBed split uses.
func (a *agent) UsePortalAt(ctx context.Context, pos models.V3) error {
	beforeTypeID, beforeName, _ := a.GetDimension()

	// Portal blocks have no collision (confirmed against mc-data-gen's
	// collision data for all three names) — walking "to" one means
	// standing inside it, not approaching an adjacent cell.
	if err := a.MoveTo(ctx, pos.X, pos.Y, pos.Z, false); err != nil {
		return fmt.Errorf("failed to walk to portal at (%.1f, %.1f, %.1f): %w", pos.X, pos.Y, pos.Z, err)
	}

	deadline := time.Now().Add(usePortalDimensionChangeTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		typeID, name, ok := a.GetDimension()
		if ok && (typeID != beforeTypeID || name != beforeName) {
			return nil
		}
	}
	return fmt.Errorf("timed out waiting for dimension change after entering portal at (%.1f, %.1f, %.1f)", pos.X, pos.Y, pos.Z)
}
