package structure

import "testing"

// simpleHutPath is a real structure-block .nbt file, saved by hand from a
// real Minecraft client (not one of this package's own hand-built test
// fixtures) - a small 7x7x7 wooden hut. Loading it caught a real bug the
// synthetic fixtures elsewhere in this package never could: this file's
// palette entries use lowercase "id"/"properties" (DataVersion 5023, newer
// than anything else named in this codebase's handler_versions), not the
// capitalized "Name"/"Properties" this package's decoder originally assumed
// exclusively - see decodePalette's own comment for why both are accepted
// now. Kept as a permanent regression test for exactly that: a decoder
// that only worked against the shape of data it was written to produce
// isn't actually proven against the shape of data Minecraft itself emits.
const simpleHutPath = "testdata/generated/reallyoldfogie/structure/simple_hut.nbt"

func TestLoadFile_RealClientGeneratedStructure(t *testing.T) {
	s, err := LoadFile(simpleHutPath)
	if err != nil {
		t.Fatalf("LoadFile(%s): %v", simpleHutPath, err)
	}

	if want := (Pos{X: 7, Y: 7, Z: 7}); s.Size != want {
		t.Errorf("Size = %+v, want %+v", s.Size, want)
	}
	if len(s.Palette) != 24 {
		t.Fatalf("len(Palette) = %d, want 24", len(s.Palette))
	}
	if len(s.Blocks) != 343 {
		t.Fatalf("len(Blocks) = %d, want 343 (7*7*7)", len(s.Blocks))
	}

	// A plain entry (no Properties at all - exercises the "no properties
	// tag present" path, not just "present but empty").
	if s.Palette[3].Name != "minecraft:oak_planks" {
		t.Errorf("Palette[3].Name = %q, want minecraft:oak_planks", s.Palette[3].Name)
	}
	if s.Palette[3].Properties != nil {
		t.Errorf("Palette[3].Properties = %+v, want nil", s.Palette[3].Properties)
	}

	// The richest single entry in this file - exercises decoding a
	// multi-key Properties compound (lowercase "properties" in the real
	// file) with several distinct value types all as strings.
	door := s.Palette[6]
	if door.Name != "minecraft:spruce_door" {
		t.Fatalf("Palette[6].Name = %q, want minecraft:spruce_door", door.Name)
	}
	wantDoorProps := map[string]string{
		"facing": "west", "half": "upper", "hinge": "left", "open": "false", "powered": "false",
	}
	for k, want := range wantDoorProps {
		if got := door.Properties[k]; got != want {
			t.Errorf("spruce_door.Properties[%q] = %q, want %q", k, got, want)
		}
	}

	// Block-type counts across the whole file, resolved through
	// Structure.Block (palette index -> entry), not just raw palette
	// length - exercises the full pos/state -> resolved-name path at
	// realistic volume (343 blocks, 24 palette entries, lots of reuse).
	counts := map[string]int{}
	for _, b := range s.Blocks {
		entry, ok := s.Block(b)
		if !ok {
			t.Fatalf("Block(%+v): palette index %d out of range", b, b.PaletteIndex)
		}
		counts[entry.Name]++
	}
	wantCounts := map[string]int{
		"minecraft:air":         218,
		"minecraft:oak_log":     63,
		"minecraft:oak_stairs":  55,
		"minecraft:tall_grass":  4,
		"minecraft:oak_planks":  2,
		"minecraft:spruce_door": 1,
	}
	for name, want := range wantCounts {
		if got := counts[name]; got != want {
			t.Errorf("block count for %s = %d, want %d", name, got, want)
		}
	}

	// PlacementOrder must drop every air block (218) and nothing else,
	// and the result must be sorted bottom-up (non-decreasing Y) - a
	// property that's easy to get right on this package's own small
	// synthetic fixtures by accident and much more convincing to confirm
	// against a real, structurally complex file.
	order := PlacementOrder(s)
	wantPlaceable := len(s.Blocks) - wantCounts["minecraft:air"]
	if len(order) != wantPlaceable {
		t.Fatalf("len(PlacementOrder) = %d, want %d (343 total - 218 air)", len(order), wantPlaceable)
	}
	for i := 1; i < len(order); i++ {
		if order[i].Pos.Y < order[i-1].Pos.Y {
			t.Fatalf("PlacementOrder not bottom-up at index %d: Y %d follows Y %d", i, order[i].Pos.Y, order[i-1].Pos.Y)
		}
	}
}
