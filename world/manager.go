// Package world provides world state management for the Minecraft agent.
// It handles chunk loading/unloading, block state lookups, and block overrides.
package world

import (
	"log/slog"
	"math"
	"sync"

	semver "github.com/aquasecurity/go-version/pkg/version"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// var _ models.World = (*Manager)(nil) confirms Manager satisfies the full
// merged World interface — see docs/plans/WORLD_STRUCT_CONSOLIDATION.md.
var _ models.World = (*Manager)(nil)

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

	// hasFluidCount is true for 26.1+, where LevelChunkSection.write() gained
	// a second short (fluidCount) after nonEmptyBlockCount, before the block
	// states container. See versionHasFluidCount.
	hasFluidCount bool

	logger *slog.Logger

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

	// entityProviderMu protects entityProvider
	entityProviderMu sync.RWMutex
	// entityProvider supplies entity snapshots for GetEntitiesInRange.
	// nil until WithEntityProvider is called; GetEntitiesInRange returns an
	// empty slice while nil.
	entityProvider models.EntityProvider

	// worldBorderMu protects worldBorder and hasWorldBorder
	worldBorderMu  sync.RWMutex
	worldBorder    models.WorldBorder
	hasWorldBorder bool

	// difficultyMu protects difficulty, difficultyLocked, and hasDifficulty
	difficultyMu     sync.RWMutex
	difficulty       uint8
	difficultyLocked bool
	hasDifficulty    bool
}

// NewManager creates a new world manager.
func NewManager(versionHandler models.VersionHandler, events EventsListener, logger *slog.Logger) *Manager {
	// Determine if we should use calculated data length based on version
	// In 1.21.5+, the data array length is not sent as a VarInt but must be calculated
	useCalculatedLen := false
	hasFluidCount := false
	if versionHandler != nil {
		useCalculatedLen = versionRequiresCalculatedDataLen(versionHandler.Version())
		hasFluidCount = versionHasFluidCount(versionHandler.Version())
	}

	return &Manager{
		versionHandler:       versionHandler,
		events:               events,
		useCalculatedDataLen: useCalculatedLen,
		hasFluidCount:        hasFluidCount,
		logger:               utils.SafeLogger(logger),
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
	v, err := semver.Parse(version)
	if err != nil {
		return false // Unknown format, assume old behavior
	}
	c, err := semver.NewConstraints(">= 1.21.5")
	if err != nil {
		return false
	}
	return c.Check(v)
}

// versionHasFluidCount returns true if the Minecraft version's LevelChunkSection
// wire format includes a second short (fluidCount) after nonEmptyBlockCount,
// before the block states palette container. Confirmed present in 26.1's
// LevelChunkSection.write() (/net/minecraft/
// world/level/chunk/LevelChunkSection.java) and absent from 1.21.11's
// equivalent ChunkSection.writePacket() (/net/minecraft/world/chunk/ChunkSection.java), which writes only
// nonEmptyBlockCount.
func versionHasFluidCount(version string) bool {
	v, err := semver.Parse(version)
	if err != nil {
		return false // Unknown format, assume old behavior
	}
	c, err := semver.NewConstraints(">= 26.1")
	if err != nil {
		return false
	}
	return c.Check(v)
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

// GetBiomeAt returns the biome registry ID at the given block coordinates.
// The second return value mirrors GetBlockAt's chunk-loaded semantics.
func (m *Manager) GetBiomeAt(x, y, z int) (uint32, bool) {
	chunkX := int32(x) >> 4
	chunkZ := int32(z) >> 4
	pos := ChunkPos{X: chunkX, Z: chunkZ}

	m.mu.RLock()
	chunk, exists := m.Columns[pos]
	m.mu.RUnlock()

	if !exists || chunk == nil {
		return 0, false
	}

	relX := x & 15
	relZ := z & 15

	return chunk.GetBiomeAt(relX, y, relZ), true
}

// GetLightLevel returns sky/block light (0-15 each) at the given block
// coordinates. loaded is false if no light data has been received for the
// containing chunk section.
func (m *Manager) GetLightLevel(x, y, z int) (sky, block uint8, loaded bool) {
	chunkX := int32(x) >> 4
	chunkZ := int32(z) >> 4
	pos := ChunkPos{X: chunkX, Z: chunkZ}

	m.mu.RLock()
	chunk, exists := m.Columns[pos]
	m.mu.RUnlock()

	if !exists || chunk == nil {
		return 0, 0, false
	}

	relX := x & 15
	relZ := z & 15

	return chunk.GetLightLevel(relX, y, relZ)
}

// HandleChunkLight stores light data for a chunk, parsed from either
// ClientboundLevelChunkWithLight (chunk load) or the standalone
// ClientboundUpdateLight packet. If the chunk hasn't been loaded yet (light
// arrived before/without its chunk — not expected on the chunk-load path,
// possible in principle for a standalone update), a placeholder ChunkData is
// created to hold the light; a subsequent HandleChunkLoad for the same
// position replaces it wholesale and any such light is lost. Not expected to
// matter in practice: light updates for a chunk the client hasn't loaded are
// not useful data (nothing renders/queries it), and this repo doesn't
// currently do anything with light besides record it.
func (m *Manager) HandleChunkLight(chunkX, chunkZ int32, light models.ChunkLightData) {
	pos := ChunkPos{X: chunkX, Z: chunkZ}

	m.mu.Lock()
	chunk, exists := m.Columns[pos]
	if !exists || chunk == nil {
		chunk = &ChunkData{X: chunkX, Z: chunkZ, logger: m.logger}
		m.Columns[pos] = chunk
	}
	m.mu.Unlock()

	chunk.SetLightData(light.SkyLightMask, light.BlockLightMask, light.SkyLight, light.BlockLight)
}

// HandleChunkBiomesUpdate applies post-load biome changes from a
// ClientboundChunkBiomes packet to already-loaded chunks. Updates for a
// chunk that isn't currently loaded are silently dropped — there is no
// section data to patch, and the chunk's eventual real load will carry
// correct up-to-date biome data of its own.
func (m *Manager) HandleChunkBiomesUpdate(updates []models.ChunkBiomeUpdate) {
	for _, u := range updates {
		pos := ChunkPos{X: u.ChunkX, Z: u.ChunkZ}
		m.mu.RLock()
		chunk, exists := m.Columns[pos]
		m.mu.RUnlock()
		if !exists || chunk == nil {
			continue
		}
		if err := chunk.ApplyBiomeUpdate(u.Data, m.useCalculatedDataLen); err != nil {
			// Non-fatal: subsequent block/biome queries just fall back to
			// whichever biome data was already loaded for the affected
			// sections.
			continue
		}
	}
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

// GetBlockStatus is GetBlockAt with integer coordinates.
//
// Deprecated: thin wrapper kept for existing int-coordinate callers — see
// models.World.GetBlockStatus's doc comment. Prefer GetBlockAt in new code.
func (m *Manager) GetBlockStatus(x, y, z int) (uint32, bool) {
	return m.GetBlockAt(float64(x), float64(y), float64(z))
}

// WithEntityProvider configures the source GetEntitiesInRange queries for
// entity snapshots. Returns m for chaining. Safe to call at any time; takes
// effect for subsequent GetEntitiesInRange calls.
func (m *Manager) WithEntityProvider(provider models.EntityProvider) *Manager {
	m.entityProviderMu.Lock()
	m.entityProvider = provider
	m.entityProviderMu.Unlock()
	return m
}

// GetEntitiesInRange returns all entities whose AABBs overlap with the query
// box. Returns an empty slice if no EntityProvider has been configured.
func (m *Manager) GetEntitiesInRange(queryBB models.AABB) []models.EntityBounds {
	m.entityProviderMu.RLock()
	provider := m.entityProvider
	m.entityProviderMu.RUnlock()

	if provider == nil {
		return []models.EntityBounds{}
	}

	snapshots := provider.GetEntitiesSnapshot()
	result := make([]models.EntityBounds, 0, len(snapshots))

	for entityID, snapshot := range snapshots {
		dimensions := models.EntityDimensionsFor(snapshot.EntityType)
		entityBB := models.AABB{
			X: models.MinMax{
				Min: snapshot.Pos.X - dimensions.Width/2,
				Max: snapshot.Pos.X + dimensions.Width/2,
			},
			Y: models.MinMax{
				Min: snapshot.Pos.Y,
				Max: snapshot.Pos.Y + dimensions.Height,
			},
			Z: models.MinMax{
				Min: snapshot.Pos.Z - dimensions.Width/2,
				Max: snapshot.Pos.Z + dimensions.Width/2,
			},
		}

		// Check for AABB overlap (separate on each axis)
		if queryBB.X.Min < entityBB.X.Max && queryBB.X.Max > entityBB.X.Min &&
			queryBB.Y.Min < entityBB.Y.Max && queryBB.Y.Max > entityBB.Y.Min &&
			queryBB.Z.Min < entityBB.Z.Max && queryBB.Z.Max > entityBB.Z.Min {
			result = append(result, models.EntityBounds{
				EntityID:  entityID,
				AABB:      entityBB,
				Velocity:  snapshot.Vel,
				Standable: snapshot.IsStandableSurface,
			})
		}
	}

	return result
}

// GetWorldBorder returns the current world border state and whether a
// ClientboundInitializeWorldBorder packet has been received yet.
func (m *Manager) GetWorldBorder() (models.WorldBorder, bool) {
	m.worldBorderMu.RLock()
	defer m.worldBorderMu.RUnlock()
	return m.worldBorder, m.hasWorldBorder
}

// SetWorldBorder replaces the full world border state. Called when a
// ClientboundInitializeWorldBorder packet is received.
func (m *Manager) SetWorldBorder(b models.WorldBorder) {
	m.worldBorderMu.Lock()
	defer m.worldBorderMu.Unlock()
	m.worldBorder = b
	m.hasWorldBorder = true
}

// SetWorldBorderCenter updates the border's center. Called when a
// ClientboundSetBorderCenter packet is received.
func (m *Manager) SetWorldBorderCenter(x, z float64) {
	m.worldBorderMu.Lock()
	defer m.worldBorderMu.Unlock()
	m.worldBorder.CenterX = x
	m.worldBorder.CenterZ = z
	m.hasWorldBorder = true
}

// SetWorldBorderSize updates the border's diameter (no lerp in progress).
// Called when a ClientboundSetBorderSize packet is received.
func (m *Manager) SetWorldBorderSize(diameter float64) {
	m.worldBorderMu.Lock()
	defer m.worldBorderMu.Unlock()
	m.worldBorder.OldDiameter = diameter
	m.worldBorder.NewDiameter = diameter
	m.worldBorder.SpeedTicks = 0
	m.hasWorldBorder = true
}

// SetWorldBorderLerpSize starts (or updates) a border resize animation.
// Called when a ClientboundSetBorderLerpSize packet is received.
func (m *Manager) SetWorldBorderLerpSize(oldDiameter, newDiameter float64, speedTicks int64) {
	m.worldBorderMu.Lock()
	defer m.worldBorderMu.Unlock()
	m.worldBorder.OldDiameter = oldDiameter
	m.worldBorder.NewDiameter = newDiameter
	m.worldBorder.SpeedTicks = speedTicks
	m.hasWorldBorder = true
}

// SetWorldBorderWarningDelay updates the border's warning time in ticks.
// Called when a ClientboundSetBorderWarningDelay packet is received.
func (m *Manager) SetWorldBorderWarningDelay(warningTimeTicks int32) {
	m.worldBorderMu.Lock()
	defer m.worldBorderMu.Unlock()
	m.worldBorder.WarningTimeTicks = warningTimeTicks
	m.hasWorldBorder = true
}

// SetWorldBorderWarningDistance updates the border's warning distance in
// blocks. Called when a ClientboundSetBorderWarningDistance packet is
// received.
func (m *Manager) SetWorldBorderWarningDistance(warningBlocks int32) {
	m.worldBorderMu.Lock()
	defer m.worldBorderMu.Unlock()
	m.worldBorder.WarningBlocks = warningBlocks
	m.hasWorldBorder = true
}

// GetDifficulty returns the current difficulty, whether it's locked, and
// whether a ClientboundChangeDifficulty packet has been received yet.
func (m *Manager) GetDifficulty() (difficulty uint8, locked bool, ok bool) {
	m.difficultyMu.RLock()
	defer m.difficultyMu.RUnlock()
	return m.difficulty, m.difficultyLocked, m.hasDifficulty
}

// SetDifficulty updates difficulty and lock state. Called when a
// ClientboundChangeDifficulty packet is received.
func (m *Manager) SetDifficulty(difficulty uint8, locked bool) {
	m.difficultyMu.Lock()
	defer m.difficultyMu.Unlock()
	m.difficulty = difficulty
	m.difficultyLocked = locked
	m.hasDifficulty = true
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
		HasFluidCount:        m.hasFluidCount,
		logger:               m.logger,
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
