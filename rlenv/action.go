package rlenv

import (
	"fmt"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
)

// Action vocabulary. RL_POLICY_INTEGRATION_PLAN.md sketched
// GO_TO/MINE/CRAFT/RETURN_HOME/WAIT; this environment now maps all five —
// docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1 wired ActionCraft in, closing
// the last gap RL_ACTION_SPACE_EXPANSION.md Phase 3 left open. Extend this
// list (and NumActions, resolveDispatch) if a future capability needs a new
// action — do not add placeholder actions that don't dispatch to anything
// real.
const (
	// ActionWait performs no dispatch: the policy chooses to not move this
	// step. See Environment.Step for why this doesn't block on anything.
	ActionWait rl.Action = iota
	// ActionGoToTarget dispatches "moveto" toward the episode's target
	// position (Environment.targetX/Y/Z, set at Reset — see task.go).
	ActionGoToTarget
	// ActionReturnHome dispatches "moveto" back toward the position the
	// bot was at when Reset was called (Environment.originX/Y/Z).
	ActionReturnHome
	// ActionMine dispatches "mine <Config.MineTargetBlock>" — mc-agent's
	// own Mine action (actions/commands.go) resolves the nearest visible
	// instance of that block name itself via FindVisibleBlock, the same
	// call Environment separately makes for observation/reward purposes
	// (see Environment.resolveMineTarget) — nothing moves between those two
	// calls within one Step, so they agree. A safe no-op (like ActionWait)
	// when Config.MineTargetBlock is unset: this environment poses at most
	// one mining task per instance (RL_ACTION_SPACE_EXPANSION.md Phase 2a
	// option (a), mirrors TargetOffset's "environment poses the task"
	// pattern), not a free-form "mine anything" capability.
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
// action names MoveTo/Mine/Craft (actions/commands.go) are keyed under (see
// actions.actionRegistry.Register: lowercased Name()).
const (
	moveToActionName = "moveto"
	mineActionName   = "mine"
	craftActionName  = "craft"
)

// actionDispatch describes what Step should send through the
// models.ActionRegistry for one rl.Action: which registered action name to
// call, and the string args that action's Execute expects (see
// actions/commands.go — MoveTo needs 3 parseFloat'able coordinates, Mine's
// single-arg form needs one block name).
type actionDispatch struct {
	name string
	args []string
}

// resolveDispatch maps action to what Step should dispatch through the
// registry this step, and whether it should dispatch anything at all.
// ok=false covers two cases: ActionWait (never dispatches, by design) and
// ActionMine with no configured target (Config.MineTargetBlock == "") —
// both are silent no-ops, not errors, the same way an unconfigured mine
// task shouldn't punish a policy for trying it. This replaces the old
// movementTarget/isMovement pair (RL_ACTION_SPACE_EXPANSION.md Phase 2b):
// ActionMine's args aren't a movement target at all, so a single
// dispatch-table shape covers both cases better than the old "give me an
// (x,y,z)" signature could.
func (e *Environment) resolveDispatch(action rl.Action) (dispatch actionDispatch, ok bool, err error) {
	switch action {
	case ActionWait:
		return actionDispatch{}, false, nil
	case ActionGoToTarget:
		return actionDispatch{name: moveToActionName, args: moveToArgs(e.targetX, e.targetY, e.targetZ)}, true, nil
	case ActionReturnHome:
		return actionDispatch{name: moveToActionName, args: moveToArgs(e.originX, e.originY, e.originZ)}, true, nil
	case ActionMine:
		if e.cfg.MineTargetBlock == "" {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: mineActionName, args: []string{e.cfg.MineTargetBlock}}, true, nil
	case ActionCraft:
		if e.cfg.CraftTargetItem == "" {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: craftActionName, args: []string{e.cfg.CraftTargetItem}}, true, nil
	default:
		return actionDispatch{}, false, fmt.Errorf("rlenv: action %d out of range [0, %d)", action, NumActions)
	}
}

// moveToArgs formats x, y, z as the string args actions.MoveTo.Execute
// expects (see actions/commands.go: parseFloat(args[0..2])).
func moveToArgs(x, y, z float64) []string {
	return []string{formatCoord(x), formatCoord(y), formatCoord(z)}
}

func formatCoord(v float64) string {
	return fmt.Sprintf("%.4f", v)
}
