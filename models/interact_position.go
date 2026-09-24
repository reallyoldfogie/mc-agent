package models

import (
	"context"
	"sort"
)

// InteractReachDistance is the maximum distance (in blocks) a candidate
// standing position may be from a target block and still be considered by
// FindInteractPosition. Chosen to comfortably cover vanilla's real
// interaction reach (~4.5 blocks) with a small margin; blockInteractionSetup
// (agent package) is still the final authority on whether an interaction
// actually succeeds once the agent is standing there.
const InteractReachDistance = 4.0

// InteractPositionAgent is the minimal capability FindInteractPosition
// needs. Narrower than the full Agent interface it's normally called with,
// so it stays easy to test and doesn't force other callers into an
// unnecessarily wide dependency - Agent satisfies it structurally, no
// adapter needed.
type InteractPositionAgent interface {
	GetWorld() World
	BlockShapeManager() BlockShapeManager
	CanInteractFromPosition(ctx context.Context, fromX, fromY, fromZ, targetX, targetY, targetZ float64) (bool, error)
}

// FindInteractPosition finds the best walkable position from which an
// agent could interact with the solid block at target: passable feet and
// head, solid ground support, and genuine line-of-sight to the block - so a
// position that's merely close by straight-line distance but on the wrong
// side of a wall from the target doesn't qualify. Candidates within
// InteractReachDistance are tried closest-to-target first (it's better to
// stand right next to the target than to find a valid spot several blocks
// away), and the first to pass both checks wins. Returns ok=false if
// nothing qualifies (e.g. the block is fully enclosed).
//
// This exists because walking directly to a solid block's own coordinates
// can never succeed: no walkable position is ever within a pathfinder's
// goal radius of a cell you can't stand inside - see
// docs/bugs/hpa-star-slowness for the investigation that found this the
// hard way (a MoveTo call site doing exactly that turned into an A* search
// that ran to its step limit every time, because the goal was
// unsatisfiable by construction).
func FindInteractPosition(ctx context.Context, agent InteractPositionAgent, target V3) (V3, bool, error) {
	positions, err := FindInteractPositions(ctx, agent, target, 1)
	if err != nil || len(positions) == 0 {
		return V3{}, false, err
	}
	return positions[0], true, nil
}

// FindInteractPositions is FindInteractPosition returning up to limit
// qualifying positions, best (closest) first. Qualifying only means
// walkable and in line of sight - not that a path there exists (e.g. the
// spot on top of a block floating two cells above the ground is the closest
// candidate, yet unreachable), so a caller about to walk there should be
// ready to fall back to the next entry when pathfinding fails.
func FindInteractPositions(ctx context.Context, agent InteractPositionAgent, target V3, limit int) ([]V3, error) {
	world := agent.GetWorld()
	shapeMgr := agent.BlockShapeManager()
	if world == nil || shapeMgr == nil || limit <= 0 {
		return nil, nil
	}

	type candidate struct {
		pos  V3
		dist float64
	}
	r := int(InteractReachDistance)
	candidates := make([]candidate, 0, (2*r+1)*(2*r+1)*(2*r+1))
	for dx := -r; dx <= r; dx++ {
		for dy := -r; dy <= r; dy++ {
			for dz := -r; dz <= r; dz++ {
				if dx == 0 && dy == 0 && dz == 0 {
					continue // can't stand inside the target itself
				}
				pos := V3{X: target.X + float64(dx), Y: target.Y + float64(dy), Z: target.Z + float64(dz)}
				dist := pos.DistanceTo(target)
				if dist > InteractReachDistance {
					continue
				}
				candidates = append(candidates, candidate{pos: pos, dist: dist})
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].dist < candidates[j].dist })

	var found []V3
	for _, c := range candidates {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !IsWalkablePosition(world, shapeMgr, c.pos) {
			continue
		}
		visible, err := agent.CanInteractFromPosition(ctx, c.pos.X, c.pos.Y, c.pos.Z, target.X, target.Y, target.Z)
		if err != nil || !visible {
			continue
		}
		found = append(found, c.pos)
		if len(found) == limit {
			break
		}
	}
	return found, nil
}

// IsWalkablePosition reports whether a bot could stand at pos: passable
// feet and head cells, with solid ground support below. Exported (not just
// FindInteractPosition's own internal helper) because rlenv's goto task
// needs the identical check for a different reason — see rlenv/walkability.go's
// own doc comment for why "can a bot physically stand at this candidate
// target" turned out to matter there too, not just for interaction
// candidates.
func IsWalkablePosition(world World, shapeMgr BlockShapeManager, pos V3) bool {
	feetID, feetLoaded := world.GetBlockAt(pos.X, pos.Y, pos.Z)
	headID, headLoaded := world.GetBlockAt(pos.X, pos.Y+1, pos.Z)
	groundID, groundLoaded := world.GetBlockAt(pos.X, pos.Y-1, pos.Z)
	if !feetLoaded || !headLoaded || !groundLoaded {
		return false
	}
	if !shapeMgr.IsPassable(feetID) || !shapeMgr.IsPassable(headID) {
		return false
	}
	return !shapeMgr.IsPassable(groundID)
}
