package structure

import "sort"

// skipPlacement is the set of palette block names PlacementOrder omits:
// real, common palette entries (every structure file that wasn't saved with
// "Include entities/air" disabled is full of them, marking the bounding
// box's empty cells) that place nothing and would otherwise either fail
// placement (nothing to equip) or, worse, be indistinguishable from "no
// data here" if ever misused as a delete/clear instruction.
var skipPlacement = map[string]bool{
	"minecraft:air":            true,
	"minecraft:structure_void": true,
}

// PlacementOrder returns s.Blocks filtered to real (non-air, non-void)
// blocks and sorted bottom-up: ascending Y first, so nothing is ever asked
// to place before the cell it will rest against exists, then a stable
// secondary order (Z, then X) purely for determinism - two runs over the
// same Structure always produce the same build order, which matters for
// reasoning about a failed/partial build and for tests.
//
// This is deliberately simple (a total order by position, not a real
// dependency graph): a structure with a genuine overhang - a block whose
// only potential support is another block at the *same* Y as itself, placed
// later in X/Z order - can still fail to find a support face when its turn
// comes. BuildStructure (docs/STRUCTURE_LOADER.md) reports that as a
// per-block placement failure and continues, rather than this function
// trying to topologically sort arbitrary structures.
func PlacementOrder(s *Structure) []BlockEntry {
	out := make([]BlockEntry, 0, len(s.Blocks))
	for _, b := range s.Blocks {
		entry, ok := s.Block(b)
		if !ok || skipPlacement[entry.Name] {
			continue
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Pos, out[j].Pos
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		if a.Z != b.Z {
			return a.Z < b.Z
		}
		return a.X < b.X
	})
	return out
}
