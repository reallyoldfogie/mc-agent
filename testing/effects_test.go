package testing

import (
	"context"
	"fmt"
	"math"
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

// TestJumpBoostRaisesJumpHeight verifies that applying the Jump Boost status
// effect (§4.7) causes the agent's own predicted physics position to jump
// higher than an unboosted baseline jump — end-to-end confirmation that
// physics.JumpBoostVelocityBonus is actually wired into state.go's jump
// velocity assignment against a real server, not just unit-tested in
// isolation. See physics/effects_test.go's TestJumpBoostVelocityBonus and
// physics/state_active_effects_test.go's TestState_JumpBoostRaisesJumpVelocity
// for the formula-level coverage this builds on.
func TestJumpBoostRaisesJumpHeight(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "jump_boost_height", "survival", false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()

			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()

			// jumpAndMeasurePeak triggers one jump via manual input and
			// tracks the highest Y reached over the following window, then
			// waits for the agent to land and settle before returning, so
			// back-to-back calls on the same agent don't interfere.
			jumpAndMeasurePeak := func() float64 {
				startPos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok, "agent position should be initialized")
				startY := startPos.Y

				require.NoError(t, env.Agent.Agent.SetManualJump(true))
				time.Sleep(100 * time.Millisecond)
				require.NoError(t, env.Agent.Agent.SetManualJump(false))

				peakY := startY
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					pos, _ := env.Agent.Agent.GetPositionSimple()
					if pos.Y > peakY {
						peakY = pos.Y
					}
					time.Sleep(50 * time.Millisecond)
				}

				// Let the agent finish landing/settling before the next jump.
				time.Sleep(1 * time.Second)
				return peakY - startY
			}

			baselineRise := jumpAndMeasurePeak()
			t.Logf("baseline jump rise=%.4f", baselineRise)
			assert.Greater(t, baselineRise, 0.0, "an ordinary jump should rise above the starting position")

			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:jump_boost 30 0", env.BotName))
			require.NoError(t, err, "apply jump boost effect")
			time.Sleep(300 * time.Millisecond)

			boostedRise := jumpAndMeasurePeak()
			t.Logf("jump-boosted rise=%.4f", boostedRise)

			assert.Greater(t, boostedRise, baselineRise, "jump boost should raise the agent's jump height above the unboosted baseline")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:jump_boost", env.BotName))
		})
	}
}

// TestSpeedAndSlownessScaleGroundDistance verifies that applying Speed/
// Slowness (§4.5/§4.6, PHASE_4_PLAN.md §2.1's prerequisite) changes how far
// the agent's own predicted physics position moves for the same manual
// throttle input over the same time window — end-to-end confirmation that
// physics.EffectSpeedMultiplier is actually wired into state.go's ground
// acceleration against a real server. See physics/effects_test.go's
// TestEffectSpeedMultiplier and
// physics/state_active_effects_test.go's
// TestState_SpeedAndSlownessScaleGroundAcceleration for the formula-level
// coverage this builds on.
func TestSpeedAndSlownessScaleGroundDistance(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "speed_slowness_distance", "survival", false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()

			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()

			// moveForwardAndMeasureDistance holds forward throttle for a
			// fixed window and returns the horizontal distance covered,
			// then lets the agent coast to a stop before returning so
			// back-to-back calls don't carry over momentum.
			moveForwardAndMeasureDistance := func() float64 {
				startPos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok, "agent position should be initialized")

				require.NoError(t, env.Agent.Agent.SetManualThrottle(0, 1))
				time.Sleep(1 * time.Second)
				require.NoError(t, env.Agent.Agent.SetManualThrottle(0, 0))
				time.Sleep(1 * time.Second)

				endPos, _ := env.Agent.Agent.GetPositionSimple()
				dx := endPos.X - startPos.X
				dz := endPos.Z - startPos.Z
				return math.Sqrt(dx*dx + dz*dz)
			}

			baselineDist := moveForwardAndMeasureDistance()
			t.Logf("baseline distance=%.4f", baselineDist)
			assert.Greater(t, baselineDist, 0.0, "ordinary forward movement should cover some distance")

			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:speed 30 1", env.BotName))
			require.NoError(t, err, "apply speed II")
			time.Sleep(300 * time.Millisecond)

			speedDist := moveForwardAndMeasureDistance()
			t.Logf("speed II distance=%.4f", speedDist)
			assert.Greater(t, speedDist, baselineDist, "speed II should cover more distance than baseline in the same window")

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:speed", env.BotName))
			require.NoError(t, err, "clear speed")
			time.Sleep(300 * time.Millisecond)

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:slowness 30 1", env.BotName))
			require.NoError(t, err, "apply slowness II")
			time.Sleep(300 * time.Millisecond)

			slownessDist := moveForwardAndMeasureDistance()
			t.Logf("slowness II distance=%.4f", slownessDist)
			assert.Less(t, slownessDist, baselineDist, "slowness II should cover less distance than baseline in the same window")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:slowness", env.BotName))
		})
	}
}

// TestBlindnessPreventsSprinting verifies that applying the Blindness status
// effect (§4.9) both prevents starting a new sprint and cancels one already
// in progress — end-to-end confirmation that physics.CanSprint is actually
// wired into movement/physics_executor_helpers.go's applyMovementState
// against a real server. Mirrors Java ClientPlayerEntity's
// canStartSprinting()/shouldStopSprinting(), both of which key off the same
// canSprint() check, so an in-progress sprint is force-stopped the instant
// Blindness lands, not just blocked from (re)starting.
func TestBlindnessPreventsSprinting(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "blindness_sprint_gate", "survival", false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()

			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()

			require.NoError(t, env.Agent.Agent.SetManualSprint(true))
			assert.Eventually(t, env.Agent.Agent.IsSprinting, 3*time.Second, 100*time.Millisecond,
				"should be able to start sprinting with no perception-restricting effect active")

			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect give %s minecraft:blindness 30 0", env.BotName))
			require.NoError(t, err, "apply blindness effect")

			assert.Eventually(t, func() bool { return !env.Agent.Agent.IsSprinting() }, 5*time.Second, 100*time.Millisecond,
				"blindness should force-stop a sprint already in progress, not just block new ones")

			// Sprint input is still held; blindness alone should keep
			// rejecting it rather than only stopping the sprint once.
			time.Sleep(500 * time.Millisecond)
			assert.False(t, env.Agent.Agent.IsSprinting(), "should not be able to (re)start sprinting while blind")

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("effect clear %s minecraft:blindness", env.BotName))
			require.NoError(t, err, "clear blindness effect")

			assert.Eventually(t, env.Agent.Agent.IsSprinting, 3*time.Second, 100*time.Millisecond,
				"should regain the ability to sprint once blindness clears, with sprint input still held")

			require.NoError(t, env.Agent.Agent.SetManualSprint(false))
		})
	}
}
