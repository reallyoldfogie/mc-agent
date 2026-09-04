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
//	index 9:  mineDx — nearest visible Config.MineTargetBlock instance's
//	          X minus current X (blocks); 0 if not visible/not configured.
//	index 10: mineDy — same, Y.
//	index 11: mineDz — same, Z.
//	index 12: mineVisible — 1.0 if Config.MineTargetBlock is set and a
//	          visible instance was found this step (see
//	          Environment.resolveMineTarget), 0.0 otherwise. Gates
//	          mineDx/Dy/Dz the same way healthKnown gates health: "no
//	          target visible" must not be misread as "target is exactly
//	          here" (mineDx=mineDy=mineDz=0 is also each field's zero
//	          value).
//
// mineDx/Dy/Dz/mineVisible are present in every observation this package
// produces, whether or not the episode's Config sets MineTargetBlock, and
// regardless of which action was actually dispatched a given step — mirrors
// how dx/dy/dz always reflect the GoToTarget target even on steps that
// dispatch ActionWait or ActionReturnHome, not only on ActionGoToTarget
// steps. There is otherwise no task-type feature: MineTargetBlock is this
// environment's second task, layered onto the existing "reach a point" one
// rather than replacing it (both can be configured on the same instance;
// see task.go). See doc.go.
const observationSize = 13

// buildObservation constructs the fixed-length feature vector described
// above from the bot's current position/rotation, health/food state, and
// (if configured/visible) nearest mine-target block position. yaw/pitch are
// float64 to match models.Position.GetPosition's return type; every feature
// in Values is float32 regardless (rl.Observation's contract).
func buildObservation(x, y, z float64, yaw, pitch float64, targetX, targetY, targetZ float64, health float32, food int32, saturation float32, healthKnown bool, mineX, mineY, mineZ float64, mineVisible bool) rl.Observation {
	known := float32(0)
	if healthKnown {
		known = 1
	}
	mineDx, mineDy, mineDz, visible := float32(0), float32(0), float32(0), float32(0)
	if mineVisible {
		mineDx, mineDy, mineDz, visible = float32(mineX-x), float32(mineY-y), float32(mineZ-z), 1
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
			mineDx,
			mineDy,
			mineDz,
			visible,
		},
	}
}
