package rlenv

import (
	"fmt"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
)

// Action vocabulary. RL_POLICY_INTEGRATION_PLAN.md originally sketched
// GO_TO/MINE/CRAFT/RETURN_HOME/WAIT; ActionReturnHome (RETURN_HOME) was
// dropped (2026-09-09) — it was structurally just ActionGoToTarget with a
// hardcoded destination (Environment.originX/Y/Z instead of targetX/Y/Z,
// same "movetoquiet" dispatch either way — see resolveDispatch's old
// case), no reward component ever referenced origin/"home" at all, and no
// design doc gave it a job beyond being part of the original five-action
// sketch. Its real cost: an untrained policy has a real chance of
// initializing toward whichever action turns out to be reward-irrelevant,
// and every reward-irrelevant action in the vocabulary is pure wasted
// probability mass that a live debugging session traced directly to a
// training run that never learned anything (2026-09-08/09, see
// cRL-go/docs/plans/19-training-time-action-masking.md for the
// longer-term fix, action masking — not yet built). Reintroduce a
// "return to a point" action only alongside an actual reward reason for
// it, not speculatively.
//
// docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1 wired ActionCraft in,
// closing the last gap RL_ACTION_SPACE_EXPANSION.md Phase 3 left open.
// Extend this list (and NumActions, resolveDispatch) if a future
// capability needs a new action — do not add placeholder actions that
// don't dispatch to anything real.
const (
	// ActionWait performs no dispatch: the policy chooses to not move this
	// step. See Environment.Step for why this doesn't block on anything.
	ActionWait rl.Action = iota
	// ActionGoToTarget dispatches "movetoquiet" toward the episode's target
	// position (Environment.targetX/Y/Z, set at Reset — see task.go).
	ActionGoToTarget
	// ActionMine dispatches "mine <x> <y> <z>" at Environment's own
	// already-resolved e.mineX/Y/Z (see refreshMineTarget) — not "mine
	// <Config.MineTargetBlock>" by name, which would make mc-agent's Mine
	// action (actions/commands.go) re-run its own independent
	// FindVisibleBlock search. Found necessary, not merely tidy: that
	// by-name form's own default search radius (32,
	// actions/commands.go's mineSearchRadius) is a plain package constant,
	// entirely disconnected from Config.MineSearchRadius — dispatching by
	// name when nothing is within Config.MineSearchRadius (Environment's
	// own, typically much smaller, search) let the dispatched action
	// re-search a much larger, effectively unbounded volume, which stalled
	// a live RL training run for minutes at a time per mine attempt
	// (2026-09-10). Dispatching Environment's own already-resolved
	// coordinates instead makes this genuinely a single shared search, not
	// two independently-radius'd ones that merely "usually agree." A safe
	// no-op (like ActionWait) when Config.MineTargetBlock is unset OR
	// nothing is currently visible (!e.mineVisible) — this environment
	// poses at most one mining task per instance
	// (RL_ACTION_SPACE_EXPANSION.md Phase 2a option (a), mirrors
	// TargetOffset's "environment poses the task" pattern), not a
	// free-form "mine anything, searching as far as it takes" capability.
	ActionMine
	// ActionCraft dispatches "craft <Config.CraftTargetItem>" — mc-agent's
	// own Craft action (actions/commands.go) resolves ingredient placement
	// itself (agent.CraftItem); Environment separately tracks inventory
	// count/craftability for observation/reward purposes (see
	// Environment.craftCountNow/craftReadyNow). A safe no-op (like
	// ActionWait/an unconfigured ActionMine) when Config.CraftTargetItem is
	// unset — docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1a, mirroring
	// ActionMine's "environment poses the task" pattern exactly.
	ActionCraft

	// NumActions is this environment's ActionSpace().
	NumActions = int(ActionCraft) + 1
)

// moveToActionName, mineActionName, and craftActionName are the registered
// action names MoveToQuiet/Mine/Craft (actions/commands.go) are keyed
// under (see actions.actionRegistry.Register: lowercased Name()).
//
// moveToActionName dispatches to "movetoquiet", not "moveto": found live,
// not anticipated — an RL-driven session dispatching "moveto" every Step
// got kicked from a real server for spamming once rollout collection
// dispatched movement fast enough to repeat "Already at target
// position"/"Navigating..." past vanilla's anti-spam threshold. See
// actions.MoveToQuiet's own doc comment and
// docs/plans/RL_TRAINING_LOOP_PLAN.md.
const (
	moveToActionName = "movetoquiet"
	mineActionName   = "mine"
	craftActionName  = "craft"
)

// actionDispatch describes what Step should send through the
// models.ActionRegistry for one rl.Action: which registered action name to
// call, and the string args that action's Execute expects (see
// actions/commands.go — MoveToQuiet and Mine's coordinate form both need 3
// parseFloat'able coordinates; Craft needs one item name).
type actionDispatch struct {
	name string
	args []string
}

// resolveDispatch maps action to what Step should dispatch through the
// registry this step, and whether it should dispatch anything at all.
// ok=false covers three cases, all silent no-ops rather than errors, the
// same way a task a policy can't currently act on shouldn't punish it for
// trying: ActionWait (never dispatches, by design), ActionMine with no
// configured target (Config.MineTargetBlock == "") or nothing currently
// visible (!e.mineVisible — see ActionMine's own doc comment for why this
// dispatches Environment's own already-resolved coordinates rather than
// redispatching by name), and ActionCraft with no configured target or
// nothing currently craftable (!e.craftReady — until 2026-09-12 this
// second condition was missing here, letting a Craft dispatch reach the
// real registry.Execute call with no ingredients present; CraftItem then
// returns a hard error, failing the whole Step rather than the graceful
// no-op ActionMine already got in the equivalent situation — found live
// via testing/rl_train_test.go's
// TestRLTrainingLoop_LongRunLearnsToConditionOnTaskAvailabilityLive, the
// first test to ever configure CraftTargetItem while craftReady could
// actually be false; every earlier test always seeded ingredients
// unconditionally, so this gap was never exercised). This
// replaces the old movementTarget/isMovement pair
// (RL_ACTION_SPACE_EXPANSION.md Phase 2b) with a single dispatch-table
// shape covering all cases.
func (e *Environment) resolveDispatch(action rl.Action) (dispatch actionDispatch, ok bool, err error) {
	switch action {
	case ActionWait:
		return actionDispatch{}, false, nil
	case ActionGoToTarget:
		return actionDispatch{name: moveToActionName, args: coordArgs(e.targetX, e.targetY, e.targetZ)}, true, nil
	case ActionMine:
		if !e.actionLegal(ActionMine) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: mineActionName, args: coordArgs(e.mineX, e.mineY, e.mineZ)}, true, nil
	case ActionCraft:
		if !e.actionLegal(ActionCraft) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: craftActionName, args: []string{e.cfg.CraftTargetItem}}, true, nil
	default:
		return actionDispatch{}, false, fmt.Errorf("rlenv: action %d out of range [0, %d)", action, NumActions)
	}
}

// actionLegal reports whether action is structurally usable right now —
// the same condition resolveDispatch's ActionMine/ActionCraft cases use to
// decide "safe no-op" (Config.MineTargetBlock/CraftTargetItem unset, or
// nothing currently visible to mine / craftable), factored out so
// ActionMask (below) can't silently drift from what Step actually
// dispatches. ActionWait and
// ActionGoToTarget are always legal — ActionWait is a genuine strategy
// choice (accruing the per-step time penalty on purpose), not
// structurally invalid, even though resolveDispatch also never dispatches
// it; "legal" and "dispatches something" are different questions for that
// one action only.
func (e *Environment) actionLegal(action rl.Action) bool {
	switch action {
	case ActionWait, ActionGoToTarget:
		return true
	case ActionMine:
		return e.cfg.MineTargetBlock != "" && e.mineVisible
	case ActionCraft:
		return e.cfg.CraftTargetItem != "" && e.craftReady
	default:
		return false
	}
}

// ActionMask implements cRL-go's rl.ActionMasker (pkg/rl/rl.go, built
// 2026-09-10 — see docs/plans/19-training-time-action-masking.md in
// ../cRL-go, resolved and implemented there at commit d3a3153): both
// pkg/reinforce and pkg/ppo's rollout loops type-assert Environment
// against this interface automatically and mask pre-softmax logits with
// whatever it returns, with no other wiring needed on this side. Without
// it, an untrained policy has a real chance of wasting probability mass
// on ActionMine/ActionCraft when this Environment instance doesn't pose
// that task (or, for Mine, nothing is currently visible) — exactly the
// training failure that doc's "Purpose / motivation" section traces a
// real run to (2026-09-08/09).
func (e *Environment) ActionMask() []bool {
	mask := make([]bool, NumActions)
	for action := rl.Action(0); int(action) < NumActions; action++ {
		mask[action] = e.actionLegal(action)
	}
	return mask
}

// coordArgs formats x, y, z as the string args actions.MoveToQuiet.Execute
// and Mine's coordinate form both expect (see actions/commands.go:
// parseFloat(args[0..2])).
func coordArgs(x, y, z float64) []string {
	return []string{formatCoord(x), formatCoord(y), formatCoord(z)}
}

func formatCoord(v float64) string {
	return fmt.Sprintf("%.4f", v)
}
