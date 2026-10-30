package pathfinding

import (
	"fmt"
	"log/slog"
	"math"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// MovePruneConfig controls distance-based pruning for move generation.
//
// Start and StartDist together bound total detour from the search's own
// start-goal line (see GetPossibleMoves' own pruning for the mechanism
// and the live-confirmed bug this closes): Start is the search's fixed
// starting position, StartDist is start.DistanceTo(goal) computed once
// at search setup. Both are zero-value-safe to omit (a caller that only
// wants the older, purely local per-step check can leave them unset -
// GetPossibleMoves skips the cumulative check whenever StartDist <= 0).
type MovePruneConfig struct {
	Start     models.V3
	StartDist float64
	DriftCap  float64

	// Goal is only needed by callers (bidirAStarPathFinder.getReverseMoves)
	// whose own move-generation function isn't already passed goal
	// directly - GetPossibleMoves ignores this field, since its own goal
	// parameter serves the same purpose.
	Goal models.V3
}

// dirOffset is a horizontal (X/Z) movement direction offset, shared by every
// GetPossibleMoves-style neighbor scan (forward in this file, reverse in
// bidir_a_star.go) so the four/eight-direction tables are allocated once at
// package init instead of on every single call.
type dirOffset struct {
	dx, dz float64
}

var cardinalDirs = []dirOffset{
	{1, 0},  // East
	{-1, 0}, // West
	{0, 1},  // South
	{0, -1}, // North
}

var diagonalDirs = []dirOffset{
	{1, 1},   // SE
	{1, -1},  // NE
	{-1, 1},  // SW
	{-1, -1}, // NW
}

var allDirs = []dirOffset{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
	{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
}

// blockCacheEntry memoizes a single World.GetBlockAt result.
type blockCacheEntry struct {
	stateID uint32
	loaded  bool
}

// MovementValidator validates whether specific movements are possible
type MovementValidator struct {
	world           models.World
	shapeMgr        models.BlockShapeManager
	playerEyeHeight float64 // Player eye height from models.PlayerEyeHeight
	playerWidth     float64 // Player collision box width (usually 0.6)
	debugCheckCount int     // Counter for debug logging
	climbDebugCount int     // Counter for climb debug logging
	logger          *slog.Logger

	// blockCacheMu guards blockCache. nil blockCache means caching is
	// disabled (the default) - see ResetBlockCache.
	blockCacheMu sync.RWMutex
	blockCache   map[models.V3]blockCacheEntry
}

// NewMovementValidator creates a new movement validator
func NewMovementValidator(w models.World, shapeMgr models.BlockShapeManager, logger *slog.Logger) *MovementValidator {
	return &MovementValidator{
		world:           w,
		shapeMgr:        shapeMgr,
		playerEyeHeight: models.PlayerEyeHeight,
		playerWidth:     0.6,
		logger:          utils.SafeLogger(logger),
	}
}

// ResetBlockCache enables and clears the per-search block-state memoization
// used by getBlock. FindPath implementations (a_star.go, epea_star.go,
// bidir_a_star.go) must call this once at the start of every search: the
// world is static for the duration of one bounded search (the same implicit
// assumption already made by those algorithms' own closedSet/gScores maps),
// so caching within that window is safe, but a long-lived MovementValidator
// used across many calls (e.g. HPA*'s cluster-building validator in
// hpa_builder.go, shared across concurrent goroutines and reused for the
// builder's whole lifetime as chunks load) must NOT cache indefinitely - it
// never calls this, so its blockCache stays nil and getBlock falls back to
// always querying the world directly, unchanged from before this cache
// existed.
func (mv *MovementValidator) ResetBlockCache() {
	mv.blockCacheMu.Lock()
	mv.blockCache = make(map[models.V3]blockCacheEntry, 256)
	mv.blockCacheMu.Unlock()
}

// getBlock is a memoizing wrapper around world.GetBlockAt. GetPossibleMoves
// makes on the order of twenty Can* checks per call, many of which re-query
// the exact same handful of positions (e.g. a stair-ascend check and its
// ground-support fallback both fetch the same groundPos), and repeated calls
// as the search expands neighboring cells re-query positions already seen.
// Profiling showed GetBlockAt's map lookup + lock dominating CPU time in all
// three pathfinders; memoizing it for the lifetime of one search (see
// ResetBlockCache) turns that into one real query per unique position.
func (mv *MovementValidator) getBlock(x, y, z float64) (uint32, bool) {
	pos := models.V3{X: x, Y: y, Z: z}

	mv.blockCacheMu.RLock()
	entry, ok := mv.blockCache[pos]
	mv.blockCacheMu.RUnlock()
	if ok {
		return entry.stateID, entry.loaded
	}

	stateID, loaded := mv.world.GetBlockAt(x, y, z)

	mv.blockCacheMu.Lock()
	if mv.blockCache != nil {
		mv.blockCache[pos] = blockCacheEntry{stateID: stateID, loaded: loaded}
	}
	mv.blockCacheMu.Unlock()

	return stateID, loaded
}

// dx/dz/dy throughout this file's CanXxx methods are wrapped in
// math.Round: they're always meant to be exact small integers by
// construction (every move generated by GetPossibleMoves/getReverseMoves
// adds an integer delta to a position via V3.Add), but `to.X - from.X`
// on a `from` reached via many prior additions can differ from the
// intended integer by ~1e-13 due to ordinary IEEE754 accumulation -
// confirmed live: a real search accumulated enough drift that
// `dx*dx+dz*dz > 1` (meant to reject anything but a single adjacent
// block) evaluated true for what should have been an exactly-adjacent
// step, silently making a perfectly valid Traverse move vanish from
// GetPossibleMoves' output. The search had no way to detect this - it
// looked identical to "no move exists here" - and would go hunting for
// an alternate route, sometimes a very long way round, to reach a
// position where the same move wasn't spuriously blocked. math.Round
// snaps the tiny drift away before any comparison sees it.

// CanTraverse checks if the bot can walk from 'from' to 'to' on the same Y level
// Requires:
// - Blocks at feet level (to.Y) and head level (to.Y+1) are passable
// - Block below feet (to.Y-1) is solid
func (mv *MovementValidator) CanTraverse(from, to models.V3) bool {
	// Check destination is adjacent horizontally
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be on same Y level
	if dy != 0 {
		return false
	}

	// Must be adjacent (1 block away horizontally)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Check if destination is passable (feet and head)
	if !mv.isPositionPassable(to) {
		return false
	}

	// Water destinations use WadeWater/Swim instead - plain Traverse is dry-ground-only
	// (they're not the same speed as dry walking, see MovementType.BaseCost()).
	if mv.destinationIsWater(to) {
		return false
	}

	// Check if there's ground to stand on
	return mv.hasGroundSupport(to)
}

// CanAscend checks if the bot can jump up from 'from' to 'to' (1 block higher)
// Requires:
// - to.Y == from.Y + 1
// - Destination is passable
// - Has ground support
func (mv *MovementValidator) CanAscend(from, to models.V3) bool {
	// Check destination is 1 block higher
	dy := math.Round(to.Y - from.Y)
	if dy != 1 {
		return false
	}

	// Check horizontal distance
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Cannot ascend/jump from water - use ExitWater instead
	if fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z); fromLoaded {
		if mv.shapeMgr.IsWater(fromStateID) {
			return false
		}
	}

	// Debug ascend checks for start position (use integer equality since models.V3 uses float64 for block coords)
	debugPos := int(from.X) == 255 && int(from.Y) == 72 && int(from.Z) == 82 && int(to.X) == 256
	if debugPos {
		utils.DebugVerbose(mv.logger, "[CanAscend]", "fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		if debugPos {
			utils.DebugVerbose(mv.logger, "[CanAscend] FAILED: destination not passable")
		}
		return false
	}

	// Check if there's ground to stand on
	if !mv.hasGroundSupport(to) {
		if debugPos {
			utils.DebugVerbose(mv.logger, "[CanAscend] FAILED: no ground support")
		}
		return false
	}

	// Check if there's headroom to jump
	jumpSpace := from.Add(models.V3{X: 0, Y: 2, Z: 0})
	if !mv.isBlockPassable(jumpSpace) {
		if debugPos {
			utils.DebugVerbose(mv.logger, "[CanAscend] FAILED: no headroom", "x", jumpSpace.X, "y", jumpSpace.Y, "z", jumpSpace.Z)
		}
		return false
	}
	if debugPos {
		utils.DebugVerbose(mv.logger, "[CanAscend] SUCCESS")
	}
	return true
}

// CanAscendStairs checks if the bot can walk up stairs from 'from' to 'to' (1 block higher)
// This is similar to CanAscend but specifically for stair blocks where no jump is needed.
// Requires:
// - to.Y == from.Y + 1
// - Ground block at target (to.Y - 1) is a stair
// - Destination is passable
// - Has ground support
func (mv *MovementValidator) CanAscendStairs(from, to models.V3) bool {
	// Check destination is 1 block higher
	dy := math.Round(to.Y - from.Y)
	if dy != 1 {
		return false
	}

	// Check horizontal distance (cardinal direction only)
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Cannot ascend stairs from water - use ExitWater instead
	if fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z); fromLoaded {
		if mv.shapeMgr.IsWater(fromStateID) {
			return false
		}
	}

	// Check if the ground block at target is a stair
	groundPos := to.Add(models.V3{X: 0, Y: -1, Z: 0})
	groundStateID, loaded := mv.getBlock(groundPos.X, groundPos.Y, groundPos.Z)
	if !loaded || groundStateID == 0 {
		return false
	}
	if !mv.shapeMgr.IsStair(groundStateID) {
		return false // Ground is not a stair
	}

	// Check if destination is passable (feet and head)
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check if there's ground support (stair provides this)
	if !mv.hasGroundSupport(to) {
		return false
	}

	// No headroom check needed for stairs - natural walking, no jump
	return true
}

// CanDescendStairs checks if the bot can walk down stairs from 'from' to 'to' (1 block lower)
// This is for walking down stair blocks naturally without falling.
// Requires:
// - to.Y == from.Y - 1
// - Ground block at current position (from.Y - 1) is a stair
// - Destination is passable
// - Has ground support
func (mv *MovementValidator) CanDescendStairs(from, to models.V3) bool {
	// Check destination is 1 block lower
	dy := math.Round(from.Y - to.Y)
	if dy != 1 {
		return false
	}

	// Check horizontal distance (cardinal direction only)
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Check if the ground block at current position is a stair
	// (we're descending FROM a stair)
	groundPos := from.Add(models.V3{X: 0, Y: -1, Z: 0})
	groundStateID, loaded := mv.getBlock(groundPos.X, groundPos.Y, groundPos.Z)
	if !loaded || groundStateID == 0 {
		return false
	}
	if !mv.shapeMgr.IsStair(groundStateID) {
		return false // Current ground is not a stair
	}

	// Check if destination is passable (feet and head)
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check if there's ground support at destination
	if !mv.hasGroundSupport(to) {
		return false
	}

	return true
}

// CanDescend checks if the bot can drop down from 'from' to 'to' (1-3 blocks lower)
// Requires:
// - to.Y < from.Y (and to.Y >= from.Y - 3 for safety)
// - Destination is passable
// - Has ground support (or is water)
func (mv *MovementValidator) CanDescend(from, to models.V3) bool {
	// Check destination is lower
	dy := math.Round(from.Y - to.Y)
	if dy <= 0 || dy > 3 {
		return false // Not descending, or too far (max 3 block drop)
	}

	// Check horizontal distance
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check if destination is water (allowed even without ground support below)
	toStateID, toLoaded := mv.getBlock(to.X, to.Y, to.Z)
	if toLoaded && toStateID != 0 && mv.shapeMgr.IsWater(toStateID) {
		return true // Water is a valid drop target even without ground support
	}

	// For non-water destinations, require ground support
	if !mv.hasGroundSupport(to) {
		return false
	}

	return true
}

// isPositionPassable checks if a position (feet and head) is passable
func (mv *MovementValidator) isPositionPassable(pos models.V3) bool {
	// Check feet level
	if !mv.isBlockPassable(pos) {
		return false
	}

	// Check head level (1 block above feet)
	head := pos.Add(models.V3{X: 0, Y: 1, Z: 0})
	return mv.isBlockPassable(head)
}

// isBlockPassable checks if a single block is passable
func (mv *MovementValidator) isBlockPassable(pos models.V3) bool {
	// Get block state ID from world
	stateID, loaded := mv.getBlock(pos.X, pos.Y, pos.Z)

	if !loaded {
		return false // Chunk not loaded, not passable
	}

	// State ID 0 is always air (passable)
	if stateID == 0 {
		return true
	}

	// Use shapeMgr to check if passable
	return mv.shapeMgr.IsPassable(stateID)
}

// hasGroundSupport checks if there's solid ground below the position
func (mv *MovementValidator) hasGroundSupport(pos models.V3) bool {
	// Check block directly below feet
	groundPos := pos.Add(models.V3{X: 0, Y: -1, Z: 0})
	stateID, loaded := mv.getBlock(groundPos.X, groundPos.Y, groundPos.Z)

	if !loaded {
		return false // Chunk not loaded
	}

	// State ID 0 is air - no ground support
	if stateID == 0 {
		return false
	}

	// Check if block is solid (provides standing surface)
	return mv.shapeMgr.IsSolid(stateID)
}

// destinationIsWater checks whether the block at pos is water.
func (mv *MovementValidator) destinationIsWater(pos models.V3) bool {
	stateID, loaded := mv.getBlock(pos.X, pos.Y, pos.Z)
	return loaded && stateID != 0 && mv.shapeMgr.IsWater(stateID)
}

// CanDiagonalTraverse checks if the bot can walk diagonally on the same Y level
func (mv *MovementValidator) CanDiagonalTraverse(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be on same Y level
	if dy != 0 {
		return false
	}

	// Must be diagonal (both dx and dz are non-zero and magnitude 1)
	if (dx == 0 || dz == 0) || (dx*dx > 1 || dz*dz > 1) {
		return false
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Water destinations use CanDiagonalWadeWater/Swim instead
	if mv.destinationIsWater(to) {
		return false
	}

	// Check ground support
	if !mv.hasGroundSupport(to) {
		return false
	}

	// Check that the two adjacent cardinal squares are also passable (no corner cutting)
	adj1 := from.Add(models.V3{X: dx, Y: 0, Z: 0})
	adj2 := from.Add(models.V3{X: 0, Y: 0, Z: dz})
	if !mv.isPositionPassable(adj1) || !mv.isPositionPassable(adj2) {
		return false
	}

	return true
}

// CanWadeWater checks if the bot can wade/tread through water on the same Y level,
// at non-sprint wade speed (WadeWater), independent of whether there's ground support
// below `to` - vanilla's travelInFluid speed doesn't depend on it (verified against
// decompiled source; see WATER_TRAVERSAL_PATHFINDING_PLAN.md). Use CanSwim instead when
// the water is deep enough to fully submerge, which is cheaper.
func (mv *MovementValidator) CanWadeWater(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	if dy != 0 {
		return false
	}
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	if !mv.isPositionPassable(to) {
		return false
	}

	return mv.destinationIsWater(to)
}

// CanDiagonalWadeWater is CanWadeWater's diagonal counterpart, mirroring
// CanDiagonalTraverse's corner-cutting check.
func (mv *MovementValidator) CanDiagonalWadeWater(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	if dy != 0 {
		return false
	}
	if (dx == 0 || dz == 0) || (dx*dx > 1 || dz*dz > 1) {
		return false
	}

	if !mv.isPositionPassable(to) {
		return false
	}
	if !mv.destinationIsWater(to) {
		return false
	}

	adj1 := from.Add(models.V3{X: dx, Y: 0, Z: 0})
	adj2 := from.Add(models.V3{X: 0, Y: 0, Z: dz})
	if !mv.isPositionPassable(adj1) || !mv.isPositionPassable(adj2) {
		return false
	}

	return true
}

// CanDiagonalAscend checks if the bot can jump up diagonally
func (mv *MovementValidator) CanDiagonalAscend(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be 1 block higher
	if dy != 1 {
		return false
	}

	// Must be diagonal
	if (dx == 0 || dz == 0) || (dx*dx > 1 || dz*dz > 1) {
		return false
	}

	// Cannot diagonally ascend from water - use ExitWater instead
	if fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z); fromLoaded {
		if mv.shapeMgr.IsWater(fromStateID) {
			return false
		}
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check ground support
	if !mv.hasGroundSupport(to) {
		return false
	}

	// Check headroom to jump
	jumpSpace := from.Add(models.V3{X: 0, Y: 2, Z: 0})
	if !mv.isBlockPassable(jumpSpace) {
		return false
	}

	// Check adjacent squares are passable (no corner cutting)
	adj1 := from.Add(models.V3{X: dx, Y: 0, Z: 0})
	adj2 := from.Add(models.V3{X: 0, Y: 0, Z: dz})
	if !mv.isPositionPassable(adj1) || !mv.isPositionPassable(adj2) {
		return false
	}

	return true
}

// CanJump2 checks if the bot can jump across a 2-block gap
func (mv *MovementValidator) CanJump2(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be on same Y level or up to 1 block higher
	if dy < 0 || dy > 1 {
		return false
	}

	// Must be 2 blocks away in one cardinal direction
	if (dx != 0 && dz != 0) || (dx*dx+dz*dz != 4) {
		return false
	}

	// Cannot jump from water - use ExitWater instead
	if fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z); fromLoaded {
		if mv.shapeMgr.IsWater(fromStateID) {
			return false
		}
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check ground support
	if !mv.hasGroundSupport(to) {
		return false
	}

	// Check headroom to jump
	jumpSpace := from.Add(models.V3{X: 0, Y: 2, Z: 0})
	return mv.isBlockPassable(jumpSpace)
}

// CanClimb checks if the bot can climb (ladders, vines, etc.)
func (mv *MovementValidator) CanClimb(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be moving vertically (up or down)
	if dy == 0 {
		return false
	}

	// Must be same horizontal position or adjacent
	if dx*dx+dz*dz > 1 {
		return false
	}

	// Cannot climb from water - use ExitWater instead
	if fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z); fromLoaded {
		if mv.shapeMgr.IsWater(fromStateID) {
			return false
		}
	}

	// Check if there's a climbable block at destination
	stateID, loaded := mv.getBlock(to.X, to.Y, to.Z)
	if !loaded || stateID == 0 {
		return false // Chunk not loaded or air, not climbable
	}

	return mv.shapeMgr.IsClimbable(stateID)
}

// CanSwim checks if the bot can swim horizontally through water
// Allows entering water from solid ground or swimming water-to-water.
func (mv *MovementValidator) CanSwim(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be on same Y level
	if dy != 0 {
		return false
	}

	// Must be adjacent (cardinal or diagonal)
	if dx*dx+dz*dz == 0 || dx*dx+dz*dz > 2 {
		return false
	}

	// Check if destination is water (required). Not gated by depth/submersion: verified
	// against decompiled source, the sprint-swim speed boost (BaseCost's Swim vs WadeWater)
	// is purely a function of sprinting while touching water, not how deep it is or
	// whether the head is submerged - see WATER_TRAVERSAL_PATHFINDING_PLAN.md.
	return mv.destinationIsWater(to)
}

// CanSwimUp checks if the bot can swim upward in water
// Allows entering water from above (e.g., descending into water then ascending) or swimming water-to-water upward.
func (mv *MovementValidator) CanSwimUp(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be moving up (1 block)
	if dy != 1 {
		return false
	}

	// Must be same horizontal position or adjacent
	if dx*dx+dz*dz > 1 {
		return false
	}

	// Check if destination is water (required) - see CanSwim's doc comment on why this
	// isn't depth/submersion-gated.
	return mv.destinationIsWater(to)
}

// CanSwimDown checks if the bot can swim downward in water
// Only allows swimming down when already in water (water-to-water downward).
// For transitioning from solid ground to water, use Descend instead.
func (mv *MovementValidator) CanSwimDown(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be moving down (1-2 blocks)
	if dy >= 0 || dy < -2 {
		return false
	}

	// Must be same horizontal position or adjacent
	if dx*dx+dz*dz > 1 {
		return false
	}

	// Check if destination is water (required) - see CanSwim's doc comment.
	if !mv.destinationIsWater(to) {
		return false
	}

	// Only allow SwimDown if already in water at the starting position
	// This ensures we're swimming within water, not transitioning from ground to water
	// Transitioning from ground to water should use Descend instead
	fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z)
	if !fromLoaded {
		return false
	}

	return mv.shapeMgr.IsWater(fromStateID)
}

// CanExitWater checks if the bot can exit from water onto adjacent solid ground
// Allows two scenarios:
// - Scenario 1: Same-level exit (destination ground at same Y as water, with solid support below)
// - Scenario 2: Step-up exit (water at Y, solid ground at Y+1 if water itself is on solid)
func (mv *MovementValidator) CanExitWater(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be adjacent horizontally (cardinal direction only)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Must be same level or up to 1 block higher
	if dy < 0 || dy > 1 {
		return false
	}

	// Must be currently in water
	fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z)
	if !fromLoaded || !mv.shapeMgr.IsWater(fromStateID) {
		return false
	}

	// Check if destination is passable (feet and head)
	if !mv.isPositionPassable(to) {
		utils.DebugVerbose(mv.logger, "[CanExitWater] FAILED: destination not passable",
			"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
		return false
	}

	// Check that destination is NOT water (we're exiting water, not swimming sideways)
	toStateID, toLoaded := mv.getBlock(to.X, to.Y, to.Z)
	if !toLoaded {
		return false // Chunk not loaded
	}
	if toStateID != 0 && mv.shapeMgr.IsWater(toStateID) {
		return false // Destination is water - not a valid exit from water
	}

	// Scenario 1: Same-level exit (dy == 0)
	// Destination must have ground support (solid block below at to.Y-1)
	if dy == 0 {
		if mv.hasGroundSupport(to) {
			utils.DebugVerbose(mv.logger, "[CanExitWater] SUCCESS: same-level exit",
				"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
			return true
		}
		utils.DebugVerbose(mv.logger, "[CanExitWater] FAILED: no ground support (same-level)",
			"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
	}

	// Scenario 2: Step-up exit (dy == 1)
	// Agent is stepping up from water into air (standing on ground that's below the water surface)
	// The destination (to.Y) should be passable (air) with solid ground support below
	if dy == 1 {
		// Check if destination is passable (feet and head)
		if !mv.isPositionPassable(to) {
			utils.DebugVerbose(mv.logger, "[CanExitWater] FAILED: destination not passable (step-up)",
				"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
			return false
		}

		// Check if there's ground support below the destination
		if !mv.hasGroundSupport(to) {
			utils.DebugVerbose(mv.logger, "[CanExitWater] FAILED: no ground support (step-up)",
				"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
			return false
		}

		utils.DebugVerbose(mv.logger, "[CanExitWater] SUCCESS: step-up exit to air with ground support",
			"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
		return true
	}

	return false
}

// isOnClimbable checks if the given position has a climbable block (ladder/vine)
func (mv *MovementValidator) isOnClimbable(pos models.V3) bool {
	stateID, loaded := mv.getBlock(pos.X, pos.Y, pos.Z)
	if !loaded || stateID == 0 {
		return false
	}
	return mv.shapeMgr.IsClimbable(stateID)
}

// CanEnterClimb checks if the bot can enter a climbable block from an adjacent position
// Requires:
// - Target position has a climbable block (ladder/vine)
// - From and to are adjacent (cardinal direction, same Y or ±1)
func (mv *MovementValidator) CanEnterClimb(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be adjacent horizontally (cardinal direction only)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Must be same Y or ±1
	if dy < -1 || dy > 1 {
		return false
	}

	// Cannot enter climb from water - use ExitWater instead
	if fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z); fromLoaded {
		if mv.shapeMgr.IsWater(fromStateID) {
			return false
		}
	}

	// Check if target has a climbable block
	return mv.isOnClimbable(to)
}

// CanExitClimb checks if the bot can exit from a climbable block onto an adjacent platform
// Handles two scenarios:
// - Scenario 1: Same-level exit (to block is air with ground support)
// - Scenario 2: Step-up exit (to block is solid, to+1 is air - stepping onto platform)
func (mv *MovementValidator) CanExitClimb(from, to models.V3) bool {
	dx := math.Round(to.X - from.X)
	dz := math.Round(to.Z - from.Z)
	dy := math.Round(to.Y - from.Y)

	// Must be adjacent horizontally (cardinal direction only)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Must be same Y level for the check (we adjust Y later for step-up)
	if dy != 0 {
		return false
	}

	// Must be on a climbable block
	if !mv.isOnClimbable(from) {
		return false
	}

	// Cannot exit climb from water (shouldn't happen if on climbable, but check for safety)
	if fromStateID, fromLoaded := mv.getBlock(from.X, from.Y, from.Z); fromLoaded {
		if mv.shapeMgr.IsWater(fromStateID) {
			return false
		}
	}

	// Get the block at target position
	toStateID, toLoaded := mv.getBlock(to.X, to.Y, to.Z)
	if !toLoaded {
		return false
	}

	toPassable := toStateID == 0 || mv.shapeMgr.IsPassable(toStateID)

	// Scenario 1: Same-level exit (to is passable with ground support)
	if toPassable {
		// Check if there's ground to stand on and head clearance
		if mv.hasGroundSupport(to) && mv.isPositionPassable(to) {
			utils.DebugVerbose(mv.logger, "[CanExitClimb] SUCCESS: same-level exit",
				"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
			return true
		}
	}

	// Scenario 2: Step-up exit (to is solid floor, step up onto it)
	// Check if to+1 is passable (air above the floor)
	toUp := to.Add(models.V3{X: 0, Y: 1, Z: 0})
	toUpStateID, toUpLoaded := mv.getBlock(toUp.X, toUp.Y, toUp.Z)
	if !toUpLoaded {
		return false
	}

	toUpPassable := toUpStateID == 0 || mv.shapeMgr.IsPassable(toUpStateID)

	if toUpPassable {
		// Check if 'to' block is solid (the floor we step onto)
		toSolid := toStateID != 0 && mv.shapeMgr.IsSolid(toStateID)
		if toSolid {
			// Check head clearance at to+2
			toUp2 := to.Add(models.V3{X: 0, Y: 2, Z: 0})
			if mv.isBlockPassable(toUp2) {
				utils.DebugVerbose(mv.logger, "[CanExitClimb] SUCCESS: step-up exit",
					"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z,
					"stepX", toUp.X, "stepY", toUp.Y, "stepZ", toUp.Z)
				return true
			}
		}
	}

	return false
}

// getExitClimbTargetY determines the correct Y coordinate for an ExitClimb move
// For same-level exits: returns to.Y
// For step-up exits: returns to.Y + 1
func (mv *MovementValidator) getExitClimbTargetY(from, to models.V3) float64 {
	// Check if this is a same-level exit
	toStateID, toLoaded := mv.getBlock(to.X, to.Y, to.Z)
	if !toLoaded {
		return to.Y
	}

	toPassable := toStateID == 0 || mv.shapeMgr.IsPassable(toStateID)

	// If to is passable and has ground support, it's a same-level exit
	if toPassable && mv.hasGroundSupport(to) {
		return to.Y
	}

	// Otherwise it's a step-up exit - target is one block higher
	return to.Y + 1
}

// FindGroundBelow finds the nearest valid ground level below the given position
// Returns the Y coordinate where the entity's feet should be (standing on solid ground)
// Returns -1 if no valid ground found
func (mv *MovementValidator) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	utils.DebugVerbose(mv.logger, "[FindGroundBelow] searching", "x", x, "z", z, "startY", startY)

	// Dimension-specific build height (Overworld: -64..319; Nether/End:
	// 0..255) — searching past either bound would either waste time
	// (Nether's column is shallower than the Overworld's) or miss valid
	// ground near a dimension's own true ceiling/floor if it were ever
	// narrower than the Overworld's hardcoded range this used to assume
	// unconditionally.
	minWorldY, maxWorldY := -64.0, 319.0
	if mv.world != nil {
		lo, hi := mv.world.GetHeightBounds()
		minWorldY, maxWorldY = float64(lo), float64(hi)
	}

	// Check current position first
	currentStateID, _ := mv.getBlock(x, startY, z)

	// If current position is inside a solid block, search UPWARD to find surface
	if currentStateID != 0 {
		isSolid := mv.shapeMgr.IsSolid(currentStateID)

		if isSolid {
			utils.DebugVerbose(mv.logger, "[FindGroundBelow] bot inside solid block, searching upward for surface",
				"startY", startY, "stateID", currentStateID)

			// Search upward to find the top of the solid terrain
			for y := startY; y <= startY+maxSearchDepth && y <= maxWorldY; y++ {
				pos := models.V3{X: x, Y: y, Z: z}

				// Check if this is a valid standing position (air with solid below)
				if mv.isPositionPassable(pos) && mv.hasGroundSupport(pos) {
					utils.DebugVerbose(mv.logger, "[FindGroundBelow] found surface searching upward",
						"y", y, "x", x, "z", z, "startY", startY)
					return y
				}
			}
			utils.DebugVerbose(mv.logger, "[FindGroundBelow] no surface found searching upward", "startY", startY)
		}
	}

	// If not in solid block, search downward for valid ground
	utils.DebugVerbose(mv.logger, "[FindGroundBelow] searching downward for valid standing position", "startY", startY)

	for y := startY; y >= startY-maxSearchDepth && y >= minWorldY; y-- {
		pos := models.V3{X: x, Y: y, Z: z}

		// Check if this position is valid:
		// 1. Feet and head blocks must be passable (air or passable blocks)
		// 2. Block below feet must be solid (ground support)
		if mv.isPositionPassable(pos) && mv.hasGroundSupport(pos) {
			utils.DebugVerbose(mv.logger, "[FindGroundBelow] found valid ground searching downward",
				"y", y, "x", x, "z", z, "startY", startY)
			return y
		}
	}

	utils.DebugVerbose(mv.logger, "[FindGroundBelow] no valid ground found", "x", x, "z", z, "startY", startY)
	return -1 // No valid ground found
}

// logTerrainAround logs all blocks around a position for debugging
func (mv *MovementValidator) logTerrainAround(from models.V3) {
	if !utils.DebugVerboseEnabled(mv.logger) {
		return
	}
	utils.DebugVerbose(mv.logger, "[TERRAIN] terrain around", "x", from.X, "y", from.Y, "z", from.Z)

	// All 8 directions plus center
	directions := []struct {
		name   string
		dx, dz float64
	}{
		{"CENTER", 0, 0},
		{"NORTH", 0, -1},
		{"SOUTH", 0, 1},
		{"EAST", 1, 0},
		{"WEST", -1, 0},
		{"NE", 1, -1},
		{"SE", 1, 1},
		{"SW", -1, 1},
		{"NW", -1, -1},
	}
	// For each direction, log blocks from Y-1 to Y+3
	for _, dir := range directions {
		x := from.X + dir.dx
		z := from.Z + dir.dz

		utils.DebugVerbose(mv.logger, "[TERRAIN] column", "dir", dir.name, "x", x, "z", z)
		for dy := float64(-1); dy <= 3; dy++ {
			y := from.Y + dy
			stateID, _ := mv.getBlock(x, y, z)

			var blockName string
			var passable bool
			var solid bool

			if stateID == 0 {
				blockName = "air"
				passable = true
				solid = false
			} else {
				blockName = fmt.Sprintf("stateID=%d", stateID)
				passable = mv.shapeMgr.IsPassable(stateID)
				solid = mv.shapeMgr.IsSolid(stateID)
			}

			var yLabel string
			switch dy {
			case -1:
				yLabel = "Y-1(ground)"
			case 0:
				yLabel = "Y  (feet)  "
			case 1:
				yLabel = "Y+1(head)  "
			case 2:
				yLabel = "Y+2(jump)  "
			case 3:
				yLabel = "Y+3        "
			}
			utils.DebugVerbose(mv.logger, "[TERRAIN] block", "level", yLabel, "block", blockName, "passable", passable, "solid", solid)
		}
	}
}

// mineBaseCostPerBlock/mineHardnessCostFactor set MineThrough/MineDown's
// real per-step cost (see mineMoveCost) - deliberately well above any
// walking detour (Traverse=1.0) so a path search almost always prefers
// walking around, but still finite, so a bot with no walkable route left at
// all (a straight 1x1 shaft, see MineDown's own doc comment) can still dig
// itself out. hardness scales the cost on top of the flat premium so a
// search offered a choice between mining through dirt vs. stone prefers the
// softer block, same reasoning CalcBreakTime (agent/mining) already applies
// to real break time - not calibrated against it directly, since the
// pathfinder doesn't know what tool (if any) the bot is holding; see
// mineObstructionAt's own doc comment for that scope note.
const (
	mineBaseCostPerBlock   = 8.0
	mineHardnessCostFactor = 2.0

	// mineStraightDownRiskSurcharge makes plain MineDown strictly pricier
	// than MineDownDiagonal's otherwise-identical per-block cost (operator
	// direction, 2026-10-14): mining the exact block directly underfoot
	// commits to whatever it reveals - a cave, lava - with no fallback,
	// since the bot was never standing anywhere else. MineDownDiagonal
	// mines one column over instead, so a search should prefer it by
	// default and only fall back to plain MineDown when there's no room
	// for the diagonal pattern at all (a shaft already dug 1-wide). Not
	// calibrated against any real quantity - purely a preference knob, the
	// same way the rest of this file's mine costs already are.
	mineStraightDownRiskSurcharge = 6.0
)

// mineMoveCost sums the pseudo-expensive cost of breaking every block in
// hardnesses (1 for MineDown, 1 or 2 for MineThrough - see
// PathStep.MineTargets' own doc comment).
func mineMoveCost(hardnesses ...float64) float64 {
	cost := 0.0
	for _, h := range hardnesses {
		if h < 0 {
			h = 0
		}
		cost += mineBaseCostPerBlock + h*mineHardnessCostFactor
	}
	return cost
}

// mineObstructionAt reports whether pos holds a solid, diggable block worth
// offering as a MineThrough/MineDown route - never for an already-passable
// cell (nothing to mine), a fluid (Swim/WadeWater already cover water, and
// mining into lava/magma is never a sane route - IsDangerous also excludes
// fire, cacti, etc.), or anything GetMiningInfo reports as undiggable
// (bedrock) or unbreakable (negative hardness, same convention
// agent/mining.BlockMiningInfo.Hardness documents).
//
// Deliberately tool-agnostic: this only decides whether a route exists and
// how it should be costed relative to walking, not whether the bot
// currently holds a tool that can break this specific block in reasonable
// time - that real-world enforcement already happens server-side (the live
// server silently drops nothing for too-low a tool tier) and is out of
// scope for a first version of this move, same as the rest of
// docs/plans/14's "What's missing" list already treats tool-tier modeling
// as a later concern.
func (mv *MovementValidator) mineObstructionAt(pos models.V3) (hardness float64, ok bool) {
	stateID, loaded := mv.getBlock(pos.X, pos.Y, pos.Z)
	if !loaded || stateID == 0 {
		return 0, false // air or unloaded chunk - nothing to mine
	}
	if mv.shapeMgr.IsPassable(stateID) {
		return 0, false // already passable - not an obstruction
	}
	if mv.shapeMgr.IsFluid(stateID) || mv.shapeMgr.IsDangerous(stateID) {
		return 0, false
	}
	h, _, diggable := mv.shapeMgr.GetMiningInfo(stateID)
	if !diggable || h < 0 {
		return 0, false
	}
	return h, true
}

// tryMineThrough checks whether MineThrough can replace an unavailable
// Traverse/WadeWater move from `from` to the horizontally-adjacent `to`:
// either or both of the feet-level cell (at `to`) and the head-level cell
// (directly above it) may need breaking, but a cell that's solid yet
// undiggable makes the whole move impossible, not just skips mining it
// (mirrors CanTraverse's own isPositionPassable/hasGroundSupport
// requirements for everything except "solid" itself).
func (mv *MovementValidator) tryMineThrough(from, to models.V3) (PathStep, bool) {
	if mv.destinationIsWater(to) {
		return PathStep{}, false // WadeWater/Swim's territory, not mining's
	}

	head := to.Add(models.V3{X: 0, Y: 1, Z: 0})
	feetHardness, feetBlocked := mv.mineObstructionAt(to)
	headHardness, headBlocked := mv.mineObstructionAt(head)

	if !feetBlocked && !headBlocked {
		return PathStep{}, false // CanTraverse already covers every other reason this move is unavailable
	}
	if !feetBlocked && !mv.isBlockPassable(to) {
		return PathStep{}, false // feet-level cell is solid but undiggable - no route at all
	}
	if !headBlocked && !mv.isBlockPassable(head) {
		return PathStep{}, false // head-level cell is solid but undiggable - no route at all
	}
	if !mv.hasGroundSupport(to) {
		return PathStep{}, false // same requirement CanTraverse enforces
	}

	targets := make([]models.V3, 0, 2)
	hardnesses := make([]float64, 0, 2)
	if feetBlocked {
		targets = append(targets, to)
		hardnesses = append(hardnesses, feetHardness)
	}
	if headBlocked {
		targets = append(targets, head)
		hardnesses = append(hardnesses, headHardness)
	}

	return PathStep{
		Position:    to,
		Movement:    MineThrough,
		Cost:        mineMoveCost(hardnesses...),
		MineTargets: targets,
	}, true
}

// tryMineDown checks whether MineDown can offer a route one block straight
// down from `from` (same X/Z column) when the block there is a solid,
// diggable obstruction. No ground-support requirement on the far side,
// unlike every other move here - the whole point is to keep digging through
// more solid material below, which the next GetPossibleMoves call will
// offer again on its own if that's also an obstruction (see MineDown's own
// doc comment). Carries mineStraightDownRiskSurcharge on top of the
// ordinary per-block cost - see that constant's own doc comment for why.
func (mv *MovementValidator) tryMineDown(from models.V3) (PathStep, bool) {
	straightDown := from.Add(models.V3{X: 0, Y: -1, Z: 0})
	hardness, ok := mv.mineObstructionAt(straightDown)
	if !ok {
		return PathStep{}, false
	}
	return PathStep{
		Position:    straightDown,
		Movement:    MineDown,
		Cost:        mineMoveCost(hardness) + mineStraightDownRiskSurcharge,
		MineTargets: []models.V3{straightDown},
	}, true
}

// tryMineDownDiagonal checks whether MineDownDiagonal can offer a route one
// block down AND one block over, in cardinal direction (dx, dz), when the
// landing cell, or either of the two cells directly above it at and above
// `from`'s own Y - both of which need to be clear for the bot's two-block
// hitbox to fit while it's still walking across at `from`'s height, just
// before it drops - is a solid, diggable obstruction. Deliberately never
// touches the block directly underfoot (see MineDownDiagonal's own doc
// comment for why that's the whole point): the column the bot is
// currently standing on stays untouched, intact rock, regardless of what
// this move reveals one column over.
//
// headroomAbove (from.Y+1) was missing for a while - headroom (from.Y)
// alone matches the single cell tryMineThrough treats as "head-level," but
// that's head-level relative to a move landing AT from.Y; here the bot is
// still standing AT from.Y walking sideways before it falls, so the two
// cells it actually occupies during the crossing are from.Y (feet) and
// from.Y+1 (head), not from.Y and landing.Y+1=from.Y. Found live: a bot
// could dig straight through a buried shaft on the first MineDownDiagonal
// (approaching along flat ground, where from.Y+1 is always open sky), then
// hang indefinitely re-pathfinding to the exact same spot on the very next
// one once it was already standing inside its own dug-out shaft - at that
// depth, the column one-over's from.Y+1 cell is ordinary undug terrain
// far more often than not, and nothing was ever offering to clear it.
//
// A solid-but-undiggable cell on any of the three makes the whole move
// impossible, same as tryMineThrough's identical reasoning for its own two
// cells. No ground-support requirement below the landing cell, same as
// tryMineDown - the next GetPossibleMoves call offers the next step (either
// MineDownDiagonal again, continuing the stagger, or MineDown as a
// fallback) on its own.
func (mv *MovementValidator) tryMineDownDiagonal(from models.V3, dx, dz float64) (PathStep, bool) {
	landing := from.Add(models.V3{X: dx, Y: -1, Z: dz})
	headroom := from.Add(models.V3{X: dx, Y: 0, Z: dz})
	headroomAbove := from.Add(models.V3{X: dx, Y: 1, Z: dz})

	landingHardness, landingBlocked := mv.mineObstructionAt(landing)
	headroomHardness, headroomBlocked := mv.mineObstructionAt(headroom)
	headroomAboveHardness, headroomAboveBlocked := mv.mineObstructionAt(headroomAbove)

	if !landingBlocked && !headroomBlocked && !headroomAboveBlocked {
		return PathStep{}, false // nothing to mine - an ordinary diagonal drop (the dropHeight=1 case this replaces) already covers it
	}
	if !landingBlocked && !mv.isBlockPassable(landing) {
		return PathStep{}, false // landing cell is solid but undiggable - no route at all
	}
	if !headroomBlocked && !mv.isBlockPassable(headroom) {
		return PathStep{}, false // headroom cell is solid but undiggable - no route at all
	}
	if !headroomAboveBlocked && !mv.isBlockPassable(headroomAbove) {
		return PathStep{}, false // headroomAbove cell is solid but undiggable - no route at all
	}

	targets := make([]models.V3, 0, 3)
	hardnesses := make([]float64, 0, 3)
	if landingBlocked {
		targets = append(targets, landing)
		hardnesses = append(hardnesses, landingHardness)
	}
	if headroomBlocked {
		targets = append(targets, headroom)
		hardnesses = append(hardnesses, headroomHardness)
	}
	if headroomAboveBlocked {
		targets = append(targets, headroomAbove)
		hardnesses = append(hardnesses, headroomAboveHardness)
	}

	return PathStep{
		Position:    landing,
		Movement:    MineDownDiagonal,
		Cost:        mineMoveCost(hardnesses...),
		MineTargets: targets,
	}, true
}

// tryMineNeighborFloor checks whether MineNeighborFloor can offer a route
// down into an adjacent column that's already walkable at `from`'s own
// height - typically the column the bot just entered from (after an
// ordinary MineThrough step mined the wall between them), but checked
// directly against world state, not path history (see below for why): mine
// that column's floor - and, if that's also a genuine obstruction, the
// block one level beneath it too - then drop into it, landing at whichever
// depth (1 or 2 blocks) turned out to need mining. The bot is never the one
// mining the exact block it's currently standing on with nothing else
// holding it up (the specific risk plain MineDown has) - it's standing in
// the ADJACENT column throughout the mining, only moving once both the dig
// and the landing cell's own ground support are confirmed.
//
// This move's own safety property is structural, the same shape
// MineDownDiagonal already established - not hazard detection. Lava/cave
// avoidance at the final landing spot is, as always, ordinary
// ground-support-required passability (mv.hasGroundSupport, checked below
// before this move is ever offered), the same requirement CanDescend
// already applies to every ordinary drop; this move doesn't add or weaken
// that, it just lets a floor that needed mining first reach the same
// check.
//
// An earlier attempt at this feature tried to represent "the bot is
// already straddling two columns" as a precondition and relocate it only
// at the very end, after both columns' floors were mined. That doesn't
// work in this search: every node here is a plain integer models.V3 (see
// a_star.go's gScores/closedSet, keyed purely on position), so a PathStep
// whose Position doesn't change from `from` is a self-loop the search
// closes out and never actually selects (closedSet[from] is already true
// by the time GetPossibleMoves' own successors are considered - confirmed
// by reading a_star.go's loop, not just suspected). This version sidesteps
// that entirely by changing position in the same step that does the
// mining, exactly like every other move here already does.
func (mv *MovementValidator) tryMineNeighborFloor(from models.V3, dx, dz float64) (PathStep, bool) {
	colA := from.Add(models.V3{X: dx, Y: 0, Z: dz})

	// colA must already be walkable at `from`'s own height - true right
	// after entering from this same direction via an ordinary MineThrough
	// step, but this checks current world state directly, not history:
	// there's no path history available to this function anyway (see its
	// own doc comment), and it doesn't need any - a neighbor that's
	// already open at this height for any other reason (e.g. a genuine
	// cave opening) is just as valid a route down.
	if !mv.isPositionPassable(colA) {
		return PathStep{}, false
	}

	level1 := colA.Add(models.V3{X: 0, Y: -1, Z: 0})
	hardness1, blocked1 := mv.mineObstructionAt(level1)
	if !blocked1 {
		return PathStep{}, false // nothing to mine - an ordinary Descend already covers this
	}

	targets := []models.V3{level1}
	hardnesses := []float64{hardness1}
	landing := level1

	// "...followed by the block under it (if there is one)" - only extend
	// one level deeper when that cell is itself a genuine, diggable
	// obstruction. Already-open (a cave) or solid-but-undiggable both
	// correctly leave the hole exactly one block deep.
	level2 := level1.Add(models.V3{X: 0, Y: -1, Z: 0})
	if hardness2, blocked2 := mv.mineObstructionAt(level2); blocked2 {
		targets = append(targets, level2)
		hardnesses = append(hardnesses, hardness2)
		landing = level2
	}

	if !mv.hasGroundSupport(landing) {
		return PathStep{}, false // no confirmed solid ground at the resulting depth - decline rather than guess deeper
	}

	return PathStep{
		Position:    landing,
		Movement:    MineNeighborFloor,
		Cost:        mineMoveCost(hardnesses...),
		MineTargets: targets,
	}, true
}

// GetPossibleMoves returns all valid moves from a given position
// Optionally filters moves based on goal to reduce search space (pass zero models.V3 to disable filtering)
func (mv *MovementValidator) GetPossibleMoves(from models.V3, goal models.V3, prune *MovePruneConfig) []PathStep {
	moves := make([]PathStep, 0, 32) // Increased capacity for more move types

	// Debug: log terrain around start position once
	if mv.debugCheckCount == 0 {
		mv.logTerrainAround(from)
	}

	// Check if we're on a climbable (affects which moves are valid)
	onClimbable := mv.isOnClimbable(from)
	atTopOfClimbable := onClimbable && !mv.CanClimb(from, from.Add(models.V3{X: 0, Y: 1, Z: 0}))

	// Cardinal movements (cardinalDirs/diagonalDirs are package-level, see top of file)
	for _, dir := range cardinalDirs {
		// Try traverse (same level)
		to := from.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if mv.CanTraverse(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: Traverse,
				Cost:     Traverse.BaseCost(),
			})
		} else if mv.CanWadeWater(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: WadeWater,
				Cost:     WadeWater.BaseCost(),
			})
		} else if step, ok := mv.tryMineThrough(from, to); ok {
			moves = append(moves, step)
		}

		// Try ascend (1 block up)
		// IMPORTANT: Skip when at top of climbable - use ExitClimb instead
		toUp := from.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if !atTopOfClimbable {
			// First try AscendStairs (walking up stairs naturally, no jump needed)
			if mv.CanAscendStairs(from, toUp) {
				moves = append(moves, PathStep{
					Position: toUp,
					Movement: AscendStairs,
					Cost:     AscendStairs.BaseCost(),
				})
			} else if mv.CanAscend(from, toUp) {
				// Fall back to AscendJump (jumping up a full block)
				moves = append(moves, PathStep{
					Position: toUp,
					Movement: AscendJump,
					Cost:     AscendJump.BaseCost(),
				})
			}
		}

		// Try descend (1 block down first for stairs, then 1-3 blocks for drops)
		toDown := from.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		// First try DescendStairs (walking down stairs naturally)
		if mv.CanDescendStairs(from, toDown) {
			moves = append(moves, PathStep{
				Position: toDown,
				Movement: DescendStairs,
				Cost:     DescendStairs.BaseCost(),
			})
		} else {
			// Fall back to Descend (dropping 1-3 blocks)
			found := false
			for dropHeight := float64(1); dropHeight <= 3; dropHeight++ {
				toDropDown := from.Add(models.V3{X: dir.dx, Y: -dropHeight, Z: dir.dz})
				if mv.CanDescend(from, toDropDown) {
					moves = append(moves, PathStep{
						Position: toDropDown,
						Movement: Descend,
						Cost:     Descend.BaseCost(),
					})
					found = true
					break // Only take the first valid drop height
				}
			}
			// No ordinary 1-3 block drop at all in this direction - try
			// MineDownDiagonal, the safer alternative to straight-down
			// MineDown (see its own doc comment), specifically for the
			// 1-block-down-1-over shape (dropHeight=1's own failure is
			// exactly what makes room for this).
			if !found {
				if step, ok := mv.tryMineDownDiagonal(from, dir.dx, dir.dz); ok {
					moves = append(moves, step)
				}
				// MineNeighborFloor only applies when the adjacent column
				// is already walkable at this height (see its own doc
				// comment) - a cheap, self-contained check everywhere
				// else, so trying it unconditionally alongside
				// MineDownDiagonal costs nothing when it doesn't apply.
				if step, ok := mv.tryMineNeighborFloor(from, dir.dx, dir.dz); ok {
					moves = append(moves, step)
				}
			}
		}

		// Try jump2 (2-block gap)
		toJump := from.Add(models.V3{X: dir.dx * 2, Y: 0, Z: dir.dz * 2})
		if mv.CanJump2(from, toJump) {
			moves = append(moves, PathStep{
				Position: toJump,
				Movement: Jump2,
				Cost:     Jump2.BaseCost(),
			})
		}

		// Try jump2 up (2-block gap, 1 block higher)
		toJumpUp := from.Add(models.V3{X: dir.dx * 2, Y: 1, Z: dir.dz * 2})
		if mv.CanJump2(from, toJumpUp) {
			moves = append(moves, PathStep{
				Position: toJumpUp,
				Movement: Jump2,
				Cost:     Jump2.BaseCost() + 0.5, // Slightly more expensive
			})
		}
	}

	// Mine straight down (same X/Z column) - deliberately outside the
	// cardinalDirs loop above, since this is never direction-dependent: see
	// MineDown's own doc comment for why no existing per-direction
	// Ascend/Descend check ever covers this case.
	if step, ok := mv.tryMineDown(from); ok {
		moves = append(moves, step)
	}

	// Diagonal movements
	for _, dir := range diagonalDirs {
		// Try diagonal traverse
		to := from.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if mv.CanDiagonalTraverse(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: DiagonalTraverse,
				Cost:     DiagonalTraverse.BaseCost(),
			})
		} else if mv.CanDiagonalWadeWater(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: WadeWater,
				Cost:     WadeWater.BaseCost(),
			})
		}

		// Try diagonal ascend
		// IMPORTANT: Skip when at top of climbable - use ExitClimb instead
		toUp := from.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if !atTopOfClimbable && mv.CanDiagonalAscend(from, toUp) {
			moves = append(moves, PathStep{
				Position: toUp,
				Movement: DiagonalAscend,
				Cost:     DiagonalAscend.BaseCost(),
			})
		}
	}

	// Climbing (world integration complete)
	for dy := float64(-1); dy <= 1; dy++ {
		if dy == 0 {
			continue
		}
		to := from.Add(models.V3{X: 0, Y: dy, Z: 0})
		if mv.CanClimb(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: Climb,
				Cost:     Climb.BaseCost(),
			})
		}
	}

	// EnterClimb - entering a climbable block from adjacent position
	for _, dir := range cardinalDirs {
		// Same level enter
		to := from.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if mv.CanEnterClimb(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: EnterClimb,
				Cost:     EnterClimb.BaseCost(),
			})
		}

		// Enter from below (jumping to grab ladder)
		toUp := from.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if mv.CanEnterClimb(from, toUp) {
			moves = append(moves, PathStep{
				Position: toUp,
				Movement: JumpToClimb,
				Cost:     JumpToClimb.BaseCost(),
			})
		}

		// Enter from above (dropping onto ladder)
		toDown := from.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if mv.CanEnterClimb(from, toDown) {
			moves = append(moves, PathStep{
				Position: toDown,
				Movement: EnterClimb,
				Cost:     EnterClimb.BaseCost(),
			})
		}
	}

	// ExitClimb - exiting from a climbable block onto adjacent platform
	// Only check if we're currently on a climbable and can't climb further up
	isOnClimb := mv.isOnClimbable(from)
	canClimbUp := mv.CanClimb(from, from.Add(models.V3{X: 0, Y: 1, Z: 0}))

	if isOnClimb && !canClimbUp {
		// At top of ladder/vine - check for exits in cardinal directions
		for _, dir := range cardinalDirs {
			to := from.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
			if mv.CanExitClimb(from, to) {
				// Determine correct Y coordinate for the exit
				targetY := mv.getExitClimbTargetY(from, to)
				targetPos := models.V3{X: to.X, Y: targetY, Z: to.Z}

				moves = append(moves, PathStep{
					Position: targetPos,
					Movement: ExitClimb,
					Cost:     ExitClimb.BaseCost(),
				})
			}
		}
	}

	// Swimming (world integration complete)
	for _, dir := range allDirs {
		// Horizontal swim
		to := from.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if mv.CanSwim(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: Swim,
				Cost:     Swim.BaseCost(),
			})
		}

		// Swim up
		toUp := from.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if mv.CanSwimUp(from, toUp) {
			moves = append(moves, PathStep{
				Position: toUp,
				Movement: SwimUp,
				Cost:     SwimUp.BaseCost(),
			})
		}

		// Swim down
		toDown := from.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if mv.CanSwimDown(from, toDown) {
			moves = append(moves, PathStep{
				Position: toDown,
				Movement: SwimDown,
				Cost:     SwimDown.BaseCost(),
			})
		}
	}

	// Exit water - cardinal directions only
	for _, dir := range cardinalDirs {
		// Same level exit
		to := from.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if mv.CanExitWater(from, to) {
			utils.DebugVerbose(mv.logger, "[GetPossibleMoves] adding ExitWater same-level",
				"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", to.X, "toY", to.Y, "toZ", to.Z)
			moves = append(moves, PathStep{
				Position: to,
				Movement: ExitWater,
				Cost:     ExitWater.BaseCost(),
			})
		}

		// Step-up exit (1 block higher)
		toUp := from.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if mv.CanExitWater(from, toUp) {
			utils.DebugVerbose(mv.logger, "[GetPossibleMoves] adding ExitWater step-up",
				"fromX", from.X, "fromY", from.Y, "fromZ", from.Z, "toX", toUp.X, "toY", toUp.Y, "toZ", toUp.Z)
			moves = append(moves, PathStep{
				Position: toUp,
				Movement: ExitWater,
				Cost:     ExitWater.BaseCost() + 0.5, // Slightly more expensive for step-up
			})
		}
	}

	// Prune moves that go too far from goal (if pruning config provided)
	if prune != nil && (goal.X != 0 || goal.Y != 0 || goal.Z != 0) {
		fromDist := from.DistanceTo(goal)
		filtered := make([]PathStep, 0, len(moves))

		// Debug: log first call to understand terrain
		if mv.debugCheckCount < 2 {
			utils.SafeLogger(mv.logger).Debug("[GetPossibleMoves] generated",
				"fromX", from.X, "fromY", from.Y, "fromZ", from.Z,
				"goalX", goal.X, "goalY", goal.Y, "goalZ", goal.Z,
				"dist", fromDist, "moves", len(moves))
		}

		pruned := 0
		for _, move := range moves {
			toDist := move.Position.DistanceTo(goal)
			// Allow moves that get closer, or slightly farther (for obstacle avoidance)
			// Use DriftCap from prune config
			if toDist > fromDist+prune.DriftCap {
				pruned++
				continue
			}

			// Bound cumulative detour from the search's actual start-goal
			// line, not just this one step's local distance-to-goal
			// change. The check above alone is a per-step filter: it only
			// rejects a move that makes THIS node's own distance to goal
			// worse than the node it came from, by more than DriftCap. A
			// long chain of individually-small missteps (each locally
			// "allowed") can still drift arbitrarily far off the direct
			// route before that check ever catches it - confirmed live,
			// a search chasing a goal only ~4 blocks away on one axis
			// wandered across a ~40-block range on that same axis before
			// exhausting its entire step/time budget, on flat,
			// unobstructed terrain with no real reason to detour at all.
			// This is the classic ellipse admissible-region filter: a
			// node contributes at most DriftCap of "extra" path length
			// beyond the direct start-goal distance, which is what
			// DriftCap's own name always implied but StartDist (present
			// since this struct's original design, never actually read
			// anywhere in the codebase) was needed, and missing, to
			// enforce.
			if prune.StartDist > 0 {
				fromStartDist := move.Position.DistanceTo(prune.Start)
				if fromStartDist+toDist > prune.StartDist+prune.DriftCap {
					pruned++
					continue
				}
			}

			filtered = append(filtered, move)
		}

		if mv.debugCheckCount < 2 {
			utils.SafeLogger(mv.logger).Debug("[GetPossibleMoves] after distance pruning", "moves", len(filtered), "pruned", pruned)
			mv.debugCheckCount++
		}

		// Remove duplicates
		seen := make(map[models.V3]bool)
		unique := make([]PathStep, 0, len(filtered))
		dups := 0
		for _, move := range filtered {
			if !seen[move.Position] {
				seen[move.Position] = true
				unique = append(unique, move)
			} else {
				dups++
			}
		}

		if mv.debugCheckCount < 2 {
			utils.SafeLogger(mv.logger).Debug("[GetPossibleMoves] after deduplication", "moves", len(unique), "removed", dups)
		}

		return unique
	}

	return moves
}
