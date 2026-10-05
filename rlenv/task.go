package rlenv

import (
	"math/rand"
	"time"
)

// Config configures an Environment. There is deliberately no notion of a
// task *type* here (see doc.go): TargetOffset is the one task this
// environment knows how to pose, "reach a point offset from wherever Reset
// found the bot."
type Config struct {
	// TargetOffset is added to the bot's position at Reset to produce this
	// episode's target (Environment.targetX/Y/Z). Using an offset from the
	// live position, rather than a fixed absolute world coordinate, is a
	// deliberate workaround for not having a real reset/teleport mechanism
	// yet (RSI_TRAINING_PLAN.md item 2, "Episode reset strategy" — still
	// unverified whether mc-agent's RCON dependency is wired up for this):
	// every Reset poses a reachable, bot-relative task without needing to
	// move the bot anywhere.
	TargetOffset [3]float64

	// ArrivalThreshold is the distance (in blocks) within which the bot
	// counts as having reached a target.
	ArrivalThreshold float64

	// StepTimeout bounds how long one Step waits for a dispatched
	// movement action's models.Completion to resolve before giving up on
	// *this step* (see environment.go's Step doc comment). The dispatched
	// action may still be running in the background past this deadline —
	// StepTimeout bounds one Step call, not the action itself.
	StepTimeout time.Duration

	// MineTargetBlock, if set, is the block name (e.g. "minecraft:stone")
	// this episode's mining task targets — enables ActionMine, following
	// TargetOffset's "environment poses the task" pattern
	// (docs/plans/RL_ACTION_SPACE_EXPANSION.md Phase 2a option (a)). Empty
	// (the default) means this Environment instance doesn't pose a mining
	// task: ActionMine becomes a safe no-op, the same way ActionWait always
	// is, rather than an error — see action.go's resolveDispatch.
	MineTargetBlock string

	// MineSearchRadius bounds FindVisibleBlock's search when locating the
	// nearest visible MineTargetBlock instance, in blocks. Ignored if
	// MineTargetBlock is empty. <= 0 defers to FindVisibleBlock's own
	// built-in default (8 blocks, see agent/actions.go) rather than being
	// validated here.
	MineSearchRadius int

	// CraftTargetItem, if set, is the item name (e.g. "minecraft:stick")
	// this episode's crafting task targets — enables ActionCraft, following
	// MineTargetBlock's exact "environment poses the task" pattern
	// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1a). Empty (the default)
	// means this Environment instance doesn't pose a crafting task:
	// ActionCraft becomes a safe no-op, not an error — see action.go's
	// resolveDispatch. Independent of MineTargetBlock/TargetOffset: one
	// instance can pose a move-to target, a mine task, and a craft task
	// simultaneously.
	CraftTargetItem string

	// CraftSearchRadius, if > 0, makes the craft task need a crafting table
	// within this many blocks: the craftReady observation feature (and so
	// ActionCraft's mask) is true only while a table is visible that close,
	// on top of the ingredients being held. It is the crafting counterpart of
	// MineSearchRadius gating ActionMine, and exists for the same reason:
	// mc-agent's craft action searches 32 blocks for a table and walks there
	// itself, so without this gate a table 20 blocks away is crafted at
	// without the policy ever travelling. Only meaningful for a recipe that
	// needs a table; for one that fits the 2x2 inventory grid no table is
	// ever seeded and the craft would never become ready. <= 0 (the default)
	// applies no table condition. Ignored if CraftTargetItem is empty.
	CraftSearchRadius int

	// Seeder, if set, is called once per Reset to prepare the world for
	// this episode's configured task(s) — see EpisodeSeeder's doc comment
	// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4) for exactly what this
	// does and doesn't cover. nil (the default) leaves Reset's existing
	// behavior unchanged. Requires the Environment's agent to also satisfy
	// SeedAgent — Reset returns an error if Seeder is set but it doesn't.
	Seeder EpisodeSeeder

	// ResetOrigin, if set, is where Environment.Reset teleports the bot
	// (via RCON — see ResetAgent) before capturing this episode's
	// origin/target, replacing TargetOffset's own doc comment's "wherever
	// Reset found the bot" workaround with an actual reset. nil (the
	// default) leaves that workaround in place, unchanged, for callers
	// without RCON wired up (e.g. unit tests, or a chat-command-driven
	// session). Requires the Environment's agent to also satisfy
	// ResetAgent — Reset returns an error if ResetOrigin is set but it
	// doesn't.
	//
	// Found necessary, not merely nice-to-have: without a real teleport, a
	// policy that converges on a non-progressing action has no way to
	// escape that spot across episode boundaries either — every Reset
	// just re-poses the same relative target from the same stuck
	// position, so the observation the policy sees never changes and
	// neither does its behavior. Traced live to a REINFORCE gradient
	// collapse (a collaborative debugging session with cRL-go, 2026-09-08)
	// — StuckTimeout below addresses the within-episode half of that,
	// this field addresses the across-episode half.
	ResetOrigin *[3]float64

	// StuckTimeout, if > 0, ends the current episode (Step returns
	// Done=true) once this many consecutive Steps have produced a
	// bit-for-bit identical observation vector to the previous Step's — a
	// generic, task-agnostic "no progress of any kind is happening"
	// signal (position, health/food, mine/craft state all unchanged).
	// Found necessary by the same debugging session ResetOrigin's doc
	// comment references: without this, a stalled policy could run an
	// entire episode producing a batch of identical (observation, action,
	// advantage) triples whose REINFORCE gradient contributions cancel to
	// the float32 noise floor. 0 (the default) disables the check,
	// matching the previous unbounded (episode_len-only) behavior.
	StuckTimeout int

	// MaxConsecutiveStepTimeouts, if > 0, ends the current episode once that
	// many dispatched Steps in a row hit StepTimeout without their action
	// resolving. StuckTimeout can't catch a wedged bot whose position jitters
	// by float noise each step (found live: a bot straddling the edge of a
	// slot it could never descend into, re-dispatching a craft that timed out
	// every step), so its episode ran the whole step budget - about 17
	// minutes at a 10s StepTimeout - while every other environment waited on
	// it at the epoch barrier. A step whose action resolves resets the count;
	// steps that dispatch nothing leave it unchanged. 0 (the default)
	// disables the check.
	MaxConsecutiveStepTimeouts int

	// ClearAreaRadius, if > 0 (and ResetOrigin is set and the agent is an
	// AreaClearer), makes each Reset fill with air the box extending this
	// many blocks either side of ResetOrigin in X and Z, from ResetOrigin's
	// own Y (the bot's feet level, i.e. the first cell above the ground) up
	// ClearAreaHeight further blocks - leaving the ground itself untouched.
	// For flat training worlds; in real terrain it would delete trees and
	// hills. Keep (2r+1)^2 * (height+1) under 32,768 (vanilla's fill limit).
	// 0 (the default) disables it. Ignored if CloneAreaFrom is also set -
	// see its own doc comment for why that's the better choice when it's
	// available at all.
	ClearAreaRadius int
	// ClearAreaHeight is how many blocks above ResetOrigin's Y to clear;
	// 4 if left 0 while ClearAreaRadius is set.
	ClearAreaHeight int

	// CloneAreaFrom, if set (and ResetOrigin is set and the agent is an
	// AreaCloner), replaces ClearAreaRadius's "fill air above a boundary
	// computed from ResetOrigin" with "clone a known-good template region
	// onto this episode's working area" each Reset: a pristine reference
	// patch - ground layers and the air space above them - copied as one
	// rigid unit. This heals ground damage the same pass that clears
	// leftover placed-block debris, and - the actual point of it over
	// ClearAreaRadius - never computes a destroy/preserve boundary from a
	// live, possibly-imprecise position reading: CloneAreaFrom's own Y is
	// used for the destination too, unconditionally, never ResetOrigin's.
	// Found worth building live 2026-10-04: ClearAreaRadius's origin-Y
	// floor rounding (since fixed - see clearArea's history) ate through
	// an entire 3-block soil layer down to bedrock on two of ten bots over
	// several days; cloning removes that whole hazard class rather than
	// making one instance of it less likely.
	//
	// Same flat-world precondition ClearAreaRadius already documents,
	// stated more precisely here because this field's correctness depends
	// on it directly: every working area this episode's ResetOrigin could
	// ever land in must share CloneAreaFrom's own ground height exactly.
	// CloneAreaFrom's own X/Z must be a location no ResetOrigin (now or
	// after -parallel-envs scales up) will ever land a working area on or
	// near - e.g., mc-rsi-trainer's shared-server working areas
	// (pkg/parallelenv.WorkingAreaOffset) only ever move along X, so
	// anywhere on the Z axis is guaranteed untouched regardless of how
	// many environments get added later.
	CloneAreaFrom *[3]float64
	// CloneAreaRadius is CloneAreaFrom's half-width/depth in blocks (both
	// the source template and every destination working area use the same
	// radius); 0 (the default) falls back to ClearAreaRadius if that's
	// set, otherwise disables cloning.
	CloneAreaRadius int
	// CloneAreaHeight is how many blocks above CloneAreaFrom's own Y to
	// include - the air-clearing portion, same role as ClearAreaHeight; 4
	// if left 0 while CloneAreaFrom is set.
	CloneAreaHeight int
	// CloneAreaDepth is how many blocks below CloneAreaFrom's own Y to
	// include - the ground-healing portion ClearAreaRadius never had; 3
	// if left 0 while CloneAreaFrom is set (this run's own soil depth -
	// stop short of bedrock, which never changes so never needs healing).
	CloneAreaDepth int

	// TimePenalty is the reward subtracted every step, independent of outcome
	// (see reward.go's timePenalty for what it's for). 0 means the built-in
	// default, 0.01. The default is too small to matter next to a +10 success
	// bonus: a policy that wastes ten steps loses 0.1, so nothing pushes it
	// to be efficient (found live: 38-70% of all steps were the Wait action,
	// and mine_far episodes took ~10.7 steps where ~3 suffice). Must not be
	// negative.
	TimePenalty float32

	// NormalizeObservation scales every observation feature to roughly
	// [-1, 1] (see NormalizedObservation), instead of handing the policy raw
	// blocks and degrees. A network fed raw values (yaw up to +-180, distances
	// of tens of blocks) against ~0.1-magnitude weights saturates its first
	// layer immediately, which with a learning rate that isn't tiny kills the
	// ReLU units outright - one such run had every second-layer unit dead after
	// two generations and never learned again. Off by default so a policy
	// trained on the raw layout keeps working; a checkpoint trained with it on
	// must be run with it on.
	NormalizeObservation bool

	// ChainStage, if non-zero, makes this a multi-step crafting-chain
	// episode (chain.go): 1 gather a log, 2 ... craft planks and a table,
	// 3 ... place the table, 4 ... craft CraftTargetItem (a chainGoals item)
	// at the placed table. Enables ActionPlace, makes ActionCraft craft the
	// next missing link toward the stage's goal instead of CraftTargetItem
	// directly, replaces the mine and craft bonuses with milestone rewards,
	// and adds the inventory-count features to the observation. Needs
	// MineTargetBlock (the tree's log), SeedAtGoal and a Seeder whose agent
	// implements ChainSeedAgent and FarSeedAgent (the tree is seeded out of
	// sight, and the inventory and stray drops cleared each episode).
	ChainStage int

	// CollectDrops makes a mine step also collect the drops it produced, so
	// the mined item is in the inventory when the step's observation and
	// reward are computed. Breaking a block leaves an item entity on the
	// ground that the bot only collects if it happens to be within about a
	// block of where it lands (drops land up to a block and a half away, and
	// cannot be collected for half a second of server time), so without this
	// a task whose next step needs the mined item - crafting planks from a
	// log - fails at random. Off by default: existing mine tasks do not use
	// the item, and collecting costs a fraction of a second per mine step.
	// Needs an agent implementing models.ItemCollector; without one it is a
	// no-op.
	CollectDrops bool

	// SeedAtGoal is Config's copy of TaskOverride.SeedAtGoal (set per
	// episode by Config.TaskSelector; may also be set statically).
	SeedAtGoal bool

	// Jitter adds a uniform-random offset in [-Jitter[i], +Jitter[i]] to
	// axis i of both ResetOrigin (if set) and TargetOffset, drawn fresh
	// every Reset — see JitterSeed for the source. [3]float64{} (the
	// default) disables jitter entirely: every axis stays exactly as
	// configured, matching pre-Jitter behavior bit-for-bit.
	//
	// Found necessary, not merely nice-to-have, by the same debugging
	// session ResetOrigin/StuckTimeout's own doc comments reference: fixing
	// the across-episode "stuck forever" bug (ResetOrigin) and the
	// within-episode one (StuckTimeout) turned out not to be sufficient for
	// a policy to actually learn anything. A fixed ResetOrigin/TargetOffset
	// pair, once the policy converges toward a single dominant action, has
	// every episode replay the *exact* same trajectory — still an
	// exact-repeat, gradient-cancelling batch every epoch, just now bounded
	// instead of unbounded. Jitter closes that gap: even a fully
	// deterministic policy now sees a genuinely different (origin, target)
	// pair, and therefore a genuinely different observation, every episode
	// — confirmed live: without Jitter, GradientNorm stayed pinned at the
	// float32 noise floor for 1999 of 2000 epochs even with ResetOrigin and
	// StuckTimeout both configured.
	Jitter [3]float64

	// JitterSeed seeds Jitter's random source (one per Environment,
	// created in New). Zero is a perfectly valid, deterministic seed
	// (matches math/rand.NewSource(0)) — not a sentinel for "disabled";
	// Jitter's own zero value ([3]float64{}) is what disables jitter, not
	// this field. A fixed JitterSeed makes a training run's episode
	// starting conditions reproducible run to run, the same way
	// cRL-go/crlconfig's own Train.Seed does for the policy side.
	JitterSeed int64

	// GoToTargetDisabled, if true, makes ActionGoToTarget illegal this
	// episode (Environment.actionLegal, ActionMask, resolveDispatch),
	// mirroring how an empty MineTargetBlock/CraftTargetItem already makes
	// ActionMine/ActionCraft illegal. Unlike those two, ActionGoToTarget has
	// no natural "unset" representation on TargetOffset itself — TargetOffset's
	// own doc comment already establishes that its zero value ([3]float64{})
	// is a legitimate, intentional configuration ("the target is the bot's
	// own Reset position"), not a sentinel for "no goto task" — so this
	// needed its own explicit field rather than overloading TargetOffset.
	// Named so its zero value (false) preserves today's behavior exactly:
	// every existing caller that never sets this field keeps
	// ActionGoToTarget unconditionally legal, unchanged. See TaskSelector
	// below for the mechanism that actually sets this per episode.
	GoToTargetDisabled bool

	// TaskSelector, if set, is called once per Reset (after any
	// Config.ResetOrigin teleport, before this episode's target/jitter is
	// computed) to choose which task(s) are active this specific episode,
	// promoting docs/plans/RL_TRAINING_LOOP_PLAN.md's live-verified
	// newAlternatingMineOrCraftSeeder proof-of-concept
	// (../mc-rsi-trainer/docs/plans/01-curriculum-generator.md item 4) into
	// a reusable capability: its TaskOverride return value's five fields
	// (GoToTargetDisabled, TargetOffset, MineTargetBlock, MineSearchRadius,
	// CraftTargetItem) overwrite this Config's own same-named fields for the
	// rest of that episode — every other field (ArrivalThreshold,
	// StepTimeout, Seeder, ResetOrigin, StuckTimeout, Jitter, JitterSeed,
	// TaskSelector itself) is untouched, so a TaskSelector only has to
	// describe what varies, not repeat every mechanics field it doesn't
	// care about. nil (the default) leaves Reset's existing static-Config
	// behavior completely unchanged — see Environment.Reset.
	//
	// Deliberately mutates this Environment's own Config in place at Reset
	// rather than computing a separate "effective" config alongside it: the
	// original static values passed to New are not meant to be recovered
	// once a TaskSelector starts choosing per-episode — the whole point is
	// that it fully owns task selection from that point on — and every
	// other read site in this package already reads Config's task fields
	// directly (e.g. Environment.actionLegal, refreshMineTarget), so this
	// way nothing else needed to change to pick up per-episode overrides.
	TaskSelector TaskSelector
}

// TaskSelector chooses which task(s) are active for one episode. episode
// is the 0-indexed count of Reset calls made against this Environment so
// far (0 for the very first episode); rng is this Environment's own
// jitter random source (Config.JitterSeed), shared rather than given a
// separate seed so a fixed JitterSeed still makes an entire run's episode
// conditions — task selection included — reproducible end to end. See
// Config.TaskSelector's own doc comment for exactly how the returned
// TaskOverride is applied.
type TaskSelector func(episode int, rng *rand.Rand) TaskOverride

// TaskOverride is TaskSelector's return value: the subset of Config that
// can vary per episode. Leaving a field at its zero value disables that
// task for the episode — GoToTargetDisabled: false is the one exception,
// since false already means "not disabled" (see its own doc comment on
// Config); a TaskSelector that wants GoToTarget active this episode must
// set TargetOffset to a real value AND leave GoToTargetDisabled false,
// exactly mirroring how MineTargetBlock/CraftTargetItem must be
// explicitly non-empty to enable their tasks.
type TaskOverride struct {
	// CollectDrops is Config.CollectDrops for this episode.
	CollectDrops bool

	// ChainStage is Config.ChainStage for this episode.
	ChainStage int

	// SeedAtGoal, for a composite episode (goto active alongside mine or
	// craft), places the mine block or crafting table next to the goto
	// target instead of beside the bot, so the bot has to travel to find
	// it. Ignored unless the goto task and a mine or craft task are both
	// active. See Environment.seedFarTargets.
	SeedAtGoal bool

	GoToTargetDisabled bool
	TargetOffset       [3]float64
	MineTargetBlock    string
	MineSearchRadius   int
	CraftTargetItem    string
	CraftSearchRadius  int
}

// DefaultConfig returns reasonable production defaults; TargetOffset must
// still be set by the caller (there's no sensible default for "how far
// away should the goal be"). MineTargetBlock is left empty — set it
// explicitly to opt an Environment instance into posing a mining task.
func DefaultConfig() Config {
	return Config{
		ArrivalThreshold: 1.5,
		StepTimeout:      10 * time.Second,
		// Matches actions/commands.go's own mineSearchRadius default for
		// the "mine <blockName>" chat command — kept as a separate literal
		// here since that constant is unexported.
		MineSearchRadius: 32,
	}
}

func (c Config) validate() error {
	if c.ArrivalThreshold <= 0 {
		return errArrivalThreshold
	}
	if c.StepTimeout <= 0 {
		return errStepTimeout
	}
	if c.TimePenalty < 0 {
		return errTimePenalty
	}
	if c.ChainStage < 0 || c.ChainStage > ChainUse {
		return errChainStage
	}
	return nil
}
