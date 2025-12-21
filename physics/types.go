package physics

import (
	"github.com/reallyoldfogie/mc-agent/pathfinding"
)

// V3 is an alias to pathfinding.V3 for 3D coordinates.
// We reuse the existing V3 type which already supports float64 coordinates.
type V3 = pathfinding.V3

// NewV3 creates a new V3 position.
func NewV3(x, y, z float64) V3 {
	return V3{X: x, Y: y, Z: z}
}

// V3Sub subtracts v2 from v1.
func V3Sub(v1, v2 V3) V3 {
	return V3{X: v1.X - v2.X, Y: v1.Y - v2.Y, Z: v1.Z - v2.Z}
}

// V3Mul multiplies a V3 by a scalar.
func V3Mul(v V3, scalar float64) V3 {
	return V3{X: v.X * scalar, Y: v.Y * scalar, Z: v.Z * scalar}
}

// V3Length returns the magnitude of the vector.
func V3Length(v V3) float64 {
	return v.DistanceTo(V3{})
}
