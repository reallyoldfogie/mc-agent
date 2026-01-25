package physics

import "math"

// Fall damage constants
const (
	// Safe fall distance - no damage below this
	SafeFallDistance = 3.0 // blocks

	// Damage per block fallen above safe distance
	DamagePerBlock = 1.0 // hearts (0.5 health points per heart)
)

// Block damage reduction multipliers (percentage)
const (
	HayBaleReduction    = 0.80 // 80% reduction
	BedReduction        = 0.50 // 50% reduction
	HoneyBlockReduction = 0.80 // 80% reduction
	SlimeBlockDamage    = 0.0  // No damage, bounces
	PowderSnowDamage    = 0.0  // No damage
	WaterDamage         = 0.0  // No damage (any depth)
)

// CalculateFallDamage computes fall damage in hearts based on fall distance and landing block.
// Returns damage in hearts (2 hearts = 1 HP in Minecraft).
// Water landing returns 0 damage regardless of height.
func CalculateFallDamage(fallDistance float64, landingBlockID uint32, shapeProvider BlockShapeProvider) float64 {
	// Water negates ALL fall damage (critical mechanic!)
	if IsWaterBlock(landingBlockID, shapeProvider) {
		return 0.0
	}

	// Check for special damage-reducing blocks
	reduction := GetDamageReduction(landingBlockID, shapeProvider)

	// Calculate base damage (max with 0 to prevent negative)
	baseDamage := math.Max(0, fallDistance-SafeFallDistance) * DamagePerBlock

	// Apply reduction
	finalDamage := baseDamage * (1.0 - reduction)

	return finalDamage
}

// GetDamageReduction returns the damage reduction multiplier for a block type.
// Returns 0.0 (no reduction) for most blocks, higher values for special blocks.
// Slime blocks and powder snow return 0.0 (no damage at all, not reduction).
//
// Note: Block type detection requires mc-data-gen integration via BlockShapeProvider.
// The real implementation will return false for all special blocks until mc-data-gen
// provides block name/type information. Tests can use mocks to verify the logic.
func GetDamageReduction(blockID uint32, shapeProvider BlockShapeProvider) float64 {
	// Check special blocks in order of most common to least common
	if shapeProvider.IsHayBale(blockID) {
		return HayBaleReduction // 80% reduction
	}
	if shapeProvider.IsHoneyBlock(blockID) {
		return HoneyBlockReduction // 80% reduction
	}
	if shapeProvider.IsBed(blockID) {
		return BedReduction // 50% reduction
	}
	// Slime blocks and powder snow provide 0 damage (100% reduction)
	// but are handled separately (bouncing, slowness effects)
	if shapeProvider.IsSlimeBlock(blockID) {
		return 1.0 // 100% reduction (no damage)
	}
	if shapeProvider.IsPowderSnow(blockID) {
		return 1.0 // 100% reduction (no damage)
	}

	// No special damage reduction
	return 0.0
}

// IsWaterBlock checks if a block is water or flowing water.
// Water completely negates fall damage regardless of depth.
func IsWaterBlock(blockID uint32, shapeProvider BlockShapeProvider) bool {
	return shapeProvider.IsWater(blockID)
}

// IsSafeLanding checks if landing at a position would be safe (no or minimal damage).
// Considers fall distance and landing block type.
func IsSafeLanding(fallDistance float64, landingBlockID uint32, shapeProvider BlockShapeProvider) bool {
	damage := CalculateFallDamage(fallDistance, landingBlockID, shapeProvider)
	// Consider safe if damage is less than 2 hearts (4 half-hearts)
	return damage < 2.0
}

// PredictFallDistance calculates how far a player will fall from a position.
// Returns the fall distance in blocks and the landing position.
func PredictFallDistance(startPos V3, w World, shapeProvider BlockShapeProvider) (fallDistance float64, landingY float64) {
	currentY := startPos.Y

	// Scan downward to find ground
	for y := int(math.Floor(startPos.Y)); y >= int(startPos.Y)-256; y-- {
		blockID, _ := w.GetBlockStatus(
			int(math.Floor(startPos.X)),
			y,
			int(math.Floor(startPos.Z)),
		)

		// Check if block is solid (not passable)
		if !shapeProvider.IsPassable(blockID) {
			landingY = float64(y + 1) // Land on top of block
			fallDistance = currentY - landingY
			return fallDistance, landingY
		}
	}

	// No ground found (void?)
	return 256, -256
}

// GetLandingBlock returns the block ID that the player would land on/in from a position.
// This checks for special blocks like water at the landing position first, then scans
// downward for solid blocks.
func GetLandingBlock(pos V3, w World, shapeProvider BlockShapeProvider) uint32 {
	// Check for water or other special blocks at the landing position first
	// When falling into water, you land IN the water, not on the solid block below
	for y := int(math.Floor(pos.Y)) - 1; y >= int(pos.Y)-256; y-- {
		blockID, _ := w.GetBlockStatus(
			int(math.Floor(pos.X)),
			y,
			int(math.Floor(pos.Z)),
		)

		// If it's water or another special fluid/block, return it immediately
		// Water negates all fall damage regardless of what's below it
		if shapeProvider.IsWater(blockID) {
			return blockID
		}

		// Otherwise, if it's a solid block, return it
		if !shapeProvider.IsPassable(blockID) {
			return blockID
		}
	}

	// No block found
	return 0 // Air
}

// CalculateFallDamageForDrop calculates total damage for a drop from one position to another.
// This is useful for pathfinding cost calculations.
func CalculateFallDamageForDrop(from, to V3, w World, shapeProvider BlockShapeProvider) float64 {
	fallDistance := from.Y - to.Y

	// If not falling, no damage
	if fallDistance <= 0 {
		return 0.0
	}

	// Get landing block
	landingBlock := GetLandingBlock(to, w, shapeProvider)

	return CalculateFallDamage(fallDistance, landingBlock, shapeProvider)
}

// IsWaterDrop checks if a drop would land in water.
// Water drops are safe from any height.
func IsWaterDrop(from, to V3, w World, shapeProvider BlockShapeProvider) bool {
	landingBlock := GetLandingBlock(to, w, shapeProvider)
	return IsWaterBlock(landingBlock, shapeProvider)
}

// GetDropSafety categorizes how safe a drop is.
type DropSafety int

const (
	DropSafeNoFall    DropSafety = iota // No fall or very short
	DropSafeWater                       // Landing in water (safe from any height)
	DropSafeShortFall                   // Short fall, minimal damage
	DropDangerous                       // Significant damage expected
	DropLethal                          // Likely to kill player
)

// GetDropSafety evaluates how dangerous a drop is.
// Useful for pathfinding to prefer safer routes.
func GetDropSafety(from, to V3, currentHealth float64, w World, shapeProvider BlockShapeProvider) DropSafety {
	fallDistance := from.Y - to.Y

	// No fall
	if fallDistance <= 0 {
		return DropSafeNoFall
	}

	// Check if water landing
	if IsWaterDrop(from, to, w, shapeProvider) {
		return DropSafeWater
	}

	// Calculate damage
	damage := CalculateFallDamageForDrop(from, to, w, shapeProvider)

	// Categorize based on damage and current health
	if damage == 0 {
		return DropSafeNoFall
	} else if damage < 2.0 {
		return DropSafeShortFall
	} else if damage < currentHealth*0.5 {
		return DropDangerous
	} else {
		return DropLethal
	}
}

// PathfindingDropCost calculates the cost penalty for a drop in pathfinding.
// Water drops have low cost, dangerous drops have high cost.
func PathfindingDropCost(from, to V3, baseMoveCost float64, w World, shapeProvider BlockShapeProvider) float64 {
	fallDistance := from.Y - to.Y

	// No fall, just base cost
	if fallDistance <= 0 {
		return baseMoveCost
	}

	// Water drop - slightly higher than base but much lower than land
	if IsWaterDrop(from, to, w, shapeProvider) {
		return baseMoveCost * 1.2 // 20% penalty for water entry
	}

	// Land drop - add cost based on damage
	damage := CalculateFallDamageForDrop(from, to, w, shapeProvider)

	// Cost formula: base + (damage * damage_cost_factor)
	// Heavily penalize damaging falls
	damageCostFactor := 10.0
	return baseMoveCost + (damage * damageCostFactor)
}
