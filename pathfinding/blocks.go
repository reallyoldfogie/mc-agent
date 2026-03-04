package pathfinding

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
	mdl "github.com/reallyoldfogie/mc-data-gen/loader"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
)

// blockShapeManager implements BlockShapeManager
type blockShapeManager struct {
	version    string
	shapeData  map[mdl.StateKey]mdl.ShapeInfo
	blockMgr   mc_versions.BlockMgr
	stateProps *StatePropertyLoader
}

// NewBlockShapeManager creates a manager for a specific Minecraft version
// dataBasePath should point to the mc-data-gen data directory (e.g., "/path/to/mc-data-gen/data")
func NewBlockShapeManager(
	version string,
	dataBasePath string,
	blockMgr mc_versions.BlockMgr,
	stateProps *StatePropertyLoader,
) (models.BlockShapeManager, error) {
	// Construct path to blocks directory for this version
	// e.g., "/path/to/mc-data-gen/data/1.21.5/blocks"
	blocksPath := filepath.Join(dataBasePath, version, "blocks")
	fullPath, _ := filepath.Abs(blocksPath)

	log.Printf("[BlockShapeManager] Loading block shapes for version %s from %s", version, fullPath)

	// Load all block shape data for this version
	data, err := loadBlocksDirInPlace(blocksPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load blocks for version %s: %w", version, err)
	}
	if envBool("MC_AGENT_MEM_STATS") {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		log.Printf("[MemStats] BlockShape load: HeapAlloc=%.1fMB TotalAlloc=%.1fMB Sys=%.1fMB NumGC=%d",
			float64(ms.HeapAlloc)/1024/1024,
			float64(ms.TotalAlloc)/1024/1024,
			float64(ms.Sys)/1024/1024,
			ms.NumGC,
		)
	}

	return &blockShapeManager{
		version:    version,
		shapeData:  data,
		blockMgr:   blockMgr,
		stateProps: stateProps,
	}, nil
}

func loadBlocksDirInPlace(root string) (map[mdl.StateKey]mdl.ShapeInfo, error) {
	// Check if directory exists before walking
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("blocks directory does not exist or is inaccessible: %s (error: %w)", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("blocks path is not a directory: %s", root)
	}

	out := make(map[mdl.StateKey]mdl.ShapeInfo)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		tmpOut, err := mdl.LoadBlocksFile(path)
		if err != nil {
			return fmt.Errorf("failed to load file %s: %s", path, err.Error())
		}
		for k, v := range tmpOut {
			out[k] = v
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk blocks directory: %w", err)
	}

	if len(out) == 0 {
		log.Printf("[BlockShapeManager] Warning: no block shape data loaded from %s", root)
	} else {
		log.Printf("[BlockShapeManager] Loaded %d block shapes from %s", len(out), root)
	}

	return out, nil
}

func envBool(key string) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return false
	}
	switch strings.ToLower(val) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// getInfo is a helper to retrieve ShapeInfo for a block state
func (bsm *blockShapeManager) getInfo(blockID string, props map[string]string) mdl.ShapeInfo {
	key := mdl.StateKey{
		BlockID:  blockID,
		PropsKey: mdl.MakePropsKey(props),
	}

	info, ok := bsm.shapeData[key]
	if !ok {
		log.Printf("[BlockShapeManager.getInfo] StateKey not found: blockID=%s, propsKey=%s (from props=%v)",
			blockID, key.PropsKey, props)

		// If exact match not found and props is nil/empty, try to find ANY state for this block
		// This handles the case where we don't have state properties but need basic block info
		if len(props) == 0 {
			for k, v := range bsm.shapeData {
				if k.BlockID == blockID {
					// Found a state for this block - use it
					// (all states of a block should have same solid/passable/dangerous properties)
					log.Printf("[BlockShapeManager.getInfo] Using fallback state for blockID=%s: propsKey=%s, IsStair=%v, IsSlab=%v",
						blockID, k.PropsKey, v.IsStair(), v.IsSlab())
					return v
				}
			}
		}
		// Return empty/air-like info for unknown blocks
		log.Printf("[BlockShapeManager.getInfo] No match found, returning Air=true for blockID=%s", blockID)
		return mdl.ShapeInfo{Air: true}
	}

	log.Printf("[BlockShapeManager.getInfo] Found StateKey: blockID=%s, propsKey=%s, IsStair=%v, IsSlab=%v",
		blockID, key.PropsKey, info.IsStair(), info.IsSlab())
	return info
}

func (bsm *blockShapeManager) blockInfoFromStateID(blockStateID uint32) (string, map[string]string) {
	if blockStateID == 0 {
		return "minecraft:air", map[string]string{}
	}
	if bsm.blockMgr == nil {
		log.Printf("[BlockShapeManager.blockInfoFromStateID] blockMgr is nil for stateID=%d", blockStateID)
		return "", map[string]string{}
	}
	if blockID, ok := bsm.blockMgr.BlockIDByStateID(uint32(blockStateID)); ok {
		if block, ok := bsm.blockMgr.GetByID(blockID); ok {
			if block.Name == "" {
				log.Printf("[BlockShapeManager.blockInfoFromStateID] block.Name is empty for stateID=%d, blockID=%d", blockStateID, blockID)
				return "", map[string]string{}
			}
			if bsm.stateProps == nil {
				log.Printf("[BlockShapeManager.blockInfoFromStateID] stateProps is nil for stateID=%d, returning name=%s with empty props", blockStateID, block.Name)
				return block.Name, map[string]string{}
			}
			props := bsm.stateProps.GetProperties(uint32(blockStateID))
			log.Printf("[BlockShapeManager.blockInfoFromStateID] stateID=%d -> name=%s, props=%v", blockStateID, block.Name, props)
			return block.Name, props
		}
	}

	log.Printf("[BlockShapeManager.blockInfoFromStateID] failed to find block for stateID=%d", blockStateID)
	return "", map[string]string{}
}

func (bsm *blockShapeManager) getInfoFromStateID(blockStateID uint32) mdl.ShapeInfo {
	blockName, props := bsm.blockInfoFromStateID(blockStateID)
	if blockName == "" {
		return mdl.ShapeInfo{Air: true}
	}
	return bsm.getInfo(blockName, props)
}

func (bsm *blockShapeManager) BlockName(blockStateID uint32) string {
	blockName, _ := bsm.blockInfoFromStateID(blockStateID)
	return blockName
}

func (bsm *blockShapeManager) FullBlockName(blockStateID uint32) string {
	blockName, props := bsm.blockInfoFromStateID(blockStateID)
	if blockName == "" {
		return ""
	}
	if len(props) == 0 {
		return blockName
	}
	propsParts := make([]string, 0, len(props))
	for k, v := range props {
		propsParts = append(propsParts, fmt.Sprintf("%s=%s", k, v))
	}
	return fmt.Sprintf("%s[%s]", blockName, strings.Join(propsParts, ","))
}

// IsPassable checks if a block allows entity movement
func (bsm *blockShapeManager) IsPassable(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsPassable()
}

// IsSolid checks if a block is a solid standing surface
func (bsm *blockShapeManager) IsSolid(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.SolidBlock || info.IsStandingSurface() > 0
}

// GetCollisionBoxes returns world-space collision AABBs
func (bsm *blockShapeManager) GetCollisionBoxes(blockStateID uint32, x, y, z int) []models.AABB {
	info := bsm.getInfoFromStateID(blockStateID)
	boxes := info.WorldCollisionBoxesAt(x, y, z)
	if len(boxes) == 0 {
		return nil
	}
	out := make([]models.AABB, len(boxes))
	for i, box := range boxes {
		out[i] = models.NewAABB(
			box.Min.X, box.Min.Y, box.Min.Z,
			box.Max.X, box.Max.Y, box.Max.Z,
		)
		out[i].BlockID = blockStateID
	}
	return out
}

// GetStandingSurfaceHeight returns the maximum Y of the collision shape (0-1)
func (bsm *blockShapeManager) GetStandingSurfaceHeight(blockStateID uint32) float64 {
	info := bsm.getInfoFromStateID(blockStateID)
	height := info.IsStandingSurface()

	// Debug logging for stairs and slabs
	if info.IsStair() || info.IsSlab() {
		blockName, props := bsm.blockInfoFromStateID(blockStateID)
		log.Printf("[BlockShapeManager.GetStandingSurfaceHeight] blockStateID=%d, name=%s, props=%v, height=%.2f",
			blockStateID, blockName, props, height)
	}

	return height
}

// IsClimbable checks if a block can be climbed (ladders, vines, scaffolding)
func (bsm *blockShapeManager) IsClimbable(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsClimbable()
}

// IsFluid checks if a block is any fluid
func (bsm *blockShapeManager) IsFluid(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsFluid()
}

// IsWater checks if a block is water
func (bsm *blockShapeManager) IsWater(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsWater()
}

// IsLava checks if a block is lava
func (bsm *blockShapeManager) IsLava(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsLava()
}

// IsDangerous detects dangerous blocks (lava, fire, magma blocks, cacti, etc.)
func (bsm *blockShapeManager) IsDangerous(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)

	// Lava is always dangerous
	if info.IsLava() {
		return true
	}

	// Check for other dangerous blocks by ID
	switch bsm.BlockName(blockStateID) {
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
func (bsm *blockShapeManager) IsDoorLike(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsDoorLike()
}

// IsFenceLike checks if block is a fence/wall (1.5 blocks tall - not jumpable)
func (bsm *blockShapeManager) IsFenceLike(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsFenceLike()
}

// IsSlab checks if block is a slab
func (bsm *blockShapeManager) IsSlab(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsSlab()
}

// IsStair checks if block is stairs
func (bsm *blockShapeManager) IsStair(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsStair()
}

// IsLogOrLeaf checks if block is tree log/leaf
func (bsm *blockShapeManager) IsLogOrLeaf(blockStateID uint32) bool {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.IsLogOrLeaf()
}

// IsHayBale checks if block is a hay bale.
func (bsm *blockShapeManager) IsHayBale(blockStateID uint32) bool {
	return bsm.BlockName(blockStateID) == "minecraft:hay_block"
}

// IsBed checks if block is a bed (any color).
func (bsm *blockShapeManager) IsBed(blockStateID uint32) bool {
	name := bsm.BlockName(blockStateID)
	return name != "" && strings.HasSuffix(name, "_bed")
}

// IsHoneyBlock checks if block is a honey block.
func (bsm *blockShapeManager) IsHoneyBlock(blockStateID uint32) bool {
	return bsm.BlockName(blockStateID) == "minecraft:honey_block"
}

// IsSlimeBlock checks if block is a slime block.
func (bsm *blockShapeManager) IsSlimeBlock(blockStateID uint32) bool {
	return bsm.BlockName(blockStateID) == "minecraft:slime_block"
}

// IsPowderSnow checks if block is powder snow.
func (bsm *blockShapeManager) IsPowderSnow(blockStateID uint32) bool {
	return bsm.BlockName(blockStateID) == "minecraft:powder_snow"
}
