package world

import "testing"

// buildSingleValueSection encodes one section (pre-1.21.5 wire shape:
// explicit trailing data-array-length VarInts, both 0 for a single-valued
// palette) whose every block is blockState and every biome is biomeID.
// Mirrors parseSection/parsePaletteContainer's expected byte layout
// exactly, so a mistake here would show up as a mismatch against the
// production parser, not just against itself.
func buildSingleValueSection(blockState, biomeID byte) []byte {
	return []byte{
		0, 0, // blockCount (int16, unused by GetBlockAt)
		0, blockState, 0, // block palette: bpe=0, singleValue=blockState, dataArrayLen=0
		0, biomeID, 0, // biome palette: bpe=0, singleValue=biomeID, dataArrayLen=0
	}
}

// buildRawChunk concatenates sectionCount single-valued sections, section i
// carrying block state byte(i+1) (1-based so section 0's state, 1, is
// distinguishable from air/0) — this lets a test assert that a given world
// Y resolves to exactly the section its index predicts, catching any
// off-by-one-section desync in the skip/parse logic.
func buildRawChunk(sectionCount int) []byte {
	var raw []byte
	for i := 0; i < sectionCount; i++ {
		raw = append(raw, buildSingleValueSection(byte(i+1), 0)...)
	}
	return raw
}

// TestChunkData_GetBlockAt_NetherShapedColumn is the regression test for
// the exact bug class this project's dimension-height work exists to
// prevent: before MinSectionY/SectionCount were per-ChunkData fields,
// loadSection's "skip N sections" logic always assumed the Overworld's
// min_y=-64 (24 sections), so a chunk column shaped like the Nether's
// min_y=0/height=256 (16 sections) would desync the whole section stream
// rather than just misreading one block.
func TestChunkData_GetBlockAt_NetherShapedColumn(t *testing.T) {
	const sectionCount = 16 // Nether/End: min_y=0, height=256
	chunk := &ChunkData{
		RawData:      buildRawChunk(sectionCount),
		MinSectionY:  0,
		SectionCount: int32(sectionCount),
	}

	// Section 0 covers Y 0-15, section 1 covers Y 16-31, ..., section 15
	// (the last) covers Y 240-255 - the top of the Nether/End's build
	// height. Each should read back exactly the state this section's
	// index encoded, confirming the skip count for section i is i, not
	// i-MIN_SECTION_Y (which would have been 4+i under the old hardcoded
	// Overworld assumption and desynced entirely).
	for i := 0; i < sectionCount; i++ {
		y := i * 16
		want := uint32(i + 1)
		if got := chunk.GetBlockAt(0, y, 0); got != want {
			t.Errorf("GetBlockAt(0, %d, 0) [section %d] = %d, want %d", y, i, got, want)
		}
	}

	// One block below the column and one above it should both read as
	// air (out of bounds), not wrap into an adjacent section.
	if got := chunk.GetBlockAt(0, -1, 0); got != 0 {
		t.Errorf("GetBlockAt(0, -1, 0) = %d, want 0 (below Nether-shaped column)", got)
	}
	if got := chunk.GetBlockAt(0, 256, 0); got != 0 {
		t.Errorf("GetBlockAt(0, 256, 0) = %d, want 0 (above Nether-shaped column)", got)
	}
}

// TestChunkData_GetBlockAt_OverworldDefault locks in that a ChunkData
// built without explicitly setting MinSectionY/SectionCount (e.g. an
// existing test, or any production code path not yet updated) still
// behaves exactly like the Overworld's hardcoded -64..319/24-section shape
// did before this field existed at all.
func TestChunkData_GetBlockAt_OverworldDefault(t *testing.T) {
	const sectionCount = 24 // Overworld: min_y=-64, height=384
	chunk := &ChunkData{
		RawData: buildRawChunk(sectionCount),
		// MinSectionY/SectionCount deliberately left zero.
	}

	// World Y -64 is the Overworld's lowest block, in section index -4
	// (slot 0 of 24); world Y 304 is in the top section (slot 23).
	if got := chunk.GetBlockAt(0, -64, 0); got != 1 {
		t.Errorf("GetBlockAt(0, -64, 0) = %d, want 1 (first section)", got)
	}
	if got := chunk.GetBlockAt(0, 304, 0); got != 24 {
		t.Errorf("GetBlockAt(0, 304, 0) = %d, want 24 (last section)", got)
	}
	if got := chunk.GetBlockAt(0, -65, 0); got != 0 {
		t.Errorf("GetBlockAt(0, -65, 0) = %d, want 0 (below Overworld build height)", got)
	}
}
