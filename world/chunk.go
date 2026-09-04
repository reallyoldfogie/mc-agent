package world

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"sync"
)

const (
	// BLOCK_SECTION_SIZE is the number of blocks in a section (16x16x16)
	BLOCK_SECTION_SIZE = 4096
	// BIOME_SECTION_SIZE is the number of biome entries in a section (4x4x4)
	BIOME_SECTION_SIZE = 64
	// SECTION_HEIGHT is the height of a section in blocks
	SECTION_HEIGHT = 16
	// SECTION_WIDTH is the width/depth of a section in blocks
	SECTION_WIDTH = 16
	// MIN_SECTION_Y is the minimum section Y index (for Y=-64)
	MIN_SECTION_Y = -4
	// MAX_SECTION_Y is the maximum section Y index (for Y=319)
	MAX_SECTION_Y = 19
	// SECTION_COUNT is the total number of sections in a chunk
	SECTION_COUNT = MAX_SECTION_Y - MIN_SECTION_Y + 1 // 24 sections
)

// ChunkData stores the data for a single chunk column.
type ChunkData struct {
	X, Z    int32
	RawData []byte // Raw ChunkData from MapChunk packet

	// UseCalculatedDataLen indicates whether to calculate data array length
	// from bits-per-entry (1.21.5+) or read it as a VarInt (pre-1.21.5).
	// In 1.21.5+, the data array length is not sent in the packet.
	UseCalculatedDataLen bool

	// HasFluidCount indicates whether each section carries a second short
	// (fluidCount) after nonEmptyBlockCount, before the block states
	// container. True for 26.1+; false for all pre-26.1 versions. See
	// versionHasFluidCount in manager.go.
	HasFluidCount bool

	// Cached decoded sections (lazy populated on first access)
	sections     [SECTION_COUNT]*Section
	sectionsLock sync.RWMutex

	// Light data, keyed by light-section slot index — see lightSlotForY's
	// doc comment for the slot<->world-Y mapping. Populated from
	// ClientboundLevelChunkWithLight (chunk load) and the standalone
	// ClientboundUpdateLight packet (SetLight is called for either). Absent
	// from RawData/the block-section palette containers entirely — light is
	// carried as separate fields on those packets, not part of the chunk
	// section byte stream this file otherwise parses.
	lightMu    sync.RWMutex
	skyLight   map[int][]byte // slot -> 2048-byte nibble-packed array (4 bits/value)
	blockLight map[int][]byte
}

// LIGHT_SECTION_COUNT is the number of light sections per chunk column: one
// per block section (SECTION_COUNT), plus one extra below the lowest block
// section and one extra above the highest — vanilla sends light for the
// "cap" sections immediately outside the block range too, since light
// propagates from/through them. Slot 0 is the below-bottom cap, slot
// LIGHT_SECTION_COUNT-1 is the above-top cap.
//
// This range (SECTION_COUNT+2, i.e. one cap section on each side) is the
// standard convention documented for LevelChunkWithLight/UpdateLight's
// light masks; verify against a live capture during implementation review
// if light values look off by one section (16 blocks) at column edges.
const LIGHT_SECTION_COUNT = SECTION_COUNT + 2

// lightSlotForY maps a world Y coordinate to its light-section slot index.
func lightSlotForY(y int) int {
	sectionIdx := y >> 4
	return sectionIdx - (MIN_SECTION_Y - 1)
}

// SetLightData stores light data parsed from a ClientboundLevelChunkWithLight
// or ClientboundUpdateLight packet, replacing any light this chunk already
// had for the mask-indicated slots. Slots whose mask bit is unset (no light
// data sent for that section) are left untouched — vanilla only resends
// light for sections that actually changed since the previous update.
// skyMask/blockMask are the section-presence
// bitsets (as packed longs, LSB = lowest light slot); skyArrays/blockArrays
// are the corresponding 2048-byte nibble-packed arrays, one per set bit, in
// ascending slot order.
func (c *ChunkData) SetLightData(skyMask, blockMask []int64, skyArrays, blockArrays [][]byte) {
	c.lightMu.Lock()
	defer c.lightMu.Unlock()
	if c.skyLight == nil {
		c.skyLight = make(map[int][]byte)
	}
	if c.blockLight == nil {
		c.blockLight = make(map[int][]byte)
	}
	applyLightMask(skyMask, skyArrays, c.skyLight)
	applyLightMask(blockMask, blockArrays, c.blockLight)
}

// applyLightMask walks a section-presence bitset (packed longs, LSB-first)
// and assigns each set bit's next array from arrays, in order, into dst
// keyed by slot index.
func applyLightMask(mask []int64, arrays [][]byte, dst map[int][]byte) {
	arrIdx := 0
	slot := 0
	for _, word := range mask {
		for bit := 0; bit < 64; bit++ {
			if word&(int64(1)<<uint(bit)) != 0 {
				if arrIdx < len(arrays) {
					dst[slot] = arrays[arrIdx]
					arrIdx++
				}
			}
			slot++
		}
	}
}

// GetLightLevel returns sky/block light (0-15 each) at the given
// chunk-relative coordinates. x and z should be 0-15, y is the world Y
// coordinate. ok is false if no light data has been received for the
// containing section.
func (c *ChunkData) GetLightLevel(x, y, z int) (sky, block uint8, ok bool) {
	slot := lightSlotForY(y)
	if slot < 0 || slot >= LIGHT_SECTION_COUNT {
		return 0, 0, false
	}

	localY := y & 15
	blockIdx := localY*SECTION_WIDTH*SECTION_WIDTH + z*SECTION_WIDTH + x

	c.lightMu.RLock()
	defer c.lightMu.RUnlock()

	skyArr, skyOK := c.skyLight[slot]
	blockArr, blockOK := c.blockLight[slot]
	if !skyOK && !blockOK {
		return 0, 0, false
	}
	if skyOK {
		sky = readNibble(skyArr, blockIdx)
	}
	if blockOK {
		block = readNibble(blockArr, blockIdx)
	}
	return sky, block, true
}

// readNibble unpacks the 4-bit value at idx from a nibble-packed byte array
// (2 values per byte: idx's low nibble if idx is even, high nibble if odd).
func readNibble(data []byte, idx int) uint8 {
	byteIdx := idx / 2
	if byteIdx < 0 || byteIdx >= len(data) {
		return 0
	}
	if idx%2 == 0 {
		return data[byteIdx] & 0x0F
	}
	return (data[byteIdx] >> 4) & 0x0F
}

// Direct-palette bit thresholds, cited from PaletteProvider.forBlockStates /
// forBiomes (decompiled via mc-data-gen/extractedSrc/<version>/net/minecraft/
// world/chunk/PaletteProvider.java; confirmed identical in the 1.21.11 and
// 26.1 trees, under Mojang's respective Strategy.java naming for 26.1):
// blocks use an indirect (palette-array) container for bitsInStorage 0-8 and
// switch to a direct/global container (bitsInMemory bits, no palette array on
// the wire) for anything above that; biomes switch at 3. A section whose
// local diversity needs more bits than its indirect cap (e.g. a section that
// happens to reference many distinct global block states) is sent directly.
const (
	blockDirectPaletteBitsThreshold = 8
	biomeDirectPaletteBitsThreshold = 3
)

// isDirectPalette reports whether a palette container with the given
// bits-per-entry is in direct (global-ID, no palette array) form for a
// container of the given sectionSize (BLOCK_SECTION_SIZE or BIOME_SECTION_SIZE).
func isDirectPalette(bpe uint8, sectionSize int) bool {
	if sectionSize == BIOME_SECTION_SIZE {
		return int(bpe) > biomeDirectPaletteBitsThreshold
	}
	return int(bpe) > blockDirectPaletteBitsThreshold
}

// Section represents a 16x16x16 section of blocks.
type Section struct {
	BlockCount   int16    // Number of non-air blocks in section
	BitsPerEntry uint8    // 0 = single-valued, >0 = palette-based
	IsDirect     bool     // true: DataArray entries are the state IDs themselves (no Palette indirection)
	Palette      []uint32 // Block state IDs (empty if BitsPerEntry == 0)
	SingleValue  uint32   // Single state ID if BitsPerEntry == 0
	DataArray    []uint64 // Packed block indices (empty if BitsPerEntry == 0)

	// Biome palette container, same shape as the block fields above but at
	// 4x4x4 granularity (BIOME_SECTION_SIZE = 64 entries per section).
	BiomeBitsPerEntry uint8
	BiomeIsDirect     bool
	BiomePalette      []uint32
	BiomeSingleValue  uint32
	BiomeDataArray    []uint64
}

// GetBlockAt returns the block state ID at the given chunk-relative coordinates.
// x and z should be 0-15, y is the world Y coordinate.
func (c *ChunkData) GetBlockAt(x, y, z int) uint32 {
	// Calculate section index
	sectionIdx := y >> 4 // y / 16
	sectionSlotIdx := sectionIdx - MIN_SECTION_Y

	// Bounds check
	if sectionSlotIdx < 0 || sectionSlotIdx >= SECTION_COUNT {
		return 0 // Air for out of bounds
	}

	// Get or load section
	section := c.getOrLoadSection(sectionSlotIdx)
	if section == nil {
		return 0 // Air if section couldn't be loaded
	}

	// Calculate block index within section
	localY := y & 15 // y % 16
	blockIdx := localY*SECTION_WIDTH*SECTION_WIDTH + z*SECTION_WIDTH + x

	return section.getBlockAt(blockIdx)
}

// ApplyBiomeUpdate re-parses a ClientboundChunkBiomes update's raw data (one
// biome-only palette container per section, concatenated in section order
// with no block data interleaved — the "data" field of one
// ChunkBiomeUpdate) and swaps the biome fields into each of this chunk's
// sections, forcing lazy-loading of any section not already loaded.
func (c *ChunkData) ApplyBiomeUpdate(data []byte, useCalculatedLen bool) error {
	r := bytes.NewReader(data)
	for i := 0; i < SECTION_COUNT; i++ {
		biomes, err := parsePaletteContainer(r, BIOME_SECTION_SIZE, useCalculatedLen)
		if err != nil {
			return err
		}
		section := c.getOrLoadSection(i)
		if section == nil {
			continue
		}
		c.sectionsLock.Lock()
		section.BiomeBitsPerEntry = biomes.bitsPerEntry
		section.BiomeIsDirect = biomes.isDirect
		section.BiomePalette = biomes.palette
		section.BiomeSingleValue = biomes.singleValue
		section.BiomeDataArray = biomes.dataArray
		c.sectionsLock.Unlock()
	}
	return nil
}

// GetBiomeAt returns the biome registry ID at the given chunk-relative
// coordinates. x and z should be 0-15, y is the world Y coordinate. Biomes
// are stored at 4x4x4 granularity, so this covers a 4x4x4 region of blocks
// per distinct returned value.
func (c *ChunkData) GetBiomeAt(x, y, z int) uint32 {
	sectionIdx := y >> 4
	sectionSlotIdx := sectionIdx - MIN_SECTION_Y

	if sectionSlotIdx < 0 || sectionSlotIdx >= SECTION_COUNT {
		return 0
	}

	section := c.getOrLoadSection(sectionSlotIdx)
	if section == nil {
		return 0
	}

	localY := (y & 15) / 4
	biomeIdx := localY*4*4 + (z/4)*4 + (x / 4)

	return section.getBiomeAt(biomeIdx)
}

func (c *ChunkData) getOrLoadSection(sectionSlotIdx int) *Section {
	// Fast path: check cache with read lock
	c.sectionsLock.RLock()
	section := c.sections[sectionSlotIdx]
	c.sectionsLock.RUnlock()

	if section != nil {
		return section
	}

	// Slow path: need to load section
	c.sectionsLock.Lock()
	defer c.sectionsLock.Unlock()

	// Double check after acquiring write lock
	if c.sections[sectionSlotIdx] != nil {
		return c.sections[sectionSlotIdx]
	}

	// Load the section from raw data
	section = c.loadSection(sectionSlotIdx)
	c.sections[sectionSlotIdx] = section
	return section
}

// loadSection parses a specific section from the raw chunk data.
// Sections are stored sequentially in the raw data.
func (c *ChunkData) loadSection(targetIdx int) *Section {
	if c.RawData == nil || len(c.RawData) == 0 {
		return &Section{BitsPerEntry: 0, SingleValue: 0} // Empty section = air
	}

	reader := bytes.NewReader(c.RawData)

	// Skip sections before target
	for i := 0; i < targetIdx; i++ {
		if err := skipSection(reader, c.UseCalculatedDataLen, c.HasFluidCount); err != nil {
			// A real decode error here silently falls back to treating the
			// section as air, which previously made a corrupt/misaligned
			// parse indistinguishable from a legitimately empty section. Log
			// it so a stream desync is visible instead of manifesting only as
			// "ground never detected".
			log.Printf("[ChunkData.loadSection][WARN] chunk(%d,%d) targetIdx=%d: skipSection(%d) failed: %v (rawLen=%d useCalcLen=%v hasFluidCount=%v)", c.X, c.Z, targetIdx, i, err, len(c.RawData), c.UseCalculatedDataLen, c.HasFluidCount)
			return &Section{BitsPerEntry: 0, SingleValue: 0}
		}
	}

	// Parse target section
	section, err := parseSection(reader, c.UseCalculatedDataLen, c.HasFluidCount)
	if err != nil {
		log.Printf("[ChunkData.loadSection][WARN] chunk(%d,%d) targetIdx=%d: parseSection failed: %v (rawLen=%d useCalcLen=%v hasFluidCount=%v)", c.X, c.Z, targetIdx, err, len(c.RawData), c.UseCalculatedDataLen, c.HasFluidCount)
		return &Section{BitsPerEntry: 0, SingleValue: 0}
	}

	return section
}

func (s *Section) getBlockAt(blockIdx int) uint32 {
	if blockIdx < 0 || blockIdx >= BLOCK_SECTION_SIZE {
		return 0
	}

	// Single-valued section
	if s.BitsPerEntry == 0 {
		return s.SingleValue
	}

	// Direct section: the packed entry IS the global state ID, no palette indirection
	if s.IsDirect {
		return s.getPaletteIndex(blockIdx)
	}

	// Palette-based section
	paletteIdx := s.getPaletteIndex(blockIdx)
	if int(paletteIdx) >= len(s.Palette) {
		return 0
	}
	return s.Palette[paletteIdx]
}

func (s *Section) getPaletteIndex(blockIdx int) uint32 {
	return getPackedIndex(blockIdx, s.BitsPerEntry, s.DataArray)
}

// getBiomeAt returns the biome registry ID at the given biome-grid index
// (0-63, 4x4x4 granularity — see ChunkData.GetBiomeAt for coordinate mapping).
func (s *Section) getBiomeAt(biomeIdx int) uint32 {
	if biomeIdx < 0 || biomeIdx >= BIOME_SECTION_SIZE {
		return 0
	}

	if s.BiomeBitsPerEntry == 0 {
		return s.BiomeSingleValue
	}

	if s.BiomeIsDirect {
		return getPackedIndex(biomeIdx, s.BiomeBitsPerEntry, s.BiomeDataArray)
	}

	paletteIdx := getPackedIndex(biomeIdx, s.BiomeBitsPerEntry, s.BiomeDataArray)
	if int(paletteIdx) >= len(s.BiomePalette) {
		return 0
	}
	return s.BiomePalette[paletteIdx]
}

// getPackedIndex unpacks the entry at idx from a bits-per-entry-packed
// []uint64 array, shared by both the block and biome palette containers.
func getPackedIndex(idx int, bitsPerEntry uint8, dataArray []uint64) uint32 {
	if bitsPerEntry == 0 {
		return 0
	}

	bits := int(bitsPerEntry)
	entriesPerLong := 64 / bits
	longIdx := idx / entriesPerLong
	bitOffset := (idx % entriesPerLong) * bits

	if longIdx >= len(dataArray) {
		return 0
	}

	mask := uint64((1 << bits) - 1)
	return uint32((dataArray[longIdx] >> bitOffset) & mask)
}

// skipSection skips over a section in the reader.
// useCalculatedLen: if true, calculate data array length (1.21.5+); if false, read as VarInt (pre-1.21.5)
// hasFluidCount: if true, a second short (fluidCount) follows nonEmptyBlockCount (26.1+)
func skipSection(r *bytes.Reader, useCalculatedLen bool, hasFluidCount bool) error {
	// Read and discard block count (Short)
	var blockCount int16
	if err := binary.Read(r, binary.BigEndian, &blockCount); err != nil {
		return err
	}
	if hasFluidCount {
		var fluidCount int16
		if err := binary.Read(r, binary.BigEndian, &fluidCount); err != nil {
			return err
		}
	}
	// Skip block states container
	if err := skipPaletteContainer(r, BLOCK_SECTION_SIZE, useCalculatedLen); err != nil {
		return err
	}
	// Skip biome container
	if err := skipPaletteContainer(r, BIOME_SECTION_SIZE, useCalculatedLen); err != nil {
		return err
	}
	return nil
}

// parseSection parses a section from the reader.
// useCalculatedLen: if true, calculate data array length (1.21.5+); if false, read as VarInt (pre-1.21.5)
// hasFluidCount: if true, a second short (fluidCount) follows nonEmptyBlockCount (26.1+)
func parseSection(r *bytes.Reader, useCalculatedLen bool, hasFluidCount bool) (*Section, error) {
	section := &Section{}

	// Read block count (not used for block lookup but stored)
	if err := binary.Read(r, binary.BigEndian, &section.BlockCount); err != nil {
		return nil, err
	}
	if hasFluidCount {
		// fluidCount (26.1+): not currently exposed, discarded like BlockCount.
		var fluidCount int16
		if err := binary.Read(r, binary.BigEndian, &fluidCount); err != nil {
			return nil, err
		}
	}

	// Parse block states palette container
	blocks, err := parsePaletteContainer(r, BLOCK_SECTION_SIZE, useCalculatedLen)
	if err != nil {
		return nil, err
	}
	section.BitsPerEntry = blocks.bitsPerEntry
	section.IsDirect = blocks.isDirect
	section.Palette = blocks.palette
	section.SingleValue = blocks.singleValue
	section.DataArray = blocks.dataArray

	// Parse biome palette container (4x4x4 granularity, BIOME_SECTION_SIZE entries)
	biomes, err := parsePaletteContainer(r, BIOME_SECTION_SIZE, useCalculatedLen)
	if err != nil {
		return nil, err
	}
	section.BiomeBitsPerEntry = biomes.bitsPerEntry
	section.BiomeIsDirect = biomes.isDirect
	section.BiomePalette = biomes.palette
	section.BiomeSingleValue = biomes.singleValue
	section.BiomeDataArray = biomes.dataArray

	return section, nil
}

// paletteData holds a parsed palette container's fields (block or biome —
// the wire shape is identical, just at different sectionSize granularity).
type paletteData struct {
	bitsPerEntry uint8
	isDirect     bool
	palette      []uint32
	singleValue  uint32
	dataArray    []uint64
}

// parsePaletteContainer parses a palette container (blocks or biomes).
// sectionSize: BLOCK_SECTION_SIZE (4096) or BIOME_SECTION_SIZE (64)
// useCalculatedLen: if true, calculate data array length (1.21.5+); if false, read as VarInt (pre-1.21.5)
func parsePaletteContainer(r *bytes.Reader, sectionSize int, useCalculatedLen bool) (*paletteData, error) {
	pd := &paletteData{}

	// Read bits per entry
	bpe, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	pd.bitsPerEntry = bpe

	if bpe == 0 {
		// Single-valued palette
		value, err := readVarInt(r)
		if err != nil {
			return nil, err
		}
		pd.singleValue = uint32(value)
		// In pre-1.21.5, there's a data array length VarInt (should be 0)
		// In 1.21.5+, there's no data array length field
		if !useCalculatedLen {
			_, err = readVarInt(r)
			if err != nil {
				return nil, err
			}
		}
		return pd, nil
	}

	// Direct (global-ID) container: no palette array on the wire at all.
	if isDirectPalette(bpe, sectionSize) {
		pd.isDirect = true
	} else {
		// Indirect: palette-based
		paletteLen, err := readVarInt(r)
		if err != nil {
			return nil, err
		}

		pd.palette = make([]uint32, paletteLen)
		for i := 0; i < int(paletteLen); i++ {
			val, err := readVarInt(r)
			if err != nil {
				return nil, err
			}
			pd.palette[i] = uint32(val)
		}
	}

	// Determine data array length
	var dataLen int
	if useCalculatedLen {
		// 1.21.5+: Calculate from bits per entry
		bits := int(bpe)
		entriesPerLong := 64 / bits
		dataLen = (sectionSize + entriesPerLong - 1) / entriesPerLong
	} else {
		// Pre-1.21.5: Read as VarInt
		dataLenVar, err := readVarInt(r)
		if err != nil {
			return nil, err
		}
		dataLen = int(dataLenVar)
	}

	pd.dataArray = make([]uint64, dataLen)
	for i := 0; i < dataLen; i++ {
		if err := binary.Read(r, binary.BigEndian, &pd.dataArray[i]); err != nil {
			return nil, err
		}
	}

	return pd, nil
}

// skipPaletteContainer skips over a palette container in the reader.
// sectionSize: BLOCK_SECTION_SIZE (4096) or BIOME_SECTION_SIZE (64)
// useCalculatedLen: if true, calculate data array length (1.21.5+); if false, read as VarInt (pre-1.21.5)
func skipPaletteContainer(r *bytes.Reader, sectionSize int, useCalculatedLen bool) error {
	// Read bits per entry
	bpe, err := r.ReadByte()
	if err != nil {
		return err
	}

	if bpe == 0 {
		// Single-valued: skip palette value
		if _, err := readVarInt(r); err != nil {
			return err
		}
		// In pre-1.21.5, there's a data array length VarInt (should be 0)
		// In 1.21.5+, there's no data array length field
		if !useCalculatedLen {
			_, err = readVarInt(r)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// Indirect: palette-based, skip palette entries. Direct (global-ID)
	// containers have no palette array on the wire at all.
	if !isDirectPalette(bpe, sectionSize) {
		paletteLen, err := readVarInt(r)
		if err != nil {
			return err
		}
		for i := 0; i < int(paletteLen); i++ {
			if _, err := readVarInt(r); err != nil {
				return err
			}
		}
	}

	// Determine data array length and skip it
	var dataLen int
	if useCalculatedLen {
		// 1.21.5+: Calculate from bits per entry
		bits := int(bpe)
		entriesPerLong := 64 / bits
		dataLen = (sectionSize + entriesPerLong - 1) / entriesPerLong
	} else {
		// Pre-1.21.5: Read as VarInt
		dataLenVar, err := readVarInt(r)
		if err != nil {
			return err
		}
		dataLen = int(dataLenVar)
	}

	// Skip dataLen * 8 bytes (each long is 8 bytes)
	_, err = r.Seek(int64(dataLen)*8, io.SeekCurrent)
	return err
}

// readVarInt reads a VarInt from the reader.
func readVarInt(r *bytes.Reader) (int32, error) {
	var result int32
	var shift uint

	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}

		result |= int32(b&0x7F) << shift

		if (b & 0x80) == 0 {
			break
		}

		shift += 7
		if shift >= 35 {
			return 0, io.ErrUnexpectedEOF
		}
	}

	return result, nil
}
