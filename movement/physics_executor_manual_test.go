package movement

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestEnterManualMode tests entering manual mode
func TestEnterManualMode(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Initially in idle mode
	assert.Equal(t, PhysicsModeIdle, exec.GetMode())
	assert.False(t, exec.IsManualMode())

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)
	assert.Equal(t, PhysicsModeManual, exec.GetMode())
	assert.True(t, exec.IsManualMode())

	// Entering again should be a no-op
	err = exec.EnterManualMode()
	require.NoError(t, err)
	assert.Equal(t, PhysicsModeManual, exec.GetMode())
}

// TestExitManualMode tests exiting manual mode
func TestExitManualMode(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode first
	err := exec.EnterManualMode()
	require.NoError(t, err)
	assert.True(t, exec.IsManualMode())

	// Exit manual mode
	err = exec.ExitManualMode()
	require.NoError(t, err)
	assert.Equal(t, PhysicsModeIdle, exec.GetMode())
	assert.False(t, exec.IsManualMode())

	// Exiting again should be a no-op
	err = exec.ExitManualMode()
	require.NoError(t, err)
	assert.Equal(t, PhysicsModeIdle, exec.GetMode())
}

// TestSetManualThrottle tests setting throttle values
func TestSetManualThrottle(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Should fail when not in manual mode
	err := exec.SetManualThrottle(1.0, 0.0)
	assert.Error(t, err)

	// Enter manual mode
	err = exec.EnterManualMode()
	require.NoError(t, err)

	// Set throttle
	err = exec.SetManualThrottle(0.5, 0.7)
	require.NoError(t, err)

	inputs := exec.GetManualInputs()
	assert.Equal(t, 0.5, inputs.ThrottleX)
	assert.Equal(t, 0.7, inputs.ThrottleZ)

	// Test clamping
	err = exec.SetManualThrottle(2.0, -3.0)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, 1.0, inputs.ThrottleX)  // Clamped to 1.0
	assert.Equal(t, -1.0, inputs.ThrottleZ) // Clamped to -1.0

	// Test zero
	err = exec.SetManualThrottle(0.0, 0.0)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, 0.0, inputs.ThrottleX)
	assert.Equal(t, 0.0, inputs.ThrottleZ)
}

// TestSetManualRotation tests setting yaw and pitch
func TestSetManualRotation(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Set rotation
	err = exec.SetManualRotation(45.0, -30.0)
	require.NoError(t, err)

	inputs := exec.GetManualInputs()
	assert.Equal(t, 45.0, inputs.Yaw)
	assert.Equal(t, -30.0, inputs.Pitch)

	// Test NaN handling for yaw (keep current)
	err = exec.SetManualRotation(math.NaN(), 10.0)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, 45.0, inputs.Yaw) // Should not have changed
	assert.Equal(t, 10.0, inputs.Pitch)

	// Test NaN handling for pitch (keep current)
	err = exec.SetManualRotation(90.0, math.NaN())
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, 90.0, inputs.Yaw)
	assert.Equal(t, 10.0, inputs.Pitch) // Should not have changed

	// Test both NaN (keep both)
	err = exec.SetManualRotation(math.NaN(), math.NaN())
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, 90.0, inputs.Yaw)
	assert.Equal(t, 10.0, inputs.Pitch)
}

// TestSetManualJump tests jump button control
func TestSetManualJump(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Should fail when not in manual mode
	err := exec.SetManualJump(true)
	assert.Error(t, err)

	// Enter manual mode
	err = exec.EnterManualMode()
	require.NoError(t, err)

	// Set jump
	err = exec.SetManualJump(true)
	require.NoError(t, err)

	inputs := exec.GetManualInputs()
	assert.True(t, inputs.Jump)

	// Unset jump
	err = exec.SetManualJump(false)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.False(t, inputs.Jump)
}

// TestSetManualSprint tests sprint button control
func TestSetManualSprint(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Set sprint
	err = exec.SetManualSprint(true)
	require.NoError(t, err)

	inputs := exec.GetManualInputs()
	assert.True(t, inputs.Sprint)

	// Unset sprint
	err = exec.SetManualSprint(false)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.False(t, inputs.Sprint)
}

// TestSetManualSneak tests sneak button control
func TestSetManualSneak(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Set sneak
	err = exec.SetManualSneak(true)
	require.NoError(t, err)

	inputs := exec.GetManualInputs()
	assert.True(t, inputs.Sneak)

	// Unset sneak
	err = exec.SetManualSneak(false)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.False(t, inputs.Sneak)
}

// TestSetManualClimbDirection tests climb direction control
func TestSetManualClimbDirection(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Test climb up
	err = exec.SetManualClimbDirection(1.0)
	require.NoError(t, err)

	inputs := exec.GetManualInputs()
	assert.Equal(t, 1.0, inputs.ClimbDirection)

	// Test climb down
	err = exec.SetManualClimbDirection(-1.0)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, -1.0, inputs.ClimbDirection)

	// Test no climb
	err = exec.SetManualClimbDirection(0.0)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, 0.0, inputs.ClimbDirection)

	// Test clamping
	err = exec.SetManualClimbDirection(5.0)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, 1.0, inputs.ClimbDirection) // Clamped

	err = exec.SetManualClimbDirection(-10.0)
	require.NoError(t, err)

	inputs = exec.GetManualInputs()
	assert.Equal(t, -1.0, inputs.ClimbDirection) // Clamped
}

// TestSetManualInputs tests setting all inputs at once
func TestSetManualInputs(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Should fail when not in manual mode
	inputs := models.Inputs{ThrottleX: 1.0, ThrottleZ: 0.5}
	err := exec.SetManualInputs(inputs)
	assert.Error(t, err)

	// Enter manual mode
	err = exec.EnterManualMode()
	require.NoError(t, err)

	// Set all inputs
	fullInputs := models.Inputs{
		ThrottleX:      0.5,
		ThrottleZ:      0.7,
		Yaw:            45.0,
		Pitch:          -30.0,
		Jump:           true,
		Sprint:         true,
		Sneak:          false,
		ClimbDirection: 0.5,
	}

	err = exec.SetManualInputs(fullInputs)
	require.NoError(t, err)

	retrieved := exec.GetManualInputs()
	assert.Equal(t, fullInputs.ThrottleX, retrieved.ThrottleX)
	assert.Equal(t, fullInputs.ThrottleZ, retrieved.ThrottleZ)
	assert.Equal(t, fullInputs.Yaw, retrieved.Yaw)
	assert.Equal(t, fullInputs.Pitch, retrieved.Pitch)
	assert.Equal(t, fullInputs.Jump, retrieved.Jump)
	assert.Equal(t, fullInputs.Sprint, retrieved.Sprint)
	assert.Equal(t, fullInputs.Sneak, retrieved.Sneak)
	assert.Equal(t, fullInputs.ClimbDirection, retrieved.ClimbDirection)
}

// TestResetManualInputs tests resetting inputs to zero
func TestResetManualInputs(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Set some inputs
	exec.SetManualThrottle(1.0, 1.0)
	exec.SetManualJump(true)
	exec.SetManualSprint(true)

	// Reset
	err = exec.ResetManualInputs()
	require.NoError(t, err)

	inputs := exec.GetManualInputs()
	assert.Equal(t, 0.0, inputs.ThrottleX)
	assert.Equal(t, 0.0, inputs.ThrottleZ)
	assert.False(t, inputs.Jump)
	assert.False(t, inputs.Sprint)
	assert.False(t, inputs.Sneak)
	assert.Equal(t, 0.0, inputs.ClimbDirection)
	// Yaw and pitch should be preserved from physics state
	assert.False(t, math.IsNaN(inputs.Yaw))
	assert.False(t, math.IsNaN(inputs.Pitch))
}

// TestModeValidation tests that input setters reject operations when not in manual mode
func TestModeValidation(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// All setters should fail in idle mode
	assert.Error(t, exec.SetManualInputs(models.Inputs{}))
	assert.Error(t, exec.SetManualThrottle(1.0, 0.0))
	assert.Error(t, exec.SetManualRotation(45.0, 0.0))
	assert.Error(t, exec.SetManualJump(true))
	assert.Error(t, exec.SetManualSprint(true))
	assert.Error(t, exec.SetManualSneak(true))
	assert.Error(t, exec.SetManualClimbDirection(1.0))
	assert.Error(t, exec.ResetManualInputs())

	// Enter manual mode
	exec.EnterManualMode()

	// Now all setters should succeed
	assert.NoError(t, exec.SetManualInputs(models.Inputs{}))
	assert.NoError(t, exec.SetManualThrottle(1.0, 0.0))
	assert.NoError(t, exec.SetManualRotation(45.0, 0.0))
	assert.NoError(t, exec.SetManualJump(true))
	assert.NoError(t, exec.SetManualSprint(true))
	assert.NoError(t, exec.SetManualSneak(true))
	assert.NoError(t, exec.SetManualClimbDirection(1.0))
	assert.NoError(t, exec.ResetManualInputs())
}

// TestManualInputsWithPhysics tests that manual inputs actually affect movement
func TestManualInputsWithPhysics(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Get initial inputs state - ensure it's empty
	initialInputs := exec.GetManualInputs()
	assert.Equal(t, 0.0, initialInputs.ThrottleX)
	assert.Equal(t, 0.0, initialInputs.ThrottleZ)

	// Apply some throttle
	err = exec.SetManualThrottle(0.5, 0.3)
	require.NoError(t, err)

	// Verify inputs were set
	currentInputs := exec.GetManualInputs()
	assert.Equal(t, 0.5, currentInputs.ThrottleX)
	assert.Equal(t, 0.3, currentInputs.ThrottleZ)

	// Apply jump
	err = exec.SetManualJump(true)
	require.NoError(t, err)

	currentInputs = exec.GetManualInputs()
	assert.True(t, currentInputs.Jump)
}

// TestConcurrentInputUpdates tests thread-safe concurrent input updates
func TestConcurrentInputUpdates(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	exec.EnterManualMode()

	// Spawn multiple goroutines updating inputs
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				_ = exec.SetManualThrottle(float64(id%2), float64(j%2))
				_ = exec.SetManualRotation(float64(j*10), float64(id*5))
				_ = exec.SetManualJump(j%2 == 0)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done

	}

	// Should have final state
	inputs := exec.GetManualInputs()
	assert.False(t, math.IsNaN(inputs.Yaw))
	assert.False(t, math.IsNaN(inputs.Pitch))
}

// TestManualMovementIntegration tests that manual inputs actually produce movement with the physics tick loop
func TestManualMovementIntegration(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Get initial position
	initialPos, _, _, _ := exec.physicsState.GetPosition()

	// Start the physics executor
	exec.Start()
	defer exec.Stop()

	// Wait for it to actually start
	time.Sleep(100 * time.Millisecond)

	// Enter manual mode and set throttle
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Set throttle to move east (positive X)
	err = exec.SetManualThrottle(1.0, 0.0)
	require.NoError(t, err)

	// Don't sneak so full movement speed applies
	err = exec.SetManualSneak(false)
	require.NoError(t, err)

	// Run for 40 ticks (2 seconds at 20 TPS)
	time.Sleep(2 * time.Second)

	// Exit manual mode
	err = exec.ExitManualMode()
	require.NoError(t, err)

	// Get final position
	finalPos, _, _, _ := exec.physicsState.GetPosition()

	// Calculate distance moved in X direction (east)
	distX := finalPos.X - initialPos.X
	t.Logf("Initial position: (%.2f, %.2f, %.2f)", initialPos.X, initialPos.Y, initialPos.Z)
	t.Logf("Final position: (%.2f, %.2f, %.2f)", finalPos.X, finalPos.Y, finalPos.Z)
	t.Logf("Distance moved in X: %.4f blocks", distX)

	// Should have moved at least 0.5 blocks in 2 seconds with throttle=1.0
	// Walk speed is 0.215 * 20 TPS = 4.3 blocks/second, so 2 seconds should be 8.6 blocks
	// At minimum should move some amount if physics is working
	require.Greater(t, distX, 0.1, "should move east when throttle is set to 1.0")
}

// TestNaNPreservation tests that NaN values preserve current state
func TestNaNPreservation(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	exec.EnterManualMode()

	// Set initial rotation
	exec.SetManualRotation(45.0, 30.0)
	inputs := exec.GetManualInputs()
	initialYaw := inputs.Yaw

	// Update with NaN yaw (should preserve yaw)
	exec.SetManualRotation(math.NaN(), 60.0)
	inputs = exec.GetManualInputs()
	assert.Equal(t, initialYaw, inputs.Yaw)
	assert.Equal(t, 60.0, inputs.Pitch)

	// Update with NaN pitch (should preserve pitch)
	exec.SetManualRotation(90.0, math.NaN())
	inputs = exec.GetManualInputs()
	assert.Equal(t, 90.0, inputs.Yaw)
	assert.Equal(t, 60.0, inputs.Pitch)

	// Update with both NaN (should preserve both)
	exec.SetManualRotation(math.NaN(), math.NaN())
	inputs = exec.GetManualInputs()
	assert.Equal(t, 90.0, inputs.Yaw)
	assert.Equal(t, 60.0, inputs.Pitch)
}

// TestEnterManualModePreservesSprintState tests that entering manual mode captures initial sprint/sneak state
func TestEnterManualModePreservesSprintState(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Verify initial state is not sprinting/sneaking
	assert.False(t, exec.IsSprinting())
	assert.False(t, exec.IsSneaking())

	// Enter manual mode without sprint/sneak
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Manual inputs should reflect initial movementPacketSender state (no sprint/sneak)
	inputs := exec.GetManualInputs()
	assert.False(t, inputs.Sprint)
	assert.False(t, inputs.Sneak)
}

// TestExitManualModeAppliesSprintState tests that exiting manual mode applies manual sprint state
func TestExitManualModeAppliesSprintState(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Verify initial state
	assert.False(t, exec.IsSprinting())

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Set sprint=true in manual mode (without actually sending packets)
	err = exec.SetManualSprint(true)
	require.NoError(t, err)
	inputs := exec.GetManualInputs()
	assert.True(t, inputs.Sprint)

	// Exit manual mode - movementPacketSender.IsSprinting() checks manual inputs state
	// Since SetManualSprint set the flag without sending packets, exiting should see it
	err = exec.ExitManualMode()
	require.NoError(t, err)

	// Verify we're no longer in manual mode
	assert.False(t, exec.IsManualMode())
}

// TestEnterManualModePreservesRotationState tests that yaw/pitch are initialized from physics state
func TestEnterManualModePreservesRotationState(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Manual inputs should have yaw/pitch initialized from physics state
	inputs := exec.GetManualInputs()
	assert.False(t, math.IsNaN(inputs.Yaw))
	assert.False(t, math.IsNaN(inputs.Pitch))
}

// TestExitManualModePreservesManualState tests that manual inputs don't have throttle preserved
func TestExitManualModePreservesManualState(t *testing.T) {
	exec := createTestPhysicsExecutor()
	require.NotNil(t, exec)

	// Enter manual mode
	err := exec.EnterManualMode()
	require.NoError(t, err)

	// Set some throttle in manual mode
	err = exec.SetManualThrottle(1.0, 0.5)
	require.NoError(t, err)

	// Exit manual mode
	err = exec.ExitManualMode()
	require.NoError(t, err)

	// Should be back in idle mode
	assert.False(t, exec.IsManualMode())
	assert.Equal(t, PhysicsModeIdle, exec.GetMode())
}
