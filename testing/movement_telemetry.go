package testing

import (
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
)

// MovementTelemetry tracks movement mode usage during path execution
type MovementTelemetry struct {
	JumpCount     int       // Number of jumps executed
	ClimbTicks    int       // Ticks spent on ladders/vines
	SneakTicks    int       // Ticks spent sneaking
	MaxDeltaY     float64   // Maximum vertical change per tick
	TotalTicks    int       // Total execution time in ticks
	PathSteps     int       // Number of PathSteps executed
	StartPos      models.V3 // Starting position
	EndPos        models.V3 // Final position
	MovementTypes []string  // Sequence of movement types used
}

// TelemetryRecorder records movement telemetry during execution
type TelemetryRecorder struct {
	mu        sync.Mutex
	telemetry MovementTelemetry
	lastPos   models.V3
	recording bool
}

// NewTelemetryRecorder creates a new telemetry recorder
func NewTelemetryRecorder() *TelemetryRecorder {
	return &TelemetryRecorder{
		telemetry: MovementTelemetry{
			MovementTypes: make([]string, 0),
		},
	}
}

// Start begins recording telemetry from the given start position
func (tr *TelemetryRecorder) Start(startPos models.V3) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.recording = true
	tr.telemetry = MovementTelemetry{
		StartPos:      startPos,
		MovementTypes: make([]string, 0),
	}
	tr.lastPos = startPos
}

// RecordTick records movement state for a single tick
// Implements models.MovementTelemetryRecorder interface
func (tr *TelemetryRecorder) RecordTick(x, y, z float64, onGround bool, climbing bool, sneaking bool) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if !tr.recording {
		return
	}

	tr.telemetry.TotalTicks++

	// Track climbing ticks
	if climbing {
		tr.telemetry.ClimbTicks++
	}

	// Track sneaking ticks
	if sneaking {
		tr.telemetry.SneakTicks++
	}

	// Track maximum vertical change per tick
	deltaY := y - tr.lastPos.Y
	if deltaY > tr.telemetry.MaxDeltaY {
		tr.telemetry.MaxDeltaY = deltaY
	}

	tr.lastPos = models.V3{X: x, Y: y, Z: z}
}

// RecordJump records a jump action
func (tr *TelemetryRecorder) RecordJump() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if !tr.recording {
		return
	}

	tr.telemetry.JumpCount++
}

// RecordStep records a completed path step with its movement type
func (tr *TelemetryRecorder) RecordStep(stepType string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if !tr.recording {
		return
	}

	tr.telemetry.PathSteps++
	tr.telemetry.MovementTypes = append(tr.telemetry.MovementTypes, stepType)
}

// Stop ends recording and returns the collected telemetry
func (tr *TelemetryRecorder) Stop(endPos models.V3) MovementTelemetry {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tr.recording = false
	tr.telemetry.EndPos = endPos

	// Return a copy to avoid mutation
	result := tr.telemetry
	result.MovementTypes = make([]string, len(tr.telemetry.MovementTypes))
	copy(result.MovementTypes, tr.telemetry.MovementTypes)

	return result
}

// GetTelemetry returns the current telemetry without stopping recording
func (tr *TelemetryRecorder) GetTelemetry() MovementTelemetry {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	// Return a copy to avoid mutation
	result := tr.telemetry
	result.MovementTypes = make([]string, len(tr.telemetry.MovementTypes))
	copy(result.MovementTypes, tr.telemetry.MovementTypes)

	return result
}

// IsRecording returns whether telemetry is currently being recorded
func (tr *TelemetryRecorder) IsRecording() bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	return tr.recording
}
