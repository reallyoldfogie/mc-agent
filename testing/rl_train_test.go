package testing

import (
	"context"
	"fmt"
	"math/rand/v2"
	"path/filepath"
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
