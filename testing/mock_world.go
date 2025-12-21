package testing

import (
	"sync"
)

// MockWorld implements a synthetic world for testing pathfinding and movement
// It provides a simple in-memory block storage without chunk complexity
type MockWorld struct {
	mu     sync.RWMutex
	blocks map[BlockPos]uint32 // Block state IDs
}

// BlockPos represents a 3D block position
type BlockPos struct {
	X, Y, Z float64
}

// NewMockWorld creates an empty test world
func NewMockWorld() *MockWorld {
	return &MockWorld{
		blocks: make(map[BlockPos]uint32),
	}
}

// GetBlockAt implements the World interface used by pathfinding
// Returns the block state ID at the given position, or 0 (air) if not set
func (mw *MockWorld) GetBlockAt(x, y, z float64) uint32 {
	mw.mu.RLock()
	defer mw.mu.RUnlock()

	pos := BlockPos{X: x, Y: y, Z: z}
	if stateID, exists := mw.blocks[pos]; exists {
		return stateID
	}
	return 0 // Air
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
