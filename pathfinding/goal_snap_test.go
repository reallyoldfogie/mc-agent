package pathfinding

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
)

// TestSnapGoalToReachableGridMatchesLiveConfirmedFailure reproduces the
// exact coordinate pair from mc-rsi-trainer's own live shared-server
// training run: goal (264.2, -60, 3.9) queried from start
// (249.5, -60, 5.5) has its nearest actually-reachable node at
// (264.5, -60, 3.5), whose Euclidean distance to the raw goal computes
// to 0.5000000000000068 in float64 -- a hair over the default 0.5
// goalRadius. snapGoalToReachableGrid must produce that exact node.
func TestSnapGoalToReachableGridMatchesLiveConfirmedFailure(t *testing.T) {
	start := models.V3{X: 249.5, Y: -60, Z: 5.5}
	goal := models.V3{X: 264.2, Y: -60, Z: 3.9}

	snapped := snapGoalToReachableGrid(start, goal)

	assert.Equal(t, models.V3{X: 264.5, Y: -60, Z: 3.5}, snapped)
	assert.Zero(t, snapped.DistanceTo(snapped), "sanity: a node at the snapped position is exactly at itself")
}

func TestSnapGoalToReachableGridIsIdempotentWhenGoalAlreadyOnGrid(t *testing.T) {
	start := models.V3{X: 0.5, Y: 0, Z: 0.5}
	goal := models.V3{X: 5.5, Y: 0, Z: -3.5}

	assert.Equal(t, goal, snapGoalToReachableGrid(start, goal))
}

func TestSnapGoalToReachableGridRoundsEachAxisIndependently(t *testing.T) {
	start := models.V3{X: 0, Y: 0, Z: 0}

	tests := []struct {
		name string
		goal models.V3
		want models.V3
	}{
		{"rounds down", models.V3{X: 2.3, Y: 0, Z: 0}, models.V3{X: 2, Y: 0, Z: 0}},
		{"rounds up", models.V3{X: 2.7, Y: 0, Z: 0}, models.V3{X: 3, Y: 0, Z: 0}},
		{"rounds negative down", models.V3{X: -2.3, Y: 0, Z: 0}, models.V3{X: -2, Y: 0, Z: 0}},
		{"rounds negative up", models.V3{X: -2.7, Y: 0, Z: 0}, models.V3{X: -3, Y: 0, Z: 0}},
		{"all three axes independently", models.V3{X: 1.4, Y: 3.6, Z: -0.5}, models.V3{X: 1, Y: 4, Z: -1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, snapGoalToReachableGrid(start, tc.goal))
		})
	}
}

// TestSnapGoalToReachableGridIsRelativeToStartNotAnAbsoluteGrid confirms
// the fix doesn't assume start itself is already .5/integer-aligned:
// snapping is always relative to wherever start actually is, not to an
// externally-assumed absolute world grid.
func TestSnapGoalToReachableGridIsRelativeToStartNotAnAbsoluteGrid(t *testing.T) {
	start := models.V3{X: 0.37, Y: 0, Z: 0.12} // deliberately off any .5/integer grid
	goal := models.V3{X: 5.1, Y: 0, Z: -2.9}

	snapped := snapGoalToReachableGrid(start, goal)

	assert.Equal(t, models.V3{X: 5.37, Y: 0, Z: -2.88}, snapped)
	// The snapped goal must be reachable via whole-number steps from start.
	assert.InDelta(t, 5.0, snapped.X-start.X, 1e-9)
	assert.InDelta(t, -3.0, snapped.Z-start.Z, 1e-9)
}
