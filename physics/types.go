package physics

import (
	"github.com/reallyoldfogie/mc-agent/models"
)

// V3 is an alias to models.V3 for 3D coordinates.
type V3 = models.V3

// NewV3 creates a new V3 position.
func NewV3(x, y, z float64) V3 {
	return V3{X: x, Y: y, Z: z}
}

// V3Sub subtracts v2 from v1.
func V3Sub(v1, v2 V3) V3 {
	return models.V3Sub(v1, v2)
}

// V3Mul multiplies a V3 by a scalar.
func V3Mul(v V3, scalar float64) V3 {
	return V3{X: v.X * scalar, Y: v.Y * scalar, Z: v.Z * scalar}
}

// V3Length returns the magnitude of the vector.
func V3Length(v V3) float64 {
	return v.DistanceTo(V3{X: 0, Y: 0, Z: 0})
}
