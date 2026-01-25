package physics

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAngle(t *testing.T) {
	tests := []struct {
		name     string
		angle    float64
		expected float64
	}{
		{"Zero", 0, 0},
		{"Positive small", 45, 45},
		{"Positive large", 270, -90},
		{"Negative small", -45, -45},
		{"Negative large", -270, 90},
		{"Exactly 180", 180, 180},
		{"Exactly -180", -180, -180},
		{"Wrap around 360", 370, 10},
		{"Wrap around -360", -370, -10},
		{"Large positive", 720, 0},
		{"Large negative", -720, 0},
		{"Just over 180", 190, -170},
		{"Just under -180", -190, 170},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeAngle(tt.angle)
			assert.InDelta(t, tt.expected, result, 0.001, "NormalizeAngle(%f) should equal %f", tt.angle, tt.expected)
		})
	}
}

func TestAngleDifference(t *testing.T) {
	tests := []struct {
		name     string
		from     float64
		to       float64
		expected float64
	}{
		{"Same angle", 0, 0, 0},
		{"Small forward", 0, 45, 45},
		{"Small backward", 45, 0, 45},
		{"90 degrees", 0, 90, 90},
		{"Wrap around (short path)", 10, 350, 20},
		{"Wrap around (other direction)", 350, 10, 20},
		{"180 degrees", 0, 180, 180},
		{"Nearly 180", 0, 179, 179},
		{"Large angle normalized", 0, 270, 90},
		{"Negative angles", -45, -135, 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AngleDifference(tt.from, tt.to)
			assert.InDelta(t, tt.expected, result, 0.001, "AngleDifference(%f, %f) should equal %f", tt.from, tt.to, tt.expected)
		})
	}
}

func TestRotateToward(t *testing.T) {
	tests := []struct {
		name     string
		current  float64
		target   float64
		maxDelta float64
		expected float64
	}{
		{"Already at target", 45, 45, 10, 45},
		{"Small rotation within limit", 0, 5, 10, 5},
		{"Large rotation limited", 0, 50, 10, 10},
		{"Negative rotation limited", 50, 0, 10, 40},
		{"Wrap around forward", 350, 10, 15, 5},    // 350 + 15 = 365 % 360 = 5
		{"Wrap around backward", 10, 350, 15, 355}, // 10 - 15 = -5 = 355
		{"Exactly at limit", 0, 10, 10, 10},
		{"Large target with small step", 0, 180, 11, 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RotateToward(tt.current, tt.target, tt.maxDelta)

			// Normalize both result and expected for comparison
			result = NormalizeAngle(result)
			expected := NormalizeAngle(tt.expected)

			assert.InDelta(t, expected, result, 0.001,
				"RotateToward(%f, %f, %f) should equal %f", tt.current, tt.target, tt.maxDelta, tt.expected)
		})
	}
}

func TestLimitRotation(t *testing.T) {
	tests := []struct {
		name          string
		currentYaw    float64
		currentPitch  float64
		targetYaw     float64
		targetPitch   float64
		expectedYaw   float64
		expectedPitch float64
	}{
		{
			name:          "No change needed",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     0,
			targetPitch:   0,
			expectedYaw:   0,
			expectedPitch: 0,
		},
		{
			name:          "Small change within limits",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     5,
			targetPitch:   3,
			expectedYaw:   5,
			expectedPitch: 3,
		},
		{
			name:          "Yaw limited to MaxYawChange",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     50,
			targetPitch:   0,
			expectedYaw:   MaxYawChange, // 11.0
			expectedPitch: 0,
		},
		{
			name:          "Pitch limited to MaxPitchChange",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     0,
			targetPitch:   50,
			expectedYaw:   0,
			expectedPitch: MaxPitchChange, // 7.0
		},
		{
			name:          "Both limited",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     100,
			targetPitch:   100,
			expectedYaw:   MaxYawChange,   // 11.0
			expectedPitch: MaxPitchChange, // 7.0
		},
		{
			name:          "Negative rotation limited",
			currentYaw:    100,
			currentPitch:  50,
			targetYaw:     0,
			targetPitch:   0,
			expectedYaw:   100 - MaxYawChange,  // 89.0
			expectedPitch: 50 - MaxPitchChange, // 43.0
		},
		{
			name:          "Wrap around yaw (short path)",
			currentYaw:    10,
			currentPitch:  0,
			targetYaw:     350,
			targetPitch:   0,
			expectedYaw:   10 - MaxYawChange, // -1.0
			expectedPitch: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newYaw, newPitch := LimitRotation(tt.currentYaw, tt.currentPitch, tt.targetYaw, tt.targetPitch)

			// Normalize for comparison
			newYaw = NormalizeAngle(newYaw)
			expectedYaw := NormalizeAngle(tt.expectedYaw)

			assert.InDelta(t, expectedYaw, newYaw, 0.001,
				"Yaw should be limited correctly")
			assert.InDelta(t, tt.expectedPitch, newPitch, 0.001,
				"Pitch should be limited correctly")
		})
	}
}

func TestIsLookingAt(t *testing.T) {
	tests := []struct {
		name           string
		currentYaw     float64
		currentPitch   float64
		targetYaw      float64
		targetPitch    float64
		yawTolerance   float64
		pitchTolerance float64
		expected       bool
	}{
		{
			name:           "Exact match",
			currentYaw:     45,
			currentPitch:   10,
			targetYaw:      45,
			targetPitch:    10,
			yawTolerance:   1,
			pitchTolerance: 1,
			expected:       true,
		},
		{
			name:           "Within tolerance",
			currentYaw:     45,
			currentPitch:   10,
			targetYaw:      45.5,
			targetPitch:    10.5,
			yawTolerance:   1,
			pitchTolerance: 1,
			expected:       true,
		},
		{
			name:           "Yaw outside tolerance",
			currentYaw:     45,
			currentPitch:   10,
			targetYaw:      47,
			targetPitch:    10,
			yawTolerance:   1,
			pitchTolerance: 1,
			expected:       false,
		},
		{
			name:           "Pitch outside tolerance",
			currentYaw:     45,
			currentPitch:   10,
			targetYaw:      45,
			targetPitch:    12,
			yawTolerance:   1,
			pitchTolerance: 1,
			expected:       false,
		},
		{
			name:           "Wrap around within tolerance",
			currentYaw:     359,
			currentPitch:   0,
			targetYaw:      1,
			targetPitch:    0,
			yawTolerance:   5,
			pitchTolerance: 1,
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLookingAt(tt.currentYaw, tt.currentPitch, tt.targetYaw, tt.targetPitch,
				tt.yawTolerance, tt.pitchTolerance)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTicksToRotate(t *testing.T) {
	tests := []struct {
		name          string
		currentYaw    float64
		currentPitch  float64
		targetYaw     float64
		targetPitch   float64
		expectedTicks int
	}{
		{
			name:          "Already at target",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     0,
			targetPitch:   0,
			expectedTicks: 0,
		},
		{
			name:          "One tick for yaw",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     10,
			targetPitch:   0,
			expectedTicks: 1, // 10 / 11 = 0.9 -> ceil = 1
		},
		{
			name:          "One tick for pitch",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     0,
			targetPitch:   6,
			expectedTicks: 1, // 6 / 7 = 0.86 -> ceil = 1
		},
		{
			name:          "Multiple ticks for yaw",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     50,
			targetPitch:   0,
			expectedTicks: 5, // 50 / 11 = 4.5 -> ceil = 5
		},
		{
			name:          "Multiple ticks for pitch",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     0,
			targetPitch:   25,
			expectedTicks: 4, // 25 / 7 = 3.57 -> ceil = 4
		},
		{
			name:          "Yaw takes longer",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     90,
			targetPitch:   14,
			expectedTicks: 9, // Yaw: 90/11 = 8.2 -> 9, Pitch: 14/7 = 2 -> max = 9
		},
		{
			name:          "Pitch takes longer",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     20,
			targetPitch:   50,
			expectedTicks: 8, // Yaw: 20/11 = 1.8 -> 2, Pitch: 50/7 = 7.1 -> 8, max = 8
		},
		{
			name:          "180 degree turn",
			currentYaw:    0,
			currentPitch:  0,
			targetYaw:     180,
			targetPitch:   0,
			expectedTicks: 17, // 180 / 11 = 16.4 -> ceil = 17
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TicksToRotate(tt.currentYaw, tt.currentPitch, tt.targetYaw, tt.targetPitch)
			assert.Equal(t, tt.expectedTicks, result,
				"TicksToRotate should calculate correct number of ticks")
		})
	}
}

// TestRotationAntiCheatCompliance verifies that rotation limiting prevents
// anti-cheat violations by never exceeding MaxYawChange or MaxPitchChange
func TestRotationAntiCheatCompliance(t *testing.T) {
	// Test many random rotations to ensure we never violate limits
	testCases := []struct {
		currentYaw, targetYaw     float64
		currentPitch, targetPitch float64
	}{
		{0, 180, 0, 90},
		{90, -90, 45, -45},
		{350, 10, -80, 80},
		{0, 359, 0, 0},
		{-180, 180, -90, 90},
	}

	for _, tc := range testCases {
		newYaw, newPitch := LimitRotation(tc.currentYaw, tc.currentPitch, tc.targetYaw, tc.targetPitch)

		// Check yaw change doesn't exceed limit
		yawChange := AngleDifference(tc.currentYaw, newYaw)
		require.LessOrEqual(t, yawChange, MaxYawChange+0.001, // +0.001 for floating point error
			"Yaw change should not exceed MaxYawChange (current: %f, new: %f, change: %f)",
			tc.currentYaw, newYaw, yawChange)

		// Check pitch change doesn't exceed limit
		pitchChange := math.Abs(newPitch - tc.currentPitch)
		require.LessOrEqual(t, pitchChange, MaxPitchChange+0.001,
			"Pitch change should not exceed MaxPitchChange (current: %f, new: %f, change: %f)",
			tc.currentPitch, newPitch, pitchChange)
	}
}

// TestRotationGradualProgress verifies that gradual rotation eventually reaches target
func TestRotationGradualProgress(t *testing.T) {
	currentYaw := 0.0
	currentPitch := 0.0
	targetYaw := 180.0
	targetPitch := 90.0

	maxIterations := 50 // Should be more than enough
	tolerance := 0.1

	for i := 0; i < maxIterations; i++ {
		// Check if we've reached target
		if IsLookingAt(currentYaw, currentPitch, targetYaw, targetPitch, tolerance, tolerance) {
			t.Logf("Reached target in %d iterations", i)
			return
		}

		// Apply one tick of rotation
		currentYaw, currentPitch = LimitRotation(currentYaw, currentPitch, targetYaw, targetPitch)
	}

	t.Errorf("Failed to reach target after %d iterations (current: yaw=%f pitch=%f, target: yaw=%f pitch=%f)",
		maxIterations, currentYaw, currentPitch, targetYaw, targetPitch)
}

// Benchmark rotation functions
func BenchmarkNormalizeAngle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NormalizeAngle(float64(i % 720))
	}
}

func BenchmarkAngleDifference(b *testing.B) {
	for i := 0; i < b.N; i++ {
		AngleDifference(float64(i%360), float64((i+90)%360))
	}
}

func BenchmarkLimitRotation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		LimitRotation(0, 0, 180, 90)
	}
}

func BenchmarkRotateToward(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RotateToward(0, 180, 11)
	}
}
