package testing

import (
	"fmt"
	"log"
	"strings"
	"testing"

	agentmodels "github.com/reallyoldfogie/mc-agent/models"
	protocolmodels "github.com/reallyoldfogie/mc-protocol-go/models"
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
	if !strings.HasPrefix(blockName, "minecraft:") {
		blockName = "minecraft:" + blockName
	}

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
func (mbm *MockBlockManager) BlockIDByStateID(stateID uint32) protocolmodels.BlockID {
	return protocolmodels.BlockID(stateID)
}

// GetByID returns block info by ID
func (mbm *MockBlockManager) GetByID(blockID protocolmodels.BlockID) protocolmodels.Block {
	// Find block name by ID
	for name, id := range mbm.registry.blocks {
		if protocolmodels.BlockID(id) == blockID {
			return protocolmodels.Block{Name: name}
		}
	}
	return protocolmodels.Block{Name: "minecraft:unknown"}
}

// BitsPerBlock returns the number of bits per block (mock returns 8)
func (mbm *MockBlockManager) BitsPerBlock() int {
	return 8 // Standard bits per block value for testing
}

// MockShapeManager is a mock implementation of pathfinding's BlockShapeManager
type MockShapeManager struct {
	blockMgr       *MockBlockManager
	solidBlocks    map[string]bool
	passableBlocks map[string]bool
	surfaceHeights map[string]float64
}

// NewMockShapeManager creates a mock shape manager with common block properties
func NewMockShapeManager() *MockShapeManager {
	msm := &MockShapeManager{
		blockMgr:       NewMockBlockManager(),
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
	msm.surfaceHeights["minecraft:oak_stairs"] = 1.0 // Stairs are full block height (top surface at Y+1.0)

	msm.solidBlocks["minecraft:oak_slab"] = true
	msm.surfaceHeights["minecraft:oak_slab"] = 0.5 // Bottom slabs are half block height (top surface at Y+0.5)

	// Passable blocks (can walk through)
	passableBlocks := []string{
		"minecraft:air", "minecraft:water", "minecraft:ladder",
	}
	for _, block := range passableBlocks {
		msm.passableBlocks[block] = true
	}

	return msm
}

func (msm *MockShapeManager) blockName(blockStateID uint32) string {
	if blockStateID == 0 {
		return "minecraft:air"
	}
	if msm.blockMgr == nil {
		return "minecraft:unknown"
	}
	blockID := msm.blockMgr.BlockIDByStateID(uint32(blockStateID))
	block := msm.blockMgr.GetByID(blockID)
	return block.Name
}

// IsSolid returns whether a block is solid (provides ground support)
func (msm *MockShapeManager) IsSolid(blockStateID uint32) bool {
	blockName := msm.blockName(blockStateID)
	return msm.solidBlocks[blockName]
}

// IsPassable returns whether a block can be walked through
func (msm *MockShapeManager) IsPassable(blockStateID uint32) bool {
	blockName := msm.blockName(blockStateID)
	if msm.passableBlocks[blockName] {
		return true
	}
	// Non-solid blocks are passable by default
	return !msm.solidBlocks[blockName]
}

// GetStandingSurfaceHeight returns the Y offset where a player stands on this block
func (msm *MockShapeManager) GetStandingSurfaceHeight(blockStateID uint32) float64 {
	blockName := msm.blockName(blockStateID)
	if height, exists := msm.surfaceHeights[blockName]; exists {
		return height
	}
	return 0.0 // Unknown blocks have no surface
}

// IsClimbable returns whether a block can be climbed
func (msm *MockShapeManager) IsClimbable(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:ladder"
}

// IsWater returns whether a block is water
func (msm *MockShapeManager) IsWater(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:water"
}

// IsLava returns whether a block is lava
func (msm *MockShapeManager) IsLava(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:lava"
}

// IsSlab returns whether a block is a slab
func (msm *MockShapeManager) IsSlab(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:oak_slab"
}

// IsStair returns whether a block is stairs
func (msm *MockShapeManager) IsStair(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:oak_stairs"
}

// GetCollisionBoxes returns collision boxes for a block (mock returns simple box for solid blocks)
func (msm *MockShapeManager) GetCollisionBoxes(blockStateID uint32, x, y, z int) []agentmodels.AABB {
	if msm.IsSolid(blockStateID) {
		// Return a simple full-block collision box
		return []agentmodels.AABB{
			agentmodels.NewAABB(
				float64(x), float64(y), float64(z),
				float64(x+1), float64(y+1), float64(z+1),
			),
		}
	}
	// No collision for non-solid blocks
	return []agentmodels.AABB{}
}

// IsFluid returns whether a block is a fluid
func (msm *MockShapeManager) IsFluid(blockStateID uint32) bool {
	blockName := msm.blockName(blockStateID)
	return blockName == "minecraft:water" || blockName == "minecraft:lava"
}

// IsDangerous returns whether a block is dangerous
func (msm *MockShapeManager) IsDangerous(blockStateID uint32) bool {
	blockName := msm.blockName(blockStateID)
	return blockName == "minecraft:lava" || blockName == "minecraft:fire" || blockName == "minecraft:cactus"
}

// IsDoorLike returns whether a block is door-like
func (msm *MockShapeManager) IsDoorLike(blockStateID uint32) bool {
	blockName := msm.blockName(blockStateID)
	return blockName == "minecraft:oak_door" || blockName == "minecraft:iron_door"
}

// IsFenceLike returns whether a block is fence-like
func (msm *MockShapeManager) IsFenceLike(blockStateID uint32) bool {
	blockName := msm.blockName(blockStateID)
	return blockName == "minecraft:oak_fence" || blockName == "minecraft:fence_gate"
}

// IsLogOrLeaf returns whether a block is a log or leaf
func (msm *MockShapeManager) IsLogOrLeaf(blockStateID uint32) bool {
	blockName := msm.blockName(blockStateID)
	return blockName == "minecraft:oak_log" || blockName == "minecraft:oak_leaves"
}

// IsHayBale returns whether a block is a hay bale
func (msm *MockShapeManager) IsHayBale(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:hay_block"
}

// IsBed returns whether a block is a bed
func (msm *MockShapeManager) IsBed(blockStateID uint32) bool {
	blockName := msm.blockName(blockStateID)
	return strings.HasSuffix(blockName, "_bed")
}

// IsHoneyBlock returns whether a block is a honey block
func (msm *MockShapeManager) IsHoneyBlock(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:honey_block"
}

// IsSlimeBlock returns whether a block is a slime block
func (msm *MockShapeManager) IsSlimeBlock(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:slime_block"
}

// IsPowderSnow returns whether a block is powder snow
func (msm *MockShapeManager) IsPowderSnow(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:powder_snow"
}

// IsCobweb returns whether a block is a cobweb
func (msm *MockShapeManager) IsCobweb(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:cobweb"
}

// IsScaffolding returns whether a block is scaffolding
func (msm *MockShapeManager) IsScaffolding(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:scaffolding"
}

// IsIce returns whether a block is ice, packed ice, or frosted ice.
func (msm *MockShapeManager) IsIce(blockStateID uint32) bool {
	switch msm.blockName(blockStateID) {
	case "minecraft:ice", "minecraft:packed_ice", "minecraft:frosted_ice":
		return true
	default:
		return false
	}
}

// IsBlueIce returns whether a block is blue ice.
func (msm *MockShapeManager) IsBlueIce(blockStateID uint32) bool {
	return msm.blockName(blockStateID) == "minecraft:blue_ice"
}

// BlockName returns the block name for a given block state ID
func (msm *MockShapeManager) BlockName(blockStateID uint32) string {
	return msm.blockName(blockStateID)
}

// FullBlockName returns the full block name (same as BlockName in this mock)
func (msm *MockShapeManager) FullBlockName(blockStateID uint32) string {
	return msm.blockName(blockStateID)
}

// GetBlockProperties returns block state properties (stub for mock - returns empty map)
func (msm *MockShapeManager) GetBlockProperties(blockStateID uint32) map[string]string {
	return map[string]string{}
}

// GetWaterFlowDirection returns the flow direction (stub for mock - returns zero vector)
func (msm *MockShapeManager) GetWaterFlowDirection(x, y, z int, world agentmodels.World) agentmodels.V3 {
	return agentmodels.V3{} // No flow in mock
}

// GetWaterFlowSpeed returns the flow speed (stub for mock - returns zero)
func (msm *MockShapeManager) GetWaterFlowSpeed(blockStateID uint32) float64 {
	return 0.0 // No flow in mock
}

// GetMiningInfo returns mining properties (stub for mock)
func (msm *MockShapeManager) GetMiningInfo(blockStateID uint32) (float64, []string, bool) {
	if blockStateID == 0 {
		return 0, nil, false
	}
	return 1.5, []string{"mineable/pickaxe"}, true
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

// TestLogHelper wraps testing.T and provides dual logging to both test output and agent log file.
type TestLogHelper struct {
	t *testing.T
}

// NewTestLogger creates a new TestLogHelper that writes to both test output and log file.
func NewTestLogger(t *testing.T) *TestLogHelper {
	return &TestLogHelper{t: t}
}

// Logf logs a formatted message to both test output and the agent log file.
func (h *TestLogHelper) Logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	h.t.Helper()
	h.t.Log(msg)
	log.Println(msg)
}

// Log logs a message to both test output and the agent log file.
func (h *TestLogHelper) Log(args ...any) {
	msg := fmt.Sprint(args...)
	h.t.Helper()
	h.t.Log(msg)
	log.Println(msg)
}
