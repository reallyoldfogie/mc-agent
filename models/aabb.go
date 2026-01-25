package models

import "fmt"

// MinMax represents a one-dimensional range with minimum and maximum bounds.
type MinMax struct {
	Min, Max float64
}

// Extend adjusts the bounds of the MinMax range.
func (mm MinMax) Extend(delta float64) MinMax {
	if delta < 0 {
		return MinMax{Min: mm.Min + delta, Max: mm.Max}
	}
	return MinMax{Min: mm.Min, Max: mm.Max + delta}
}

// Contract reduces both the minimum and maximum bounds by the provided amount.
func (mm MinMax) Contract(amt float64) MinMax {
	return MinMax{Min: mm.Min + amt, Max: mm.Max - amt}
}

// Expand increases both the minimum and maximum bounds by the provided amount.
func (mm MinMax) Expand(amt float64) MinMax {
	return MinMax{Min: mm.Min - amt, Max: mm.Max + amt}
}

// Offset translates both the minimum and maximum bounds by the same amount.
func (mm MinMax) Offset(amt float64) MinMax {
	return MinMax{Min: mm.Min + amt, Max: mm.Max + amt}
}

// String returns a string representation of the MinMax range.
func (mm MinMax) String() string {
	return fmt.Sprintf("[%.2f, %.2f]", mm.Min, mm.Max)
}

// AABB (Axis-Aligned Bounding Box) represents a 3D rectangular collision box.
type AABB struct {
	X, Y, Z MinMax
	BlockID uint32
}

// NewAABB creates a new AABB with the given bounds.
func NewAABB(minX, minY, minZ, maxX, maxY, maxZ float64) AABB {
	return AABB{
		X: MinMax{Min: minX, Max: maxX},
		Y: MinMax{Min: minY, Max: maxY},
		Z: MinMax{Min: minZ, Max: maxZ},
	}
}

// NewPlayerAABB creates an AABB representing the player's collision box.
func NewPlayerAABB(pos V3) AABB {
	return AABB{
		X: MinMax{Min: pos.X - PlayerWidth/2, Max: pos.X + PlayerWidth/2},
		Y: MinMax{Min: pos.Y, Max: pos.Y + PlayerHeight},
		Z: MinMax{Min: pos.Z - PlayerWidth/2, Max: pos.Z + PlayerWidth/2},
	}
}

// Extend adjusts the minimum (negative delta) or maximum (positive delta) bounds.
func (bb AABB) Extend(dx, dy, dz float64) AABB {
	return AABB{X: bb.X.Extend(dx), Y: bb.Y.Extend(dy), Z: bb.Z.Extend(dz), BlockID: bb.BlockID}
}

// Contract reduces the bounds in all dimensions (positive values shrink the box).
func (bb AABB) Contract(x, y, z float64) AABB {
	return AABB{X: bb.X.Contract(x), Y: bb.Y.Contract(y), Z: bb.Z.Contract(z), BlockID: bb.BlockID}
}

// Expand increases the bounds in all dimensions (positive values grow the box).
func (bb AABB) Expand(x, y, z float64) AABB {
	return AABB{X: bb.X.Expand(x), Y: bb.Y.Expand(y), Z: bb.Z.Expand(z), BlockID: bb.BlockID}
}

// Offset translates the AABB by the given amounts in each dimension.
func (bb AABB) Offset(x, y, z float64) AABB {
	return AABB{X: bb.X.Offset(x), Y: bb.Y.Offset(y), Z: bb.Z.Offset(z), BlockID: bb.BlockID}
}

// Intersects reports whether this AABB overlaps another (touching edges is not intersection).
func (bb AABB) Intersects(o AABB) bool {
	return bb.X.Max > o.X.Min && bb.X.Min < o.X.Max &&
		bb.Y.Max > o.Y.Min && bb.Y.Min < o.Y.Max &&
		bb.Z.Max > o.Z.Min && bb.Z.Min < o.Z.Max
}

// Contains reports whether the point lies within the AABB (inclusive of edges).
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

// XOffset resolves collision along the X axis.
func (bb AABB) XOffset(o AABB, xOffset float64) float64 {
	if o.Y.Max > bb.Y.Min && o.Y.Min < bb.Y.Max &&
		o.Z.Max > bb.Z.Min && o.Z.Min < bb.Z.Max {
		if xOffset > 0.0 && o.X.Max <= bb.X.Min {
			maxOffset := bb.X.Min - o.X.Max
			if maxOffset < xOffset {
				xOffset = maxOffset
			}
		}
		if xOffset < 0.0 && o.X.Min >= bb.X.Max {
			maxOffset := bb.X.Max - o.X.Min
			if maxOffset > xOffset {
				xOffset = maxOffset
			}
		}
	}
	return xOffset
}

// YOffset resolves collision along the Y axis.
func (bb AABB) YOffset(o AABB, yOffset float64) float64 {
	if o.X.Max > bb.X.Min && o.X.Min < bb.X.Max &&
		o.Z.Max > bb.Z.Min && o.Z.Min < bb.Z.Max {
		if yOffset > 0.0 && o.Y.Max <= bb.Y.Min {
			maxOffset := bb.Y.Min - o.Y.Max
			if maxOffset < yOffset {
				yOffset = maxOffset
			}
		}
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
func (bb AABB) ZOffset(o AABB, zOffset float64) float64 {
	if o.X.Max > bb.X.Min && o.X.Min < bb.X.Max &&
		o.Y.Max > bb.Y.Min && o.Y.Min < bb.Y.Max {
		if zOffset > 0.0 && o.Z.Max <= bb.Z.Min {
			maxOffset := bb.Z.Min - o.Z.Max
			if maxOffset < zOffset {
				zOffset = maxOffset
			}
		}
		if zOffset < 0.0 && o.Z.Min >= bb.Z.Max {
			maxOffset := bb.Z.Max - o.Z.Min
			if maxOffset > zOffset {
				zOffset = maxOffset
			}
		}
	}
	return zOffset
}
