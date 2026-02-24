package movement_test

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/movement"
)

// MockPositionTracker tracks bot position for testing
type MockPositionTracker struct {
	x, y, z         float64
	yaw, pitch      float32
	positionHistory []Position
}

type Position struct {
	X, Y, Z    float64
	Yaw, Pitch float32
}

func (mpt *MockPositionTracker) GetPosition() (float64, float64, float64, float32, float32, bool) {
	return mpt.x, mpt.y, mpt.z, mpt.yaw, mpt.pitch, true
}

func (mpt *MockPositionTracker) SetPosition(x, y, z float64, yaw, pitch float32) {
	mpt.x = x
	mpt.y = y
	mpt.z = z
	mpt.yaw = yaw
	mpt.pitch = pitch

	// Record position history
	mpt.positionHistory = append(mpt.positionHistory, Position{
		X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch,
	})
}

func (mpt *MockPositionTracker) GetEntityID() int32 {
	return 1 // Mock entity ID
}

// TestMoveTowards_FlatGround tests movement on flat terrain
func TestMoveTowards_FlatGround(t *testing.T) {
	// Setup
	tracker := &MockPositionTracker{
		x: 0, y: 65, z: 0,
		yaw: 0, pitch: 0,
	}

	executor := movement.NewLegacyMovementExecutor(
		nil, // client not needed for test
		nil, // packetMgr not needed for test
		tracker.GetPosition,
		tracker.SetPosition,
		tracker.GetEntityID,
	)

	// Test: Move 0.2 blocks towards (10, 65, 0)
	newX, newY, newZ, err := executor.MoveTowards(10, 65, 0, 0.2, true)

	// Assert: No error
	if err != nil {
		t.Fatalf("MoveTowards failed: %v", err)
	}

	// Assert: Moved correct distance horizontally
	distance := math.Sqrt(newX*newX + newZ*newZ)
	expectedDist := 0.2
	if math.Abs(distance-expectedDist) > 0.01 {
		t.Errorf("Moved wrong distance. Expected %.2f, got %.2f", expectedDist, distance)
	}

	// Assert: Y unchanged (flat ground)
	if math.Abs(newY-65.0) > 0.01 {
		t.Errorf("Y changed on flat ground. Expected 65.0, got %.2f", newY)
	}

	t.Logf("Moved from (0, 65, 0) to (%.2f, %.2f, %.2f)", newX, newY, newZ)
}

// TestMoveTowards_UpwardMovement tests the smart physics fix for upward movement
func TestMoveTowards_UpwardMovement(t *testing.T) {
	// Setup
	tracker := &MockPositionTracker{
		x: 0, y: 65, z: 0,
	}

	executor := movement.NewLegacyMovementExecutor(
		nil,
		nil,
		tracker.GetPosition,
		tracker.SetPosition,
		tracker.GetEntityID,
	)

	// Test: Move towards higher position (stairs at Y=65.5)
	targetY := 65.5
	newX, newY, newZ, err := executor.MoveTowards(1, targetY, 0, 0.2, true)

	// Assert: No error
	if err != nil {
		t.Fatalf("MoveTowards failed: %v", err)
	}

	// Assert: Y reaches target immediately (smart physics fix)
	if math.Abs(newY-targetY) > 0.01 {
		t.Errorf("Y should reach target immediately. Expected %.2f, got %.2f", targetY, newY)
	}

	t.Logf("Upward movement: moved to (%.2f, %.2f, %.2f), Y went from 65.0 to %.2f (target %.2f)",
		newX, newY, newZ, newY, targetY)
}

// TestMoveTowards_DownwardMovement tests controlled descent (gravity simulation)
func TestMoveTowards_DownwardMovement(t *testing.T) {
	// Setup
	tracker := &MockPositionTracker{
		x: 0, y: 65, z: 0,
	}

	executor := movement.NewLegacyMovementExecutor(
		nil,
		nil,
		tracker.GetPosition,
		tracker.SetPosition,
		tracker.GetEntityID,
	)

	// Test: Move towards lower position (falling)
	targetY := 64.0
	newX, newY, newZ, err := executor.MoveTowards(1, targetY, 0, 0.2, true)

	// Assert: No error
	if err != nil {
		t.Fatalf("MoveTowards failed: %v", err)
	}

	// Assert: Y descends with gravity (max 0.5 blocks/tick)
	// dy = 65.0 - 64.0 = -1.0 (downward)
	// fallAmount = min(-dy, 0.5) = min(1.0, 0.5) = 0.5
	// newY = max(targetY, botY - fallAmount) = max(64.0, 65.0 - 0.5) = 64.5
	expectedY := 64.5
	if math.Abs(newY-expectedY) > 0.01 {
		t.Errorf("Y should descend gradually. Expected %.2f, got %.2f", expectedY, newY)
	}

	t.Logf("Downward movement: moved to (%.2f, %.2f, %.2f), Y went from 65.0 to %.2f (target %.2f, capped at 0.5 blocks/tick)",
		newX, newY, newZ, newY, targetY)
}

// TestMoveTowards_LevelMovement tests nearly level movement (< 0.1 block difference)
func TestMoveTowards_LevelMovement(t *testing.T) {
	// Setup
	tracker := &MockPositionTracker{
		x: 0, y: 65, z: 0,
	}

	executor := movement.NewLegacyMovementExecutor(
		nil,
		nil,
		tracker.GetPosition,
		tracker.SetPosition,
		tracker.GetEntityID,
	)

	// Test: Move towards position with tiny Y difference (< 0.1)
	targetY := 65.05
	newX, newY, newZ, err := executor.MoveTowards(1, targetY, 0, 0.2, true)

	// Assert: No error
	if err != nil {
		t.Fatalf("MoveTowards failed: %v", err)
	}

	// Assert: Y reaches target (treated as level movement when dy < 0.1)
	if math.Abs(newY-targetY) > 0.01 {
		t.Errorf("Y should reach target for level movement. Expected %.2f, got %.2f", targetY, newY)
	}

	t.Logf("Level movement: moved to (%.2f, %.2f, %.2f), Y went from 65.0 to %.2f (tiny difference)",
		newX, newY, newZ, newY)
}

// TestMoveTowards_MultipleSteps tests position history tracking
func TestMoveTowards_MultipleSteps(t *testing.T) {
	// Setup
	tracker := &MockPositionTracker{
		x: 0, y: 65, z: 0,
	}

	executor := movement.NewLegacyMovementExecutor(
		nil,
		nil,
		tracker.GetPosition,
		tracker.SetPosition,
		tracker.GetEntityID,
	)

	// Test: Move multiple times toward goal
	target := struct{ x, y, z float64 }{10, 65, 0}

	for i := range 5 {
		_, _, _, err := executor.MoveTowards(target.x, target.y, target.z, 0.2, true)
		if err != nil {
			t.Fatalf("Move %d failed: %v", i+1, err)
		}
	}

	// Assert: Position history recorded
	if len(tracker.positionHistory) != 5 {
		t.Errorf("Expected 5 position updates, got %d", len(tracker.positionHistory))
	}

	// Assert: Bot moved closer to target
	finalPos := tracker.positionHistory[len(tracker.positionHistory)-1]
	distance := math.Sqrt(math.Pow(target.x-finalPos.X, 2) + math.Pow(target.z-finalPos.Z, 2))

	if distance > 9.0 {
		t.Errorf("Bot didn't move closer to target. Distance: %.2f", distance)
	}

	t.Logf("After 5 moves: position (%.2f, %.2f, %.2f), distance to goal: %.2f",
		finalPos.X, finalPos.Y, finalPos.Z, distance)
}

// TestMoveTowards_AlreadyAtTarget tests behavior when already at target
func TestMoveTowards_AlreadyAtTarget(t *testing.T) {
	// Setup - bot already at target horizontally
	tracker := &MockPositionTracker{
		x: 10, y: 65, z: 10,
	}

	executor := movement.NewLegacyMovementExecutor(
		nil,
		nil,
		tracker.GetPosition,
		tracker.SetPosition,
		tracker.GetEntityID,
	)

	// Test: Try to move when already at horizontal target
	newX, newY, newZ, err := executor.MoveTowards(10, 65, 10, 0.2, true)

	// Assert: No error
	if err != nil {
		t.Fatalf("MoveTowards failed: %v", err)
	}

	// Assert: Position unchanged
	if math.Abs(newX-10) > 0.01 || math.Abs(newZ-10) > 0.01 {
		t.Errorf("Position changed when already at target")
	}

	t.Logf("Already at target - position unchanged: (%.2f, %.2f, %.2f)", newX, newY, newZ)
}
