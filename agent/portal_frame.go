package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Vanilla nether portal frame bounds, in INTERIOR (the air opening a lit
// portal fills) terms: width 2-21, height 3-21 (4x5 to 23x23 including the
// obsidian border). Corners are optional in vanilla - a player-built frame
// need not have them, only the game's own generated portals always do -
// so frame detection below never checks the four corner cells.
//
// Sourced from the Minecraft Wiki and cross-checked against independent
// portal-size calculators, not verified against decompiled game source in
// this session (no local copy of PortalShape.java was available to check
// its MIN_WIDTH/MAX_WIDTH/MIN_HEIGHT/MAX_HEIGHT constants directly) -
// confirm live against a real server if a frame right at these bounds
// (minimum 2x3, or anything near the 21x21 maximum) is ever rejected or
// accepted incorrectly.
const (
	portalFrameMinWidth  = 2
	portalFrameMaxWidth  = 21
	portalFrameMinHeight = 3
	portalFrameMaxHeight = 21
)

// portalFrameSearchDefaultMaxDistance mirrors usePortalDefaultMaxDistance's
// own default (actions/commands.go) for LightPortal's own by-radius form.
const portalFrameSearchDefaultMaxDistance = 16

// isObsidianAt reports whether the block at the given integer coordinates
// is loaded and is minecraft:obsidian.
func (a *agent) isObsidianAt(x, y, z int) bool {
	world := a.GetWorld()
	if world == nil {
		return false
	}
	stateID, loaded := world.GetBlockAt(float64(x)+0.5, float64(y)+0.5, float64(z)+0.5)
	if !loaded {
		return false
	}
	return a.blockStateMatchesName(stateID, "minecraft:obsidian")
}

// isPassableAt reports whether the block at the given integer coordinates
// is loaded and passable (air or otherwise non-obstructing) - what a
// portal's interior must consist of entirely.
func (a *agent) isPassableAt(x, y, z int) bool {
	world, shapeMgr := a.GetWorld(), a.BlockShapeManager()
	if world == nil || shapeMgr == nil {
		return false
	}
	stateID, loaded := world.GetBlockAt(float64(x)+0.5, float64(y)+0.5, float64(z)+0.5)
	if !loaded {
		return false
	}
	return shapeMgr.IsPassable(stateID)
}

// tryFrameFromSeed attempts to interpret seed as a block belonging to one
// of a frame's two side columns, for one axis (dx,dz is the +1 step along
// the frame's width direction: (1,0) for axis "x", (0,1) for axis "z").
// Finds the column's true bottom (seed may be any block within it, not
// necessarily the lowest), then its height, then searches
// portalFrameMinWidth..MaxWidth for a matching right column, bottom/top
// bridge rows, and an all-passable interior. Returns ok=false if seed
// doesn't anchor a valid frame on this axis at all.
func (a *agent) tryFrameFromSeed(seed models.V3, dx, dz int, axis string) (models.PortalFrame, bool) {
	x, y, z := int(seed.X), int(seed.Y), int(seed.Z)

	// The true interior height can't be found by just counting how many
	// obsidian blocks stack up from seed: when the frame's (optional)
	// corners are present, they sit directly above/below the column and
	// are obsidian too, merging visually with it - counting would include
	// them and overshoot the real interior by up to two rows. Instead,
	// find a Y where seed's column (i=0) is obsidian AND the adjacent
	// interior cell (i=1, one step in the width direction) is passable -
	// that combination is unique to a genuine interior row (a bridge/
	// corner row has the i=1 neighbor obsidian too, not passable) - then
	// expand from there while both conditions keep holding.
	isInteriorRow := func(atY int) bool {
		return a.isObsidianAt(x, atY, z) && a.isPassableAt(x+dx, atY, z+dz)
	}
	y0, foundInteriorRow := 0, false
	for probe := y - portalFrameMaxHeight; probe <= y+portalFrameMaxHeight; probe++ {
		if isInteriorRow(probe) {
			y0, foundInteriorRow = probe, true
			break
		}
	}
	if !foundInteriorRow {
		return models.PortalFrame{}, false
	}
	yHigh := y0
	for isInteriorRow(y0 - 1) {
		y0--
	}
	for isInteriorRow(yHigh + 1) {
		yHigh++
	}
	h := yHigh - y0 + 1
	if h < portalFrameMinHeight || h > portalFrameMaxHeight {
		return models.PortalFrame{}, false
	}

	for w := portalFrameMinWidth; w <= portalFrameMaxWidth; w++ {
		rx, rz := x+dx*(w+1), z+dz*(w+1)

		// Right column must match the left column's exact height.
		rightOK := true
		for dy := 0; dy < h; dy++ {
			if !a.isObsidianAt(rx, y0+dy, rz) {
				rightOK = false
				break
			}
		}
		if !rightOK {
			continue
		}

		// Interior (excluding both columns) must be entirely passable.
		interiorOK := true
		for i := 1; i <= w && interiorOK; i++ {
			ix, iz := x+dx*i, z+dz*i
			for dy := 0; dy < h; dy++ {
				if !a.isPassableAt(ix, y0+dy, iz) {
					interiorOK = false
					break
				}
			}
		}
		if !interiorOK {
			continue
		}

		// Bottom/top bridge rows, excluding the four corner cells.
		bridgeOK := true
		for i := 1; i <= w && bridgeOK; i++ {
			ix, iz := x+dx*i, z+dz*i
			if !a.isObsidianAt(ix, y0-1, iz) || !a.isObsidianAt(ix, y0+h, iz) {
				bridgeOK = false
			}
		}
		if !bridgeOK {
			continue
		}

		// Only the width axis has two columns to exclude from the
		// interior - the other coordinate is a fixed single-block-thick
		// slab at seed's own position, not a min/max of the two columns
		// (which would be wrong here precisely because they're equal:
		// x==rx or z==rz on whichever axis isn't the width direction, so
		// a min(...)+1/max(...)-1 formula would incorrectly shift it).
		interiorMin, interiorMax := models.V3{Y: float64(y0)}, models.V3{Y: float64(y0 + h - 1)}
		if axis == "x" {
			interiorMin.X, interiorMax.X = float64(x+1), float64(x+w)
			interiorMin.Z, interiorMax.Z = float64(z), float64(z)
		} else {
			interiorMin.Z, interiorMax.Z = float64(z+1), float64(z+w)
			interiorMin.X, interiorMax.X = float64(x), float64(x)
		}
		ignitionFace := models.FaceEast
		if axis == "z" {
			ignitionFace = models.FaceSouth
		}
		return models.PortalFrame{
			Axis:          axis,
			InteriorMin:   interiorMin,
			InteriorMax:   interiorMax,
			IgnitionBlock: models.V3{X: float64(x), Y: float64(y0), Z: float64(z)},
			IgnitionFace:  ignitionFace,
		}, true
	}
	return models.PortalFrame{}, false
}

// FindPortalFrame searches for the nearest unlit nether portal frame (a
// rectangular obsidian ring with a passable interior, portalFrameMinWidth..
// MaxWidth by portalFrameMinHeight..MaxHeight) within maxDistance blocks of
// the agent's own position, by scanning for obsidian candidates and trying
// each as a possible frame side-column on both axes. found is false if
// none was located.
func (a *agent) FindPortalFrame(ctx context.Context, maxDistance int) (frame models.PortalFrame, found bool, err error) {
	pos, _, _, ok := a.GetPosition()
	if !ok {
		return models.PortalFrame{}, false, fmt.Errorf("find portal frame: position not initialized")
	}
	if maxDistance <= 0 {
		maxDistance = portalFrameSearchDefaultMaxDistance
	}

	baseX, baseY, baseZ := int(math.Floor(pos.X)), int(math.Floor(pos.Y)), int(math.Floor(pos.Z))
	bestDistSq := math.MaxFloat64

	for dx := -maxDistance; dx <= maxDistance; dx++ {
		for dy := -maxDistance; dy <= maxDistance; dy++ {
			for dz := -maxDistance; dz <= maxDistance; dz++ {
				if err := ctx.Err(); err != nil {
					return models.PortalFrame{}, false, err
				}
				x, y, z := baseX+dx, baseY+dy, baseZ+dz
				if !a.isObsidianAt(x, y, z) {
					continue
				}
				seed := models.V3{X: float64(x), Y: float64(y), Z: float64(z)}
				for _, cand := range []struct {
					dx, dz int
					axis   string
				}{{1, 0, "x"}, {0, 1, "z"}} {
					f, ok := a.tryFrameFromSeed(seed, cand.dx, cand.dz, cand.axis)
					if !ok {
						continue
					}
					ddx, ddy, ddz := float64(x)-pos.X, float64(y)-pos.Y, float64(z)-pos.Z
					distSq := ddx*ddx + ddy*ddy + ddz*ddz
					if !found || distSq < bestDistSq {
						found = true
						bestDistSq = distSq
						frame = f
					}
				}
			}
		}
	}
	return frame, found, nil
}

// portalIgnitionConfirmTimeout/PollInterval bound how long LightPortal
// waits for a lit frame's interior to actually turn into nether_portal
// blocks after the flint-and-steel click - mirrors
// usePortalDimensionChangeTimeout's own "generous but bounded" shape.
var (
	portalIgnitionConfirmTimeout  = 5 * time.Second
	portalIgnitionConfirmPollRate = 100 * time.Millisecond
)

// LightPortalFrame ignites an already-detected, unlit frame: walks to a
// reachable position with line of sight to its ignition block, equips
// flint and steel, right-clicks the ignition face, and waits for the
// interior to actually turn into nether_portal blocks - not just that the
// click was sent.
func (a *agent) LightPortalFrame(ctx context.Context, frame models.PortalFrame) error {
	const flintAndSteel = "minecraft:flint_and_steel"
	if a.InventoryCount(flintAndSteel) == 0 {
		return fmt.Errorf("light portal: no %s in inventory", flintAndSteel)
	}

	err := models.TryInteractPositions(ctx, a, frame.IgnitionBlock, func(standAt models.V3) error {
		if err := a.MoveTo(ctx, standAt.X, standAt.Y, standAt.Z, false); err != nil {
			return err
		}
		if err := a.equipAndConfirm(ctx, flintAndSteel, normalizeItemName(flintAndSteel)); err != nil {
			return err
		}
		return a.UseItemOnBlock(ctx, frame.IgnitionBlock.X, frame.IgnitionBlock.Y, frame.IgnitionBlock.Z, frame.IgnitionFace, models.MainHand)
	})
	if err != nil {
		return fmt.Errorf("light portal at %v: %w", frame.IgnitionBlock, err)
	}

	deadline := time.Now().Add(portalIgnitionConfirmTimeout)
	checkX, checkY, checkZ := frame.InteriorMin.X, frame.InteriorMin.Y, frame.InteriorMin.Z
	for time.Now().Before(deadline) {
		if a.blockNameMatchesAny(int(checkX), int(checkY), int(checkZ), portalBlockNames) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(portalIgnitionConfirmPollRate):
		}
	}
	return fmt.Errorf("light portal at %v: ignition never confirmed within %s", frame.IgnitionBlock, portalIgnitionConfirmTimeout)
}

// blockNameMatchesAny reports whether the block at the given integer
// coordinates is loaded and its name matches any of names.
func (a *agent) blockNameMatchesAny(x, y, z int, names []string) bool {
	world := a.GetWorld()
	if world == nil {
		return false
	}
	stateID, loaded := world.GetBlockAt(float64(x)+0.5, float64(y)+0.5, float64(z)+0.5)
	if !loaded {
		return false
	}
	for _, name := range names {
		if a.blockStateMatchesName(stateID, name) {
			return true
		}
	}
	return false
}

// LightPortal finds the nearest unlit nether portal frame within
// maxDistance blocks and ignites it - see models.CommandAgent's doc
// comment for the full contract.
func (a *agent) LightPortal(ctx context.Context, maxDistance int) error {
	frame, found, err := a.FindPortalFrame(ctx, maxDistance)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("light portal: no unlit portal frame found within %d blocks", maxDistance)
	}
	return a.LightPortalFrame(ctx, frame)
}
