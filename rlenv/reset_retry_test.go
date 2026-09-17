package rlenv_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// TestResetOuterRetryRecoversFromATransientTeleportFailure is a
// regression test for Reset's own outer retry loop (see Reset's doc
// comment in environment.go): a single flaky TeleportTo call - the kind
// of one-off transient failure that would otherwise crash potentially
// hours of otherwise-healthy training over one unlucky episode boundary
// - must not fail the whole Reset call as long as a later attempt
// succeeds within resetOuterRetryAttempts.
func TestResetOuterRetryRecoversFromATransientTeleportFailure(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.teleportFailFirstN = 1 // first attempt's TeleportTo fails, second succeeds

	origin := [3]float64{10, 0, 10}
	cfg := testConfig()
	cfg.ResetOrigin = &origin
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: want the outer retry to recover from one transient TeleportTo failure, got error: %v", err)
	}
	if len(agent.teleportCalls) != 2 {
		t.Fatalf("TeleportTo was called %d times, want exactly 2 (the failed first attempt, then the successful retry)", len(agent.teleportCalls))
	}
}

// TestResetFailsAfterExhaustingOuterRetryAttempts is the companion test:
// a TeleportTo failure that never clears must still fail loudly once
// Reset's own outer retry budget is exhausted, not retry forever.
func TestResetFailsAfterExhaustingOuterRetryAttempts(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.teleportErr = errors.New("permanent teleport failure")

	origin := [3]float64{10, 0, 10}
	cfg := testConfig()
	cfg.ResetOrigin = &origin
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err == nil {
		t.Fatal("Reset with a permanently-failing TeleportTo: want error, got nil")
	}
	// Every attempt fails identically here, so the number of TeleportTo
	// calls directly reveals how many attempts Reset actually made -
	// this is the bound that keeps a genuinely broken config from
	// retrying indefinitely.
	if len(agent.teleportCalls) == 0 {
		t.Fatal("expected at least one TeleportTo call")
	}
}

// TestResetOuterRetryDoesNotSkipEpisodeIndicesSeenByTaskSelector is a
// regression test for the episode-index bug an earlier version of this
// fix introduced: Config.TaskSelector must see a strictly monotonic,
// gap-free episode index (0, 1, 2, ...) across successive *external*
// Reset calls, even when one of those calls internally retries several
// times before succeeding - a failed-then-retried attempt must not
// silently consume an index TaskSelector never gets to see.
//
// The failure is injected via Config.Seeder rather than TeleportTo:
// TeleportTo runs *before* TaskSelector within one attempt, so a failed
// TeleportTo never reaches TaskSelector at all and wouldn't exercise
// this risk. Seeder runs *after* both TaskSelector and the episode
// index is read, so a Seeder failure forces a real internal retry of
// the whole attempt - including a second TaskSelector call for the same
// external episode - which is exactly the scenario that could skip an
// index if e.episode were still incremented per internal attempt
// instead of once per external Reset call.
func TestResetOuterRetryDoesNotSkipEpisodeIndicesSeenByTaskSelector(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)

	var seenEpisodes []int
	cfg := testConfig()
	cfg.TaskSelector = func(episode int, _ *rand.Rand) rlenv.TaskOverride {
		seenEpisodes = append(seenEpisodes, episode)
		return rlenv.TaskOverride{TargetOffset: cfg.TargetOffset}
	}
	seederCalls := 0
	cfg.Seeder = func(context.Context, rlenv.SeedAgent, rlenv.Config) error {
		seederCalls++
		if seederCalls == 1 {
			return errors.New("forced transient seeder failure")
		}
		return nil
	}
	env := newTestEnvironment(t, agent, cfg)

	for i := 0; i < 3; i++ {
		if _, err := env.Reset(context.Background()); err != nil {
			t.Fatalf("Reset %d: %v", i, err)
		}
	}

	// Episode 0's Reset call made two internal attempts (the failed one,
	// then the successful retry), so TaskSelector was actually invoked 4
	// times total - but the episode *index* it saw must still be exactly
	// [0, 0, 1, 2]: repeated for episode 0's own retry, never skipping
	// straight to 1.
	want := []int{0, 0, 1, 2}
	if len(seenEpisodes) != len(want) {
		t.Fatalf("seenEpisodes = %v, want %v", seenEpisodes, want)
	}
	for i, got := range seenEpisodes {
		if got != want[i] {
			t.Fatalf("seenEpisodes = %v, want %v", seenEpisodes, want)
		}
	}
}
