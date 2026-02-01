package world

import (
	"bytes"
	"encoding/binary"
	"io"
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

	// Cached decoded sections (lazy populated on first access)
	sections     [SECTION_COUNT]*Section
	sectionsLock sync.RWMutex
}

// Section represents a 16x16x16 section of blocks.
type Section struct {
	BlockCount   int16    // Number of non-air blocks in section
	BitsPerEntry uint8    // 0 = single-valued, >0 = palette-based
	Palette      []uint32 // Block state IDs (empty if BitsPerEntry == 0)
	SingleValue  uint32   // Single state ID if BitsPerEntry == 0
	DataArray    []uint64 // Packed block indices (empty if BitsPerEntry == 0)
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
		if err := skipSection(reader, c.UseCalculatedDataLen); err != nil {
			return &Section{BitsPerEntry: 0, SingleValue: 0}
		}
	}

	// Parse target section
	section, err := parseSection(reader, c.UseCalculatedDataLen)
	if err != nil {
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

	// Palette-based section
	paletteIdx := s.getPaletteIndex(blockIdx)
	if int(paletteIdx) >= len(s.Palette) {
		return 0
	}
	return s.Palette[paletteIdx]
}

func (s *Section) getPaletteIndex(blockIdx int) uint32 {
	if s.BitsPerEntry == 0 {
		return 0
	}

	bitsPerEntry := int(s.BitsPerEntry)
	entriesPerLong := 64 / bitsPerEntry
	longIdx := blockIdx / entriesPerLong
	bitOffset := (blockIdx % entriesPerLong) * bitsPerEntry

	if longIdx >= len(s.DataArray) {
		return 0
	}

	mask := uint64((1 << bitsPerEntry) - 1)
	return uint32((s.DataArray[longIdx] >> bitOffset) & mask)
}

// skipSection skips over a section in the reader.
// useCalculatedLen: if true, calculate data array length (1.21.5+); if false, read as VarInt (pre-1.21.5)
func skipSection(r *bytes.Reader, useCalculatedLen bool) error {
	// Read and discard block count (Short)
	var blockCount int16
	if err := binary.Read(r, binary.BigEndian, &blockCount); err != nil {
		return err
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
func parseSection(r *bytes.Reader, useCalculatedLen bool) (*Section, error) {
	section := &Section{}

	// Read block count (not used for block lookup but stored)
	if err := binary.Read(r, binary.BigEndian, &section.BlockCount); err != nil {
		return nil, err
	}

	// Parse block states palette container
	if err := parsePaletteContainer(r, section, BLOCK_SECTION_SIZE, useCalculatedLen); err != nil {
		return nil, err
	}

	// Skip biome container (we don't need it for block lookups)
	if err := skipPaletteContainer(r, BIOME_SECTION_SIZE, useCalculatedLen); err != nil {
		return nil, err
	}

	return section, nil
}

// parsePaletteContainer parses a palette container (blocks or biomes).
// sectionSize: BLOCK_SECTION_SIZE (4096) or BIOME_SECTION_SIZE (64)
// useCalculatedLen: if true, calculate data array length (1.21.5+); if false, read as VarInt (pre-1.21.5)
func parsePaletteContainer(r *bytes.Reader, section *Section, sectionSize int, useCalculatedLen bool) error {
	// Read bits per entry
	bpe, err := r.ReadByte()
	if err != nil {
		return err
	}
	section.BitsPerEntry = bpe

	if bpe == 0 {
		// Single-valued palette
		value, err := readVarInt(r)
		if err != nil {
			return err
		}
		section.SingleValue = uint32(value)
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

	// Palette-based
	paletteLen, err := readVarInt(r)
	if err != nil {
		return err
	}

	section.Palette = make([]uint32, paletteLen)
	for i := 0; i < int(paletteLen); i++ {
		val, err := readVarInt(r)
		if err != nil {
			return err
		}
		section.Palette[i] = uint32(val)
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
			return err
		}
		dataLen = int(dataLenVar)
	}

	section.DataArray = make([]uint64, dataLen)
	for i := 0; i < dataLen; i++ {
		if err := binary.Read(r, binary.BigEndian, &section.DataArray[i]); err != nil {
			return err
		}
	}

	return nil
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

	// Palette-based: skip palette entries
	paletteLen, err := readVarInt(r)
	if err != nil {
		return err
	}
	for i := 0; i < int(paletteLen); i++ {
		if _, err := readVarInt(r); err != nil {
			return err
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
