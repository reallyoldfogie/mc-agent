package physics

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestMinMax_Extend tests the Extend method for MinMax.
func TestMinMax_Extend(t *testing.T) {
	tests := []struct {
		name     string
		mm       MinMax
		delta    float64
		expected MinMax
	}{
		{
			name:     "Extend positive - increases max",
			mm:       MinMax{Min: 0, Max: 10},
			delta:    5,
			expected: MinMax{Min: 0, Max: 15},
		},
		{
			name:     "Extend negative - decreases min",
			mm:       MinMax{Min: 0, Max: 10},
			delta:    -5,
			expected: MinMax{Min: -5, Max: 10},
		},
		{
			name:     "Extend zero - no change",
			mm:       MinMax{Min: 0, Max: 10},
			delta:    0,
			expected: MinMax{Min: 0, Max: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mm.Extend(tt.delta)
			if result != tt.expected {
				t.Errorf("Extend(%v) = %v, want %v", tt.delta, result, tt.expected)
			}
		})
	}
}

// TestMinMax_Contract tests the Contract method for MinMax.
func TestMinMax_Contract(t *testing.T) {
	tests := []struct {
		name     string
		mm       MinMax
		amt      float64
		expected MinMax
	}{
		{
			name:     "Contract positive - shrinks range",
			mm:       MinMax{Min: 0, Max: 10},
			amt:      2,
			expected: MinMax{Min: 2, Max: 8},
		},
		{
			name:     "Contract negative - grows range",
			mm:       MinMax{Min: 0, Max: 10},
			amt:      -2,
			expected: MinMax{Min: -2, Max: 12},
		},
		{
			name:     "Contract zero - no change",
			mm:       MinMax{Min: 0, Max: 10},
			amt:      0,
			expected: MinMax{Min: 0, Max: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mm.Contract(tt.amt)
			if result != tt.expected {
				t.Errorf("Contract(%v) = %v, want %v", tt.amt, result, tt.expected)
			}
		})
	}
}

// TestMinMax_Expand tests the Expand method for MinMax.
func TestMinMax_Expand(t *testing.T) {
	tests := []struct {
		name     string
		mm       MinMax
		amt      float64
		expected MinMax
	}{
		{
			name:     "Expand positive - grows range",
			mm:       MinMax{Min: 5, Max: 10},
			amt:      2,
			expected: MinMax{Min: 3, Max: 12},
		},
		{
			name:     "Expand negative - shrinks range",
			mm:       MinMax{Min: 5, Max: 10},
			amt:      -2,
			expected: MinMax{Min: 7, Max: 8},
		},
		{
			name:     "Expand zero - no change",
			mm:       MinMax{Min: 5, Max: 10},
			amt:      0,
			expected: MinMax{Min: 5, Max: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mm.Expand(tt.amt)
			if result != tt.expected {
				t.Errorf("Expand(%v) = %v, want %v", tt.amt, result, tt.expected)
			}
		})
	}
}

// TestMinMax_Offset tests the Offset method for MinMax.
func TestMinMax_Offset(t *testing.T) {
	tests := []struct {
		name     string
		mm       MinMax
		amt      float64
		expected MinMax
	}{
		{
			name:     "Offset positive - shifts right",
			mm:       MinMax{Min: 0, Max: 10},
			amt:      5,
			expected: MinMax{Min: 5, Max: 15},
		},
		{
			name:     "Offset negative - shifts left",
			mm:       MinMax{Min: 0, Max: 10},
			amt:      -5,
			expected: MinMax{Min: -5, Max: 5},
		},
		{
			name:     "Offset zero - no change",
			mm:       MinMax{Min: 0, Max: 10},
			amt:      0,
			expected: MinMax{Min: 0, Max: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mm.Offset(tt.amt)
			if result != tt.expected {
				t.Errorf("Offset(%v) = %v, want %v", tt.amt, result, tt.expected)
			}
		})
	}
}

// TestAABB_Offset tests the Offset method for AABB.
func TestAABB_Offset(t *testing.T) {
	bb := NewAABB(0, 0, 0, 1, 1, 1)
	result := bb.Offset(5, 10, -3)

	expected := NewAABB(5, 10, -3, 6, 11, -2)
	if result.X != expected.X || result.Y != expected.Y || result.Z != expected.Z {
		t.Errorf("Offset(5, 10, -3) = %v, want %v", result, expected)
	}
}

// TestAABB_XOffset tests X-axis collision resolution.
func TestAABB_XOffset(t *testing.T) {
	tests := []struct {
		name     string
		bb       AABB // Obstacle
		o        AABB // Moving object
		xOffset  float64
		expected float64
	}{
		{
			name:     "No collision - Y doesn't overlap",
			bb:       NewAABB(5, 5, 0, 10, 10, 1),
			o:        NewAABB(0, 0, 0, 1, 1, 1), // Below obstacle
			xOffset:  10,
			expected: 10, // No collision, full movement
		},
		{
			name:     "No collision - Z doesn't overlap",
			bb:       NewAABB(5, 0, 5, 10, 1, 10),
			o:        NewAABB(0, 0, 0, 1, 1, 1), // Different Z
			xOffset:  10,
			expected: 10, // No collision, full movement
		},
		{
			name:     "Collision moving right - stops at obstacle",
			bb:       NewAABB(5, 0, 0, 10, 1, 1),
			o:        NewAABB(0, 0, 0, 1, 1, 1),
			xOffset:  10,
			expected: 4, // Stops when o.X.Max (1) reaches bb.X.Min (5), so offset = 5-1 = 4
		},
		{
			name:     "Collision moving left - stops at obstacle",
			bb:       NewAABB(-10, 0, 0, -5, 1, 1),
			o:        NewAABB(0, 0, 0, 1, 1, 1),
			xOffset:  -10,
			expected: -5, // Stops when o.X.Min (0) reaches bb.X.Max (-5), so offset = -5-0 = -5
		},
		{
			name:     "No collision - moving away from obstacle",
			bb:       NewAABB(5, 0, 0, 10, 1, 1),
			o:        NewAABB(0, 0, 0, 1, 1, 1),
			xOffset:  -5, // Moving left, away from obstacle on right
			expected: -5, // No collision
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.bb.XOffset(tt.o, tt.xOffset)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("XOffset() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestAABB_YOffset tests Y-axis collision resolution.
func TestAABB_YOffset(t *testing.T) {
	tests := []struct {
		name     string
		bb       AABB
		o        AABB
		yOffset  float64
		expected float64
	}{
		{
			name:     "Collision moving up - stops at ceiling",
			bb:       NewAABB(0, 5, 0, 1, 10, 1),
			o:        NewAABB(0, 0, 0, 1, 1.8, 1), // Player height
			yOffset:  10,
			expected: 3.2, // Stops when o.Y.Max (1.8) reaches bb.Y.Min (5), so offset = 5-1.8 = 3.2
		},
		{
			name:     "Collision moving down - stops at floor",
			bb:       NewAABB(0, -1, 0, 1, 0, 1),
			o:        NewAABB(0, 5, 0, 1, 6.8, 1),
			yOffset:  -10,
			expected: -5, // Stops when o.Y.Min (5) reaches bb.Y.Max (0), so offset = 0-5 = -5
		},
		{
			name:     "No collision - X doesn't overlap",
			bb:       NewAABB(5, 0, 0, 10, 1, 1),
			o:        NewAABB(0, 0, 0, 1, 1, 1),
			yOffset:  -5,
			expected: -5, // No collision
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.bb.YOffset(tt.o, tt.yOffset)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("YOffset() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestAABB_ZOffset tests Z-axis collision resolution.
func TestAABB_ZOffset(t *testing.T) {
	tests := []struct {
		name     string
		bb       AABB
		o        AABB
		zOffset  float64
		expected float64
	}{
		{
			name:     "Collision moving forward - stops at obstacle",
			bb:       NewAABB(0, 0, 5, 1, 1, 10),
			o:        NewAABB(0, 0, 0, 1, 1, 1),
			zOffset:  10,
			expected: 4, // Stops when o.Z.Max (1) reaches bb.Z.Min (5), so offset = 5-1 = 4
		},
		{
			name:     "Collision moving backward - stops at obstacle",
			bb:       NewAABB(0, 0, -10, 1, 1, -5),
			o:        NewAABB(0, 0, 0, 1, 1, 1),
			zOffset:  -10,
			expected: -5, // Stops when o.Z.Min (0) reaches bb.Z.Max (-5), so offset = -5-0 = -5
		},
		{
			name:     "No collision - Y doesn't overlap",
			bb:       NewAABB(0, 5, 0, 1, 10, 1),
			o:        NewAABB(0, 0, 0, 1, 1, 1),
			zOffset:  10,
			expected: 10, // No collision
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.bb.ZOffset(tt.o, tt.zOffset)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("ZOffset() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestAABB_Intersects tests AABB intersection detection.
func TestAABB_Intersects(t *testing.T) {
	tests := []struct {
		name     string
		bb1      AABB
		bb2      AABB
		expected bool
	}{
		{
			name:     "Overlapping AABBs",
			bb1:      NewAABB(0, 0, 0, 10, 10, 10),
			bb2:      NewAABB(5, 5, 5, 15, 15, 15),
			expected: true,
		},
		{
			name:     "Touching AABBs (shared edge)",
			bb1:      NewAABB(0, 0, 0, 10, 10, 10),
			bb2:      NewAABB(10, 0, 0, 20, 10, 10),
			expected: false, // Touching but not overlapping (< and > in Intersects)
		},
		{
			name:     "Separated AABBs",
			bb1:      NewAABB(0, 0, 0, 10, 10, 10),
			bb2:      NewAABB(20, 20, 20, 30, 30, 30),
			expected: false,
		},
		{
			name:     "Nested AABBs",
			bb1:      NewAABB(0, 0, 0, 10, 10, 10),
			bb2:      NewAABB(2, 2, 2, 8, 8, 8),
			expected: true,
		},
		{
			name:     "Same AABB",
			bb1:      NewAABB(0, 0, 0, 10, 10, 10),
			bb2:      NewAABB(0, 0, 0, 10, 10, 10),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.bb1.Intersects(tt.bb2)
			if result != tt.expected {
				t.Errorf("Intersects() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestAABB_Contains tests point containment.
func TestAABB_Contains(t *testing.T) {
	bb := NewAABB(0, 0, 0, 10, 10, 10)

	tests := []struct {
		name     string
		x, y, z  float64
		expected bool
	}{
		{"Center point", 5, 5, 5, true},
		{"Corner point (min)", 0, 0, 0, true},
		{"Corner point (max)", 10, 10, 10, true},
		{"Outside point", 15, 5, 5, false},
		{"Edge point", 10, 5, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bb.Contains(tt.x, tt.y, tt.z)
			if result != tt.expected {
				t.Errorf("Contains(%v, %v, %v) = %v, want %v",
					tt.x, tt.y, tt.z, result, tt.expected)
			}
		})
	}
}

// TestAABB_Center tests center point calculation.
func TestAABB_Center(t *testing.T) {
	bb := NewAABB(0, 0, 0, 10, 20, 30)
	center := bb.Center()

	expected := models.V3{X: 5, Y: 10, Z: 15}
	if center.X != expected.X || center.Y != expected.Y || center.Z != expected.Z {
		t.Errorf("Center() = %v, want %v", center, expected)
	}
}

// TestNewPlayerAABB tests player AABB creation.
func TestNewPlayerAABB(t *testing.T) {
	pos := models.V3{X: 10, Y: 64, Z: 20}
	bb := NewPlayerAABB(pos)

	// Player should be 0.6 wide (0.3 on each side of center)
	expectedMinX := 10 - PlayerWidth/2
	expectedMaxX := 10 + PlayerWidth/2
	expectedMinY := 64.0
	expectedMaxY := 64 + PlayerHeight
	expectedMinZ := 20 - PlayerWidth/2
	expectedMaxZ := 20 + PlayerWidth/2

	if math.Abs(bb.X.Min-expectedMinX) > 0.0001 ||
		math.Abs(bb.X.Max-expectedMaxX) > 0.0001 ||
		math.Abs(bb.Y.Min-expectedMinY) > 0.0001 ||
		math.Abs(bb.Y.Max-expectedMaxY) > 0.0001 ||
		math.Abs(bb.Z.Min-expectedMinZ) > 0.0001 ||
		math.Abs(bb.Z.Max-expectedMaxZ) > 0.0001 {
		t.Errorf("NewPlayerAABB() = %v, want X:[%v, %v], Y:[%v, %v], Z:[%v, %v]",
			bb, expectedMinX, expectedMaxX, expectedMinY, expectedMaxY, expectedMinZ, expectedMaxZ)
	}
}

// BenchmarkAABB_XOffset benchmarks X-axis collision resolution.
func BenchmarkAABB_XOffset(b *testing.B) {
	bb := NewAABB(5, 0, 0, 10, 1, 1)
	o := NewAABB(0, 0, 0, 1, 1, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bb.XOffset(o, 10)
	}
}

// BenchmarkAABB_Intersects benchmarks intersection detection.
func BenchmarkAABB_Intersects(b *testing.B) {
	bb1 := NewAABB(0, 0, 0, 10, 10, 10)
	bb2 := NewAABB(5, 5, 5, 15, 15, 15)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bb1.Intersects(bb2)
	}
}
