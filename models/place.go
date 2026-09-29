package models

import (
	"context"
	"math"
)

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
// out along each axis, all at the bot's own level; then the same 12 offsets
// one level down. Only geometry: whether a cell is actually free and has a
// floor is for the caller to check.
//
// The lower tier is for a bot standing on top of something rather than the
// floor (a leftover block the chain's cleanup or its own mining missed):
// every neighbour at the bot's own level then looks floorless - its floor
// would be at by-1, which is open air one level above the real, normal
// ground - even though the real ground is right there, one step down and
// well within click reach. Found live: chain_place/chain_use episodes
// occasionally reporting "no free cell" while standing at a fractional Y a
// full block above the usual -60/-61 pair, with every one of the 12
// same-level candidates rejected for want of a floor.
func PlacementCandidates(bx, by, bz int) []PlacementCell {
	offsets := [][2]int{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
		{2, 0}, {-2, 0}, {0, 2}, {0, -2},
	}
	out := make([]PlacementCell, 0, len(offsets)*2)
	for _, dy := range [2]int{0, -1} {
		y := float64(by + dy)
		for _, o := range offsets {
			x, z := float64(bx+o[0]), float64(bz+o[1])
			out = append(out, PlacementCell{
				Cell:  V3{X: x, Y: y, Z: z},
				Floor: V3{X: x, Y: y - 1, Z: z},
			})
		}
	}
	return out
}

// standingTolerance absorbs floating-point drift in a resting bot's feet Y:
// a bot standing on the ground reads -60.0000000001 as often as -60, and
// floor(-60.0000000001) is the floor block's cell, not the bot's.
const standingTolerance = 1e-3

// StandingCell returns the block cell a bot at feet position pos occupies,
// treating a Y within standingTolerance below an integer as that integer.
func StandingCell(pos V3) (x, y, z int) {
	return int(math.Floor(pos.X)), int(math.Floor(pos.Y + standingTolerance)), int(math.Floor(pos.Z))
}
