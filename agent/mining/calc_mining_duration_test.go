package mining

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Block fixtures using real mc-data-gen values ---

var (
	stoneBlock = BlockMiningInfo{
		Hardness: 1.5,
		Material: []string{"mineable/pickaxe"},
		Diggable: true,
	}
	dirtBlock = BlockMiningInfo{
		Hardness: 0.5,
		Material: []string{"mineable/shovel"},
		Diggable: true,
	}
	obsidianBlock = BlockMiningInfo{
		Hardness: 50.0,
		Material: []string{"mineable/pickaxe"},
		Diggable: true,
	}
	bedrockBlock = BlockMiningInfo{
		Hardness: -1.0,
		Material: nil,
		Diggable: false,
	}
	oakLogBlock = BlockMiningInfo{
		Hardness: 2.0,
		Material: []string{"mineable/axe"},
		Diggable: true,
	}
	grassBlock = BlockMiningInfo{
		Hardness: 0.0,
		Material: nil,
		Diggable: true,
	}
)

// --- Tool fixtures ---

func ironPickaxe() ToolInfo {
	return ToolInfo{
		Category: ToolCategoryPickaxe,
		Tier:     ToolTierIron,
	}
}

func diamondPickaxe() ToolInfo {
	return ToolInfo{
		Category: ToolCategoryPickaxe,
		Tier:     ToolTierDiamond,
	}
}

func ironShovel() ToolInfo {
	return ToolInfo{
		Category: ToolCategoryShovel,
		Tier:     ToolTierIron,
	}
}

func woodenAxe() ToolInfo {
	return ToolInfo{
		Category: ToolCategoryAxe,
		Tier:     ToolTierWood,
	}
}

// --- CalcBreakTime tests ---

func TestStoneWithIronPickaxe(t *testing.T) {
	// Stone (hardness 1.5) with iron pickaxe (speed 6.0)
	// damage = (6.0 / 1.5) / 30.0 = 4.0 / 30.0 = 0.1333...
	// ticks = ceil(1 / 0.1333) = ceil(7.5) = 8
	// seconds = 8 / 20 = 0.40
	result := CalcBreakTime(stoneBlock, ironPickaxe(), 0, 0, false, true)
	assert.InDelta(t, 0.40, result, 0.001)
}

func TestDirtWithIronShovel(t *testing.T) {
	// Dirt (hardness 0.5) with iron shovel (speed 6.0)
	// damage = (6.0 / 0.5) / 30.0 = 12.0 / 30.0 = 0.4
	// ticks = ceil(1 / 0.4) = ceil(2.5) = 3
	// seconds = 3 / 20 = 0.15
	result := CalcBreakTime(dirtBlock, ironShovel(), 0, 0, false, true)
	assert.InDelta(t, 0.15, result, 0.001)
}

func TestStoneWithHand(t *testing.T) {
	// Stone (hardness 1.5) with hand (speed 1.0, wrong tool)
	// damage = (1.0 / 1.5) / 100.0 = 0.006666...
	// ticks = ceil(1 / 0.006666) = ceil(150.0) = 150
	// seconds = 150 / 20 = 7.5
	result := CalcBreakTime(stoneBlock, HandTool(), 0, 0, false, true)
	assert.InDelta(t, 7.5, result, 0.001)
}

func TestObsidianWithDiamondPickaxe(t *testing.T) {
	// Obsidian (hardness 50.0) with diamond pickaxe (speed 8.0)
	// damage = (8.0 / 50.0) / 30.0 = 0.16 / 30.0 = 0.005333...
	// ticks = ceil(1 / 0.005333) = ceil(187.5) = 188
	// seconds = 188 / 20 = 9.4
	result := CalcBreakTime(obsidianBlock, diamondPickaxe(), 0, 0, false, true)
	assert.InDelta(t, 9.4, result, 0.001)
}

func TestBedrockIsUnbreakable(t *testing.T) {
	result := CalcBreakTime(bedrockBlock, diamondPickaxe(), 0, 0, false, true)
	require.True(t, math.IsInf(result, 1), "bedrock should be unbreakable")
}

func TestZeroHardnessInstantBreak(t *testing.T) {
	// Blocks with hardness 0 (e.g. tall grass) break in 1 tick
	result := CalcBreakTime(grassBlock, HandTool(), 0, 0, false, true)
	assert.InDelta(t, 0.05, result, 0.001)
}

func TestHasteEffect(t *testing.T) {
	// Stone with iron pickaxe and Haste II
	// speed = 6.0 * (1 + 0.2*2) = 6.0 * 1.4 = 8.4
	// damage = (8.4 / 1.5) / 30.0 = 5.6 / 30.0 = 0.18666...
	// ticks = ceil(1 / 0.18666) = ceil(5.357) = 6
	// seconds = 6 / 20 = 0.30
	result := CalcBreakTime(stoneBlock, ironPickaxe(), 2, 0, false, true)
	assert.InDelta(t, 0.30, result, 0.001)
}

func TestMiningFatigueLevel1(t *testing.T) {
	// Stone with iron pickaxe and Mining Fatigue I
	// speed = 6.0 * 0.3 = 1.8
	// damage = (1.8 / 1.5) / 30.0 = 1.2 / 30.0 = 0.04
	// ticks = ceil(1 / 0.04) = ceil(25.0) = 25
	// seconds = 25 / 20 = 1.25
	result := CalcBreakTime(stoneBlock, ironPickaxe(), 0, 1, false, true)
	assert.InDelta(t, 1.25, result, 0.001)
}

func TestMiningFatigueLevel4(t *testing.T) {
	// Mining Fatigue IV+ uses 0.00081 multiplier
	result := CalcBreakTime(stoneBlock, ironPickaxe(), 0, 4, false, true)
	require.Greater(t, result, 10.0, "Mining Fatigue IV should be very slow")
}

func TestUnderwaterPenalty(t *testing.T) {
	// Stone with iron pickaxe, underwater, on ground
	// speed = 6.0 / 5.0 = 1.2
	// damage = (1.2 / 1.5) / 30.0 = 0.8 / 30.0 = 0.02666...
	// ticks = ceil(1 / 0.02666) = ceil(37.5) = 38
	// seconds = 38 / 20 = 1.90
	result := CalcBreakTime(stoneBlock, ironPickaxe(), 0, 0, true, true)
	assert.InDelta(t, 1.90, result, 0.001)
}

func TestAirbornePenalty(t *testing.T) {
	// Stone with iron pickaxe, not underwater, airborne
	// speed = 6.0 / 5.0 = 1.2 (same as underwater alone)
	// Same result as underwater-only since both divide by 5
	result := CalcBreakTime(stoneBlock, ironPickaxe(), 0, 0, false, false)
	assert.InDelta(t, 1.90, result, 0.001)
}

func TestUnderwaterAndAirborneStack(t *testing.T) {
	// Both penalties should stack multiplicatively: speed / 25
	// speed = 6.0 / 5.0 / 5.0 = 0.24
	// damage = (0.24 / 1.5) / 30.0 = 0.16 / 30.0 = 0.005333...
	// ticks = ceil(1 / 0.005333) = ceil(187.5) = 188
	// seconds = 188 / 20 = 9.4
	result := CalcBreakTime(stoneBlock, ironPickaxe(), 0, 0, true, false)
	assert.InDelta(t, 9.4, result, 0.001)
}

func TestWrongToolOnPickaxeBlock(t *testing.T) {
	// Using a shovel on stone (pickaxe block) = wrong tool
	result := CalcBreakTime(stoneBlock, ironShovel(), 0, 0, false, true)
	// Should be the same as hand since wrong tool category
	handResult := CalcBreakTime(stoneBlock, HandTool(), 0, 0, false, true)
	assert.InDelta(t, handResult, result, 0.001)
}

func TestEfficiencyEnchantment(t *testing.T) {
	// Stone with iron pickaxe + Efficiency V
	// speed = 6.0 + (5^2 + 1) = 6.0 + 26 = 32.0
	// damage = (32.0 / 1.5) / 30.0 = 21.333 / 30.0 = 0.7111...
	// ticks = ceil(1 / 0.7111) = ceil(1.406) = 2
	// seconds = 2 / 20 = 0.10
	tool := ToolInfo{
		Category:        ToolCategoryPickaxe,
		Tier:            ToolTierIron,
		EfficiencyLevel: 5,
	}
	result := CalcBreakTime(stoneBlock, tool, 0, 0, false, true)
	assert.InDelta(t, 0.10, result, 0.001)
}

func TestOakLogWithWoodenAxe(t *testing.T) {
	// Oak log (hardness 2.0) with wooden axe (speed 2.0)
	// damage = (2.0 / 2.0) / 30.0 = 1.0 / 30.0 = 0.0333...
	// ticks = ceil(1 / 0.0333) = ceil(30.0) = 30
	// seconds = 30 / 20 = 1.5
	result := CalcBreakTime(oakLogBlock, woodenAxe(), 0, 0, false, true)
	assert.InDelta(t, 1.5, result, 0.001)
}

// --- NewToolInfoFromItem tests ---

func TestNewToolInfoFromItem_IronPickaxe(t *testing.T) {
	tool := NewToolInfoFromItem("minecraft:iron_pickaxe", []string{
		"minecraft:pickaxes",
		"minecraft:enchantable/mining",
	})
	assert.Equal(t, ToolCategoryPickaxe, tool.Category)
	assert.Equal(t, ToolTierIron, tool.Tier)
	assert.Equal(t, 0, tool.EfficiencyLevel)
}

func TestNewToolInfoFromItem_DiamondAxe(t *testing.T) {
	tool := NewToolInfoFromItem("minecraft:diamond_axe", []string{"minecraft:axes"})
	assert.Equal(t, ToolCategoryAxe, tool.Category)
	assert.Equal(t, ToolTierDiamond, tool.Tier)
}

func TestNewToolInfoFromItem_NetheriteShovel(t *testing.T) {
	tool := NewToolInfoFromItem("minecraft:netherite_shovel", []string{"minecraft:shovels"})
	assert.Equal(t, ToolCategoryShovel, tool.Category)
	assert.Equal(t, ToolTierNetherite, tool.Tier)
}

func TestNewToolInfoFromItem_GoldenHoe(t *testing.T) {
	tool := NewToolInfoFromItem("minecraft:golden_hoe", []string{"minecraft:hoes"})
	assert.Equal(t, ToolCategoryHoe, tool.Category)
	assert.Equal(t, ToolTierGold, tool.Tier)
}

func TestNewToolInfoFromItem_WoodenSword(t *testing.T) {
	tool := NewToolInfoFromItem("minecraft:wooden_sword", []string{"minecraft:swords"})
	assert.Equal(t, ToolCategorySword, tool.Category)
	assert.Equal(t, ToolTierWood, tool.Tier)
}

func TestNewToolInfoFromItem_StoneShovel(t *testing.T) {
	tool := NewToolInfoFromItem("minecraft:stone_shovel", []string{"minecraft:shovels"})
	assert.Equal(t, ToolCategoryShovel, tool.Category)
	assert.Equal(t, ToolTierStone, tool.Tier)
}

func TestNewToolInfoFromItem_Shears(t *testing.T) {
	tool := NewToolInfoFromItem("minecraft:shears", nil)
	assert.Equal(t, ToolCategoryShears, tool.Category)
	assert.Equal(t, ToolTierNone, tool.Tier)
}

func TestNewToolInfoFromItem_UnknownItem(t *testing.T) {
	// Non-tool item should default to hand
	tool := NewToolInfoFromItem("minecraft:stick", []string{})
	assert.Equal(t, ToolCategoryHand, tool.Category)
	assert.Equal(t, ToolTierNone, tool.Tier)
}

// --- Helper function tests ---

func TestHandTool(t *testing.T) {
	tool := HandTool()
	assert.Equal(t, ToolCategoryHand, tool.Category)
	assert.Equal(t, ToolTierNone, tool.Tier)
	assert.Equal(t, 0, tool.EfficiencyLevel)
}

func TestNewBlockMiningInfo(t *testing.T) {
	info := NewBlockMiningInfo(1.5, []string{"mineable/pickaxe"}, true)
	assert.Equal(t, 1.5, info.Hardness)
	assert.Equal(t, []string{"mineable/pickaxe"}, info.Material)
	assert.True(t, info.Diggable)
}

// --- NewToolInfoFromItemName tests (name-only, no tags) ---

func TestNewToolInfoFromItemName_IronPickaxe(t *testing.T) {
	tool := NewToolInfoFromItemName("minecraft:iron_pickaxe")
	assert.Equal(t, ToolCategoryPickaxe, tool.Category)
	assert.Equal(t, ToolTierIron, tool.Tier)
}

func TestNewToolInfoFromItemName_DiamondAxe(t *testing.T) {
	tool := NewToolInfoFromItemName("minecraft:diamond_axe")
	assert.Equal(t, ToolCategoryAxe, tool.Category)
	assert.Equal(t, ToolTierDiamond, tool.Tier)
}

func TestNewToolInfoFromItemName_GoldenShovel(t *testing.T) {
	tool := NewToolInfoFromItemName("minecraft:golden_shovel")
	assert.Equal(t, ToolCategoryShovel, tool.Category)
	assert.Equal(t, ToolTierGold, tool.Tier)
}

func TestNewToolInfoFromItemName_NetheriteHoe(t *testing.T) {
	tool := NewToolInfoFromItemName("minecraft:netherite_hoe")
	assert.Equal(t, ToolCategoryHoe, tool.Category)
	assert.Equal(t, ToolTierNetherite, tool.Tier)
}

func TestNewToolInfoFromItemName_WoodenSword(t *testing.T) {
	tool := NewToolInfoFromItemName("minecraft:wooden_sword")
	assert.Equal(t, ToolCategorySword, tool.Category)
	assert.Equal(t, ToolTierWood, tool.Tier)
}

func TestNewToolInfoFromItemName_Shears(t *testing.T) {
	tool := NewToolInfoFromItemName("minecraft:shears")
	assert.Equal(t, ToolCategoryShears, tool.Category)
	assert.Equal(t, ToolTierNone, tool.Tier)
}

func TestNewToolInfoFromItemName_NonTool(t *testing.T) {
	tool := NewToolInfoFromItemName("minecraft:cobblestone")
	assert.Equal(t, ToolCategoryHand, tool.Category)
}

func TestNewToolInfoFromItemName_Empty(t *testing.T) {
	tool := NewToolInfoFromItemName("")
	assert.Equal(t, ToolCategoryHand, tool.Category)
}

// --- MatchesMaterial tests ---

func TestMatchesMaterial_PickaxeOnPickaxeBlock(t *testing.T) {
	tool := ToolInfo{Category: ToolCategoryPickaxe, Tier: ToolTierIron}
	assert.True(t, MatchesMaterial(tool, []string{"mineable/pickaxe"}))
}

func TestMatchesMaterial_ShovelOnPickaxeBlock(t *testing.T) {
	tool := ToolInfo{Category: ToolCategoryShovel, Tier: ToolTierIron}
	assert.False(t, MatchesMaterial(tool, []string{"mineable/pickaxe"}))
}

func TestMatchesMaterial_HandOnAnyBlock(t *testing.T) {
	assert.False(t, MatchesMaterial(HandTool(), []string{"mineable/pickaxe"}))
}

func TestMatchesMaterial_EmptyMaterial(t *testing.T) {
	tool := ToolInfo{Category: ToolCategoryPickaxe, Tier: ToolTierIron}
	assert.False(t, MatchesMaterial(tool, nil))
}

// --- ToolSpeed tests ---

func TestToolSpeed_Tiers(t *testing.T) {
	assert.Equal(t, 12.0, ToolSpeed(ToolTierGold))
	assert.Equal(t, 9.0, ToolSpeed(ToolTierNetherite))
	assert.Equal(t, 8.0, ToolSpeed(ToolTierDiamond))
	assert.Equal(t, 6.0, ToolSpeed(ToolTierIron))
	assert.Equal(t, 4.0, ToolSpeed(ToolTierStone))
	assert.Equal(t, 2.0, ToolSpeed(ToolTierWood))
	assert.Equal(t, 1.0, ToolSpeed(ToolTierNone))
}
