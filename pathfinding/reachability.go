package pathfinding

import (
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// goalUnreachable reports whether goal is *definitely* unreachable: no
// walkable cell (passable feet+head, solid ground support) exists anywhere
// within goalRadius of it. Cells in an unloaded chunk are treated as
// "unknown" rather than unwalkable, so this never produces a false
// negative that blocks a legitimately pending world - it only fires when
// every candidate cell's data is available and none of them qualify.
//
// Shared by all A*-family pathfinders (plain A*, EPEA*, bidirectional A*)
// so a search never burns its entire step budget on a goal that is
// guaranteed to fail regardless of maxSteps or context deadline - e.g. a
// caller passing a solid block's own coordinates as the target. See
// docs/bugs/hpa-star-slowness.
func goalUnreachable(world models.World, shapeMgr models.BlockShapeManager, goalRadius float64, goal models.V3) bool {
	radius := int(math.Ceil(goalRadius))
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			for dz := -radius; dz <= radius; dz++ {
				candidate := models.V3{X: goal.X + float64(dx), Y: goal.Y + float64(dy), Z: goal.Z + float64(dz)}
				if candidate.DistanceTo(goal) > goalRadius {
					continue
				}
				feetID, feetLoaded := world.GetBlockAt(candidate.X, candidate.Y, candidate.Z)
				headID, headLoaded := world.GetBlockAt(candidate.X, candidate.Y+1, candidate.Z)
				groundID, groundLoaded := world.GetBlockAt(candidate.X, candidate.Y-1, candidate.Z)
				if !feetLoaded || !headLoaded || !groundLoaded {
					return false // unknown - don't block, let the real search decide
				}
				if shapeMgr.IsPassable(feetID) && shapeMgr.IsPassable(headID) && !shapeMgr.IsPassable(groundID) {
					return false // found a walkable cell within goalRadius
				}
			}
		}
	}
	return true
}
