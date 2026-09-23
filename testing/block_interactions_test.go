package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// BlockInteractionsFlatSuite is a
// version-parameterized suite for Phase-5 block-interaction tests: one
// server per version, shared by every test method below, instead of the
// previous per-test-function StartServer/StopServer pattern (each via
// setupStandaloneTestWithModeAndBlockPlacement). WorldGen = WorldGenFlat and
// Difficulty = DifficultyEasy, matching every pre-conversion TestXxx
// function's own choice.
type BlockInteractionsFlatSuite struct {
	VersionWorldSuite
}

func TestBlockInteractionsFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &BlockInteractionsFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestPowderSnowLeatherBoots verifies Phase 5 §5.1 end-to-end against a
// real server: without leather boots, walking onto a powder snow patch
// sinks the agent to the real floor beneath it (powder snow has no static
// collision box) and slows horizontal movement to a crawl; with leather
// boots equipped, the agent instead stands on top of the patch and crosses
// it at normal speed. Equivalent to the original
// TestPowderSnowSinksSlowsAndLeatherBootsFixBoth. See
// physics/state_active_effects_test.go's
// TestState_PowderSnowSlowsMovementWithoutLeatherBoots/
// TestState_LeatherBootsWalkOnPowderSnowAtNormalSpeed for the
// formula/mock-world-level coverage this builds on.
func (s *BlockInteractionsFlatSuite) TestPowderSnowLeatherBoots() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("PowderSnowBot", "powder_snow_boots")
	require.NoError(t, err, "spawn agent")

	snowX := int(math.Floor(leader.Origin.X))
	snowY := int(math.Floor(leader.Origin.Y))
	snowZ := int(math.Floor(leader.Origin.Z))
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:powder_snow",
		snowX-4, snowY, snowZ-4, snowX+4, snowY, snowZ+4))
	require.NoError(t, err, "fill powder snow patch")
	time.Sleep(300 * time.Millisecond)

	center := fmt.Sprintf("teleport %s %.1f %d %.1f", leader.Name, float64(snowX)+0.5, snowY, float64(snowZ)+0.5)
	_, err = s.Inst.RCON.Exec(s.Ctx, center)
	require.NoError(t, err, "teleport agent onto powder snow patch")
	time.Sleep(800 * time.Millisecond)

	noBootsPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")
	t.Logf("position without boots: Y=%.3f (real floor at Y=%d)", noBootsPos.Y, snowY)
	assert.InDelta(t, float64(snowY), noBootsPos.Y, 0.3,
		"without leather boots the agent should sink to the real floor below the powder snow, not stand on top of it")

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

	noBootsDist := walkForwardAndMeasureDistance()
	t.Logf("distance without boots: %.4f", noBootsDist)
	assert.Less(t, noBootsDist, 2.0, "walking through powder snow without leather boots should be slower than a normal 1-second walk")

	require.NoError(t, leader.Agent.ExitManualMode())

	// Re-center and equip leather boots.
	_, err = s.Inst.RCON.Exec(s.Ctx, center)
	require.NoError(t, err, "re-center agent on powder snow patch")
	time.Sleep(500 * time.Millisecond)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:leather_boots 1", leader.Name))
	require.NoError(t, err, "give leather boots")
	screenMgr := leader.ScreenManager()
	_, _, ok = waitForInventorySlot(screenMgr, func(index int, sl screen.Slot) bool {
		return index >= 9 && index <= 44 && sl.Count > 0
	}, 10*time.Second)
	require.True(t, ok, "leather boots never appeared in inventory")

	equipped := sayCommandUntil(t, s.Ctx, s.Inst.RCON, leader.Name, "equip leather_boots", func() bool {
		return waitForSlotState(screenMgr, 8, func(sl screen.Slot) bool { return sl.Count > 0 }, 500*time.Millisecond)
	})
	require.True(t, equipped, "'equip leather_boots' chat command never equipped them")
	t.Log("'equip leather_boots' chat command equipped them")

	// Re-teleport from ABOVE the patch, not to the same boundary Y used for
	// the no-boots case: with boots now equipped, Y=snowY is inside the
	// newly-solid powder snow block, an illegal overlap a raw RCON teleport
	// (unlike normal client movement) does not get corrected out of - it
	// leaves the agent clipped in place rather than resolving to the top
	// surface. Falling onto it from above lets normal collision resolve it
	// onto the synthetic solid box at Y=snowY+1 instead.
	bootsCenter := fmt.Sprintf("teleport %s %.1f %d %.1f", leader.Name, float64(snowX)+0.5, snowY+3, float64(snowZ)+0.5)
	_, err = s.Inst.RCON.Exec(s.Ctx, bootsCenter)
	require.NoError(t, err, "teleport booted agent above powder snow patch")
	time.Sleep(1200 * time.Millisecond)

	bootedPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")
	t.Logf("position with boots: Y=%.3f (snow surface at Y=%d)", bootedPos.Y, snowY+1)
	assert.InDelta(t, float64(snowY)+1.0, bootedPos.Y, 0.3,
		"with leather boots the agent should stand on top of the powder snow, not sink into it")

	require.NoError(t, leader.Agent.EnterManualMode(), "re-enter manual movement mode")
	bootedDist := walkForwardAndMeasureDistance()
	t.Logf("distance with boots: %.4f", bootedDist)
	assert.Greater(t, bootedDist, noBootsDist, "leather boots should let the agent cross powder snow much faster than without them")
}

// TestHoneyBlockMovement verifies Phase 5 §5.2 end-to-end: standing on
// honey slows horizontal movement (velocityMultiplier) and reduces jump
// height (jumpVelocityMultiplier) relative to normal ground, both via the
// real chat/manual-movement pipeline against a live server. Equivalent to
// the original TestHoneyBlockSlowsMovementAndReducesJumpHeight. See
// physics/state_active_effects_test.go's
// TestState_HoneyBlockSlowsGroundMovement/TestState_HoneyBlockReducesJumpHeight
// for the mock-world-level coverage this builds on.
func (s *BlockInteractionsFlatSuite) TestHoneyBlockMovement() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("HoneyMoveBot", "honey_block_movement")
	require.NoError(t, err, "spawn agent")

	honeyX := int(math.Floor(leader.Origin.X))
	honeyY := int(math.Floor(leader.Origin.Y)) - 1 // replace the floor itself with honey
	honeyZ := int(math.Floor(leader.Origin.Z))
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:honey_block",
		honeyX-4, honeyY, honeyZ-4, honeyX+4, honeyY, honeyZ+4))
	require.NoError(t, err, "fill honey block floor")
	time.Sleep(300 * time.Millisecond)

	honeyCenter := fmt.Sprintf("teleport %s %.1f %d %.1f", leader.Name, float64(honeyX)+0.5, honeyY+1, float64(honeyZ)+0.5)
	normalCenter := fmt.Sprintf("teleport %s %.1f %d %.1f", leader.Name, float64(honeyX)+30, honeyY+1, float64(honeyZ)+0.5)

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

	measurePeakJumpHeight := func() float64 {
		startPos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok, "agent position should be initialized")

		require.NoError(t, leader.Agent.SetManualJump(true))
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, leader.Agent.SetManualJump(false))

		peak := startPos.Y
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) {
			pos, ok := leader.Agent.GetPositionSimple()
			if ok && pos.Y > peak {
				peak = pos.Y
			}
			time.Sleep(50 * time.Millisecond)
		}
		return peak - startPos.Y
	}

	_, err = s.Inst.RCON.Exec(s.Ctx, honeyCenter)
	require.NoError(t, err, "teleport agent onto honey block floor")
	time.Sleep(500 * time.Millisecond)

	honeyDist := walkForwardAndMeasureDistance()
	t.Logf("distance on honey: %.4f", honeyDist)

	_, err = s.Inst.RCON.Exec(s.Ctx, honeyCenter)
	require.NoError(t, err, "re-center agent on honey block floor")
	time.Sleep(500 * time.Millisecond)

	honeyPeak := measurePeakJumpHeight()
	t.Logf("jump height on honey: %.4f", honeyPeak)

	_, err = s.Inst.RCON.Exec(s.Ctx, normalCenter)
	require.NoError(t, err, "teleport agent to normal ground")
	time.Sleep(500 * time.Millisecond)

	normalDist := walkForwardAndMeasureDistance()
	t.Logf("distance on normal ground: %.4f", normalDist)

	_, err = s.Inst.RCON.Exec(s.Ctx, normalCenter)
	require.NoError(t, err, "re-center agent on normal ground")
	time.Sleep(500 * time.Millisecond)

	normalPeak := measurePeakJumpHeight()
	t.Logf("jump height on normal ground: %.4f", normalPeak)

	assert.Greater(t, normalDist, honeyDist, "honey block should slow horizontal movement below normal ground")
	assert.Greater(t, normalPeak, honeyPeak, "honey block should reduce jump height below normal ground")
	assert.Greater(t, honeyPeak, 0.0, "honey block should still allow some jump, not fully block it")
}

// TestHoneyBlockSideSlide verifies Phase 5 §5.2's other descoped-then-
// implemented item: falling directly alongside (not on top of) a tall
// honey column caps descent to a slow glide and continually resets fall
// distance, mirroring HoneyBlock.isSliding/updateSlidingVelocity.
// Equivalent to the original TestHoneyBlockSideSlideAvoidsFallDamage.
// See physics/state.go's isSlidingOnHoney and
// physics/state_active_effects_test.go's
// TestState_HoneyBlockSideSlideCapsDescent for the mock-world-level
// coverage this builds on. Checked via real fall damage (server-
// authoritative, per TestElytraFlareLanding's doc comment on why that's the
// reliable signal), not client-side velocity sampling: a drop this tall
// would deal substantial damage without the slide mechanic.
func (s *BlockInteractionsFlatSuite) TestHoneyBlockSideSlide() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("HoneySlideBot", "honey_side_slide")
	require.NoError(t, err, "spawn agent")

	pillarX := int(math.Floor(leader.Origin.X))
	floorY := int(math.Floor(leader.Origin.Y))
	z := int(math.Floor(leader.Origin.Z))

	// Landing floor for both the pillar and the adjacent fall column.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:stone",
		pillarX-1, floorY, z-1, pillarX+2, floorY, z+1))
	require.NoError(t, err, "build landing floor")

	// A tall honey pillar the agent will fall directly alongside. Kept
	// modest (not e.g. 19 blocks) because honey's slide rate is
	// deliberately slow - HoneySlideDescentRate (0.05 blocks/tick, the real
	// vanilla steady-state target) means traversing a taller pillar this
	// way would take many seconds longer than this test's wait below
	// budgets for.
	pillarTop := floorY + 10
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:honey_block",
		pillarX, floorY+1, z, pillarX, pillarTop, z))
	require.NoError(t, err, "build honey pillar")
	time.Sleep(500 * time.Millisecond)

	startHealth, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get starting health")
	t.Logf("starting health: %.1f", startHealth)

	// Fall in the column immediately beside the pillar, close enough to
	// trigger isSlidingOnHoney's block-cell-level overlap check (see its
	// doc comment - it doesn't care about exact sub-block position), but
	// outside honey's REAL collision box, which is horizontally inset to
	// [0.0625, 0.9375] within its cell (confirmed against mc-data-gen's
	// collision_boxes data, not assumed) - an earlier offset here (0.85)
	// overlapped that real box enough to let the agent actually land ON the
	// pillar instead of falling past its side. With player half-width 0.3,
	// the valid "same cell, no real overlap" window is
	// [pillarX+1.2375, pillarX+1.3); 1.28 sits safely inside it.
	fallX := float64(pillarX) + 1.28
	fallZ := float64(z) + 0.5
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"teleport %s %.2f %d %.2f", leader.Name, fallX, pillarTop+3, fallZ))
	require.NoError(t, err, "teleport agent above the honey pillar")

	// Wait for the fall, slide, and landing to complete. At
	// HoneySlideDescentRate (0.05 blocks/tick, 1 block/sec) the ~13-block
	// total drop takes on the order of 13 seconds once sliding engages,
	// plus a brief initial free-fall before it does.
	time.Sleep(16 * time.Second)

	endPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")
	t.Logf("landed at Y=%.3f (floor at Y=%d)", endPos.Y, floorY)
	assert.InDelta(t, float64(floorY+1), endPos.Y, 2.0, "agent should have landed on the floor beside the honey pillar")

	endHealth, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get ending health")
	t.Logf("ending health: %.1f", endHealth)
	assert.InDelta(t, startHealth, endHealth, 0.5,
		"sliding down the side of a %d-block honey pillar should avoid fall damage", pillarTop+3-floorY)
}

// TestScaffoldingClimbAndSneak verifies Phase 5 §5.3's descoped-then-
// implemented items end-to-end: holding jump while touching scaffolding
// grabs and climbs it with no explicit climb-direction input (the
// jump-to-climb mechanic - see physics/constants.go's JumpToClimbBoost),
// and sneaking on scaffolding does not freeze descent the way it does on a
// ladder. Equivalent to the original
// TestScaffoldingJumpToClimbAndSneakDoesNotFreezeDescent (already covered
// at the mock-world level by physics/state_test.go's
// TestState_ScaffoldingJumpToClimb/TestState_ScaffoldingSneakingDoesNotPreventDescend).
func (s *BlockInteractionsFlatSuite) TestScaffoldingClimbAndSneak() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ScaffoldBot", "scaffolding_climb")
	require.NoError(t, err, "spawn agent")

	baseX := int(math.Floor(leader.Origin.X))
	baseY := int(math.Floor(leader.Origin.Y))
	baseZ := int(math.Floor(leader.Origin.Z))

	// A tall scaffolding column, comfortably taller than the jump-to-climb
	// ascent this test drives, so the agent stays mid-column (still
	// touching climbable space) throughout.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:scaffolding",
		baseX, baseY, baseZ, baseX, baseY+20, baseZ))
	require.NoError(t, err, "fill scaffolding column")
	time.Sleep(300 * time.Millisecond)

	center := fmt.Sprintf("teleport %s %.1f %d %.1f", leader.Name, float64(baseX)+0.5, baseY, float64(baseZ)+0.5)
	_, err = s.Inst.RCON.Exec(s.Ctx, center)
	require.NoError(t, err, "teleport agent onto scaffolding column base")
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	startPos2, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")

	// Jump-to-climb: hold jump with no explicit climb-direction input.
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(1200 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))

	climbedPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")
	t.Logf("climbed from Y=%.3f to Y=%.3f via jump-to-climb", startPos2.Y, climbedPos.Y)
	assert.Greater(t, climbedPos.Y, startPos2.Y+1.0,
		"holding jump while touching scaffolding should climb it, with no explicit climb-direction input")

	// Let residual vertical velocity settle before measuring sneak
	// descent, so the sneak phase isn't contaminated by leftover climb
	// momentum.
	time.Sleep(500 * time.Millisecond)
	preSneakPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")

	// Sneaking on a ladder would freeze descent; on scaffolding it should
	// not.
	require.NoError(t, leader.Agent.SetManualSneak(true))
	time.Sleep(2 * time.Second)
	afterSneakPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")
	require.NoError(t, leader.Agent.SetManualSneak(false))

	t.Logf("sneaking on scaffolding: Y=%.3f -> Y=%.3f", preSneakPos.Y, afterSneakPos.Y)
	assert.Less(t, afterSneakPos.Y, preSneakPos.Y-0.5,
		"sneaking on scaffolding should not freeze descent the way it does on a ladder")
}

// TestIceGroundFriction verifies the descoped-then-implemented ice
// ground-friction item end-to-end: ice (a genuine
// AbstractBlock.Settings.slipperiness(0.98F) value) accelerates slower than
// normal ground from a standing start but carries much more momentum once
// moving. Equivalent to the original
// TestIceCoastsFurtherThanStoneAfterThrottleRelease, matching
// physics/state_active_effects_test.go's
// TestState_IceAcceleratesSlowerButCoastsFurther.
func (s *BlockInteractionsFlatSuite) TestIceGroundFriction() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("IceFrictionBot", "ice_ground_friction")
	require.NoError(t, err, "spawn agent")

	iceX := int(math.Floor(leader.Origin.X))
	iceY := int(math.Floor(leader.Origin.Y)) - 1
	iceZ := int(math.Floor(leader.Origin.Z))
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:ice",
		iceX-6, iceY, iceZ-6, iceX+6, iceY, iceZ+6))
	require.NoError(t, err, "fill ice floor")
	time.Sleep(300 * time.Millisecond)

	iceCenter := fmt.Sprintf("teleport %s %.1f %d %.1f", leader.Name, float64(iceX)+0.5, iceY+1, float64(iceZ)+0.5)
	normalCenter := fmt.Sprintf("teleport %s %.1f %d %.1f", leader.Name, float64(iceX)+30, iceY+1, float64(iceZ)+0.5)

	_, err = s.Inst.RCON.Exec(s.Ctx, iceCenter)
	require.NoError(t, err, "teleport agent onto ice floor")
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	burstThenCoast := func() (burstDist, coastDist float64) {
		startPos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok, "agent position should be initialized")

		require.NoError(t, leader.Agent.SetManualThrottle(0, 1))
		time.Sleep(750 * time.Millisecond)
		require.NoError(t, leader.Agent.SetManualThrottle(0, 0))

		burstPos, _ := leader.Agent.GetPositionSimple()
		dx := burstPos.X - startPos.X
		dz := burstPos.Z - startPos.Z
		burstDist = math.Sqrt(dx*dx + dz*dz)

		time.Sleep(1500 * time.Millisecond) // coast, no input
		coastPos, _ := leader.Agent.GetPositionSimple()
		dx = coastPos.X - burstPos.X
		dz = coastPos.Z - burstPos.Z
		coastDist = math.Sqrt(dx*dx + dz*dz)
		return
	}

	iceBurst, iceCoast := burstThenCoast()
	t.Logf("on ice: burst=%.4f coast=%.4f", iceBurst, iceCoast)

	_, err = s.Inst.RCON.Exec(s.Ctx, normalCenter)
	require.NoError(t, err, "teleport agent to normal ground")
	time.Sleep(500 * time.Millisecond)

	normalBurst, normalCoast := burstThenCoast()
	t.Logf("on normal ground: burst=%.4f coast=%.4f", normalBurst, normalCoast)

	assert.Greater(t, normalBurst, iceBurst, "ice should accelerate slower than normal ground from a standing start")
	assert.Greater(t, iceCoast, normalCoast, "ice should carry momentum further than normal ground once moving")
}
