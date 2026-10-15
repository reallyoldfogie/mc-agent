package pathfinding_test

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/require"
)

// undiggableOverride wraps MockShapeManager so a single test can mark one
// specific block state as solid-but-undiggable (bedrock-like) - the shared
// mock's own GetMiningInfo always reports diggable=true for any non-air
// state, which can't exercise mineObstructionAt's undiggable gate on its
// own.
type undiggableOverride struct {
	*mctesting.MockShapeManager
	undiggableStateID uint32
}

func (u *undiggableOverride) IsPassable(stateID uint32) bool {
	if stateID == u.undiggableStateID {
		return false
	}
	return u.MockShapeManager.IsPassable(stateID)
}

func (u *undiggableOverride) IsSolid(stateID uint32) bool {
	if stateID == u.undiggableStateID {
		return true
	}
	return u.MockShapeManager.IsSolid(stateID)
}

func (u *undiggableOverride) GetMiningInfo(stateID uint32) (float64, []string, bool) {
	if stateID == u.undiggableStateID {
		return -1, nil, false
	}
	return u.MockShapeManager.GetMiningInfo(stateID)
}

func findMove(moves []models.PathStep, movement pathfinding.MovementType, pos models.V3) *models.PathStep {
	for i := range moves {
		if moves[i].Movement == movement && moves[i].Position == pos {
			return &moves[i]
		}
	}
	return nil
}

func hasMoveTo(moves []models.PathStep, pos models.V3) bool {
	for _, m := range moves {
		if m.Position == pos {
			return true
		}
	}
	return false
}

// TestGetPossibleMoves_OffersMineDownThroughTheFloor is a regression test
// for docs/plans/14's narrow-shaft-navigability finding: a bot that has
// dug itself into a straight 1x1 vertical shaft has no adjacent column left
// for any existing per-direction Ascend/Descend check to offer - this is
// the move (MineDown) that lets it keep going. The bot stands on solid,
// diggable stone; MineDown should target that exact block, carry it as its
// only MineTarget, and cost the pseudo-expensive flat premium plus the
// block's hardness scaling (mock hardness is 1.5 for any non-air block).
func TestGetPossibleMoves_OffersMineDownThroughTheFloor(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	stoneID := registry.GetStateID("minecraft:stone", nil)

	world := mctesting.NewWorldBuilder(registry).
		SetBlockDirect(0, 0, 0, stoneID). // the floor the bot stands on - also what MineDown must target
		Build()
	shapeMgr := mctesting.NewMockShapeManager()
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	from := models.V3{X: 0, Y: 1, Z: 0} // feet at Y=1, standing on the stone floor at Y=0
	moves := mv.GetPossibleMoves(from, models.V3{}, nil)

	target := models.V3{X: 0, Y: 0, Z: 0}
	mineDown := findMove(moves, pathfinding.MineDown, target)
	require.NotNil(t, mineDown, "expected a MineDown move to %v, got moves=%v", target, moves)
	require.Equal(t, []models.V3{target}, mineDown.MineTargets)
	require.InDelta(t, 8.0+1.5*2.0+6.0, mineDown.Cost, 0.001,
		"MineDown cost should be the flat premium plus hardness scaling plus the straight-down risk surcharge, not BaseCost()'s representative 8.0 alone")
}

// TestGetPossibleMoves_DoesNotOfferMineDownOverOpenFloor confirms MineDown
// is never added redundantly when the floor is already open - it's a last-
// resort route for an obstruction, not an alternative to a normal fall.
func TestGetPossibleMoves_DoesNotOfferMineDownOverOpenFloor(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	grassID := registry.GetStateID("minecraft:grass_block", nil)

	// Solid ground two blocks down (a normal 1-block step/drop), nothing
	// solid directly underfoot.
	world := mctesting.NewWorldBuilder(registry).
		SetBlockDirect(0, -1, 0, grassID).
		Build()
	shapeMgr := mctesting.NewMockShapeManager()
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	from := models.V3{X: 0, Y: 1, Z: 0}
	moves := mv.GetPossibleMoves(from, models.V3{}, nil)

	for _, m := range moves {
		require.NotEqual(t, pathfinding.MineDown, m.Movement, "MineDown should never be offered when the floor is already open: %v", m)
	}
}

// TestGetPossibleMoves_OffersMineDownDiagonalCheaperThanMineDown is a
// regression test for the operator's 2026-10-14 safety direction: mining
// straight down through the exact block underfoot commits to whatever it
// reveals (a cave, lava) with no fallback, so the safer MineDownDiagonal -
// which lands in an *adjacent* column instead, never the one the bot is
// currently standing on - must be strictly cheaper, not just an equally
// weighted alternative. Sets up both routes at once (stone directly
// underfoot for MineDown, stone one block east-and-down for
// MineDownDiagonal, with open headroom there so only the landing cell
// itself needs breaking) and checks the ordering directly, not just that
// both exist.
func TestGetPossibleMoves_OffersMineDownDiagonalCheaperThanMineDown(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	stoneID := registry.GetStateID("minecraft:stone", nil)

	world := mctesting.NewWorldBuilder(registry).
		SetBlockDirect(0, 0, 0, stoneID). // straight-down floor (MineDown's target)
		SetBlockDirect(1, 0, 0, stoneID). // diagonal landing, east (MineDownDiagonal's target)
		Build()
	shapeMgr := mctesting.NewMockShapeManager()
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	from := models.V3{X: 0, Y: 1, Z: 0}
	moves := mv.GetPossibleMoves(from, models.V3{}, nil)

	mineDown := findMove(moves, pathfinding.MineDown, models.V3{X: 0, Y: 0, Z: 0})
	mineDownDiagonal := findMove(moves, pathfinding.MineDownDiagonal, models.V3{X: 1, Y: 0, Z: 0})
	require.NotNil(t, mineDown, "expected MineDown, got moves=%v", moves)
	require.NotNil(t, mineDownDiagonal, "expected MineDownDiagonal, got moves=%v", moves)

	require.Equal(t, []models.V3{{X: 1, Y: 0, Z: 0}}, mineDownDiagonal.MineTargets,
		"headroom at (1,1,0) is open by default, so only the landing cell itself should need breaking")
	require.InDelta(t, 8.0+1.5*2.0, mineDownDiagonal.Cost, 0.001)
	require.Greater(t, mineDown.Cost, mineDownDiagonal.Cost,
		"MineDown must be strictly more expensive than MineDownDiagonal so a search prefers the safer route by default")
}

// TestGetPossibleMoves_DoesNotOfferMineDownDiagonalWhenNormalDropExists
// confirms MineDownDiagonal, like every other Mine* move, is only offered
// as a fallback - never redundantly alongside an already-available ordinary
// drop.
func TestGetPossibleMoves_DoesNotOfferMineDownDiagonalWhenNormalDropExists(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	grassID := registry.GetStateID("minecraft:grass_block", nil)

	// Open 1-block drop to the east (nothing solid at the landing or above
	// it) with real ground support two levels down.
	world := mctesting.NewWorldBuilder(registry).
		SetBlockDirect(1, -1, 0, grassID).
		Build()
	shapeMgr := mctesting.NewMockShapeManager()
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	from := models.V3{X: 0, Y: 1, Z: 0}
	moves := mv.GetPossibleMoves(from, models.V3{}, nil)

	for _, m := range moves {
		require.NotEqual(t, pathfinding.MineDownDiagonal, m.Movement, "MineDownDiagonal should never be offered when an ordinary drop already exists: %v", m)
	}
	require.NotNil(t, findMove(moves, pathfinding.Descend, models.V3{X: 1, Y: 0, Z: 0}), "the ordinary drop itself should still be offered")
}

// TestGetPossibleMoves_UndiggableBlockOffersNoMineDownDiagonalRoute is
// MineDownDiagonal's counterpart to the plain-obstruction undiggable test
// above: a bedrock-like landing cell must block the route entirely, not
// just skip mining it.
func TestGetPossibleMoves_UndiggableBlockOffersNoMineDownDiagonalRoute(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	bedrockID := registry.GetStateID("minecraft:bedrock", nil)

	world := mctesting.NewWorldBuilder(registry).
		SetBlockDirect(1, 0, 0, bedrockID). // diagonal landing, east
		Build()
	shapeMgr := &undiggableOverride{MockShapeManager: mctesting.NewMockShapeManager(), undiggableStateID: bedrockID}
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	from := models.V3{X: 0, Y: 1, Z: 0}
	moves := mv.GetPossibleMoves(from, models.V3{}, nil)

	require.False(t, hasMoveTo(moves, models.V3{X: 1, Y: 0, Z: 0}), "an undiggable landing cell must offer no route at all, got moves=%v", moves)
}

// TestGetPossibleMoves_OffersMineThroughWhenWalledIn covers MineThrough:
// every cardinal direction is blocked by solid, diggable stone at both
// feet and head level, with normal ground support beyond each wall. Each
// direction should get exactly one MineThrough move with both cells as
// MineTargets (double the single-block cost), and no Traverse move.
func TestGetPossibleMoves_OffersMineThroughWhenWalledIn(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	stoneID := registry.GetStateID("minecraft:stone", nil)
	grassID := registry.GetStateID("minecraft:grass_block", nil)

	wb := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-3, -3, 3, 3, -1, grassID) // wide ground-support floor one level down
	dirs := []struct{ dx, dz float64 }{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, d := range dirs {
		wb.SetBlockDirect(d.dx, 0, d.dz, stoneID) // feet-level wall
		wb.SetBlockDirect(d.dx, 1, d.dz, stoneID) // head-level wall
	}
	world := wb.Build()
	shapeMgr := mctesting.NewMockShapeManager()
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	from := models.V3{X: 0, Y: 0, Z: 0}
	moves := mv.GetPossibleMoves(from, models.V3{}, nil)

	for _, d := range dirs {
		to := models.V3{X: d.dx, Y: 0, Z: d.dz}
		mineThrough := findMove(moves, pathfinding.MineThrough, to)
		require.NotNil(t, mineThrough, "expected a MineThrough move to %v, got moves=%v", to, moves)
		require.ElementsMatch(t, []models.V3{to, {X: d.dx, Y: 1, Z: d.dz}}, mineThrough.MineTargets)
		require.InDelta(t, 2*(8.0+1.5*2.0), mineThrough.Cost, 0.001, "two blocked cells should cost double the single-block premium")
		require.Nil(t, findMove(moves, pathfinding.Traverse, to), "Traverse must not also be offered where the way is walled off")
	}
}

// TestGetPossibleMoves_UndiggableBlockOffersNoRouteAtAll confirms an
// undiggable obstruction (bedrock-like - solid, but GetMiningInfo reports
// diggable=false) blocks the move entirely rather than being silently
// skipped as "just don't mine it": mineObstructionAt's gate exists so a
// solid-but-impossible-to-mine cell never gets offered as a route by
// MineThrough/MineDown at all, the same way it already blocks Traverse.
func TestGetPossibleMoves_UndiggableBlockOffersNoRouteAtAll(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	bedrockID := registry.GetStateID("minecraft:bedrock", nil)

	world := mctesting.NewWorldBuilder(registry).
		SetBlockDirect(1, 0, 0, bedrockID). // feet-level, +X direction
		SetBlockDirect(1, 1, 0, bedrockID). // head-level, +X direction
		Build()
	shapeMgr := &undiggableOverride{MockShapeManager: mctesting.NewMockShapeManager(), undiggableStateID: bedrockID}
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	from := models.V3{X: 0, Y: 0, Z: 0}
	moves := mv.GetPossibleMoves(from, models.V3{}, nil)

	blocked := models.V3{X: 1, Y: 0, Z: 0}
	require.False(t, hasMoveTo(moves, blocked), "an undiggable obstruction must offer no route at all (Traverse or MineThrough) toward it, got moves=%v", moves)
}
