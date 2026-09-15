package pathfinding

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
	mdl "github.com/reallyoldfogie/mc-data-gen/loader"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
)

// blockShapeManager implements BlockShapeManager
type blockShapeManager struct {
	version    string
	shapeData  map[mdl.StateKey]mdl.ShapeInfo
	blockMgr   mc_versions.BlockMgr
	stateProps *StatePropertyLoader
	logger     *slog.Logger
}

// NewBlockShapeManager creates a manager for a specific Minecraft version
// dataBasePath should point to the mc-data-gen data directory (e.g., "/path/to/mc-data-gen/data")
func NewBlockShapeManager(
	version string,
	dataBasePath string,
	blockMgr mc_versions.BlockMgr,
	stateProps *StatePropertyLoader,
	logger *slog.Logger,
) (models.BlockShapeManager, error) {
	logger = utils.SafeLogger(logger)

	// Construct path to blocks directory for this version
	// e.g., "/path/to/mc-data-gen/data/1.21.5/blocks"
	blocksPath := filepath.Join(dataBasePath, version, "blocks")
	fullPath, _ := filepath.Abs(blocksPath)

	logger.Info("[BlockShapeManager] loading block shapes", "version", version, "path", fullPath)

	// Load all block shape data for this version
	data, err := loadBlocksDirInPlace(logger, blocksPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load blocks for version %s: %w", version, err)
	}
	if envBool("MC_AGENT_MEM_STATS") {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		logger.Debug("[MemStats] BlockShape load",
			"heapAllocMB", float64(ms.HeapAlloc)/1024/1024,
			"totalAllocMB", float64(ms.TotalAlloc)/1024/1024,
			"sysMB", float64(ms.Sys)/1024/1024,
			"numGC", ms.NumGC,
		)
	}

	return &blockShapeManager{
		version:    version,
		shapeData:  data,
		blockMgr:   blockMgr,
		stateProps: stateProps,
		logger:     logger,
	}, nil
}

func loadBlocksDirInPlace(logger *slog.Logger, root string) (map[mdl.StateKey]mdl.ShapeInfo, error) {
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
		logger.Warn("[BlockShapeManager] no block shape data loaded", "path", root)
	} else {
		logger.Info("[BlockShapeManager] loaded block shapes", "count", len(out), "path", root)
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

// getInfo is a helper to retrieve ShapeInfo for a block state. Logging
// (including the found-a-match, everything-is-fine case) is gated behind
// MC_AGENT_VERBOSE_LOG (see bsm.verbose's own doc comment) — this is
// called from every collision/passability query, so unconditional logging
// here (as this used to do) compounds into gigabytes of output over any
// real movement-heavy session, dominating wall-clock time; found live
// during an RL training run, 2026-09-09.
func (bsm *blockShapeManager) getInfo(blockID string, props map[string]string) mdl.ShapeInfo {
	key := mdl.StateKey{
		BlockID:  blockID,
		PropsKey: mdl.MakePropsKey(props),
	}

	info, ok := bsm.shapeData[key]
	if !ok {
		utils.DebugVerbose(bsm.logger, "[BlockShapeManager.getInfo] StateKey not found", "blockID", blockID, "propsKey", key.PropsKey, "props", props)

		// If exact match not found and props is nil/empty, try to find ANY state for this block
		// This handles the case where we don't have state properties but need basic block info
		if len(props) == 0 {
			for k, v := range bsm.shapeData {
				if k.BlockID == blockID {
					// Found a state for this block - use it
					// (all states of a block should have same solid/passable/dangerous properties)
					utils.DebugVerbose(bsm.logger, "[BlockShapeManager.getInfo] using fallback state",
						"blockID", blockID, "propsKey", k.PropsKey, "isStair", v.IsStair(), "isSlab", v.IsSlab())
					return v
				}
			}
		}
		// Return empty/air-like info for unknown blocks
		utils.DebugVerbose(bsm.logger, "[BlockShapeManager.getInfo] no match found, returning Air=true", "blockID", blockID)
		return mdl.ShapeInfo{Air: true}
	}

	utils.DebugVerbose(bsm.logger, "[BlockShapeManager.getInfo] found StateKey",
		"blockID", blockID, "propsKey", key.PropsKey, "isStair", info.IsStair(), "isSlab", info.IsSlab())
	return info
}

// blockInfoFromStateID resolves a raw block state ID to a name and state
// properties. Logging is gated behind MC_AGENT_VERBOSE_LOG — see
// getInfo's doc comment; this is called from every collision/passability
// query via getInfoFromStateID (and directly elsewhere in this file), so
// the same unconditional-logging-compounds-into-gigabytes concern applies
// here too.
func (bsm *blockShapeManager) blockInfoFromStateID(blockStateID uint32) (string, map[string]string) {
	if blockStateID == 0 {
		return "minecraft:air", map[string]string{}
	}
	if bsm.blockMgr == nil {
		utils.DebugVerbose(bsm.logger, "[BlockShapeManager.blockInfoFromStateID] blockMgr is nil", "stateID", blockStateID)
		return "", map[string]string{}
	}
	if blockID, ok := bsm.blockMgr.BlockIDByStateID(uint32(blockStateID)); ok {
		if block, ok := bsm.blockMgr.GetByID(blockID); ok {
			if block.Name == "" {
				utils.DebugVerbose(bsm.logger, "[BlockShapeManager.blockInfoFromStateID] block.Name is empty", "stateID", blockStateID, "blockID", blockID)
				return "", map[string]string{}
			}
			if bsm.stateProps == nil {
				utils.DebugVerbose(bsm.logger, "[BlockShapeManager.blockInfoFromStateID] stateProps is nil, returning empty props", "stateID", blockStateID, "name", block.Name)
				return block.Name, map[string]string{}
			}
			props := bsm.stateProps.GetProperties(uint32(blockStateID))
			utils.DebugVerbose(bsm.logger, "[BlockShapeManager.blockInfoFromStateID] resolved", "stateID", blockStateID, "name", block.Name, "props", props)
			return block.Name, props
		}
	}

	utils.DebugVerbose(bsm.logger, "[BlockShapeManager.blockInfoFromStateID] failed to find block", "stateID", blockStateID)
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

// GetBlockProperties returns the named block state properties for a given state ID.
func (bsm *blockShapeManager) GetBlockProperties(blockStateID uint32) map[string]string {
	_, props := bsm.blockInfoFromStateID(blockStateID)
	return props
}

// GetMiningInfo returns the mining-relevant properties for a block state.
func (bsm *blockShapeManager) GetMiningInfo(blockStateID uint32) (hardness float64, material []string, diggable bool) {
	info := bsm.getInfoFromStateID(blockStateID)
	return info.Hardness, info.Material, info.Diggable
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
		utils.DebugVerbose(bsm.logger, "[BlockShapeManager.GetStandingSurfaceHeight]",
			"stateID", blockStateID, "name", blockName, "props", props, "height", height)
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

// IsMagma reports whether the block is a magma block specifically.
func (bsm *blockShapeManager) IsMagma(blockStateID uint32) bool {
	return bsm.BlockName(blockStateID) == "minecraft:magma_block"
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

// IsCobweb checks if block is a cobweb.
func (bsm *blockShapeManager) IsCobweb(blockStateID uint32) bool {
	return bsm.BlockName(blockStateID) == "minecraft:cobweb"
}

// IsScaffolding checks if block is scaffolding. Scaffolding is in
// BlockTags.CLIMBABLE like ladders and vines (so IsClimbable already
// returns true for it — see mc-data-gen's "climbable" field, sourced
// directly from that tag), but LivingEntity.applyClimbingSpeed explicitly
// excludes it from the sneak-freezes-descent behavior ladders get
// (`!getBlockStateAtPos().isOf(Blocks.SCAFFOLDING)`), so callers need to
// distinguish it from other climbable blocks for that one behavior.
func (bsm *blockShapeManager) IsScaffolding(blockStateID uint32) bool {
	return bsm.BlockName(blockStateID) == "minecraft:scaffolding"
}

// IsIce checks if block is ice, packed ice, or frosted ice (all share
// vanilla's 0.98F slipperiness — see IsBlueIce for the separate 0.989F case).
func (bsm *blockShapeManager) IsIce(blockStateID uint32) bool {
	switch bsm.BlockName(blockStateID) {
	case "minecraft:ice", "minecraft:packed_ice", "minecraft:frosted_ice":
		return true
	default:
		return false
	}
}

// IsBlueIce checks if block is blue ice (0.989F slipperiness, slipperier
// than ordinary ice's 0.98F).
func (bsm *blockShapeManager) IsBlueIce(blockStateID uint32) bool {
	return bsm.BlockName(blockStateID) == "minecraft:blue_ice"
}

// GetWaterFlowSpeed returns the flow speed multiplier for water based on the block's age property.
// Age 0 (source): returns 0.0 (no flow)
// Age 1-7 (flowing): returns proportional speed where age 7 = 1.0
func (bsm *blockShapeManager) GetWaterFlowSpeed(blockStateID uint32) float64 {
	if !bsm.IsWater(blockStateID) {
		return 0.0 // Not water
	}

	// Get water age/level property (0-7)
	_, props := bsm.blockInfoFromStateID(blockStateID)
	if len(props) == 0 {
		// No properties means water source (age 0)
		return 0.0
	}

	// Check for 'level' or 'age' property (Minecraft uses 'level' for water)
	levelStr, hasLevel := props["level"]
	if !hasLevel {
		levelStr, hasLevel = props["age"]
	}
	if !hasLevel {
		return 0.0 // Source block, no flow
	}

	// Parse level (0-7)
	var level int
	fmt.Sscanf(levelStr, "%d", &level)
	if level < 0 || level > 7 {
		level = 0
	}

	if level == 0 {
		return 0.0 // Source, no flow
	}

	// Flowing water: age 1 = 1/7 speed, age 7 = 7/7 speed
	// Normalized to 0.0-1.0
	return float64(level) / 7.0
}

// GetWaterFlowDirection determines the direction water flows at a given position.
// Returns normalized V3 vector (0,0,0) if not flowing water.
// For flowing water, calculates direction toward neighboring blocks with lower age.
func (bsm *blockShapeManager) GetWaterFlowDirection(x, y, z int, world models.World) models.V3 {
	if world == nil {
		return models.V3{} // No direction
	}

	blockState, loaded := world.GetBlockStatus(x, y, z)
	if !loaded || !bsm.IsWater(blockState) {
		return models.V3{} // Not water
	}

	// Get current water level
	_, props := bsm.blockInfoFromStateID(blockState)
	levelStr, hasLevel := props["level"]
	if !hasLevel {
		levelStr, hasLevel = props["age"]
	}
	if !hasLevel {
		return models.V3{} // Source block, no flow direction
	}

	var currentLevel int
	fmt.Sscanf(levelStr, "%d", &currentLevel)
	// Level 0 = source block. It should still flow toward adjacent water.
	// Levels 1-7 and 8 are valid flowing water.
	if currentLevel < 0 || currentLevel > 8 {
		return models.V3{} // Invalid level
	}

	// Find neighboring water blocks with lower level (flow direction)
	var flowDir models.V3
	neighbors := [][3]int{
		{x + 1, y, z}, // East
		{x - 1, y, z}, // West
		{x, y, z + 1}, // South
		{x, y, z - 1}, // North
		{x, y - 1, z}, // Down
	}

	for i, neighbor := range neighbors {
		nState, nLoaded := world.GetBlockStatus(neighbor[0], neighbor[1], neighbor[2])
		if !nLoaded || !bsm.IsWater(nState) {
			continue
		}

		// Get neighbor water level
		_, nProps := bsm.blockInfoFromStateID(nState)
		nLevelStr, nHasLevel := nProps["level"]
		if !nHasLevel {
			nLevelStr, nHasLevel = nProps["age"]
		}

		var neighborLevel int
		if nHasLevel {
			fmt.Sscanf(nLevelStr, "%d", &neighborLevel)
			// Clamp to valid range 0-8 (8 is valid downflow)
			if neighborLevel < 0 {
				neighborLevel = 0
			} else if neighborLevel > 8 {
				neighborLevel = 8
			}
		} else {
			neighborLevel = 0 // Source block (level 0)
		}

		if utils.DebugVerboseEnabled(bsm.logger) {
			neighborName := "?"
			switch i {
			case 0:
				neighborName = "East"
			case 1:
				neighborName = "West"
			case 2:
				neighborName = "South"
			case 3:
				neighborName = "North"
			case 4:
				neighborName = "Down"
			}
			utils.DebugVerbose(bsm.logger, "[WaterFlow] flowDirection",
				"neighbor", neighborName, "x", neighbor[0], "y", neighbor[1], "z", neighbor[2],
				"level", neighborLevel, "cur", currentLevel, "diff", neighborLevel-currentLevel)
		}

		// Flow toward higher level numbers (lower surface height)
		// Flow away from lower level numbers (higher surface height, sources)
		// Special case: Level 8 is downflow - never flow horizontally toward it
		// Only consider if neighbor level is different from current
		if neighborLevel != currentLevel {
			switch i {
			case 0: // East (+X)
				if neighborLevel == 8 {
					// Never flow horizontally toward downflow
					flowDir.X -= 1.0 // Flow away from level 8
				} else if neighborLevel > currentLevel {
					flowDir.X += 1.0 // Flow toward lower surface (higher level number)
				} else {
					flowDir.X -= 1.0 // Flow away from higher surface (lower level number)
				}
			case 1: // West (-X)
				if neighborLevel == 8 {
					flowDir.X += 1.0 // Flow away from level 8
				} else if neighborLevel > currentLevel {
					flowDir.X -= 1.0
				} else {
					flowDir.X += 1.0
				}
			case 2: // South (+Z)
				if neighborLevel == 8 {
					flowDir.Z -= 1.0 // Flow away from level 8
				} else if neighborLevel > currentLevel {
					flowDir.Z += 1.0
				} else {
					flowDir.Z -= 1.0
				}
			case 3: // North (-Z)
				if neighborLevel == 8 {
					flowDir.Z += 1.0 // Flow away from level 8
				} else if neighborLevel > currentLevel {
					flowDir.Z -= 1.0
				} else {
					flowDir.Z += 1.0
				}
			case 4: // Down (-Y)
				if neighborLevel > currentLevel {
					flowDir.Y -= 1.0 // Flow down toward lower surface
				} else {
					flowDir.Y += 1.0 // Flow away from higher surface
				}
			}
		}
	}

	// Normalize direction
	magnitude := flowDir.DistanceTo(models.V3{})
	if magnitude < 0.001 {
		// Special case: level 8 water is downflow. If no gradient found, flow downward.
		if currentLevel == 8 {
			utils.DebugVerbose(bsm.logger, "[WaterFlow] level 8, no gradient, defaulting to downflow", "x", x, "y", y, "z", z)
			return models.V3{X: 0, Y: -1, Z: 0}
		}
		utils.DebugVerbose(bsm.logger, "[WaterFlow] no flow direction, no gradient", "x", x, "y", y, "z", z, "level", currentLevel)
		return models.V3{} // No flow direction
	} else {
		utils.DebugVerbose(bsm.logger, "[WaterFlow] magnitude", "magnitude", magnitude, "x", x, "y", y, "z", z, "level", currentLevel)
	}

	result := models.V3{
		X: flowDir.X / magnitude,
		Y: flowDir.Y / magnitude,
		Z: flowDir.Z / magnitude,
	}

	utils.DebugVerbose(bsm.logger, "[WaterFlow] resolved",
		"x", x, "y", y, "z", z, "level", currentLevel, "flowX", result.X, "flowY", result.Y, "flowZ", result.Z, "magnitude", magnitude)

	return result
}
