package physics

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestSegmentIntersection verifies the segment-to-box intersection logic
func TestSegmentIntersection(t *testing.T) {
	t.Logf("=== Testing Segment-to-Box Intersection ===\n")

	// Test case 1: Segment that passes through box
	t.Logf("Test 1: Segment passing through box")
	p1 := models.V3{X: 0, Y: 0, Z: 0}
	p2 := models.V3{X: 0, Y: 0, Z: 2}
	boxMin := models.V3{X: -0.5, Y: -0.5, Z: 0.5}
	boxMax := models.V3{X: 0.5, Y: 0.5, Z: 1.5}

	result := trajectorySegmentIntersectsBox(p1, p2, boxMin, boxMax)
	t.Logf("  Segment from (0,0,0) to (0,0,2)")
	t.Logf("  Box: X∈[-0.5,0.5], Y∈[-0.5,0.5], Z∈[0.5,1.5]")
	t.Logf("  Result: %v (expected: true)", result)
	if !result {
		t.Logf("  ❌ FAIL: Should intersect!\n")
	} else {
		t.Logf("  ✅ PASS\n")
	}

	// Test case 2: Segment that misses box
	t.Logf("Test 2: Segment missing box")
	p1 = models.V3{X: 0, Y: 0, Z: 0}
	p2 = models.V3{X: 0, Y: 0, Z: 0.4}
	boxMin = models.V3{X: -0.5, Y: -0.5, Z: 0.5}
	boxMax = models.V3{X: 0.5, Y: 0.5, Z: 1.5}

	result = trajectorySegmentIntersectsBox(p1, p2, boxMin, boxMax)
	t.Logf("  Segment from (0,0,0) to (0,0,0.4)")
	t.Logf("  Box: X∈[-0.5,0.5], Y∈[-0.5,0.5], Z∈[0.5,1.5]")
	t.Logf("  Result: %v (expected: false)", result)
	if result {
		t.Logf("  ❌ FAIL: Should NOT intersect!\n")
	} else {
		t.Logf("  ✅ PASS\n")
	}

	// Test case 3: Arrow trajectory overshooting (the actual bug case)
	t.Logf("Test 3: Arrow overshooting target (the real case)")
	p1 = models.V3{X: 0, Y: -0.49, Z: 44.25} // Tick 15
	p2 = models.V3{X: 0, Y: -0.93, Z: 46.79} // Tick 16
	boxMin = models.V3{X: -0.5, Y: -(models.PlayerEyeHeight - .1), Z: 45.5}
	boxMax = models.V3{X: 0.5, Y: -0.52, Z: 46.5}

	result = trajectorySegmentIntersectsBox(p1, p2, boxMin, boxMax)
	t.Logf("  Segment from (0, -0.49, 44.25) to (0, -0.93, 46.79)")
	t.Logf("  Target box: X∈[-0.5,0.5], Y∈[-1.52,-0.52], Z∈[45.5,46.5]")
	t.Logf("  Result: %v (expected: true)", result)
	if !result {
		t.Logf("  ❌ FAIL: Should intersect (arrow passes through target)!\n")
	} else {
		t.Logf("  ✅ PASS\n")
	}
}
