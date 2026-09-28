package models

import "context"

// BlockPlacer is the optional capability of placing a block the bot holds
// on the ground next to it and confirming it appeared. Callers type-assert
// for it; agents without it cannot place.
type BlockPlacer interface {
	// PlaceHeldBlock places one itemName (a block item, whose block name
	// is the same, e.g. minecraft:crafting_table) on a standable cell beside
	// the bot and returns the cell it now occupies. It verifies in the
	// bot's own world view that the block appeared and retries (another
	// attempt, then another cell) before failing: a placement request that
	// the server silently ignores is a real, observed outcome.
	PlaceHeldBlock(ctx context.Context, itemName string) (V3, error)
}

// PlacementCell is a candidate cell for placing a block: Cell is where the
// block goes, Floor the block it is placed against (the face clicked is the
// floor's top).
type PlacementCell struct {
	Cell, Floor V3
}

// PlacementCandidates lists the cells around a bot standing in block cell
// (bx, by, bz) that a block could be placed in, nearest first: the four
// orthogonal neighbours, then the four diagonals, then the cells two blocks
// out along each axis. Only geometry: whether a cell is actually free and
// has a floor is for the caller to check.
func PlacementCandidates(bx, by, bz int) []PlacementCell {
	offsets := [][2]int{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
		{2, 0}, {-2, 0}, {0, 2}, {0, -2},
	}
	out := make([]PlacementCell, 0, len(offsets))
	for _, o := range offsets {
		x, z := float64(bx+o[0]), float64(bz+o[1])
		out = append(out, PlacementCell{
			Cell:  V3{X: x, Y: float64(by), Z: z},
			Floor: V3{X: x, Y: float64(by - 1), Z: z},
		})
	}
	return out
}
