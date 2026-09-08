package rlenv

import "time"

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
	return nil
}
