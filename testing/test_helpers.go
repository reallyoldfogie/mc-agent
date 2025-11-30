package testing

import (
	"fmt"

	mdl "github.com/reallyoldfogie/mc-data-gen/loader"
	"github.com/reallyoldfogie/mc-protocol-go/models"
)

// SimpleBlockRegistry is a simple mock block registry for testing
// Maps block names to hardcoded state IDs
type SimpleBlockRegistry struct {
	blocks map[string]uint32
	nextID uint32
}

// NewSimpleBlockRegistry creates a simple block registry with common blocks
func NewSimpleBlockRegistry() *SimpleBlockRegistry {
	registry := &SimpleBlockRegistry{
		blocks: make(map[string]uint32),
		nextID: 1,
	}

	// Register common blocks with fixed IDs
	registry.Register("minecraft:air", 0)
	registry.Register("minecraft:grass_block", 9)
	registry.Register("minecraft:stone", 1)
	registry.Register("minecraft:dirt", 10)
	registry.Register("minecraft:oak_planks", 15)
	registry.Register("minecraft:oak_stairs", 3391)
	registry.Register("minecraft:cobblestone", 14)
	registry.Register("minecraft:water", 34)
	registry.Register("minecraft:ladder", 3653)
	registry.Register("minecraft:oak_slab", 8301)

	return registry
}

// Register adds a block to the registry
func (sbr *SimpleBlockRegistry) Register(blockName string, stateID uint32) {
	sbr.blocks[blockName] = stateID
	if stateID >= sbr.nextID {
		sbr.nextID = stateID + 1
	}
}

// GetStateID returns the state ID for a block name
// Properties are ignored in this simple implementation
func (sbr *SimpleBlockRegistry) GetStateID(blockName string, properties map[string]string) uint32 {
	if stateID, exists := sbr.blocks[blockName]; exists {
		return stateID
	}

	// Auto-register unknown blocks with next available ID
	stateID := sbr.nextID
	sbr.nextID++
	sbr.blocks[blockName] = stateID
	return stateID
}

// MockBlockManager is a mock implementation of pathfinding's BlockManager interface
// Used for testing when real BlockManager is not available
type MockBlockManager struct {
	registry *SimpleBlockRegistry
}

// NewMockBlockManager creates a mock block manager
func NewMockBlockManager() *MockBlockManager {
	return &MockBlockManager{
		registry: NewSimpleBlockRegistry(),
	}
}

// BlockIDByStateID converts state ID to block ID (simplified - returns state ID as BlockID)
func (mbm *MockBlockManager) BlockIDByStateID(stateID uint32) models.BlockID {
	return models.BlockID(stateID)
}

// GetByID returns block info by ID
func (mbm *MockBlockManager) GetByID(blockID models.BlockID) models.Block {
	// Find block name by ID
	for name, id := range mbm.registry.blocks {
		if models.BlockID(id) == blockID {
			return models.Block{Name: name}
		}
	}
	return models.Block{Name: "minecraft:unknown"}
}

// BitsPerBlock returns the number of bits per block (mock returns 8)
func (mbm *MockBlockManager) BitsPerBlock() int {
	return 8 // Standard bits per block value for testing
}

// MockShapeManager is a mock implementation of pathfinding's BlockShapeManager
type MockShapeManager struct {
	solidBlocks    map[string]bool
	passableBlocks map[string]bool
	surfaceHeights map[string]float64
}

// NewMockShapeManager creates a mock shape manager with common block properties
func NewMockShapeManager() *MockShapeManager {
	msm := &MockShapeManager{
		solidBlocks:    make(map[string]bool),
		passableBlocks: make(map[string]bool),
		surfaceHeights: make(map[string]float64),
	}

	// Solid blocks (provide ground support)
	solidBlocks := []string{
		"minecraft:grass_block", "minecraft:stone", "minecraft:dirt",
		"minecraft:cobblestone", "minecraft:oak_planks",
	}
	for _, block := range solidBlocks {
		msm.solidBlocks[block] = true
		msm.surfaceHeights[block] = 1.0 // Full block height
	}

	// Partial blocks
	msm.solidBlocks["minecraft:oak_stairs"] = true
	msm.surfaceHeights["minecraft:oak_stairs"] = 0.5 // Stairs surface at half height

	msm.solidBlocks["minecraft:oak_slab"] = true
	msm.surfaceHeights["minecraft:oak_slab"] = 0.5 // Slab surface at half height

	// Passable blocks (can walk through)
	passableBlocks := []string{
		"minecraft:air", "minecraft:water", "minecraft:ladder",
	}
	for _, block := range passableBlocks {
		msm.passableBlocks[block] = true
	}

	return msm
}

// IsSolid returns whether a block is solid (provides ground support)
func (msm *MockShapeManager) IsSolid(blockName string, props map[string]string) bool {
	return msm.solidBlocks[blockName]
}

// IsPassable returns whether a block can be walked through
func (msm *MockShapeManager) IsPassable(blockName string, props map[string]string) bool {
	if msm.passableBlocks[blockName] {
		return true
	}
	// Non-solid blocks are passable by default
	return !msm.solidBlocks[blockName]
}

// GetStandingSurfaceHeight returns the Y offset where a player stands on this block
func (msm *MockShapeManager) GetStandingSurfaceHeight(blockName string, props map[string]string) float64 {
	if height, exists := msm.surfaceHeights[blockName]; exists {
		return height
	}
	return 0.0 // Unknown blocks have no surface
}

// IsClimbable returns whether a block can be climbed
func (msm *MockShapeManager) IsClimbable(blockName string, props map[string]string) bool {
	return blockName == "minecraft:ladder"
}

// IsWater returns whether a block is water
func (msm *MockShapeManager) IsWater(blockName string, props map[string]string) bool {
	return blockName == "minecraft:water"
}

// IsLava returns whether a block is lava
func (msm *MockShapeManager) IsLava(blockName string, props map[string]string) bool {
	return blockName == "minecraft:lava"
}

// IsSlab returns whether a block is a slab
func (msm *MockShapeManager) IsSlab(blockName string, props map[string]string) bool {
	return blockName == "minecraft:oak_slab"
}

// IsStair returns whether a block is stairs
func (msm *MockShapeManager) IsStair(blockName string, props map[string]string) bool {
	return blockName == "minecraft:oak_stairs"
}

// GetCollisionBoxes returns collision boxes for a block (mock returns simple box for solid blocks)
func (msm *MockShapeManager) GetCollisionBoxes(blockName string, props map[string]string, x, y, z int) []mdl.AABB {
	if msm.IsSolid(blockName, props) {
		// Return a simple full-block collision box
		return []mdl.AABB{
			{
				Min: mdl.Vec3{X: float64(x), Y: float64(y), Z: float64(z)},
				Max: mdl.Vec3{X: float64(x + 1), Y: float64(y + 1), Z: float64(z + 1)},
			},
		}
	}
	// No collision for non-solid blocks
	return []mdl.AABB{}
}

// IsFluid returns whether a block is a fluid
func (msm *MockShapeManager) IsFluid(blockName string, props map[string]string) bool {
	return blockName == "minecraft:water" || blockName == "minecraft:lava"
}

// IsDangerous returns whether a block is dangerous
func (msm *MockShapeManager) IsDangerous(blockName string, props map[string]string) bool {
	return blockName == "minecraft:lava" || blockName == "minecraft:fire" || blockName == "minecraft:cactus"
}

// IsDoorLike returns whether a block is door-like
func (msm *MockShapeManager) IsDoorLike(blockName string, props map[string]string) bool {
	return blockName == "minecraft:oak_door" || blockName == "minecraft:iron_door"
}

// IsFenceLike returns whether a block is fence-like
func (msm *MockShapeManager) IsFenceLike(blockName string, props map[string]string) bool {
	return blockName == "minecraft:oak_fence" || blockName == "minecraft:fence_gate"
}

// IsLogOrLeaf returns whether a block is a log or leaf
func (msm *MockShapeManager) IsLogOrLeaf(blockName string, props map[string]string) bool {
	return blockName == "minecraft:oak_log" || blockName == "minecraft:oak_leaves"
}

// TestLogger provides logging for tests
type TestLogger struct {
	messages []string
}

// Printf implements a simple logger interface
func (tl *TestLogger) Printf(format string, args ...any) {
	tl.messages = append(tl.messages, fmt.Sprintf(format, args...))
}

// GetMessages returns all logged messages
func (tl *TestLogger) GetMessages() []string {
	return tl.messages
}

// Clear clears all logged messages
func (tl *TestLogger) Clear() {
	tl.messages = nil
}
