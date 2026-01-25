package models

// MovementTelemetryRecorder is an interface for recording movement telemetry during path execution.
// This allows the movement executor to record telemetry without depending on the testing package.
type MovementTelemetryRecorder interface {
	// RecordTick records movement state for a single tick
	RecordTick(x, y, z float64, onGround bool, climbing bool, sneaking bool)

	// RecordJump records a jump action
	RecordJump()

	// RecordStep records a completed path step with its movement type
	RecordStep(stepType string)
}
