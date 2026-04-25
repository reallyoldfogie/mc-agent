package mining

import (
	"math"
	"strings"
)

// ToolCategory represents the type of tool being used.
type ToolCategory string

const (
	ToolCategoryHand    ToolCategory = "hand"
	ToolCategoryPickaxe ToolCategory = "pickaxe"
	ToolCategoryAxe     ToolCategory = "axe"
	ToolCategoryShovel  ToolCategory = "shovel"
	ToolCategoryHoe     ToolCategory = "hoe"
	ToolCategoryShears  ToolCategory = "shears"
	ToolCategorySword   ToolCategory = "sword"
)

// ToolTier represents the material tier of a tool, which determines base mining speed.
type ToolTier string

const (
	ToolTierNone      ToolTier = ""
	ToolTierWood      ToolTier = "wood"
	ToolTierStone     ToolTier = "stone"
	ToolTierIron      ToolTier = "iron"
	ToolTierDiamond   ToolTier = "diamond"
	ToolTierNetherite ToolTier = "netherite"
	ToolTierGold      ToolTier = "gold"
)

// tierBaseSpeed maps tool tiers to their vanilla base mining speed multiplier.
// These are hardcoded constants from vanilla Minecraft that are not exposed
// by any data generator.
var tierBaseSpeed = map[ToolTier]float64{
	ToolTierWood:      2.0,
	ToolTierStone:     4.0,
	ToolTierIron:      6.0,
	ToolTierDiamond:   8.0,
	ToolTierNetherite: 9.0,
	ToolTierGold:      12.0,
}

// categoryToMineableTag maps a ToolCategory to the corresponding block material
// tag used in mc-data-gen's Material field.
var categoryToMineableTag = map[ToolCategory]string{
	ToolCategoryPickaxe: "mineable/pickaxe",
	ToolCategoryAxe:     "mineable/axe",
	ToolCategoryShovel:  "mineable/shovel",
	ToolCategoryHoe:     "mineable/hoe",
}

// ToolInfo describes the tool the agent is holding for mining calculations.
type ToolInfo struct {
	Category        ToolCategory
	Tier            ToolTier
	EfficiencyLevel int // Efficiency enchantment level (0 = none)
}

// BlockMiningInfo holds the block properties relevant to mining speed calculation.
// These values come from mc-data-gen's ShapeInfo (hardness, material, diggable).
type BlockMiningInfo struct {
	Hardness float64  // Block hardness; negative means unbreakable
	Material []string // e.g. ["mineable/pickaxe"]
	Diggable bool     // Whether the block can be broken at all
}

// HandTool returns a ToolInfo representing bare-hand mining.
func HandTool() ToolInfo {
	return ToolInfo{Category: ToolCategoryHand}
}

// NewBlockMiningInfo constructs a BlockMiningInfo from individual fields,
// typically extracted from a ShapeInfo.
func NewBlockMiningInfo(hardness float64, material []string, diggable bool) BlockMiningInfo {
	return BlockMiningInfo{
		Hardness: hardness,
		Material: material,
		Diggable: diggable,
	}
}

// tagToCategory maps item tags (from mc-data-gen ItemInfo.Tags) to ToolCategory.
var tagToCategory = map[string]ToolCategory{
	"minecraft:pickaxes": ToolCategoryPickaxe,
	"minecraft:axes":     ToolCategoryAxe,
	"minecraft:shovels":  ToolCategoryShovel,
	"minecraft:hoes":     ToolCategoryHoe,
	"minecraft:swords":   ToolCategorySword,
}

// tierPrefixes maps item ID tier prefixes to ToolTier.
var tierPrefixes = []struct {
	prefix string
	tier   ToolTier
}{
	{"wooden_", ToolTierWood},
	{"stone_", ToolTierStone},
	{"iron_", ToolTierIron},
	{"diamond_", ToolTierDiamond},
	{"netherite_", ToolTierNetherite},
	{"golden_", ToolTierGold},
}

// NewToolInfoFromItem derives a ToolInfo from an item ID and its tags.
// itemID should be the full namespaced ID (e.g. "minecraft:iron_pickaxe").
// tags should be the item's tag list (e.g. ["minecraft:pickaxes", "minecraft:enchantable/mining"]).
func NewToolInfoFromItem(itemID string, tags []string) ToolInfo {
	info := ToolInfo{Category: ToolCategoryHand}

	// Check for shears by item ID (no tag-based category for shears)
	if itemID == "minecraft:shears" {
		info.Category = ToolCategoryShears
		return info
	}

	// Determine category from tags
	for _, tag := range tags {
		if category, ok := tagToCategory[tag]; ok {
			info.Category = category
			break
		}
	}

	// Determine tier from item ID
	// Strip namespace prefix to get the bare name (e.g. "iron_pickaxe")
	bareName := itemID
	if idx := strings.LastIndex(itemID, ":"); idx >= 0 {
		bareName = itemID[idx+1:]
	}

	for _, entry := range tierPrefixes {
		if strings.HasPrefix(bareName, entry.prefix) {
			info.Tier = entry.tier
			break
		}
	}

	return info
}

// nameSuffixToCategory maps item name suffixes to ToolCategory.
// Used when tags are not available (e.g. deriving tool info from inventory item names).
var nameSuffixToCategory = map[string]ToolCategory{
	"_pickaxe": ToolCategoryPickaxe,
	"_axe":     ToolCategoryAxe,
	"_shovel":  ToolCategoryShovel,
	"_hoe":     ToolCategoryHoe,
	"_sword":   ToolCategorySword,
}

// NewToolInfoFromItemName derives a ToolInfo purely from an item name string
// (e.g. "minecraft:iron_pickaxe"). This is useful when only the item name is
// available (no tags), such as when reading from the inventory system.
func NewToolInfoFromItemName(itemName string) ToolInfo {
	info := ToolInfo{Category: ToolCategoryHand}

	if itemName == "" {
		return info
	}

	// Shears special case
	if itemName == "minecraft:shears" {
		info.Category = ToolCategoryShears
		return info
	}

	// Strip namespace
	bareName := itemName
	if idx := strings.LastIndex(itemName, ":"); idx >= 0 {
		bareName = itemName[idx+1:]
	}

	// Determine tier from prefix
	for _, entry := range tierPrefixes {
		if strings.HasPrefix(bareName, entry.prefix) {
			info.Tier = entry.tier
			break
		}
	}

	// Determine category from suffix
	for suffix, category := range nameSuffixToCategory {
		if strings.HasSuffix(bareName, suffix) {
			info.Category = category
			return info
		}
	}

	return info
}

// ToolSpeed returns the base mining speed for a tool's tier.
// Exported so callers can compare tools by speed.
func ToolSpeed(tier ToolTier) float64 {
	return toolSpeed(tier)
}

// MatchesMaterial returns true if the tool is the correct tool type for the
// given block material tags.
func MatchesMaterial(tool ToolInfo, material []string) bool {
	tag, ok := categoryToMineableTag[tool.Category]
	if !ok {
		return false
	}
	for _, blockTag := range material {
		if blockTag == tag {
			return true
		}
	}
	return false
}

// isCorrectTool checks whether the tool's category matches one of the
// block's material tags (e.g. tool is pickaxe, block has "mineable/pickaxe").
func isCorrectTool(tool ToolInfo, block BlockMiningInfo) bool {
	if tool.Category == ToolCategoryHand {
		return false
	}

	tag, ok := categoryToMineableTag[tool.Category]
	if !ok {
		// Swords and shears don't have a mineable/* tag; they use special-case
		// vanilla logic (e.g. swords are effective on cobwebs). For the generic
		// formula treat them as not matching the block's material.
		return false
	}

	for _, blockTag := range block.Material {
		if blockTag == tag {
			return true
		}
	}
	return false
}

// toolSpeed returns the base mining speed multiplier for the tool's tier.
func toolSpeed(tier ToolTier) float64 {
	if speed, ok := tierBaseSpeed[tier]; ok {
		return speed
	}
	return 1.0
}

// efficiencyBonus computes the speed bonus from the Efficiency enchantment.
// Vanilla formula: level^2 + 1 (for level > 0).
func efficiencyBonus(level int) float64 {
	if level <= 0 {
		return 0
	}
	return float64(level*level + 1)
}

// CalcBreakTime computes the time in seconds to break a block, following vanilla
// Minecraft mechanics (https://minecraft.wiki/w/Breaking#Speed).
//
// Parameters:
//   - block: mining-relevant properties from mc-data-gen ShapeInfo
//   - tool: the held tool (use HandTool() for bare hands)
//   - hasteLevel: Haste / Conduit Power effect level (0 = none)
//   - miningFatigueLevel: Mining Fatigue effect level (0 = none)
//   - underwater: true if the player's head is submerged (no Aqua Affinity)
//   - onGround: true if the player is standing on the ground
func CalcBreakTime(block BlockMiningInfo, tool ToolInfo, hasteLevel int, miningFatigueLevel int, underwater bool, onGround bool) float64 {
	// Unbreakable blocks (hardness < 0, e.g. bedrock)
	if block.Hardness < 0 {
		return math.Inf(1)
	}

	// Instant-break: hardness == 0 always breaks in one tick
	if block.Hardness == 0 {
		return 0.05 // 1 tick = 1/20 second
	}

	speed := 1.0
	correctTool := isCorrectTool(tool, block)

	if correctTool {
		speed = toolSpeed(tool.Tier)
		speed += efficiencyBonus(tool.EfficiencyLevel)
	}

	// Haste effect: multiply by (1 + 0.2 * level)
	if hasteLevel > 0 {
		speed *= (1.0 + 0.2*float64(hasteLevel))
	}

	// Mining Fatigue effect
	if miningFatigueLevel > 0 {
		switch miningFatigueLevel {
		case 1:
			speed *= 0.3
		case 2:
			speed *= 0.09
		case 3:
			speed *= 0.0027
		default:
			speed *= 0.00081
		}
	}

	// Underwater penalty (no Aqua Affinity)
	if underwater {
		speed /= 5.0
	}

	// Airborne penalty
	if !onGround {
		speed /= 5.0
	}

	damage := speed / block.Hardness

	if correctTool {
		damage /= 30.0
	} else {
		damage /= 100.0
	}

	if damage <= 0 {
		return math.Inf(1)
	}

	// Instant mine check: if damage >= 1 on first tick, it breaks instantly
	if damage >= 1.0 {
		return 0.05 // 1 tick
	}

	ticks := math.Ceil(1.0 / damage)
	return ticks / 20.0
}
