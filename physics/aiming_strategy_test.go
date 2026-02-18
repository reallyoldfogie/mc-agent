package physics

import (
	"math"
	"os"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestAimingStrategyToggle verifies the toggle mechanism works between implementations
func TestAimingStrategyToggle(t *testing.T) {
	origin := models.V3{X: 0, Y: 65, Z: 0}
	target := models.V3{X: 5, Y: 66, Z: 0}

	// Test with environment variable (uses old implementation)
	os.Setenv("USE_OLD_AIMING", "1")
	pitchOld, powerOld, errorOld, _ := FindOptimalAimingWithStrategy(models.Arrow, origin, target)
	t.Logf("Old implementation: pitch=%.2f°, error=%.4f", pitchOld, errorOld)
	require.Equal(t, 1.0, powerOld, "Power should always be 1.0")

	// Test without environment variable (uses new implementation)
	os.Unsetenv("USE_OLD_AIMING")
	pitchNew, powerNew, errorNew, _ := FindOptimalAimingWithStrategy(models.Arrow, origin, target)
	t.Logf("New implementation: pitch=%.2f°, error=%.4f", pitchNew, errorNew)

	// New implementation should find solution (the old one has known issues)
	require.NotEqual(t, math.MaxFloat64, errorNew, "New implementation should find solution")
	require.Equal(t, 1.0, powerNew, "Power should always be 1.0")
	require.Greater(t, pitchNew, -90.0, "Pitch should be reasonable")
	require.Less(t, pitchNew, 90.0, "Pitch should be reasonable")

	// Verify the results are different (proving toggle works)
	require.NotEqual(t, pitchOld, pitchNew, "Different implementations should be invoked")

	// Clean up
	os.Unsetenv("USE_OLD_AIMING")
}

// TestAimingStrategyUnreachable verifies both implementations handle unreachable targets
func TestAimingStrategyUnreachable(t *testing.T) {
	origin := models.V3{X: 0, Y: 65, Z: 0}
	target := models.V3{X: 1000, Y: 65, Z: 1000} // Very far away, unreachable

	// Test old implementation
	os.Setenv("USE_OLD_AIMING", "1")
	_, _, errorOld, trajOld := FindOptimalAimingWithStrategy(models.Arrow, origin, target)

	require.Equal(t, math.MaxFloat64, errorOld, "Old implementation should return max error for unreachable")
	require.Equal(t, 0, len(trajOld), "Should return empty trajectory for unreachable")

	// Test new implementation
	os.Unsetenv("USE_OLD_AIMING")
	_, _, errorNew, trajNew := FindOptimalAimingWithStrategy(models.Arrow, origin, target)

	require.Equal(t, math.MaxFloat64, errorNew, "New implementation should return max error for unreachable")
	require.Equal(t, 0, len(trajNew), "Should return empty trajectory for unreachable")

	// Clean up
	os.Unsetenv("USE_OLD_AIMING")
}

// TestGetProjectileProps verifies conversion from ProjectileType to ProjectileProps
func TestGetProjectileProps(t *testing.T) {
	props := GetProjectileProps(models.Arrow)

	require.Equal(t, ArrowInitialSpeed, props.Speed, "Speed should match")
	require.Equal(t, ArrowDrag, props.Drag, "Drag should match")
	require.Equal(t, ArrowGravity, props.Gravity, "Gravity should match")
	require.Equal(t, 400, props.MaxTicks, "MaxTicks should be 400")
	require.Equal(t, OrderMoveDragGravity, props.Order, "Order should be OrderMoveDragGravity")
}

// TestNewAimingImplementation verifies the new implementation works correctly
func TestNewAimingImplementation(t *testing.T) {
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

	os.Unsetenv("USE_OLD_AIMING")
	defer os.Unsetenv("USE_OLD_AIMING")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pitch, power, err, _ := FindOptimalAimingWithStrategy(models.Arrow, tc.origin, tc.target)

			// Should find solution
			require.NotEqual(t, math.MaxFloat64, err, "Should find solution for %s", tc.name)
			require.Equal(t, 1.0, power, "Power should be 1.0")
			require.Greater(t, pitch, -90.0, "Pitch should be reasonable")
			require.Less(t, pitch, 90.0, "Pitch should be reasonable")

			t.Logf("%s: pitch=%.2f°, error=%.4f", tc.name, pitch, err)
		})
	}
}
