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
//	index 17: chainLogs — oak logs held, / 8 when normalized.
//	index 18: chainPlanks — oak planks held, / 8 when normalized.
//	index 19: chainTables — crafting tables held (not placed), / 8 when normalized.
//	index 20: chainTablePlaced — 1.0 if this chain episode has placed its table.
//
// indices 17-20 are the multi-step chain block (Config.ChainStage,
// docs/plans/12 in mc-rsi-trainer): all zero outside chain episodes, so
// every other task's observation differs from before only by four trailing
// zeros. The action space also grew by one (ActionPlace); together they
// change the checkpoint environment ID, so old checkpoints are refused
// rather than misread.
//
//	index 21: equipReady — 1.0 if Config.EquipTargetItem is set and
//	          currently equipped (see LiveAgent.IsEquipped), 0.0 otherwise.
//	          Plays the same role for the equip task that craftReady plays
//	          for craft: no position/distance to encode (an item equips
//	          from wherever it is in the inventory), so "is the task already
//	          done" is the useful signal instead — see
//	          docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 1.
//	index 22: goalEquipActive — 1.0 if ActionEquip/ActionUseItem are legal
//	          this episode (Config.EquipTargetItem set), 0.0 otherwise.
//	          Same active-vs-ready distinction as goalMineActive/
//	          goalCraftActive (indices 15-16) have against their own ready
//	          bits, kept here as a trailing pair instead of inside the
//	          original indices 9-16 block since no trained checkpoint
//	          existed yet to make reordering worth avoiding — appending was
//	          simpler and matches how the chain block (17-20) itself was
//	          appended rather than interleaved.
//
//	index 23: pickupDx — nearest visible instance of Config.PickupTargetItem
//	          (resolved via LiveAgent.FindNearestVisibleItemOfType) minus
//	          current X (blocks); 0 if not visible/not configured.
//	index 24: pickupDy — same, Y.
//	index 25: pickupDz — same, Z.
//	index 26: pickupVisible — 1.0 if Config.PickupTargetItem is set and a
//	          visible instance was found this step, 0.0 otherwise. Mirrors
//	          mineVisible's role exactly (indices 9-12) — gates pickupDx/
//	          Dy/Dz the same way, and also doubles as ActionPickUpItem's
//	          legality/mask condition (see action.go's actionLegal), same
//	          dual role mineVisible already plays for ActionMine.
//	index 27: goalPickupActive — 1.0 if ActionPickUpItem is legal this
//	          episode (Config.PickupTargetItem set), 0.0 otherwise. Same
//	          active-vs-ready distinction goalMineActive/goalCraftActive
//	          have against their own ready bits (docs/plans/
//	          RL_ACTION_SPACE_EXPANSION_II.md Phase 2).
//
//	index 28: flyDx — Environment.flyTargetX minus current X (blocks); 0
//	          if Config.FlyTargetActive is false. Unlike mineDx/pickupDx,
//	          always non-zero while active — the flight target is a fixed
//	          point, not something that can be "not currently visible."
//	index 29: flyDy — same, Y. Informational only (altitude difference) —
//	          see Config.FlyTargetOffset's own doc comment on why Y never
//	          drives arrival.
//	index 30: flyDz — same, Z.
//	index 31: isFlying — 1.0 if LiveAgent.IsGliding() reports elytra-
//	          gliding physics currently active, 0.0 otherwise. Reflects
//	          real bot state unconditionally (not gated on
//	          Config.FlyTargetActive) — the bot could be gliding for
//	          reasons unrelated to this episode's own task, and hiding
//	          that would make the observation lie about physical reality.
//	index 32: hasElytra — 1.0 if LiveAgent.IsEquipped("minecraft:elytra"),
//	          0.0 otherwise. Same "real state, ungated" reasoning as
//	          isFlying above.
//	index 33: goalFlyActive — 1.0 if Config.FlyTargetActive, 0.0
//	          otherwise. Same role as goalPickupActive/goalEquipActive
//	          above (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3).
//
//	index 34: rideDx — nearest visible instance of Config.RideVehicleType
//	          (resolved via LiveAgent.FindNearestVisibleEntityOfType)
//	          minus current X (blocks); 0 if not visible/not configured.
//	index 35: rideDy — same, Y.
//	index 36: rideDz — same, Z.
//	index 37: rideVisible — 1.0 if Config.RideVehicleType is set and a
//	          visible instance was found this step, 0.0 otherwise. Mirrors
//	          mineVisible's dual role exactly — gates rideDx/Dy/Dz and
//	          doubles as ActionMount's legality/mask condition.
//	index 38: isMounted — 1.0 if LiveAgent.IsMounted(), 0.0 otherwise.
//	          Reflects real bot state unconditionally (not gated on
//	          Config.RideVehicleType), same reasoning as isFlying/
//	          hasElytra above.
//	index 39: goalRideActive — 1.0 if Config.RideVehicleType is set, 0.0
//	          otherwise. Same role as goalFlyActive/goalPickupActive above
//	          (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 4).
//
//	index 40: combatDx — nearest visible instance of
//	          Config.CombatTargetMobType (resolved via
//	          LiveAgent.FindNearestVisibleEntityOfType) minus current X
//	          (blocks); 0 if not visible/not configured.
//	index 41: combatDy — same, Y.
//	index 42: combatDz — same, Z.
//	index 43: combatTargetHealth — the resolved target's last-known
//	          health (LiveAgent.GetEntityHealth), 0 if not visible/not
//	          configured. Gated the same way mineDx/Dy/Dz are, not
//	          reported unconditionally like isFlying/hasElytra: unlike
//	          those (real bot state, always meaningful), a target's
//	          health is meaningless with no resolved target to attach it
//	          to. The bot's OWN health is already index 5 — this is only
//	          the opponent's.
//	index 44: combatVisible — 1.0 if Config.CombatTargetMobType is set and
//	          a visible instance was found this step, 0.0 otherwise.
//	          Mirrors mineVisible's dual role — gates combatDx/Dy/Dz/
//	          combatTargetHealth and doubles as ActionCombat's legality/
//	          mask condition.
//	index 45: goalCombatActive — 1.0 if Config.CombatTargetMobType is set,
//	          0.0 otherwise. Same role as goalRideActive/goalFlyActive
//	          above (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 5).
//
//	index 46: placeReady — 1.0 if Environment.placeStandaloneReady()
//	          (Config.PlaceTargetItem set, outside a chain episode, and
//	          currently held), 0.0 otherwise. Plays the same role for
//	          standalone placement that craftReady plays for craft — no
//	          position/distance to encode, so "is this attemptable right
//	          now" is the useful signal (docs/plans/
//	          RL_ACTION_SPACE_EXPANSION_III.md item 2).
//	index 47: goalPlaceStandaloneActive — 1.0 if Config.PlaceTargetItem is
//	          set and Config.ChainStage == 0, 0.0 otherwise. Same
//	          active-vs-ready distinction as goalCraftActive/craftReady.
//
//	index 48: followTargetVisible — 1.0 if Config.FollowTargetPlayerName is
//	          set and currently resolvable (CommandAgent.FindPlayerByName),
//	          0.0 otherwise. Mirrors mineVisible's dual role — doubles as
//	          ActionFollow's legality/mask condition. No position features
//	          accompany this (unlike mine/pickup/ride/combat): the target's
//	          own position isn't something the policy acts on directly once
//	          following/manager.go takes over (docs/plans/
//	          RL_ACTION_SPACE_EXPANSION_III.md item 4).
//	index 49: isFollowing — 1.0 if CommandAgent.IsFollowing(), 0.0
//	          otherwise. Reflects real bot state unconditionally (not
//	          gated on Config.FollowTargetPlayerName), same reasoning as
//	          isFlying/hasElytra/isMounted above.
//	index 50: goalFollowActive — 1.0 if Config.FollowTargetPlayerName is
//	          set, 0.0 otherwise. Same role as goalRideActive/
//	          goalCombatActive above.
//
//	index 51: creeperDx — nearest visible creeper's X minus current X
//	          (blocks); 0 if not visible/Config.CreeperGunpowderActive is
//	          false. Unlike CombatTargetMobType, there is no mob-type
//	          field: the search is always for "minecraft:creeper" (see
//	          Config.CreeperGunpowderActive's own doc comment).
//	index 52: creeperDy — same, Y.
//	index 53: creeperDz — same, Z.
//	index 54: creeperVisible — 1.0 if Config.CreeperGunpowderActive is
//	          true and a visible creeper was found this step, 0.0
//	          otherwise. Mirrors combatVisible's dual role — gates
//	          creeperDx/Dy/Dz and doubles as
//	          ActionKillCreeperForGunpowder's legality/mask condition.
//	index 55: goalCreeperGunpowderActive — 1.0 if
//	          Config.CreeperGunpowderActive is true, 0.0 otherwise. Same
//	          role as goalCombatActive/goalRideActive above (docs/plans/
//	          RL_ACTION_SPACE_EXPANSION_III.md item 5).
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
const observationSize = 56

// NormalizedObservation's scales (Config.NormalizeObservation): every feature
// divided by the largest magnitude it normally takes, so they all land near
// [-1, 1]. Distances use 16 blocks, which puts the far tasks' 18-24 block
// targets at 1.1-1.5 and the 8-block mine search radius at 0.5.
const (
	obsDistanceScale = 16
	obsYawScale      = 180
	obsPitchScale    = 90
	obsVitalScale    = 20 // health, food level and saturation all top out at 20
)

// buildObservation constructs the fixed-length feature vector described
// above from the bot's current position/rotation, health/food state,
// (if configured/visible) nearest mine-target block position, (if
// configured) whether the craft target currently looks assembleable, and
// this episode's goal-conditioning block (which of the three tasks are
// actually active — see observationSize's own doc comment on indices
// 14-16). yaw/pitch are float64 to match models.Position.GetPosition's
// return type; every feature in Values is float32 regardless
// (rl.Observation's contract).
func buildObservation(x, y, z float64, yaw, pitch float64, targetX, targetY, targetZ float64, health float32, food int32, saturation float32, healthKnown bool, mineX, mineY, mineZ float64, mineVisible bool, craftReady bool, goToActive, mineActive, craftActive, normalize bool, chain chainObs, equipReady, equipActive bool, pickupX, pickupY, pickupZ float64, pickupVisible, pickupActive bool, flyTargetX, flyTargetY, flyTargetZ float64, flyActive, isFlying, hasElytra bool, rideX, rideY, rideZ float64, rideVisible, isMounted, rideActive bool, combatX, combatY, combatZ float64, combatHealth float32, combatVisible, combatActive bool, placeReady, placeActive bool, followTargetVisible, isFollowing, followActive bool, creeperX, creeperY, creeperZ float64, creeperVisible, creeperActive bool) rl.Observation {
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
	pickupDx, pickupDy, pickupDz, pickupVisibleFlag := float32(0), float32(0), float32(0), float32(0)
	if pickupVisible {
		pickupDx, pickupDy, pickupDz, pickupVisibleFlag = float32(pickupX-x), float32(pickupY-y), float32(pickupZ-z), 1
	}
	flyDx, flyDy, flyDz := float32(0), float32(0), float32(0)
	if flyActive {
		flyDx, flyDy, flyDz = float32(flyTargetX-x), float32(flyTargetY-y), float32(flyTargetZ-z)
	}
	rideDx, rideDy, rideDz, rideVisibleFlag := float32(0), float32(0), float32(0), float32(0)
	if rideVisible {
		rideDx, rideDy, rideDz, rideVisibleFlag = float32(rideX-x), float32(rideY-y), float32(rideZ-z), 1
	}
	combatDx, combatDy, combatDz, combatTargetHealth, combatVisibleFlag := float32(0), float32(0), float32(0), float32(0), float32(0)
	if combatVisible {
		combatDx, combatDy, combatDz, combatTargetHealth, combatVisibleFlag = float32(combatX-x), float32(combatY-y), float32(combatZ-z), combatHealth, 1
	}
	creeperDx, creeperDy, creeperDz, creeperVisibleFlag := float32(0), float32(0), float32(0), float32(0)
	if creeperVisible {
		creeperDx, creeperDy, creeperDz, creeperVisibleFlag = float32(creeperX-x), float32(creeperY-y), float32(creeperZ-z), 1
	}
	if normalize {
		return rl.Observation{
			Values: []float32{
				float32(targetX-x) / obsDistanceScale,
				float32(targetY-y) / obsDistanceScale,
				float32(targetZ-z) / obsDistanceScale,
				float32(yaw) / obsYawScale,
				float32(pitch) / obsPitchScale,
				health / obsVitalScale,
				float32(food) / obsVitalScale,
				saturation / obsVitalScale,
				known,
				mineDx / obsDistanceScale,
				mineDy / obsDistanceScale,
				mineDz / obsDistanceScale,
				visible,
				ready,
				boolToFloat32(goToActive),
				boolToFloat32(mineActive),
				boolToFloat32(craftActive),
				float32(chain.logs) / chainInventoryCountNorm,
				float32(chain.planks) / chainInventoryCountNorm,
				float32(chain.tables) / chainInventoryCountNorm,
				boolToFloat32(chain.placed),
				boolToFloat32(equipReady),
				boolToFloat32(equipActive),
				pickupDx / obsDistanceScale,
				pickupDy / obsDistanceScale,
				pickupDz / obsDistanceScale,
				pickupVisibleFlag,
				boolToFloat32(pickupActive),
				flyDx / obsDistanceScale,
				flyDy / obsDistanceScale,
				flyDz / obsDistanceScale,
				boolToFloat32(isFlying),
				boolToFloat32(hasElytra),
				boolToFloat32(flyActive),
				rideDx / obsDistanceScale,
				rideDy / obsDistanceScale,
				rideDz / obsDistanceScale,
				rideVisibleFlag,
				boolToFloat32(isMounted),
				boolToFloat32(rideActive),
				combatDx / obsDistanceScale,
				combatDy / obsDistanceScale,
				combatDz / obsDistanceScale,
				combatTargetHealth / obsVitalScale,
				combatVisibleFlag,
				boolToFloat32(combatActive),
				boolToFloat32(placeReady),
				boolToFloat32(placeActive),
				boolToFloat32(followTargetVisible),
				boolToFloat32(isFollowing),
				boolToFloat32(followActive),
				creeperDx / obsDistanceScale,
				creeperDy / obsDistanceScale,
				creeperDz / obsDistanceScale,
				creeperVisibleFlag,
				boolToFloat32(creeperActive),
			},
		}
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
			float32(chain.logs),
			float32(chain.planks),
			float32(chain.tables),
			boolToFloat32(chain.placed),
			boolToFloat32(equipReady),
			boolToFloat32(equipActive),
			pickupDx,
			pickupDy,
			pickupDz,
			pickupVisibleFlag,
			boolToFloat32(pickupActive),
			flyDx,
			flyDy,
			flyDz,
			boolToFloat32(isFlying),
			boolToFloat32(hasElytra),
			boolToFloat32(flyActive),
			rideDx,
			rideDy,
			rideDz,
			rideVisibleFlag,
			boolToFloat32(isMounted),
			boolToFloat32(rideActive),
			combatDx,
			combatDy,
			combatDz,
			combatTargetHealth,
			combatVisibleFlag,
			boolToFloat32(combatActive),
			boolToFloat32(placeReady),
			boolToFloat32(placeActive),
			boolToFloat32(followTargetVisible),
			boolToFloat32(isFollowing),
			boolToFloat32(followActive),
			creeperDx,
			creeperDy,
			creeperDz,
			creeperVisibleFlag,
			boolToFloat32(creeperActive),
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

// chainObs is the chain block of an observation (indices 17-20).
type chainObs struct {
	logs, planks, tables int
	placed               bool
}
