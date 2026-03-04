package movement

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// MockWorld implements physics.World for testing
type MockWorld struct {
	blocks map[[3]int]uint32
}

func NewMockWorld() *MockWorld {
	return &MockWorld{
		blocks: make(map[[3]int]uint32),
	}
}

func (mw *MockWorld) GetBlockStatus(x, y, z int) (uint32, bool) {
	val, found := mw.blocks[[3]int{x, y, z}]
	return val, found
}

func (mw *MockWorld) SetBlock(x, y, z int, blockState uint32) {
	mw.blocks[[3]int{x, y, z}] = blockState
}

// MockShapeProvider implements physics.BlockShapeProvider for testing
type MockShapeProvider struct{}

func (msp *MockShapeProvider) GetCollisionBoxes(blockStateID uint32, x, y, z int) []physics.AABB {
	if blockStateID == 0 {
		// Air - no collision
		return []physics.AABB{}
	}
	// Solid block - full 1x1x1 collision box
	return []physics.AABB{
		{
			X:       physics.MinMax{Min: float64(x), Max: float64(x) + 1},
			Y:       physics.MinMax{Min: float64(y), Max: float64(y) + 1},
			Z:       physics.MinMax{Min: float64(z), Max: float64(z) + 1},
			BlockID: blockStateID,
		},
	}
}

func (msp *MockShapeProvider) IsPassable(blockStateID uint32) bool {
	return blockStateID == 0 // Only air (0) is passable
}

func (msp *MockShapeProvider) IsSolid(blockStateID uint32) bool {
	return blockStateID != 0
}

func (msp *MockShapeProvider) GetStandingSurfaceHeight(blockStateID uint32) float64 {
	if blockStateID == 0 {
		return 0
	}
	return 1.0
}

func (msp *MockShapeProvider) IsClimbable(blockStateID uint32) bool {
	return false // No climbable blocks in mock
}

func (msp *MockShapeProvider) IsFluid(blockStateID uint32) bool {
	return false // No fluid blocks in mock
}

func (msp *MockShapeProvider) IsWater(blockStateID uint32) bool {
	return false // No water blocks in mock
}

func (msp *MockShapeProvider) IsLava(blockStateID uint32) bool {
	return false // No lava blocks in mock
}

func (msp *MockShapeProvider) IsDangerous(blockStateID uint32) bool {
	return false // No dangerous blocks in mock
}

func (msp *MockShapeProvider) IsDoorLike(blockStateID uint32) bool {
	return false // No door-like blocks in mock
}

func (msp *MockShapeProvider) IsFenceLike(blockStateID uint32) bool {
	return false // No fence-like blocks in mock
}

func (msp *MockShapeProvider) IsSlab(blockStateID uint32) bool {
	return false // No slabs in mock
}

func (msp *MockShapeProvider) IsStair(blockStateID uint32) bool {
	return false // No stairs in mock
}

func (msp *MockShapeProvider) IsLogOrLeaf(blockStateID uint32) bool {
	return false // No logs or leaves in mock
}

func (msp *MockShapeProvider) IsHayBale(blockStateID uint32) bool {
	return false // No hay bales in mock
}

func (msp *MockShapeProvider) IsBed(blockStateID uint32) bool {
	return false // No beds in mock
}

func (msp *MockShapeProvider) IsHoneyBlock(blockStateID uint32) bool {
	return false // No honey blocks in mock
}

func (msp *MockShapeProvider) IsSlimeBlock(blockStateID uint32) bool {
	return false // No slime blocks in mock
}

func (msp *MockShapeProvider) IsPowderSnow(blockStateID uint32) bool {
	return false // No powder snow in mock
}

func (msp *MockShapeProvider) BlockName(blockStateID uint32) string {
	if blockStateID == 0 {
		return "minecraft:air"
	}
	return "minecraft:stone"
}

func (msp *MockShapeProvider) FullBlockName(blockStateID uint32) string {
	return msp.BlockName(blockStateID)
}

// Test helper: Create physics executor for testing
func createTestPhysicsExecutor() *PhysicsMovementExecutor {
	// Mock position tracking
	currentX, currentY, currentZ := 0.0, 64.0, 0.0
	var currentYaw, currentPitch float32 = 0.0, 0.0
	var entityID int32 = 1

	getBotPos := func() (float64, float64, float64, float32, float32, bool) {
		return currentX, currentY, currentZ, currentYaw, currentPitch, true
	}

	setBotPos := func(x, y, z float64, yaw, pitch float32) {
		currentX, currentY, currentZ = x, y, z
		currentYaw, currentPitch = yaw, pitch
	}

	getBotEntityID := func() int32 {
		return entityID
	}

	world := NewMockWorld()
	shapeProvider := &MockShapeProvider{}

	// Build a solid world floor at Y=63 (bot starts at Y=64, standing on Y=63 ground)
	// Also add blocks below to prevent falling through
	for x := -20; x <= 20; x++ {
		for z := -20; z <= 20; z++ {
			for y := 0; y <= 63; y++ {
				world.SetBlock(x, y, z, 1) // Stone
			}
		}
	}

	return NewPhysicsMovementExecutor(
		context.Background(),
		nil, // client (nil for test mode)
		nil, // packetMgr (nil for test mode)
		getBotPos,
		setBotPos,
		getBotEntityID,
		world,
		shapeProvider,
	)
}

// TestPhysicsExecutor_Creation tests that executor is created correctly
func TestPhysicsExecutor_Creation(t *testing.T) {
	exec := createTestPhysicsExecutor()

	if exec == nil {
		t.Fatal("Failed to create physics executor")
	}

	if exec.physicsState == nil {
		t.Error("Physics state not initialized")
	}

	if exec.inputGen == nil {
		t.Error("Input generator not initialized")
	}

	if exec.tickRate != 50*time.Millisecond {
		t.Errorf("Unexpected tick rate: %v (expected 50ms)", exec.tickRate)
	}
}

// TestPhysicsExecutor_InitialPosition tests that initial position is set correctly
func TestPhysicsExecutor_InitialPosition(t *testing.T) {
	exec := createTestPhysicsExecutor()

	pos, yaw, pitch, _ := exec.physicsState.GetPosition()

	if pos.X != 0 || pos.Y != 64 || pos.Z != 0 {
		t.Errorf("Initial position incorrect: (%.1f, %.1f, %.1f), expected (0, 64, 0)",
			pos.X, pos.Y, pos.Z)
	}

	if yaw != 0 || pitch != 0 {
		t.Errorf("Initial rotation incorrect: yaw=%.1f pitch=%.1f, expected (0, 0)", yaw, pitch)
	}
}

// TestPhysicsExecutor_SyncWithServer tests server position correction
func TestPhysicsExecutor_SyncWithServer(t *testing.T) {
	exec := createTestPhysicsExecutor()

	// Initial position
	pos, _, _, _ := exec.physicsState.GetPosition()
	if pos.X != 0 {
		t.Errorf("Initial X should be 0, got %.1f", pos.X)
	}

	// Simulate server correction
	exec.SyncWithServer(5.0, 64.0, 5.0, 90.0, 0.0, true)

	// Verify position updated
	pos, yaw, pitch, onGround := exec.physicsState.GetPosition()
	if pos.X != 5.0 || pos.Y != 64.0 || pos.Z != 5.0 {
		t.Errorf("Position after sync incorrect: (%.1f, %.1f, %.1f), expected (5, 64, 5)",
			pos.X, pos.Y, pos.Z)
	}

	if yaw != 90.0 {
		t.Errorf("Yaw after sync incorrect: %.1f, expected 90", yaw)
	}

	if pitch != 0.0 {
		t.Errorf("Pitch after sync incorrect: %.1f, expected 0", pitch)
	}

	if !onGround {
		t.Error("OnGround should be true after sync")
	}

	// Verify velocity was reset
	vel := exec.physicsState.GetVelocity()
	if vel.X != 0 || vel.Y != 0 || vel.Z != 0 {
		t.Errorf("Velocity should be reset after sync, got (%.3f, %.3f, %.3f)",
			vel.X, vel.Y, vel.Z)
	}
}

// TestPhysicsExecutor_PredictionErrorTracking tests prediction error monitoring
func TestPhysicsExecutor_PredictionErrorTracking(t *testing.T) {
	exec := createTestPhysicsExecutor()

	// Initially no errors
	avgError := exec.GetAveragePredictionError()
	if avgError != 0 {
		t.Errorf("Initial average error should be 0, got %.6f", avgError)
	}

	// Simulate a small correction (0.1 blocks in each axis)
	exec.SyncWithServer(0.1, 64.1, 0.1, 0, 0, true)

	// Error = 0.1² + 0.1² + 0.1² = 0.03
	avgError = exec.GetAveragePredictionError()
	expectedError := 0.1*0.1 + 0.1*0.1 + 0.1*0.1
	if avgError < expectedError-0.001 || avgError > expectedError+0.001 {
		t.Errorf("Average error incorrect: %.6f, expected %.6f", avgError, expectedError)
	}

	// Simulate multiple corrections
	for i := 0; i < 10; i++ {
		exec.SyncWithServer(
			float64(i)*0.1,
			64.0,
			float64(i)*0.1,
			0, 0, true,
		)
	}

	// Should have 11 errors tracked (1 initial + 10 in loop)
	if len(exec.predictionErrors) != 11 {
		t.Errorf("Should have 11 errors tracked, got %d", len(exec.predictionErrors))
	}
}

// TestPhysicsExecutor_ExecutePath_SimpleTraverse tests path execution
func TestPhysicsExecutor_ExecutePath_SimpleTraverse(t *testing.T) {
	exec := createTestPhysicsExecutor()

	// Reduce tick rate for faster testing
	exec.tickRate = 1 * time.Millisecond

	// Start continuous mode
	exec.Start()
	defer exec.Stop()

	// Create a simple 3-step path
	path := &pathfinding.Path{
		Steps: []pathfinding.PathStep{
			{Position: models.V3{X: 1, Y: 64, Z: 0}, Movement: pathfinding.Traverse, Cost: 1.0},
			{Position: models.V3{X: 2, Y: 64, Z: 0}, Movement: pathfinding.Traverse, Cost: 1.0},
			{Position: models.V3{X: 3, Y: 64, Z: 0}, Movement: pathfinding.Traverse, Cost: 1.0},
		},
		TotalCost: 3.0,
		StartPos:  models.V3{X: 0, Y: 64, Z: 0},
		GoalPos:   models.V3{X: 3, Y: 64, Z: 0},
		Found:     true,
	}

	// Execute path
	err := exec.ExecutePath(path)
	if err != nil {
		t.Fatalf("ExecutePath failed: %v", err)
	}

	// Check final position
	pos, _, _, _ := exec.physicsState.GetPosition()

	// Should be close to (3, 64, 0)
	deltaX := pos.X - 3.0
	deltaY := pos.Y - 64.0
	deltaZ := pos.Z - 0.0

	if deltaX*deltaX > 0.18*0.18 {
		t.Errorf("Final X position too far from target: %.2f (expected ~3.0)", pos.X)
	}

	if deltaY < -0.065 || deltaY > 0.08 {
		t.Errorf("Final Y position out of range: %.2f (expected ~64.0)", pos.Y)
	}

	if deltaZ*deltaZ > 0.18*0.18 {
		t.Errorf("Final Z position too far from target: %.2f (expected ~0.0)", pos.Z)
	}
}

// TestPhysicsExecutor_ExecutePath_NotFound tests handling of failed paths
func TestPhysicsExecutor_ExecutePath_NotFound(t *testing.T) {
	exec := createTestPhysicsExecutor()

	// Start continuous mode
	exec.Start()
	defer exec.Stop()

	// Create a path that wasn't found
	path := &pathfinding.Path{
		Steps:     []pathfinding.PathStep{},
		TotalCost: 0,
		StartPos:  models.V3{X: 0, Y: 64, Z: 0},
		GoalPos:   models.V3{X: 100, Y: 64, Z: 100},
		Found:     false,
	}

	// Execute path should fail
	err := exec.ExecutePath(path)
	if err == nil {
		t.Error("ExecutePath should fail for path not found")
	}
}

// TestPhysicsExecutor_Sprint tests sprint state management
func TestPhysicsExecutor_Sprint(t *testing.T) {
	exec := createTestPhysicsExecutor()

	// Initially not sprinting
	if exec.IsSprinting() {
		t.Error("Should not be sprinting initially")
	}

	// Note: We can't actually test StartSprinting/StopSprinting with nil client
	// because it will panic trying to send packets. The state management
	// is tested through baseExecutor tests in executor_test.go
	// Just verify the IsSprinting method works
}

// TestPhysicsExecutor_Sneak tests sneak state management
func TestPhysicsExecutor_Sneak(t *testing.T) {
	exec := createTestPhysicsExecutor()

	// Initially not sneaking
	if exec.IsSneaking() {
		t.Error("Should not be sneaking initially")
	}

	// Note: We can't actually test StartSneaking/StopSneaking with nil client
	// because it will panic trying to send packets. The state management
	// is tested through baseExecutor tests in executor_test.go
	// Just verify the IsSneaking method works
}

// TestPhysicsExecutor_GetCurrentPosition tests position query
func TestPhysicsExecutor_GetCurrentPosition(t *testing.T) {
	exec := createTestPhysicsExecutor()

	x, y, z := exec.GetCurrentPosition()

	if x != 0 || y != 64 || z != 0 {
		t.Errorf("GetCurrentPosition returned (%.1f, %.1f, %.1f), expected (0, 64, 0)", x, y, z)
	}

	// Move to new position
	exec.SyncWithServer(10, 65, 20, 0, 0, true)

	x, y, z = exec.GetCurrentPosition()
	if x != 10 || y != 65 || z != 20 {
		t.Errorf("GetCurrentPosition returned (%.1f, %.1f, %.1f), expected (10, 65, 20)", x, y, z)
	}
}

// TestPhysicsExecutor_MoveTowards_NotSupported tests that MoveTowards is not supported
func TestPhysicsExecutor_MoveTowards_NotSupported(t *testing.T) {
	exec := createTestPhysicsExecutor()

	_, _, _, err := exec.MoveTowards(1, 64, 0, 0.1, true)
	if err == nil {
		t.Error("MoveTowards should return error (not supported by physics executor)")
	}
}

// BenchmarkPhysicsExecutor_SyncWithServer benchmarks server correction handling
func BenchmarkPhysicsExecutor_SyncWithServer(b *testing.B) {
	exec := createTestPhysicsExecutor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		exec.SyncWithServer(
			float64(i%10),
			64.0,
			float64(i%10),
			0, 0, true,
		)
	}
}
