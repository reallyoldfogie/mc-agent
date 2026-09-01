package rlenv

import (
	"fmt"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
)

// Action vocabulary. Deliberately small: RL_POLICY_INTEGRATION_PLAN.md
// sketched GO_TO/MINE/CRAFT/RETURN_HOME/WAIT, but mc-agent's action
// registry (actions.NewRegistry, as of this branch's base commit) has no
// mine/craft actions yet, so this environment only maps to what actually
// exists today. Extend this list (and NumActions, mapAction) once more
// mapped capabilities exist — do not add placeholder actions that don't
// dispatch to anything real.
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

	// NumActions is this environment's ActionSpace().
	NumActions = int(ActionReturnHome) + 1
)

// moveToActionName is the registered action name MoveTo (actions/commands.go)
// is keyed under (see actions.actionRegistry.Register: lowercased Name()).
const moveToActionName = "moveto"

// movementTarget resolves action to the (x, y, z) a movement action should
// dispatch toward, and whether action is a movement action at all (false
// for ActionWait, which never reaches the registry).
func (e *Environment) movementTarget(action rl.Action) (x, y, z float64, isMovement bool, err error) {
	switch action {
	case ActionWait:
		return 0, 0, 0, false, nil
	case ActionGoToTarget:
		return e.targetX, e.targetY, e.targetZ, true, nil
	case ActionReturnHome:
		return e.originX, e.originY, e.originZ, true, nil
	default:
		return 0, 0, 0, false, fmt.Errorf("rlenv: action %d out of range [0, %d)", action, NumActions)
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
