package models

// BlockFace represents the face of a block.
type BlockFace int

const (
	FaceDown  BlockFace = 0 // -Y
	FaceUp    BlockFace = 1 // +Y
	FaceNorth BlockFace = 2 // -Z
	FaceSouth BlockFace = 3 // +Z
	FaceWest  BlockFace = 4 // -X
	FaceEast  BlockFace = 5 // +X
)

// BlockShapeManager loads and provides access to block shape data for a specific Minecraft version.
type BlockShapeManager interface {
	GetCollisionBoxes(blockStateID uint32, x, y, z int) []AABB
	IsPassable(blockStateID uint32) bool
	IsSolid(blockStateID uint32) bool
	GetStandingSurfaceHeight(blockStateID uint32) float64
	IsClimbable(blockStateID uint32) bool
	IsFluid(blockStateID uint32) bool
	IsWater(blockStateID uint32) bool
	IsLava(blockStateID uint32) bool
	IsDangerous(blockStateID uint32) bool
	// IsMagma reports whether the block is a magma block specifically -
	// narrower than IsDangerous (which also covers lava, fire, cacti, etc.),
	// for callers that only care about magma's real-vehicle-specific hazard
	// (sinking a boat that touches it, per vanilla AbstractBoatEntity). See
	// docs/plans/WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 7.
	IsMagma(blockStateID uint32) bool
	IsDoorLike(blockStateID uint32) bool
	IsFenceLike(blockStateID uint32) bool
	IsSlab(blockStateID uint32) bool
	IsStair(blockStateID uint32) bool
	IsLogOrLeaf(blockStateID uint32) bool
	IsHayBale(blockStateID uint32) bool
	IsBed(blockStateID uint32) bool
	IsHoneyBlock(blockStateID uint32) bool
	IsSlimeBlock(blockStateID uint32) bool
	IsPowderSnow(blockStateID uint32) bool
	IsCobweb(blockStateID uint32) bool
	IsScaffolding(blockStateID uint32) bool
	// IsIce reports whether the block is ice, packed ice, or frosted ice —
	// the three blocks sharing vanilla's 0.98F slipperiness (Blocks.java).
	// Blue ice is a separate, slipperier value; see IsBlueIce.
	IsIce(blockStateID uint32) bool
	// IsBlueIce reports whether the block is blue ice specifically —
	// slipperiness 0.989F in vanilla, distinct from (and higher than)
	// ordinary ice's 0.98F.
	IsBlueIce(blockStateID uint32) bool

	BlockName(blockStateID uint32) string
	FullBlockName(blockStateID uint32) string

	// GetBlockProperties returns the named block state properties for a given state ID.
	// Returns {"shape": "north_south", "powered": "true", ...} for rail blocks.
	// Returns an empty map for air or unknown state IDs.
	GetBlockProperties(blockStateID uint32) map[string]string

	// GetMiningInfo returns the mining-relevant properties for a block state.
	// Returns hardness (negative = unbreakable), material tags (e.g. ["mineable/pickaxe"]),
	// and whether the block is diggable.
	GetMiningInfo(blockStateID uint32) (hardness float64, material []string, diggable bool)

	// GetWaterFlowDirection returns the direction water flows at the given block position.
	// Returns a normalized V3 vector (0,0,0) if not flowing water.
	// For flowing water (age 1-7), calculates flow direction toward lower age blocks.
	GetWaterFlowDirection(x, y, z int, world World) V3

	// GetWaterFlowSpeed returns the flow speed multiplier (0.0-1.0) based on water state.
	// Water sources (age 0) return 0.0 (no flow).
	// Flowing water (age 1-7) return proportional speed (age 1 = ~0.14, age 7 = 1.0).
	GetWaterFlowSpeed(blockStateID uint32) float64
}

// BlockRegistry provides block state ID lookup for world building.
type BlockRegistry interface {
	GetStateID(blockName string, properties map[string]string) uint32
}
