package physics

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestGetProjectileProps verifies conversion from ProjectileType to ProjectileProps
func TestGetProjectileProps(t *testing.T) {
	props := GetProjectileProps(models.Arrow)

	require.Equal(t, ArrowInitialSpeed, props.Speed, "Speed should match")
	require.Equal(t, ArrowDrag, props.Drag, "Drag should match")
	require.Equal(t, ArrowGravity, props.Gravity, "Gravity should match")
	require.Equal(t, 400, props.MaxTicks, "MaxTicks should be 400")
	require.Equal(t, OrderMoveDragGravity, props.Order, "Order should be OrderMoveDragGravity")
}

// TestAimingImplementation verifies the new implementation works correctly
func TestAimingImplementation(t *testing.T) {
	testCases := []struct {
		name   string
		origin models.V3
		target models.V3
	}{
		{
			name:   "short upward shot",
			origin: models.V3{X: 0, Y: 65, Z: 0},
			target: models.V3{X: 10, Y: 67, Z: 0},
		},
		{
			name:   "medium upward shot",
			origin: models.V3{X: 0, Y: 65, Z: 0},
			target: models.V3{X: 15, Y: 70, Z: 0},
		},
		{
			name:   "downward shot",
			origin: models.V3{X: 0, Y: 75, Z: 0},
			target: models.V3{X: 10, Y: 68, Z: 0},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pitch, power, err, _ := FindOptimalAiming(models.Arrow, tc.origin, tc.target)

			// Should find solution
			require.NotEqual(t, math.MaxFloat64, err, "Should find solution for %s", tc.name)
			require.Equal(t, 1.0, power, "Power should be 1.0")
			require.Greater(t, pitch, -90.0, "Pitch should be reasonable")
			require.Less(t, pitch, 90.0, "Pitch should be reasonable")

			t.Logf("%s: pitch=%.2f°, error=%.4f", tc.name, pitch, err)
		})
	}
}
