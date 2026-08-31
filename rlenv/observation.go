package rlenv

import "github.com/reallyoldfogie/cRL-go/pkg/rl"

// observationSize is the fixed length of every rl.Observation this package
// produces (Environment.ObservationSize). Feature layout — this is a fixed
// contract with whatever policy network is trained against it (see
// RL_POLICY_INTEGRATION_PLAN.md item 2's note that this layout must be
// documented alongside the builder):
//
//	index 0: dx   — targetX - current X (blocks)
//	index 1: dy   — targetY - current Y (blocks)
//	index 2: dz   — targetZ - current Z (blocks)
//	index 3: yaw  — current yaw, degrees
//	index 4: pitch — current pitch, degrees
//	index 5: health
//	index 6: food level
//	index 7: food saturation
//	index 8: healthKnown — 1.0 if a HealthChange event has been observed
//	         yet, 0.0 otherwise. Without this, an unknown health reads as
//	         health=0 (the zero value), which is indistinguishable from
//	         "dead" — this bit exists specifically to avoid that ambiguity,
//	         not as speculative extra state.
//
// There is no task-type feature: this environment only poses one task (see
// task.go), so there is nothing yet to condition on. See doc.go.
const observationSize = 9

// buildObservation constructs the fixed-length feature vector described
// above from the bot's current position/rotation and health/food state.
// yaw/pitch are float64 to match models.Position.GetPosition's return type;
// every feature in Values is float32 regardless (rl.Observation's contract).
func buildObservation(x, y, z float64, yaw, pitch float64, targetX, targetY, targetZ float64, health float32, food int32, saturation float32, healthKnown bool) rl.Observation {
	known := float32(0)
	if healthKnown {
		known = 1
	}
	return rl.Observation{
		Values: []float32{
			float32(targetX - x),
			float32(targetY - y),
			float32(targetZ - z),
			float32(yaw),
			float32(pitch),
			health,
			float32(food),
			saturation,
			known,
		},
	}
}
