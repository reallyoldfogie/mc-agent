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

	// ActionPlace places the crafting table the bot holds on the ground
	// beside it ("place minecraft:crafting_table" - see actions.Place),
	// confirming it appeared. Only legal in a chain episode (Config.ChainStage
	// >= ChainPlace) holding a table with none placed yet. Appended after
	// the original four so their numbers never change.
	ActionPlace

	// ActionEquip dispatches "equip <Config.EquipTargetItem>" — mc-agent's
	// own Equip action (actions/commands.go) finds the item anywhere in the
	// inventory and readies it (wears armor/elytra, holds anything else).
	// A safe no-op (like an unconfigured ActionMine/ActionCraft) when
	// Config.EquipTargetItem is unset — docs/plans/RL_ACTION_SPACE_EXPANSION_II.md
	// Phase 1, mirroring ActionMine/ActionCraft's "environment poses the
	// task" pattern exactly. Appended after ActionPlace so earlier actions'
	// numbers never change.
	ActionEquip
	// ActionUseItem dispatches "useitem" (or "useitem offhand" if
	// Config.UseItemOffhand) — mc-agent's own UseItem action, a plain
	// right-click interaction with whatever is held. Gated on
	// Config.EquipTargetItem being set, the same as ActionEquip: using an
	// item only makes sense as part of an equip task, not as an
	// independent one (see Config.EquipTargetItem's own doc comment).
	// Deliberately has no reward component of its own — see
	// docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 1's note on why.
	ActionUseItem

	// ActionPickUpItem dispatches "pickupitem <x> <y> <z>" at Environment's
	// own already-resolved e.pickupX/Y/Z (see refreshPickupTarget) — not
	// "pickupitem <itemName>" by name, for the same reason ActionMine
	// dispatches resolved coordinates rather than redispatching by name
	// (see ActionMine's own doc comment): avoids a second, independently-
	// radius'd FindNearestVisibleItemOfType search inside the dispatched
	// action. A safe no-op when Config.PickupTargetItem is unset or
	// nothing is currently visible (!e.pickupVisible) — this environment
	// poses at most one pickup task per instance, mirroring
	// Config.MineTargetBlock's own "environment poses the task" pattern
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 2).
	ActionPickUpItem

	// ActionFlyTo dispatches "flyto <x> <y> <z>" at Environment's own
	// already-resolved e.flyTargetX/Y/Z (computed once at Reset from
	// Config.FlyTargetOffset, mirroring targetX/Y/Z — see
	// Config.FlyTargetOffset's own doc comment for why Y isn't what
	// drives arrival). Unlike ActionMine/ActionPickUpItem there is no
	// independent-search mismatch risk to avoid here: the flight target
	// is a fixed point resolved once, not re-searched every step, so
	// there was never a second resolution to disagree with in the first
	// place. A safe no-op when Config.FlyTargetActive is false (see
	// action.go's resolveDispatch) — docs/plans/
	// RL_ACTION_SPACE_EXPANSION_II.md Phase 3.
	ActionFlyTo

	// ActionMount dispatches "mount <entityID>" at Environment's own
	// already-resolved e.rideEntityID (see refreshRideTarget) — not
	// "mount <entityType>" by name, the same lesson ActionMine's own doc
	// comment already establishes (avoids a second, independently-
	// resolved search). A safe no-op when Config.RideVehicleType is
	// unset, nothing matching is currently visible, or the bot is already
	// mounted (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 4).
	ActionMount
	// ActionDismount dispatches "dismount". Legal only while mounted and
	// Config.RideVehicleType is set.
	ActionDismount
	// ActionVehicleJump dispatches "vehiclejump" (default/max power —
	// this discrete action space doesn't expose a continuous power
	// parameter). Legal only while mounted and Config.RideVehicleType is
	// set, the same condition as ActionDismount.
	ActionVehicleJump

	// ActionCombat dispatches "runcombat <radius> false" — mc-agent's own
	// autonomous combat loop (agent.RunCombat), which ranks and engages
	// targets itself every combat tick rather than being aimed at one
	// resolved entity. Unlike ActionMine/ActionMount, there is no
	// independent-search mismatch to avoid in the dispatch itself (the
	// dispatch never references an entity ID) — Environment's own
	// e.combatEntityID (see refreshCombatTarget) exists purely to judge
	// the reward (did THIS specific resolved entity die), not to steer
	// the dispatch. A safe no-op when Config.CombatTargetMobType is unset
	// or nothing matching is currently visible
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 5). Bot death
	// mid-combat needs no special handling here — see
	// Config.CombatTargetMobType's own doc comment.
	ActionCombat

	// ActionFollow dispatches "follow <name>" — mc-agent's own following
	// manager (following/manager.go), a continuous background behavior
	// (sprint/sneak distance thresholds, stuck recovery, continuous path
	// recalculation toward a moving target) with no natural "done" state,
	// structurally unlike every other task in this package. Legal only
	// when not already following and the named player is currently
	// resolvable — mirrors ActionMount's own gating exactly
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 4). Real
	// precondition, not solvable by this package: a second real connected
	// player (or bot) with Config.FollowTargetPlayerName must already be
	// in the session — see that Config field's own doc comment.
	ActionFollow
	// ActionStopFollow dispatches "stopfollow". Legal only while
	// following and Config.FollowTargetPlayerName is set — mirrors
	// ActionDismount's own gating exactly.
	ActionStopFollow

	// ActionKillCreeperForGunpowder dispatches "killcreeperforgunpowder"
	// (no args) — mc-agent's own complete, hardcoded-to-creepers encounter
	// (agent.KillCreeperForGunpowder), which resolves its own target
	// internally rather than being aimed at one resolved entity, the same
	// shape ActionCombat's own dispatch already has. Environment's own
	// e.creeperEntityID (see refreshCreeperTarget) exists purely to gate
	// legality/feed the observation, not to steer the dispatch. A safe
	// no-op when Config.CreeperGunpowderActive is false, nothing is
	// currently visible, or the required melee weapon isn't equipped
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 5).
	ActionKillCreeperForGunpowder

	// ActionSpearChargeWithApproach dispatches
	// "spearchargewithapproach <entityID> <itemName> <holdMs> <engagedMs>
	// <tiredMs> <minSpeed> <minAlignment>" at Environment's own
	// already-resolved e.spearEntityID (see refreshSpearTarget) — not by
	// mob type, since agent.SpearChargeAtWithApproach needs one specific
	// targetID (unlike RunCombat's own self-ranking loop), the same
	// by-ID-not-by-name reasoning ActionMount's own doc comment gives.
	// itemName and the hold/engaged/tired/speed/alignment values are all
	// fixed constants for a first pass (see spearChargeWeaponName/
	// spearChargeHoldDuration's own doc comments), not Config knobs. A
	// safe no-op when Config.SpearChargeTargetMobType is unset, nothing is
	// currently visible, or the required spear isn't equipped (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_III.md item 6).
	ActionSpearChargeWithApproach

	// ActionMaceSmashWithApproach dispatches
	// "macesmashwithapproach <entityID> <itemName>" at Environment's own
	// already-resolved e.maceEntityID (see refreshMaceTarget) — same
	// by-ID-not-by-name reasoning as ActionSpearChargeWithApproach.
	// itemName is a fixed constant (maceSmashWeaponName), not a Config
	// knob. A safe no-op when Config.MaceSmashTargetMobType is unset,
	// nothing is currently visible, or the required mace isn't equipped
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 7).
	ActionMaceSmashWithApproach

	// NumActions is this environment's ActionSpace().
	NumActions = int(ActionMaceSmashWithApproach) + 1
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
	moveToActionName                  = "movetoquiet"
	mineActionName                    = "mine"
	craftActionName                   = "craft"
	placeActionName                   = "place"
	equipActionName                   = "equip"
	useItemActionName                 = "useitem"
	useItemOffhandArg                 = "offhand"
	pickUpItemActionName              = "pickupitem"
	flyToActionName                   = "flyto"
	mountActionName                   = "mount"
	dismountActionName                = "dismount"
	vehicleJumpActionName             = "vehiclejump"
	combatActionName                  = "runcombat"
	followActionName                  = "follow"
	stopFollowActionName              = "stopfollow"
	moveToAndSneakActionName          = "movetoandsneakquiet"
	killCreeperForGunpowderActionName = "killcreeperforgunpowder"
	spearChargeWithApproachActionName = "spearchargewithapproach"
	maceSmashWithApproachActionName   = "macesmashwithapproach"
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
		if !e.actionLegal(ActionGoToTarget) {
			return actionDispatch{}, false, nil
		}
		// Config.ArriveSneaking (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
		// item 3) is purely a dispatch-time choice — arrival detection
		// itself (distance-based, in Step) is unaffected either way, so
		// there's no separate action/reward for this, just which quiet
		// movement primitive gets dispatched.
		if e.cfg.ArriveSneaking {
			return actionDispatch{name: moveToAndSneakActionName, args: coordArgs(e.targetX, e.targetY, e.targetZ)}, true, nil
		}
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
		if e.cfg.ChainStage > 0 {
			return actionDispatch{name: craftActionName, args: []string{e.chainCraftTarget()}}, true, nil
		}
		return actionDispatch{name: craftActionName, args: []string{e.cfg.CraftTargetItem}}, true, nil
	case ActionPlace:
		if !e.actionLegal(ActionPlace) {
			return actionDispatch{}, false, nil
		}
		if e.cfg.ChainStage > 0 {
			return actionDispatch{name: placeActionName, args: []string{chainTable}}, true, nil
		}
		// Standalone placement (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
		// item 2) — Config.ChainStage's own branch above is unchanged.
		return actionDispatch{name: placeActionName, args: []string{e.cfg.PlaceTargetItem}}, true, nil
	case ActionEquip:
		if !e.actionLegal(ActionEquip) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: equipActionName, args: []string{e.cfg.EquipTargetItem}}, true, nil
	case ActionUseItem:
		if !e.actionLegal(ActionUseItem) {
			return actionDispatch{}, false, nil
		}
		args := []string{}
		if e.cfg.UseItemOffhand {
			args = []string{useItemOffhandArg}
		}
		return actionDispatch{name: useItemActionName, args: args}, true, nil
	case ActionPickUpItem:
		if !e.actionLegal(ActionPickUpItem) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: pickUpItemActionName, args: coordArgs(e.pickupX, e.pickupY, e.pickupZ)}, true, nil
	case ActionFlyTo:
		if !e.actionLegal(ActionFlyTo) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: flyToActionName, args: coordArgs(e.flyTargetX, e.flyTargetY, e.flyTargetZ)}, true, nil
	case ActionMount:
		if !e.actionLegal(ActionMount) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: mountActionName, args: []string{fmt.Sprintf("%d", e.rideEntityID)}}, true, nil
	case ActionDismount:
		if !e.actionLegal(ActionDismount) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: dismountActionName, args: nil}, true, nil
	case ActionVehicleJump:
		if !e.actionLegal(ActionVehicleJump) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: vehicleJumpActionName, args: nil}, true, nil
	case ActionCombat:
		if !e.actionLegal(ActionCombat) {
			return actionDispatch{}, false, nil
		}
		radius := e.cfg.CombatRadius
		if radius <= 0 {
			radius = DefaultConfig().CombatRadius
		}
		return actionDispatch{name: combatActionName, args: []string{fmt.Sprintf("%.4f", radius), "false"}}, true, nil
	case ActionFollow:
		if !e.actionLegal(ActionFollow) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: followActionName, args: []string{e.cfg.FollowTargetPlayerName}}, true, nil
	case ActionStopFollow:
		if !e.actionLegal(ActionStopFollow) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: stopFollowActionName, args: nil}, true, nil
	case ActionKillCreeperForGunpowder:
		if !e.actionLegal(ActionKillCreeperForGunpowder) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: killCreeperForGunpowderActionName, args: nil}, true, nil
	case ActionSpearChargeWithApproach:
		if !e.actionLegal(ActionSpearChargeWithApproach) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: spearChargeWithApproachActionName, args: []string{
			fmt.Sprintf("%d", e.spearEntityID),
			spearChargeWeaponName,
			fmt.Sprintf("%d", spearChargeHoldDuration.Milliseconds()),
			fmt.Sprintf("%d", spearChargeEngagedDuration.Milliseconds()),
			fmt.Sprintf("%d", spearChargeTiredDuration.Milliseconds()),
			fmt.Sprintf("%.4f", spearChargeMinSpeed),
			fmt.Sprintf("%.4f", spearChargeMinAlignment),
		}}, true, nil
	case ActionMaceSmashWithApproach:
		if !e.actionLegal(ActionMaceSmashWithApproach) {
			return actionDispatch{}, false, nil
		}
		return actionDispatch{name: maceSmashWithApproachActionName, args: []string{
			fmt.Sprintf("%d", e.maceEntityID),
			maceSmashWeaponName,
		}}, true, nil
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
	case ActionWait:
		return true
	case ActionGoToTarget:
		if e.cfg.GoToTargetDisabled {
			return false
		}
		// In a go-there-then-mine/craft episode, once the bot has arrived
		// and the mine or craft it came for is possible, going anywhere is
		// only a wasted step. Left legal, a policy that has learned "goto"
		// is the right answer far away keeps choosing it at the target: 3.7
		// and 4.9 goto steps per episode where 1-2 suffice, and a greedy
		// evaluation that never picks anything else stands there until the
		// step limit. Stays legal if the task action isn't ready (the block
		// slipped out of sight, say), so the bot can still re-approach.
		return !(e.compositeGotoTask() && e.arrived && e.taskActionReady())
	case ActionMine:
		return e.cfg.MineTargetBlock != "" && e.mineVisible
	case ActionCraft:
		if e.cfg.ChainStage > 0 {
			return e.chainCraftTarget() != ""
		}
		return e.cfg.CraftTargetItem != "" && e.craftReady
	case ActionPlace:
		if e.cfg.ChainStage > 0 {
			return chainPlaceLegal(e.cfg.ChainStage, e.chainInv, e.chainTablePlaced)
		}
		// Standalone placement (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
		// item 2) — gated on actually holding the target item, the same
		// masking-efficiency reasoning craftReady's own gate uses.
		return e.placeStandaloneReady()
	case ActionEquip, ActionUseItem:
		// Both gated on the same field — see Config.EquipTargetItem's own
		// doc comment for why ActionUseItem has no independent enable flag.
		return e.cfg.EquipTargetItem != ""
	case ActionPickUpItem:
		return e.pickupTaskActive() && e.pickupVisible
	case ActionFlyTo:
		// Gated on an elytra currently being equipped, the same masking-
		// efficiency reasoning ActionCraft's craftReady gate already
		// uses: agent.FlyTo itself already fails gracefully without one
		// (not a crash), but letting the mask stay permissive here would
		// waste probability mass on an attempt that deterministically
		// fails — see docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3.
		return e.cfg.FlyTargetActive && e.agent.IsEquipped(elytraItemName)
	case ActionMount:
		return e.cfg.RideVehicleType != "" && !e.agent.IsMounted() && e.rideVisible
	case ActionDismount, ActionVehicleJump:
		return e.cfg.RideVehicleType != "" && e.agent.IsMounted()
	case ActionCombat:
		return e.cfg.CombatTargetMobType != "" && e.combatVisible
	case ActionFollow:
		return e.cfg.FollowTargetPlayerName != "" && !e.agent.IsFollowing() && e.followTargetVisible
	case ActionStopFollow:
		return e.cfg.FollowTargetPlayerName != "" && e.agent.IsFollowing()
	case ActionKillCreeperForGunpowder:
		// Gated on the required melee weapon currently equipped, the same
		// masking-efficiency reasoning ActionFlyTo's elytra gate uses:
		// agent.KillCreeperForGunpowder itself already fails gracefully
		// without one (not a crash, see agent.validateCreeperWeapon), but
		// letting the mask stay permissive here would waste probability
		// mass on an attempt that deterministically fails.
		return e.cfg.CreeperGunpowderActive && e.creeperVisible && e.agent.IsEquipped(creeperGunpowderWeaponName)
	case ActionSpearChargeWithApproach:
		// Gated on the required spear currently equipped, the same
		// masking-efficiency reasoning ActionKillCreeperForGunpowder's own
		// weapon gate uses.
		return e.cfg.SpearChargeTargetMobType != "" && e.spearVisible && e.agent.IsEquipped(spearChargeWeaponName)
	case ActionMaceSmashWithApproach:
		// Gated on the required mace currently equipped, the same
		// masking-efficiency reasoning ActionSpearChargeWithApproach's own
		// weapon gate uses.
		return e.cfg.MaceSmashTargetMobType != "" && e.maceVisible && e.agent.IsEquipped(maceSmashWeaponName)
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
