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
}

// BlockRegistry provides block state ID lookup for world building.
type BlockRegistry interface {
	GetStateID(blockName string, properties map[string]string) uint32
}
