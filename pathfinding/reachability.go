package pathfinding

import (
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// snapGoalToReachableGrid returns goal rounded onto the same discrete
// step-grid start actually occupies: start plus the nearest whole number
// of unit steps in each axis toward goal. Every move GetPossibleMoves
// generates only ever adds a small integer delta to a position (see
// movement.go's cardinalDirs/diagonalDirs and vertical step offsets), so
// every node a search can actually reach lies on exactly this grid - a
// goal that doesn't is one no generated node's position can ever equal,
// no matter how close start.DistanceTo(goal)/current.pos.DistanceTo(goal)
// gets, and no matter how large maxSteps or the context deadline is.
//
// Deliberately computed relative to start rather than snapped to an
// assumed-absolute .5/integer world grid: start's own position (a live
// bot's actual reported coordinates, or - for this package's own
// reachability sanity-check callers, see rlenv.Config.Jitter - a
// continuous, possibly non-.5-aligned RCON teleport target) isn't
// guaranteed to already sit exactly on that grid either. Rounding the
// raw displacement (goal-start) to the nearest integer per axis and
// adding it back onto start guarantees the result is reachable via
// *some* sequence of unit steps from wherever start actually is, without
// assuming anything about start's own absolute alignment.
//
// Live-confirmed root cause this closes: goal (264.2, -60, 3.9) queried
// from start (249.5, -60, 5.5) - a real coordinate pair from
// mc-rsi-trainer's own Config.Jitter output - has its nearest actually-
// reachable node at (264.5, -60, 3.5), whose real Euclidean distance to
// the raw goal is 0.5000000000000068: a hair over the default 0.5
// goalRadius purely because the raw goal was never on the reachable grid
// to begin with, not because that node is genuinely far away. Every
// FindPath implementation in this package calls this once, on entry,
// before any other goal-radius logic (the early start.DistanceTo(goal)
// check, goalUnreachable, and the main search loop's own termination
// check) so all of them operate on a goal that's actually achievable,
// restoring goalRadius to its intended meaning - genuine slack around an
// achievable point, not an accidental gap in grid coverage. Geometrically
// unavoidable otherwise: circles of radius 0.5 around grid points spaced
// 1.0 apart only cover ~78.5% of the plane, so roughly a fifth of all
// possible continuous goals have no reachable node within the default
// radius at all, independent of any floating-point rounding.
func snapGoalToReachableGrid(start, goal models.V3) models.V3 {
	return models.V3{
		X: start.X + math.Round(goal.X-start.X),
		Y: start.Y + math.Round(goal.Y-start.Y),
		Z: start.Z + math.Round(goal.Z-start.Z),
	}
}

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
