package rlenv

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
	"github.com/reallyoldfogie/mc-agent/models"
)

// Environment wraps a live bot session (LiveAgent) plus its action
// registry to satisfy cRL-go's rl.Environment, per
// RL_POLICY_INTEGRATION_PLAN.md item 5. One Environment owns one bot
// session; per that document's scope item 6, callers should drive it with
// pkg/reinforce/pkg/ppo's PersistentEnvFactory path (Workers=1, one
// long-lived instance reused across episodes via Reset), not the
// per-episode EnvFactory path the toy environments use — constructing a
// live bot session per rollout would be far too expensive, and nothing
// about a live session is safe to run concurrently with another instance
// sharing the same bot.
type Environment struct {
	agent    LiveAgent
	registry models.ActionRegistry[models.CommandAgent]
	cfg      Config

	targetX, targetY, targetZ float64
	prevDistance              float64
	prevHealth                float32
	prevHealthKnown           bool
	episodeStarted            bool
	// lastDoneReason explains the most recent Step that returned Done=true.
	// It is intentionally separate from rl.StepResult, whose public shape is
	// only observation/reward/done. Callers such as the trainer can use this
	// diagnostic without changing cRL-go's interface.
	lastDoneReason string

	// minedBlocks are the blocks this environment's mined-target detection
	// has seen broken since the last Reset, restored at the start of the
	// next one (see BlockRestorer).
	minedBlocks []minedBlock

	// inflight is the last dispatched action while it may still be running
	// (a step that timed out leaves it going); see cancelInflight.
	inflight *inflightAction

	// consecutiveStepTimeouts backs Config.MaxConsecutiveStepTimeouts.
	consecutiveStepTimeouts int

	// arrived is whether this episode's goto target has been reached yet;
	// only consulted when arrival doesn't end the episode (see
	// compositeGotoTask).
	arrived bool

	// chainState/chainInv/chainTablePlaced/chainAnchor back Config.ChainStage
	// (chain.go). chainInv and chainTablePlaced cache the inventory and table
	// state as of the last Step or Reset (refreshChain), read by the action
	// mask and the craft resolver the way craftReady is; chainState remembers
	// which milestones have paid; chainAnchor is the tree's base, kept to
	// clear its stray drops at the next Reset.
	chainState       chainState
	chainInv         chainInventory
	chainTablePlaced bool
	// placeAttempted is whether this episode has dispatched a place action.
	// Only then can a table beside the bot be one it put there: a table the
	// craft tasks seeded, or another bot placed, is not (found live, in the
	// first minutes of a parallel run: a table already standing near the
	// bot paid the placement bonus after a single wait).
	placeAttempted bool
	placedAt       *farSeededBlock
	chainAnchor    *farSeededBlock

	// farSeeded are the blocks seedFarTargets placed near this episode's
	// goto target, removed at the next Reset (they sit outside
	// Config.ClearAreaRadius).
	farSeeded []farSeededBlock

	// mineX/Y/Z/mineVisible cache the last-resolved nearest visible
	// Config.MineTargetBlock instance (see resolveMineTarget), the same
	// way prevDistance/prevHealth cache the last-known state of their own
	// features — refreshed once per Step (and once at Reset) rather than
	// re-resolved multiple times within one call.
	mineX, mineY, mineZ float64
	mineVisible         bool

	// mineKnownX/Y/Z/mineEverSeen are the minimum-viable "memory map"
	// (docs/plans/14-buried-block-mining-and-memory-map.md, mc-rsi-trainer):
	// the last position Config.MineTargetBlock was actually confirmed
	// visible at THIS episode, kept even once e.mineX/Y/Z/mineVisible
	// themselves revert to "not currently visible" because line-of-sight
	// was lost (walked past, turned away, started digging into it so the
	// bot's own body blocks the ray, ...). Read only by buildObservation
	// (resolveDispatch/actionLegal deliberately keep reading the live
	// e.mineX/Y/Z/mineVisible, unchanged): a target glimpsed once stays a
	// navigable goal for the rest of the episode instead of vanishing from
	// the policy's input the instant current-step LOS is lost, without
	// changing which actions are legal or what coordinates an actual Mine
	// dispatch targets. Cleared at Reset (see resetAttempt) — carrying it
	// across episodes would let a later episode's policy "remember" a
	// position from a world state that's since been restored/reseeded.
	mineKnownX, mineKnownY, mineKnownZ float64
	mineEverSeen                       bool

	// mineTargetNameInUse is whichever block name the most recent
	// refreshMineTarget call actually resolved against (see
	// mineTargetBlockInUse) — tracked so a change mid-episode (a
	// ChainMine-stage chain switching from the gathered log to the buried
	// block once its goal item is crafted) can detect itself and clear
	// mineKnownX/Y/Z/mineEverSeen: that remembered position belonged to
	// the *old* target and would mislead the policy (and the observation's
	// mineDx/Dy/Dz) if carried over to the new one.
	mineTargetNameInUse string

	// craftCount caches the last-known held count of Config.CraftTargetItem
	// (see craftCountNow), read at the start of each Step as "before this
	// step" state — the craft analogue of prevDistance/prevHealth's caching
	// shape, simpler than mine's since there's no position to resolve, just
	// a count. craftReady caches the last-computed Craftable() result, for
	// the observation returned to the caller (see craftReadyNow).
	craftCount int
	craftReady bool

	// equipReady caches the last-computed LiveAgent.IsEquipped(Config.
	// EquipTargetItem) result — the equip-task analogue of craftReady,
	// read at the start of each Step as "before this step" state (see
	// Step's equippedThisStep check) and for the observation returned to
	// the caller.
	equipReady bool

	// pickupX/Y/Z/pickupVisible cache the last-resolved nearest visible
	// Config.PickupTargetItem (or, for Config.PickupAnyItem, whatever's
	// nearest regardless of content) instance (see refreshPickupTarget) —
	// the pickup-task analogue of mineX/Y/Z/mineVisible, same caching shape
	// and same reason (refreshed once per Step/Reset, read as "before this
	// step" state). pickupEntityID additionally backs PickupAnyItem's own
	// reward check (see Step's pickedUpThisStep) — an entity-ID-based
	// "still tracked" check, the only reward shape available when there's
	// no single item name to run an InventoryCount delta against
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 1).
	pickupX, pickupY, pickupZ float64
	pickupEntityID            int32
	pickupVisible             bool

	// pickupCount caches the last-known held count of
	// Config.PickupTargetItem (see pickupCountNow) — the pickup-task
	// analogue of craftCount, read at the start of each Step as "before
	// this step" state for the reward check (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_II.md Phase 2).
	pickupCount int

	// placeCount caches the last-known held count of
	// Config.PlaceTargetItem (see placeCountNow) — the standalone-
	// placement-task analogue of craftCount, except judged on a
	// *decrease* (placing consumes one unit) rather than an increase
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 2).
	placeCount int

	// flyTargetX/Y/Z is this episode's elytra-flight target, computed
	// once at Reset from Config.FlyTargetOffset (plus jitter) — a fixed
	// point, unlike mine/pickup's live-resolved targets, so there is
	// nothing to re-resolve per Step the way refreshMineTarget/
	// refreshPickupTarget do (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md
	// Phase 3).
	flyTargetX, flyTargetY, flyTargetZ float64

	// rideEntityID/rideX/Y/Z/rideVisible cache the last-resolved nearest
	// visible Config.RideVehicleType instance (see refreshRideTarget) —
	// the ride-task analogue of mineX/Y/Z/mineVisible, same caching shape
	// and reason, plus the entity ID ActionMount dispatches against
	// (mount <entityID>, not by type name — see ActionMount's own doc
	// comment).
	rideEntityID        int32
	rideX, rideY, rideZ float64
	rideVisible         bool

	// mounted caches the last-computed LiveAgent.IsMounted() result — the
	// ride-task analogue of equipReady, read at the start of each Step as
	// "before this step" state for the reward transition check
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 4).
	mounted bool

	// combatEntityID/combatX/Y/Z/combatVisible cache the last-resolved
	// nearest visible Config.CombatTargetMobType instance (see
	// refreshCombatTarget) — the combat-task analogue of mineX/Y/Z/
	// mineVisible, same caching shape. Unlike rideEntityID, the entity ID
	// here is never dispatched against (ActionCombat's own dispatch
	// never references it — see its own doc comment); it exists purely
	// so Step's reward check can ask "did THIS specific entity die,"
	// across the gap between resolving it and the dispatched combat
	// loop's own effect.
	combatEntityID            int32
	combatX, combatY, combatZ float64
	combatVisible             bool

	// creeperEntityID/creeperX/Y/Z/creeperVisible cache the last-resolved
	// nearest visible creeper instance (see refreshCreeperTarget) — the
	// creeper-gunpowder-task analogue of combatEntityID/combatX/Y/Z/
	// combatVisible, same caching shape (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_III.md item 5). Unlike combatEntityID,
	// never dispatched against: ActionKillCreeperForGunpowder dispatches
	// "killcreeperforgunpowder" with no args (agent.KillCreeperForGunpowder
	// resolves its own target internally) — this exists purely to gate
	// ActionKillCreeperForGunpowder's legality/mask and feed the
	// observation's distance features.
	creeperEntityID              int32
	creeperX, creeperY, creeperZ float64
	creeperVisible               bool

	// gunpowderCount caches the last-known held count of
	// "minecraft:gunpowder" (see gunpowderCountNow) — the creeper-
	// gunpowder-task analogue of craftCount, read at the start of each
	// Step as "before this step" state for the reward check.
	gunpowderCount int

	// spearEntityID/spearX/Y/Z/spearVisible cache the last-resolved
	// nearest visible Config.SpearChargeTargetMobType instance (see
	// refreshSpearTarget) — the spear-charge-task analogue of
	// combatEntityID/combatX/Y/Z/combatVisible, same caching shape (docs/
	// plans/RL_ACTION_SPACE_EXPANSION_III.md item 6). Unlike combatEntityID,
	// this one IS dispatched against: ActionSpearChargeWithApproach needs
	// one specific targetID (agent.SpearChargeAtWithApproach has no
	// self-ranking loop of its own the way RunCombat does).
	spearEntityID          int32
	spearX, spearY, spearZ float64
	spearVisible           bool

	// followTargetVisible/followed cache Config.FollowTargetPlayerName's
	// own resolvability (see refreshFollowTarget) and the last-computed
	// CommandAgent.IsFollowing() result — the follow-task analogues of
	// mineVisible/mounted, same caching shape (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_III.md item 4). No position is cached: the
	// target's own position isn't something the policy acts on directly
	// once following/manager.go takes over.
	followTargetVisible bool
	followed            bool

	// lastObsValues/stepsWithoutObservationChange back Config.StuckTimeout:
	// lastObsValues is the previous Step's (or Reset's) observation vector,
	// stepsWithoutObservationChange counts how many consecutive Steps have
	// reproduced it bit-for-bit. Both reset to their zero value at Reset,
	// same as prevDistance/prevHealth/mineVisible/craftReady above.
	lastObsValues                 []float32
	stepsWithoutObservationChange int

	// rng backs Config.Jitter — one per Environment, seeded once in New
	// from Config.JitterSeed, not reseeded per Reset (successive Resets
	// must draw different jitter values from each other, which reseeding
	// to the same JitterSeed every time would defeat). Also shared with
	// Config.TaskSelector — see its own doc comment for why.
	rng *rand.Rand

	// episode counts how many times Reset has been called on this
	// instance, 0-indexed. Reset captures its current value into a local
	// episodeIndex, increments it, then passes episodeIndex (not this
	// field) to Config.TaskSelector on every resetAttempt call that
	// external Reset call makes — see Reset's own doc comment for why
	// that indirection matters: without it, a Reset call that internally
	// retries would let TaskSelector see this field already incremented
	// past what "0 for the very first episode" promises. Never reset
	// itself; an Environment's episode count only ever grows across its
	// whole lifetime.
	episode int
}

const selfPreservationDistance = 3.0

// moveAwayFromDamage gives a live agent an immediate escape attempt when its
// health has fallen since the previous environment step. This is primarily
// for environmental damage such as suffocation: the policy's next action may
// be ActionWait, or may keep targeting the same blocked location, while the
// server continues applying damage. The normal reward still charges the
// health loss.
func (e *Environment) moveAwayFromDamage(ctx context.Context) {
	if !e.prevHealthKnown {
		return
	}
	health, _, _, known := e.agent.Health()
	if !known || health <= 0 || health >= e.prevHealth {
		return
	}
	pos, _, _, ok := e.agent.GetPosition()
	if !ok {
		return
	}
	moveCtx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	e.cancelInflight()
	for _, target := range [][3]float64{
		{pos.X + selfPreservationDistance, pos.Y, pos.Z},
		{pos.X, pos.Y, pos.Z + selfPreservationDistance},
	} {
		if err := e.agent.MoveTo(moveCtx, target[0], target[1], target[2], false); err == nil {
			return
		}
		if moveCtx.Err() != nil {
			return
		}
	}
}

// New constructs an Environment. registry is typically actions.NewRegistry()
// (or a test double registering only what's needed); it must dispatch
// "movetoquiet" the way actions.MoveToQuiet does (parseFloat'd x/y/z args,
// no chat narration — see that action's own doc comment on why this
// environment uses the quiet variant, not "moveto") for ActionGoToTarget to
// work.
func New(agent LiveAgent, registry models.ActionRegistry[models.CommandAgent], cfg Config) (*Environment, error) {
	if agent == nil {
		return nil, errNilAgent
	}
	if registry == nil {
		return nil, errNilRegistry
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Environment{agent: agent, registry: registry, cfg: cfg, rng: rand.New(rand.NewSource(cfg.JitterSeed))}, nil
}

// ObservationSize implements rl.Environment.
func (e *Environment) ObservationSize() int { return observationSize }

// ActionSpace implements rl.Environment.
func (e *Environment) ActionSpace() int { return NumActions }

// Reset implements rl.Environment, retrying resetAttempt up to
// resetOuterRetryAttempts times before giving up. resetAttempt's own
// walkability retry loop already tolerates "this particular jittered
// target didn't pan out, try another" within one attempt (bounded by
// resetWalkabilityBudget); this outer layer instead tolerates "the
// attempt as a whole hit a bad outcome" - a flaky TeleportTo, or an
// unlucky run of jitter draws that each needed the full ring search
// and burned the whole aggregate budget - without crashing potentially
// hours of otherwise-healthy training over one unlucky episode
// boundary. Confirmed live as a real failure mode, not a hypothetical
// one: see docs/bugs/pathfinder-shared-block-cache-contention.md and
// docs/bugs/movement-adjacency-check-floating-point-drift.md in
// ../mc-agent for the pathfinding-side bugs that made this loop run
// far longer than intended before either was fixed.
//
// e.episode is incremented exactly once per external Reset call here,
// not once per internal resetAttempt call - resetAttempt reads
// e.episode (for Config.TaskSelector) but never mutates it, so a
// failed-then-retried attempt doesn't silently skip an episode index
// out from under TaskSelector's own "0 for the very first episode,
// monotonic thereafter" contract.
//
// Every resetAttempt failure is retried blindly, including ones that
// can never actually succeed on retry (e.g. errResetOriginRequiresResetAgent,
// a live agent that will never grow the missing capability mid-run):
// deliberately not special-cased, since those fail near-instantly
// anyway and classifying every possible error as retryable-or-not would
// add real complexity for negligible savings. ctx cancellation is the
// one exception - retrying past the caller's own cancellation would be
// pure waste.
func (e *Environment) Reset(ctx context.Context) (rl.Observation, error) {
	e.lastDoneReason = ""
	// episodeIndex is captured once, before any retrying, and passed
	// explicitly to every resetAttempt call below rather than letting it
	// read e.episode itself - this is what keeps TaskSelector's "0 for the
	// very first episode, monotonic thereafter" contract intact regardless
	// of how many internal attempts a given external Reset call takes.
	episodeIndex := e.episode
	e.episode++

	var lastErr error
	for attempt := 1; attempt <= resetOuterRetryAttempts; attempt++ {
		obs, err := e.resetAttempt(ctx, episodeIndex)
		if err == nil {
			return obs, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return rl.Observation{}, fmt.Errorf("rlenv: Reset failed after %d attempt(s): %w", resetOuterRetryAttempts, lastErr)
}

// resetAttempt is Reset's single-attempt body. If Config.ResetOrigin is
// set, it actually teleports the bot there first (see ResetAgent);
// otherwise it falls back to TargetOffset's original workaround —
// capturing wherever the bot currently is as this episode's origin and
// posing a new target relative to it, without repositioning anything.
// Either way, the posed target is then checked against
// Config.ArrivalThreshold (see the Config.Jitter retry loop below) so an
// episode never starts already "arrived" — see Config.Jitter's own doc
// comment for why that's a real risk once jitter is in play, not a
// hypothetical one. episodeIndex is Reset's own captured episode counter
// (see its doc comment), passed through unchanged for Config.TaskSelector.
func (e *Environment) resetAttempt(ctx context.Context, episodeIndex int) (rl.Observation, error) {
	e.cancelInflight()
	e.dismountForReset()
	e.stopSneakForReset()
	e.clearRideVehicle(ctx)
	e.restoreMinedBlocks(ctx)
	e.removeFarSeeded(ctx)
	e.clearChainDrops(ctx)
	if e.cfg.CloneAreaFrom != nil {
		e.cloneArea(ctx)
	} else {
		e.clearArea(ctx)
	}
	e.chainState, e.chainInv, e.chainTablePlaced, e.placedAt, e.placeAttempted = chainState{}, chainInventory{}, false, nil, false

	pos, yaw, pitch, ok := e.agent.GetPosition()
	if !ok {
		return rl.Observation{}, errPositionUnknown
	}
	x, y, z := pos.X, pos.Y, pos.Z

	if e.cfg.ResetOrigin != nil {
		resetAgent, ok := e.agent.(ResetAgent)
		if !ok {
			return rl.Observation{}, errResetOriginRequiresResetAgent
		}
		origin := *e.cfg.ResetOrigin
		for i := range origin {
			origin[i] = e.jitter(origin[i], i)
		}
		if err := resetAgent.TeleportTo(ctx, origin[0], origin[1], origin[2]); err != nil {
			return rl.Observation{}, fmt.Errorf("rlenv: resetting to origin: %w", err)
		}
		pos, yaw, pitch, ok = e.agent.GetPosition()
		if !ok {
			return rl.Observation{}, errPositionUnknown
		}
		x, y, z = pos.X, pos.Y, pos.Z

		// The origin's Y was chosen somewhere else (a captured spawn, a
		// working-area copy of it hundreds of blocks away, an x/z jitter
		// off it) and can be inside terrain here: the bot then suffocates
		// and dies, and its respawn scores as thousands of blocks of
		// "movement". TeleportTo has just waited for this chunk to load,
		// so the terrain is now checkable - if the bot isn't standing
		// somewhere walkable, move it to the nearest Y that is.
		if walkAgent, ok := e.agent.(WalkabilityAgent); ok {
			world, shapeMgr := walkAgent.GetWorld(), walkAgent.BlockShapeManager()
			if world != nil && shapeMgr != nil {
				if snapped, found := groundSnap(world, shapeMgr, x, y, z); found && snapped != y {
					if err := resetAgent.TeleportTo(ctx, x, snapped, z); err != nil {
						return rl.Observation{}, fmt.Errorf("rlenv: moving out of terrain after reset: %w", err)
					}
					pos, yaw, pitch, ok = e.agent.GetPosition()
					if !ok {
						return rl.Observation{}, errPositionUnknown
					}
					x, y, z = pos.X, pos.Y, pos.Z
				}
			}
		}
	}

	e.stepsWithoutObservationChange = 0
	e.consecutiveStepTimeouts = 0
	e.arrived = false
	e.mineKnownX, e.mineKnownY, e.mineKnownZ, e.mineEverSeen = 0, 0, 0, false
	e.mineTargetNameInUse = ""

	// Config.TaskSelector: choose this episode's active task(s), if the
	// caller opted in. Deliberately after the ResetOrigin teleport above
	// (TaskSelector doesn't influence where the bot resets to) and before
	// targetOffset is read just below (TaskSelector's TargetOffset override
	// must be in place before that read, or GoToTarget episodes would still
	// jitter/pose the stale pre-override target). See Config.TaskSelector's
	// own doc comment for exactly which fields this mutates.
	if e.cfg.TaskSelector != nil {
		override := e.cfg.TaskSelector(episodeIndex, e.rng)
		e.cfg.GoToTargetDisabled = override.GoToTargetDisabled
		e.cfg.ArriveSneaking = override.ArriveSneaking
		e.cfg.TargetOffset = override.TargetOffset
		e.cfg.MineTargetBlock = override.MineTargetBlock
		e.cfg.MineSearchRadius = override.MineSearchRadius
		e.cfg.MineRequiredTool = override.MineRequiredTool
		e.cfg.MineTargetBuried = override.MineTargetBuried
		e.cfg.CraftTargetItem = override.CraftTargetItem
		e.cfg.CraftSearchRadius = override.CraftSearchRadius
		e.cfg.SeedAtGoal = override.SeedAtGoal
		e.cfg.CollectDrops = override.CollectDrops
		e.cfg.ChainStage = override.ChainStage
		e.cfg.ChainMineTargetBlock = override.ChainMineTargetBlock
		e.cfg.EquipTargetItem = override.EquipTargetItem
		e.cfg.PickupTargetItem = override.PickupTargetItem
		e.cfg.PickupSearchRadius = override.PickupSearchRadius
		e.cfg.PickupAnyItem = override.PickupAnyItem
		e.cfg.PlaceTargetItem = override.PlaceTargetItem
		e.cfg.FlyTargetOffset = override.FlyTargetOffset
		e.cfg.FlyTargetActive = override.FlyTargetActive
		e.cfg.FlyArrivalRadius = override.FlyArrivalRadius
		e.cfg.RideVehicleType = override.RideVehicleType
		e.cfg.RideSearchRadius = override.RideSearchRadius
		e.cfg.CombatTargetMobType = override.CombatTargetMobType
		e.cfg.CombatRadius = override.CombatRadius
		e.cfg.CreeperGunpowderActive = override.CreeperGunpowderActive
		e.cfg.CreeperGunpowderRadius = override.CreeperGunpowderRadius
		e.cfg.SpearChargeTargetMobType = override.SpearChargeTargetMobType
		e.cfg.SpearChargeRadius = override.SpearChargeRadius
		e.cfg.FollowTargetPlayerName = override.FollowTargetPlayerName
	}

	targetOffset := e.cfg.TargetOffset
	for i := range targetOffset {
		targetOffset[i] = e.jitter(targetOffset[i], i)
	}

	// Ground-snap/repair the computed target if the live agent can tell us
	// about terrain (WalkabilityAgent) and the goto task is actually active
	// this episode — skipped when GoToTargetDisabled, since an inactive
	// goto task's target is never walked to and checking it would just be
	// wasted work (and could spuriously error a mine/craft-only episode
	// whose TargetOffset happens to be unset/zero). See
	// findWalkableTarget's own doc comment (walkability.go) for the bug
	// this closes: TargetOffset is a flat 3D offset with no idea what
	// terrain is at the far end, so on real (non-flat) worlds it can land
	// somewhere the pathfinder can never reach, silently producing a
	// zero-progress episode instead of a loud error.
	walkAgent, checkWalkability := e.agent.(WalkabilityAgent)
	checkWalkability = checkWalkability && !e.cfg.GoToTargetDisabled
	jitterActive := e.cfg.Jitter != ([3]float64{})

	// walkabilityCtx bounds the *whole* retry loop below with one
	// aggregate deadline, not just each individual reachable() call.
	// Without this, the loop's own worst case is maxJitterRetries drawn
	// targets, each potentially sweeping up to (2*horizontalSearchRadius+1)^2-1
	// ring candidates (findWalkableTarget), each candidate's own
	// reachable() bounded only by reachabilityCheckTimeout in isolation —
	// 20 * 80 * 3s ≈ 81 minutes in the worst case, with nothing to stop
	// it short of that. Confirmed live against a shared-server training
	// run's deliberately-flattened world: nearly every nearby column
	// trivially passes findWalkableTarget's cheap groundSnap check, so a
	// bot whose actual position is (for whatever reason) reachability-
	// disconnected from its own surroundings burns the full per-candidate
	// timeout on every single one of them, repeatedly, well past what a
	// single Reset call should ever cost — see
	// docs/bugs/reset-walkability-retry-storm.md. context.WithTimeout's
	// own "earlier of the two deadlines" semantics mean reachable's
	// existing per-candidate context.WithTimeout(ctx, ...) call
	// automatically inherits whichever fires first, so no other code path
	// needs to change to get this bound enforced.
	walkabilityCtx := ctx
	if checkWalkability {
		var cancelWalkabilityCtx context.CancelFunc
		walkabilityCtx, cancelWalkabilityCtx = context.WithTimeout(ctx, resetWalkabilityBudget)
		defer cancelWalkabilityCtx()
	}

	// Draw (and, if Jitter is active, redraw) a target offset until one
	// both clears ArrivalThreshold and — if checkWalkability — passes
	// findWalkableTarget, or maxJitterRetries is exhausted. Both checks
	// share one retry budget: each is just "this particular draw didn't
	// work out, try another," not fundamentally different problems, and a
	// bad Config (offset/terrain combination with no usable draw at all)
	// should still fail loudly rather than retrying forever either way.
	// Without Jitter there's no randomness to retry with, so a single
	// deterministic attempt is definitive — matches pre-retry-loop
	// behavior exactly.
	tooClose, walkableFound := false, true
	for attempt := 0; ; attempt++ {
		e.targetX = x + targetOffset[0]
		e.targetY = y + targetOffset[1]
		e.targetZ = z + targetOffset[2]

		tooClose = jitterActive && distance3(x, y, z, e.targetX, e.targetY, e.targetZ) <= e.cfg.ArrivalThreshold
		walkableFound = true
		if !tooClose && checkWalkability {
			var wx, wy, wz float64
			if e.seedAtGoal() {
				wx, wy, wz, walkableFound = findGroundedTarget(walkabilityCtx, walkAgent, e.targetX, e.targetY, e.targetZ)
			} else {
				wx, wy, wz, walkableFound = findWalkableTarget(walkabilityCtx, walkAgent, e.agent, e.targetX, e.targetY, e.targetZ)
			}
			if walkableFound {
				e.targetX, e.targetY, e.targetZ = wx, wy, wz
			}
			if !walkableFound && walkabilityCtx.Err() != nil {
				return rl.Observation{}, fmt.Errorf("rlenv: walkability retry loop exceeded its %s aggregate time budget on attempt %d/%d (last tried (%.1f,%.1f,%.1f)) — check Config.TargetOffset/Jitter/terrain, or whether the bot's own position is reachability-disconnected from its surroundings", resetWalkabilityBudget, attempt, maxJitterRetries, e.targetX, e.targetY, e.targetZ)
			}
		}
		if !tooClose && walkableFound {
			break
		}
		if !jitterActive {
			return rl.Observation{}, fmt.Errorf("rlenv: no walkable+reachable cell found near goto target (%.1f,%.1f,%.1f) within %d blocks vertically / %d horizontally — check Config.TargetOffset/terrain", e.targetX, e.targetY, e.targetZ, verticalSearchRadius, horizontalSearchRadius)
		}
		if attempt >= maxJitterRetries {
			if tooClose {
				return rl.Observation{}, fmt.Errorf("rlenv: %d consecutive jittered targets landed within ArrivalThreshold (%.2f) of the reset position — check Config.TargetOffset/Jitter/ArrivalThreshold", maxJitterRetries, e.cfg.ArrivalThreshold)
			}
			return rl.Observation{}, fmt.Errorf("rlenv: %d consecutive jittered targets found no walkable+reachable cell (last tried (%.1f,%.1f,%.1f)) within %d blocks vertically / %d horizontally — check Config.TargetOffset/Jitter/terrain", maxJitterRetries, e.targetX, e.targetY, e.targetZ, verticalSearchRadius, horizontalSearchRadius)
		}
		targetOffset = e.cfg.TargetOffset
		for i := range targetOffset {
			targetOffset[i] = e.jitter(targetOffset[i], i)
		}
	}

	e.prevDistance = distance3(x, y, z, e.targetX, e.targetY, e.targetZ)

	// The flight target is a fixed point, resolved once here like
	// targetX/Y/Z above — no walkability/reachability retry loop applies
	// (that machinery is ground-pathfinding-specific; an airborne glide
	// target has no "walkable cell" to find). Jittered the same way
	// TargetOffset is, from the same base (x, y, z), using the same
	// Config.Jitter magnitudes — a second, independent jitter field was
	// judged unnecessary for a first pass (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_II.md Phase 3).
	flyOffset := e.cfg.FlyTargetOffset
	for i := range flyOffset {
		flyOffset[i] = e.jitter(flyOffset[i], i)
	}
	e.flyTargetX = x + flyOffset[0]
	e.flyTargetY = y + flyOffset[1]
	e.flyTargetZ = z + flyOffset[2]

	e.episodeStarted = true

	health, food, saturation, healthKnown := e.agent.Health()
	e.prevHealth, e.prevHealthKnown = health, healthKnown

	if e.cfg.Seeder != nil {
		seedAgent, ok := e.agent.(SeedAgent)
		if !ok {
			return rl.Observation{}, errSeederRequiresSeedAgent
		}
		// A composite "go there, then mine/craft" episode's mine block or
		// crafting table belongs near the goto target, out of sight of the
		// start - the seeder's near-the-bot placement would defeat the task.
		seedCfg := e.cfg
		if e.seedAtGoal() {
			seedCfg.MineTargetBlock, seedCfg.CraftTargetItem = "", ""
		}
		if err := e.cfg.Seeder(ctx, seedAgent, seedCfg); err != nil {
			return rl.Observation{}, fmt.Errorf("rlenv: episode seeding: %w", err)
		}
		if e.seedAtGoal() {
			if err := e.seedFarTargets(ctx); err != nil {
				return rl.Observation{}, fmt.Errorf("rlenv: episode seeding: %w", err)
			}
		}
	}

	if err := e.refreshMineTarget(ctx); err != nil {
		return rl.Observation{}, err
	}
	if err := e.refreshPickupTarget(ctx); err != nil {
		return rl.Observation{}, err
	}
	if err := e.refreshRideTarget(ctx); err != nil {
		return rl.Observation{}, err
	}
	if err := e.refreshCombatTarget(ctx); err != nil {
		return rl.Observation{}, err
	}
	if err := e.refreshCreeperTarget(ctx); err != nil {
		return rl.Observation{}, err
	}
	if err := e.refreshSpearTarget(ctx); err != nil {
		return rl.Observation{}, err
	}
	if err := e.refreshFollowTarget(ctx); err != nil {
		return rl.Observation{}, err
	}
	e.craftCount = e.craftCountNow()
	e.pickupCount = e.pickupCountNow()
	e.placeCount = e.placeCountNow()
	e.gunpowderCount = e.gunpowderCountNow()
	e.mounted = e.agent.IsMounted()
	e.followed = e.agent.IsFollowing()
	e.refreshChain()
	e.craftReady = e.craftReadyNow(ctx)
	e.equipReady = e.equipReadyNow()
	obsMineX, obsMineY, obsMineZ, obsMineVisible := e.observationMineTarget()
	obsCombatHealth, _ := e.combatTargetHealthNow()
	obs := buildObservation(x, y, z, yaw, pitch, e.targetX, e.targetY, e.targetZ, health, food, saturation, healthKnown, obsMineX, obsMineY, obsMineZ, obsMineVisible, e.craftReady, !e.cfg.GoToTargetDisabled, e.cfg.MineTargetBlock != "", e.cfg.CraftTargetItem != "", e.cfg.NormalizeObservation, e.chainObs(), e.equipReady, e.cfg.EquipTargetItem != "", e.pickupX, e.pickupY, e.pickupZ, e.pickupVisible, e.pickupTaskActive(), e.flyTargetX, e.flyTargetY, e.flyTargetZ, e.cfg.FlyTargetActive, e.agent.IsGliding(), e.agent.IsEquipped(elytraItemName), e.rideX, e.rideY, e.rideZ, e.rideVisible, e.mounted, e.cfg.RideVehicleType != "", e.combatX, e.combatY, e.combatZ, obsCombatHealth, e.combatVisible, e.cfg.CombatTargetMobType != "", e.placeStandaloneReady(), e.cfg.ChainStage == 0 && e.cfg.PlaceTargetItem != "", e.followTargetVisible, e.followed, e.cfg.FollowTargetPlayerName != "", e.creeperX, e.creeperY, e.creeperZ, e.creeperVisible, e.cfg.CreeperGunpowderActive, e.spearX, e.spearY, e.spearZ, e.spearVisible, e.cfg.SpearChargeTargetMobType != "")
	// Seed Config.StuckTimeout's baseline with this episode's starting
	// observation, not nil — a bot that's already idle from the very first
	// Step (nothing moved it since Reset) should count toward the timeout
	// starting there, not only once a second Step happens to repeat the
	// first Step's own result. See Step's own use of lastObsValues.
	e.lastObsValues = append(e.lastObsValues[:0], obs.Values...)
	return obs, nil
}

// craftCountNow returns the bot's current held count of
// Config.CraftTargetItem (see LiveAgent.InventoryCount), or 0 if
// Config.CraftTargetItem is unset — mirrors refreshMineTarget's
// empty-target handling.
func (e *Environment) craftCountNow() int {
	if e.cfg.CraftTargetItem == "" {
		return 0
	}
	return e.agent.InventoryCount(e.cfg.CraftTargetItem)
}

// craftReadyNow reports whether Environment's currently configured craft
// target (if any) looks assembleable right now (see LiveAgent.Craftable),
// or false if Config.CraftTargetItem is unset. With Config.CraftSearchRadius
// set it additionally requires a crafting table visible within that radius,
// so a table further away can't be crafted at (see that field for why).
func (e *Environment) craftReadyNow(ctx context.Context) bool {
	if e.cfg.ChainStage > 0 {
		return e.chainCraftTarget() != ""
	}
	if e.cfg.CraftTargetItem == "" {
		return false
	}
	if !e.agent.Craftable(e.cfg.CraftTargetItem) {
		return false
	}
	if e.cfg.CraftSearchRadius <= 0 {
		return true
	}
	// A search error is treated as "no table in range" rather than failing
	// the step: the next step searches again, and mining's equivalent
	// (refreshMineTarget) only fails on errors that mean the world isn't
	// ready, which Reset and Step have already surfaced by this point.
	_, _, _, found, err := e.agent.FindVisibleBlock(ctx, craftingTableBlock, e.cfg.CraftSearchRadius)
	return err == nil && found
}

// craftingTableBlock is the block Config.CraftSearchRadius looks for.
const craftingTableBlock = "minecraft:crafting_table"

// gunpowderCountNow returns the bot's current held count of
// "minecraft:gunpowder" (see LiveAgent.InventoryCount), or 0 if
// Config.CreeperGunpowderActive is false — mirrors craftCountNow's
// empty-target handling, except gated on the task's own active flag
// rather than an item-name field (there is none — see
// Config.CreeperGunpowderActive's own doc comment).
func (e *Environment) gunpowderCountNow() int {
	if !e.cfg.CreeperGunpowderActive {
		return 0
	}
	return e.agent.InventoryCount(creeperGunpowderItemName)
}

// elytraItemName is the item ActionFlyTo's legality/observation checks
// (LiveAgent.IsEquipped/agent.FlyTo itself) require worn — the one and
// only item Flight cares about, so it's a fixed constant, not a Config
// field (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3).
const elytraItemName = "minecraft:elytra"

// creeperGunpowderMobType is the entity type
// ActionKillCreeperForGunpowder's legality/observation checks search for —
// the method itself is hardcoded to creepers by design (see
// Config.CreeperGunpowderActive's own doc comment), so this is a fixed
// constant, not a Config field, mirroring elytraItemName's own reasoning.
const creeperGunpowderMobType = "minecraft:creeper"

// creeperGunpowderWeaponName mirrors agent's own unexported
// combatSeedWeapon constant (agent/rl_seed.go) — this package can't
// import that directly (unexported, different package), so the value is
// duplicated here deliberately, the same way elytraItemName duplicates
// agent.elytraItemName's own value rather than importing it.
// ActionKillCreeperForGunpowder's legality requires this exact item
// equipped, matching agent.validateCreeperWeapon's own real requirement
// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 5).
const creeperGunpowderWeaponName = "minecraft:iron_sword"

// creeperGunpowderItemName is the item gunpowderCountNow's InventoryCount
// check and reward.go's creeperGunpowderRewardBonus delta watch — the
// drop agent.KillCreeperForGunpowder collects.
const creeperGunpowderItemName = "minecraft:gunpowder"

// spearChargeWeaponName mirrors agent's own unexported spearSeedWeapon
// constant (agent/rl_seed.go) — this package can't import that directly
// (unexported, different package), so the value is duplicated here
// deliberately, the same reasoning creeperGunpowderWeaponName's own doc
// comment gives for duplicating combatSeedWeapon.
// ActionSpearChargeWithApproach's dispatch/legality both require this
// exact item equipped (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item
// 6).
const spearChargeWeaponName = "minecraft:iron_spear"

// spearChargeHoldDuration/engagedDuration/tiredDuration/minSpeed/
// minAlignment are fixed constants ActionSpearChargeWithApproach's
// dispatch passes through to agent.SpearChargeAtWithApproach — not
// exposed as continuous Config knobs for a first pass, matching
// combatSeedWeapon/flySeedRocketCount's own "fixed constant, not a
// policy parameter" precedent (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
// item 6). engagedDuration/tiredDuration feed
// combat.SpearChargeProfile.StageAt; holdDuration is chosen to land
// within the engaged stage (full damage/knockback/dismount, see
// combat.ContactOutcome) rather than tired/disengaged. minSpeed/
// minAlignment are passed through to agent.SpearChargeAt's own
// ChargeEligible check, which — as that method's own doc comment notes —
// currently only verifies the caller-supplied values against themselves,
// not real measured motion, so their exact magnitude has no live-gameplay
// effect yet; kept non-zero/positive only to stay within
// combat.SpearAttackRequest.Validate's own bounds.
const (
	spearChargeHoldDuration    = 400 * time.Millisecond
	spearChargeEngagedDuration = 500 * time.Millisecond
	spearChargeTiredDuration   = 1000 * time.Millisecond
	spearChargeMinSpeed        = 0.05
	spearChargeMinAlignment    = 0.9
)

// equipReadyNow reports whether Config.EquipTargetItem (if any) is
// currently equipped (see LiveAgent.IsEquipped), or false if it's unset —
// mirrors craftReadyNow's empty-target handling.
func (e *Environment) equipReadyNow() bool {
	if e.cfg.EquipTargetItem == "" {
		return false
	}
	return e.agent.IsEquipped(e.cfg.EquipTargetItem)
}

// pickupCountNow returns the bot's current held count of
// Config.PickupTargetItem (see LiveAgent.InventoryCount), or 0 if
// Config.PickupTargetItem is unset — mirrors craftCountNow's
// empty-target handling.
func (e *Environment) pickupCountNow() int {
	if e.cfg.PickupTargetItem == "" {
		return 0
	}
	return e.agent.InventoryCount(e.cfg.PickupTargetItem)
}

// placeCountNow returns the bot's current held count of
// Config.PlaceTargetItem, or 0 if it's unset or a chain episode is active
// (which keeps its own existing placement tracking) — mirrors
// pickupCountNow's empty-target handling.
// placeStandaloneReady reports whether this episode's standalone placement
// task (Config.PlaceTargetItem, outside a chain episode) is currently
// attemptable — holding at least one of the target item. Shared by
// actionLegal and buildObservation so the two can't drift apart
// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 2).
func (e *Environment) placeStandaloneReady() bool {
	return e.cfg.ChainStage == 0 && e.cfg.PlaceTargetItem != "" && e.agent.InventoryCount(e.cfg.PlaceTargetItem) > 0
}

func (e *Environment) placeCountNow() int {
	if e.cfg.ChainStage > 0 || e.cfg.PlaceTargetItem == "" {
		return 0
	}
	return e.agent.InventoryCount(e.cfg.PlaceTargetItem)
}

// pickupTaskActive reports whether this episode poses a pickup task at
// all, either named (Config.PickupTargetItem) or unnamed
// (Config.PickupAnyItem) — shared by actionLegal and both buildObservation
// call sites so the two conditions can't drift apart (docs/plans/
// RL_ACTION_SPACE_EXPANSION_III.md item 1).
func (e *Environment) pickupTaskActive() bool {
	return e.cfg.PickupTargetItem != "" || e.cfg.PickupAnyItem
}

// refreshPickupTarget re-resolves the nearest currently-visible instance of
// Config.PickupTargetItem (or, for Config.PickupAnyItem, the nearest visible
// item of any content) and stores it (with its entity ID) into
// e.pickupEntityID/pickupX/Y/Z/pickupVisible, for observation/dispatch
// purposes — the pickup-task analogue of refreshMineTarget (see its own doc
// comment for why this is called once at Reset and once per Step, reading
// the *previous* call's cached result as "before this step" state). Leaves
// e.pickupVisible=false without error if neither field is set or nothing
// matching is currently visible; only a genuine
// FindNearestVisibleItemOfType/FindNearestVisibleItem error is returned.
func (e *Environment) refreshPickupTarget(ctx context.Context) error {
	switch {
	case e.cfg.PickupTargetItem != "":
		id, x, y, z, found, err := e.agent.FindNearestVisibleItemOfType(ctx, e.cfg.PickupTargetItem, float64(e.cfg.PickupSearchRadius))
		if err != nil {
			return fmt.Errorf("rlenv: resolving pickup target: %w", err)
		}
		e.pickupEntityID, e.pickupX, e.pickupY, e.pickupZ, e.pickupVisible = id, x, y, z, found
	case e.cfg.PickupAnyItem:
		// FindNearestVisibleItem is the unfiltered predecessor
		// FindNearestVisibleItemOfType was built on top of — still here,
		// never removed, exactly what "any item" needs (docs/plans/
		// RL_ACTION_SPACE_EXPANSION_III.md item 1).
		id, x, y, z, found, err := e.agent.FindNearestVisibleItem(ctx, float64(e.cfg.PickupSearchRadius))
		if err != nil {
			return fmt.Errorf("rlenv: resolving pickup target (any item): %w", err)
		}
		e.pickupEntityID, e.pickupX, e.pickupY, e.pickupZ, e.pickupVisible = id, x, y, z, found
	default:
		e.pickupEntityID, e.pickupX, e.pickupY, e.pickupZ, e.pickupVisible = 0, 0, 0, 0, false
	}
	return nil
}

// refreshRideTarget re-resolves the nearest currently-visible instance of
// Config.RideVehicleType and stores it (with its entity ID) into
// e.rideEntityID/rideX/Y/Z/rideVisible — the ride-task analogue of
// refreshMineTarget/refreshPickupTarget. Leaves e.rideVisible=false
// without error if Config.RideVehicleType is unset or nothing matching is
// currently visible; only a genuine FindNearestVisibleEntityOfType error
// is returned.
func (e *Environment) refreshRideTarget(ctx context.Context) error {
	if e.cfg.RideVehicleType == "" {
		e.rideEntityID, e.rideX, e.rideY, e.rideZ, e.rideVisible = 0, 0, 0, 0, false
		return nil
	}
	id, x, y, z, found, err := e.agent.FindNearestVisibleEntityOfType(ctx, e.cfg.RideVehicleType, float64(e.cfg.RideSearchRadius))
	if err != nil {
		return fmt.Errorf("rlenv: resolving ride target: %w", err)
	}
	e.rideEntityID, e.rideX, e.rideY, e.rideZ, e.rideVisible = id, x, y, z, found
	return nil
}

// refreshCombatTarget re-resolves the nearest currently-visible instance
// of Config.CombatTargetMobType and stores it (with its entity ID) into
// e.combatEntityID/combatX/Y/Z/combatVisible — the combat-task analogue
// of refreshRideTarget. Leaves e.combatVisible=false without error if
// Config.CombatTargetMobType is unset or nothing matching is currently
// visible; only a genuine FindNearestVisibleEntityOfType error is
// returned.
func (e *Environment) refreshCombatTarget(ctx context.Context) error {
	if e.cfg.CombatTargetMobType == "" {
		e.combatEntityID, e.combatX, e.combatY, e.combatZ, e.combatVisible = 0, 0, 0, 0, false
		return nil
	}
	radius := e.cfg.CombatRadius
	if radius <= 0 {
		radius = DefaultConfig().CombatRadius
	}
	id, x, y, z, found, err := e.agent.FindNearestVisibleEntityOfType(ctx, e.cfg.CombatTargetMobType, radius)
	if err != nil {
		return fmt.Errorf("rlenv: resolving combat target: %w", err)
	}
	e.combatEntityID, e.combatX, e.combatY, e.combatZ, e.combatVisible = id, x, y, z, found
	return nil
}

// refreshCreeperTarget re-resolves the nearest currently-visible creeper
// and stores it (with its entity ID) into e.creeperEntityID/creeperX/Y/Z/
// creeperVisible — the creeper-gunpowder-task analogue of
// refreshCombatTarget, same caching shape, hardcoded mob type instead of
// a Config field (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 5).
// Leaves e.creeperVisible=false without error if Config.CreeperGunpowderActive
// is false or nothing matching is currently visible; only a genuine
// FindNearestVisibleEntityOfType error is returned.
func (e *Environment) refreshCreeperTarget(ctx context.Context) error {
	if !e.cfg.CreeperGunpowderActive {
		e.creeperEntityID, e.creeperX, e.creeperY, e.creeperZ, e.creeperVisible = 0, 0, 0, 0, false
		return nil
	}
	radius := e.cfg.CreeperGunpowderRadius
	if radius <= 0 {
		radius = DefaultConfig().CreeperGunpowderRadius
	}
	id, x, y, z, found, err := e.agent.FindNearestVisibleEntityOfType(ctx, creeperGunpowderMobType, radius)
	if err != nil {
		return fmt.Errorf("rlenv: resolving creeper-gunpowder target: %w", err)
	}
	e.creeperEntityID, e.creeperX, e.creeperY, e.creeperZ, e.creeperVisible = id, x, y, z, found
	return nil
}

// refreshSpearTarget re-resolves the nearest currently-visible instance of
// Config.SpearChargeTargetMobType and stores it (with its entity ID) into
// e.spearEntityID/spearX/Y/Z/spearVisible — the spear-charge-task analogue
// of refreshCombatTarget, same caching shape (docs/plans/
// RL_ACTION_SPACE_EXPANSION_III.md item 6). Leaves e.spearVisible=false
// without error if Config.SpearChargeTargetMobType is unset or nothing
// matching is currently visible; only a genuine
// FindNearestVisibleEntityOfType error is returned.
func (e *Environment) refreshSpearTarget(ctx context.Context) error {
	if e.cfg.SpearChargeTargetMobType == "" {
		e.spearEntityID, e.spearX, e.spearY, e.spearZ, e.spearVisible = 0, 0, 0, 0, false
		return nil
	}
	radius := e.cfg.SpearChargeRadius
	if radius <= 0 {
		radius = DefaultConfig().SpearChargeRadius
	}
	id, x, y, z, found, err := e.agent.FindNearestVisibleEntityOfType(ctx, e.cfg.SpearChargeTargetMobType, radius)
	if err != nil {
		return fmt.Errorf("rlenv: resolving spear-charge target: %w", err)
	}
	e.spearEntityID, e.spearX, e.spearY, e.spearZ, e.spearVisible = id, x, y, z, found
	return nil
}

// refreshFollowTarget re-resolves whether Config.FollowTargetPlayerName is
// currently connected/resolvable and stores it into
// e.followTargetVisible — the follow-task analogue of refreshCombatTarget.
// Leaves e.followTargetVisible=false without error if
// Config.FollowTargetPlayerName is unset, this agent has no follow
// subsystem at all (HasFollowManager), or the named player simply isn't
// currently resolvable.
//
// FindPlayerByName's own contract differs from FindVisibleBlock/
// FindNearestVisibleEntityOfType's "found=false, err=nil for nothing
// there": it returns a real error for "not found," not just found=false
// (agent/command_actions.go wraps the target selector's own error
// through unchanged). A routine "the other player/bot isn't connected
// right now" is exactly the common case this task's own precondition
// describes (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 4) — it
// must not be treated as fatal to Step/Reset the way a genuine error
// would be for every other task's own refresh. Only a caller-context
// cancellation propagates as a real error here.
func (e *Environment) refreshFollowTarget(ctx context.Context) error {
	if e.cfg.FollowTargetPlayerName == "" || !e.agent.HasFollowManager() {
		e.followTargetVisible = false
		return nil
	}
	_, _, _, found, err := e.agent.FindPlayerByName(ctx, e.cfg.FollowTargetPlayerName)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("rlenv: resolving follow target: %w", err)
		}
		e.followTargetVisible = false
		return nil
	}
	e.followTargetVisible = found
	return nil
}

// combatTargetHealthNow returns the last-known health of
// e.combatEntityID if it's currently tracked, 0 otherwise — used both for
// the observation (when e.combatVisible) and, via its "found" half, by
// Step's own kill check.
func (e *Environment) combatTargetHealthNow() (health float32, found bool) {
	if !e.combatVisible {
		return 0, false
	}
	return e.agent.GetEntityHealth(e.combatEntityID)
}

// dismountForReset dismounts the bot if currently mounted, before any
// teleport/cleanup runs — unconditional, not gated on
// Config.RideVehicleType, since a Reset must never leave a Config change
// (e.g. a TaskSelector that stopped posing the ride task) carrying a
// stale mount across episode boundaries. A Reset that teleported the bot
// away while still mounted risks the same class of desync BlockRestorer's
// own doc comment warns about for mining pits — moving the bot out from
// under its own tracked vehicle rather than off of it cleanly. Best
// effort: a failed dismount is logged, never fails the Reset.
func (e *Environment) dismountForReset() {
	if !e.agent.IsMounted() {
		return
	}
	if err := e.agent.DismountEntity(); err != nil {
		log.Printf("rlenv: dismounting before reset: %v", err)
	}
}

// stopSneakForReset clears sneaking before any teleport/cleanup runs —
// unconditional, the same reasoning dismountForReset's own doc comment
// gives for mounting: a persistent toggle (here, from a previous episode's
// Config.ArriveSneaking) must never survive into the next episode
// regardless of this episode's own Config (docs/plans/
// RL_ACTION_SPACE_EXPANSION_III.md item 3). Best effort: a failure is
// logged, never fails the Reset.
func (e *Environment) stopSneakForReset() {
	if err := e.agent.StopSneaking(); err != nil {
		log.Printf("rlenv: stopping sneak before reset: %v", err)
	}
}

// rideVehicleCleanupRadius is how far around the bot's own current
// position clearRideVehicle looks for Config.RideVehicleType instances to
// remove — see that method's own doc comment for why "current position,"
// not the stale reset origin.
const rideVehicleCleanupRadius = 16

// clearRideVehicle kills every live instance of the *previous* episode's
// Config.RideVehicleType near wherever the bot currently is (called
// before Config.TaskSelector's override runs, so this still reads the
// previous episode's value — see resetAttempt's own ordering) — the
// Reset-time cleanup counterpart to SeedVehicle. Deliberately centered on
// the bot's own current position, not a remembered seed location: riding
// could have carried the bot (and its mount) far from wherever it was
// seeded, the same reason dismountForReset runs first, so the vehicle is
// still right where the bot just dismounted it. Not ID-specific — see
// docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 0's own note on why
// precise per-episode-entity targeting (to avoid a shared-server
// episode's cleanup killing another bot's own mount) is deferred, not
// solved here: a position-bounded kill centered on this bot's own
// position is a reasonable, simple approximation of that precision for a
// single-bot (or well-separated shared-server working area) training
// run. Best effort: a failure is logged, never fails the Reset.
func (e *Environment) clearRideVehicle(ctx context.Context) {
	if e.cfg.RideVehicleType == "" {
		return
	}
	clearer, ok := e.agent.(EntitySeedAgent)
	if !ok {
		return
	}
	pos, _, _, ok := e.agent.GetPosition()
	if !ok {
		return
	}
	x, y, z := int(math.Floor(pos.X)), int(math.Floor(pos.Y)), int(math.Floor(pos.Z))
	if err := clearer.ClearEntitiesOfType(ctx, x, y, z, rideVehicleCleanupRadius, e.cfg.RideVehicleType); err != nil {
		log.Printf("rlenv: clearing ridden vehicle at (%d,%d,%d): %v", x, y, z, err)
	}
}

// mineTargetBlockInUse resolves which block name refreshMineTarget actually
// tracks right now: ordinarily Config.MineTargetBlock, but for a
// ChainMine-stage chain episode, Config.ChainMineTargetBlock once the goal
// item is already crafted — the gather stage's log is done with by then,
// and the chain has moved on to finding the buried block (docs/plans/14-
// buried-block-mining-and-memory-map.md, mc-rsi-trainer, "Part 4").
func (e *Environment) mineTargetBlockInUse() string {
	if e.cfg.ChainStage == ChainMine && e.chainState.craftedGoal {
		return e.cfg.ChainMineTargetBlock
	}
	return e.cfg.MineTargetBlock
}

// mineTargetMemoryEnabled keeps the last-seen target coordinates for buried
// mining, where the target is expected to disappear behind blocks, but leaves
// ordinary mine tasks on the pre-buried behavior: their observation reports
// only a target that is currently visible.
func (e *Environment) mineTargetMemoryEnabled() bool {
	return e.cfg.MineTargetBuried || e.cfg.ChainStage == ChainMine
}

func (e *Environment) observationMineTarget() (x, y, z float64, visible bool) {
	if e.mineTargetMemoryEnabled() {
		return e.mineKnownX, e.mineKnownY, e.mineKnownZ, e.mineEverSeen
	}
	return e.mineX, e.mineY, e.mineZ, e.mineVisible
}

// refreshMineTarget re-resolves the nearest currently-visible instance of
// mineTargetBlockInUse() and stores it into e.mineX/Y/Z/mineVisible, for
// observation/reward purposes. Called once at Reset and once per Step
// (after that step's dispatch — see Step), not more: unlike TargetOffset's
// fixed point, mining consumes its target, so the nearest visible instance
// can genuinely change step to step as blocks are broken (docs/plans/
// RL_ACTION_SPACE_EXPANSION.md Phase 2a) and needs re-resolving regularly,
// but Step reads the *previous* call's cached result as "before this step"
// state (see its mined-this-step check) rather than re-resolving twice per
// call — the same caching shape prevDistance/prevHealth already use.
// Leaves e.mineVisible=false without error if the resolved target is unset
// (the common case for instances that don't pose a mining task) or if
// FindVisibleBlock simply finds nothing in range; only a genuine
// FindVisibleBlock error (e.g. world/block-manager not ready) is returned.
//
// Known cost tradeoff, not solved here: this runs FindVisibleBlock once per
// step whenever a target is configured, regardless of which action
// was dispatched, plus a second FindVisibleBlock call inside the "mine"
// action's own Execute on steps that actually dispatch ActionMine (see
// action.go's resolveDispatch). Acceptable for now; revisit (e.g. shrinking
// MineSearchRadius, or resolving less often) if it matters in practice.
func (e *Environment) refreshMineTarget(ctx context.Context) error {
	target := e.mineTargetBlockInUse()
	if target != e.mineTargetNameInUse {
		// The effective mine target just changed (a ChainMine-stage chain
		// switching from the gathered log to the buried block, or a fresh
		// episode) — any remembered position belonged to the old target.
		e.mineKnownX, e.mineKnownY, e.mineKnownZ, e.mineEverSeen = 0, 0, 0, false
		e.mineTargetNameInUse = target
	}
	if target == "" {
		e.mineX, e.mineY, e.mineZ, e.mineVisible = 0, 0, 0, false
		return nil
	}
	x, y, z, found, err := e.agent.FindVisibleBlock(ctx, target, e.cfg.MineSearchRadius)
	if err != nil {
		return fmt.Errorf("rlenv: resolving mine target: %w", err)
	}
	e.mineX, e.mineY, e.mineZ, e.mineVisible = x, y, z, found
	if found {
		e.mineKnownX, e.mineKnownY, e.mineKnownZ, e.mineEverSeen = x, y, z, true
	}
	return nil
}

// Step implements rl.Environment.
//
// Movement completion, resolved via models.Completion: actions.MoveToQuiet.Execute
// (what "movetoquiet" dispatches to) launches MoveTo (notifyChat=false)
// in its own goroutine and returns immediately, but as of
// models.ActionRegistry's uniform completion signal (see
// models/completion.go), that goroutine's
// eventual outcome is available via the models.Completion Execute
// returns — closing the "done signal" gap RL_POLICY_INTEGRATION_PLAN.md
// item 3 flagged. Step waits on that Completion, bounded by
// Config.StepTimeout, instead of polling position for a proxy signal.
// A step timing out (the Completion hasn't resolved yet) is treated as a
// normal, non-fatal outcome — the episode continues, and the dispatched
// move may still be running in the background past this Step call.
func (e *Environment) Step(ctx context.Context, action rl.Action) (rl.StepResult, error) {
	if !e.episodeStarted {
		return rl.StepResult{}, fmt.Errorf("rlenv: Step called before Reset")
	}

	if _, _, _, ok := e.agent.GetPosition(); !ok {
		return rl.StepResult{}, errPositionUnknown
	}
	e.moveAwayFromDamage(ctx)

	dispatch, shouldDispatch, err := e.resolveDispatch(action)
	if err != nil {
		return rl.StepResult{}, err
	}

	// e.mineX/Y/Z/mineVisible still hold whatever the previous Step (or
	// Reset) last resolved — read here as "before this step's action"
	// state, regardless of whether that action was ActionMine (mirrors
	// refreshMineTarget's own doc comment on why this isn't gated to
	// mine-dispatch steps).
	prevMineX, prevMineY, prevMineZ, prevMineVisible := e.mineX, e.mineY, e.mineZ, e.mineVisible
	prevMineBlockName := ""
	if prevMineVisible {
		prevMineBlockName = e.agent.BlockNameAt(int(prevMineX), int(prevMineY), int(prevMineZ))
	}
	// e.craftCount still holds whatever the previous Step (or Reset) last
	// resolved — read here as "before this step's action" state, regardless
	// of whether that action was ActionCraft (mirrors prevMineVisible's own
	// reasoning above).
	prevCraftCount := e.craftCount
	// e.equipReady mirrors prevCraftCount's own reasoning above, for the
	// equip task.
	prevEquipReady := e.equipReady
	// e.pickupCount mirrors prevCraftCount's own reasoning above, for the
	// pickup task.
	prevPickupCount := e.pickupCount
	// e.placeCount mirrors prevPickupCount's own reasoning above, for the
	// standalone placement task.
	prevPlaceCount := e.placeCount
	// e.gunpowderCount mirrors prevPlaceCount's own reasoning above, for
	// the creeper-gunpowder task.
	prevGunpowderCount := e.gunpowderCount
	// prevPickupEntityID/prevPickupVisible back Config.PickupAnyItem's own
	// reward check (see pickedUpThisStep below) — captured before dispatch
	// for the same reason prevCombatEntityID is.
	prevPickupEntityID := e.pickupEntityID
	prevPickupVisible := e.pickupVisible
	// e.mounted mirrors prevEquipReady's own reasoning above, for the
	// ride task.
	prevMounted := e.mounted
	// e.followed mirrors prevMounted's own reasoning above, for the
	// follow task.
	prevFollowed := e.followed
	// prevCombatEntityID/prevCombatVisible mirror prevMounted's own
	// reasoning above, for the combat task — captured before dispatch so
	// Step's post-dispatch check asks "did THIS entity die," not
	// whichever entity happens to be nearest after the fact.
	prevCombatEntityID := e.combatEntityID
	prevCombatVisible := e.combatVisible
	// prevSpearEntityID/prevSpearVisible mirror prevCombatEntityID's own
	// reasoning above, for the spear-charge task.
	prevSpearEntityID := e.spearEntityID
	prevSpearVisible := e.spearVisible

	if shouldDispatch {
		e.cancelInflight()
		actionCtx, cancelAction := context.WithCancel(ctx)
		completion, err := e.registry.Execute(actionCtx, dispatch.name, e.agent, dispatch.args)
		if err != nil {
			cancelAction()
			return rl.StepResult{}, fmt.Errorf("rlenv: dispatching %s: %w", dispatch.name, err)
		}
		resolved, err := e.awaitStep(ctx, completion)
		if err != nil {
			cancelAction()
			return rl.StepResult{}, err
		}
		if resolved {
			cancelAction()
			e.consecutiveStepTimeouts = 0
		} else {
			e.inflight = &inflightAction{cancel: cancelAction, completion: completion}
			e.consecutiveStepTimeouts++
		}
	}
	if shouldDispatch && dispatch.name == placeActionName {
		e.placeAttempted = true
	}
	if shouldDispatch && dispatch.name == mineActionName && e.cfg.CollectDrops {
		e.collectDrops(ctx)
	}
	pos, yaw, pitch, ok := e.agent.GetPosition()
	if !ok {
		return rl.StepResult{}, errPositionUnknown
	}
	x, y, z := pos.X, pos.Y, pos.Z
	newHealth, food, saturation, newHealthKnown := e.agent.Health()

	// mined is true if the pre-dispatch target position's block changed at
	// all this step — not narrowly "became air," since a block could also
	// be replaced by something else (e.g. a falling block landing there).
	// Judged from actual world state, not from whether ActionMine was the
	// dispatched action (see reward.go's mineRewardBonus doc comment).
	mined := false
	if prevMineVisible {
		newMineBlockName := e.agent.BlockNameAt(int(prevMineX), int(prevMineY), int(prevMineZ))
		mined = newMineBlockName != prevMineBlockName
	}

	// craftedThisStep is judged from an actual inventory-count increase,
	// not from whether ActionCraft was the dispatched action — mirrors
	// mined's own reasoning above (see reward.go's craftRewardBonus doc
	// comment). Guarded on CraftTargetItem being set so an unconfigured
	// instance's permanently-zero craftCount never spuriously reads as
	// "increased."
	newCraftCount := e.craftCountNow()
	craftedThisStep := e.cfg.CraftTargetItem != "" && newCraftCount > prevCraftCount

	// equippedThisStep is judged from an actual IsEquipped transition
	// (false -> true), not from whether ActionEquip was the dispatched
	// action — mirrors craftedThisStep's own reasoning above (see
	// reward.go's equipRewardBonus doc comment).
	newEquipReady := e.equipReadyNow()
	equippedThisStep := e.cfg.EquipTargetItem != "" && newEquipReady && !prevEquipReady

	// pickedUpThisStep is judged from an actual world-state change, not
	// from whether ActionPickUpItem was the dispatched action — mirrors
	// craftedThisStep's own reasoning above (see reward.go's
	// pickupRewardBonus doc comment). Config.PickupTargetItem judges this
	// via an InventoryCount increase (there's one specific item name to
	// check); Config.PickupAnyItem has no such name, so it judges from the
	// resolved item entity no longer being tracked instead (reusing
	// GetEntityHealth purely for its "found" signal — see
	// docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 1).
	newPickupCount := e.pickupCountNow()
	pickedUpThisStep := e.cfg.PickupTargetItem != "" && newPickupCount > prevPickupCount
	if e.cfg.PickupAnyItem && prevPickupVisible {
		_, stillTracked := e.agent.GetEntityHealth(prevPickupEntityID)
		pickedUpThisStep = pickedUpThisStep || !stillTracked
	}

	// placedStandaloneThisStep is judged from an actual InventoryCount
	// decrease (placing consumes one unit), not from whether ActionPlace
	// was the dispatched action — mirrors pickedUpThisStep's own reasoning
	// above. Chain-episode placement (Config.ChainStage > 0) keeps its own
	// existing milestone-reward handling further down, unaffected by this
	// (placeCountNow returns 0 during a chain episode, so this is always
	// false then — docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 2).
	newPlaceCount := e.placeCountNow()
	placedStandaloneThisStep := e.cfg.ChainStage == 0 && e.cfg.PlaceTargetItem != "" && newPlaceCount < prevPlaceCount

	// gunpowderCollectedThisStep is judged from an actual InventoryCount
	// increase, not from whether ActionKillCreeperForGunpowder was the
	// dispatched action — mirrors craftedThisStep's own reasoning above.
	// Faithful to the real encounter's uncertain drop rate: a kill with no
	// drop (a valid, documented outcome of agent.KillCreeperForGunpowder)
	// earns nothing here, matching how a real player farming creepers
	// experiences it (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 5).
	newGunpowderCount := e.gunpowderCountNow()
	gunpowderCollectedThisStep := e.cfg.CreeperGunpowderActive && newGunpowderCount > prevGunpowderCount

	// mountedThisStep is judged from an actual IsMounted transition
	// (false -> true), not from whether ActionMount was the dispatched
	// action — mirrors equippedThisStep's own reasoning above (see
	// reward.go's rideRewardBonus doc comment). Not narrowed to "mounted
	// specifically e.rideEntityID": a first pass has nothing else to
	// mount in a normal single-vehicle-seeded episode, so this is judged
	// sufficient without tracking which entity was actually boarded — see
	// docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 4's own note.
	newMounted := e.agent.IsMounted()
	mountedThisStep := e.cfg.RideVehicleType != "" && newMounted && !prevMounted

	// followedThisStep mirrors mountedThisStep's own reasoning above, for
	// the follow task — a one-shot IsFollowing() transition, not
	// continuous proximity-shaping (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_III.md item 4's own note on why).
	newFollowed := e.agent.IsFollowing()
	followedThisStep := e.cfg.FollowTargetPlayerName != "" && newFollowed && !prevFollowed

	// combatKilledThisStep is judged from the specific entity resolved
	// before this step dying — not visible/tracked at all, or tracked
	// with health <= 0 (see reward.go's combatRewardBonus doc comment for
	// why either counts) — not from whether ActionCombat was the
	// dispatched action, mirroring every other task bonus's philosophy.
	combatKilledThisStep := false
	if e.cfg.CombatTargetMobType != "" && prevCombatVisible {
		health, found := e.agent.GetEntityHealth(prevCombatEntityID)
		combatKilledThisStep = !found || health <= 0
	}

	// spearKilledThisStep mirrors combatKilledThisStep's own reasoning
	// above, for the spear-charge task (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_III.md item 6).
	spearKilledThisStep := false
	if e.cfg.SpearChargeTargetMobType != "" && prevSpearVisible {
		health, found := e.agent.GetEntityHealth(prevSpearEntityID)
		spearKilledThisStep = !found || health <= 0
	}

	// A chain episode replaces the mine and craft bonuses (which end the
	// episode on the first mined block or crafted item) with milestone
	// rewards for the whole chain; see chain.go. goalCrafted feeds the
	// craftedGoal milestone at both ChainUse (terminal there) and ChainMine
	// (intermediate there — the chain continues on to mining); minedBuried
	// feeds the new terminal milestone at ChainMine only. mined itself is
	// generic — true whenever *whatever* mineTargetBlockInUse() currently
	// resolves to just changed, the gathered log during the gather phase
	// included — so minedBuried additionally requires e.chainState.
	// craftedGoal (as of the end of the *previous* step, since this runs
	// before chainState.advance below updates it): only once that's true
	// has refreshMineTarget actually switched tracking to the buried
	// block, so only then does a generic mined event mean *that* was
	// mined, not a gathered log.
	var chainReward float32
	chainDone := false
	if e.chainActive() {
		e.refreshChain()
		goalCrafted := (e.cfg.ChainStage == ChainUse || e.cfg.ChainStage == ChainMine) && craftedThisStep
		minedBuried := e.cfg.ChainStage == ChainMine && e.chainState.craftedGoal && mined
		chainReward, chainDone = e.chainState.advance(e.cfg.ChainStage, e.chainInv, e.chainTablePlaced, goalCrafted, minedBuried)
		craftedThisStep = false
	}

	// Distance shaping and the arrival bonus belong to the goto task only.
	// Applied unconditionally they scored mine and craft episodes against a
	// phantom "target" (the jittered reset position, 1.5-2.8 blocks away):
	// moving toward it paid +1 per block, and reaching it paid +10 and ended
	// the episode without the block ever being mined or the item crafted.
	gotoActive := !e.cfg.GoToTargetDisabled
	newDistance := distance3(x, y, z, e.targetX, e.targetY, e.targetZ)
	if !gotoActive {
		newDistance = e.prevDistance
	}
	reward, done := computeReward(stepOutcome{
		timePenalty:       e.cfg.TimePenalty,
		prevDistance:      e.prevDistance,
		newDistance:       newDistance,
		prevHealth:        e.prevHealth,
		newHealth:         newHealth,
		healthKnownBefore: e.prevHealthKnown,
		healthKnownAfter:  newHealthKnown,
	})
	doneReason := ""
	if done {
		doneReason = "reward"
	}
	if gotoActive && newDistance <= e.cfg.ArrivalThreshold {
		if e.compositeGotoTask() {
			// "Go there, then mine/craft": reaching the target pays once
			// but doesn't end the episode - the mine or craft still has to
			// happen.
			if !e.arrived {
				reward += arrivalBonus
				e.arrived = true
			}
		} else {
			reward += arrivalBonus
			done = true
			doneReason = "goto"
		}
	}
	if mined {
		if !e.chainActive() {
			reward += mineRewardBonus
			done = true
			doneReason = "mine"
		}
		if restorableBlockName(prevMineBlockName) {
			e.minedBlocks = append(e.minedBlocks, minedBlock{int(prevMineX), int(prevMineY), int(prevMineZ), prevMineBlockName})
		}
	}
	if craftedThisStep {
		reward += craftRewardBonus
		done = true
		doneReason = "craft"
	}
	if equippedThisStep {
		reward += equipRewardBonus
		done = true
		doneReason = "equip"
	}
	if pickedUpThisStep {
		reward += pickupRewardBonus
		done = true
		doneReason = "pickup"
	}
	if placedStandaloneThisStep {
		reward += placeRewardBonus
		done = true
		doneReason = "place"
	}
	if gunpowderCollectedThisStep {
		reward += creeperGunpowderRewardBonus
		done = true
		doneReason = "creepergunpowder"
	}
	if e.cfg.FlyTargetActive {
		arrivalRadius := e.cfg.FlyArrivalRadius
		if arrivalRadius <= 0 {
			arrivalRadius = DefaultConfig().FlyArrivalRadius
		}
		if distanceXZ(x, z, e.flyTargetX, e.flyTargetZ) <= arrivalRadius {
			reward += flyRewardBonus
			done = true
			doneReason = "fly"
		}
	}
	if mountedThisStep {
		reward += rideRewardBonus
		done = true
		doneReason = "ride"
	}
	if followedThisStep {
		reward += followRewardBonus
		done = true
		doneReason = "follow"
	}
	if combatKilledThisStep {
		reward += combatRewardBonus
		done = true
		doneReason = "combat"
	}
	if spearKilledThisStep {
		reward += spearChargeRewardBonus
		done = true
		doneReason = "spearcharge"
	}
	reward += chainReward
	if chainDone {
		done = true
		doneReason = "chain"
	}
	e.prevDistance = newDistance
	e.prevHealth, e.prevHealthKnown = newHealth, newHealthKnown

	// Refresh the cached mine target for the observation returned to the
	// caller (and for the next Step's "before" read): mining (or anything
	// else) this step may have changed what's nearest/visible, and the
	// observation should reflect post-step state the same way
	// position/health already do.
	if err := e.refreshMineTarget(ctx); err != nil {
		return rl.StepResult{}, err
	}
	if err := e.refreshPickupTarget(ctx); err != nil {
		return rl.StepResult{}, err
	}
	if err := e.refreshRideTarget(ctx); err != nil {
		return rl.StepResult{}, err
	}
	if err := e.refreshCombatTarget(ctx); err != nil {
		return rl.StepResult{}, err
	}
	if err := e.refreshCreeperTarget(ctx); err != nil {
		return rl.StepResult{}, err
	}
	if err := e.refreshSpearTarget(ctx); err != nil {
		return rl.StepResult{}, err
	}
	if err := e.refreshFollowTarget(ctx); err != nil {
		return rl.StepResult{}, err
	}
	// Same refresh for craft state: newCraftCount is already this step's
	// post-dispatch value, so it becomes the next Step's "before" read
	// directly; craftReady is recomputed since inventory contents may have
	// changed even when craftedThisStep is false (e.g. an ingredient was
	// picked up, not the target item itself).
	e.craftCount = newCraftCount
	e.craftReady = e.craftReadyNow(ctx)
	e.equipReady = newEquipReady
	e.pickupCount = newPickupCount
	e.placeCount = newPlaceCount
	e.gunpowderCount = newGunpowderCount
	e.mounted = newMounted
	e.followed = newFollowed
	obsMineX, obsMineY, obsMineZ, obsMineVisible := e.observationMineTarget()
	obsCombatHealth, _ := e.combatTargetHealthNow()
	obs := buildObservation(x, y, z, yaw, pitch, e.targetX, e.targetY, e.targetZ, newHealth, food, saturation, newHealthKnown, obsMineX, obsMineY, obsMineZ, obsMineVisible, e.craftReady, !e.cfg.GoToTargetDisabled, e.cfg.MineTargetBlock != "", e.cfg.CraftTargetItem != "", e.cfg.NormalizeObservation, e.chainObs(), e.equipReady, e.cfg.EquipTargetItem != "", e.pickupX, e.pickupY, e.pickupZ, e.pickupVisible, e.pickupTaskActive(), e.flyTargetX, e.flyTargetY, e.flyTargetZ, e.cfg.FlyTargetActive, e.agent.IsGliding(), e.agent.IsEquipped(elytraItemName), e.rideX, e.rideY, e.rideZ, e.rideVisible, e.mounted, e.cfg.RideVehicleType != "", e.combatX, e.combatY, e.combatZ, obsCombatHealth, e.combatVisible, e.cfg.CombatTargetMobType != "", e.placeStandaloneReady(), e.cfg.ChainStage == 0 && e.cfg.PlaceTargetItem != "", e.followTargetVisible, e.followed, e.cfg.FollowTargetPlayerName != "", e.creeperX, e.creeperY, e.creeperZ, e.creeperVisible, e.cfg.CreeperGunpowderActive, e.spearX, e.spearY, e.spearZ, e.spearVisible, e.cfg.SpearChargeTargetMobType != "")

	// Config.StuckTimeout: force the episode done once too many consecutive
	// Steps have reproduced the exact same observation — see its doc
	// comment for why (traced to a REINFORCE gradient collapse, 2026-09-08).
	if same := observationsEqual(obs.Values, e.lastObsValues); same {
		e.stepsWithoutObservationChange++
	} else {
		e.stepsWithoutObservationChange = 0
	}
	e.lastObsValues = append(e.lastObsValues[:0], obs.Values...)
	if e.cfg.StuckTimeout > 0 && e.stepsWithoutObservationChange >= e.cfg.StuckTimeout {
		done = true
		doneReason = "stuck_timeout"
	}
	if e.cfg.MaxConsecutiveStepTimeouts > 0 && e.consecutiveStepTimeouts >= e.cfg.MaxConsecutiveStepTimeouts {
		done = true
		doneReason = "step_timeout"
	}
	if done {
		e.lastDoneReason = doneReason
	}

	return rl.StepResult{Observation: obs, Reward: reward, Done: done}, nil
}

// DoneReason reports why the most recent Step returned Done=true. It returns
// an empty string while the current episode is still active. This is a
// diagnostic extension; rl.Environment consumers do not need to know about
// it.
func (e *Environment) DoneReason() string {
	return e.lastDoneReason
}

// maxJitterRetries bounds Reset's already-arrived guard (see its call
// site): how many times to redraw a jittered target before giving up and
// returning an error, rather than retrying forever against a
// TargetOffset/Jitter/ArrivalThreshold combination that can never
// possibly produce a valid (non-arrived) target.
const maxJitterRetries = 20

// resetWalkabilityBudget bounds Reset's *entire* walkability retry loop
// (every jitter draw combined, each draw's own findWalkableTarget ring
// sweep, each ring candidate's own reachable() call) with one aggregate
// wall-clock deadline — see the retry loop's own comment for the
// unbounded-worst-case multiplication (maxJitterRetries * ring candidate
// count * reachabilityCheckTimeout) this closes. 45s is generous relative
// to how fast a genuinely reachable target resolves (single-digit
// milliseconds to low seconds, confirmed live) while still bounding a
// single Reset call to a small, predictable fraction of a training
// episode even in the pathological case.
const resetWalkabilityBudget = 45 * time.Second

// resetOuterRetryAttempts bounds Reset's own outer retry loop around
// resetAttempt (see Reset's doc comment): how many times to retry the
// whole attempt - fresh TeleportTo, fresh jitter draws, fresh
// walkability search - before giving up and returning an error. Worst
// case this multiplies resetWalkabilityBudget by this constant (up to
// 3*45s = 135s) before a genuinely broken Config/terrain combination
// fails loudly, still a small, bounded fraction of a training run, in
// exchange for tolerating a single unlucky attempt (a flaky TeleportTo,
// or a run of jitter draws that each needed the full ring search)
// without crashing potentially hours of otherwise-healthy training.
const resetOuterRetryAttempts = 3

// jitter adds a uniform-random offset in [-Config.Jitter[axis],
// +Config.Jitter[axis]] to base — see Config.Jitter's own doc comment. A
// zero magnitude (the default for any axis Jitter doesn't set) returns
// base unchanged without consuming from e.rng, so a Config with no jitter
// configured behaves identically, including exact RNG-call-count
// determinism for any other future rng use, to one built before Jitter
// existed.
func (e *Environment) jitter(base float64, axis int) float64 {
	magnitude := e.cfg.Jitter[axis]
	if magnitude == 0 {
		return base
	}
	return base + (e.rng.Float64()*2-1)*magnitude
}

// observationsEqual reports whether a and b are the same length and every
// element compares bit-for-bit equal — backs Config.StuckTimeout's
// "identical observation" signal (see Step).
func observationsEqual(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// awaitStep waits, bounded by Config.StepTimeout, for completion (the
// models.Completion the dispatched action returned) to resolve. Three
// outcomes:
//   - completion resolves within the timeout: its outcome is returned as
//     Step's own final position/observation already reflects it, so a
//     non-nil error here (the action's own effect failed, e.g. a
//     pathfinding error) is not itself fatal to the step — the episode
//     continues and reward is computed from wherever the bot actually
//     ended up.
//   - Config.StepTimeout elapses first: not an error (see Step's doc
//     comment) — the dispatched action may still be running in the
//     background past this call.
//   - ctx is canceled by the caller: propagated as a real error, since
//     that's a caller-initiated abort, distinguished from our own
//     StepTimeout by checking ctx's own error after the wait.
//
// resolved reports whether the action finished within the timeout; false
// means it is still running and the caller must track it (see inflight).
func (e *Environment) awaitStep(ctx context.Context, completion models.Completion) (resolved bool, err error) {
	stepCtx, cancel := context.WithTimeout(ctx, e.cfg.StepTimeout)
	defer cancel()
	if err := completion.Wait(stepCtx); err != nil && ctx.Err() != nil {
		return false, ctx.Err()
	}
	return stepCtx.Err() == nil, nil
}

// inflightAction is a dispatched action that outlived its step.
type inflightAction struct {
	cancel     context.CancelFunc
	completion models.Completion
}

// inflightCancelTimeout bounds how long cancelInflight waits for a canceled
// action to unwind before moving on.
const inflightCancelTimeout = 5 * time.Second

// cancelInflight cancels an action a timed-out step left running and waits
// (bounded) for it to unwind. Actions run in goroutines on the caller's
// context, so without this a craft started in one episode kept going across
// the next Reset and spent the new episode's freshly seeded ingredients
// (found live: a leftover bowl craft took 3 of a chest episode's 9 planks,
// leaving 6 of the 8 needed, and the unwinnable episode retried its craft
// for the whole step budget); two actions could also run at once within an
// episode, clicking the same inventory concurrently.
func (e *Environment) cancelInflight() {
	inflight := e.inflight
	e.inflight = nil
	if inflight == nil {
		return
	}
	inflight.cancel()
	waitCtx, cancel := context.WithTimeout(context.Background(), inflightCancelTimeout)
	defer cancel()
	_ = inflight.completion.Wait(waitCtx)
}

// minedBlock is a block a mine episode broke, and what it was.
type minedBlock struct {
	x, y, z int
	name    string
}

// restorableBlockName reports whether name is a real block worth putting
// back - not air and not one of BlockNameAt's "couldn't tell" placeholders.
func restorableBlockName(name string) bool {
	return name != "" && name != "minecraft:air" && name != "unknown" && !strings.HasPrefix(name, "<")
}

// restoreMinedBlocks puts back every block broken since the last Reset (see
// BlockRestorer), before the bot is moved so nothing is restored under it
// after it has walked into the hole. Best effort: a failed restore is
// logged and skipped, never fails the Reset.
func (e *Environment) restoreMinedBlocks(ctx context.Context) {
	blocks := e.minedBlocks
	e.minedBlocks = nil
	restorer, ok := e.agent.(BlockRestorer)
	if !ok {
		return
	}
	for _, b := range blocks {
		if err := restorer.RestoreBlock(ctx, b.x, b.y, b.z, b.name); err != nil {
			log.Printf("rlenv: restoring mined %s at (%d,%d,%d): %v", b.name, b.x, b.y, b.z, err)
		}
	}
}

// defaultClearAreaHeight is Config.ClearAreaHeight's default.
const defaultClearAreaHeight = 4

// clearArea wipes the space above the ground around ResetOrigin (see
// Config.ClearAreaRadius). Done before the teleport, while the area is
// still loaded around the bot. Best effort: a failure is logged, never
// fails the Reset.
func (e *Environment) clearArea(ctx context.Context) {
	if e.cfg.ClearAreaRadius <= 0 || e.cfg.ResetOrigin == nil {
		return
	}
	clearer, ok := e.agent.(AreaClearer)
	if !ok {
		return
	}
	height := e.cfg.ClearAreaHeight
	if height <= 0 {
		height = defaultClearAreaHeight
	}
	// y uses Round, not Floor: o[1] is the bot's raw, server-reported
	// feet-Y at the moment ResetOrigin was captured (applyAutoResetOrigin,
	// mc-rsi-trainer's cmd/rsi-train/main.go), typically right after
	// connecting - before physics has necessarily settled it to the clean
	// integer a resting feet-Y always converges to (groundY+1). A capture
	// landing a hair below that integer (e.g. -60.00003 instead of -60.0)
	// still means "resting on the block at -61", but Floor rounds it down
	// to -61 instead - the ground block itself, not the air cell above it
	// - so the fill below (y to y+height) wipes the ground along with it.
	// Found live 2026-10-04: repeated trainer restarts, each re-capturing
	// ResetOrigin fresh, ate through an entire 3-block soil layer down to
	// bedrock on two of ten bots over several days. Round has no such
	// one-sided bias; X/Z keep Floor since an off-by-one there only
	// shifts the clear box's footprint by a block, never reaches into the
	// ground layer the way a mis-floored Y does.
	o := *e.cfg.ResetOrigin
	x, y, z := int(math.Floor(o[0])), int(math.Round(o[1])), int(math.Floor(o[2]))
	r := e.cfg.ClearAreaRadius
	if err := clearer.ClearAir(ctx, x-r, y, z-r, x+r, y+height, z+r); err != nil {
		log.Printf("rlenv: clearing area around (%d,%d,%d): %v", x, y, z, err)
	}
}

// defaultCloneAreaDepth is Config.CloneAreaDepth's default.
const defaultCloneAreaDepth = 3

// cloneArea restores the space around ResetOrigin to a known-good template
// (see Config.CloneAreaFrom's own doc comment for why this exists instead
// of, or alongside, clearArea). Done before the teleport, same as
// clearArea, while the area is still loaded around the bot. Best effort: a
// failure is logged, never fails the Reset.
func (e *Environment) cloneArea(ctx context.Context) {
	if e.cfg.CloneAreaFrom == nil || e.cfg.ResetOrigin == nil {
		return
	}
	cloner, ok := e.agent.(AreaCloner)
	if !ok {
		return
	}
	height := e.cfg.CloneAreaHeight
	if height <= 0 {
		height = defaultClearAreaHeight
	}
	depth := e.cfg.CloneAreaDepth
	if depth <= 0 {
		depth = defaultCloneAreaDepth
	}
	r := e.cfg.CloneAreaRadius
	if r <= 0 {
		r = e.cfg.ClearAreaRadius
	}
	if r <= 0 {
		return
	}
	src := *e.cfg.CloneAreaFrom
	sx, sy, sz := int(math.Floor(src[0])), int(math.Floor(src[1])), int(math.Floor(src[2]))
	// dy is CloneAreaFrom's own Y, not ResetOrigin's - the entire point of
	// cloning over clearing (see CloneAreaFrom's doc comment): no boundary
	// is ever computed from the bot's own, possibly-imprecise position.
	o := *e.cfg.ResetOrigin
	dx, dz := int(math.Floor(o[0])), int(math.Floor(o[2]))
	if err := cloner.CloneArea(ctx, sx-r, sy-depth, sz-r, sx+r, sy+height, sz+r, dx-r, sy-depth, dz-r); err != nil {
		log.Printf("rlenv: cloning area from (%d,%d,%d) onto (%d,%d,%d): %v", sx, sy, sz, dx, sy, dz, err)
	}
}

// collectDropsTimeout bounds one collectDrops call: a few drops at a walk
// and a short wait each, well inside Config.StepTimeout's default.
const collectDropsTimeout = 6 * time.Second

// collectDrops collects the items a mine step just dropped (see
// Config.CollectDrops). Best effort: a drop that cannot be collected leaves
// the step as it was and the reward judges the inventory as it stands, so a
// failure is logged, not returned.
func (e *Environment) collectDrops(ctx context.Context) {
	collector, ok := e.agent.(models.ItemCollector)
	if !ok {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, collectDropsTimeout)
	defer cancel()
	if _, err := collector.CollectNearbyItems(cctx, collectDropsRadius); err != nil && ctx.Err() == nil {
		log.Printf("rlenv: collecting drops after a mine step: %v", err)
	}
}

// collectDropsRadius is how far around the bot collectDrops looks for drops:
// a block's drop lands within a block or two of it, and the bot stands next
// to the block.
const collectDropsRadius = 6.0

// chainActive reports whether this episode is a multi-step chain episode.
func (e *Environment) chainActive() bool { return e.cfg.ChainStage > 0 }

// chainObs is the chain block of the observation from the cached state.
func (e *Environment) chainObs() chainObs {
	if !e.chainActive() {
		return chainObs{}
	}
	return chainObs{logs: e.chainInv.logs, planks: e.chainInv.planks, tables: e.chainInv.tables, placed: e.chainTablePlaced}
}

// chainCraftTarget is what ActionCraft crafts right now in a chain episode
// ("" if nothing useful can be), from the cached inventory.
func (e *Environment) chainCraftTarget() string {
	return nextChainCraft(e.cfg.ChainStage, e.cfg.CraftTargetItem, e.chainInv, e.chainTablePlaced)
}

// refreshChain re-reads the inventory counts the chain cares about and
// whether the table this episode placed is still there.
func (e *Environment) refreshChain() {
	if !e.chainActive() {
		return
	}
	e.chainInv = chainInventory{
		logs:   e.agent.InventoryCount(chainLog),
		planks: e.agent.InventoryCount(chainPlanks),
		sticks: e.agent.InventoryCount(chainStick),
		tables: e.agent.InventoryCount(chainTable),
	}
	if e.cfg.ChainStage >= ChainPlace && e.placeAttempted {
		e.recordPlacedTable() // every table beside the bot, for removal at Reset
	}
	e.chainTablePlaced = e.placedAt != nil && e.agent.BlockNameAt(e.placedAt.x, e.placedAt.y, e.placedAt.z) == chainTable
	if !e.chainTablePlaced && e.placeAttempted && e.chainInv.tables == 0 {
		// Not where we recorded it (or never recorded), and none in hand: a
		// table the bot's view was slow to show when it was placed may be in
		// sight by now (only after a place action: see placeAttempted).
		if x, y, z, found, err := e.agent.FindVisibleBlock(context.Background(), chainTable, chainTableSearchRadius); err == nil && found {
			e.notePlacedTable(farSeededBlock{int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))})
			e.chainTablePlaced = true
		}
	}
}

// chainTableSearchRadius is how far refreshChain looks for a placed table
// that it has no record of.
const chainTableSearchRadius = 6

// recordPlacedTable finds the tables beside the bot (a place step chooses
// its own cell and reports only success) and remembers them, so they are
// seen as placed and removed at the next Reset. refreshChain runs it every
// step, so a table is found even when the step that placed it saw nothing.
func (e *Environment) recordPlacedTable() {
	if !e.chainActive() {
		return
	}
	pos, _, _, ok := e.agent.GetPosition()
	if !ok {
		return
	}
	bx, by, bz := models.StandingCell(pos)
	for _, c := range models.PlacementCandidates(bx, by, bz) {
		x, y, z := int(c.Cell.X), int(c.Cell.Y), int(c.Cell.Z)
		if e.agent.BlockNameAt(x, y, z) == chainTable {
			e.notePlacedTable(farSeededBlock{x, y, z})
		}
	}
}

// notePlacedTable remembers a table in the world: the first is "the" placed
// table (what tablePlaced tracks), and every one is queued for removal at
// the next Reset - a second table (a placement whose success the bot's view
// was slow to show, then repeated) must not be left standing.
func (e *Environment) notePlacedTable(b farSeededBlock) {
	if e.placedAt == nil {
		e.placedAt = &b
	}
	for _, s := range e.farSeeded {
		if s == b {
			return
		}
	}
	e.farSeeded = append(e.farSeeded, b)
}

// seedChain prepares a chain episode at (x, y, z), the cell two blocks beyond
// the goto target: an empty inventory, no stray drops, and a tree (a column
// of logs tall enough for the chain plus a spare) standing there for the bot
// to find.
func (e *Environment) seedChain(ctx context.Context, far FarSeedAgent, x, y, z int) error {
	chain, ok := e.agent.(ChainSeedAgent)
	if !ok {
		return errChainSeedRequiresChainSeedAgent
	}
	if err := chain.ClearInventory(ctx); err != nil {
		return fmt.Errorf("clearing the inventory for a chain episode: %w", err)
	}
	if err := chain.ClearDroppedItems(ctx, x, y, z, chainDropClearRadius); err != nil {
		return fmt.Errorf("clearing stray drops at the tree: %w", err)
	}
	height := ChainTreeHeight(e.cfg.ChainStage, e.cfg.CraftTargetItem)
	// Wipe the ground-level space around the tree first. A previous episode's
	// table is normally removed at Reset, but one this instance never
	// recorded (a crash or restart between episodes, a placement whose
	// success it could not see) would still be standing where the bot is
	// about to work, and right-clicking beside it opens its crafting screen
	// instead of placing anything. Found live.
	if clearer, ok := e.agent.(AreaClearer); ok {
		if err := clearer.ClearAir(ctx, x-chainSeedClearRadius, y, z-chainSeedClearRadius, x+chainSeedClearRadius, y+height+1, z+chainSeedClearRadius); err != nil {
			log.Printf("rlenv: clearing the chain work area around (%d,%d,%d): %v", x, y, z, err)
		}
	}
	for i := 0; i < height; i++ {
		if err := far.SeedBlockAt(ctx, e.cfg.MineTargetBlock, x, y+i, z); err != nil {
			return fmt.Errorf("seeding the chain tree: %w", err)
		}
		e.farSeeded = append(e.farSeeded, farSeededBlock{x, y + i, z})
	}
	e.chainAnchor = &farSeededBlock{x, y, z}
	return nil
}

// chainSeedClearRadius is how far around the tree base seedChain clears.
const chainSeedClearRadius = 4

// chainDropClearRadius is how far around the tree, and around the bot's
// previous position, stray drops are cleared.
const chainDropClearRadius = 16

// clearChainDrops removes what the previous chain episode left lying about:
// drops near where the bot stood and near the tree. Best effort.
func (e *Environment) clearChainDrops(ctx context.Context) {
	chain, ok := e.agent.(ChainSeedAgent)
	if !ok || (e.chainAnchor == nil && !e.chainActive()) {
		return
	}
	if pos, _, _, ok := e.agent.GetPosition(); ok {
		if err := chain.ClearDroppedItems(ctx, int(math.Floor(pos.X)), int(math.Floor(pos.Y)), int(math.Floor(pos.Z)), chainDropClearRadius); err != nil {
			log.Printf("rlenv: clearing drops around the bot: %v", err)
		}
	}
	if a := e.chainAnchor; a != nil {
		if err := chain.ClearDroppedItems(ctx, a.x, a.y, a.z, chainDropClearRadius); err != nil {
			log.Printf("rlenv: clearing drops at the tree: %v", err)
		}
	}
}

// compositeGotoTask reports whether this episode is "go to the target, then
// mine/craft there": the goto task active alongside a mine or craft task.
// Reaching the target doesn't end such an episode.
func (e *Environment) compositeGotoTask() bool {
	return !e.cfg.GoToTargetDisabled && (e.cfg.MineTargetBlock != "" || e.cfg.CraftTargetItem != "")
}

// taskActionReady reports whether the mine or craft this episode's target is
// for can be done right now: the block is visible, or the craft is ready.
func (e *Environment) taskActionReady() bool {
	return (e.cfg.MineTargetBlock != "" && e.mineVisible) || (e.cfg.CraftTargetItem != "" && e.craftReady)
}

// seedAtGoal reports whether this episode's mine block / crafting table go
// next to the goto target (Config.SeedAtGoal on a composite episode).
func (e *Environment) seedAtGoal() bool {
	return e.cfg.SeedAtGoal && e.compositeGotoTask()
}

// farSeededBlock is a block seedFarTargets placed.
type farSeededBlock struct{ x, y, z int }

// farSeedOffset leaves a full block of horizontal clearance between the goto
// target and a seeded block. A one-block offset is reachable, but a player
// navigating to the target can still enter the seeded block's collision box.
const farSeedOffset = 2

// seedFarTargets places this composite episode's mine block and/or crafting
// table (and the craft ingredients) next to the goto target, on the cell two
// blocks +X of it, so the bot must travel to find them without being spawned
// inside the placement. Placed blocks are recorded and removed at the next
// Reset.
func (e *Environment) seedFarTargets(ctx context.Context) error {
	far, ok := e.agent.(FarSeedAgent)
	if !ok {
		return errFarSeedRequiresFarSeedAgent
	}
	x, y, z := int(math.Floor(e.targetX))+farSeedOffset, int(math.Floor(e.targetY)), int(math.Floor(e.targetZ))
	if e.cfg.ChainStage > 0 {
		return e.seedChain(ctx, far, x, y, z)
	}
	if e.cfg.MineTargetBlock != "" {
		if err := far.SeedBlockAt(ctx, e.cfg.MineTargetBlock, x, y, z); err != nil {
			return fmt.Errorf("seeding mine target at the goto target: %w", err)
		}
		e.farSeeded = append(e.farSeeded, farSeededBlock{x, y, z})
	}
	if e.cfg.CraftTargetItem != "" {
		if err := far.SeedCraftIngredientsAt(ctx, e.cfg.CraftTargetItem, x, y, z); err != nil {
			return fmt.Errorf("seeding craft ingredients at the goto target: %w", err)
		}
		e.farSeeded = append(e.farSeeded, farSeededBlock{x, y, z})
	}
	return nil
}

// removeFarSeeded clears the blocks the previous episode's seedFarTargets
// placed (best effort; see BlockRestorer). They are outside the reach of
// Config.ClearAreaRadius, so without this they would pile up across the map.
func (e *Environment) removeFarSeeded(ctx context.Context) {
	blocks := e.farSeeded
	e.farSeeded = nil
	restorer, ok := e.agent.(BlockRestorer)
	if !ok {
		return
	}
	for _, b := range blocks {
		if err := restorer.RestoreBlock(ctx, b.x, b.y, b.z, "minecraft:air"); err != nil {
			log.Printf("rlenv: removing far-seeded block at (%d,%d,%d): %v", b.x, b.y, b.z, err)
		}
	}
}
