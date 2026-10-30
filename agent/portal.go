package agent

import (
	"context"
	"fmt"
	"math"
	"time"
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

// findNearestPortalBlock searches for the nearest block matching any of
// portalBlockNames within maxDistance, by calling FindVisibleBlock once
// per name and keeping the closest result - a Nether portal and an End
// portal are never the same search, and there's no existing
// multi-name variant of FindVisibleBlock to share one shell-scan across
// all three names in a single pass.
func (a *agent) findNearestPortalBlock(ctx context.Context, maxDistance int) (x, y, z float64, found bool, err error) {
	pos, _, _, posOK := a.GetPosition()
	bestDistSq := math.MaxFloat64

	for _, name := range portalBlockNames {
		px, py, pz, ok, ferr := a.FindVisibleBlock(ctx, name, maxDistance)
		if ferr != nil {
			return 0, 0, 0, false, ferr
		}
		if !ok {
			continue
		}

		// Without a known position there's nothing to compare distances
		// against - keep whichever name matched first.
		if !posOK {
			if !found {
				x, y, z, found = px, py, pz, true
			}
			continue
		}

		dx, dy, dz := px-pos.X, py-pos.Y, pz-pos.Z
		distSq := dx*dx + dy*dy + dz*dz
		if !found || distSq < bestDistSq {
			found = true
			bestDistSq = distSq
			x, y, z = px, py, pz
		}
	}
	return x, y, z, found, nil
}

// UsePortal walks to the nearest nether_portal/end_portal/end_gateway
// block within maxDistance blocks and waits for the dimension to actually
// change, rather than treating "arrived at the portal's coordinates" as
// done — see models.CommandAgent's doc comment for the full contract.
func (a *agent) UsePortal(ctx context.Context, maxDistance int) error {
	if maxDistance <= 0 {
		maxDistance = 16
	}

	beforeTypeID, beforeName, _ := a.GetDimension()

	x, y, z, found, err := a.findNearestPortalBlock(ctx, maxDistance)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no portal block found within %d blocks", maxDistance)
	}

	// Portal blocks have no collision (confirmed against mc-data-gen's
	// collision data for all three names) — walking "to" one means
	// standing inside it, not approaching an adjacent cell.
	if err := a.MoveTo(ctx, x, y, z, false); err != nil {
		return fmt.Errorf("failed to walk to portal at (%.1f, %.1f, %.1f): %w", x, y, z, err)
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
	return fmt.Errorf("timed out waiting for dimension change after entering portal at (%.1f, %.1f, %.1f)", x, y, z)
}
