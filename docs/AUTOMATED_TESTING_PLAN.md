# Automated Testing Framework Plan

## Executive Summary

This document outlines a comprehensive automated testing framework for the mc-agent pathfinding and movement systems. The framework enables testing without manual Minecraft gameplay by constructing synthetic worlds and recording/replaying packet data.

**Status**: Planning
**Priority**: High (enables rapid development and regression testing)
**Dependencies**: None (can start immediately)

---

## Goals

### Primary Goals
1. **Test pathfinding** without connecting to a real Minecraft server
2. **Validate movement logic** with synthetic world data
3. **Create reproducible test scenarios** for debugging
4. **Regression testing** to prevent breaking existing functionality
5. **Performance benchmarking** for pathfinding algorithms

### Success Criteria
- ✅ Can run full test suite in < 5 seconds
- ✅ Tests cover 80%+ of pathfinding code
- ✅ Can reproduce bug scenarios from packet captures
- ✅ CI/CD integration ready (run tests on every commit)
- ✅ Easy to add new test cases (< 10 lines of code)

---

## Architecture Design

### Testing Pyramid

```
                    ┌─────────────────┐
                    │  Manual Tests   │  (Rare - full integration)
                    │  (In-game)      │
                    └─────────────────┘
                           ▲
                    ┌─────────────────┐
                    │ Integration     │  (Moderate - system interaction)
                    │ Tests           │  (Packet replay)
                    └─────────────────┘
                           ▲
                    ┌─────────────────┐
                    │  Unit Tests     │  (Frequent - isolated components)
                    │  (Mocked World) │
                    └─────────────────┘
```

### Component Architecture

```
┌──────────────────────────────────────────────────────────┐
│                  Testing Framework                        │
└──────────────────────────────────────────────────────────┘
                         │
        ┌────────────────┼────────────────┐
        │                │                │
        ▼                ▼                ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ World Builder│  │Packet Capture│  │Test Assertions│
│              │  │  & Replay    │  │              │
└──────────────┘  └──────────────┘  └──────────────┘
        │                │                │
        └────────────────┼────────────────┘
                         ▼
               ┌──────────────────┐
               │  Mock Interfaces │
               │  (World, Comms)  │
               └──────────────────┘
                         │
        ┌────────────────┼────────────────┐
        │                │                │
        ▼                ▼                ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ PathFinder   │  │  Movement    │  │  Following   │
│ (Under Test) │  │  (Under Test)│  │ (Under Test) │
└──────────────┘  └──────────────┘  └──────────────┘
```

---

## Implementation Plan

### Phase 1: Mock World Interface

#### Step 1.1: Create MockWorld Implementation

**File:** `testing/mock_world.go`

```go
package testing

import (
	"sync"
	"github.com/reallyoldfogie/mc-bot-go/bot/world"
)

// MockWorld implements a synthetic world for testing
type MockWorld struct {
	mu     sync.RWMutex
	chunks map[ChunkPos]*ChunkData
	blocks map[BlockPos]uint32 // Simple block storage
}

type ChunkPos struct {
	X, Z int32
}

type BlockPos struct {
	X, Y, Z int
}

// NewMockWorld creates an empty test world
func NewMockWorld() *MockWorld {
	return &MockWorld{
		chunks: make(map[ChunkPos]*ChunkData),
		blocks: make(map[BlockPos]uint32),
	}
}

// GetBlockAt implements the World interface
func (mw *MockWorld) GetBlockAt(x, y, z int) uint32 {
	mw.mu.RLock()
	defer mw.mu.RUnlock()

	pos := BlockPos{X: x, Y: y, Z: z}
	if stateID, exists := mw.blocks[pos]; exists {
		return stateID
	}
	return 0 // Air
}

// SetBlock sets a block in the mock world
func (mw *MockWorld) SetBlock(x, y, z int, stateID uint32) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	pos := BlockPos{X: x, Y: y, Z: z}
	mw.blocks[pos] = stateID
}

// LoadChunk loads a pre-built chunk
func (mw *MockWorld) LoadChunk(chunkX, chunkZ int32, chunk *ChunkData) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	pos := ChunkPos{X: chunkX, Z: chunkZ}
	mw.chunks[pos] = chunk
}
```

#### Step 1.2: World Builder Helper

**File:** `testing/world_builder.go`

```go
package testing

import (
	"github.com/reallyoldfogie/mc-agent/pathfinding"
)

// WorldBuilder provides fluent API for building test worlds
type WorldBuilder struct {
	world *MockWorld
	blockMgr BlockManager
}

// NewWorldBuilder creates a new world builder
func NewWorldBuilder(blockMgr BlockManager) *WorldBuilder {
	return &WorldBuilder{
		world:    NewMockWorld(),
		blockMgr: blockMgr,
	}
}

// FlatGround creates a flat grass platform
func (wb *WorldBuilder) FlatGround(x1, z1, x2, z2, y int) *WorldBuilder {
	grassStateID := wb.blockMgr.GetStateID("minecraft:grass_block", nil)

	for x := x1; x <= x2; x++ {
		for z := z1; z <= z2; z++ {
			wb.world.SetBlock(x, y, z, grassStateID)
		}
	}

	return wb
}

// Stairs creates a staircase
func (wb *WorldBuilder) Stairs(startX, startY, startZ, length int, direction string) *WorldBuilder {
	stairStateID := wb.blockMgr.GetStateID("minecraft:oak_stairs", map[string]string{
		"facing": direction,
		"half":   "bottom",
	})

	for i := 0; i < length; i++ {
		switch direction {
		case "north":
			wb.world.SetBlock(startX, startY+i, startZ-i, stairStateID)
		case "south":
			wb.world.SetBlock(startX, startY+i, startZ+i, stairStateID)
		case "east":
			wb.world.SetBlock(startX+i, startY+i, startZ, stairStateID)
		case "west":
			wb.world.SetBlock(startX-i, startY+i, startZ, stairStateID)
		}
	}

	return wb
}

// Wall creates a vertical wall
func (wb *WorldBuilder) Wall(x1, z1, x2, z2, yBottom, yTop int, blockName string) *WorldBuilder {
	stateID := wb.blockMgr.GetStateID(blockName, nil)

	for x := x1; x <= x2; x++ {
		for z := z1; z <= z2; z++ {
			for y := yBottom; y <= yTop; y++ {
				wb.world.SetBlock(x, y, z, stateID)
			}
		}
	}

	return wb
}

// Gap creates a gap in the ground (for jump testing)
func (wb *WorldBuilder) Gap(x, z, y, width int) *WorldBuilder {
	for i := 0; i < width; i++ {
		wb.world.SetBlock(x+i, y, z, 0) // Air
	}
	return wb
}

// Water fills an area with water
func (wb *WorldBuilder) Water(x1, y1, z1, x2, y2, z2 int) *WorldBuilder {
	waterStateID := wb.blockMgr.GetStateID("minecraft:water", nil)

	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			for z := z1; z <= z2; z++ {
				wb.world.SetBlock(x, y, z, waterStateID)
			}
		}
	}

	return wb
}

// Ladder places a ladder
func (wb *WorldBuilder) Ladder(x, z, yBottom, yTop int, facing string) *WorldBuilder {
	ladderStateID := wb.blockMgr.GetStateID("minecraft:ladder", map[string]string{
		"facing": facing,
	})

	for y := yBottom; y <= yTop; y++ {
		wb.world.SetBlock(x, y, z, ladderStateID)
	}

	return wb
}

// Build returns the constructed world
func (wb *WorldBuilder) Build() *MockWorld {
	return wb.world
}
```

---

### Phase 2: Test Scenarios

#### Step 2.1: Pathfinding Unit Tests

**File:** `pathfinding/pathfinding_test.go`

```go
package pathfinding_test

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

func TestFlatGroundPathfinding(t *testing.T) {
	// Setup: Create flat grass world
	blockMgr := loadTestBlockManager(t)
	shapeMgr := loadTestShapeManager(t)

	world := mctesting.NewWorldBuilder(blockMgr).
		FlatGround(0, 0, 20, 20, 64).
		Build()

	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	// Test: Find path from (0, 65, 0) to (10, 65, 10)
	start := pathfinding.V3{X: 0, Y: 65, Z: 0}
	goal := pathfinding.V3{X: 10, Y: 65, Z: 10}

	path, err := pathFinder.FindPath(start, goal, 200)

	// Assert: Path found and valid
	if err != nil {
		t.Fatalf("Failed to find path: %v", err)
	}

	if path == nil {
		t.Fatal("Path is nil")
	}

	// Assert: Path reaches goal
	lastStep := path.Steps[len(path.Steps)-1]
	if lastStep.Position.X != goal.X || lastStep.Position.Z != goal.Z {
		t.Errorf("Path doesn't reach goal. Last step: (%d, %d, %d), Goal: (%d, %d, %d)",
			lastStep.Position.X, lastStep.Position.Y, lastStep.Position.Z,
			goal.X, goal.Y, goal.Z)
	}

	// Assert: Path is efficient (Manhattan distance + some tolerance)
	expectedSteps := abs(goal.X-start.X) + abs(goal.Z-start.Z)
	if len(path.Steps) > expectedSteps*2 {
		t.Errorf("Path too long. Expected ~%d steps, got %d", expectedSteps, len(path.Steps))
	}
}

func TestStairsPathfinding(t *testing.T) {
	blockMgr := loadTestBlockManager(t)
	shapeMgr := loadTestShapeManager(t)

	// Setup: Flat ground with stairs going up
	world := mctesting.NewWorldBuilder(blockMgr).
		FlatGround(0, 0, 20, 20, 64).
		Stairs(5, 65, 5, 5, "north"). // 5 stairs going north
		Build()

	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	// Test: Find path from bottom to top of stairs
	start := pathfinding.V3{X: 5, Y: 65, Z: 10}
	goal := pathfinding.V3{X: 5, Y: 70, Z: 5} // Top of stairs

	path, err := pathFinder.FindPath(start, goal, 200)

	// Assert: Path found
	if err != nil {
		t.Fatalf("Failed to find path up stairs: %v", err)
	}

	// Assert: Path uses Ascend movements for stairs
	ascendCount := 0
	for _, step := range path.Steps {
		if step.Movement == pathfinding.Ascend {
			ascendCount++
		}
	}

	if ascendCount < 3 {
		t.Errorf("Expected multiple Ascend movements for stairs, got %d", ascendCount)
	}
}

func TestWallBlocksPath(t *testing.T) {
	blockMgr := loadTestBlockManager(t)
	shapeMgr := loadTestShapeManager(t)

	// Setup: Flat ground with wall blocking direct path
	world := mctesting.NewWorldBuilder(blockMgr).
		FlatGround(0, 0, 20, 20, 64).
		Wall(10, 0, 10, 20, 65, 67, "minecraft:stone"). // 3-block tall wall
		Build()

	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	// Test: Find path around wall
	start := pathfinding.V3{X: 5, Y: 65, Z: 10}
	goal := pathfinding.V3{X: 15, Y: 65, Z: 10}

	path, err := pathFinder.FindPath(start, goal, 200)

	// Assert: Path found (goes around wall)
	if err != nil {
		t.Fatalf("Failed to find path around wall: %v", err)
	}

	// Assert: Path doesn't go through wall (X=10)
	for _, step := range path.Steps {
		if step.Position.X == 10 && step.Position.Z >= 0 && step.Position.Z <= 20 {
			t.Errorf("Path goes through wall at (%d, %d, %d)",
				step.Position.X, step.Position.Y, step.Position.Z)
		}
	}
}

func TestGapJumping(t *testing.T) {
	blockMgr := loadTestBlockManager(t)
	shapeMgr := loadTestShapeManager(t)

	// Setup: Flat ground with 1-block gap
	world := mctesting.NewWorldBuilder(blockMgr).
		FlatGround(0, 0, 20, 20, 64).
		Gap(10, 10, 64, 1). // 1-block gap
		Build()

	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	// Test: Find path across gap
	start := pathfinding.V3{X: 5, Y: 65, Z: 10}
	goal := pathfinding.V3{X: 15, Y: 65, Z: 10}

	path, err := pathFinder.FindPath(start, goal, 200)

	// Assert: Path found
	if err != nil {
		t.Fatalf("Failed to find path across gap: %v", err)
	}

	// Assert: Path uses Jump movement
	hasJump := false
	for _, step := range path.Steps {
		if step.Movement == pathfinding.Jump {
			hasJump = true
			break
		}
	}

	if !hasJump {
		t.Error("Expected Jump movement for 1-block gap")
	}
}

func TestNegativeYCoordinates(t *testing.T) {
	blockMgr := loadTestBlockManager(t)
	shapeMgr := loadTestShapeManager(t)

	// Setup: Flat ground at negative Y
	world := mctesting.NewWorldBuilder(blockMgr).
		FlatGround(0, 0, 20, 20, -60). // Y = -60
		Build()

	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	// Test: Find path at negative Y
	start := pathfinding.V3{X: 0, Y: -59, Z: 0}
	goal := pathfinding.V3{X: 10, Y: -59, Z: 10}

	path, err := pathFinder.FindPath(start, goal, 200)

	// Assert: Path found (tests negative Y coordinate fix)
	if err != nil {
		t.Fatalf("Failed to find path at negative Y: %v", err)
	}

	// Assert: All path steps at correct Y level
	for i, step := range path.Steps {
		if step.Position.Y < -60 || step.Position.Y > -58 {
			t.Errorf("Step %d has wrong Y: %d (expected -59)", i, step.Position.Y)
		}
	}
}

// Helper functions
func loadTestBlockManager(t *testing.T) BlockManager {
	// Load from test data or create mock
	// This would use actual mc-protocol-go block manager
	t.Helper()
	// Implementation here
	return nil
}

func loadTestShapeManager(t *testing.T) ShapeManager {
	// Load from mc-data-gen test data
	t.Helper()
	// Implementation here
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
```

---

### Phase 3: Movement Executor Tests

**File:** `movement/executor_test.go`

```go
package movement_test

import (
	"testing"
	"math"

	"github.com/reallyoldfogie/mc-agent/movement"
)

// MockPositionTracker tracks bot position for testing
type MockPositionTracker struct {
	x, y, z          float64
	yaw, pitch       float32
	positionHistory  []Position
}

type Position struct {
	X, Y, Z    float64
	Yaw, Pitch float32
}

func (mpt *MockPositionTracker) GetPosition() (float64, float64, float64, float32, float32, bool) {
	return mpt.x, mpt.y, mpt.z, mpt.yaw, mpt.pitch, true
}

func (mpt *MockPositionTracker) SetPosition(x, y, z float64, yaw, pitch float32, onGround bool) error {
	mpt.x = x
	mpt.y = y
	mpt.z = z
	mpt.yaw = yaw
	mpt.pitch = pitch

	// Record position history
	mpt.positionHistory = append(mpt.positionHistory, Position{
		X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch,
	})

	return nil
}

func TestMoveTowards_FlatGround(t *testing.T) {
	// Setup
	tracker := &MockPositionTracker{
		x: 0, y: 65, z: 0,
		yaw: 0, pitch: 0,
	}

	executor := movement.NewMovementExecutor(
		tracker.GetPosition,
		tracker.SetPosition,
		nil, // packetMgr not needed for test
	)

	// Test: Move 0.2 blocks towards (10, 65, 0)
	newX, newY, newZ, err := executor.MoveTowards(10, 65, 0, 0.2, true)

	// Assert: No error
	if err != nil {
		t.Fatalf("MoveTowards failed: %v", err)
	}

	// Assert: Moved correct distance
	distance := math.Sqrt(newX*newX + newZ*newZ)
	expectedDist := 0.2
	if math.Abs(distance-expectedDist) > 0.01 {
		t.Errorf("Moved wrong distance. Expected %.2f, got %.2f", expectedDist, distance)
	}

	// Assert: Y unchanged (flat ground)
	if math.Abs(newY-65.0) > 0.01 {
		t.Errorf("Y changed on flat ground. Expected 65.0, got %.2f", newY)
	}
}

func TestMoveTowards_UpwardMovement(t *testing.T) {
	// Setup
	tracker := &MockPositionTracker{
		x: 0, y: 65, z: 0,
	}

	executor := movement.NewMovementExecutor(
		tracker.GetPosition,
		tracker.SetPosition,
		nil,
	)

	// Test: Move towards higher position (stairs)
	targetY := 65.5 // Stair surface
	newX, newY, newZ, err := executor.MoveTowards(1, targetY, 0, 0.2, true)

	// Assert: No error
	if err != nil {
		t.Fatalf("MoveTowards failed: %v", err)
	}

	// Assert: Y reaches target immediately (smart physics fix)
	if math.Abs(newY-targetY) > 0.01 {
		t.Errorf("Y should reach target immediately. Expected %.2f, got %.2f", targetY, newY)
	}
}

func TestMoveTowards_DownwardMovement(t *testing.T) {
	// Setup
	tracker := &MockPositionTracker{
		x: 0, y: 65, z: 0,
	}

	executor := movement.NewMovementExecutor(
		tracker.GetPosition,
		tracker.SetPosition,
		nil,
	)

	// Test: Move towards lower position (falling)
	targetY := 64.0
	newX, newY, newZ, err := executor.MoveTowards(1, targetY, 0, 0.2, true)

	// Assert: No error
	if err != nil {
		t.Fatalf("MoveTowards failed: %v", err)
	}

	// Assert: Y descends with gravity (max 0.5 blocks/tick)
	expectedY := 64.5 // 65.0 - 0.5 (max fall)
	if math.Abs(newY-expectedY) > 0.01 {
		t.Errorf("Y should descend gradually. Expected %.2f, got %.2f", expectedY, newY)
	}
}
```

---

### Phase 4: Packet Capture & Replay

#### Step 4.1: Packet Recorder

**File:** `testing/packet_recorder.go`

```go
package testing

import (
	"encoding/json"
	"os"
	"time"
)

// PacketRecord represents a captured packet
type PacketRecord struct {
	Timestamp time.Time              `json:"timestamp"`
	PacketID  int32                  `json:"packet_id"`
	PacketName string                `json:"packet_name"`
	Data      map[string]interface{} `json:"data"`
}

// PacketRecorder records packets to a file
type PacketRecorder struct {
	file    *os.File
	encoder *json.Encoder
}

// NewPacketRecorder creates a new packet recorder
func NewPacketRecorder(filename string) (*PacketRecorder, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	return &PacketRecorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}, nil
}

// RecordPacket records a packet
func (pr *PacketRecorder) RecordPacket(packetID int32, packetName string, data map[string]interface{}) error {
	record := PacketRecord{
		Timestamp:  time.Now(),
		PacketID:   packetID,
		PacketName: packetName,
		Data:       data,
	}

	return pr.encoder.Encode(record)
}

// Close closes the recorder
func (pr *PacketRecorder) Close() error {
	return pr.file.Close()
}
```

#### Step 4.2: Packet Replayer

**File:** `testing/packet_replayer.go`

```go
package testing

import (
	"encoding/json"
	"os"
	"time"
)

// PacketReplayer replays packets from a file
type PacketReplayer struct {
	file     *os.File
	decoder  *json.Decoder
	handlers map[string]PacketHandler
}

// PacketHandler processes a replayed packet
type PacketHandler func(data map[string]interface{}) error

// NewPacketReplayer creates a new packet replayer
func NewPacketReplayer(filename string) (*PacketReplayer, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	return &PacketReplayer{
		file:     file,
		decoder:  json.NewDecoder(file),
		handlers: make(map[string]PacketHandler),
	}, nil
}

// RegisterHandler registers a handler for a packet type
func (pr *PacketReplayer) RegisterHandler(packetName string, handler PacketHandler) {
	pr.handlers[packetName] = handler
}

// Replay replays all packets
func (pr *PacketReplayer) Replay() error {
	for {
		var record PacketRecord
		err := pr.decoder.Decode(&record)
		if err != nil {
			if err.Error() == "EOF" {
				break // End of file
			}
			return err
		}

		// Call handler if registered
		if handler, exists := pr.handlers[record.PacketName]; exists {
			if err := handler(record.Data); err != nil {
				return err
			}
		}
	}

	return nil
}

// Close closes the replayer
func (pr *PacketReplayer) Close() error {
	return pr.file.Close()
}
```

#### Step 4.3: Integration Test with Packet Replay

**File:** `testing/integration_test.go`

```go
package testing_test

import (
	"testing"

	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

func TestReplayFollowingSession(t *testing.T) {
	// Load recorded packet capture from real game session
	replayer, err := mctesting.NewPacketReplayer("testdata/follow_session_001.json")
	if err != nil {
		t.Fatalf("Failed to load packet capture: %v", err)
	}
	defer replayer.Close()

	// Setup: Create test world from recorded chunks
	world := mctesting.NewMockWorld()

	// Register handlers to populate world
	replayer.RegisterHandler("ClientboundLevelChunkWithLight", func(data map[string]interface{}) error {
		// Parse chunk data and load into mock world
		// Implementation here
		return nil
	})

	replayer.RegisterHandler("ClientboundAddEntity", func(data map[string]interface{}) error {
		// Track entities
		// Implementation here
		return nil
	})

	// Replay entire session
	if err := replayer.Replay(); err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	// Now test pathfinding with the replayed world state
	// Assert expected behavior
}
```

---

### Phase 5: Benchmarking

**File:** `pathfinding/benchmark_test.go`

```go
package pathfinding_test

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

func BenchmarkPathfinding_FlatGround_10Blocks(b *testing.B) {
	blockMgr := loadTestBlockManager(b)
	shapeMgr := loadTestShapeManager(b)

	world := mctesting.NewWorldBuilder(blockMgr).
		FlatGround(-50, -50, 50, 50, 64).
		Build()

	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	start := pathfinding.V3{X: 0, Y: 65, Z: 0}
	goal := pathfinding.V3{X: 10, Y: 65, Z: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pathFinder.FindPath(start, goal, 200)
	}
}

func BenchmarkPathfinding_ComplexTerrain_50Blocks(b *testing.B) {
	blockMgr := loadTestBlockManager(b)
	shapeMgr := loadTestShapeManager(b)

	world := mctesting.NewWorldBuilder(blockMgr).
		FlatGround(-50, -50, 50, 50, 64).
		Stairs(10, 65, 10, 5, "north").
		Wall(20, -10, 20, 10, 65, 67, "minecraft:stone").
		Water(30, 65, 30, 35, 65, 35).
		Build()

	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	start := pathfinding.V3{X: 0, Y: 65, Z: 0}
	goal := pathfinding.V3{X: 50, Y: 65, Z: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pathFinder.FindPath(start, goal, 200)
	}
}
```

---

## Test Data Management

### Directory Structure

```
mc-agent/
├── testing/
│   ├── mock_world.go
│   ├── world_builder.go
│   ├── packet_recorder.go
│   ├── packet_replayer.go
│   └── testdata/
│       ├── block_manager_1.21.5.json
│       ├── shape_manager_1.21.5.json
│       ├── follow_session_001.json
│       ├── follow_session_002.json
│       └── stairs_navigation_001.json
│
├── pathfinding/
│   ├── pathfinding_test.go
│   └── benchmark_test.go
│
├── movement/
│   └── executor_test.go
│
└── following/
    └── manager_test.go
```

### Test Data Files

**testdata/block_manager_1.21.5.json**:
```json
{
  "version": "1.21.5",
  "blocks": {
    "minecraft:grass_block": {
      "id": 8,
      "states": [
        {"id": 9, "properties": {"snowy": "false"}, "default": true},
        {"id": 10, "properties": {"snowy": "true"}}
      ]
    }
  }
}
```

**testdata/follow_session_001.json**:
```json
[
  {
    "timestamp": "2025-11-27T10:00:00Z",
    "packet_id": 35,
    "packet_name": "ClientboundLevelChunkWithLight",
    "data": {
      "chunkX": 0,
      "chunkZ": 0,
      "chunkData": "..."
    }
  }
]
```

---

## Running Tests

### Command Line

```bash
# Run all tests
go test ./...

# Run specific test
go test -run TestFlatGroundPathfinding ./pathfinding

# Run benchmarks
go test -bench=. ./pathfinding

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### CI/CD Integration

**File:** `.github/workflows/test.yml`

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.24'

      - name: Run tests
        run: go test -v -cover ./...

      - name: Run benchmarks
        run: go test -bench=. ./pathfinding

      - name: Upload coverage
        uses: codecov/codecov-action@v3
```

---

## Test Scenarios Catalog

### Pathfinding Tests

| Test Name | Description | World Setup | Expected Outcome |
|-----------|-------------|-------------|------------------|
| FlatGroundPathfinding | Basic A* on flat terrain | 20×20 grass platform | Straight line path |
| StairsAscent | Navigate up stairs | 5-stair staircase | Uses Ascend movements |
| StairsDescent | Navigate down stairs | 5-stair staircase | Uses Descend movements |
| WallAvoidance | Path around obstacle | Wall blocking direct path | Detours around |
| GapJumping | Jump 1-block gap | 1-block gap in floor | Uses Jump movement |
| WideGapDetour | Too wide to jump | 3-block gap | Detours around |
| WaterNavigation | Swim through water | Water-filled area | Uses Swim movements |
| LadderClimbing | Climb vertical ladder | Vertical ladder | Uses Climb movements |
| NegativeYCoordinates | Pathfind at Y<0 | Ground at Y=-60 | Correct ground detection |
| ComplexTerrain | Mixed obstacles | Stairs+walls+water | Valid path found |

### Movement Tests

| Test Name | Description | Setup | Expected Outcome |
|-----------|-------------|-------|------------------|
| FlatGroundMovement | Move on flat terrain | Bot at (0,65,0) | Moves 0.2 blocks/tick |
| UpwardMovement | Move onto stairs | Target Y=65.5 | Y reaches target immediately |
| DownwardMovement | Fall/descend | Target Y=64.0 | Y descends 0.5 blocks/tick |
| LookAtTarget | Rotate towards point | Target at (10,65,10) | Correct yaw/pitch |
| StopAtDestination | Stop when close | Distance < 0.2 | Movement halts |

### Integration Tests

| Test Name | Description | Data Source | Expected Outcome |
|-----------|-------------|-------------|------------------|
| ReplayFollowingSession | Full following scenario | Recorded packets | Bot follows player |
| ReplayStuckRecovery | Recovery from stuck | Recorded stuck scenario | Bot recovers |
| ReplayStairsNavigation | Real stairs navigation | Recorded stairs session | Bot ascends correctly |

---

## Benefits

### Development Speed
- **Faster iteration**: No need to launch Minecraft for every test
- **Reproducible bugs**: Capture problematic scenarios once, replay forever
- **Parallel testing**: Run hundreds of scenarios in seconds

### Quality
- **Regression prevention**: Tests catch breaking changes immediately
- **Edge case coverage**: Test rare scenarios (negative Y, complex terrain)
- **Performance monitoring**: Benchmarks detect slowdowns

### Debugging
- **Isolated testing**: Test pathfinding without network/server complexity
- **Step-through debugging**: Easier to debug with synthetic data
- **Deterministic**: Same input = same output (no server randomness)

---

## Implementation Timeline

**Week 1: Foundation**
- Implement MockWorld and WorldBuilder
- Create basic pathfinding tests
- Setup test running infrastructure

**Week 2: Comprehensive Tests**
- Add movement executor tests
- Create following manager tests
- Build test scenario catalog

**Week 3: Packet Replay**
- Implement packet recorder/replayer
- Capture real game sessions
- Create integration tests

**Week 4: CI/CD**
- Setup GitHub Actions
- Add coverage reporting
- Documentation and examples

---

## Success Metrics

### Coverage Targets
- Pathfinding: 80%+ code coverage
- Movement: 80%+ code coverage
- Following: 70%+ code coverage

### Performance Targets
- Test suite runtime: < 5 seconds
- Pathfinding (10 blocks): < 5ms
- Pathfinding (50 blocks): < 50ms

### Quality Targets
- Zero false negatives (tests pass when code works)
- Zero false positives (tests fail when code broken)
- All major bug scenarios captured as tests

---

## Future Enhancements

1. **Visual Test Replay** - Render path/movement in 3D viewer
2. **Fuzz Testing** - Generate random worlds to find edge cases
3. **Property-Based Testing** - Verify pathfinding properties (optimality, correctness)
4. **Mutation Testing** - Verify tests catch code changes
5. **Performance Regression** - Track performance over time

---

## Conclusion

Automated testing provides:
- ✅ **Fast feedback loop** (seconds instead of minutes)
- ✅ **Comprehensive coverage** (hundreds of scenarios)
- ✅ **Reproducible debugging** (capture bugs once)
- ✅ **Confidence in changes** (regression prevention)
- ✅ **Better code quality** (forces testable design)

The testing framework is **orthogonal** to the bug fixes implemented today - it can be added incrementally without disrupting existing code.

---

**Document Status**: ✅ Ready for Implementation
**Last Updated**: 2025-11-27
**Author**: Claude Code (Design + Planning)
