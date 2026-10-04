package structure

// Pos is an integer block coordinate, local to a Structure's own origin
// (0,0,0) - not a world position. A plain int triple rather than models.V3
// (which is float64, a world/physics position) because these are exact
// grid cells read straight out of the file, and callers that need a world
// position add a float64 origin to this later (see BuildStructure,
// docs/STRUCTURE_LOADER.md).
type Pos struct {
	X, Y, Z int
}

// Add returns p+o.
func (p Pos) Add(o Pos) Pos {
	return Pos{X: p.X + o.X, Y: p.Y + o.Y, Z: p.Z + o.Z}
}

// PaletteEntry is one block type a Structure's blocks can reference: a
// block name (e.g. "minecraft:oak_stairs") and its block-state properties
// (e.g. {"facing": "north", "half": "bottom"}). Properties is read from the
// source file but not currently used for placement - see "Known
// limitations" in docs/STRUCTURE_LOADER.md for why and what using it would
// take.
type PaletteEntry struct {
	Name       string
	Properties map[string]string
}

// BlockEntry is one block placement within a Structure: a local position
// and an index into the owning Structure's Palette. BlockEntityData, when
// non-nil, is that cell's block-entity NBT (chest contents, sign text,
// etc.) - captured because the source file has it, but unused by v1 of the
// placement pipeline (see Non-goals in the plan doc).
type BlockEntry struct {
	Pos             Pos
	PaletteIndex    int
	BlockEntityData *Tag
}

// Structure is the format-agnostic result of decoding any supported
// template file. Every Format implementation (vanilla.go today; a future
// litematica.go/sponge.go) produces one of these, and everything downstream
// - PlacementOrder, and the agent's BuildStructure - is written against
// this type only, never against a specific file format.
type Structure struct {
	Size    Pos
	Palette []PaletteEntry
	Blocks  []BlockEntry
}

// Block returns the PaletteEntry a BlockEntry refers to, and false if its
// PaletteIndex is out of range (a malformed source file).
func (s *Structure) Block(b BlockEntry) (PaletteEntry, bool) {
	if b.PaletteIndex < 0 || b.PaletteIndex >= len(s.Palette) {
		return PaletteEntry{}, false
	}
	return s.Palette[b.PaletteIndex], true
}
