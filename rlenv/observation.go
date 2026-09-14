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
//	index 13: craftReady — 1.0 if Config.CraftTargetItem is set and its
//	          recipe currently looks assembleable from held ingredients
//	          (see LiveAgent.Craftable — an approximate signal, not a
//	          guarantee), 0.0 otherwise. Plays the same role for the craft
//	          task that mineVisible plays for mine: crafting has no natural
//	          position/distance to encode (both the 2x2 grid and an opened
//	          table are reachable from wherever the bot stands, once
//	          CraftItem itself handles locating a table if one's needed),
//	          so "does this look worth attempting" is the useful signal
//	          instead — see docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1c.
//
//	index 14: goalGoToActive — 1.0 if ActionGoToTarget is legal this
//	          episode (!Config.GoToTargetDisabled), 0.0 otherwise.
//	index 15: goalMineActive — 1.0 if ActionMine is legal this episode
//	          (Config.MineTargetBlock set), 0.0 otherwise. Reflects whether
//	          mining is this episode's *task*, not whether a target is
//	          currently *visible* — that's index 12 (mineVisible)'s job;
//	          the two differ whenever MineTargetBlock is configured but
//	          nothing happens to be in view right now.
//	index 16: goalCraftActive — 1.0 if ActionCraft is legal this episode
//	          (Config.CraftTargetItem set), 0.0 otherwise. Same
//	          active-vs-ready distinction as goalMineActive above, against
//	          index 13 (craftReady).
//
// indices 14-16 are this environment's goal-conditioning block
// (../mc-rsi-trainer/docs/plans/06-per-episode-task-selection-and-goal-conditioning.md):
// a multi-hot (not strictly one-hot — see Config.TaskSelector's own doc
// comment on why more than one task can be simultaneously active) signal
// telling a policy which task(s) this specific episode is actually about,
// as opposed to indices 0-13's always-present *per-task* numeric signals
// (dx/dy/dz, mineDx/Dy/Dz, mineVisible, craftReady), which exist and are
// non-zero whenever their task happens to be configured/visible,
// regardless of whether Config.TaskSelector chose that task as this
// episode's actual goal. Mirrors cRL-go's own subgoal-one-hot observation
// augmentation (pkg/hierarchical/augment.go, see
// docs/plans/11-hierarchical-meta-controller-and-subpolicies.md) rather
// than inventing a new encoding — this repo's task generator and this
// package's own observation builder are the two writers of the same kind
// of goal block, per ../mc-rsi-trainer/docs/plans/00's explicit
// instruction not to invent a separate curriculum-only encoding. No
// additional numeric parameters are appended beyond the three
// active-flags themselves (unlike a from-scratch goal-conditioning
// design might need): every numeric parameter goal-conditioning would
// otherwise want (target distance, mine-target position, ...) already
// exists in indices 0-13, since this environment's observation was
// already fully per-task before this block was added — the only thing
// actually missing was "which of these is the real goal," which a
// three-bit flag answers completely on its own.
//
// mineDx/Dy/Dz/mineVisible/craftReady (and now goalGoToActive/
// goalMineActive/goalCraftActive) are present in every observation this
// package produces, whether or not the episode's Config sets
// MineTargetBlock/CraftTargetItem, and regardless of which action was
// actually dispatched a given step — mirrors how dx/dy/dz always reflect
// the GoToTarget target even on steps that dispatch ActionWait, not only
// on ActionGoToTarget steps. MineTargetBlock/CraftTargetItem are this
// environment's second and third tasks, layered onto the existing "reach a
// point" one rather than replacing it (all three can be configured on the
// same instance; see task.go). See doc.go.
//
// Changing this length is a breaking change for any checkpoint trained
// against the old 14-length observation — see actorcritic.Load's existing
// EnvironmentID/shape validation, and mint a new EnvironmentID for
// anything trained against this length (e.g. cmd/rl-train's own
// "mc-agent-rlenv:actions=%d:obs=%d" already derives its ID from
// ObservationSize()/ActionSpace() directly, so it picks up this change
// automatically with no separate version bump needed there).
const observationSize = 17

// buildObservation constructs the fixed-length feature vector described
// above from the bot's current position/rotation, health/food state,
// (if configured/visible) nearest mine-target block position, (if
// configured) whether the craft target currently looks assembleable, and
// this episode's goal-conditioning block (which of the three tasks are
// actually active — see observationSize's own doc comment on indices
// 14-16). yaw/pitch are float64 to match models.Position.GetPosition's
// return type; every feature in Values is float32 regardless
// (rl.Observation's contract).
func buildObservation(x, y, z float64, yaw, pitch float64, targetX, targetY, targetZ float64, health float32, food int32, saturation float32, healthKnown bool, mineX, mineY, mineZ float64, mineVisible bool, craftReady bool, goToActive, mineActive, craftActive bool) rl.Observation {
	known := float32(0)
	if healthKnown {
		known = 1
	}
	mineDx, mineDy, mineDz, visible := float32(0), float32(0), float32(0), float32(0)
	if mineVisible {
		mineDx, mineDy, mineDz, visible = float32(mineX-x), float32(mineY-y), float32(mineZ-z), 1
	}
	ready := float32(0)
	if craftReady {
		ready = 1
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
			ready,
			boolToFloat32(goToActive),
			boolToFloat32(mineActive),
			boolToFloat32(craftActive),
		},
	}
}

// boolToFloat32 converts b to rl.Observation's 1.0/0.0 boolean-flag
// convention, matching every other boolean feature buildObservation
// already produces inline (known, visible, ready above) — factored out
// only for the three goal-conditioning flags, which have no other
// per-feature computation alongside them to inline into.
func boolToFloat32(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
