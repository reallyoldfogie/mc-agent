package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLevitationLiftsAgent verifies that applying the Levitation status
// effect (Phase 4a) causes the agent's own predicted physics position to
// rise, not fall — end-to-end confirmation that ClientboundEntityEffect is
// parsed, tracked, and actually wired into physics.State.Tick() against a
// real server, not just unit-tested in isolation. See
// physics/effects_test.go and physics/state_active_effects_test.go for the
// formula-level coverage this builds on.
func TestLevitationLiftsAgent(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "levitation_lift", "survival", false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()

			// Teleport well clear of the ground so a real fall (rather than
			// landing on something) would be the alternative outcome if
			// levitation had no effect on the agent's own physics. Apply the
			// effect immediately (not after settling first): SetPosition
			// never touches velocity, so any delay here would let ordinary
			// gravity build up real fall momentum before the effect ever
			// gets a chance to act on it, contaminating the measurement.
			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("teleport %s 100 80 100", env.BotName))
			require.NoError(t, err, "teleport agent into open air")
			time.Sleep(300 * time.Millisecond)

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:levitation 10 0", env.BotName))
			require.NoError(t, err, "apply levitation effect")
			time.Sleep(300 * time.Millisecond)

			startPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")

			// Give physics time to respond — several ticks of accumulated
			// upward velocity, not just the first tick.
			time.Sleep(3 * time.Second)

			endPos, _ := env.Agent.Agent.GetPositionSimple()
			t.Logf("start Y=%.3f, end Y=%.3f", startPos.Y, endPos.Y)

			assert.Greater(t, endPos.Y, startPos.Y,
				"agent's own predicted position should rise under levitation, not fall")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:levitation", env.BotName))
		})
	}
}

// TestSlowFallingSlowsAgentDescent verifies that applying the Slow Falling
// status effect causes the agent's own predicted physics position to
// descend far slower than ordinary gravity would, end-to-end against a real
// server. This deliberately checks the agent's own tracked position rather
// than server-reported fall damage: vanilla's server already negates fall
// damage for Slow Falling on its own regardless of what our client does, so
// a damage-based assertion couldn't distinguish "our implementation works"
// from "our implementation does nothing and the server saved us anyway".
//
// The bound below is not an arbitrary "should be small" guess: Slow Falling
// caps the per-tick gravity *increment* at physics.SlowFallingMaxGravity
// (0.01), not the eventual terminal velocity — physics.Drag (0.98) still
// compounds that small increment into a real steady-state descent of
// SlowFallingMaxGravity*Drag/(1-Drag) ≈ 0.49 blocks/tick (~9.8 blocks/sec).
// An earlier version of this test asserted a much tighter bound (<5 blocks
// over the test's window) from an unfounded assumption that the gravity cap
// alone would keep descent near-zero, and failed against a correct
// implementation as a result — confirmed empirically to produce ~28 blocks
// over 3 seconds, matching the steady-state math almost exactly.
func TestSlowFallingSlowsAgentDescent(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "slow_falling_descent", "survival", false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()

			// As in TestLevitationLiftsAgent: apply the effect immediately
			// after teleporting, before any measurement, so gravity never
			// gets a window to build real fall momentum first. Slow
			// Falling in particular only caps the per-tick gravity
			// *increment* (see physics/effects.go's EffectiveGravity) — it
			// does not undo existing velocity, so a fall that was already
			// underway before the effect landed would keep most of its
			// momentum and defeat this test even with a correct
			// implementation.
			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("teleport %s 100 80 100", env.BotName))
			require.NoError(t, err, "teleport agent into open air")
			time.Sleep(300 * time.Millisecond)

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:slow_falling 30 0", env.BotName))
			require.NoError(t, err, "apply slow falling effect")
			time.Sleep(300 * time.Millisecond)

			startPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")

			time.Sleep(3 * time.Second)

			endPos, _ := env.Agent.Agent.GetPositionSimple()
			drop := startPos.Y - endPos.Y
			t.Logf("start Y=%.3f, end Y=%.3f, drop=%.3f", startPos.Y, endPos.Y, drop)

			// Steady-state descent (see doc comment above); the actual drop
			// can only be less than this over a window that starts from
			// rest, since velocity approaches the steady state from below.
			// A generous multiplier absorbs test-timing jitter (wall-clock
			// sleep vs. exact tick count) without hiding a regression back
			// toward normal gravity's much larger steady state
			// (Gravity*Drag/(1-Drag) ≈ 3.92 blocks/tick, ~8x larger).
			slowFallingSteadyStatePerTick := physics.SlowFallingMaxGravity * physics.Drag / (1 - physics.Drag)
			maxExpectedDrop := slowFallingSteadyStatePerTick * float64(physics.TicksPerSecond) * 3.0 * 1.5
			assert.Less(t, drop, maxExpectedDrop,
				"agent's own predicted descent should stay near the slow-falling steady state, not accelerate toward normal gravity's much larger one")
			assert.Greater(t, drop, 1.0, "agent should still be descending, just slowly, not frozen in place")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:slow_falling", env.BotName))
		})
	}
}
