package rlenv

import (
	"context"
	"fmt"

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

	originX, originY, originZ float64
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
}

// New constructs an Environment. registry is typically actions.NewRegistry()
// (or a test double registering only what's needed); it must dispatch
// "moveto" the way actions.MoveTo does (parseFloat'd x/y/z args) for
// ActionGoToTarget/ActionReturnHome to work.
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
	return &Environment{agent: agent, registry: registry, cfg: cfg}, nil
}

// ObservationSize implements rl.Environment.
func (e *Environment) ObservationSize() int { return observationSize }

// ActionSpace implements rl.Environment.
func (e *Environment) ActionSpace() int { return NumActions }

// Reset implements rl.Environment. It does not move or respawn the bot —
// per Config.TargetOffset's doc comment, there is no real reset/teleport
// mechanism wired up yet (RSI_TRAINING_PLAN.md item 2 is still open), so
// "starting a fresh episode" means capturing wherever the bot currently is
// as this episode's origin and posing a new target relative to it, not
// actually repositioning anything.
func (e *Environment) Reset(ctx context.Context) (rl.Observation, error) {
	pos, yaw, pitch, ok := e.agent.GetPosition()
	if !ok {
		return rl.Observation{}, errPositionUnknown
	}
	x, y, z := pos.X, pos.Y, pos.Z

	e.originX, e.originY, e.originZ = x, y, z
	e.targetX = x + e.cfg.TargetOffset[0]
	e.targetY = y + e.cfg.TargetOffset[1]
	e.targetZ = z + e.cfg.TargetOffset[2]
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
	return buildObservation(x, y, z, yaw, pitch, e.targetX, e.targetY, e.targetZ, health, food, saturation, healthKnown, e.mineX, e.mineY, e.mineZ, e.mineVisible, e.craftReady), nil
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
// Movement completion, resolved via models.Completion: actions.MoveTo.Execute
// (what "moveto" dispatches to) launches MoveToWithChat in its own
// goroutine and returns immediately, but as of models.ActionRegistry's
// uniform completion signal (see models/completion.go), that goroutine's
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
	obs := buildObservation(x, y, z, yaw, pitch, e.targetX, e.targetY, e.targetZ, newHealth, food, saturation, newHealthKnown, e.mineX, e.mineY, e.mineZ, e.mineVisible, e.craftReady)
	return rl.StepResult{Observation: obs, Reward: reward, Done: done}, nil
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
