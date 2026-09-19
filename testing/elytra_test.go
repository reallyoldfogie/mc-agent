package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// sayCommand sends a real chat message addressed to the bot via RCON's
// `/say`, exercising the actual chat pipeline (agent.OnDisguisedChat ->
// handleChatCommand -> the real command registry) rather than calling the
// underlying agent method directly - this is what proves the commands are
// actually *wired*, not just that the underlying capability works. Takes
// an RCONHelper directly (not *StandaloneTestEnv) so shared-server suite
// methods (VersionWorldSuite's s.Inst.RCON) can call it too, not just the
// pre-Phase-1 per-test-server pattern's env.Inst.RCON.
func sayCommand(t *testing.T, ctx context.Context, rcon testenv.RCONHelper, botName, command string) {
	t.Helper()
	_, err := rcon.Exec(ctx, fmt.Sprintf("say >>>%s<<< %s", botName, command))
	require.NoError(t, err, "send chat command %q", command)
}

// sayCommandUntil sends command via chat, repeating it if condition doesn't
// become and stay true within a few seconds. This server/RCON combination
// (or possibly RCON's own retry-on-EOF logic in mc-client-test-go) can
// deliver a single `say` invocation as more than one distinct chat packet -
// confirmed live via a raw packet-byte dump for one case (two
// byte-identical ClientboundProfilelessChat packets), and via a second,
// non-byte-identical case for another (a real second "Equip failed" side
// effect logged ~500ms after the first command's own success, consistent
// with the SAME logical command executing twice but not from a literal
// packet repeat this time). Neither is a bug in the production dispatch
// path - the production side already drops exact byte-for-byte repeats
// (see dedupeChatPacket in agent/game_chat.go); a real player's client
// only ever sends a chat message once, so this is specific to how this
// RCON-driven test triggers commands. Rather than fight it further at the
// dispatch layer (an earlier attempt deduplicated on decoded command text
// within a time window instead of raw bytes, which broke tests that
// intentionally send the same no-op-safe command twice in a row, e.g.
// stopFollow, to check both the normal and "not following" responses),
// this requires the condition to hold not just once but again after a
// settle delay, so a stray second delivery landing shortly after the first
// one's success - which can undo it, e.g. a second "equip firework_rocket"
// shift-clicking the already-selected item back out of the hotbar - gets
// detected and the whole send+wait cycle retried instead of silently
// proceeding on a state that's about to change underneath it.
func sayCommandUntil(t *testing.T, ctx context.Context, rcon testenv.RCONHelper, botName, command string, condition func() bool) bool {
	t.Helper()
	const settleDelay = 700 * time.Millisecond
	for attempt := 0; attempt < 4; attempt++ {
		sayCommand(t, ctx, rcon, botName, command)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if condition() {
				time.Sleep(settleDelay)
				if condition() {
					return true
				}
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return condition()
}

// ensureFireworkBoost fires another firework rocket if the agent is
// currently gliding without an active boost — call this periodically
// throughout a flight so it doesn't stall out on gravity alone once each
// rocket's boost (well under a second by default) lapses.
func ensureFireworkBoost(t *testing.T, agent models.Agent) {
	t.Helper()
	if !agent.HasActiveFireworkBoost() {
		_ = agent.UseFireworkRocket()
	}
}

// ElytraFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite consolidating every elytra test (previously
// split across elytra_test.go, elytra_commands_test.go,
// elytra_flare_landing_test.go, elytra_navigation_test.go, and
// elytra_unequip_test.go - each container-free and already using the
// identical setupStandaloneTestWithModeAndBlockPlacement configuration:
// survival, DifficultyEasy, no placed block, replay enabled): one server
// per version, shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern.
type ElytraFlatSuite struct {
	VersionWorldSuite
}

func TestElytraFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ElytraFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// equipElytraViaShiftClick gives leader an elytra and equips it via a real
// shift-click (not RCON's `item replace entity ... armor.chest with ...`,
// which does change the entity's equipment server-side but isn't reliably
// broadcast to observers only just then starting to track the entity -
// confirmed live via a real replay recording where a companion observer
// agent showed the bot never wearing the elytra despite gliding correctly).
// A shift-click goes through the same server-authoritative container-click
// path a real player's equip action would.
func equipElytraViaShiftClick(t *testing.T, s *ElytraFlatSuite, leader *WorkingAreaAgent) {
	t.Helper()
	_, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:elytra 1", leader.Name))
	require.NoError(t, err, "give elytra")

	screenMgr := leader.ScreenManager()
	elytraSlot, elytraSlotData, ok := waitForInventorySlot(screenMgr, func(index int, sl screen.Slot) bool {
		return index >= 9 && index <= 44 && sl.Count > 0
	}, 10*time.Second)
	require.True(t, ok, "elytra never appeared in bot's main/hotbar inventory")

	require.NoError(t, leader.Agent.ShiftClickSlot(int16(elytraSlot), slotToItemStack(elytraSlotData)), "shift-click elytra to equip it")
	require.True(t, waitForSlotState(screenMgr, 6, func(sl screen.Slot) bool {
		return sl.Count > 0
	}, 10*time.Second), "elytra never landed in the chest armor slot")
	t.Log("elytra equipped via shift-click")
}

// giveFireworkRockets gives leader n fireworks and waits for them to
// arrive in inventory before returning.
func giveFireworkRockets(t *testing.T, s *ElytraFlatSuite, leader *WorkingAreaAgent, n int) {
	t.Helper()
	_, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:firework_rocket %d", leader.Name, n))
	require.NoError(t, err, "give firework rockets")
	_, _, ok := waitForInventorySlot(leader.ScreenManager(), func(index int, sl screen.Slot) bool {
		return index >= 9 && index <= 44 && sl.Count > 0
	}, 10*time.Second)
	require.True(t, ok, "firework rockets never appeared in bot's main/hotbar inventory")
}

// TestGlideSlowsDescentAndAddsForwardMotion verifies that jumping while
// airborne with an elytra equipped triggers real elytra-gliding physics
// end-to-end against a real server: much slower descent than a plain
// fall, plus real forward motion in the look direction (which plain
// falling never produces, since there's no WASD thrust in the air) — see
// physics/elytra.go's GlidingVelocity and physics/state_elytra_test.go for
// the formula-level and state-level coverage this builds on. Equivalent to
// the pre-Phase-1 TestElytraGlideSlowsDescentAndAddsForwardMotion.
func (s *ElytraFlatSuite) TestGlideSlowsDescentAndAddsForwardMotion() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ElytraGlideBot", "elytra_glide")
	require.NoError(t, err, "spawn agent")

	equipElytraViaShiftClick(t, s, leader)

	// Teleport well clear of the ground, facing south (yaw 0) and looking
	// level (pitch 0), so gliding forward motion accumulates in a known
	// (+Z) direction.
	tpCmd := fmt.Sprintf("teleport %s %.2f %.2f %.2f 0 0", leader.Name, leader.Origin.X, leader.Origin.Y+openAirY, leader.Origin.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, tpCmd)
	require.NoError(t, err, "teleport agent into open air")
	time.Sleep(300 * time.Millisecond)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()
	require.NoError(t, leader.Agent.SetManualRotation(0, 0), "face south, level")

	startPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")

	// Jump while airborne to trigger the client-predicted checkGliding()
	// transition (see physics/state.go's applyGlideStateTransition) - held
	// briefly, not just one tick, to absorb scheduling jitter between this
	// goroutine and the physics tick loop.
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))

	time.Sleep(3 * time.Second)

	endPos, _ := leader.Agent.GetPositionSimple()
	drop := startPos.Y - endPos.Y
	forwardDist := endPos.Z - startPos.Z
	t.Logf("start=(%.2f,%.2f,%.2f) end=(%.2f,%.2f,%.2f) drop=%.3f forwardDist=%.3f",
		startPos.X, startPos.Y, startPos.Z, endPos.X, endPos.Y, endPos.Z, drop, forwardDist)

	// A 3-second plain fall from rest covers roughly 60 blocks under this
	// codebase's gravity/drag constants (steady state well above 15
	// blocks/sec); gliding at level pitch should cover a small fraction of
	// that.
	assert.Less(t, drop, 20.0, "elytra gliding at level pitch should descend far slower than an ordinary fall")
	assert.Greater(t, forwardDist, 1.0, "gliding while facing south should accumulate real forward (+Z) motion, unlike a plain fall")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("item replace entity %s armor.chest with air", leader.Name))
}

// TestDoubleJumpFireworkRocketTakeoff verifies the real "double jump"
// takeoff sequence real players use — jump once (a normal ground jump),
// release jump, then press it again while still airborne — actually starts
// gliding (exercising physics/state.go's applyGlideStateTransition
// rising-edge fix; see physics/state_elytra_test.go's
// TestState_ElytraRequiresFreshJumpPressAfterGroundJump for the unit-level
// coverage of why a held jump must NOT also trigger this), then that
// immediately using firework rockets while gliding — facing almost straight
// up, re-firing as soon as each one's boost lapses — produces a real,
// dramatic near-vertical launch (physics/elytra.go's FireworkBoostVelocity)
// starting from flat ground, end to end against a real server.
//
// An earlier version of this test faced level (yaw/pitch 0,0) and needed an
// elevated platform to launch from: at level pitch, calcGlidingVelocity's
// own dive/climb terms depend on a look-direction *pitch*, and a single
// boost mostly converts speed into forward distance rather than altitude,
// so a flat-ground takeoff had already landed again (confirmed via physics
// tick logs, not assumed) before gaining any real height. Facing steeply up
// changes what the boost's velocity nudge is aimed at: FireworkBoostVelocity
// uses the full 3D look vector (not just its horizontal component the way
// GlidingVelocity's own ease term does), so pointing it mostly along +Y
// turns the same formula into a near-vertical thrust instead - matching how
// real players launch almost straight up. Equivalent to the pre-Phase-1
// TestElytraDoubleJumpFireworkRocketTakeoff.
func (s *ElytraFlatSuite) TestDoubleJumpFireworkRocketTakeoff() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ElytraTakeoffBot", "elytra_firework_takeoff")
	require.NoError(t, err, "spawn agent")

	// Clear a tall column above spawn for the vertical launch - no
	// platform, no teleport, this starts right from natural ground spawn.
	clearX := int(math.Floor(leader.Origin.X))
	clearZ := int(math.Floor(leader.Origin.Z))
	clearY := int(math.Floor(leader.Origin.Y))
	require.NoError(t, ClearArea(s.Ctx, s.Inst.RCON, clearX-15, clearY+1, clearZ-15, clearX+15, clearY+80, clearZ+15), "clear vertical launch airspace")
	time.Sleep(300 * time.Millisecond)

	equipElytraViaShiftClick(t, s, leader)

	// Give several firework rockets - with the elytra now moved out of the
	// hotbar into the chest slot, these land back in the already-selected
	// hotbar slot (0), ready to use immediately, and there's enough to
	// re-fire a few times as each one's boost runs out.
	giveFireworkRockets(t, s, leader, 5)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	// Face nearly straight up. Not exactly -90: GlidingVelocity's own
	// horizontal-direction terms divide by the look vector's horizontal
	// magnitude, which is only exactly zero at a true -90 - staying just
	// short of it keeps those terms numerically well-behaved (they remain
	// well-defined near-zero, but there's no reason to court the exact
	// edge) while still being "almost straight up" the way real players
	// fly it.
	const climbPitch = -80.0
	require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))

	startPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "agent position should be initialized")
	t.Logf("takeoff start position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

	// The real double-jump takeoff: a normal ground jump (single press), a
	// genuine release, then a fresh press while airborne. JumpVelocity is
	// small (the whole hop lasts under half a second), so this must all
	// happen well within that window.
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))

	// Fire the rocket immediately - real players don't glide first, since
	// every tick spent gliding-only from a cold, near-zero speed start
	// burns altitude for very little gain. A brief settle (one or two
	// physics ticks) just lets the glide-start transition actually land
	// before the use-item packet goes out.
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.UseFireworkRocket(), "use firework rocket to launch")

	// Climb for a few seconds, re-firing as soon as each boost lapses (a
	// default firework's boost lasts under a second) so the climb doesn't
	// stall out on gravity alone between them.
	climbDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(climbDeadline) {
		require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))
		if !leader.Agent.HasActiveFireworkBoost() {
			_ = leader.Agent.UseFireworkRocket()
		}
		time.Sleep(150 * time.Millisecond)
	}

	endPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)

	climb := endPos.Y - startPos.Y
	t.Logf("after double-jump + repeated near-vertical firework boosts: start=(%.2f,%.2f,%.2f) end=(%.2f,%.2f,%.2f) climb=%.3f",
		startPos.X, startPos.Y, startPos.Z, endPos.X, endPos.Y, endPos.Z, climb)

	// A single boost alone reaches a steady-state ~1.7 blocks/tick
	// (physics/elytra_test.go's TestFireworkBoostVelocity); sustained
	// near-vertically for 3 seconds with re-firing, real altitude gain
	// should be large and unambiguous - a plain, unboosted double-jump
	// gains only a fraction of a block.
	assert.Greater(t, climb, 20.0, "repeated near-vertical firework boosts after a double-jump should gain a large amount of altitude")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("item replace entity %s armor.chest with air", leader.Name))
}

// TestEquipAndUseItem verifies the "equip" and "useItem" chat commands end
// to end against a real server: "equip elytra" wears it (via a real
// shift-click), "equip firework_rocket" selects it into the hand (the
// "select rockets" step), and "useItem" - sent as a real chat message, not
// called directly - fires the currently held firework while gliding and
// produces a real, measurable velocity boost. Equivalent to the pre-Phase-1
// TestChatCommand_EquipAndUseItem.
func (s *ElytraFlatSuite) TestEquipAndUseItem() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ElytraEquipUseBot", "chat_equip_useitem")
	require.NoError(t, err, "spawn agent")

	clearX := int(math.Floor(leader.Origin.X))
	clearZ := int(math.Floor(leader.Origin.Z))
	clearY := int(math.Floor(leader.Origin.Y))
	require.NoError(t, ClearArea(s.Ctx, s.Inst.RCON, clearX-15, clearY+1, clearZ-15, clearX+15, clearY+40, clearZ+15), "clear airspace")
	time.Sleep(300 * time.Millisecond)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:elytra 1", leader.Name))
	require.NoError(t, err, "give elytra")
	screenMgr := leader.ScreenManager()
	_, _, ok := waitForInventorySlot(screenMgr, func(index int, sl screen.Slot) bool {
		return index >= 9 && index <= 44 && sl.Count > 0
	}, 10*time.Second)
	require.True(t, ok, "elytra never appeared in inventory")

	equipped := sayCommandUntil(t, s.Ctx, s.Inst.RCON, leader.Name, "equip elytra", func() bool {
		return waitForSlotState(screenMgr, 6, func(sl screen.Slot) bool { return sl.Count > 0 }, 500*time.Millisecond)
	})
	require.True(t, equipped, "'equip elytra' chat command never equipped it")
	t.Log("'equip elytra' chat command equipped the elytra")

	giveFireworkRockets(t, s, leader, 5)

	selected := sayCommandUntil(t, s.Ctx, s.Inst.RCON, leader.Name, "equip firework_rocket", func() bool {
		return waitForSlotState(screenMgr, 36, func(sl screen.Slot) bool { return sl.Count > 0 }, 500*time.Millisecond)
	})
	require.True(t, selected, "'equip firework_rocket' chat command never selected it into hand")
	t.Log("'equip firework_rocket' chat command selected it into hand")

	// Get airborne and gliding directly (not via chat - there's no "jump"
	// command), so there's a real elytra flight in progress for "useItem"
	// to boost. Pitch steeply up (matching the other elytra tests' takeoff,
	// not a level 0 pitch) so the glide has real airborne time to work
	// with: an earlier version of this test double-jumped level, which
	// barely leaves the ground and lands again in well under a second
	// (confirmed live via position/IsGliding polling) - not enough time
	// for a real server round trip plus the "useItem" chat command's own
	// processing to land before it's too late to observe a boost.
	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()
	const climbPitch = -80.0
	require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.True(t, leader.Agent.IsGliding(), "double-jump should have started gliding")

	_, preVY, _, preOK := leader.Agent.GetVelocity()
	require.True(t, preOK)
	t.Logf("velocity before 'useItem': vy=%.3f", preVY)

	boosted := sayCommandUntil(t, s.Ctx, s.Inst.RCON, leader.Name, "useItem", func() bool {
		return leader.Agent.HasActiveFireworkBoost()
	})
	assert.True(t, boosted, "'useItem' chat command should have used the firework rocket and started a boost")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("item replace entity %s armor.chest with air", leader.Name))
}

// TestFlyTo verifies the "flyTo" chat command end to end against a real
// server: from natural ground spawn (no platform/teleport, matching the
// other elytra tests), a single chat message takes off, cruises to a
// distant target, and lands - reusing the takeoff/cruise/land sequence
// validated by TestGlideSlowsDescentAndAddsForwardMotion and
// TestFlightNavigatesWaypointsAndLands above/below, now driven entirely
// through the real chat command pipeline rather than direct Go calls.
// Equivalent to the pre-Phase-1 TestChatCommand_FlyTo.
func (s *ElytraFlatSuite) TestFlyTo() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ElytraFlyToBot", "chat_flyto")
	require.NoError(t, err, "spawn agent")

	clearX := int(math.Floor(leader.Origin.X))
	clearZ := int(math.Floor(leader.Origin.Z))
	clearY := int(math.Floor(leader.Origin.Y))
	require.NoError(t, ClearArea(s.Ctx, s.Inst.RCON, clearX-40, clearY+1, clearZ-20, clearX+40, clearY+60, clearZ+220), "clear flight airspace")
	time.Sleep(300 * time.Millisecond)

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:elytra 1", leader.Name))
	require.NoError(t, err, "give elytra")
	screenMgr := leader.ScreenManager()
	_, _, ok := waitForInventorySlot(screenMgr, func(index int, sl screen.Slot) bool {
		return index >= 9 && index <= 44 && sl.Count > 0
	}, 10*time.Second)
	require.True(t, ok, "elytra never appeared in inventory")

	sayCommand(t, s.Ctx, s.Inst.RCON, leader.Name, "equip elytra")
	require.True(t, waitForSlotState(screenMgr, 6, func(sl screen.Slot) bool {
		return sl.Count > 0
	}, 10*time.Second), "'equip elytra' chat command never equipped it")

	giveFireworkRockets(t, s, leader, 20)

	targetX := leader.Origin.X
	targetY := leader.Origin.Y
	targetZ := leader.Origin.Z + 200

	sayCommand(t, s.Ctx, s.Inst.RCON, leader.Name, fmt.Sprintf("flyTo %.0f %.0f %.0f", targetX, targetY, targetZ))

	var finalPos models.V3
	arrived := false
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok)
		finalPos = pos
		dx, dz := targetX-pos.X, targetZ-pos.Z
		if math.Sqrt(dx*dx+dz*dz) <= 15.0 && pos.Y <= leader.Origin.Y+3.0 {
			arrived = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("final position: (%.2f, %.2f, %.2f), target: (%.0f, %.0f, %.0f)", finalPos.X, finalPos.Y, finalPos.Z, targetX, targetY, targetZ)
	assert.True(t, arrived, "'flyTo' chat command should have flown near the target and landed within the timeout")

	_ = leader.Agent.ExitManualMode()
	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("item replace entity %s armor.chest with air", leader.Name))
}

// TestFlareLanding verifies the real "flare" landing technique elytra
// pilots use to avoid fall damage without a long, gradual glide down: come
// in close to the ground at speed, then look sharply up (no rocket boost
// involved) to bleed horizontal speed into a brief climb, and drop the
// remaining short distance to the ground gently.
//
// This works because of GlidingVelocity's "pitching up trades forward
// speed for lift" term (physics/elytra.go): looking up sharply converts a
// large fraction of horizontal speed into upward velocity in a single tick,
// which is exactly what real vanilla's LivingEntity.limitFallDistance()
// needs to avoid fall damage - it caps accumulated fall distance to 1 block
// every tick the entity's vertical velocity is above -0.5, but only while
// gliding (LivingEntity.tickGliding() is what calls it). A steep, sustained
// dive straight into the ground never gets that chance; flaring does.
// Fall damage itself is server-authoritative (this codebase's own
// physics.State doesn't implement limitFallDistance()'s cap at all, since
// client-side fallDistance here is only used for pathfinding drop-cost
// estimation, not damage) - checked via a real RCON health read before and
// after, not inferred from anything client-side. Equivalent to the
// pre-Phase-1 TestElytraFlareLanding.
func (s *ElytraFlatSuite) TestFlareLanding() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ElytraFlareBot", "elytra_flare_landing")
	require.NoError(t, err, "spawn agent")

	clearX := int(math.Floor(leader.Origin.X))
	clearZ := int(math.Floor(leader.Origin.Z))
	clearY := int(math.Floor(leader.Origin.Y))
	require.NoError(t, ClearArea(s.Ctx, s.Inst.RCON, clearX-40, clearY+1, clearZ-10, clearX+40, clearY+60, clearZ+150), "clear flare-approach airspace")
	time.Sleep(300 * time.Millisecond)

	startHealth, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get starting health")
	t.Logf("starting health: %.1f", startHealth)

	equipElytraViaShiftClick(t, s, leader)
	giveFireworkRockets(t, s, leader, 5)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	const climbPitch = -80.0
	const steerInterval = 100 * time.Millisecond
	require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))

	startPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("start position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

	// Double-jump takeoff (see TestDoubleJumpFireworkRocketTakeoff).
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.UseFireworkRocket(), "use firework rocket to launch")

	// A brief, single-boost climb (no re-boosting) - just enough altitude
	// for a controlled approach, not a sustained ascent. Earlier versions
	// of this test used a longer, re-boosted climb (matching
	// TestDoubleJumpFireworkRocketTakeoff's technique) and found the
	// resulting momentum was still huge once it cascaded through leveling
	// off and diving: flaring converted it into a climb straight back up
	// past 200 blocks instead of a gentle settle - a real, dramatic
	// "balloon" this physics genuinely produces at high energy, just not
	// what a landing-technique test wants to demonstrate.
	const climbDuration = 800 * time.Millisecond
	climbDeadline := time.Now().Add(climbDuration)
	for time.Now().Before(climbDeadline) {
		require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))
		time.Sleep(steerInterval)
	}

	apexPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("apex before diving approach: (%.2f, %.2f, %.2f)", apexPos.X, apexPos.Y, apexPos.Z)

	// Coast level for a moment before diving. Momentum doesn't reverse
	// instantly: an earlier version of this test switched straight from
	// climb pitch to dive pitch and found the agent was still ascending
	// (vertical velocity still positive from the climb) for the entire
	// "dive" window that followed - it actually gained altitude instead of
	// losing it, confirmed by the logged apex/flare positions, not
	// assumed. A brief level coast lets gravity bleed off the residual
	// climb momentum first, so the dive that follows is a real dive.
	require.NoError(t, leader.Agent.SetManualRotation(0, 0))
	time.Sleep(600 * time.Millisecond)

	// Now dive - steep enough to actually lose altitude quickly. A shallow
	// dive (pitch 20) was tried too and barely descends at all: gliding's
	// horizontal-ease term keeps building forward speed for as long as the
	// dive continues, and a shallow dive took long enough (100+ blocks of
	// travel to lose just 15 blocks of altitude) that accumulated speed
	// was enormous by the time it reached the flare trigger - the same
	// "balloon on flare" problem, just from a different cause (dive
	// duration instead of climb energy). A steep dive converts altitude to
	// speed quickly instead of slowly, bounding total energy buildup.
	const divePitch = 40.0
	require.NoError(t, leader.Agent.SetManualRotation(0, divePitch))

	// Approach: keep diving until close to the ground, then flare - look
	// sharply up, no further rocket use - and ride it down. Also bounded
	// by a maximum dive duration as a second safety net against excess
	// speed buildup, independent of altitude.
	//
	// flareAltitude needs real margin, not just "close to the ground": an
	// earlier version of this test flared at 6 blocks (or wherever
	// maxDiveDuration cut the dive short, sometimes as low as ~3.5) and
	// took real, sometimes lethal fall damage on most versions - confirmed
	// via a live 6-version run, not assumed. Vanilla's
	// LivingEntity.limitFallDistance() only caps accumulated fall distance
	// while vertical velocity is above -0.5, and that condition takes a
	// tick or two of the flare's own upward conversion to actually reach
	// after diving in fast - if the ground arrives first, no cap ever
	// applies and the fall damage already built up during the dive lands
	// in full, real crashes included. Flaring with real altitude to spare
	// gives that conversion time to finish before impact.
	groundY := leader.Origin.Y
	const flareAltitude = 10.0
	const maxDiveDuration = 1 * time.Second

	flared := false
	var preFlarePos, prePreFlarePos models.V3
	diveStart := time.Now()
	approachDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(approachDeadline) {
		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok)

		if !flared && (pos.Y-groundY <= flareAltitude || time.Since(diveStart) >= maxDiveDuration) {
			prePreFlarePos = preFlarePos
			preFlarePos = pos
			flared = true
			t.Logf("flaring at (%.2f, %.2f, %.2f) after %v of diving", pos.X, pos.Y, pos.Z, time.Since(diveStart))
		} else if !flared {
			prePreFlarePos = preFlarePos
			preFlarePos = pos
		}

		require.NoError(t, leader.Agent.SetManualRotation(0, divePitch))
		time.Sleep(steerInterval)

		if flared {
			break
		}
	}
	require.True(t, flared, "should have descended low enough to trigger the flare within the approach window")

	// The flare itself: hold nose-up until velocity confirms the descent
	// has actually been arrested and started to climb again - not for a
	// blind fixed duration. GlidingVelocity's "pitching up trades speed for
	// lift" term scales with current horizontal speed (physics/elytra.go),
	// so a fixed wall-clock pulse produces a wildly different amount of
	// lift depending on how fast the dive happened to be going when it
	// triggered: an earlier version of this test used a fixed 150ms pulse
	// and, watched back in replay, barely looked like a flare at all -
	// just an ordinary glide down. The term also has no velocity gate at
	// all (unlike the dive term, which only fires while already falling),
	// so holding a steep up-look continuously all the way to the ground (a
	// different earlier version's bug) compounds the lift impulse tick
	// after tick and rockets the agent hundreds of blocks back up instead
	// of landing - confirmed live, not assumed. Holding only until
	// vertical velocity crosses a small positive threshold scales the
	// pulse to whatever energy is really there, producing a visible arc
	// every time, and is still bounded: the instant pitch goes non-negative
	// below (settlePitch), the climb term stops firing entirely (it only
	// fires while pitch < 0) - so there's no risk of the earlier balloon
	// bug recurring, since that came from holding across the whole
	// remaining descent, not from crossing this threshold once and then
	// leveling off.
	const pulseFlarePitch = -60.0
	const flarePopVelocityThreshold = 0.8 // blocks/tick, upward
	const settlePitch = 10.0
	const maxFlareDuration = 1500 * time.Millisecond // safety net if velocity is ever unavailable

	preFlareVX, preFlareVY, preFlareVZ, preFlareVelOK := leader.Agent.GetVelocity()
	t.Logf("velocity at flare trigger: (%.3f, %.3f, %.3f) ok=%v", preFlareVX, preFlareVY, preFlareVZ, preFlareVelOK)

	// diveBottomY/flareApexY track the true low and high points of the
	// maneuver, not just the position at the moment the flare was decided:
	// the dive keeps sinking for a beat after that decision while the pop
	// pitch fights the residual downward velocity, confirmed live - the
	// real bottom came in noticeably lower than preFlarePos.Y (13.55 vs
	// 19.92 in one run), so comparing the eventual apex against
	// preFlarePos.Y understated (in one case, entirely erased) the climb
	// that actually happened.
	flareStart := time.Now()
	diveBottomY := preFlarePos.Y
	flareApexY := preFlarePos.Y
	for {
		require.NoError(t, leader.Agent.SetManualRotation(0, pulseFlarePitch))
		time.Sleep(steerInterval)

		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok)
		if pos.Y < diveBottomY {
			diveBottomY = pos.Y
		}
		if pos.Y > flareApexY {
			flareApexY = pos.Y
		}

		_, vy, _, velOK := leader.Agent.GetVelocity()
		if velOK && vy >= flarePopVelocityThreshold {
			t.Logf("flare pop detected: vy=%.3f after %v", vy, time.Since(flareStart))
			break
		}
		if time.Since(flareStart) >= maxFlareDuration {
			t.Logf("flare safety timeout hit after %v (last vy=%.3f, velOK=%v)", time.Since(flareStart), vy, velOK)
			break
		}
	}
	t.Logf("true dive bottom: %.2f (vs. flare-trigger position %.2f)", diveBottomY, preFlarePos.Y)

	// Coast to the true apex: crossing flarePopVelocityThreshold only
	// confirms upward velocity exists, not that the position has actually
	// risen yet (velocity and position are read at the start/end of the
	// same tick) - switching straight to a steep dive the instant the
	// threshold is crossed killed the climb before it could show up as a
	// position change at all, an earlier version of this test found live
	// (flareApexY came back identical to the flare-trigger position).
	// Leveling off (pitch=0, h≈1, ~25% gravity - physics/elytra.go)
	// instead lets the residual upward velocity actually carry the
	// position up for real over the next several ticks, matching the
	// "coast" phase already used between the climb and dive above for the
	// same momentum-isn't-instant reason.
	const apexCoastPitch = 0.0
	const maxApexCoastDuration = 1 * time.Second

	coastStart := time.Now()
	for time.Now().Before(coastStart.Add(maxApexCoastDuration)) {
		require.NoError(t, leader.Agent.SetManualRotation(0, apexCoastPitch))
		time.Sleep(steerInterval)

		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok)
		if pos.Y > flareApexY {
			flareApexY = pos.Y
		}

		_, vy, _, velOK := leader.Agent.GetVelocity()
		if velOK && vy <= 0 {
			t.Logf("true apex reached: vy=%.3f after %v of coasting", vy, time.Since(coastStart))
			break
		}
	}

	// The drop: GlidingVelocity's gravity and lift terms are both scaled
	// by h = cos²(pitch) (physics/elytra.go) - looking level (h≈1) cuts
	// effective gravity to ~25% and keeps converting sink into lift, which
	// is exactly why an earlier version of this test that leveled off to a
	// shallow pitch (10°) right after the pop just looked like an ordinary
	// glide down - h was still close to 1, so the same floaty cushioning
	// that makes gliding gentle in general kept right on cushioning it.
	// Looking steeply down instead (h≈0, confirmed from decompiled
	// LivingEntity.calcGlidingVelocity: at pitch=±90 the gravity blend
	// reduces to unmodified -gravity, i.e. genuine free fall) removes that
	// cushioning and produces a real, fast, visually obvious drop - still
	// nominally "gliding" (isGliding stays true; vanilla only exits
	// gliding on ground/vehicle/levitation/losing the elytra, confirmed
	// from PlayerEntity.canGlide() - there's no jump-based cancel), which
	// is what keeps limitFallDistance()'s fall-damage cap available for
	// the final settle below. dropPitch is positive (nose down), never
	// negative, specifically so the climb term (pitch<0) can't reactivate
	// and re-trigger the balloon bug.
	//
	// The breakout condition is velocity, not altitude: at h≈0 this phase
	// applies close to full, uncushioned gravity, so vy grows roughly
	// 0.1-0.3 blocks/tick faster each tick and blows past a fixed altitude
	// threshold in a single 100ms poll once it's diving fast - confirmed
	// live (targeting a 6-block breakout altitude actually broke out
	// around 4 blocks, already doing -1.7 blocks/tick, too fast for the
	// settle phase below to recover from before impact: settlePitch's
	// dive-term recovery is only ~10%/tick, so unwinding from -1.7 back
	// above the -0.5 fall-distance-reset threshold needs ~600ms and ~10+
	// blocks of altitude on its own, real damage resulted). Capping the
	// drop phase by vy instead keeps the speed at handoff predictable
	// regardless of how high the apex happened to be, so the settle
	// phase's margin requirement stays the same too.
	const dropPitch = 80.0
	const dropVelocityFloor = -1.0 // blocks/tick; still a fast, visible drop
	const dropSafetyAltitude = 8.0 // stop the steep drop no matter what below this

	dropDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(dropDeadline) {
		require.NoError(t, leader.Agent.SetManualRotation(0, dropPitch))
		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok)
		if pos.Y > flareApexY {
			flareApexY = pos.Y
		}
		_, vy, _, velOK := leader.Agent.GetVelocity()
		if pos.Y-groundY <= dropSafetyAltitude {
			break
		}
		if velOK && vy <= dropVelocityFloor {
			break
		}
		time.Sleep(steerInterval)
	}
	t.Logf("flare apex: %.2f (climbed %.2f blocks above the true dive bottom)", flareApexY, flareApexY-diveBottomY)

	// Final settle: level off with real altitude to spare so
	// limitFallDistance()'s vy>-0.5 condition has a few ticks to
	// re-engage (the drop phase above can build vertical speed well past
	// that) before touching down, same margin reasoning as flareAltitude
	// above.
	settleDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(settleDeadline) {
		require.NoError(t, leader.Agent.SetManualRotation(0, settlePitch))
		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok)
		if pos.Y-groundY <= 1.5 {
			break
		}
		time.Sleep(steerInterval)
	}

	// Give it a moment to actually settle onto the ground.
	time.Sleep(1 * time.Second)
	finalPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("final position: (%.2f, %.2f, %.2f)", finalPos.X, finalPos.Y, finalPos.Z)

	// The descent rate immediately before the flare (the fastest part of
	// the dive) versus immediately after starting it - the flare should
	// visibly slow the descent, not just coincidentally avoid damage.
	preFlareDescentRate := prePreFlarePos.Y - preFlarePos.Y
	t.Logf("descent rate in the tick(s) just before flaring: %.3f blocks/%v", preFlareDescentRate, steerInterval)

	endHealth, err := GetPlayerHealth(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get ending health")
	t.Logf("ending health: %.1f (started at %.1f)", endHealth, startHealth)

	assert.InDelta(t, startHealth, endHealth, 0.01, "flaring to land should avoid fall damage entirely")
	assert.Less(t, finalPos.Y, groundY+3.0, "should have actually come down to the ground, not gotten stuck hovering")

	// A real flare goes down, then up, then down again - not just a dive
	// that quietly cancels to zero. Require a real, visible climb between
	// the dive's low point and the flare's apex.
	assert.Greater(t, flareApexY-diveBottomY, 1.5, "flare should produce a visible upward arc before dropping, not just arrest the dive")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("item replace entity %s armor.chest with air", leader.Name))
}

// TestFlightNavigatesWaypointsAndLands verifies a full elytra flight end to
// end against a real server, starting from flat ground: a double-jump
// takeoff (see TestDoubleJumpFireworkRocketTakeoff for that sequence's own
// dedicated coverage) followed by a near-vertical climb, leveling off into
// a multi-leg course of waypoints steered purely by look direction (gliding
// ignores WASD throttle entirely — see physics/state.go's
// applyMovementInputs early-return while isGliding), re-firing a firework
// whenever the current one's boost lapses, and finally descending to land.
//
// Cruise steering only ever sets yaw from utils.GetYawAndPitch toward the
// current waypoint, deliberately ignoring its raw computed pitch: gliding's
// own formula (physics/elytra.go's GlidingVelocity) is sensitive to pitch in
// ways that make "look straight at a distant target" an unrealistic and
// unstable way to fly it in practice - real elytra flight is steered mostly
// by yaw at a shallow, controlled pitch, trading a little altitude for
// forward progress.
//
// The waypoint course is also cross-checked against a NavigationCourse
// (see PHASE_9_PLAN.md and testing/navigation_course.go): a second,
// server-authoritative witness that doesn't rely on the same client-side
// position tracking this test's own per-leg loop uses to decide "reached."
// Its radius is wider than the client-side loop's (25 vs. 15 blocks) to
// tolerate real elytra altitude drift over the course - the client-side
// check is deliberately horizontal-only (Y is "informational only," never
// steered toward), but the course's marker entities sit at a fixed Y, and
// vanilla's distance selector is full 3D. Equivalent to the pre-Phase-1
// TestElytraFlightNavigatesWaypointsAndLands.
func (s *ElytraFlatSuite) TestFlightNavigatesWaypointsAndLands() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ElytraNavBot", "elytra_flight_navigation")
	require.NoError(t, err, "spawn agent")

	// No platform, no teleport - takes off from natural ground spawn. Clear
	// a tall column for the vertical climb plus a wide swath for the
	// waypoint course that follows it.
	clearX := int(math.Floor(leader.Origin.X))
	clearZ := int(math.Floor(leader.Origin.Z))
	clearY := int(math.Floor(leader.Origin.Y))
	require.NoError(t, ClearArea(s.Ctx, s.Inst.RCON, clearX-60, clearY+1, clearZ-30, clearX+60, clearY+70, clearZ+330), "clear flight airspace")
	time.Sleep(300 * time.Millisecond)

	centerX := leader.Origin.X
	centerZ := leader.Origin.Z

	equipElytraViaShiftClick(t, s, leader)

	// Give plenty of firework rockets: one for the initial launch, several
	// more for re-firing during the climb, and spares for mid-cruise
	// re-boosts.
	giveFireworkRockets(t, s, leader, 20)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	// Face nearly straight up for a near-vertical launch (see
	// TestDoubleJumpFireworkRocketTakeoff's doc comment for why this,
	// rather than a level takeoff, is what actually gains altitude
	// quickly).
	const climbPitch = -80.0
	require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))

	startPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("flight start position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

	// Double-jump takeoff.
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.UseFireworkRocket(), "use firework rocket to launch the flight")

	// Climb near-vertically for a couple of seconds, re-firing as soon as
	// each boost lapses, to bank enough altitude for the whole waypoint
	// course before leveling off.
	const steerInterval = 100 * time.Millisecond
	climbDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(climbDeadline) {
		require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))
		ensureFireworkBoost(t, leader.Agent)
		time.Sleep(steerInterval)
	}

	cruiseStart, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("cruise altitude after climb: (%.2f, %.2f, %.2f)", cruiseStart.X, cruiseStart.Y, cruiseStart.Z)

	// Level off - pitching from steep-up back to level converts a lot of
	// the climb's vertical speed into forward speed via GlidingVelocity's
	// own dive term, giving the cruise a strong head start.
	require.NoError(t, leader.Agent.SetManualRotation(0, 0))
	time.Sleep(400 * time.Millisecond)

	// A course of waypoints continuing generally in the takeoff heading
	// (+Z, yaw 0) with gentle lateral drift rather than sharp turns - real
	// elytra flight steers gradually. Y values are informational only
	// (logged, not steered toward): cruise pitch is held level (see
	// below).
	cruiseY := cruiseStart.Y
	type waypoint struct{ x, y, z float64 }
	waypoints := []waypoint{
		{centerX + 15, cruiseY, centerZ + 70},
		{centerX + 35, cruiseY, centerZ + 140},
		{centerX + 20, cruiseY, centerZ + 210},
		{centerX - 15, cruiseY, centerZ + 260},
		{centerX - 35, cruiseY, centerZ + 300},
	}

	const waypointRadius = 15.0
	const perLegTimeout = 8 * time.Second
	const cruisePitch = 0.0

	courseWaypoints := make([]CourseWaypoint, len(waypoints))
	for i, wp := range waypoints {
		courseWaypoints[i] = CourseWaypoint{Pos: models.V3{X: wp.x, Y: wp.y, Z: wp.z}}
	}
	const navCourseRadius = 25.0
	navCourse, err := SetupNavigationCourse(s.Ctx, s.Inst.RCON, leader.Name, navCourseRadius, courseWaypoints)
	require.NoError(t, err, "set up navigation course")
	defer func() { _ = navCourse.Cleanup(context.Background(), s.Inst.RCON) }()

	for i, wp := range waypoints {
		legStart := time.Now()
		reached := false
		for time.Since(legStart) < perLegTimeout {
			pos, ok := leader.Agent.GetPositionSimple()
			require.True(t, ok)

			dx := wp.x - pos.X
			dz := wp.z - pos.Z
			horizontalDist := math.Sqrt(dx*dx + dz*dz)
			if horizontalDist <= waypointRadius {
				reached = true
				break
			}

			yaw, _ := utils.GetYawAndPitch(pos, models.V3{X: wp.x, Y: wp.y, Z: wp.z})
			require.NoError(t, leader.Agent.SetManualRotation(yaw, cruisePitch))
			ensureFireworkBoost(t, leader.Agent)
			time.Sleep(steerInterval)
		}
		pos, _ := leader.Agent.GetPositionSimple()
		t.Logf("waypoint %d target=(%.1f,%.1f,%.1f) reached=%v final pos=(%.2f,%.2f,%.2f)",
			i, wp.x, wp.y, wp.z, reached, pos.X, pos.Y, pos.Z)
		assert.True(t, reached, "should reach waypoint %d within %v", i, perLegTimeout)
	}

	navResults, err := navCourse.Results(s.Ctx, s.Inst.RCON)
	require.NoError(t, err, "query navigation course results")
	require.Len(t, navResults, len(waypoints))
	var lastTriggeredTime int64
	for i, res := range navResults {
		t.Logf("navigation course waypoint %d: TriggeredTime=%d", i, res.TriggeredTime)
		assert.Greater(t, res.TriggeredTime, int64(0), "server should independently confirm waypoint %d was reached", i)
		assert.GreaterOrEqual(t, res.TriggeredTime, lastTriggeredTime, "server should confirm waypoint %d wasn't reached before the previous one", i)
		lastTriggeredTime = res.TriggeredTime
	}

	// Land: pitch down and let the descent bring it down; keep steering
	// back toward the original takeoff spot so it doesn't drift away while
	// descending.
	landDeadline := time.Now().Add(15 * time.Second)
	var lastY, stableSince float64
	landed := false
	for time.Now().Before(landDeadline) {
		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok)

		yaw, _ := utils.GetYawAndPitch(pos, models.V3{X: centerX, Y: pos.Y, Z: centerZ})
		require.NoError(t, leader.Agent.SetManualRotation(yaw, 15))

		if math.Abs(pos.Y-lastY) < 0.02 {
			if stableSince == 0 {
				stableSince = float64(time.Now().UnixMilli())
			} else if float64(time.Now().UnixMilli())-stableSince > 500 {
				landed = true
				break
			}
		} else {
			stableSince = 0
		}
		lastY = pos.Y
		time.Sleep(steerInterval)
	}

	finalPos, _ := leader.Agent.GetPositionSimple()
	t.Logf("final position: (%.2f, %.2f, %.2f), landed=%v", finalPos.X, finalPos.Y, finalPos.Z, landed)
	assert.True(t, landed, "should settle to a stable altitude (landed) within the descent window")

	_, _ = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("item replace entity %s armor.chest with air", leader.Name))
}

// TestUnequippingStopsGliding verifies that removing the elytra mid-flight
// ends gliding immediately, matching vanilla's canGlide() gate
// (physics/elytra.go's CanGlide requires elytraEquipped every tick, not
// just at glide-start) - unlike landing, entering water, or gaining
// Levitation, this is a piece of behavior a player can trigger deliberately
// at any altitude, so it deserves its own direct check rather than relying
// on the other tests only ever un-equipping after landing.
//
// Removing the elytra via a real shift-click on the armor slot while
// airborne (the mirror image of the shift-click every elytra test already
// uses to equip it, not RCON's `item replace` - see the mid-flight
// shift-click's own comment below for why) exercises the exact server
// round-trip this depends on: the agent has no inventory access of its own
// inside physics.State, so CanGlide only sees the equipment change once
// movement/physics_executor.go's syncEquipment reads it back from the
// ClientboundSetSlot the server sends in response to the click - this test
// also confirms that path doesn't lag long enough to matter, not just that
// the flag flips in principle. Equivalent to the pre-Phase-1
// TestElytraUnequippingStopsGliding.
func (s *ElytraFlatSuite) TestUnequippingStopsGliding() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ElytraUnequipBot", "elytra_unequip_stops_gliding")
	require.NoError(t, err, "spawn agent")

	clearX := int(math.Floor(leader.Origin.X))
	clearZ := int(math.Floor(leader.Origin.Z))
	clearY := int(math.Floor(leader.Origin.Y))
	require.NoError(t, ClearArea(s.Ctx, s.Inst.RCON, clearX-20, clearY+1, clearZ-20, clearX+20, clearY+40, clearZ+20), "clear airspace")
	time.Sleep(300 * time.Millisecond)

	equipElytraViaShiftClick(t, s, leader)
	giveFireworkRockets(t, s, leader, 5)

	require.NoError(t, leader.Agent.EnterManualMode(), "enter manual movement mode")
	defer func() { _ = leader.Agent.ExitManualMode() }()

	const climbPitch = -80.0
	const steerInterval = 100 * time.Millisecond
	require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))

	// Double-jump takeoff (see TestDoubleJumpFireworkRocketTakeoff).
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.SetManualJump(false))
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, leader.Agent.UseFireworkRocket(), "use firework rocket to launch")

	// A brief climb - just enough altitude to have real room to fall
	// afterward, not a sustained ascent (this test isn't measuring altitude
	// gain).
	const climbDuration = 800 * time.Millisecond
	climbDeadline := time.Now().Add(climbDuration)
	for time.Now().Before(climbDeadline) {
		require.NoError(t, leader.Agent.SetManualRotation(0, climbPitch))
		time.Sleep(steerInterval)
	}

	require.True(t, leader.Agent.IsGliding(), "should be gliding after a double-jump takeoff with the elytra equipped")

	apexPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("altitude before unequipping: (%.2f, %.2f, %.2f)", apexPos.X, apexPos.Y, apexPos.Z)

	// Level off before unequipping so the fall that follows is a clean,
	// mostly-vertical drop rather than tangled up with whatever horizontal
	// speed a continued dive would have built.
	require.NoError(t, leader.Agent.SetManualRotation(0, 0))
	time.Sleep(300 * time.Millisecond)

	// Remove the elytra mid-flight via a real shift-click on the armor
	// slot itself - the mirror image of the shift-click used to equip it
	// above, and not RCON's `item replace`: that command changes the
	// entity's equipment server-side but, per
	// TestGlideSlowsDescentAndAddsForwardMotion's doc comment, isn't
	// reliably broadcast to every observer already tracking the entity
	// (confirmed there via a real replay recording where a companion agent
	// never saw the elytra equipped despite the bot gliding correctly). A
	// shift-click goes through the same server-authoritative
	// container-click path a real player's unequip action would.
	screenMgr := leader.ScreenManager()
	armorSlot := screenMgr.Inventory().GetSlots()[6]
	require.NoError(t, leader.Agent.ShiftClickSlot(6, slotToItemStack(armorSlot)), "shift-click elytra out of the armor slot mid-flight")
	require.True(t, waitForSlotState(screenMgr, 6, func(sl screen.Slot) bool {
		return sl.Count == 0
	}, 10*time.Second), "chest armor slot never cleared after unequipping")

	// Verify against real server-side NBT inventory data (not just the
	// client's own predicted screen state, which only proves what the
	// client predicted, not what the server actually did) that the elytra
	// genuinely moved into the main inventory rather than being duplicated
	// or lost. waitForSlotState above only confirms local prediction,
	// which fires synchronously the instant ShiftClickSlot is called -
	// well before the real server-side click processing and its
	// RCON-visible NBT update necessarily complete, so this polls rather
	// than checking once (confirmed live: a single immediate check raced
	// ahead of the server and saw neither the armor slot nor the inventory
	// holding the elytra for a brief window).
	elytraCount := 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		invItems, invErr := GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name)
		require.NoError(t, invErr, "get real inventory state via RCON")
		elytraCount = 0
		for _, it := range invItems {
			if it.ID == "minecraft:elytra" {
				elytraCount++
			}
		}
		if elytraCount == 1 {
			break
		}
		time.Sleep(steerInterval)
	}
	assert.Equal(t, 1, elytraCount, "should be exactly one real elytra in inventory after unequipping, not duplicated or lost")

	// CanGlide is re-checked every tick (physics/state.go's
	// applyGlideStateTransition), so this should clear within a tick or two
	// of syncEquipment picking up the change - not waiting for landing,
	// water, or any other unrelated condition.
	glideStoppedDeadline := time.Now().Add(2 * time.Second)
	stoppedGliding := false
	for time.Now().Before(glideStoppedDeadline) {
		if !leader.Agent.IsGliding() {
			stoppedGliding = true
			break
		}
		time.Sleep(steerInterval)
	}
	assert.True(t, stoppedGliding, "gliding should stop within 2 seconds of unequipping the elytra mid-flight")

	// Let it fall the rest of the way and confirm it actually lands under
	// normal (non-glide) physics rather than getting stuck.
	landDeadline := time.Now().Add(15 * time.Second)
	var lastY, stableSince float64
	landed := false
	for time.Now().Before(landDeadline) {
		pos, ok := leader.Agent.GetPositionSimple()
		require.True(t, ok)

		if math.Abs(pos.Y-lastY) < 0.02 {
			if stableSince == 0 {
				stableSince = float64(time.Now().UnixMilli())
			} else if float64(time.Now().UnixMilli())-stableSince > 500 {
				landed = true
				break
			}
		} else {
			stableSince = 0
		}
		lastY = pos.Y
		time.Sleep(steerInterval)
	}

	finalPos, _ := leader.Agent.GetPositionSimple()
	t.Logf("final position: (%.2f, %.2f, %.2f), landed=%v", finalPos.X, finalPos.Y, finalPos.Z, landed)
	assert.True(t, landed, "should fall and settle on the ground within the descent window after unequipping")
	assert.False(t, leader.Agent.IsGliding(), "should still not be gliding after landing")
}
