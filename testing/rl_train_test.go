package testing

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/reallyoldfogie/cRL-go/pkg/checkpoint"
	"github.com/reallyoldfogie/cRL-go/pkg/config"
	"github.com/reallyoldfogie/cRL-go/pkg/policy"
	"github.com/reallyoldfogie/cRL-go/pkg/reinforce"
	"github.com/reallyoldfogie/cRL-go/pkg/rl"
	"github.com/stretchr/testify/require"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// This file is the permanent, automated version of the manual live-server
// verification described in docs/plans/RL_TRAINING_LOOP_PLAN.md's Phase 5
// update — that phase's own note explicitly left "a permanent, automated
// live-server test" as follow-up work rather than attempting it there;
// this closes that gap.
//
// Deliberately run against a single version (see rlTrainTestVersion), not
// models.StandardVersionTests — nothing under test here is
// protocol-version-specific (rlenv/reinforce/cmd/rl-train all sit above
// the already-version-tested agent primitives they dispatch to: MoveTo,
// Mine, FindVisibleBlock, RCON setblock), so looping across every version
// would just be repeated server-startup cost for no additional coverage.
// Mirrors the precedent already set by TestElytraUnequippingStopsGliding's
// sibling investigation note ("spot-checked ... against 1.21.1 to confirm
// the fix doesn't regress already-working ... interactions") for the same
// reasoning: a single representative version is enough when the thing
// being tested doesn't vary by version.
const rlTrainTestVersion = "1.21.5"

// rlTrainSettings returns a small, fast config.Settings for these tests —
// values chosen to keep episodes/epochs short against a real server (each
// step is real network + game-tick time, unlike a toy environment), not to
// resemble a real training run's hyperparameters. Workers is 1, matching
// docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 3's requirement for a single
// live bot session.
func rlTrainSettings(epochs int) config.Settings {
	return config.Settings{
		Epochs:        epochs,
		RolloutSize:   1,
		EpisodeLen:    10,
		Gamma:         0.99,
		LearningRate:  0.05,
		GridSize:      36, // unused by rlenv (fixed ObservationSize/ActionSpace) — see rlconfig.Settings' own doc comment on this known wart
		HiddenSize:    16,
		Seed:          1,
		Workers:       1,
		ClipEpsilon:   0.2,
		EntropyCoef:   0.01,
		ValueCoef:     0.5,
		GAELambda:     0.95,
		PPOEpochs:     1,
		MinibatchSize: 4,
	}
}

// TestRLTrainingLoop_BasicTaskTrainsAndCheckpointRoundTrips reproduces
// Phase 5's Run 1 (docs/plans/RL_TRAINING_LOOP_PLAN.md): construct
// rlenv.Environment against a real live bot session, drive it through
// cRL-go's REINFORCE trainer via reinforce.NewWithPersistentEnv exactly the
// way cmd/rl-train does, run a couple of real epochs, and confirm a saved
// checkpoint reloads cleanly. No mine/craft task — just the base
// GoToTarget/ReturnHome/Wait vocabulary.
func TestRLTrainingLoop_BasicTaskTrainsAndCheckpointRoundTrips(t *testing.T) {
	env := setupStandaloneTestForEntity(t, "rl_train_basic", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{3, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	settings := rlTrainSettings(2)
	// PersistentEnvFactory is only ever invoked once by
	// reinforce.NewWithPersistentEnv — see rlenv/environment.go's own doc
	// comment and cmd/rl-train/main.go's identical pattern.
	persistentFactory := func(*rand.Rand) (rl.Environment, error) { return rlEnv, nil }

	trainer, err := reinforce.NewWithPersistentEnv(settings, persistentFactory, nil)
	require.NoError(t, err, "construct trainer")

	for epoch := 0; epoch < settings.Epochs; epoch++ {
		stats, err := trainer.RunEpoch(env.Ctx, epoch)
		require.NoError(t, err, "RunEpoch %d", epoch)
		t.Logf("epoch %d: average return %.3f, samples %d", stats.Epoch, stats.AverageReturn, stats.SampleCount)
	}

	// Same environmentID shape cmd/rl-train/main.go builds — see its own
	// doc comment on why (guards a checkpoint against loading into a
	// mismatched observation/action-space shape).
	environmentID := fmt.Sprintf("mc-agent-rlenv-test:actions=%d:obs=%d", rlEnv.ActionSpace(), rlEnv.ObservationSize())
	checkpointPath := filepath.Join(t.TempDir(), "checkpoint.json")
	meta := checkpoint.Metadata{Epoch: settings.Epochs - 1}
	require.NoError(t, policy.SaveFile(checkpointPath, trainer.Params(), environmentID, meta), "save checkpoint")

	reloaded, reloadedMeta, err := policy.LoadFile(checkpointPath, environmentID)
	require.NoError(t, err, "reload checkpoint")
	require.NotNil(t, reloaded, "reloaded params should not be nil")
	require.Equal(t, settings.Epochs-1, reloadedMeta.Epoch, "reloaded metadata should match what was saved")

	t.Log("✓ RL training loop against a live server ran end to end and its checkpoint round-tripped")
}

// TestRLTrainingLoop_MineTaskSeedingEarnsRewardAndEndsEpisode reproduces
// Phase 5's Run 2: the mine task combined with RCON episode seeding
// (rlenv.DefaultEpisodeSeeder, docs/plans/RL_TRAINING_LOOP_PLAN.md Phase
// 4) — the genuinely new, first-time-exercised-live path, not just a
// rerun of already-covered mine/craft chat-command behavior. Drives
// Environment.Reset/Step directly (not through a randomly-initialized
// policy) so the assertions are deterministic: force ActionMine and check
// the seeded block actually gets mined and rewarded, rather than hoping a
// random policy happens to choose it within a short episode.
func TestRLTrainingLoop_MineTaskSeedingEarnsRewardAndEndsEpisode(t *testing.T) {
	env := setupStandaloneTestForEntity(t, "rl_train_mine_seed", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = env.Agent.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent (the test framework already wires RCON into agent.Config.RCON for every spawned agent)")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:stone",
		// Small on purpose: a live run against MineSearchRadius=16 in a
		// region with no matching block anywhere nearby was found to take
		// a very long time (see this plan's Phase 5 update on
		// FindVisibleBlock's exhaustive-search cost) — 4 blocks is both
		// realistic for what episode seeding actually needs (something
		// near the bot, not far away) and confirmed fast in that same
		// investigation.
		MineSearchRadius: 4,
		Seeder:           rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	obs, err := rlEnv.Reset(env.Ctx)
	require.NoError(t, err, "Reset (includes RCON episode seeding)")
	require.Equal(t, float32(1), obs.Values[12], "mineVisible should be 1 right after seeding placed a stone block within search radius")

	// See stepUntilDone's own doc comment for why this retries rather than
	// asserting Done after a single Step call: MineBlockAt's "finished"
	// signal doesn't guarantee this bot's local BlockNameAt tracking has
	// caught up within that same call — found live, not anticipated, by
	// this exact test.
	result := stepUntilDone(t, rlEnv, env.Ctx, rlenv.ActionMine, rlTrainStepRetryAttempts)
	require.True(t, result.Done, "episode should end once the seeded block is mined (within %d attempts)", rlTrainStepRetryAttempts)
	require.Greater(t, result.Reward, float32(5), "reward should include mineRewardBonus")

	t.Log("✓ RCON episode seeding + ActionMine dispatch + reward all confirmed against a live server")
}

// TestRLTrainingLoop_CraftTaskSeedingEarnsRewardAndEndsEpisode is
// TestRLTrainingLoop_MineTaskSeedingEarnsRewardAndEndsEpisode's craft
// analogue — the gap docs/plans/RL_TRAINING_LOOP_PLAN.md's Phase 5 update
// explicitly left open ("only the mine path is exercised... a reasonable
// next addition, not attempted here"). Confirms SeedCraftIngredients (whose
// own fix was previously verified only by code-reading parity with
// SeedNearbyBlock's proven fix, not by a live run of its own) actually
// works end to end: RCON give → craftReady observation → ActionCraft
// dispatch → craftRewardBonus.
func TestRLTrainingLoop_CraftTaskSeedingEarnsRewardAndEndsEpisode(t *testing.T) {
	env := setupStandaloneTestForEntity(t, "rl_train_craft_seed", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = env.Agent.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent (the test framework already wires RCON into agent.Config.RCON for every spawned agent)")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		// "minecraft:stick" fits the player's own 2x2 inventory grid (no
		// crafting table needed — see agent/craft.go's fitsInventoryGrid)
		// and is tag-gated (#minecraft:planks), the same recipe
		// testing/craft_test.go's TestCraftItem_StickFromPlanks already
		// proves CraftItem itself handles correctly. Using it here
		// isolates what's actually new (SeedCraftIngredients + the
		// ActionCraft dispatch/reward chain) from recipe-resolution
		// correctness, which is already covered elsewhere and doesn't
		// need re-proving through rlenv.
		CraftTargetItem: "minecraft:stick",
		Seeder:          rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	obs, err := rlEnv.Reset(env.Ctx)
	require.NoError(t, err, "Reset (includes RCON episode seeding)")
	require.Equal(t, float32(1), obs.Values[13], "craftReady should be 1 right after seeding gave the recipe's ingredients")

	// Same live-server-sync tolerance as the mine test above — CraftItem's
	// own real inventory-click sequence is exactly the same class of
	// real-round-trip action MineBlockAt is, so the same class of
	// finished-but-not-yet-locally-visible race is expected here too.
	result := stepUntilDone(t, rlEnv, env.Ctx, rlenv.ActionCraft, rlTrainStepRetryAttempts)
	require.True(t, result.Done, "episode should end once the seeded ingredients are crafted (within %d attempts)", rlTrainStepRetryAttempts)
	require.Greater(t, result.Reward, float32(5), "reward should include craftRewardBonus")

	t.Log("✓ RCON ingredient seeding + ActionCraft dispatch + reward all confirmed against a live server")
}

// TestRLTrainingLoop_ActionMaskExcludesUnconfiguredTasksLive is the live
// end-to-end check for cRL-go's action masking (rl.ActionMasker,
// docs/plans/19-training-time-action-masking.md in ../cRL-go, resolved at
// commit d3a3153; rlenv.Environment implements it, see
// rlenv/action.go's ActionMask). Unit coverage already confirms
// Environment.ActionMask's own legality logic (rlenv/environment_test.go);
// what only a live run can confirm is that the real, published cRL-go
// pipeline this environment feeds — policy.Actor.Act, in turn
// reinforce.SampleMaskedAction — actually excludes an illegal action when
// driven by a real bot session's real ActionMask() output end to end, the
// same call shape pkg/reinforce's own rollout loop uses
// (collectTrajectoryFromEnv: `actor.Act(observation, mask, rng)`).
//
// This Environment instance never configures Mine/Craft, so their mask
// entries are always false (see TestActionMaskAllLegalWhenNoTaskConfigured
// for the equivalent fake-agent-backed assertion) — sampling repeatedly
// against a freshly, randomly initialized policy (so nothing biases
// sampling toward or away from any particular action) must never return
// ActionMine or ActionCraft, deterministically, not just "rarely": a
// correctly implemented mask makes an illegal action's post-mask
// probability exactly zero, not merely small.
func TestRLTrainingLoop_ActionMaskExcludesUnconfiguredTasksLive(t *testing.T) {
	env := setupStandaloneTestForEntity(t, "rl_train_action_mask", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{3, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		// MineTargetBlock/CraftTargetItem deliberately left unset — the
		// whole point is exercising the "unconfigured task" mask entries
		// against a real observation from a real bot session.
	})
	require.NoError(t, err, "construct rlenv.Environment")

	obs, err := rlEnv.Reset(env.Ctx)
	require.NoError(t, err, "Reset")

	mask := rlEnv.ActionMask()
	require.Equal(t, []bool{true, true, false, false}, mask,
		"Wait/GoToTarget legal, Mine/Craft illegal with nothing configured")

	rng := rand.New(rand.NewPCG(1, 2))
	params := policy.NewParams(rng, rlEnv.ObservationSize(), 16, rlEnv.ActionSpace())
	actor, err := policy.NewActor(params)
	require.NoError(t, err, "construct policy.Actor")

	const sampleCount = 200
	seenGoToTarget := false
	for i := 0; i < sampleCount; i++ {
		action, err := actor.Act(obs, mask, rng)
		require.NoError(t, err, "Act sample %d", i)
		require.NotEqual(t, rlenv.ActionMine, action, "sample %d: masked-illegal ActionMine was sampled", i)
		require.NotEqual(t, rlenv.ActionCraft, action, "sample %d: masked-illegal ActionCraft was sampled", i)
		if action == rlenv.ActionGoToTarget {
			seenGoToTarget = true
		}
	}
	// Sanity check that this actually exercised a real, non-degenerate
	// distribution over the two legal actions rather than e.g. a broken
	// mask that (by accident) always zeroes out every entry except one —
	// with 200 draws from a freshly random-initialized network, seeing
	// only ActionWait would itself be worth investigating.
	require.True(t, seenGoToTarget, "200 samples never once drew the other legal action (ActionGoToTarget) — suspiciously degenerate")

	t.Log("✓ Action masking confirmed end to end against a live server: 200 real policy.Actor.Act samples against a real Environment.ActionMask() output never returned an illegal action")
}

// rlTrainStepRetryAttempts bounds stepUntilDone's retry loop.
const rlTrainStepRetryAttempts = 5

// stepUntilDone dispatches action against rlEnv repeatedly, up to
// maxAttempts times, until a Step result reports Done — tolerating the
// live-server world-state-sync lag both TestRLTrainingLoop_
// MineTaskSeedingEarnsRewardAndEndsEpisode and TestRLTrainingLoop_
// CraftTaskSeedingEarnsRewardAndEndsEpisode found live: MineBlockAt/
// CraftItem's own "finished" signal doesn't guarantee this bot's locally
// tracked world state has caught up within that same Step call. Not a
// weakening of either assertion — Environment.Step's own reward design
// already only promises "judged from world-state deltas checked every
// step," never "on any one particular step" (see mineRewardBonus/
// craftRewardBonus's doc comments), and a real training loop calls Step
// repeatedly with no artificial pause, so this gap self-corrects within
// another step or two of real time regardless. Fails the test (via
// require.NoError) on any real Step error; simply returns the last result
// if maxAttempts is exhausted without Done, leaving the caller's own
// require.True(result.Done, ...) to report that failure with full context.
func stepUntilDone(t *testing.T, rlEnv *rlenv.Environment, ctx context.Context, action rl.Action, maxAttempts int) rl.StepResult {
	t.Helper()
	var result rl.StepResult
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err = rlEnv.Step(ctx, action)
		require.NoError(t, err, "Step attempt %d", attempt)
		if result.Done {
			return result
		}
		t.Logf("attempt %d: not done yet (reward=%.3f) — retrying", attempt, result.Reward)
	}
	return result
}

// TestRLTrainingLoop_LongRunShowsLearningOnGoToTargetLive is a long
// (wall-clock-bounded, not epoch-count-bounded) live-server run whose
// purpose is answering the one thing every short verification run so far
// (Phase 5's 1-2 epochs, this file's other tests' single-digit epochs,
// the FvbFixBot check's 4 epochs) has left open, per
// docs/plans/RL_TRAINING_LOOP_PLAN.md's own "Still open" note: does
// gradient descent against real live-bot rollouts actually improve the
// policy over time, or does REINFORCE's plumbing just happen not to
// crash?
//
// Deliberately scoped to the GoToTarget-only task (Config.MineTargetBlock/
// CraftTargetItem left unset, same Config TestRLTrainingLoop_
// BasicTaskTrainsAndCheckpointRoundTrips already verified live) rather
// than Mine/Craft: its reward is dense (distance progress every step, not
// only on task completion) and its credit-assignment problem is close to
// trivial — the only real choice being learned is "prefer ActionGoToTarget
// over ActionWait," since GoToTarget's own dispatched pathfinding (not the
// RL policy) handles how to get there. That makes this the cleanest
// possible "does learning happen at all" signal: a real pipeline bug
// (loss computation, gradient application, sampling) should already show
// up here, well before a harder sparse-reward task would be a fair test
// of anything. Mine/Craft learning is explicitly NOT what this test
// answers — a separate, harder follow-up if this one passes.
//
// This is an observational run, not a fixed pass/fail gate: REINFORCE
// against a live, high-variance, single-rollout-per-epoch signal is not
// guaranteed to have visibly improved by any particular stopping point,
// so a "no visible improvement yet" outcome logs clearly rather than
// failing the test outright — converting this into a hard regression gate
// is a reasonable follow-up once a real run establishes what magnitude of
// improvement to expect.
//
// Skipped by default (env-var gated, MCAGENT_LONG_RL_TRAIN_TEST=1): unlike
// every other test in this file, this intentionally runs for minutes, not
// seconds, and running it unconditionally would make a bare `go test
// ./testing/...` unpredictably slow — exactly what this repo's own
// testing/ live-server-cost convention warns against.
//
// **First live run (2026-09-10): learning confirmed.** 907 epochs in
// 9m33s (stopped by env.Ctx's own fixed 10-minute deadline — see
// runBudget's doc comment below — not by exhausting the intended budget).
// Average return started near/below zero (epoch 0: 12.189, epoch 10:
// -0.096, epoch 20: -0.096 — an undertrained policy still frequently
// choosing ActionWait) and had fully converged to a stable 11.576 by
// roughly epoch 30, staying there for the remaining ~870 epochs (a
// deterministic outcome once the policy reliably picks ActionGoToTarget
// immediately every episode, given this task's near-trivial
// credit-assignment problem — see this function's own doc comment above).
// First-quarter vs. last-quarter mean (10.236 vs. 11.576) understates how
// fast this actually happened, since most of the first quarter (226 of
// 907 epochs) was already past the ~30-epoch convergence point. This
// answers RL_TRAINING_LOOP_PLAN.md's "Still open: whether the agent
// actually learns" note in the affirmative, for the base task specifically
// — Mine/Craft's much harder sparse-reward learning remains unanswered.
func TestRLTrainingLoop_LongRunShowsLearningOnGoToTargetLive(t *testing.T) {
	if os.Getenv("MCAGENT_LONG_RL_TRAIN_TEST") == "" {
		t.Skip("set MCAGENT_LONG_RL_TRAIN_TEST=1 to run this multi-minute live training run")
	}

	env := setupStandaloneTestForEntity(t, "rl_train_long_run", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{3, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	// Epochs is deliberately very large — runBudget (wall-clock), not
	// epoch count, is the real stopping condition here, since each
	// episode's cost is real network/game-tick time, not a fixed compute
	// budget. runCtx (not env.Ctx directly) bounds that, so the loop below
	// can stop cleanly and log a summary instead of being killed mid-epoch
	// by `go test`'s own -timeout.
	settings := rlTrainSettings(1_000_000)
	persistentFactory := func(*rand.Rand) (rl.Environment, error) { return rlEnv, nil }
	trainer, err := reinforce.NewWithPersistentEnv(settings, persistentFactory, nil)
	require.NoError(t, err, "construct trainer")

	// runBudget is aspirational, not the real ceiling: env.Ctx itself
	// (setupStandaloneTestWithModeAndBlockPlacement, container_standalone_test.go)
	// carries its own fixed 10-minute context.WithTimeout, so
	// context.WithTimeout below actually resolves to whichever deadline is
	// sooner — found live, not anticipated: the first real run here
	// stopped at 9m33s/907 epochs with "context deadline exceeded" well
	// under this 18-minute value. Left higher than 10 minutes anyway (contrast
	// with a value like 9 minutes) so a future bump to that framework
	// timeout is picked up automatically without this file needing a
	// matching edit.
	const runBudget = 18 * time.Minute
	runCtx, cancelRun := context.WithTimeout(env.Ctx, runBudget)
	defer cancelRun()

	returns := make([]float64, 0, 512)
	start := time.Now()
	for epoch := 0; epoch < settings.Epochs; epoch++ {
		stats, err := trainer.RunEpoch(runCtx, epoch)
		if err != nil {
			if runCtx.Err() != nil {
				t.Logf("stopping at epoch %d after %s (run budget reached): %v", epoch, time.Since(start).Round(time.Second), err)
				break
			}
			require.NoError(t, err, "RunEpoch %d", epoch)
		}
		returns = append(returns, float64(stats.AverageReturn))
		if epoch%10 == 0 {
			t.Logf("epoch %d (%s elapsed): average return %.3f", epoch, time.Since(start).Round(time.Second), stats.AverageReturn)
		}
	}

	require.NotEmpty(t, returns, "no epochs completed within the run budget")
	t.Logf("completed %d epochs in %s", len(returns), time.Since(start).Round(time.Second))

	quarter := max(1, len(returns)/4)
	firstMean := meanFloat64(returns[:quarter])
	lastMean := meanFloat64(returns[len(returns)-quarter:])
	t.Logf("first-quarter mean return: %.3f (n=%d) | last-quarter mean return: %.3f (n=%d)", firstMean, quarter, lastMean, quarter)

	if lastMean > firstMean {
		t.Logf("✓ average return improved over the run (%.3f -> %.3f, delta %.3f) — learning is happening, at least on this dense-reward task", firstMean, lastMean, lastMean-firstMean)
	} else {
		t.Logf("✗ average return did NOT improve over the run (%.3f -> %.3f) — the training pipeline may not be learning even on this near-trivial task; investigate before trusting it on anything harder", firstMean, lastMean)
	}
}

// TestRLTrainingLoop_LongRunShowsLearningOnMineTaskLive is
// TestRLTrainingLoop_LongRunShowsLearningOnGoToTargetLive's Mine-task
// analogue, answering the harder half of the question that test's own doc
// comment left open: does learning happen on a task with a real,
// exclusive-to-one-action bonus, not just "prefer moving over waiting"?
//
// Isolating that cleanly needs a different Config shape than simply adding
// MineTargetBlock to the GoToTarget test's config, for a reward-design
// reason found while designing this test, not a pre-existing bug:
// reward.go's distance term is *unscaled* per block
// (distanceRewardScale=1.0), so a GoToTarget task with any meaningfully
// large TargetOffset would let the policy bank far more reward from raw
// walking distance than mineRewardBonus's flat +10 — making Mine the
// *worse* choice whenever the target is more than ~10 blocks away, the
// opposite of an isolating test. Making the target far away (the first,
// wrong idea tried here) makes this worse, not better, since achievable
// distance-reward scales with how far the target is.
//
// The fix: TargetOffset is [0,0,0] — the target *is* the bot's own Reset
// position. rlenv/task.go's own doc comment confirms this is an
// intentional, supported configuration, not a guarded-against trap ("a
// deterministic TargetOffset that happens to be within ArrivalThreshold
// is the caller's explicit, visible choice"), and Config.validate() only
// requires ArrivalThreshold/StepTimeout > 0, nothing about TargetOffset's
// magnitude. The effect: arrival (and its flat +10 arrivalBonus) is
// already satisfied the instant Reset returns, so environment.go's
// `newDistance <= ArrivalThreshold` check ends every episode after
// exactly one Step, regardless of which action was chosen — Wait,
// GoToTarget (which dispatches toward a target it's already at, moving
// nowhere), and an unconfigured Craft all net the same flat +10 floor.
// Mine, seeded fresh every Reset via rlenv.DefaultEpisodeSeeder
// (guaranteeing mineVisible=1 — see rlenv/seed.go), is the only action
// that can add another +10 (mineRewardBonus) on top when it actually
// mines. This collapses the problem to exactly the same shape as the
// GoToTarget test's: a one-decision-per-episode comparison between a
// strictly-better action (here, Mine, worth ~20 total) and every other
// action (worth ~10) — clean enough that the expected numbers are
// predictable in advance: an untrained, roughly-uniform-over-4-actions
// policy should average around 0.25*20 + 0.75*10 = 12.5 early on, rising
// toward ~20 if the policy learns to prefer Mine when it's visible
// (mineVisible is index 12 of the observation — the signal is literally
// there for the policy to condition on).
//
// See TestRLTrainingLoop_LongRunShowsLearningOnGoToTargetLive's own doc
// comment for why this is observational (not a hard pass/fail gate) and
// why it's env-var gated the same way (MCAGENT_LONG_RL_TRAIN_TEST=1).
//
// **First live run (2026-09-10): learning NOT clearly confirmed — a
// materially weaker result than the GoToTarget test's.** 537 epochs in
// 9m0s. First-quarter mean return 10.662 (n=134), last-quarter mean
// 11.333 (n=134) — a real but small improvement (+0.672), nowhere near
// this task's ~20 ceiling, and the every-10th-epoch samples logged during
// the run show ActionMine's ~20-return outcome scattered roughly evenly
// across the whole run (epochs 180/190/250/360/490/520), not clustering
// late the way it would if the policy were reliably converging toward
// preferring it. Contrast with the GoToTarget test's decisive, complete
// convergence by ~epoch 30. Two plausible explanations were considered:
// (a) genuinely harder credit assignment — REINFORCE with RolloutSize=1
// (one high-variance sample per gradient step) may need many more than
// ~500 episodes to reliably separate a 2x reward difference (10 vs. 20)
// from noise, unlike GoToTarget's much larger, unambiguous per-step
// distance signal; (b) a live-timing confound — TestRLTrainingLoop_
// MineTaskSeedingEarnsRewardAndEndsEpisode's own doc comment documents
// MineBlockAt's "finished" signal not always being reflected in this
// bot's local BlockNameAt tracking within the same call. **(b) checked and
// RULED OUT** by TestRLTrainingLoop_MineActionRegistersImmediatelyLive
// (2026-09-10): 30/30 deterministic ActionMine dispatches against a
// freshly seeded, visible target registered mined=true on the very first
// Step, no retries needed — the mine reward signal is clean when Mine is
// actually chosen. That left (a), REINFORCE's single-rollout variance, as
// the more likely bottleneck, and it's since been confirmed directly, not
// just by elimination: TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLargerRolloutLive
// (2026-09-10) reran this exact Config with RolloutSize=8 instead of 1 and
// saw a decisive improvement (first-quarter mean 9.990 -> last-quarter
// 14.434, vs. this run's marginal 10.662 -> 11.333) — see that test's own
// doc comment for the full result.
// Bottom line: unlike the base task, Mine-task learning is not yet
// demonstrated by this run; treat it as still open.
func TestRLTrainingLoop_LongRunShowsLearningOnMineTaskLive(t *testing.T) {
	if os.Getenv("MCAGENT_LONG_RL_TRAIN_TEST") == "" {
		t.Skip("set MCAGENT_LONG_RL_TRAIN_TEST=1 to run this multi-minute live training run")
	}

	env := setupStandaloneTestForEntity(t, "rl_train_long_run_mine", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = env.Agent.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{0, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:stone",
		MineSearchRadius: 4,
		Seeder:           rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	// See the GoToTarget version of this test for why Epochs is huge and
	// runBudget (not epoch count) is the real stopping condition, and why
	// it's lower than 10 minutes to leave headroom under env.Ctx's own
	// fixed 10-minute deadline (container_standalone_test.go) rather than
	// racing it — found live in that test's own first run.
	settings := rlTrainSettings(1_000_000)
	persistentFactory := func(*rand.Rand) (rl.Environment, error) { return rlEnv, nil }
	trainer, err := reinforce.NewWithPersistentEnv(settings, persistentFactory, nil)
	require.NoError(t, err, "construct trainer")

	const runBudget = 9 * time.Minute
	runCtx, cancelRun := context.WithTimeout(env.Ctx, runBudget)
	defer cancelRun()

	returns := make([]float64, 0, 512)
	start := time.Now()
	for epoch := 0; epoch < settings.Epochs; epoch++ {
		stats, err := trainer.RunEpoch(runCtx, epoch)
		if err != nil {
			if runCtx.Err() != nil {
				t.Logf("stopping at epoch %d after %s (run budget reached): %v", epoch, time.Since(start).Round(time.Second), err)
				break
			}
			require.NoError(t, err, "RunEpoch %d", epoch)
		}
		returns = append(returns, float64(stats.AverageReturn))
		if epoch%10 == 0 {
			t.Logf("epoch %d (%s elapsed): average return %.3f", epoch, time.Since(start).Round(time.Second), stats.AverageReturn)
		}
	}

	require.NotEmpty(t, returns, "no epochs completed within the run budget")
	t.Logf("completed %d epochs in %s", len(returns), time.Since(start).Round(time.Second))

	quarter := max(1, len(returns)/4)
	firstMean := meanFloat64(returns[:quarter])
	lastMean := meanFloat64(returns[len(returns)-quarter:])
	t.Logf("first-quarter mean return: %.3f (n=%d) | last-quarter mean return: %.3f (n=%d)", firstMean, quarter, lastMean, quarter)

	if lastMean > firstMean {
		t.Logf("✓ average return improved over the run (%.3f -> %.3f, delta %.3f) — the policy is learning to prefer ActionMine when it's visible", firstMean, lastMean, lastMean-firstMean)
	} else {
		t.Logf("✗ average return did NOT improve over the run (%.3f -> %.3f) — the policy may not be learning to use the mine bonus; investigate", firstMean, lastMean)
	}
}

// TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLargerRolloutLive
// tests the surviving explanation from
// TestRLTrainingLoop_LongRunShowsLearningOnMineTaskLive's weak result:
// REINFORCE's single-rollout variance (RolloutSize=1 — one episode's
// return backs the entire gradient estimate for that update) may simply
// need more samples per update to reliably separate a 2x reward
// difference from noise, given TestRLTrainingLoop_MineActionRegistersImmediatelyLive
// already ruled out a noisy/delayed reward signal as the cause.
//
// Same Config as the RolloutSize=1 version (TargetOffset: [0,0,0], seeded
// Mine task — see that test's own doc comment for why this specific shape
// isolates the comparison), same ~9-minute wall-clock budget, only
// settings.RolloutSize changed (1 -> 8): each gradient update now averages
// over 8 episodes' returns instead of 1, trading update *count* (fewer
// gradient steps fit in the same wall-clock budget, since total episode
// throughput is roughly fixed by real per-episode network/game-tick cost
// regardless of how episodes are grouped into epochs) for update
// *quality* (each step's gradient estimate is less noisy). If variance
// really is the bottleneck, this should show visibly cleaner/faster
// convergence in far fewer epochs than the 537 the RolloutSize=1 run
// needed to barely move; if it doesn't help either, that's evidence
// against the variance explanation too, not just for it.
//
// **First live run (2026-09-10): variance hypothesis CONFIRMED, not just
// surviving by elimination.** 74 epochs (592 episodes) in 9m0s.
// Epochs 0-55 (440 episodes) sat dead flat at average return 9.990 with
// return std 0.005 — 0/8 episodes mined, every single epoch, the same
// "never mines" trap the RolloutSize=1 run showed, just more starkly
// visible here since std makes "all 8 samples identical" explicit. Then,
// around epoch 56 (~39s elapsed), it broke out: return climbed to a
// stable 14.990 by epoch 59 and held there through epoch 73 — almost
// exactly the midpoint between 9.99 (no mine) and 19.99 (mine), i.e. the
// policy shifted to mining in roughly 4 of 8 episodes per epoch and
// plateaued. First-quarter mean 9.990 -> last-quarter mean 14.434, a
// decisive improvement, qualitatively different from the RolloutSize=1
// run's marginal 10.662 -> 11.333 over a comparable wall-clock budget.
// **Extended rerun (2026-09-10, same day): plateau confirmed to be a
// stable equilibrium, not "needs more time."** Using
// MCAGENT_TEST_TIMEOUT=27m + MCAGENT_LONG_RL_TRAIN_RUN_BUDGET=25m (see
// runBudget's own doc comment below and
// setupStandaloneTestWithModeAndBlockPlacement's timeout override), the
// same task ran 100 epochs (800 episodes) in 25m0s. Average return
// reached 14.990 by epoch 53 (1m11s elapsed) and then held there —
// return std pinned at exactly 5.000 — through every one of the
// remaining 47 epochs (376 episodes, ~24 more minutes) with zero further
// movement. First-quarter mean 9.990 -> last-quarter mean 14.990. Nearly
// 24 minutes of additional training produced literally no improvement
// beyond the plateau this test's first (9-minute) run already found —
// this rules out "needs more time" conclusively. The remaining candidate
// explanation (the entropy coefficient, 0.01 in rlTrainSettings, holding
// a stable ~50/50 mixed strategy rather than letting the policy converge
// to deterministic "always mine") was not itself tested here (that would
// mean varying EntropyCoef, not RolloutSize or run length) — flagged as
// the next thing to check, not confirmed.
func TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLargerRolloutLive(t *testing.T) {
	if os.Getenv("MCAGENT_LONG_RL_TRAIN_TEST") == "" {
		t.Skip("set MCAGENT_LONG_RL_TRAIN_TEST=1 to run this multi-minute live training run")
	}

	env := setupStandaloneTestForEntity(t, "rl_train_long_run_mine_rollout8", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = env.Agent.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{0, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:stone",
		MineSearchRadius: 4,
		Seeder:           rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	settings := rlTrainSettings(1_000_000)
	settings.RolloutSize = 8
	persistentFactory := func(*rand.Rand) (rl.Environment, error) { return rlEnv, nil }
	trainer, err := reinforce.NewWithPersistentEnv(settings, persistentFactory, nil)
	require.NoError(t, err, "construct trainer")

	// 9 minutes by default (see the RolloutSize=1 version's own doc
	// comment for why 9, not env.Ctx's full 10) — overridable via
	// MCAGENT_LONG_RL_TRAIN_RUN_BUDGET (paired with a matching, larger
	// MCAGENT_TEST_TIMEOUT — see setupStandaloneTestWithModeAndBlockPlacement's
	// own doc comment — since this budget is capped by whichever of the
	// two deadlines env.Ctx and runBudget itself is sooner) to check
	// whether the ~50% mine-rate plateau this test's first run hit is a
	// stable equilibrium or would keep improving given more time.
	runBudget := 9 * time.Minute
	if raw := os.Getenv("MCAGENT_LONG_RL_TRAIN_RUN_BUDGET"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		require.NoError(t, err, "parse MCAGENT_LONG_RL_TRAIN_RUN_BUDGET %q", raw)
		runBudget = parsed
	}
	runCtx, cancelRun := context.WithTimeout(env.Ctx, runBudget)
	defer cancelRun()

	returns := make([]float64, 0, 128)
	start := time.Now()
	for epoch := 0; epoch < settings.Epochs; epoch++ {
		stats, err := trainer.RunEpoch(runCtx, epoch)
		if err != nil {
			if runCtx.Err() != nil {
				t.Logf("stopping at epoch %d after %s (run budget reached): %v", epoch, time.Since(start).Round(time.Second), err)
				break
			}
			require.NoError(t, err, "RunEpoch %d", epoch)
		}
		returns = append(returns, float64(stats.AverageReturn))
		// Fewer epochs expected than the RolloutSize=1 run (each one costs
		// ~8x the episodes), so log every epoch rather than every 10th —
		// still readable, and doesn't risk missing the whole trend in a
		// short run.
		t.Logf("epoch %d (%s elapsed): average return %.3f (return std %.3f, samples %d)",
			epoch, time.Since(start).Round(time.Second), stats.AverageReturn, stats.ReturnStd, stats.SampleCount)
	}

	require.NotEmpty(t, returns, "no epochs completed within the run budget")
	t.Logf("completed %d epochs (%d episodes) in %s", len(returns), len(returns)*settings.RolloutSize, time.Since(start).Round(time.Second))

	quarter := max(1, len(returns)/4)
	firstMean := meanFloat64(returns[:quarter])
	lastMean := meanFloat64(returns[len(returns)-quarter:])
	t.Logf("first-quarter mean return: %.3f (n=%d epochs) | last-quarter mean return: %.3f (n=%d epochs)", firstMean, quarter, lastMean, quarter)

	if lastMean > firstMean {
		t.Logf("✓ average return improved over the run (%.3f -> %.3f, delta %.3f) — larger RolloutSize helped the policy learn to prefer ActionMine", firstMean, lastMean, lastMean-firstMean)
	} else {
		t.Logf("✗ average return did NOT improve over the run (%.3f -> %.3f) — larger RolloutSize alone did not fix Mine-task learning; the variance explanation may be wrong, or need an even larger RolloutSize/more total episodes than this run's budget allowed", firstMean, lastMean)
	}
}

// TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLowEntropyLive tests
// the one candidate explanation TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLargerRolloutLive's
// extended rerun left standing after ruling out "needs more time": that
// EntropyCoef (0.01 in rlTrainSettings) is holding the policy at a stable
// ~50/50 mixed-strategy equilibrium (return ~14.99, confirmed unmoving
// across 24+ minutes and 376+ episodes) rather than letting it converge to
// deterministic "always mine" (return ~20). REINFORCE's entropy bonus
// rewards keeping the action distribution non-degenerate; if its pull
// balances the reward gradient's pull toward Mine at roughly 50/50, that
// would explain the exact equilibrium already observed — this test checks
// that by setting EntropyCoef to 0 (removing the counter-pull entirely,
// the cleanest single-variable test) rather than merely lowering it, on
// top of the already-confirmed-necessary RolloutSize=8 fix (so this
// isolates entropy specifically, not re-litigating variance). Same
// mine-task Config, same runBudget override mechanism as the RolloutSize=8
// version (env.Ctx via MCAGENT_TEST_TIMEOUT, this run's own budget via
// MCAGENT_LONG_RL_TRAIN_RUN_BUDGET — see that test's own doc comments).
//
// Real risk this test doesn't control for, noted rather than solved: zero
// entropy removes REINFORCE's only defense against premature convergence
// to the *wrong* deterministic policy (e.g. locking onto "never mine"
// before the reward gradient ever gets a real signal) — entropy_coef=0.01
// already reliably broke out of exactly that trap in both prior runs, so
// this test's own result needs reading in light of that tradeoff: a
// failure to reach ~20 here doesn't automatically vindicate the entropy
// hypothesis if it instead reveals a *different* failure mode (e.g.
// collapsing to all-Wait/all-GoToTarget instead).
//
// **First live run (2026-09-10): entropy hypothesis FALSIFIED, not
// confirmed — the exact same plateau, reached later.** 324 epochs (2592
// episodes) in 25m0s. This test's own automated verdict (first-quarter
// 10.005, last-quarter 12.845, logged as "improved but didn't reach the
// ceiling") is a misleading read of what actually happened: because
// non-mining epochs are far cheaper than mining ones (no RCON block
// placement needed), this run raced through epochs 0-275 dead flat at
// 9.990 in under a minute, then broke out at epoch 276 and locked to
// EXACTLY 14.990 by epoch 278 — the identical value
// TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLargerRolloutLive's
// EntropyCoef=0.01 run converged to — and held there for the remaining 46
// epochs (~24 more minutes), just as rock-solid as the entropy=0.01 case.
// The "last-quarter mean 12.845" is an artifact of splitting 324 epochs
// into quarters by epoch *count*: the last quarter (epochs 243-324) still
// spans part of the long, cheap dead prefix, blending it with the
// plateau — not a clean read of the converged state. (Lesson for any
// future long-run test with this lopsided a cost structure: quartile
// splits by epoch count can be misleading; splitting by elapsed wall time,
// or reporting the trailing-N-epoch mean directly, would read more
// truthfully.) Conclusion: EntropyCoef changed how many epochs it took to
// escape the initial "never mines" trap (276 here vs. ~52-56 at 0.01) but
// did NOT change where the policy ultimately settles — the ~50% mine-rate
// equilibrium is not an entropy artifact. Something else, structural to
// this reward/optimization setup, produces that exact equilibrium; not
// identified here.
func TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLowEntropyLive(t *testing.T) {
	if os.Getenv("MCAGENT_LONG_RL_TRAIN_TEST") == "" {
		t.Skip("set MCAGENT_LONG_RL_TRAIN_TEST=1 to run this multi-minute live training run")
	}

	env := setupStandaloneTestForEntity(t, "rl_train_long_run_mine_lowent", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = env.Agent.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{0, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:stone",
		MineSearchRadius: 4,
		Seeder:           rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	settings := rlTrainSettings(1_000_000)
	settings.RolloutSize = 8
	settings.EntropyCoef = 0
	persistentFactory := func(*rand.Rand) (rl.Environment, error) { return rlEnv, nil }
	trainer, err := reinforce.NewWithPersistentEnv(settings, persistentFactory, nil)
	require.NoError(t, err, "construct trainer")

	// See the RolloutSize=8 version's own doc comment for these two
	// overrides.
	runBudget := 9 * time.Minute
	if raw := os.Getenv("MCAGENT_LONG_RL_TRAIN_RUN_BUDGET"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		require.NoError(t, err, "parse MCAGENT_LONG_RL_TRAIN_RUN_BUDGET %q", raw)
		runBudget = parsed
	}
	runCtx, cancelRun := context.WithTimeout(env.Ctx, runBudget)
	defer cancelRun()

	returns := make([]float64, 0, 128)
	start := time.Now()
	for epoch := 0; epoch < settings.Epochs; epoch++ {
		stats, err := trainer.RunEpoch(runCtx, epoch)
		if err != nil {
			if runCtx.Err() != nil {
				t.Logf("stopping at epoch %d after %s (run budget reached): %v", epoch, time.Since(start).Round(time.Second), err)
				break
			}
			require.NoError(t, err, "RunEpoch %d", epoch)
		}
		returns = append(returns, float64(stats.AverageReturn))
		t.Logf("epoch %d (%s elapsed): average return %.3f (return std %.3f, samples %d)",
			epoch, time.Since(start).Round(time.Second), stats.AverageReturn, stats.ReturnStd, stats.SampleCount)
	}

	require.NotEmpty(t, returns, "no epochs completed within the run budget")
	t.Logf("completed %d epochs (%d episodes) in %s", len(returns), len(returns)*settings.RolloutSize, time.Since(start).Round(time.Second))

	quarter := max(1, len(returns)/4)
	firstMean := meanFloat64(returns[:quarter])
	lastMean := meanFloat64(returns[len(returns)-quarter:])
	t.Logf("first-quarter mean return: %.3f (n=%d epochs) | last-quarter mean return: %.3f (n=%d epochs)", firstMean, quarter, lastMean, quarter)

	const nearCeiling = 18.0 // meaningfully closer to the ~20 ceiling than the ~14.99 equilibrium EntropyCoef=0.01 settled at
	switch {
	case lastMean >= nearCeiling:
		t.Logf("✓ average return reached %.3f (near the ~20 ceiling) — the entropy coefficient was holding back full convergence; removing it let the policy converge much closer to always-mine", lastMean)
	case lastMean > firstMean:
		t.Logf("~ average return improved (%.3f -> %.3f) but did not reach near the ~20 ceiling — entropy may be part of the story but not the whole explanation", firstMean, lastMean)
	default:
		t.Logf("✗ average return did NOT improve (%.3f -> %.3f) — the entropy hypothesis looks wrong, or zero entropy caused a different failure mode (e.g. premature collapse away from Mine); inspect the per-epoch log above", firstMean, lastMean)
	}
}

// mineEquilibriumEpisode records one episode's worth of what
// docs/bugs/mine-task-fifty-percent-equilibrium.md needs to distinguish
// its two live hypotheses: the policy still genuinely mixing ~50/50, vs.
// the policy already converged to "always Mine when legal" while
// something in rlenv's episode-to-episode state (most likely
// DefaultEpisodeSeeder/refreshMineTarget) only makes Mine legal on every
// other Reset, independent of the dispatched action.
type mineEquilibriumEpisode struct {
	mineVisibleAtReset float32
	mineLegalAtReset   bool
	action             rl.Action
	reward             float32
}

// mineEquilibriumRecorder wraps a *rlenv.Environment, forwarding every
// rl.Environment/rl.ActionMasker call to it unchanged, while recording one
// mineEquilibriumEpisode per Reset/Step pair — see
// TestRLTrainingLoop_MineEquilibriumRootCauseLive. This task's episodes
// are always exactly one Step long (TargetOffset:[0,0,0] means arrival's
// distance check is already satisfied at Reset — see that test's Config,
// copied from the earlier long-run tests), so "the most recent episode"
// unambiguously means "the last Reset before whichever Step is being
// recorded," with no risk of a second Step overwriting the wrong entry.
type mineEquilibriumRecorder struct {
	inner *rlenv.Environment

	mu       sync.Mutex
	episodes []mineEquilibriumEpisode
}

func (r *mineEquilibriumRecorder) Reset(ctx context.Context) (rl.Observation, error) {
	obs, err := r.inner.Reset(ctx)
	if err != nil {
		return obs, err
	}
	r.mu.Lock()
	r.episodes = append(r.episodes, mineEquilibriumEpisode{mineVisibleAtReset: obs.Values[12]})
	r.mu.Unlock()
	return obs, nil
}

func (r *mineEquilibriumRecorder) Step(ctx context.Context, action rl.Action) (rl.StepResult, error) {
	result, err := r.inner.Step(ctx, action)
	r.mu.Lock()
	if n := len(r.episodes); n > 0 {
		r.episodes[n-1].action = action
		r.episodes[n-1].reward = result.Reward
	}
	r.mu.Unlock()
	return result, err
}

func (r *mineEquilibriumRecorder) ObservationSize() int { return r.inner.ObservationSize() }
func (r *mineEquilibriumRecorder) ActionSpace() int     { return r.inner.ActionSpace() }

// ActionMask forwards to the real Environment's ActionMask (satisfying
// rl.ActionMasker so pkg/reinforce's rollout loop keeps masking exactly as
// it would against the unwrapped Environment) and records whether Mine
// specifically was legal for the episode ActionMask was just asked about
// — the direct measurement this whole diagnostic exists for.
func (r *mineEquilibriumRecorder) ActionMask() []bool {
	mask := r.inner.ActionMask()
	r.mu.Lock()
	if n := len(r.episodes); n > 0 && len(mask) > int(rlenv.ActionMine) {
		r.episodes[n-1].mineLegalAtReset = mask[rlenv.ActionMine]
	}
	r.mu.Unlock()
	return mask
}

// actionName renders a rlenv action constant for readable log output —
// this file has no existing String() method for rl.Action, and adding one
// on rlenv.Action itself isn't warranted for one diagnostic test's logs.
func actionName(action rl.Action) string {
	switch action {
	case rlenv.ActionWait:
		return "Wait"
	case rlenv.ActionGoToTarget:
		return "GoToTarget"
	case rlenv.ActionMine:
		return "Mine"
	case rlenv.ActionCraft:
		return "Craft"
	default:
		return fmt.Sprintf("action(%d)", action)
	}
}

// TestRLTrainingLoop_MineEquilibriumRootCauseLive investigates
// docs/bugs/mine-task-fifty-percent-equilibrium.md's open question: is the
// exact, zero-scatter 14.990 (4-of-8-every-epoch) plateau a real ~50%
// mixed policy, or is the policy already converged to "always Mine when
// legal" while Mine is only actually *legal* (mineVisible/ActionMask) on
// every other episode for environment reasons unrelated to the dispatched
// action? Same mine-task Config and RolloutSize=8/EntropyCoef=0.01 as
// TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLargerRolloutLive
// (the run that most cleanly characterized the plateau), but the
// persistent environment handed to reinforce.NewWithPersistentEnv is
// wrapped in mineEquilibriumRecorder so every episode's
// (mineVisible-at-Reset, mineLegal-at-Reset, dispatched action, reward)
// can be inspected after the run — cRL-go's own Trainer/EpochStats API
// has no hook for this, hence the wrapper rather than a code change to
// cRL-go or rlenv.
func TestRLTrainingLoop_MineEquilibriumRootCauseLive(t *testing.T) {
	if os.Getenv("MCAGENT_LONG_RL_TRAIN_TEST") == "" {
		t.Skip("set MCAGENT_LONG_RL_TRAIN_TEST=1 to run this multi-minute live training run")
	}

	env := setupStandaloneTestForEntity(t, "rl_train_mine_equilibrium", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = env.Agent.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{0, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:stone",
		MineSearchRadius: 4,
		Seeder:           rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")
	recorder := &mineEquilibriumRecorder{inner: rlEnv}

	settings := rlTrainSettings(1_000_000)
	settings.RolloutSize = 8
	persistentFactory := func(*rand.Rand) (rl.Environment, error) { return recorder, nil }
	trainer, err := reinforce.NewWithPersistentEnv(settings, persistentFactory, nil)
	require.NoError(t, err, "construct trainer")

	// See the RolloutSize=8 test's own doc comment for these two
	// overrides. Default 9 minutes is plenty here — the plateau is
	// reached within roughly a minute in every prior run, leaving ample
	// post-plateau episodes to inspect regardless.
	runBudget := 9 * time.Minute
	if raw := os.Getenv("MCAGENT_LONG_RL_TRAIN_RUN_BUDGET"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		require.NoError(t, err, "parse MCAGENT_LONG_RL_TRAIN_RUN_BUDGET %q", raw)
		runBudget = parsed
	}
	runCtx, cancelRun := context.WithTimeout(env.Ctx, runBudget)
	defer cancelRun()

	start := time.Now()
	epoch := 0
	for ; epoch < settings.Epochs; epoch++ {
		stats, err := trainer.RunEpoch(runCtx, epoch)
		if err != nil {
			if runCtx.Err() != nil {
				t.Logf("stopping at epoch %d after %s (run budget reached): %v", epoch, time.Since(start).Round(time.Second), err)
				break
			}
			require.NoError(t, err, "RunEpoch %d", epoch)
		}
		if epoch%10 == 0 {
			t.Logf("epoch %d (%s elapsed): average return %.3f", epoch, time.Since(start).Round(time.Second), stats.AverageReturn)
		}
	}

	recorder.mu.Lock()
	episodes := append([]mineEquilibriumEpisode(nil), recorder.episodes...)
	recorder.mu.Unlock()
	require.NotEmpty(t, episodes, "no episodes recorded within the run budget")
	t.Logf("recorded %d episodes across %d epochs in %s", len(episodes), epoch, time.Since(start).Round(time.Second))

	// A fixed trailing window, not "back half": this diagnostic's own
	// first live run found the breakout point lands well past the
	// halfway mark of total episodes (episode ~400 of 594, since the
	// pre-breakout "never mines" phase is cheap and racks up many more
	// episodes per minute than the post-breakout phase — the same lopsided
	// cost shape TestRLTrainingLoop_LongRunShowsLearningOnMineTaskWithLowEntropyLive's
	// doc comment already found), so a back-half split still captured a
	// long pre-breakout prefix and produced a misleading blended read.
	// 120 episodes (~15 epochs at RolloutSize=8) comfortably fits inside
	// the settled tail confirmed by that run's own epoch-level log
	// (epoch 60 and 70 both showed exactly 14.990).
	tailSize := 120
	if tailSize > len(episodes) {
		tailSize = len(episodes)
	}
	steadyState := episodes[len(episodes)-tailSize:]

	var visibleAndChoseMine, visibleAndChoseOther, notVisibleAndChoseMine, notVisibleAndChoseOther int
	var visibleCount int
	togglePairs, toggleAlternates := 0, 0
	for i, ep := range steadyState {
		switch {
		case ep.mineLegalAtReset && ep.action == rlenv.ActionMine:
			visibleAndChoseMine++
		case ep.mineLegalAtReset:
			visibleAndChoseOther++
		case ep.action == rlenv.ActionMine:
			notVisibleAndChoseMine++
		default:
			notVisibleAndChoseOther++
		}
		if ep.mineLegalAtReset {
			visibleCount++
		}
		if i > 0 {
			togglePairs++
			if steadyState[i-1].mineLegalAtReset != ep.mineLegalAtReset {
				toggleAlternates++
			}
		}
	}
	total := len(steadyState)

	t.Logf("steady-state sample: last %d episodes (of %d total)", total, len(episodes))
	t.Logf("mineVisible/legal at Reset: %d/%d (%.1f%%)", visibleCount, total, 100*float64(visibleCount)/float64(total))
	t.Logf("breakdown: visible+chose Mine=%d, visible+chose other=%d, not-visible+chose Mine=%d (should be 0, masked illegal), not-visible+chose other=%d",
		visibleAndChoseMine, visibleAndChoseOther, notVisibleAndChoseMine, notVisibleAndChoseOther)
	t.Logf("mineVisible alternation rate: %d/%d consecutive pairs flipped (%.1f%%) — 100%% would mean strict alternation every episode",
		toggleAlternates, togglePairs, 100*float64(toggleAlternates)/float64(togglePairs))

	// Log every steady-state episode verbatim (not capped, unlike an
	// earlier version of this test) — the raw per-episode sequence is
	// exactly what distinguishes this investigation's hypotheses, and a
	// truncated sample already once produced a misleading read (see
	// tailSize's own doc comment above) by cutting off before the real
	// pattern was visible.
	for i, ep := range steadyState {
		t.Logf("steady-state episode %d: mineLegalAtReset=%v action=%s reward=%.3f", i, ep.mineLegalAtReset, actionName(ep.action), ep.reward)
	}

	require.Zero(t, notVisibleAndChoseMine, "ActionMine was recorded as dispatched while masked illegal — should never happen (ActionMask/resolveDispatch bug, not this investigation's question)")

	switch {
	case visibleAndChoseOther > 0:
		t.Logf("✗ HYPOTHESIS A (environment-only toggle) REJECTED: the policy chose a non-Mine action %d times even when Mine was legal — it has not converged to \"always Mine when legal,\" so at least part of the ~50%% rate is genuinely the policy's own choice", visibleAndChoseOther)
	case visibleCount < total:
		t.Logf("✓ HYPOTHESIS A (environment-only toggle) SUPPORTED: whenever Mine was legal the policy always chose it (visible+chose other=0), and Mine was legal in only %d/%d episodes — the ~50%% rate looks like an environment/seeding artifact (mineVisible not toggling with the dispatched action), not an unconverged policy", visibleCount, total)
	default:
		t.Logf("? inconclusive: Mine was legal in all %d steady-state episodes and always chosen — no 50%% pattern observed in this sample; rerun or investigate the recorded episodes above", total)
	}
}

// TestRLTrainingLoop_MineSeedingAlternationRootCauseLive is the follow-up
// TestRLTrainingLoop_MineEquilibriumRootCauseLive's result demands: that
// test found a *perfect, unbroken period-2 alternation* in reward
// (19.990, 9.990, 19.990, 9.990, ...) across 100+ consecutive episodes,
// with the dispatched action constantly ActionMine — i.e. the policy had
// already converged to "always Mine," and something in the environment
// itself only delivers the mine bonus every other attempt. This test
// removes the policy/training machinery entirely (no reinforce.Trainer, no
// Actor, no gradient updates) and just repeats "Reset (which seeds a fresh
// target via DefaultEpisodeSeeder), then dispatch ActionMine once" many
// times in a tight loop with no artificial delay — the same environment
// mechanics, isolated from RL, to see whether the alternation is
// reproducible from the environment/seeding path alone.
//
// This looks, at first glance, like a rerun of
// TestRLTrainingLoop_MineActionRegistersImmediatelyLive (30/30 first-
// attempt successes, no alternation seen) — the difference is trial count
// (200 here vs. 30 there) and no per-trial retry allowance (single-shot
// only, matching a training Step's own single-shot nature exactly): if
// the alternation only emerges after some churn/warm-up, or is a
// close-to-50%-but-not-exactly-alternating stochastic effect that a
// 30-trial sample was too small to reveal clearly, a longer single-shot
// run should show it; if it doesn't, that would point back toward
// something specific to how reinforce's rollout loop drives Reset/Step
// (e.g. batching/pacing within RunEpoch) rather than the seeding/mine
// mechanics being inherently alternating.
func TestRLTrainingLoop_MineSeedingAlternationRootCauseLive(t *testing.T) {
	if os.Getenv("MCAGENT_LONG_RL_TRAIN_TEST") == "" {
		t.Skip("set MCAGENT_LONG_RL_TRAIN_TEST=1 to run this multi-minute live diagnostic")
	}

	env := setupStandaloneTestForEntity(t, "rl_train_mine_alternation", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = env.Agent.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{0, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:stone",
		MineSearchRadius: 4,
		Seeder:           rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	const trials = 200
	type trialResult struct {
		mineVisibleAtReset float32
		reward             float32
	}
	results := make([]trialResult, 0, trials)
	for i := 0; i < trials; i++ {
		obs, err := rlEnv.Reset(env.Ctx)
		require.NoError(t, err, "trial %d: Reset", i)
		result, err := rlEnv.Step(env.Ctx, rlenv.ActionMine)
		require.NoError(t, err, "trial %d: Step", i)
		results = append(results, trialResult{mineVisibleAtReset: obs.Values[12], reward: result.Reward})
	}

	succeeded, alternations := 0, 0
	for i, r := range results {
		if r.reward > 15 {
			succeeded++
		}
		t.Logf("trial %d: mineVisibleAtReset=%.0f reward=%.3f", i, r.mineVisibleAtReset, r.reward)
		if i > 0 {
			prevSucceeded := results[i-1].reward > 15
			curSucceeded := r.reward > 15
			if prevSucceeded != curSucceeded {
				alternations++
			}
		}
	}
	t.Logf("summary: %d/%d trials (%.1f%%) succeeded (reward > 15)", succeeded, trials, 100*float64(succeeded)/float64(trials))
	t.Logf("summary: %d/%d consecutive pairs alternated (%.1f%%) — 100%% would mean perfect period-2 alternation like the training-loop finding", alternations, trials-1, 100*float64(alternations)/float64(trials-1))

	if alternations > (trials-1)*8/10 {
		t.Logf("✓ Reproduced outside training: the environment/seeding path alone produces near-perfect alternation with no policy or gradient machinery involved — this is an rlenv/agent seeding bug, not an RL-algorithm question")
	} else {
		t.Logf("✗ Did NOT reproduce in isolation (%.1f%% alternation, vs. ~100%% seen inside actual training) — something about how reinforce's rollout loop drives Reset/Step differs from this tight standalone loop; investigate that difference next, not the seeding code in isolation",
			100*float64(alternations)/float64(trials-1))
	}
}

// TestRLTrainingLoop_MineActionRegistersImmediatelyLive measures how often
// a single ActionMine dispatch against a freshly seeded, currently-visible
// target registers mined=true on that same Step call, versus needing a
// retry — checking one of the two unconfirmed explanations
// TestRLTrainingLoop_LongRunShowsLearningOnMineTaskLive's own doc comment
// raised for that run's weak learning signal: a live MineBlockAt-
// completion-timing race (already documented, and worked around with a
// retry loop, by TestRLTrainingLoop_MineTaskSeedingEarnsRewardAndEndsEpisode's
// stepUntilDone). If a *training* loop's single-shot Step often dispatches
// a correct "mine" choice that doesn't register as mined until a later
// Step, that action's reward signal is noisier than GoToTarget's simpler
// success condition, which could slow or prevent REINFORCE from ever
// distinguishing it from noise — independent of whether the policy learns
// to *choose* Mine at all (a separate, already-answered question this
// test doesn't touch: dispatch here is always ActionMine, never sampled
// from a policy).
//
// **First live run (2026-09-10): timing-race hypothesis RULED OUT.** 30/30
// trials (100%) registered mined=true on the very first ActionMine
// dispatch — no retries needed at all, contrary to what the mine-seeding
// test's own documented race would predict if it applied here. This
// doesn't mean that race is fictional (that test's own doc comment
// describes it as found live, not assumed), but it does mean it's not a
// significant contributor to TestRLTrainingLoop_LongRunShowsLearningOnMineTaskLive's
// weak result — the mine reward signal is clean and reliable when Mine is
// actually chosen. That leaves the other candidate explanation (REINFORCE's
// single-rollout variance needing far more than ~500 episodes to separate
// a 2x reward gap from noise) as the more likely bottleneck, by
// elimination rather than direct confirmation.
func TestRLTrainingLoop_MineActionRegistersImmediatelyLive(t *testing.T) {
	if os.Getenv("MCAGENT_LONG_RL_TRAIN_TEST") == "" {
		t.Skip("set MCAGENT_LONG_RL_TRAIN_TEST=1 to run this multi-minute live diagnostic")
	}

	env := setupStandaloneTestForEntity(t, "rl_train_mine_timing", rlTrainTestVersion)
	defer env.Cancel()

	liveAgent, ok := env.Agent.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = env.Agent.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		TargetOffset:     [3]float64{0, 0, 0},
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:stone",
		MineSearchRadius: 4,
		Seeder:           rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	const trials = 30
	const maxAttempts = 5
	// attemptsToSucceed[i] is the 1-based attempt number trial i's mined
	// signal registered on, or 0 if it never did within maxAttempts.
	attemptsToSucceed := make([]int, 0, trials)

	for trial := 0; trial < trials; trial++ {
		obs, err := rlEnv.Reset(env.Ctx)
		require.NoError(t, err, "trial %d: Reset", trial)
		require.Equal(t, float32(1), obs.Values[12], "trial %d: mineVisible should be 1 right after seeding", trial)

		succeededAt := 0
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			result, err := rlEnv.Step(env.Ctx, rlenv.ActionMine)
			require.NoError(t, err, "trial %d attempt %d: Step", trial, attempt)
			if result.Done {
				succeededAt = attempt
				break
			}
		}
		attemptsToSucceed = append(attemptsToSucceed, succeededAt)
		t.Logf("trial %d: mined registered on attempt %d (0 = never within %d attempts)", trial, succeededAt, maxAttempts)
	}

	firstAttemptSuccesses := 0
	neverSucceeded := 0
	for _, a := range attemptsToSucceed {
		if a == 1 {
			firstAttemptSuccesses++
		}
		if a == 0 {
			neverSucceeded++
		}
	}

	t.Logf("summary: %d/%d trials (%.1f%%) registered mined=true on the FIRST ActionMine dispatch",
		firstAttemptSuccesses, trials, 100*float64(firstAttemptSuccesses)/float64(trials))
	t.Logf("summary: %d/%d trials never registered within %d attempts", neverSucceeded, trials, maxAttempts)

	require.Zero(t, neverSucceeded, "at least one trial never registered mined=true within %d attempts — something beyond timing may be wrong", maxAttempts)
}

// meanFloat64 returns the arithmetic mean of values, or 0 for an empty
// slice (callers here always pass a non-empty slice — see max(1, ...)
// above — 0 is just a safe default, not a case expected to matter).
func meanFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
