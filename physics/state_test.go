package physics

import (
	"math"
	"testing"
)

// Mock implementations for testing

// mockWorld is a simple grid-based world for testing
type mockWorld struct {
	blocks map[[3]int]uint32
}

func newMockWorld() *mockWorld {
	return &mockWorld{
		blocks: make(map[[3]int]uint32),
	}
}

func (m *mockWorld) SetBlock(x, y, z int, blockID uint32) {
	m.blocks[[3]int{x, y, z}] = blockID
}

func (m *mockWorld) GetBlockStatus(x, y, z int) (uint32, bool) {
	val, exists := m.blocks[[3]int{x, y, z}]
	return val, exists // m.blocks[[3]int{x, y, z}]
}

// mockShapeProvider provides simple block collision data
type mockShapeProvider struct {
	passableBlocks   map[uint32]bool
	climbableBlocks  map[uint32]bool
	hayBaleBlocks    map[uint32]bool
	bedBlocks        map[uint32]bool
	honeyBlocks      map[uint32]bool
	slimeBlocks      map[uint32]bool
	powderSnowBlocks map[uint32]bool
}

func newMockShapeProvider() *mockShapeProvider {
	return &mockShapeProvider{
		passableBlocks:   make(map[uint32]bool),
		climbableBlocks:  make(map[uint32]bool),
		hayBaleBlocks:    make(map[uint32]bool),
		bedBlocks:        make(map[uint32]bool),
		honeyBlocks:      make(map[uint32]bool),
		slimeBlocks:      make(map[uint32]bool),
		powderSnowBlocks: make(map[uint32]bool),
	}
}

func (m *mockShapeProvider) SetPassable(blockID uint32, passable bool) {
	m.passableBlocks[blockID] = passable
}

func (m *mockShapeProvider) SetClimbable(blockID uint32, climbable bool) {
	m.climbableBlocks[blockID] = climbable
}

func (m *mockShapeProvider) SetHayBale(blockID uint32, isHayBale bool) {
	m.hayBaleBlocks[blockID] = isHayBale
}

func (m *mockShapeProvider) SetBed(blockID uint32, isBed bool) {
	m.bedBlocks[blockID] = isBed
}

func (m *mockShapeProvider) SetHoneyBlock(blockID uint32, isHoney bool) {
	m.honeyBlocks[blockID] = isHoney
}

func (m *mockShapeProvider) SetSlimeBlock(blockID uint32, isSlime bool) {
	m.slimeBlocks[blockID] = isSlime
}

func (m *mockShapeProvider) SetPowderSnow(blockID uint32, isPowderSnow bool) {
	m.powderSnowBlocks[blockID] = isPowderSnow
}

func (m *mockShapeProvider) IsPassable(blockID uint32) bool {
	// Block ID 0 (air) is always passable
	if blockID == 0 {
		return true
	}
	return m.passableBlocks[blockID]
}

func (m *mockShapeProvider) IsSolid(blockID uint32) bool {
	return !m.IsPassable(blockID)
}

func (m *mockShapeProvider) GetStandingSurfaceHeight(blockID uint32) float64 {
	if blockID == BlockHalfSlab {
		return 0.5
	}
	if m.IsSolid(blockID) {
		return 1.0
	}
	return 0.0
}

func (m *mockShapeProvider) IsClimbable(blockID uint32) bool {
	return m.climbableBlocks[blockID]
}

func (m *mockShapeProvider) IsFluid(blockID uint32) bool {
	return blockID == BlockWater
}

func (m *mockShapeProvider) IsWater(blockID uint32) bool {
	// Block ID 2 is water in our test setup
	return blockID == BlockWater
}

func (m *mockShapeProvider) IsLava(blockID uint32) bool {
	return false
}

func (m *mockShapeProvider) IsDangerous(blockID uint32) bool {
	return false
}

func (m *mockShapeProvider) IsDoorLike(blockID uint32) bool {
	return false
}

func (m *mockShapeProvider) IsFenceLike(blockID uint32) bool {
	return false
}

func (m *mockShapeProvider) IsSlab(blockID uint32) bool {
	return blockID == BlockHalfSlab
}

func (m *mockShapeProvider) IsStair(blockID uint32) bool {
	return false
}

func (m *mockShapeProvider) IsLogOrLeaf(blockID uint32) bool {
	return false
}

func (m *mockShapeProvider) IsHayBale(blockID uint32) bool {
	return m.hayBaleBlocks[blockID]
}

func (m *mockShapeProvider) IsBed(blockID uint32) bool {
	return m.bedBlocks[blockID]
}

func (m *mockShapeProvider) IsHoneyBlock(blockID uint32) bool {
	return m.honeyBlocks[blockID]
}

func (m *mockShapeProvider) IsSlimeBlock(blockID uint32) bool {
	return m.slimeBlocks[blockID]
}

func (m *mockShapeProvider) IsPowderSnow(blockID uint32) bool {
	return m.powderSnowBlocks[blockID]
}

func (m *mockShapeProvider) GetCollisionBoxes(blockStateID uint32, x, y, z int) []AABB {
	// Air (0) has no collision
	if blockStateID == 0 {
		return nil
	}

	// Passable blocks have no collision
	if m.IsPassable(blockStateID) {
		return nil
	}

	// Full block (default)
	return []AABB{
		NewAABB(
			float64(x), float64(y), float64(z),
			float64(x)+1.0, float64(y)+1.0, float64(z)+1.0,
		),
	}
}

// mockShapeProviderWithSlabs extends mockShapeProvider to handle half-slabs
type mockShapeProviderWithSlabs struct {
	mockShapeProvider
}

func (m *mockShapeProviderWithSlabs) GetCollisionBoxes(blockStateID uint32, x, y, z int) []AABB {
	// Half slab (0.5 blocks high)
	if blockStateID == BlockHalfSlab {
		return []AABB{
			NewAABB(
				float64(x), float64(y), float64(z),
				float64(x)+1.0, float64(y)+0.5, float64(z)+1.0,
			),
		}
	}

	// Delegate to base implementation
	return m.mockShapeProvider.GetCollisionBoxes(blockStateID, x, y, z)
}

// Test block IDs
const (
	BlockAir      uint32 = 0
	BlockStone    uint32 = 1
	BlockWater    uint32 = 2
	BlockLadder   uint32 = 3
	BlockHalfSlab uint32 = 4 // Custom for testing
)

// Test helper: create a simple flat world with a floor at Y=0
func createFlatWorld() (*mockWorld, *mockShapeProvider) {
	world := newMockWorld()
	shapes := newMockShapeProvider()

	// Create a large 40x40 platform at Y=0 (prevent falling off edges in tests)
	for x := -20; x <= 20; x++ {
		for z := -20; z <= 20; z++ {
			world.SetBlock(x, 0, z, BlockStone)
		}
	}

	// Configure block properties
	shapes.SetPassable(BlockAir, true)
	shapes.SetPassable(BlockWater, true)
	shapes.SetPassable(BlockStone, false)
	shapes.SetPassable(BlockLadder, true) // Ladders are passable but climbable
	shapes.SetClimbable(BlockLadder, true)

	return world, shapes
}

// Tests

func TestNewState(t *testing.T) {
	shapes := newMockShapeProvider()
	state := NewState(shapes)

	if state == nil {
		t.Fatal("NewState returned nil")
	}

	width, height, eyeHeight := state.GetDimensions()
	if width != PlayerWidth {
		t.Errorf("Expected width=%.2f, got %.2f", PlayerWidth, width)
	}
	if height != PlayerHeight {
		t.Errorf("Expected height=%.2f, got %.2f", PlayerHeight, height)
	}
	if eyeHeight != PlayerEyeHeight {
		t.Errorf("Expected eyeHeight=%.2f, got %.2f", PlayerEyeHeight, eyeHeight)
	}
}

func TestState_SetPosition(t *testing.T) {
	shapes := newMockShapeProvider()
	state := NewState(shapes)

	pos := V3{X: 10.5, Y: 64.0, Z: 20.3}
	yaw, pitch := 45.0, -30.0

	state.SetPosition(pos, yaw, pitch, true)

	gotPos, gotYaw, gotPitch, gotOnGround := state.GetPosition()

	if gotPos != pos {
		t.Errorf("Position mismatch: expected %v, got %v", pos, gotPos)
	}
	if gotYaw != yaw {
		t.Errorf("Yaw mismatch: expected %.2f, got %.2f", yaw, gotYaw)
	}
	if gotPitch != pitch {
		t.Errorf("Pitch mismatch: expected %.2f, got %.2f", pitch, gotPitch)
	}
	if gotOnGround != true {
		t.Errorf("OnGround mismatch: expected true, got false")
	}

	// Velocity should be reset
	if state.Velocity().X != 0 || state.Velocity().Y != 0 || state.Velocity().Z != 0 {
		t.Errorf("Velocity not reset: %v", state.Velocity())
	}
}

func TestState_GetAABB(t *testing.T) {
	shapes := newMockShapeProvider()
	state := NewState(shapes)
	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})

	bb := state.GetAABB()

	// Player AABB should be centered on X/Z, start at Y
	expectedMinX := -PlayerWidth / 2
	expectedMaxX := PlayerWidth / 2
	expectedMinY := 1.0
	expectedMaxY := 1.0 + PlayerHeight
	expectedMinZ := -PlayerWidth / 2
	expectedMaxZ := PlayerWidth / 2

	if bb.X.Min != expectedMinX || bb.X.Max != expectedMaxX {
		t.Errorf("X bounds wrong: expected [%.2f, %.2f], got [%.2f, %.2f]",
			expectedMinX, expectedMaxX, bb.X.Min, bb.X.Max)
	}
	if bb.Y.Min != expectedMinY || bb.Y.Max != expectedMaxY {
		t.Errorf("Y bounds wrong: expected [%.2f, %.2f], got [%.2f, %.2f]",
			expectedMinY, expectedMaxY, bb.Y.Min, bb.Y.Max)
	}
	if bb.Z.Min != expectedMinZ || bb.Z.Max != expectedMaxZ {
		t.Errorf("Z bounds wrong: expected [%.2f, %.2f], got [%.2f, %.2f]",
			expectedMinZ, expectedMaxZ, bb.Z.Min, bb.Z.Max)
	}
}

func TestState_Freefall(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Place player in the air
	state.SetPositionSimple(V3{X: 0, Y: 10, Z: 0})
	state.SetVelocity(V3{})

	// Simulate 20 ticks of freefall
	for i := 0; i < 20; i++ {
		state.Tick(Inputs{}, world)
	}

	// Player should have fallen (Y decreased)
	if state.Position().Y >= 10 {
		t.Errorf("Player did not fall: Y=%.2f (expected < 10)", state.Position().Y)
	}

	// Velocity should be negative (falling)
	if state.Velocity().Y >= 0 {
		t.Errorf("Player velocity not negative: velY=%.3f", state.Velocity().Y)
	}

	// Eventually should land on ground at Y=1 (on top of Y=0 block)
	for i := 0; i < 100 && !state.OnGround(); i++ {
		state.Tick(Inputs{}, world)
	}

	if !state.OnGround() {
		t.Errorf("Player never landed on ground after 100 ticks: Y=%.2f", state.Position().Y)
	}

	// Should be standing at Y=1 (on top of Y=0 stone block)
	if math.Abs(state.Position().Y-1.0) > 0.01 {
		t.Errorf("Player not at correct height: Y=%.3f (expected 1.0)", state.Position().Y)
	}
}

func TestState_HorizontalMovement(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Place player well above ground and let physics settle them
	state.SetPositionSimple(V3{X: 0, Y: 5, Z: 0})
	state.SetVelocity(V3{})

	// Let player fall and settle on ground
	groundTicks := 0
	for i := 0; i < 200; i++ {
		state.Tick(Inputs{}, world)
		if state.OnGround() {
			groundTicks++
			if groundTicks >= 5 {
				// Been on ground for 5 ticks, considered settled
				break
			}
		} else {
			groundTicks = 0
		}
	}

	if !state.OnGround() || groundTicks < 5 {
		t.Fatalf("Player never settled on ground: Y=%.3f, onGround=%v, groundTicks=%d", state.Position().Y, state.OnGround(), groundTicks)
	}

	// Apply forward throttle (positive Z)
	input := Inputs{
		ThrottleZ: 1.0,
	}

	startZ := state.Position().Z
	startY := state.Position().Y

	// Simulate 50 ticks of movement
	for i := 0; i < 50; i++ {
		state.Tick(input, world)
	}

	// Player should have moved forward
	if state.Position().Z <= startZ {
		t.Errorf("Player did not move forward: Z=%.3f (started at %.3f)", state.Position().Z, startZ)
	}

	// Should still be on ground
	if !state.OnGround() {
		t.Errorf("Player not on ground after horizontal movement: Y=%.3f", state.Position().Y)
	}

	// Should be at approximately same Y (allow small variation)
	if math.Abs(state.Position().Y-startY) > 0.2 {
		t.Errorf("Player height changed significantly during horizontal movement: Y=%.3f (started at %.3f)", state.Position().Y, startY)
	}
}

func TestState_Jump(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Place player on ground and let them settle
	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetVelocity(V3{})

	// Let player settle on ground first
	for i := 0; i < 20; i++ {
		state.Tick(Inputs{}, world)
		if state.OnGround() && math.Abs(state.Velocity().Y) < 0.01 {
			break
		}
	}

	if !state.OnGround() {
		t.Fatalf("Player never settled on ground before jump test")
	}

	// Apply jump input
	input := Inputs{
		Jump: true,
	}

	// First tick: should jump
	state.Tick(input, world)

	// Velocity should be positive (jumping up)
	if state.Velocity().Y <= 0 {
		t.Errorf("Player did not jump: velY=%.3f, onGround=%v", state.Velocity().Y, state.OnGround())
	}

	// Should not be on ground anymore (after a few ticks in the air)
	state.Tick(Inputs{}, world)
	if state.OnGround() {
		t.Errorf("Player still on ground after jump")
	}

	// Try to jump again immediately (should fail due to cooldown)
	prevVelY := state.Velocity().Y
	state.Tick(input, world)

	// Velocity should have decreased due to gravity (not jumped again)
	if state.Velocity().Y >= prevVelY {
		t.Errorf("Player jumped again before cooldown expired")
	}
}

func TestState_JumpCooldown(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Place player on ground and let them settle
	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetVelocity(V3{})

	// Let player settle on ground first
	for i := 0; i < 20; i++ {
		state.Tick(Inputs{}, world)
		if state.OnGround() && math.Abs(state.Velocity().Y) < 0.01 {
			break
		}
	}

	if !state.OnGround() {
		t.Fatalf("Player never settled on ground")
	}

	// Jump once
	state.Tick(Inputs{Jump: true}, world)
	if state.Velocity().Y <= 0 {
		t.Fatalf("First jump failed: velY=%.3f", state.Velocity().Y)
	}

	// Land back on ground
	for i := 0; i < 100; i++ {
		state.Tick(Inputs{}, world)
		if state.OnGround() {
			break
		}
	}

	if !state.OnGround() {
		t.Fatal("Player never landed after first jump")
	}

	// Try to jump again immediately (should fail - cooldown not expired)
	state.Tick(Inputs{Jump: true}, world)
	if state.Velocity().Y > 0 {
		t.Error("Player jumped before MinJumpTicks cooldown")
	}

	// Wait for cooldown
	for i := uint32(0); i < MinJumpTicks+5; i++ {
		state.Tick(Inputs{}, world)
	}

	// Now jump should work
	state.Tick(Inputs{Jump: true}, world)
	if state.Velocity().Y <= 0 {
		t.Error("Player did not jump after cooldown expired")
	}
}

func TestState_StepUp_Success(t *testing.T) {
	world, shapes := createFlatWorld()

	// Create a custom shape provider that returns half-height boxes for slabs
	customShapes := &mockShapeProviderWithSlabs{
		mockShapeProvider: *shapes,
	}
	customShapes.SetPassable(BlockHalfSlab, false)

	state := NewState(customShapes)

	// Place half slab in front of player
	world.SetBlock(1, 1, 0, BlockHalfSlab)

	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetVelocity(V3{})
	state.SetOnGround(true)

	// Move forward into the half slab
	input := Inputs{
		ThrottleX: 1.0, // Move in +X direction
	}

	startX := state.Position().X

	// Simulate movement
	for i := 0; i < 50 && state.Position().X < 1.5; i++ {
		state.Tick(input, world)
	}

	// Player should have stepped up and moved forward
	if state.Position().X <= startX {
		t.Errorf("Player did not move forward over half slab: X=%.3f", state.Position().X)
	}

	// Should have stepped up (Y increased)
	if state.Position().Y < 1.4 {
		t.Errorf("Player did not step up onto half slab: Y=%.3f", state.Position().Y)
	}
}

func TestState_StepUp_TooHigh(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Place a full-height block as obstacle
	world.SetBlock(1, 1, 0, BlockStone)

	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetVelocity(V3{})

	// Let player settle on ground first
	for i := 0; i < 20; i++ {
		state.Tick(Inputs{}, world)
		if state.OnGround() && math.Abs(state.Velocity().Y) < 0.01 {
			break
		}
	}

	// Try to walk into the wall
	input := Inputs{
		ThrottleX: 1.0,
	}

	// Simulate movement
	for i := 0; i < 30; i++ {
		state.Tick(input, world)
	}

	// Player should not have moved much (blocked by wall)
	// Player can move until their edge touches the wall
	// Wall is at X=1, player starts at X=0, player width is 0.6
	// Player can move to X=0.7 (edge at X=1.0) before hitting wall
	maxX := 1.0 - PlayerWidth/2 + 0.05 // Wall position minus half-width plus small tolerance
	if state.Position().X > maxX {
		t.Errorf("Player walked through wall: X=%.3f (max=%.3f)", state.Position().X, maxX)
	}
}

func TestState_LadderClimbing(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Place a ladder column
	world.SetBlock(0, 1, 0, BlockLadder)
	world.SetBlock(0, 2, 0, BlockLadder)
	world.SetBlock(0, 3, 0, BlockLadder)
	world.SetBlock(0, 4, 0, BlockLadder)

	// Place blocks behind the ladder (ladders need a wall to climb on)
	world.SetBlock(0, 1, 1, BlockStone)
	world.SetBlock(0, 2, 1, BlockStone)
	world.SetBlock(0, 3, 1, BlockStone)
	world.SetBlock(0, 4, 1, BlockStone)

	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetVelocity(V3{})

	// Let player settle on ground first
	for i := 0; i < 20; i++ {
		state.Tick(Inputs{}, world)
		if state.OnGround() && math.Abs(state.Velocity().Y) < 0.01 {
			break
		}
	}

	startY := state.Position().Y

	// Climb the ladder using ClimbDirection input
	// ClimbDirection +1.0 = climb up, -1.0 = descend
	input := Inputs{
		ClimbDirection: 1.0, // Climb up
	}

	// Simulate climbing
	for i := 0; i < 50; i++ {
		state.Tick(input, world)
	}

	// Player should have climbed (Y increased)
	if state.Position().Y <= startY+0.5 {
		t.Errorf("Player did not climb ladder significantly: Y=%.3f (started at %.3f)", state.Position().Y, startY)
	}
}

func TestState_LadderSneakingPreventsDescend(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Place a ladder column
	world.SetBlock(0, 1, 0, BlockLadder)
	world.SetBlock(0, 2, 0, BlockLadder)
	world.SetBlock(0, 3, 0, BlockLadder)
	world.SetBlock(0, 4, 0, BlockLadder)

	// Place blocks behind the ladder
	world.SetBlock(0, 1, 1, BlockStone)
	world.SetBlock(0, 2, 1, BlockStone)
	world.SetBlock(0, 3, 1, BlockStone)
	world.SetBlock(0, 4, 1, BlockStone)

	// Start player at Y=3 on the ladder (mid-height)
	state.SetPositionSimple(V3{X: 0.5, Y: 3.0, Z: 0.5})
	state.SetVelocity(V3{})

	startY := state.Position().Y

	// Sneak on ladder without any movement input
	// In vanilla Minecraft, sneaking on a ladder should prevent descent
	input := Inputs{
		Sneak: true,
	}

	// Simulate 50 ticks of sneaking on ladder
	for i := 0; i < 50; i++ {
		state.Tick(input, world)
	}

	// Player should NOT have descended (Y should be >= startY or very close)
	// Allow small tolerance for floating point
	if state.Position().Y < startY-0.1 {
		t.Errorf("Player descended while sneaking on ladder: Y=%.3f (started at %.3f)", state.Position().Y, startY)
	}
}

func TestState_LadderDescendWithoutSneak(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Place a ladder column
	world.SetBlock(0, 1, 0, BlockLadder)
	world.SetBlock(0, 2, 0, BlockLadder)
	world.SetBlock(0, 3, 0, BlockLadder)
	world.SetBlock(0, 4, 0, BlockLadder)

	// Place blocks behind the ladder
	world.SetBlock(0, 1, 1, BlockStone)
	world.SetBlock(0, 2, 1, BlockStone)
	world.SetBlock(0, 3, 1, BlockStone)
	world.SetBlock(0, 4, 1, BlockStone)

	// Start player at Y=3 on the ladder (mid-height)
	state.SetPositionSimple(V3{X: 0.5, Y: 3.0, Z: 0.5})
	state.SetVelocity(V3{})

	startY := state.Position().Y

	// No sneak - should allow descent via gravity
	input := Inputs{
		Sneak: false,
	}

	// Simulate 50 ticks without sneaking
	for i := 0; i < 50; i++ {
		state.Tick(input, world)
	}

	// Player SHOULD have descended (gravity applies when not sneaking)
	// On a ladder, descent is clamped to LadderMaxSpeed (0.15/tick)
	if state.Position().Y >= startY-0.5 {
		t.Errorf("Player did not descend on ladder without sneaking: Y=%.3f (started at %.3f)", state.Position().Y, startY)
	}
}

func TestState_VelocityDeadzone(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	// Set very small velocities (below deadzone threshold)
	state.SetPositionSimple(V3{X: 0, Y: 10, Z: 0})
	state.SetVelocity(V3{
		X: ResetVelocity / 2,
		Y: ResetVelocity / 2,
		Z: ResetVelocity / 2,
	})

	state.Tick(Inputs{}, world)

	// All velocities should be reset to zero
	if state.Velocity().X != 0 {
		t.Errorf("VelX not reset: %.6f", state.Velocity().X)
	}
	if state.Velocity().Z != 0 {
		t.Errorf("VelZ not reset: %.6f", state.Velocity().Z)
	}
	// Note: VelY might not be zero due to gravity
}

func TestState_CollisionDetection(t *testing.T) {
	world, shapes := createFlatWorld()

	// Build a box around the player (walls must not overlap player's starting position)
	// Player at (0,1,0) has AABB from (-0.3,1,-0.3) to (0.3,2.8,0.3)
	world.SetBlock(1, 1, 0, BlockStone)  // Right wall (X: 1-2, no overlap)
	world.SetBlock(-2, 1, 0, BlockStone) // Left wall (X: -2 to -1, no overlap)
	world.SetBlock(0, 1, 1, BlockStone)  // Front wall (Z: 1-2, no overlap)
	world.SetBlock(0, 1, -2, BlockStone) // Back wall (Z: -2 to -1, no overlap)

	// Try to move in each direction
	directions := []struct {
		name     string
		throttle Inputs
	}{
		{"right", Inputs{ThrottleX: 1.0}},
		{"left", Inputs{ThrottleX: -1.0}},
		{"forward", Inputs{ThrottleZ: 1.0}},
		{"back", Inputs{ThrottleZ: -1.0}},
	}

	for _, dir := range directions {
		t.Run(dir.name, func(t *testing.T) {
			// Create a fresh state for each subtest
			state := NewState(shapes)

			// Start player on ground at origin (same as TestState_StepUp_TooHigh)
			state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
			state.SetVelocity(V3{})

			// Let player settle on ground (same pattern as TestState_StepUp_TooHigh)
			for i := 0; i < 20; i++ {
				state.Tick(Inputs{}, world)
				if state.OnGround() && math.Abs(state.Velocity().Y) < 0.01 {
					break
				}
			}

			if !state.OnGround() {
				t.Fatalf("Player never settled before collision test")
			}

			startPos := state.Position()

			// Try to move (use 30 ticks like TestState_StepUp_TooHigh)
			for i := 0; i < 30; i++ {
				state.Tick(dir.throttle, world)
			}

			// Calculate horizontal distance moved (ignore Y changes)
			deltaX := state.Position().X - startPos.X
			deltaZ := state.Position().Z - startPos.Z
			horizDist := math.Sqrt(deltaX*deltaX + deltaZ*deltaZ)

			// Should not have moved much horizontally (blocked by walls)
			// Walls are at ±1, player starts at 0
			// Player can move to position where edge touches wall: 1.0 - PlayerWidth/2
			maxDist := 1.0 - PlayerWidth/2 + 0.05 // Same tolerance as TestState_StepUp_TooHigh
			if horizDist > maxDist {
				t.Errorf("Player moved through wall: horizontal distance=%.3f (max=%.3f)", horizDist, maxDist)
			}
		})
	}
}

func TestState_AtLookTarget(t *testing.T) {
	shapes := newMockShapeProvider()
	state := NewState(shapes)

	state.SetYaw(90.0)
	state.SetPitch(0.0)

	tests := []struct {
		name        string
		targetYaw   float64
		targetPitch float64
		expected    bool
	}{
		{"exact match", 90.0, 0.0, true},
		{"within tolerance", 90.5, 0.5, true},
		{"yaw out of range", 92.0, 0.0, false},
		{"pitch out of range", 90.0, 2.0, false},
		{"both out of range", 92.0, 2.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := state.AtLookTarget(tt.targetYaw, tt.targetPitch)
			if result != tt.expected {
				t.Errorf("AtLookTarget(%0.1f, %0.1f) = %v, expected %v",
					tt.targetYaw, tt.targetPitch, result, tt.expected)
			}
		})
	}
}

func TestState_LookRateLimiting(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetYaw(0.0)
	state.SetPitch(0.0)

	// Try to turn 180 degrees instantly (should be rate-limited)
	input := Inputs{
		Yaw:   180.0,
		Pitch: 45.0,
	}

	state.Tick(input, world)

	// Should not have turned the full amount
	if state.Yaw() > MaxYawChange+0.1 {
		t.Errorf("Yaw not rate-limited: turned %.2f degrees (max=%.2f)", state.Yaw(), MaxYawChange)
	}

	if math.Abs(state.Pitch()) > MaxPitchChange+0.1 {
		t.Errorf("Pitch not rate-limited: turned %.2f degrees (max=%.2f)", math.Abs(state.Pitch()), MaxPitchChange)
	}
}

func TestState_SprintMultiplier(t *testing.T) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)

	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetVelocity(V3{})
	state.SetOnGround(true)

	// Move without sprint
	normalInput := Inputs{ThrottleZ: 1.0}
	state.Tick(normalInput, world)
	normalVel := math.Abs(state.Velocity().Z)

	// Reset
	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetVelocity(V3{})
	state.SetOnGround(true)

	// Move with sprint
	sprintInput := Inputs{ThrottleZ: 1.0, Sprint: true}
	state.Tick(sprintInput, world)
	sprintVel := math.Abs(state.Velocity().Z)

	// Sprint should be faster
	expectedRatio := SprintMultiplier
	actualRatio := sprintVel / normalVel

	if math.Abs(actualRatio-expectedRatio) > 0.01 {
		t.Errorf("Sprint multiplier incorrect: %.2f (expected %.2f)", actualRatio, expectedRatio)
	}
}

// Benchmarks

func BenchmarkState_Tick(b *testing.B) {
	world, shapes := createFlatWorld()
	state := NewState(shapes)
	state.SetPositionSimple(V3{X: 0, Y: 10, Z: 0})

	input := Inputs{
		ThrottleX: 0.5,
		ThrottleZ: 0.5,
		Jump:      false,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.Tick(input, world)
	}
}

func BenchmarkState_Tick_WithCollisions(b *testing.B) {
	world, shapes := createFlatWorld()

	// Add some obstacles
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			if x != 0 || z != 0 {
				world.SetBlock(x, 1, z, BlockStone)
			}
		}
	}

	state := NewState(shapes)
	state.SetPositionSimple(V3{X: 0, Y: 1, Z: 0})
	state.SetOnGround(true)

	input := Inputs{
		ThrottleX: 0.5,
		ThrottleZ: 0.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.Tick(input, world)
	}
}

func BenchmarkState_GetSurroundingBoxes(b *testing.B) {
	world, shapes := createFlatWorld()

	// Add many blocks
	for x := -5; x <= 5; x++ {
		for y := -1; y <= 3; y++ {
			for z := -5; z <= 5; z++ {
				world.SetBlock(x, y, z, BlockStone)
			}
		}
	}

	state := NewState(shapes)
	queryBB := NewAABB(-1, 0, -1, 1, 2, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = state.GetSurroundingBoxes(queryBB, world)
	}
}
