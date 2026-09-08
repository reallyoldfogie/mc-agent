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
