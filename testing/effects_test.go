package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// EffectsFlatSuite is a
// version-parameterized suite for status-effect tests: one server per
// version, shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern (each via
// setupStandaloneTestWithModeAndBlockPlacement). WorldGen = WorldGenFlat and
// Difficulty = DifficultyEasy, matching every pre-conversion TestXxx
// function's own choice - preserved even though none of these tests
// actually depend on Easy specifically (over Peaceful, VersionWorldSuite's
// own default), to avoid silently diverging from a previously-passing
// test's own conditions.
type EffectsFlatSuite struct {
	VersionWorldSuite
}

func TestEffectsFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &EffectsFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// openAirY is added to a working area's Origin.Y (which, for a flat-world
// suite, is the agent's own unmodified flat-world spawn height - see
// SpawnWorkingAreaAgent's own doc comment) to reach a point well clear of
// the ground, matching the pre-conversion originals' hardcoded "teleport
// ... 100 80 100" - not itself a meaningful absolute Y, just "high enough
// above a flat world's thin ground layer that a real fall would be the
// alternative outcome if an effect had no influence."
const openAirY = 76.0

// TestLevitation verifies that applying the Levitation status effect
// (Phase 4a) causes the agent's own predicted physics position to rise, not
// fall - end-to-end confirmation that ClientboundEntityEffect is parsed,
// tracked, and actually wired into physics.State.Tick() against a real
// server, not just unit-tested in isolation. Equivalent to the original
// TestLevitationLiftsAgent. See physics/effects_test.go and
// physics/state_active_effects_test.go for the formula-level coverage this
// builds on.
func (s *EffectsFlatSuite) TestLevitation() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("LevitationBot", "levitation_lift")
	require.NoError(t, err, "spawn agent")

	// Teleport well clear of the ground so a real fall (rather than landing
	// on something) would be the alternative outcome if levitation had no
	// effect on the agent's own physics. Apply the effect immediately (not
	// after settling first): SetPosition never touches velocity, so any
	// delay here would let ordinary gravity build up real fall momentum
	// before the effect ever gets a chance to act on it, contaminating the
	// measurement.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("teleport %s %.2f %.2f %.2f", leader.Name, leader.Origin.X, leader.Origin.Y+openAirY, leader.Origin.Z))
	require.NoError(t, err, "teleport agent into open air")
	time.Sleep(300 * time.Millisecond)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect give %s minecraft:levitation 10 0", leader.Name))
	require.NoError(t, err, "apply levitation effect")
	time.Sleep(300 * time.Millisecond)

	startPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")

	// Give physics time to respond - several ticks of accumulated upward
	// velocity, not just the first tick.
	time.Sleep(3 * time.Second)

	endPos, _ := leader.Agent.GetPositionSimple()
	t.Logf("start Y=%.3f, end Y=%.3f", startPos.Y, endPos.Y)

	assert.Greater(t, endPos.Y, startPos.Y,
		"agent's own predicted position should rise under levitation, not fall")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect clear %s minecraft:levitation", leader.Name))
}

// TestSlowFalling verifies that applying the Slow Falling status effect
// causes the agent's own predicted physics position to descend far slower
// than ordinary gravity would, end-to-end against a real server. Equivalent
// to the original TestSlowFallingSlowsAgentDescent - see that function's
// own doc comment (git history) for the full steady-state-math rationale
// behind the bound below, unchanged here.
func (s *EffectsFlatSuite) TestSlowFalling() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("SlowFallBot", "slow_falling_descent")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("teleport %s %.2f %.2f %.2f", leader.Name, leader.Origin.X, leader.Origin.Y+openAirY, leader.Origin.Z))
	require.NoError(t, err, "teleport agent into open air")
	time.Sleep(300 * time.Millisecond)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect give %s minecraft:slow_falling 30 0", leader.Name))
	require.NoError(t, err, "apply slow falling effect")
	time.Sleep(300 * time.Millisecond)

	startPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")

	time.Sleep(3 * time.Second)

	endPos, _ := leader.Agent.GetPositionSimple()
	drop := startPos.Y - endPos.Y
	t.Logf("start Y=%.3f, end Y=%.3f, drop=%.3f", startPos.Y, endPos.Y, drop)

	slowFallingSteadyStatePerTick := physics.SlowFallingMaxGravity * physics.Drag / (1 - physics.Drag)
	maxExpectedDrop := slowFallingSteadyStatePerTick * float64(physics.TicksPerSecond) * 3.0 * 1.5
	assert.Less(t, drop, maxExpectedDrop,
		"agent's own predicted descent should stay near the slow-falling steady state, not accelerate toward normal gravity's much larger one")
	assert.Greater(t, drop, 1.0, "agent should still be descending, just slowly, not frozen in place")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect clear %s minecraft:slow_falling", leader.Name))
}

// TestJumpBoost verifies that applying the Jump Boost status effect (§4.7)
// causes the agent's own predicted physics position to jump higher than an
// unboosted baseline jump. Equivalent to the original
// TestJumpBoostRaisesJumpHeight.
func (s *EffectsFlatSuite) TestJumpBoost() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("JumpBoostBot", "jump_boost_height")
	require.NoError(t, err, "spawn agent")

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	jumpAndMeasurePeak := func() float64 {
		startPos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok, "agent position should be initialized")
		startY := startPos.Y

		require.NoError(t, leader.Agent.SetManualJump(true))
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, leader.Agent.SetManualJump(false))

		peakY := startY
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			pos, _ := leader.Agent.GetPositionSimple()
			if pos.Y > peakY {
				peakY = pos.Y
			}
			time.Sleep(50 * time.Millisecond)
		}

		time.Sleep(1 * time.Second)
		return peakY - startY
	}

	baselineRise := jumpAndMeasurePeak()
	t.Logf("baseline jump rise=%.4f", baselineRise)
	assert.Greater(t, baselineRise, 0.0, "an ordinary jump should rise above the starting position")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect give %s minecraft:jump_boost 30 0", leader.Name))
	require.NoError(t, err, "apply jump boost effect")
	time.Sleep(300 * time.Millisecond)

	boostedRise := jumpAndMeasurePeak()
	t.Logf("jump-boosted rise=%.4f", boostedRise)

	assert.Greater(t, boostedRise, baselineRise, "jump boost should raise the agent's jump height above the unboosted baseline")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect clear %s minecraft:jump_boost", leader.Name))
}

// TestSpeedAndSlowness verifies that applying Speed/Slowness (§4.5/§4.6)
// changes how far the agent's own predicted physics position moves for the
// same manual throttle input over the same time window. Equivalent to the
// original TestSpeedAndSlownessScaleGroundDistance.
func (s *EffectsFlatSuite) TestSpeedAndSlowness() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("SpeedSlowBot", "speed_slowness_distance")
	require.NoError(t, err, "spawn agent")

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	moveForwardAndMeasureDistance := func() float64 {
		startPos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok, "agent position should be initialized")

		require.NoError(t, leader.Agent.SetManualThrottle(0, 1))
		time.Sleep(1 * time.Second)
		require.NoError(t, leader.Agent.SetManualThrottle(0, 0))
		time.Sleep(1 * time.Second)

		endPos, _ := leader.Agent.GetPositionSimple()
		dx := endPos.X - startPos.X
		dz := endPos.Z - startPos.Z
		return math.Sqrt(dx*dx + dz*dz)
	}

	baselineDist := moveForwardAndMeasureDistance()
	t.Logf("baseline distance=%.4f", baselineDist)
	assert.Greater(t, baselineDist, 0.0, "ordinary forward movement should cover some distance")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect give %s minecraft:speed 30 1", leader.Name))
	require.NoError(t, err, "apply speed II")
	time.Sleep(300 * time.Millisecond)

	speedDist := moveForwardAndMeasureDistance()
	t.Logf("speed II distance=%.4f", speedDist)
	assert.Greater(t, speedDist, baselineDist, "speed II should cover more distance than baseline in the same window")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect clear %s minecraft:speed", leader.Name))
	require.NoError(t, err, "clear speed")
	time.Sleep(300 * time.Millisecond)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect give %s minecraft:slowness 30 1", leader.Name))
	require.NoError(t, err, "apply slowness II")
	time.Sleep(300 * time.Millisecond)

	slownessDist := moveForwardAndMeasureDistance()
	t.Logf("slowness II distance=%.4f", slownessDist)
	assert.Less(t, slownessDist, baselineDist, "slowness II should cover less distance than baseline in the same window")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect clear %s minecraft:slowness", leader.Name))
}

// TestBlindnessPreventsSprinting verifies that applying the Blindness status
// effect (§4.9) both prevents starting a new sprint and cancels one already
// in progress. Equivalent to the original function of the same name.
func (s *EffectsFlatSuite) TestBlindnessPreventsSprinting() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("BlindnessBot", "blindness_sprint_gate")
	require.NoError(t, err, "spawn agent")

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	require.NoError(t, leader.Agent.SetManualSprint(true))
	assert.Eventually(t, leader.Agent.IsSprinting, 3*time.Second, 100*time.Millisecond,
		"should be able to start sprinting with no perception-restricting effect active")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect give %s minecraft:blindness 30 0", leader.Name))
	require.NoError(t, err, "apply blindness effect")

	assert.Eventually(t, func() bool { return !leader.Agent.IsSprinting() }, 5*time.Second, 100*time.Millisecond,
		"blindness should force-stop a sprint already in progress, not just block new ones")

	time.Sleep(500 * time.Millisecond)
	assert.False(t, leader.Agent.IsSprinting(), "should not be able to (re)start sprinting while blind")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect clear %s minecraft:blindness", leader.Name))
	require.NoError(t, err, "clear blindness effect")

	assert.Eventually(t, leader.Agent.IsSprinting, 3*time.Second, 100*time.Millisecond,
		"should regain the ability to sprint once blindness clears, with sprint input still held")

	require.NoError(t, leader.Agent.SetManualSprint(false))
}

// TestDolphinsGrace verifies that applying the Dolphin's Grace status effect
// increases how far the agent's own predicted physics position moves for
// the same manual throttle input while submerged in water. Equivalent to
// the original TestDolphinsGraceIncreasesSwimSpeed.
func (s *EffectsFlatSuite) TestDolphinsGrace() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("DolphinsGraceBot", "dolphins_grace_swim_speed")
	require.NoError(t, err, "spawn agent")

	// Fill a wide, deep water pool centered on the agent's own working-area
	// origin (dynamically computed, matching water_flow_test.go's approach
	// - not a hardcoded absolute Y, since flat-world ground height isn't a
	// stable assumption to hardcode). Tall and wide enough that neither a
	// settling swim nor several seconds of forward throttle in either
	// direction reaches the floor, ceiling, or walls.
	poolX := int(math.Floor(leader.Origin.X))
	poolY := int(math.Floor(leader.Origin.Y))
	poolZ := int(math.Floor(leader.Origin.Z))
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:water",
		poolX-15, poolY, poolZ-15, poolX+15, poolY+6, poolZ+15))
	require.NoError(t, err, "fill water pool")
	time.Sleep(300 * time.Millisecond)

	teleportCmd := fmt.Sprintf("teleport %s %.1f %.1f %.1f", leader.Name, float64(poolX)+0.5, float64(poolY)+3, float64(poolZ)+0.5)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport agent into water pool")
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	swimForwardAndMeasureDistance := func() float64 {
		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok, "agent position should be initialized")

		require.NoError(t, leader.Agent.SetManualThrottle(0, 1))
		time.Sleep(1 * time.Second)
		require.NoError(t, leader.Agent.SetManualThrottle(0, 0))
		time.Sleep(1 * time.Second)

		endPos, _ := leader.Agent.GetPositionSimple()
		dx := endPos.X - pos.X
		dz := endPos.Z - pos.Z
		return math.Sqrt(dx*dx + dz*dz)
	}

	baselineDist := swimForwardAndMeasureDistance()
	t.Logf("baseline swim distance=%.4f", baselineDist)
	assert.Greater(t, baselineDist, 0.0, "ordinary forward swimming should cover some distance")

	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "re-center agent in water pool")
	time.Sleep(500 * time.Millisecond)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect give %s minecraft:dolphins_grace 30 0", leader.Name))
	require.NoError(t, err, "apply dolphins grace")
	time.Sleep(300 * time.Millisecond)

	gracedDist := swimForwardAndMeasureDistance()
	t.Logf("dolphins grace swim distance=%.4f", gracedDist)
	assert.Greater(t, gracedDist, baselineDist, "dolphins grace should cover more distance swimming than baseline in the same window")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect clear %s minecraft:dolphins_grace", leader.Name))
}

// TestWeavingReducesCobwebSlowdown verifies both halves of §4.11/§2.3
// end-to-end against a real server: that walking into cobwebs slows
// horizontal movement well below normal, and that Weaving halves that
// slowdown's severity rather than restoring full speed. Equivalent to the
// original function of the same name.
func (s *EffectsFlatSuite) TestWeavingReducesCobwebSlowdown() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("WeavingBot", "weaving_cobweb_slowdown")
	require.NoError(t, err, "spawn agent")

	// Cover a patch of ground at the agent's own working-area origin (not a
	// hardcoded absolute height), wide enough that the slowed agent never
	// walks out of it within the test's throttle windows.
	webX := int(math.Floor(leader.Origin.X))
	webY := int(math.Floor(leader.Origin.Y))
	webZ := int(math.Floor(leader.Origin.Z))
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:cobweb",
		webX-4, webY, webZ-4, webX+4, webY, webZ+4))
	require.NoError(t, err, "fill cobweb patch")
	time.Sleep(300 * time.Millisecond)

	center := fmt.Sprintf("teleport %s %.1f %.1f %.1f", leader.Name, float64(webX)+0.5, float64(webY), float64(webZ)+0.5)
	_, err = s.Inst.RCON.Exec(s.Ctx, center)
	require.NoError(t, err, "teleport agent onto cobweb patch")
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	walkForwardAndMeasureDistance := func() float64 {
		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok, "agent position should be initialized")

		require.NoError(t, leader.Agent.SetManualThrottle(0, 1))
		time.Sleep(1 * time.Second)
		require.NoError(t, leader.Agent.SetManualThrottle(0, 0))

		endPos, _ := leader.Agent.GetPositionSimple()
		dx := endPos.X - pos.X
		dz := endPos.Z - pos.Z
		return math.Sqrt(dx*dx + dz*dz)
	}

	cobwebDist := walkForwardAndMeasureDistance()
	t.Logf("cobweb (no weaving) distance=%.4f", cobwebDist)
	assert.Greater(t, cobwebDist, 0.0, "cobweb should still allow some crawl, not fully immobilize")
	assert.Less(t, cobwebDist, 1.0, "cobweb should slow movement to a crawl well under a full walking pace in one second")

	_, err = s.Inst.RCON.Exec(s.Ctx, center)
	require.NoError(t, err, "re-center agent on cobweb patch")
	time.Sleep(500 * time.Millisecond)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect give %s minecraft:weaving 30 0", leader.Name))
	require.NoError(t, err, "apply weaving")
	time.Sleep(300 * time.Millisecond)

	wovenDist := walkForwardAndMeasureDistance()
	t.Logf("cobweb + weaving distance=%.4f", wovenDist)
	assert.Greater(t, wovenDist, cobwebDist, "weaving should let the agent crawl further than plain cobweb slowdown")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("effect clear %s minecraft:weaving", leader.Name))
}
