package testing

import "github.com/reallyoldfogie/mc-agent/pathfinding"

// BlockRegistry provides block state ID lookup for world building
// This is a simplified interface - real implementation would use mc-protocol-go BlockManager
type BlockRegistry interface {
	GetStateID(blockName string, properties map[string]string) uint32
}

// WorldBuilder provides a fluent API for constructing test worlds
type WorldBuilder struct {
	world         *MockWorld
	blockRegistry BlockRegistry
}

func newWorld() pathfinding.World {
	return &MockWorld{}
}

// NewWorldBuilder creates a new world builder
// If blockRegistry is nil, you must use SetBlockDirect() with state IDs
func NewWorldBuilder(blockRegistry BlockRegistry) *WorldBuilder {
	return &WorldBuilder{
		world:         NewMockWorld(),
		blockRegistry: blockRegistry,
	}
}

// FlatGround creates a flat horizontal platform of blocks
func (wb *WorldBuilder) FlatGround(x1, z1, x2, z2, y float64, blockName string) *WorldBuilder {
	if wb.blockRegistry == nil {
		panic("BlockRegistry required for FlatGround - use NewWorldBuilder(registry) or SetBlockDirect()")
	}

	stateID := wb.blockRegistry.GetStateID(blockName, nil)

	for x := x1; x <= x2; x++ {
		for z := z1; z <= z2; z++ {
			wb.world.SetBlock(x, y, z, stateID)
		}
	}

	return wb
}

// FlatGroundDirect creates a flat platform using a state ID directly
func (wb *WorldBuilder) FlatGroundDirect(x1, z1, x2, z2, y float64, stateID uint32) *WorldBuilder {
	for x := x1; x <= x2; x++ {
		for z := z1; z <= z2; z++ {
			wb.world.SetBlock(x, y, z, stateID)
		}
	}

	return wb
}

// Stairs creates a staircase
func (wb *WorldBuilder) Stairs(startX, startY, startZ float64, length int, direction string, blockName string) *WorldBuilder {
	if wb.blockRegistry == nil {
		panic("BlockRegistry required for Stairs")
	}

	// Get stair state ID with properties
	stateID := wb.blockRegistry.GetStateID(blockName, map[string]string{
		"facing": direction,
		"half":   "bottom",
	})

	wb.StairsDirect(startX, startY, startZ, length, direction, stateID)
	return wb
}

// StairsDirect creates a staircase using state ID directly
func (wb *WorldBuilder) StairsDirect(startX, startY, startZ float64, length int, direction string, stateID uint32) *WorldBuilder {
	for i := range length {
		floatI := float64(i)
		switch direction {
		case "north":
			wb.world.SetBlock(startX, startY+floatI, startZ-floatI, stateID)
		case "south":
			wb.world.SetBlock(startX, startY+floatI, startZ+floatI, stateID)
		case "east":
			wb.world.SetBlock(startX+floatI, startY+floatI, startZ, stateID)
		case "west":
			wb.world.SetBlock(startX-floatI, startY+floatI, startZ, stateID)
		}
	}

	return wb
}

// Wall creates a vertical wall
func (wb *WorldBuilder) Wall(x1, z1, x2, z2, yBottom, yTop float64, blockName string) *WorldBuilder {
	if wb.blockRegistry == nil {
		panic("BlockRegistry required for Wall")
	}

	stateID := wb.blockRegistry.GetStateID(blockName, nil)
	wb.world.SetBlockRange(x1, yBottom, z1, x2, yTop, z2, stateID)

	return wb
}

// WallDirect creates a vertical wall using state ID directly
func (wb *WorldBuilder) WallDirect(x1, z1, x2, z2, yBottom, yTop float64, stateID uint32) *WorldBuilder {
	wb.world.SetBlockRange(x1, yBottom, z1, x2, yTop, z2, stateID)
	return wb
}

// Gap creates a gap (air) in existing terrain
func (wb *WorldBuilder) Gap(x, z, y float64, width float64) *WorldBuilder {
	for i := float64(0); i < width; i++ {
		wb.world.SetBlock(x+i, y, z, 0) // Air
	}
	return wb
}

// Water fills an area with water
func (wb *WorldBuilder) Water(x1, y1, z1, x2, y2, z2 float64, blockName string) *WorldBuilder {
	if wb.blockRegistry == nil {
		panic("BlockRegistry required for Water")
	}

	stateID := wb.blockRegistry.GetStateID(blockName, nil)
	wb.world.SetBlockRange(x1, y1, z1, x2, y2, z2, stateID)

	return wb
}

// WaterDirect fills an area with water using state ID directly
func (wb *WorldBuilder) WaterDirect(x1, y1, z1, x2, y2, z2 float64, stateID uint32) *WorldBuilder {
	wb.world.SetBlockRange(x1, y1, z1, x2, y2, z2, stateID)
	return wb
}

// Ladder places a vertical ladder
func (wb *WorldBuilder) Ladder(x, z, yBottom, yTop float64, facing string, blockName string) *WorldBuilder {
	if wb.blockRegistry == nil {
		panic("BlockRegistry required for Ladder")
	}

	stateID := wb.blockRegistry.GetStateID(blockName, map[string]string{
		"facing": facing,
	})

	wb.LadderDirect(x, z, yBottom, yTop, stateID)

	return wb
}

// LadderDirect places a vertical ladder using state ID directly
func (wb *WorldBuilder) LadderDirect(x, z, yBottom, yTop float64, stateID uint32) *WorldBuilder {
	for y := yBottom; y <= yTop; y++ {
		wb.world.SetBlock(x, y, z, stateID)
	}
	return wb
}

// SetBlock sets a single block (for custom scenarios)
func (wb *WorldBuilder) SetBlock(x, y, z float64, blockName string, properties map[string]string) *WorldBuilder {
	if wb.blockRegistry == nil {
		panic("BlockRegistry required for SetBlock")
	}

	stateID := wb.blockRegistry.GetStateID(blockName, properties)
	wb.world.SetBlock(x, y, z, stateID)

	return wb
}

// SetBlockDirect sets a single block using state ID directly
func (wb *WorldBuilder) SetBlockDirect(x, y, z float64, stateID uint32) *WorldBuilder {
	wb.world.SetBlock(x, y, z, stateID)
	return wb
}

// Build returns the constructed world
func (wb *WorldBuilder) Build() *MockWorld {
	return wb.world
}
