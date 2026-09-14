package rlenv

import (
	"context"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// WalkabilityAgent is the minimal capability Environment.Reset can use, if
// the live agent has it, to verify the goto task's computed target is
// actually somewhere a bot could stand before committing to it — see
// findWalkableTarget's own doc comment for the bug this closes.
// Deliberately not folded into LiveAgent, same reasoning as
// ResetAgent/SeedAgent (reset.go, seed.go): not every LiveAgent
// implementation necessarily has world/shape access wired up (e.g. a
// minimal test fake), so this stays an optional, type-asserted capability
// rather than a hard requirement — Reset simply skips the check if the
// agent doesn't satisfy it, matching pre-existing behavior exactly.
type WalkabilityAgent interface {
	GetWorld() models.World
	BlockShapeManager() models.BlockShapeManager
}

// verticalSearchRadius bounds how far above/below a candidate target
// findWalkableTarget will scan for a walkable Y before giving up on that
// (X, Z) column and falling back to horizontalSearchRadius's ring search
// instead.
//
// Small on purpose, not "generous enough for real hills" as an earlier
// version of this comment claimed: a cell can be perfectly walkable
// (solid ground below, passable above) while still being physically
// unreachable by ground pathfinding from the origin — standing there and
// walking there are different questions, and this function only checks
// the first one. Confirmed live
// (../mc-rsi-trainer/docs/plans/08-parallel-environments-and-scaling.md's
// own "Status"): with this constant originally set to 16, a Reset once
// ground-snapped onto a technically-walkable ledge 10 blocks straight up
// from the bot's position — standable, but not climbable by a bot with no
// stairs/ladder — and the pathfinder then burned 8-14+ minutes per
// episode searching thousands of A* nodes before giving up, far worse
// than the silent-but-fast failure this whole mechanism exists to
// replace. 4 keeps ground-snapping to roughly what a bot can actually
// climb in the course of a short walk (a gentle slope or a couple of
// steps), trading some hills it won't fully correct for on its own
// (falls through to horizontalSearchRadius's ring search instead, which
// found ordinary nearby ground quickly in the same live run) against not
// ever proposing a target that looks reachable but isn't. See
// findWalkableTarget's own doc comment for why a real pathfinder-based
// reachability check (verify a path actually exists, not just that the
// endpoint is standable) would be the more correct fix and was
// deliberately not taken here — this smaller radius was chosen instead as
// the safer, smaller-scoped fix for now.
const verticalSearchRadius = 4

// horizontalSearchRadius bounds findWalkableTarget's fallback ring search
// in X/Z once the exact target column has no walkable Y anywhere within
// verticalSearchRadius (e.g. the offset landed inside a cliff face) —
// small on purpose: this is a "the exact spot is bad, is next door any
// better" repair, not a general "find me anywhere reachable" search (that
// job belongs to Config.Jitter, which is about RL exploration diversity,
// not physical-world validity).
const horizontalSearchRadius = 4

// reachabilityAgent is the minimal capability findWalkableTarget needs to
// verify a standability-passing candidate is actually reachable by real
// pathfinding, not just locally standable — see findWalkableTarget's own
// doc comment for why standability alone wasn't enough. Every real
// LiveAgent already has this (models.CommandAgent embeds
// models.MovementAgent's FindPath), so Environment.Reset passes its own
// agent directly rather than needing a separate type assertion the way
// WalkabilityAgent needs one — FindPath is a required LiveAgent method,
// not an optional capability.
type reachabilityAgent interface {
	FindPath(ctx context.Context, x, y, z float64) (*models.Path, error)
}

// reachabilityCheckTimeout bounds how long findWalkableTarget will let a
// single FindPath call run before giving up on that candidate. Necessary,
// not just a nicety: agent.FindPath's own default search budget
// (maxSteps, derived from straight-line distance) is generous by design
// for real movement, and confirmed live to take 8-14+ minutes searching
// thousands of A* nodes for a genuinely disconnected target
// (../mc-rsi-trainer/docs/plans/08-parallel-environments-and-scaling.md's
// own "Status") — completely unacceptable for a per-Reset sanity check.
// FindPath's underlying A* search does respect context cancellation
// mid-search (confirmed by the same live run's own "[A*] context deadline
// exceeded" log lines once the outer test context was cancelled), so
// wrapping the call in a short timeout reliably bounds it. Short on
// purpose: candidates this function considers are already within
// verticalSearchRadius/horizontalSearchRadius of the original target, so
// a genuinely reachable one should resolve quickly; anything slower is
// treated as unreachable (see reachable's own doc comment for why that's
// a deliberate, accepted tradeoff, not an oversight).
const reachabilityCheckTimeout = 3 * time.Second

// reachable reports whether agent's pathfinder can find a route to
// (x, y, z) within reachabilityCheckTimeout. A timeout is treated
// identically to "no path found" — deliberately conservative: a
// candidate that can't be confirmed reachable quickly is rejected in
// favor of a nearer/simpler one (or a real Reset error if none exists),
// rather than risking Step later inheriting the same multi-minute search
// reachabilityCheckTimeout exists to avoid.
func reachable(ctx context.Context, agent reachabilityAgent, x, y, z float64) bool {
	checkCtx, cancel := context.WithTimeout(ctx, reachabilityCheckTimeout)
	defer cancel()
	_, err := agent.FindPath(checkCtx, x, y, z)
	return err == nil
}

// findWalkableTarget adjusts a goto task's computed (x, y, z) target so it
// actually lands somewhere a bot can both stand and actually walk to,
// using walkAgent's world/shape access and reachAgent's pathfinder to
// check both. Environment.Reset computes a goto target as a fixed offset
// from wherever the bot currently is (Config.TargetOffset's own doc
// comment); the offset has no idea what terrain is at the far end, so on
// real (non-flat, randomly generated) worlds it can land inside a hill,
// over a drop, or otherwise somewhere the pathfinder can never reach —
// confirmed live
// (../mc-rsi-trainer/docs/plans/08-parallel-environments-and-scaling.md's
// own "Status"): every Step then fails identically via pathfinding's own
// "no walkable cell"/"no valid path exists" error, contributing pure
// time-penalty reward with zero real learning signal for the whole
// episode, while Environment.Step itself returns no error at all — a
// silent failure mode, not a loud one.
//
// Two passes, cheapest first, each gated by both checks:
//  1. Ground-snap: scan Y outward from the target's own Y (0, -1, +1, -2,
//     +2, ...) for the closest standable cell directly above/below the
//     unadjusted (x, z), then confirm it with reachable — fixes the
//     common case, a slope or step under a Y-less offset.
//  2. If nothing in verticalSearchRadius at that exact column passes both
//     checks, widen to a small ring of nearby (x, z) columns (closest
//     first), ground-snapping and reachability-checking each the same way
//     — fixes the rarer case, the offset landing inside a wall, over a
//     genuine drop, or (the case that originally motivated adding
//     reachable at all) on a standable cell a real path just doesn't
//     connect to the origin.
//
// ok is false only if nothing walkable *and* reachable turned up within
// both bounds — Reset treats that as a real error (see its own call site)
// rather than silently posing an unreachable target, since that's exactly
// the failure this function exists to prevent.
//
// Standability alone (an earlier version of this function) was not
// sufficient on its own: confirmed live that a standable-but-disconnected
// candidate (blocked by water, a wall, or a gap a 3-cell local check can't
// see) reproduces the exact same zero-progress episode the walkability
// check was built to prevent, just via a different pathfinder error
// ("no valid path exists" instead of "no walkable cell") — see the plan
// doc referenced above for that run's data. reachable closes that gap
// directly rather than trying to infer it from more local-only checks.
func findWalkableTarget(ctx context.Context, walkAgent WalkabilityAgent, reachAgent reachabilityAgent, x, y, z float64) (wx, wy, wz float64, ok bool) {
	world := walkAgent.GetWorld()
	shapeMgr := walkAgent.BlockShapeManager()
	if world == nil || shapeMgr == nil {
		return 0, 0, 0, false
	}

	if snapped, ok := groundSnap(world, shapeMgr, x, y, z); ok && reachable(ctx, reachAgent, x, snapped, z) {
		return x, snapped, z, true
	}

	for _, off := range ringOffsets(horizontalSearchRadius) {
		cx, cz := x+float64(off[0]), z+float64(off[1])
		if snapped, ok := groundSnap(world, shapeMgr, cx, y, cz); ok && reachable(ctx, reachAgent, cx, snapped, cz) {
			return cx, snapped, cz, true
		}
	}
	return 0, 0, 0, false
}

// groundSnap scans Y outward from y (0, -1, +1, -2, +2, ...) up to
// verticalSearchRadius, returning the first Y at (x, z) where
// models.IsWalkablePosition holds.
func groundSnap(world models.World, shapeMgr models.BlockShapeManager, x, y, z float64) (float64, bool) {
	if models.IsWalkablePosition(world, shapeMgr, models.V3{X: x, Y: y, Z: z}) {
		return y, true
	}
	for d := 1; d <= verticalSearchRadius; d++ {
		for _, dy := range [2]float64{-float64(d), float64(d)} {
			if models.IsWalkablePosition(world, shapeMgr, models.V3{X: x, Y: y + dy, Z: z}) {
				return y + dy, true
			}
		}
	}
	return 0, false
}

// ringOffsets returns integer (dx, dz) offsets within radius r, ordered
// closest-ring-first (radius 1's 8 neighbors, then radius 2's 16, ...),
// excluding (0, 0) — findWalkableTarget's own exact-column ground-snap
// already covers that case.
func ringOffsets(r int) [][2]int {
	offsets := make([][2]int, 0, (2*r+1)*(2*r+1)-1)
	for radius := 1; radius <= r; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				if max(abs(dx), abs(dz)) != radius {
					continue // already emitted at a smaller radius
				}
				offsets = append(offsets, [2]int{dx, dz})
			}
		}
	}
	return offsets
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
