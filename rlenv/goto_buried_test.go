package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// TestGoToLegalAndTargetsExplorationPointWhenBuriedTargetUnseen is the
// core regression this session's live-training finding demanded: before
// this wiring, mine_buried's only ever-legal action was ActionWait, for
// the entire episode, every episode (confirmed live via
// rsi_action_mask_total, mc-rsi-trainer, 2026-10-26) — GoToTargetDisabled
// is unconditionally true for mine_buried (it's modeled on the plain
// "mine" task, which assumes the target is already in reach), and with no
// TargetOffset of its own either, nothing could ever move the bot at all.
// goToTargetActive/currentGoToTarget close that gap: GoToTarget becomes
// legal while a buried target is unresolved, and dispatches toward the
// exact point the observation's own unknownDx/Dy/Dz already describes —
// the same signal, not a second, disconnected one.
func TestGoToLegalAndTargetsExplorationPointWhenBuriedTargetUnseen(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true)
	env := newTestEnvironment(t, agent, buriedTestConfig())

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if !legal(env, rlenv.ActionGoToTarget) {
		t.Fatal("ActionGoToTarget not legal with a buried target unresolved — the bot has no way to move at all")
	}
	unknownDx, unknownDy, unknownDz, unknownValid := obs.Values[21], obs.Values[22], obs.Values[23], obs.Values[24]
	if unknownValid != 1 {
		t.Fatalf("unknownValid = %v, want 1 (buried target never seen, nothing explored yet)", unknownValid)
	}

	result := step(t, env, rlenv.ActionGoToTarget)
	if result.Done {
		t.Fatal("Done = true dispatching GoToTarget toward the exploration point — nothing should end the episode here")
	}

	pos, _, _, ok := agent.GetPosition()
	if !ok {
		t.Fatal("agent position should be known after Step")
	}
	wantX, wantY, wantZ := float64(unknownDx), float64(unknownDy), float64(unknownDz)
	if pos.X != wantX || pos.Y != wantY || pos.Z != wantZ {
		t.Fatalf("agent ended at (%v,%v,%v), want (%v,%v,%v) — the exploration point unknownDx/Dy/Dz pointed at from the origin", pos.X, pos.Y, pos.Z, wantX, wantY, wantZ)
	}
	// Arrival at an exploration point reuses compositeGotoTask's existing
	// one-shot arrival bonus (user's explicit choice, 2026-10-26: keep it
	// one-shot for now) — should be well over the tiny per-step time
	// penalty alone.
	if result.Reward < 5 {
		t.Fatalf("reward on arrival = %v, want the arrival bonus to have paid (compositeGotoTask: buried target configured, doesn't end the episode)", result.Reward)
	}

	// A second arrival (same point, already there) must not pay again —
	// compositeGotoTask's existing one-shot e.arrived gate.
	again := step(t, env, rlenv.ActionWait)
	if again.Reward > 0 {
		t.Fatalf("reward on a later step at the same point = %v, want no repeat arrival bonus", again.Reward)
	}
}

// TestGoToTargetsExactPositionOnceBuriedTargetSeen confirms
// currentGoToTarget's second phase: once the buried target has been
// resolved via FindVisibleBlock even once (mineEverSeen), GoToTarget
// should walk the bot precisely to it, not just to the right general
// exploration quadrant.
func TestGoToTargetsExactPositionOnceBuriedTargetSeen(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:coal_ore", 3, 0, 4) // visible from the start
	env := newTestEnvironment(t, agent, buriedTestConfig())

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if !legal(env, rlenv.ActionGoToTarget) {
		t.Fatal("ActionGoToTarget not legal with the buried target already resolved")
	}

	result := step(t, env, rlenv.ActionGoToTarget)
	if result.Done {
		t.Fatal("Done = true dispatching GoToTarget toward the resolved ore position")
	}
	pos, _, _, ok := agent.GetPosition()
	if !ok {
		t.Fatal("agent position should be known after Step")
	}
	if pos.X != 3 || pos.Y != 0 || pos.Z != 4 {
		t.Fatalf("agent ended at (%v,%v,%v), want the exact resolved ore position (3,0,4)", pos.X, pos.Y, pos.Z)
	}
}

// TestChainGatherGoToUnaffectedByBuriedWiring confirms the precision fix
// this design needed: ChainStage is fixed for a chain_mine episode's
// entire duration, so mineTargetMemoryEnabled() alone is true from step
// 1 — including during an early stage like ChainGather, well before the
// ore-search phase starts. goToTargetActive/currentGoToTarget must not
// hijack movement away from the tree during gather; this confirms goto
// still targets the chain's own fixed TargetOffset point there.
func TestChainGatherGoToUnaffectedByBuriedWiring(t *testing.T) {
	agent, env := newChainEnv(t, rlenv.ChainGather, "")
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	step(t, env, rlenv.ActionGoToTarget)
	pos, _, _, ok := agent.GetPosition()
	if !ok {
		t.Fatal("agent position should be known after Step")
	}
	// newChainEnv sets TargetOffset{12,0,0} from origin (0,-60,0).
	if pos.X != 12 || pos.Y != -60 || pos.Z != 0 {
		t.Fatalf("agent ended at (%v,%v,%v), want the chain's own fixed target (12,-60,0) — buried-search wiring must not apply during gather", pos.X, pos.Y, pos.Z)
	}
}

// TestChainMineGoToExploresBeforeOreSeenThenHomesInOnceSeen covers the
// ChainMine-stage counterpart to the two mine_buried tests above, in one
// episode driven through the real chain progression: goto must still
// target the chain's own fixed point during gather/table/place/use, then
// switch to the exploration signal once craftedGoal is true and the
// buried ore is still unseen, then switch again to the ore's exact
// position once it's been glimpsed.
func TestChainMineGoToExploresBeforeOreSeenThenHomesInOnceSeen(t *testing.T) {
	agent := newFakeAgent(0, -60, 0)
	agent.chainInv = map[string]int{}
	agent.chainBlocks = map[[3]int]string{}
	const oreX, oreY, oreZ = 12, -60, 30
	agent.chainBlocks[[3]int{oreX, oreY, oreZ}] = "minecraft:coal_ore"

	cfg := testConfig()
	cfg.TargetOffset = [3]float64{12, 0, 0}
	cfg.ArrivalThreshold = 1.5
	cfg.MineTargetBlock = "minecraft:oak_log"
	cfg.MineSearchRadius = 8
	cfg.CraftTargetItem = "minecraft:wooden_pickaxe"
	cfg.ChainStage = rlenv.ChainMine
	cfg.ChainMineTargetBlock = "minecraft:coal_ore"
	cfg.CollectDrops = true
	cfg.SeedAtGoal = true
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Gather, table, pickaxe — identical to TestChainMineEpisodeRunsTheWholeChain.
	step(t, env, rlenv.ActionGoToTarget)
	for i := 0; i < 4; i++ {
		r := step(t, env, rlenv.ActionMine)
		if r.Done {
			t.Fatalf("gathering log %d ended the episode", i+1)
		}
	}
	// Gather is the only phase that ever queries FindVisibleBlock for
	// something chainBlocksHidden would need to stay visible for (the
	// auto-seeded tree logs) - hide the ore now, before the pickaxe craft
	// loop below is what actually flips craftedGoal true and triggers the
	// very next refreshMineTarget's switch to searching for it.
	agent.chainBlocksHidden = true

	step(t, env, rlenv.ActionCraft)
	step(t, env, rlenv.ActionCraft)
	if r := step(t, env, rlenv.ActionPlace); r.Done {
		t.Fatal("placing the table ended a ChainMine episode")
	}
	craftedPickaxe := false
	for i := 0; i < 10 && !craftedPickaxe; i++ {
		r := step(t, env, rlenv.ActionCraft)
		if r.Done {
			t.Fatalf("crafting the goal item ended a ChainMine episode (craft #%d)", i+1)
		}
		craftedPickaxe = agent.chainInv["minecraft:wooden_pickaxe"] >= 1
	}
	if !craftedPickaxe {
		t.Fatalf("never crafted the pickaxe: %v", agent.chainInv)
	}

	// Now in the buried-search sub-phase: ore configured as the mine
	// target but still unresolved (chainBlocksHidden). taskActionReady
	// must ignore leftover craft-readiness here (nextChainCraft has no
	// "already crafted" check, only an ingredient-sufficiency one, so
	// chainCraftTarget can keep returning the goal item if enough
	// planks remain) — only mine-readiness should gate goto now.
	if !legal(env, rlenv.ActionGoToTarget) {
		t.Fatalf("ActionGoToTarget not legal once craftedGoal is true with the buried ore unresolved (mask=%v)", env.ActionMask())
	}
	beforeExplore := step(t, env, rlenv.ActionWait).Observation
	unknownDx, unknownDy, unknownDz, unknownValid := beforeExplore.Values[21], beforeExplore.Values[22], beforeExplore.Values[23], beforeExplore.Values[24]
	if unknownValid != 1 {
		t.Fatalf("unknownValid = %v, want 1 (buried ore configured, still unresolved)", unknownValid)
	}
	posBefore, _, _, _ := agent.GetPosition()
	r := step(t, env, rlenv.ActionGoToTarget)
	if r.Done {
		t.Fatal("Done = true dispatching GoToTarget toward the exploration point")
	}
	pos, _, _, _ := agent.GetPosition()
	wantX, wantY, wantZ := posBefore.X+float64(unknownDx), posBefore.Y+float64(unknownDy), posBefore.Z+float64(unknownDz)
	if pos.X != wantX || pos.Y != wantY || pos.Z != wantZ {
		t.Fatalf("agent ended at (%v,%v,%v), want the exploration point (%v,%v,%v), not the chain's original target", pos.X, pos.Y, pos.Z, wantX, wantY, wantZ)
	}

	// Reveal the ore (as if the bot had just dug into line of sight).
	// mineEverSeen only updates via refreshMineTarget's own post-dispatch
	// call, one step after the one that unhides it - the unhiding step's
	// own dispatch still reads the stale (hidden) state, so a throwaway
	// step is needed before the rest of this reacts to it.
	agent.chainBlocksHidden = false
	waitResult := step(t, env, rlenv.ActionWait)
	if waitResult.Observation.Values[12] != 1 {
		t.Fatalf("mineVisible = %v after unhiding the ore, want 1", waitResult.Observation.Values[12])
	}
	// GoToTarget turns back off here, by design (actionLegal's existing
	// "already arrived and the task is ready - don't waste a step going
	// anywhere, just act" rule - e.arrived was set during the very first
	// GoToTarget dispatch at the start of gather, long before this phase):
	// currentGoToTarget's exact-position homing matters when the bot
	// discovers a target without having arrived anywhere first (plain
	// mine_buried, or a chain_mine episode that happens to spot the ore
	// via blind digging before ever using goto in this sub-phase) - not
	// this specific path, where Mine itself is now the direct answer.
	if legal(env, rlenv.ActionGoToTarget) {
		t.Fatal("ActionGoToTarget still legal once the ore is visible and ready - should defer to ActionMine directly here")
	}

	// Mining it is what actually ends the episode — unchanged from
	// TestChainMineEpisodeRunsTheWholeChain.
	r = step(t, env, rlenv.ActionMine)
	if !r.Done || r.Reward < 9 {
		t.Fatalf("mining the buried block: done=%v reward=%v, want the completion bonus and done", r.Done, r.Reward)
	}
}
