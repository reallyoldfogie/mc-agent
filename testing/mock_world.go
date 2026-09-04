package testing

import (
	"log"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
)

// MockWorld implements a synthetic world for testing pathfinding and movement
// It provides a simple in-memory block storage without chunk complexity
type MockWorld struct {
	mu        sync.RWMutex
	blocks    map[BlockPos]uint32 // Block state IDs
	worldAge  int64               // Server world age in ticks (-1 = not initialized)
	timeOfDay int64               // Time of day in ticks (-1 = not initialized)
}

// BlockPos represents a 3D block position
type BlockPos struct {
	X, Y, Z float64
}

// NewMockWorld creates an empty test world
func NewMockWorld() *MockWorld {
	return &MockWorld{
		blocks:    make(map[BlockPos]uint32),
		worldAge:  -1, // Not initialized
		timeOfDay: -1, // Not initialized
	}
}

// GetBlockAt implements the World interface used by pathfinding
// Returns the block state ID at the given position, or 0 (air) if not set.
// The second return value is always true for MockWorld since it's always "fully loaded".
func (mw *MockWorld) GetBlockAt(x, y, z float64) (uint32, bool) {
	mw.mu.RLock()
	defer mw.mu.RUnlock()

	pos := BlockPos{X: x, Y: y, Z: z}
	if stateID, exists := mw.blocks[pos]; exists {
		return stateID, true
	}
	return 0, true // Air, but chunk is "loaded"
}

// SetBlock sets a block in the mock world
// This is NOT part of the World interface - only for test setup
func (mw *MockWorld) SetBlock(x, y, z float64, stateID uint32) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	pos := BlockPos{X: x, Y: y, Z: z}
	if stateID == 0 {
		// Delete air blocks to save memory
		delete(mw.blocks, pos)
	} else {
		mw.blocks[pos] = stateID
	}
}

// SetBlockRange sets a rectangular region to the same block type
func (mw *MockWorld) SetBlockRange(x1, y1, z1, x2, y2, z2 float64, stateID uint32) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			for z := z1; z <= z2; z++ {
				pos := BlockPos{X: x, Y: y, Z: z}
				if stateID == 0 {
					delete(mw.blocks, pos)
				} else {
					mw.blocks[pos] = stateID
				}
			}
		}
	}
}

// Clear removes all blocks from the world
func (mw *MockWorld) Clear() {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	mw.blocks = make(map[BlockPos]uint32)
}

// BlockCount returns the number of non-air blocks in the world
func (mw *MockWorld) BlockCount() int {
	mw.mu.RLock()
	defer mw.mu.RUnlock()

	return len(mw.blocks)
}

func (mw *MockWorld) DumpWorld() {
	mw.mu.RLock()
	defer mw.mu.RUnlock()

	for pos, stateID := range mw.blocks {
		log.Printf("Block at (%.2f, %.2f, %.2f): StateID=%d", pos.X, pos.Y, pos.Z, stateID)
	}
}

// GetWorldAge returns the mock world's age and whether it's been initialized
func (mw *MockWorld) GetWorldAge() (int64, bool) {
	mw.mu.RLock()
	defer mw.mu.RUnlock()
	if mw.worldAge < 0 {
		return 0, false
	}
	return mw.worldAge, true
}

// GetTimeOfDay returns the mock world's time of day and whether it's been initialized
func (mw *MockWorld) GetTimeOfDay() (int64, bool) {
	mw.mu.RLock()
	defer mw.mu.RUnlock()
	if mw.timeOfDay < 0 {
		return 0, false
	}
	return mw.timeOfDay, true
}

// SetWorldTime sets both the mock world's age and time of day
func (mw *MockWorld) SetWorldTime(worldAge, timeOfDay int64) {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.worldAge = worldAge
	mw.timeOfDay = timeOfDay
}

// GetBlockStatus is GetBlockAt with integer coordinates.
func (mw *MockWorld) GetBlockStatus(x, y, z int) (uint32, bool) {
	return mw.GetBlockAt(float64(x), float64(y), float64(z))
}

// GetEntitiesInRange, GetLightLevel, GetBiomeAt, and the world-border/
// difficulty accessors below are stubbed to keep MockWorld satisfying
// models.World post-merge — real implementations deferred, see
// docs/plans/WORLD_STRUCT_CONSOLIDATION.md.

func (mw *MockWorld) GetEntitiesInRange(queryBB models.AABB) []models.EntityBounds {
	return []models.EntityBounds{}
}

func (mw *MockWorld) GetLightLevel(x, y, z int) (skyLight, blockLight uint8, loaded bool) {
	return 0, 0, false
}

func (mw *MockWorld) GetBiomeAt(x, y, z int) (biomeID uint32, loaded bool) {
	return 0, false
}

func (mw *MockWorld) GetWorldBorder() (models.WorldBorder, bool) {
	return models.WorldBorder{}, false
}

func (mw *MockWorld) SetWorldBorder(b models.WorldBorder)                    {}
func (mw *MockWorld) SetWorldBorderCenter(x, z float64)                      {}
func (mw *MockWorld) SetWorldBorderSize(diameter float64)                    {}
func (mw *MockWorld) SetWorldBorderLerpSize(oldD, newD float64, speed int64) {}
func (mw *MockWorld) SetWorldBorderWarningDelay(warningTimeTicks int32)      {}
func (mw *MockWorld) SetWorldBorderWarningDistance(warningBlocks int32)      {}

func (mw *MockWorld) GetDifficulty() (difficulty uint8, locked bool, ok bool) {
	return 0, false, false
}

func (mw *MockWorld) SetDifficulty(difficulty uint8, locked bool) {}
