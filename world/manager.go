// Package world provides world state management for the Minecraft agent.
// It handles chunk loading/unloading, block state lookups, and block overrides.
package world

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
)

// ChunkPos represents a chunk position in the world.
type ChunkPos struct {
	X, Z int32
}

// blockPos represents a block position for override storage.
type blockPos struct {
	X, Y, Z int64
}

// EventsListener contains callbacks for world events.
type EventsListener struct {
	// LoadChunk is called when a new chunk is loaded
	LoadChunk func(pos ChunkPos) error
	// UnloadChunk is called when a chunk is unloaded
	UnloadChunk func(pos ChunkPos) error
}

// Manager implements the world state manager.
// It stores loaded chunks and provides block state lookups.
type Manager struct {
	versionHandler models.VersionHandler
	events         EventsListener

	// useCalculatedDataLen is true for 1.21.5+ where data array length
	// is calculated, not sent as VarInt
	useCalculatedDataLen bool

	mu      sync.RWMutex
	Columns map[ChunkPos]*ChunkData

	overridesMu    sync.RWMutex
	blockOverrides map[blockPos]uint32

	// worldTimeMu protects worldAge and timeOfDay
	worldTimeMu sync.RWMutex
	// worldAge is the current server world age in ticks.
	// Updated from ClientboundUpdateTime packets.
	// -1 indicates not yet initialized (no Update Time packet received).
	worldAge int64
	// timeOfDay is the current time of day in ticks (0-23999 per day).
	// Updated from ClientboundUpdateTime packets.
	// -1 indicates not yet initialized (no Update Time packet received).
	timeOfDay int64
}

// NewManager creates a new world manager.
func NewManager(versionHandler models.VersionHandler, events EventsListener) *Manager {
	// Determine if we should use calculated data length based on version
	// In 1.21.5+, the data array length is not sent as a VarInt but must be calculated
	useCalculatedLen := false
	if versionHandler != nil {
		useCalculatedLen = versionRequiresCalculatedDataLen(versionHandler.Version())
	}

	return &Manager{
		versionHandler:       versionHandler,
		events:               events,
		useCalculatedDataLen: useCalculatedLen,
		Columns:              make(map[ChunkPos]*ChunkData),
		blockOverrides:       make(map[blockPos]uint32),
		worldAge:             -1, // Sentinel: not yet initialized
		timeOfDay:            -1, // Sentinel: not yet initialized
	}
}

// versionRequiresCalculatedDataLen returns true if the Minecraft version
// requires calculating data array length instead of reading it as a VarInt.
// This changed in 1.21.5.
func versionRequiresCalculatedDataLen(version string) bool {
	// Parse version string like "1.21.5" or "1.21.4"
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false // Unknown format, assume old behavior
	}

	// Parse major.minor.patch
	var major, minor, patch int
	if len(parts) >= 1 {
		fmt.Sscanf(parts[0], "%d", &major)
	}
	if len(parts) >= 2 {
		fmt.Sscanf(parts[1], "%d", &minor)
	}
	if len(parts) >= 3 {
		fmt.Sscanf(parts[2], "%d", &patch)
	}

	// Version 1.21.5 and later require calculated data length
	if major > 1 {
		return true
	}
	if major == 1 {
		if minor > 21 {
			return true
		}
		if minor == 21 && patch >= 5 {
			return true
		}
	}
	return false
}

// GetBlockAt returns the block state ID at the given world coordinates.
// The second return value indicates whether the chunk is loaded:
//   - true: chunk is loaded (stateID is valid, may be 0 for air)
//   - false: chunk is not loaded (stateID should be ignored)
func (m *Manager) GetBlockAt(x, y, z float64) (uint32, bool) {
	// Convert floats to block coordinates using floor
	bx, by, bz := toBlockCoords(x, y, z)

	// Check block overrides first (single block updates from packets)
	if stateID, ok := m.getBlockOverride(bx, by, bz); ok {
		return stateID, true
	}

	// Calculate chunk position
	chunkX := int32(bx) >> 4 // x / 16
	chunkZ := int32(bz) >> 4 // z / 16
	pos := ChunkPos{X: chunkX, Z: chunkZ}

	// Get chunk with read lock
	m.mu.RLock()
	chunk, exists := m.Columns[pos]
	m.mu.RUnlock()

	// Return false if chunk not loaded
	if !exists || chunk == nil {
		return 0, false
	}

	// Calculate chunk-relative coordinates
	relX := int(bx) & 15 // x % 16
	relZ := int(bz) & 15 // z % 16

	// Use ChunkData's GetBlockAt method
	return chunk.GetBlockAt(relX, int(by), relZ), true
}

// GetWorldAge returns the current server world age in ticks and whether it's been initialized.
// Returns (0, false) if no Update Time packet has been received yet.
// Returns (worldAge, true) when the value is valid from the server.
func (m *Manager) GetWorldAge() (int64, bool) {
	m.worldTimeMu.RLock()
	defer m.worldTimeMu.RUnlock()
	if m.worldAge < 0 {
		return 0, false
	}
	return m.worldAge, true
}

// GetTimeOfDay returns the current time of day in ticks and whether it's been initialized.
// Time of day ranges from 0-23999 ticks per day.
// Returns (0, false) if no Update Time packet has been received yet.
// Returns (timeOfDay, true) when the value is valid from the server.
func (m *Manager) GetTimeOfDay() (int64, bool) {
	m.worldTimeMu.RLock()
	defer m.worldTimeMu.RUnlock()
	if m.timeOfDay < 0 {
		return 0, false
	}
	return m.timeOfDay, true
}

// SetWorldTime updates both world age and time of day from the server's Update Time packet.
// This should be called whenever a ClientboundUpdateTime packet is received.
func (m *Manager) SetWorldTime(worldAge, timeOfDay int64) {
	m.worldTimeMu.Lock()
	defer m.worldTimeMu.Unlock()
	m.worldAge = worldAge
	m.timeOfDay = timeOfDay
}

// HandleChunkLoad processes a chunk data packet and stores the chunk.
func (m *Manager) HandleChunkLoad(chunkX, chunkZ int32, data []byte) error {
	pos := ChunkPos{X: chunkX, Z: chunkZ}

	// Create chunk data with version-appropriate parsing flag
	chunk := &ChunkData{
		X:                    chunkX,
		Z:                    chunkZ,
		RawData:              data,
		UseCalculatedDataLen: m.useCalculatedDataLen,
	}

	// Store the chunk
	m.mu.Lock()
	m.Columns[pos] = chunk
	m.mu.Unlock()

	// Clear any overrides for this chunk since we have fresh data
	m.clearOverridesForChunk(chunkX, chunkZ)

	// Notify listener
	if m.events.LoadChunk != nil {
		return m.events.LoadChunk(pos)
	}
	return nil
}

// HandleChunkUnload removes a chunk from the world.
func (m *Manager) HandleChunkUnload(chunkX, chunkZ int32) error {
	pos := ChunkPos{X: chunkX, Z: chunkZ}

	// Remove the chunk
	m.mu.Lock()
	delete(m.Columns, pos)
	m.mu.Unlock()

	// Clear overrides for this chunk
	m.clearOverridesForChunk(chunkX, chunkZ)

	// Notify listener
	if m.events.UnloadChunk != nil {
		return m.events.UnloadChunk(pos)
	}
	return nil
}

// HandleBlockUpdate updates a single block in the world.
func (m *Manager) HandleBlockUpdate(x, y, z int64, blockStateID int32) {
	m.setBlockOverride(x, y, z, uint32(blockStateID))
}

// HandleSectionBlocksUpdate updates multiple blocks in a section.
func (m *Manager) HandleSectionBlocksUpdate(blocks []models.BlockUpdate) {
	for _, block := range blocks {
		m.setBlockOverride(block.X, block.Y, block.Z, uint32(block.BlockStateID))
	}
}

// Reset clears all world data (called on respawn/dimension change).
func (m *Manager) Reset() {
	m.mu.Lock()
	m.Columns = make(map[ChunkPos]*ChunkData)
	m.mu.Unlock()

	m.clearAllOverrides()
}

// Block override methods

func (m *Manager) getBlockOverride(x, y, z int64) (uint32, bool) {
	m.overridesMu.RLock()
	defer m.overridesMu.RUnlock()
	stateID, ok := m.blockOverrides[blockPos{x, y, z}]
	return stateID, ok
}

func (m *Manager) setBlockOverride(x, y, z int64, state uint32) {
	m.overridesMu.Lock()
	defer m.overridesMu.Unlock()
	m.blockOverrides[blockPos{x, y, z}] = state
}

func (m *Manager) clearOverridesForChunk(chunkX, chunkZ int32) {
	m.overridesMu.Lock()
	defer m.overridesMu.Unlock()

	// Calculate block coordinate ranges for this chunk
	minX := int64(chunkX * 16)
	maxX := int64(minX + 15)
	minZ := int64(chunkZ * 16)
	maxZ := int64(minZ + 15)

	// Remove all overrides within this chunk's block range
	for pos := range m.blockOverrides {
		if pos.X >= minX && pos.X <= maxX && pos.Z >= minZ && pos.Z <= maxZ {
			delete(m.blockOverrides, pos)
		}
	}
}

func (m *Manager) clearAllOverrides() {
	m.overridesMu.Lock()
	defer m.overridesMu.Unlock()
	m.blockOverrides = make(map[blockPos]uint32)
}

// Utility functions

func toBlockCoords(x, y, z float64) (int64, int64, int64) {
	return int64(math.Floor(x)), int64(math.Floor(y)), int64(math.Floor(z))
}
