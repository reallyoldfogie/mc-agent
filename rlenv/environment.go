package rlenv

import (
	"context"
	"fmt"
	"math/rand"
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

	// mineX/Y/Z/mineVisible cache the last-resolved nearest visible
	// Config.MineTargetBlock instance (see resolveMineTarget), the same
	// way prevDistance/prevHealth cache the last-known state of their own
	// features — refreshed once per Step (and once at Reset) rather than
	// re-resolved multiple times within one call.
	mineX, mineY, mineZ float64
	mineVisible         bool

	// craftCount caches the last-known held count of Config.CraftTargetItem
	// (see craftCountNow), read at the start of each Step as "before this
	// step" state — the craft analogue of prevDistance/prevHealth's caching
	// shape, simpler than mine's since there's no position to resolve, just
	// a count. craftReady caches the last-computed Craftable() result, for
	// the observation returned to the caller (see craftReadyNow).
	craftCount int
	craftReady bool

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
	}

	e.stepsWithoutObservationChange = 0

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
		e.cfg.TargetOffset = override.TargetOffset
		e.cfg.MineTargetBlock = override.MineTargetBlock
		e.cfg.MineSearchRadius = override.MineSearchRadius
		e.cfg.CraftTargetItem = override.CraftTargetItem
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
			wx, wy, wz, walkableFound = findWalkableTarget(walkabilityCtx, walkAgent, e.agent, e.targetX, e.targetY, e.targetZ)
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
	e.episodeStarted = true

	health, food, saturation, healthKnown := e.agent.Health()
	e.prevHealth, e.prevHealthKnown = health, healthKnown

	if e.cfg.Seeder != nil {
		seedAgent, ok := e.agent.(SeedAgent)
		if !ok {
			return rl.Observation{}, errSeederRequiresSeedAgent
		}
		if err := e.cfg.Seeder(ctx, seedAgent, e.cfg); err != nil {
			return rl.Observation{}, fmt.Errorf("rlenv: episode seeding: %w", err)
		}
	}

	if err := e.refreshMineTarget(ctx); err != nil {
		return rl.Observation{}, err
	}
	e.craftCount = e.craftCountNow()
	e.craftReady = e.craftReadyNow()
	obs := buildObservation(x, y, z, yaw, pitch, e.targetX, e.targetY, e.targetZ, health, food, saturation, healthKnown, e.mineX, e.mineY, e.mineZ, e.mineVisible, e.craftReady, !e.cfg.GoToTargetDisabled, e.cfg.MineTargetBlock != "", e.cfg.CraftTargetItem != "")
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
// or false if Config.CraftTargetItem is unset.
func (e *Environment) craftReadyNow() bool {
	if e.cfg.CraftTargetItem == "" {
		return false
	}
	return e.agent.Craftable(e.cfg.CraftTargetItem)
}

// refreshMineTarget re-resolves the nearest currently-visible instance of
// Config.MineTargetBlock and stores it into e.mineX/Y/Z/mineVisible, for
// observation/reward purposes. Called once at Reset and once per Step
// (after that step's dispatch — see Step), not more: unlike TargetOffset's
// fixed point, mining consumes its target, so the nearest visible instance
// can genuinely change step to step as blocks are broken (docs/plans/
// RL_ACTION_SPACE_EXPANSION.md Phase 2a) and needs re-resolving regularly,
// but Step reads the *previous* call's cached result as "before this step"
// state (see its mined-this-step check) rather than re-resolving twice per
// call — the same caching shape prevDistance/prevHealth already use.
// Leaves e.mineVisible=false without error if MineTargetBlock is unset (the
// common case for instances that don't pose a mining task) or if
// FindVisibleBlock simply finds nothing in range; only a genuine
// FindVisibleBlock error (e.g. world/block-manager not ready) is returned.
//
// Known cost tradeoff, not solved here: this runs FindVisibleBlock once per
// step whenever MineTargetBlock is configured, regardless of which action
// was dispatched, plus a second FindVisibleBlock call inside the "mine"
// action's own Execute on steps that actually dispatch ActionMine (see
// action.go's resolveDispatch). Acceptable for now; revisit (e.g. shrinking
// MineSearchRadius, or resolving less often) if it matters in practice.
func (e *Environment) refreshMineTarget(ctx context.Context) error {
	if e.cfg.MineTargetBlock == "" {
		e.mineX, e.mineY, e.mineZ, e.mineVisible = 0, 0, 0, false
		return nil
	}
	x, y, z, found, err := e.agent.FindVisibleBlock(ctx, e.cfg.MineTargetBlock, e.cfg.MineSearchRadius)
	if err != nil {
		return fmt.Errorf("rlenv: resolving mine target: %w", err)
	}
	e.mineX, e.mineY, e.mineZ, e.mineVisible = x, y, z, found
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

	if shouldDispatch {
		completion, err := e.registry.Execute(ctx, dispatch.name, e.agent, dispatch.args)
		if err != nil {
			return rl.StepResult{}, fmt.Errorf("rlenv: dispatching %s: %w", dispatch.name, err)
		}
		if err := e.awaitStep(ctx, completion); err != nil {
			return rl.StepResult{}, err
		}
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

	newDistance := distance3(x, y, z, e.targetX, e.targetY, e.targetZ)
	reward, done := computeReward(stepOutcome{
		prevDistance:      e.prevDistance,
		newDistance:       newDistance,
		prevHealth:        e.prevHealth,
		newHealth:         newHealth,
		healthKnownBefore: e.prevHealthKnown,
		healthKnownAfter:  newHealthKnown,
	})
	if newDistance <= e.cfg.ArrivalThreshold {
		reward += arrivalBonus
		done = true
	}
	if mined {
		reward += mineRewardBonus
		done = true
	}
	if craftedThisStep {
		reward += craftRewardBonus
		done = true
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
	// Same refresh for craft state: newCraftCount is already this step's
	// post-dispatch value, so it becomes the next Step's "before" read
	// directly; craftReady is recomputed since inventory contents may have
	// changed even when craftedThisStep is false (e.g. an ingredient was
	// picked up, not the target item itself).
	e.craftCount = newCraftCount
	e.craftReady = e.craftReadyNow()
	obs := buildObservation(x, y, z, yaw, pitch, e.targetX, e.targetY, e.targetZ, newHealth, food, saturation, newHealthKnown, e.mineX, e.mineY, e.mineZ, e.mineVisible, e.craftReady, !e.cfg.GoToTargetDisabled, e.cfg.MineTargetBlock != "", e.cfg.CraftTargetItem != "")

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
	}

	return rl.StepResult{Observation: obs, Reward: reward, Done: done}, nil
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
func (e *Environment) awaitStep(ctx context.Context, completion models.Completion) error {
	stepCtx, cancel := context.WithTimeout(ctx, e.cfg.StepTimeout)
	defer cancel()
	if err := completion.Wait(stepCtx); err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}
