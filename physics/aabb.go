package physics

import (
	"fmt"
)

// MinMax represents a one-dimensional range with minimum and maximum bounds.
type MinMax struct {
	Min, Max float64
}

// Extend adjusts the bounds of the MinMax range.
// For negative delta values, it reduces the minimum bound.
// For positive delta values, it increases the maximum bound.
func (mm MinMax) Extend(delta float64) MinMax {
	if delta < 0 {
		return MinMax{
			Min: mm.Min + delta,
			Max: mm.Max,
		}
	}
	return MinMax{
		Min: mm.Min,
		Max: mm.Max + delta,
	}
}

// Contract reduces both the minimum and maximum bounds by the provided amount,
// effectively shrinking the range. For positive values, the difference between
// Min and Max decreases.
func (mm MinMax) Contract(amt float64) MinMax {
	return MinMax{
		Min: mm.Min + amt,
		Max: mm.Max - amt,
	}
}

// Expand increases both the minimum and maximum bounds by the provided amount,
// effectively growing the range. For positive values, the difference between
// Min and Max increases.
func (mm MinMax) Expand(amt float64) MinMax {
	return MinMax{
		Min: mm.Min - amt,
		Max: mm.Max + amt,
	}
}

// Offset translates both the minimum and maximum bounds by the same amount,
// shifting the entire range without changing its size.
func (mm MinMax) Offset(amt float64) MinMax {
	return MinMax{
		Min: mm.Min + amt,
		Max: mm.Max + amt,
	}
}

// String returns a string representation of the MinMax range.
func (mm MinMax) String() string {
	return fmt.Sprintf("[%.2f, %.2f]", mm.Min, mm.Max)
}

// AABB (Axis-Aligned Bounding Box) represents a 3D rectangular collision box
// aligned with the world axes. Used for collision detection and resolution.
type AABB struct {
	X, Y, Z MinMax

	// BlockID is an optional reference to the block state ID this AABB represents.
	// Used for debugging and visualization. Zero value means no associated block.
	BlockID int32
}

// NewAABB creates a new AABB with the given bounds.
func NewAABB(minX, minY, minZ, maxX, maxY, maxZ float64) AABB {
	return AABB{
		X: MinMax{Min: minX, Max: maxX},
		Y: MinMax{Min: minY, Max: maxY},
		Z: MinMax{Min: minZ, Max: maxZ},
	}
}

// NewPlayerAABB creates an AABB representing the player's collision box
// centered at the given position (feet level).
func NewPlayerAABB(pos V3) AABB {
	return AABB{
		X: MinMax{Min: pos.X - PlayerWidth/2, Max: pos.X + PlayerWidth/2},
		Y: MinMax{Min: pos.Y, Max: pos.Y + PlayerHeight},
		Z: MinMax{Min: pos.Z - PlayerWidth/2, Max: pos.Z + PlayerWidth/2},
	}
}

// Extend adjusts the minimum (negative delta) or maximum (positive delta) bounds
// for each dimension.
func (bb AABB) Extend(dx, dy, dz float64) AABB {
	return AABB{
		X:       bb.X.Extend(dx),
		Y:       bb.Y.Extend(dy),
		Z:       bb.Z.Extend(dz),
		BlockID: bb.BlockID,
	}
}

// Contract reduces the bounds in all dimensions (positive values shrink the box).
func (bb AABB) Contract(x, y, z float64) AABB {
	return AABB{
		X:       bb.X.Contract(x),
		Y:       bb.Y.Contract(y),
		Z:       bb.Z.Contract(z),
		BlockID: bb.BlockID,
	}
}

// Expand increases the bounds in all dimensions (positive values grow the box).
func (bb AABB) Expand(x, y, z float64) AABB {
	return AABB{
		X:       bb.X.Expand(x),
		Y:       bb.Y.Expand(y),
		Z:       bb.Z.Expand(z),
		BlockID: bb.BlockID,
	}
}

// Offset translates the AABB by the given amounts in each dimension.
func (bb AABB) Offset(x, y, z float64) AABB {
	return AABB{
		X:       bb.X.Offset(x),
		Y:       bb.Y.Offset(y),
		Z:       bb.Z.Offset(z),
		BlockID: bb.BlockID,
	}
}

// XOffset resolves collision along the X axis.
// Given a desired X offset (movement), it returns the maximum offset possible
// before colliding with this AABB. If no collision would occur, returns the
// original offset unchanged.
//
// This implements the sweep-and-prune collision resolution used in Minecraft.
// The 'o' parameter is the other AABB (typically the player), and xOffset is
// the desired movement along X.
func (bb AABB) XOffset(o AABB, xOffset float64) float64 {
	// Only check collision if Y and Z ranges overlap
	if o.Y.Max > bb.Y.Min && o.Y.Min < bb.Y.Max &&
		o.Z.Max > bb.Z.Min && o.Z.Min < bb.Z.Max {

		// Moving right (positive X)
		if xOffset > 0.0 && o.X.Max <= bb.X.Min {
			// Calculate how far we can move before collision
			maxOffset := bb.X.Min - o.X.Max
			if maxOffset < xOffset {
				xOffset = maxOffset
			}
		}

		// Moving left (negative X)
		if xOffset < 0.0 && o.X.Min >= bb.X.Max {
			// Calculate how far we can move before collision
			maxOffset := bb.X.Max - o.X.Min
			if maxOffset > xOffset {
				xOffset = maxOffset
			}
		}
	}
	return xOffset
}

// YOffset resolves collision along the Y axis.
// Works identically to XOffset but for the Y dimension.
// Used for gravity and jumping collision resolution.
func (bb AABB) YOffset(o AABB, yOffset float64) float64 {
	// Only check collision if X and Z ranges overlap
	if o.X.Max > bb.X.Min && o.X.Min < bb.X.Max &&
		o.Z.Max > bb.Z.Min && o.Z.Min < bb.Z.Max {

		// Moving up (positive Y)
		if yOffset > 0.0 && o.Y.Max <= bb.Y.Min {
			maxOffset := bb.Y.Min - o.Y.Max
			if maxOffset < yOffset {
				yOffset = maxOffset
			}
		}

		// Moving down (negative Y)
		if yOffset < 0.0 && o.Y.Min >= bb.Y.Max {
			maxOffset := bb.Y.Max - o.Y.Min
			if maxOffset > yOffset {
				yOffset = maxOffset
			}
		}
	}
	return yOffset
}

// ZOffset resolves collision along the Z axis.
// Works identically to XOffset but for the Z dimension.
func (bb AABB) ZOffset(o AABB, zOffset float64) float64 {
	// Only check collision if X and Y ranges overlap
	if o.X.Max > bb.X.Min && o.X.Min < bb.X.Max &&
		o.Y.Max > bb.Y.Min && o.Y.Min < bb.Y.Max {

		// Moving forward (positive Z)
		if zOffset > 0.0 && o.Z.Max <= bb.Z.Min {
			maxOffset := bb.Z.Min - o.Z.Max
			if maxOffset < zOffset {
				zOffset = maxOffset
			}
		}

		// Moving backward (negative Z)
		if zOffset < 0.0 && o.Z.Min >= bb.Z.Max {
			maxOffset := bb.Z.Max - o.Z.Min
			if maxOffset > zOffset {
				zOffset = maxOffset
			}
		}
	}
	return zOffset
}

// Intersects returns true if this AABB overlaps with another AABB.
// Returns true even if they only touch (no gap between them).
func (bb AABB) Intersects(o AABB) bool {
	return bb.X.Min < o.X.Max && bb.X.Max > o.X.Min &&
		bb.Y.Min < o.Y.Max && bb.Y.Max > o.Y.Min &&
		bb.Z.Min < o.Z.Max && bb.Z.Max > o.Z.Min
}

// Contains returns true if point (x, y, z) is inside this AABB.
func (bb AABB) Contains(x, y, z float64) bool {
	return x >= bb.X.Min && x <= bb.X.Max &&
		y >= bb.Y.Min && y <= bb.Y.Max &&
		z >= bb.Z.Min && z <= bb.Z.Max
}

// Center returns the center point of the AABB.
func (bb AABB) Center() V3 {
	return V3{
		X: (bb.X.Min + bb.X.Max) / 2,
		Y: (bb.Y.Min + bb.Y.Max) / 2,
		Z: (bb.Z.Min + bb.Z.Max) / 2,
	}
}

// String returns a string representation of the AABB.
func (bb AABB) String() string {
	return fmt.Sprintf("AABB{X:%s, Y:%s, Z:%s}", bb.X, bb.Y, bb.Z)
}
