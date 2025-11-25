package pathfinding

import (
	"fmt"
	"path/filepath"

	mdl "github.com/reallyoldfogie/mc-data-gen/loader"
)

// BlockShapeManager loads and provides access to block shape data for a specific Minecraft version
type BlockShapeManager interface {
	// Core collision
	IsPassable(blockID string, props map[string]string) bool
	IsSolid(blockID string, props map[string]string) bool
	GetCollisionBoxes(blockID string, props map[string]string, x, y, z int) []mdl.AABB
	GetStandingSurfaceHeight(blockID string, props map[string]string) float64

	// Movement types
	IsClimbable(blockID string, props map[string]string) bool

	// Fluid detection
	IsFluid(blockID string, props map[string]string) bool
	IsWater(blockID string, props map[string]string) bool
	IsLava(blockID string, props map[string]string) bool

	// Danger detection
	IsDangerous(blockID string, props map[string]string) bool

	// Block type classification
	IsDoorLike(blockID string, props map[string]string) bool
	IsFenceLike(blockID string, props map[string]string) bool
	IsSlab(blockID string, props map[string]string) bool
	IsStair(blockID string, props map[string]string) bool
	IsLogOrLeaf(blockID string, props map[string]string) bool
}

// blockShapeManager implements BlockShapeManager
type blockShapeManager struct {
	version   string
	shapeData map[mdl.StateKey]mdl.ShapeInfo
}

// NewBlockShapeManager creates a manager for a specific Minecraft version
// dataBasePath should point to the mc-data-gen data directory (e.g., "/path/to/mc-data-gen/data")
func NewBlockShapeManager(version string, dataBasePath string) (BlockShapeManager, error) {
	// Construct path to blocks directory for this version
	// e.g., "/path/to/mc-data-gen/data/1.21.5/blocks"
	blocksPath := filepath.Join(dataBasePath, version, "blocks")

	// Load all block shape data for this version
	data, err := mdl.LoadBlocksDir(blocksPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load blocks for version %s: %w", version, err)
	}

	return &blockShapeManager{
		version:   version,
		shapeData: data,
	}, nil
}

// getInfo is a helper to retrieve ShapeInfo for a block state
func (bsm *blockShapeManager) getInfo(blockID string, props map[string]string) mdl.ShapeInfo {
	key := mdl.StateKey{
		BlockID:  blockID,
		PropsKey: mdl.MakePropsKey(props),
	}

	info, ok := bsm.shapeData[key]
	if !ok {
		// Return empty/air-like info for unknown blocks
		return mdl.ShapeInfo{Air: true}
	}

	return info
}

// IsPassable checks if a block allows entity movement
func (bsm *blockShapeManager) IsPassable(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsPassable()
}

// IsSolid checks if a block is a solid standing surface
func (bsm *blockShapeManager) IsSolid(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.SolidBlock || info.IsStandingSurface() > 0
}

// GetCollisionBoxes returns world-space collision AABBs
func (bsm *blockShapeManager) GetCollisionBoxes(blockID string, props map[string]string, x, y, z int) []mdl.AABB {
	info := bsm.getInfo(blockID, props)
	return info.WorldCollisionBoxesAt(x, y, z)
}

// GetStandingSurfaceHeight returns the maximum Y of the collision shape (0-1)
func (bsm *blockShapeManager) GetStandingSurfaceHeight(blockID string, props map[string]string) float64 {
	info := bsm.getInfo(blockID, props)
	return info.IsStandingSurface()
}

// IsClimbable checks if a block can be climbed (ladders, vines, scaffolding)
func (bsm *blockShapeManager) IsClimbable(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsClimbable()
}

// IsFluid checks if a block is any fluid
func (bsm *blockShapeManager) IsFluid(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsFluid()
}

// IsWater checks if a block is water
func (bsm *blockShapeManager) IsWater(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsWater()
}

// IsLava checks if a block is lava
func (bsm *blockShapeManager) IsLava(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsLava()
}

// IsDangerous detects dangerous blocks (lava, fire, magma blocks, cacti, etc.)
func (bsm *blockShapeManager) IsDangerous(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)

	// Lava is always dangerous
	if info.IsLava() {
		return true
	}

	// Check for other dangerous blocks by ID
	switch blockID {
	case "minecraft:fire", "minecraft:soul_fire":
		return true
	case "minecraft:magma_block":
		return true
	case "minecraft:cactus":
		return true
	case "minecraft:sweet_berry_bush":
		return true
	case "minecraft:wither_rose":
		return true
	}

	return false
}

// IsDoorLike checks if block is a door/trapdoor/gate
func (bsm *blockShapeManager) IsDoorLike(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsDoorLike()
}

// IsFenceLike checks if block is a fence/wall (1.5 blocks tall - not jumpable)
func (bsm *blockShapeManager) IsFenceLike(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsFenceLike()
}

// IsSlab checks if block is a slab
func (bsm *blockShapeManager) IsSlab(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsSlab()
}

// IsStair checks if block is stairs
func (bsm *blockShapeManager) IsStair(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsStair()
}

// IsLogOrLeaf checks if block is tree log/leaf
func (bsm *blockShapeManager) IsLogOrLeaf(blockID string, props map[string]string) bool {
	info := bsm.getInfo(blockID, props)
	return info.IsLogOrLeaf()
}
