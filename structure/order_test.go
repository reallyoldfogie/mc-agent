package structure

import "testing"

func TestPlacementOrder_FiltersAirAndVoid(t *testing.T) {
	s := &Structure{
		Palette: []PaletteEntry{
			{Name: "minecraft:air"},            // 0
			{Name: "minecraft:stone"},          // 1
			{Name: "minecraft:structure_void"}, // 2
		},
		Blocks: []BlockEntry{
			{Pos: Pos{0, 0, 0}, PaletteIndex: 0},
			{Pos: Pos{1, 0, 0}, PaletteIndex: 1},
			{Pos: Pos{2, 0, 0}, PaletteIndex: 2},
		},
	}
	out := PlacementOrder(s)
	if len(out) != 1 {
		t.Fatalf("len(PlacementOrder) = %d, want 1 (air and structure_void filtered)", len(out))
	}
	if out[0].Pos != (Pos{1, 0, 0}) {
		t.Errorf("surviving block Pos = %+v, want {1 0 0}", out[0].Pos)
	}
}

func TestPlacementOrder_BottomUpDeterministic(t *testing.T) {
	stone := PaletteEntry{Name: "minecraft:stone"}
	s := &Structure{
		Palette: []PaletteEntry{stone},
		Blocks: []BlockEntry{
			{Pos: Pos{X: 1, Y: 2, Z: 0}, PaletteIndex: 0},
			{Pos: Pos{X: 0, Y: 0, Z: 1}, PaletteIndex: 0},
			{Pos: Pos{X: 0, Y: 0, Z: 0}, PaletteIndex: 0},
			{Pos: Pos{X: 1, Y: 0, Z: 0}, PaletteIndex: 0},
			{Pos: Pos{X: 0, Y: 1, Z: 0}, PaletteIndex: 0},
		},
	}
	want := []Pos{
		{X: 0, Y: 0, Z: 0},
		{X: 1, Y: 0, Z: 0},
		{X: 0, Y: 0, Z: 1},
		{X: 0, Y: 1, Z: 0},
		{X: 1, Y: 2, Z: 0},
	}
	for run := 0; run < 3; run++ {
		out := PlacementOrder(s)
		if len(out) != len(want) {
			t.Fatalf("run %d: len(out) = %d, want %d", run, len(out), len(want))
		}
		for i, b := range out {
			if b.Pos != want[i] {
				t.Errorf("run %d: out[%d].Pos = %+v, want %+v", run, i, b.Pos, want[i])
			}
		}
	}
}

func TestPlacementOrder_EmptyStructure(t *testing.T) {
	s := &Structure{Palette: []PaletteEntry{{Name: "minecraft:air"}}}
	if out := PlacementOrder(s); len(out) != 0 {
		t.Fatalf("PlacementOrder on an empty structure = %v, want empty", out)
	}
}
