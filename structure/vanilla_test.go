package structure

import (
	"os"
	"testing"
)

// fixtureStructureTag builds a small but representative vanilla structure
// root: a 2x2x1 slab with a stone base, an oak_stairs on top (exercising
// Properties), a chest with block-entity data (exercising the optional
// "nbt" field), and an air cell (which PlacementOrder, tested separately,
// is responsible for dropping - vanillaFormat itself keeps it, since
// filtering is an ordering concern, not a decode concern).
func fixtureStructureTag() Tag {
	palette := listTag(TagCompound,
		compoundTag(map[string]Tag{ // index 0
			"Name": stringTag("minecraft:stone"),
		}),
		compoundTag(map[string]Tag{ // index 1
			"Name": stringTag("minecraft:oak_stairs"),
			"Properties": compoundTag(map[string]Tag{
				"facing": stringTag("north"),
				"half":   stringTag("bottom"),
			}),
		}),
		compoundTag(map[string]Tag{ // index 2
			"Name": stringTag("minecraft:air"),
		}),
		compoundTag(map[string]Tag{ // index 3
			"Name": stringTag("minecraft:chest"),
		}),
	)

	blocks := listTag(TagCompound,
		compoundTag(map[string]Tag{
			"pos":   listTag(TagInt, intTag(0), intTag(0), intTag(0)),
			"state": intTag(0),
		}),
		compoundTag(map[string]Tag{
			"pos":   listTag(TagInt, intTag(1), intTag(0), intTag(0)),
			"state": intTag(1),
		}),
		compoundTag(map[string]Tag{
			"pos":   listTag(TagInt, intTag(0), intTag(1), intTag(0)),
			"state": intTag(2),
		}),
		compoundTag(map[string]Tag{
			"pos":   listTag(TagInt, intTag(1), intTag(0), intTag(1)),
			"state": intTag(3),
			"nbt": compoundTag(map[string]Tag{
				"id": stringTag("minecraft:chest"),
			}),
		}),
	)

	return compoundTag(map[string]Tag{
		"DataVersion": intTag(3955),
		"size":        listTag(TagInt, intTag(2), intTag(2), intTag(2)),
		"palette":     palette,
		"blocks":      blocks,
		"entities":    listTag(TagCompound),
	})
}

func TestVanillaFormat_Sniff(t *testing.T) {
	f := vanillaFormat{}
	if !f.Sniff(fixtureStructureTag()) {
		t.Fatal("Sniff should recognize a well-formed vanilla structure root")
	}

	cases := map[string]Tag{
		"missing size":    compoundTag(map[string]Tag{"palette": listTag(TagCompound), "blocks": listTag(TagCompound)}),
		"missing palette": compoundTag(map[string]Tag{"size": listTag(TagInt), "blocks": listTag(TagCompound)}),
		"missing blocks":  compoundTag(map[string]Tag{"size": listTag(TagInt), "palette": listTag(TagCompound)}),
		"size wrong type": compoundTag(map[string]Tag{"size": intTag(1), "palette": listTag(TagCompound), "blocks": listTag(TagCompound)}),
		"litematica-shaped": compoundTag(map[string]Tag{
			"Regions":              compoundTag(map[string]Tag{}),
			"MinecraftDataVersion": intTag(3955),
		}),
	}
	for name, root := range cases {
		if f.Sniff(root) {
			t.Errorf("Sniff(%s): expected false, got true", name)
		}
	}
}

func TestVanillaFormat_Decode(t *testing.T) {
	s, err := vanillaFormat{}.Decode(fixtureStructureTag())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if s.Size != (Pos{X: 2, Y: 2, Z: 2}) {
		t.Errorf("Size = %+v, want {2 2 2}", s.Size)
	}
	if len(s.Palette) != 4 {
		t.Fatalf("len(Palette) = %d, want 4", len(s.Palette))
	}
	if s.Palette[1].Name != "minecraft:oak_stairs" {
		t.Errorf("Palette[1].Name = %q, want minecraft:oak_stairs", s.Palette[1].Name)
	}
	if got := s.Palette[1].Properties["facing"]; got != "north" {
		t.Errorf("Palette[1].Properties[facing] = %q, want north", got)
	}
	if got := s.Palette[1].Properties["half"]; got != "bottom" {
		t.Errorf("Palette[1].Properties[half] = %q, want bottom", got)
	}
	if s.Palette[0].Properties != nil {
		t.Errorf("Palette[0].Properties = %+v, want nil (no Properties tag in fixture)", s.Palette[0].Properties)
	}

	if len(s.Blocks) != 4 {
		t.Fatalf("len(Blocks) = %d, want 4", len(s.Blocks))
	}
	chest := s.Blocks[3]
	if chest.Pos != (Pos{X: 1, Y: 0, Z: 1}) {
		t.Errorf("chest Pos = %+v, want {1 0 1}", chest.Pos)
	}
	if chest.BlockEntityData == nil {
		t.Fatal("chest.BlockEntityData = nil, want the captured nbt compound")
	}
	idTag, ok := chest.BlockEntityData.Get("id")
	if !ok || idTag.Str != "minecraft:chest" {
		t.Errorf("chest.BlockEntityData[id] = %+v, want minecraft:chest", idTag)
	}

	plain := s.Blocks[0]
	if plain.BlockEntityData != nil {
		t.Errorf("Blocks[0].BlockEntityData = %+v, want nil (fixture has no nbt for it)", plain.BlockEntityData)
	}

	entry, ok := s.Block(chest)
	if !ok || entry.Name != "minecraft:chest" {
		t.Errorf("Block(chest) = %+v, %v, want minecraft:chest palette entry", entry, ok)
	}
}

func TestVanillaFormat_Decode_Errors(t *testing.T) {
	cases := map[string]Tag{
		"missing size": compoundTag(map[string]Tag{
			"palette": listTag(TagCompound),
			"blocks":  listTag(TagCompound),
		}),
		"missing palette": compoundTag(map[string]Tag{
			"size":   listTag(TagInt, intTag(1), intTag(1), intTag(1)),
			"blocks": listTag(TagCompound),
		}),
		"missing blocks": compoundTag(map[string]Tag{
			"size":    listTag(TagInt, intTag(1), intTag(1), intTag(1)),
			"palette": listTag(TagCompound),
		}),
		"size wrong length": compoundTag(map[string]Tag{
			"size":    listTag(TagInt, intTag(1), intTag(1)),
			"palette": listTag(TagCompound),
			"blocks":  listTag(TagCompound),
		}),
		"palette entry missing Name": compoundTag(map[string]Tag{
			"size":    listTag(TagInt, intTag(1), intTag(1), intTag(1)),
			"palette": listTag(TagCompound, compoundTag(map[string]Tag{})),
			"blocks":  listTag(TagCompound),
		}),
		"block state out of range": compoundTag(map[string]Tag{
			"size": listTag(TagInt, intTag(1), intTag(1), intTag(1)),
			"palette": listTag(TagCompound, compoundTag(map[string]Tag{
				"Name": stringTag("minecraft:stone"),
			})),
			"blocks": listTag(TagCompound, compoundTag(map[string]Tag{
				"pos":   listTag(TagInt, intTag(0), intTag(0), intTag(0)),
				"state": intTag(5), // only index 0 exists
			})),
		}),
		"block missing pos": compoundTag(map[string]Tag{
			"size": listTag(TagInt, intTag(1), intTag(1), intTag(1)),
			"palette": listTag(TagCompound, compoundTag(map[string]Tag{
				"Name": stringTag("minecraft:stone"),
			})),
			"blocks": listTag(TagCompound, compoundTag(map[string]Tag{
				"state": intTag(0),
			})),
		}),
	}
	f := vanillaFormat{}
	for name, root := range cases {
		if _, err := f.Decode(root); err == nil {
			t.Errorf("Decode(%s): expected an error, got none", name)
		}
	}
}

// TestLoadFile_Vanilla_EndToEnd exercises the full path: bytes on disk
// (gzip-wrapped, like a real Structure Block save) -> DecodeNBT ->
// vanillaFormat.Decode -> Structure, via the public LoadFile entry point
// rather than calling internals directly.
func TestLoadFile_Vanilla_EndToEnd(t *testing.T) {
	data := gzipBytes(encodeRootCompound(fixtureStructureTag()))
	path := t.TempDir() + "/fixture.nbt"
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if s.Size != (Pos{X: 2, Y: 2, Z: 2}) {
		t.Errorf("Size = %+v, want {2 2 2}", s.Size)
	}
	if len(s.Blocks) != 4 {
		t.Errorf("len(Blocks) = %d, want 4", len(s.Blocks))
	}
}
