package rlenv

import "math"

// The multi-step crafting chain (docs/plans/12 in mc-rsi-trainer): a tree
// out of sight -> mine logs -> craft planks -> craft a crafting table ->
// place it -> craft a goal item at it. Config.ChainStage picks how far along
// the chain an episode is asked to go.
//
// Everything here is plain logic over inventory counts, so the environment's
// use of it is thin and this can be tested exhaustively without a bot.

// Chain stages (Config.ChainStage). 0 means no chain.
const (
	// ChainGather: mine a log and have it in the inventory.
	ChainGather = 1
	// ChainTable: ... craft planks and a crafting table.
	ChainTable = 2
	// ChainPlace: ... place the table.
	ChainPlace = 3
	// ChainUse: ... craft the goal item (Config.CraftTargetItem) at the
	// placed table.
	ChainUse = 4
)

const (
	chainLog    = "minecraft:oak_log"
	chainPlanks = "minecraft:oak_planks"
	chainStick  = "minecraft:stick"
	chainTable  = "minecraft:crafting_table"

	// chainMilestoneBonus is paid once, the first time each intermediate
	// milestone of the episode's chain is reached; chainCompletionBonus when
	// the stage's own goal is. Three or four milestones sum to well under
	// the completion bonus, so finishing always beats collecting milestones,
	// and each is paid once so crafting and un-crafting cannot farm them.
	chainMilestoneBonus     float32 = 2.0
	chainCompletionBonus    float32 = 10.0
	chainPlanksPerLog               = 4
	chainTablePlanks                = 4
	chainSticksPerCraft             = 4
	chainPlanksPerSticks            = 2
	chainTreeSpareLogs              = 1
	chainInventoryCountNorm         = 8.0
)

// chainGoalRecipe is what a stage-4 goal item costs in planks and sticks.
type chainGoalRecipe struct{ planks, sticks int }

// chainGoals are the goal items a stage-4 episode can ask for.
var chainGoals = map[string]chainGoalRecipe{
	"minecraft:bowl":           {planks: 3},
	"minecraft:chest":          {planks: 8},
	"minecraft:wooden_pickaxe": {planks: 3, sticks: 2},
}

// ChainGoalKnown reports whether item can be a stage-4 goal.
func ChainGoalKnown(item string) bool {
	_, ok := chainGoals[item]
	return ok
}

// chainInventory is the slice of the inventory the chain cares about.
type chainInventory struct {
	logs, planks, sticks, tables int
}

// chainPlanksNeeded is how many planks a stage's whole chain consumes.
func chainPlanksNeeded(stage int, goal string) int {
	need := 0
	if stage >= ChainTable {
		need += chainTablePlanks
	}
	if stage >= ChainUse {
		g := chainGoals[goal]
		need += g.planks
		if g.sticks > 0 {
			need += chainPlanksPerSticks * ((g.sticks + chainSticksPerCraft - 1) / chainSticksPerCraft)
		}
	}
	return need
}

// ChainLogsNeeded is how many logs an episode at this stage (and goal item,
// for stage 4) consumes: what it takes to make its planks, at least one.
func ChainLogsNeeded(stage int, goal string) int {
	logs := int(math.Ceil(float64(chainPlanksNeeded(stage, goal)) / chainPlanksPerLog))
	if logs < 1 {
		logs = 1
	}
	return logs
}

// ChainTreeHeight is how many logs the seeded tree has: the logs the chain
// needs plus a spare, so a dropped or missed log does not make an episode
// unwinnable.
func ChainTreeHeight(stage int, goal string) int {
	return ChainLogsNeeded(stage, goal) + chainTreeSpareLogs
}

// nextChainCraft is what "craft" should craft right now for a chain episode:
// the next missing link toward the stage's goal, given what is held and
// whether the table is placed. Empty means nothing useful can be crafted
// yet (mine more logs, or place the table).
//
// The policy still has to learn when to gather, craft and place; this only
// spares it choosing between recipes.
func nextChainCraft(stage int, goal string, inv chainInventory, tablePlaced bool) string {
	if stage < ChainTable {
		return ""
	}
	haveTable := inv.tables > 0 || tablePlaced
	if !haveTable {
		switch {
		case inv.planks >= chainTablePlanks:
			return chainTable
		case inv.logs > 0:
			return chainPlanks
		}
		return ""
	}
	if stage < ChainUse || !tablePlaced {
		// Stages 2 and 3 end at the table; stage 4 needs it placed first.
		return ""
	}
	g := chainGoals[goal]
	switch {
	case g.sticks > 0 && inv.sticks < g.sticks:
		if inv.planks >= chainPlanksPerSticks {
			return chainStick
		}
		if inv.logs > 0 {
			return chainPlanks
		}
		return ""
	case inv.planks >= g.planks:
		return goal
	case inv.logs > 0:
		return chainPlanks
	}
	return ""
}

// chainPlaceLegal reports whether placing the table makes sense now: one is
// held and none is placed yet, in an episode that reaches that far.
func chainPlaceLegal(stage int, inv chainInventory, tablePlaced bool) bool {
	return stage >= ChainPlace && inv.tables > 0 && !tablePlaced
}

// chainState tracks which milestones an episode has reached; each pays once.
type chainState struct {
	gotLog, gotPlanks, gotTable, placed bool
}

// advance folds the inventory and table state after a step into the state
// and returns the reward it earns and whether the episode's stage is
// complete. goalCrafted is whether the goal item's count rose this step
// (only meaningful at ChainUse).
func (s *chainState) advance(stage int, inv chainInventory, tablePlaced, goalCrafted bool) (reward float32, done bool) {
	// Milestones in chain order; completion is whichever one the stage ends on.
	type milestone struct {
		reached bool
		flag    *bool
	}
	ms := []milestone{
		{inv.logs > 0, &s.gotLog},
		{inv.planks > 0, &s.gotPlanks},
		{inv.tables > 0 || tablePlaced, &s.gotTable},
		{tablePlaced, &s.placed},
		{goalCrafted, new(bool)}, // the goal has no persistent flag: it ends the episode
	}
	end := map[int]int{ChainGather: 0, ChainTable: 2, ChainPlace: 3, ChainUse: 4}[stage]
	for i, m := range ms {
		if i > end || !m.reached || *m.flag {
			continue
		}
		*m.flag = true
		if i == end {
			reward += chainCompletionBonus
			done = true
		} else {
			reward += chainMilestoneBonus
		}
	}
	return reward, done
}
