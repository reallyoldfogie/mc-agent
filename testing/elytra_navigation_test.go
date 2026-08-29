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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestElytraFlightNavigatesWaypointsAndLands verifies a full elytra flight
// end to end against a real server, starting from flat ground: a
// double-jump takeoff (see TestElytraDoubleJumpFireworkRocketTakeoff for
// that sequence's own dedicated coverage) followed by a near-vertical
// climb, leveling off into a multi-leg course of waypoints steered purely
// by look direction (gliding ignores WASD throttle entirely — see
// physics/state.go's applyMovementInputs early-return while isGliding),
// re-firing a firework whenever the current one's boost lapses, and finally
// descending to land.
//
// Cruise steering only ever sets yaw from utils.GetYawAndPitch toward the
// current waypoint, deliberately ignoring its raw computed pitch: gliding's
// own formula (physics/elytra.go's GlidingVelocity) is sensitive to pitch in
// ways that make "look straight at a distant target" an unrealistic and
// unstable way to fly it in practice - real elytra flight is steered mostly
// by yaw at a shallow, controlled pitch, trading a little altitude for
// forward progress.
func TestElytraFlightNavigatesWaypointsAndLands(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "elytra_flight_navigation", "survival", false, tt.MCVersion, DifficultyEasy, true)
			defer env.Cancel()

			ctx := context.Background()

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")

			// No platform, no teleport - takes off from natural ground
			// spawn. Clear a tall column for the vertical climb plus a wide
			// swath for the waypoint course that follows it.
			clearX := int(math.Floor(botPos.X))
			clearZ := int(math.Floor(botPos.Z))
			clearY := int(math.Floor(botPos.Y))
			require.NoError(t, ClearArea(ctx, env.Inst.RCON, clearX-60, clearY+1, clearZ-30, clearX+60, clearY+70, clearZ+330), "clear flight airspace")
			time.Sleep(300 * time.Millisecond)

			centerX := botPos.X
			centerZ := botPos.Z

			// Equip the elytra via a real shift-click (see
			// TestElytraGlideSlowsDescentAndAddsForwardMotion's doc comment
			// for why not RCON).
			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:elytra 1", env.BotName))
			require.NoError(t, err, "give elytra")

			elytraSlot, elytraSlotData, ok := waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "elytra never appeared in bot's main/hotbar inventory")

			require.NoError(t, env.Agent.Agent.ShiftClickSlot(int16(elytraSlot), slotToItemStack(elytraSlotData)), "shift-click elytra to equip it")
			require.True(t, waitForSlotState(env.ScreenMgr, 6, func(s screen.Slot) bool {
				return s.Count > 0
			}, 10*time.Second), "elytra never landed in the chest armor slot")
			t.Log("elytra equipped via shift-click")

			// Give plenty of firework rockets: one for the initial launch,
			// several more for re-firing during the climb, and spares for
			// mid-cruise re-boosts.
			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:firework_rocket 20", env.BotName))
			require.NoError(t, err, "give firework rockets")
			_, _, ok = waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "firework rockets never appeared in bot's main/hotbar inventory")

			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()

			// Face nearly straight up for a near-vertical launch (see
			// TestElytraDoubleJumpFireworkRocketTakeoff's doc comment for
			// why this, rather than a level takeoff, is what actually gains
			// altitude quickly).
			const climbPitch = -80.0
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))

			startPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok)
			t.Logf("flight start position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

			// Double-jump takeoff.
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(150 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.UseFireworkRocket(), "use firework rocket to launch the flight")

			// Climb near-vertically for a couple of seconds, re-firing as
			// soon as each boost lapses, to bank enough altitude for the
			// whole waypoint course before leveling off.
			const steerInterval = 100 * time.Millisecond
			climbDeadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(climbDeadline) {
				require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))
				ensureFireworkBoost(t, env.Agent.Agent)
				time.Sleep(steerInterval)
			}

			cruiseStart, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok)
			t.Logf("cruise altitude after climb: (%.2f, %.2f, %.2f)", cruiseStart.X, cruiseStart.Y, cruiseStart.Z)

			// Level off - pitching from steep-up back to level converts a
			// lot of the climb's vertical speed into forward speed via
			// GlidingVelocity's own dive term, giving the cruise a strong
			// head start.
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, 0))
			time.Sleep(400 * time.Millisecond)

			// A course of waypoints continuing generally in the takeoff
			// heading (+Z, yaw 0) with gentle lateral drift rather than
			// sharp turns - real elytra flight steers gradually. Y values
			// are informational only (logged, not steered toward): cruise
			// pitch is held level (see below).
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

			for i, wp := range waypoints {
				legStart := time.Now()
				reached := false
				for time.Since(legStart) < perLegTimeout {
					pos, ok := env.Agent.Agent.GetPositionSimple()
					require.True(t, ok)

					dx := wp.x - pos.X
					dz := wp.z - pos.Z
					horizontalDist := math.Sqrt(dx*dx + dz*dz)
					if horizontalDist <= waypointRadius {
						reached = true
						break
					}

					yaw, _ := utils.GetYawAndPitch(pos, models.V3{X: wp.x, Y: wp.y, Z: wp.z})
					require.NoError(t, env.Agent.Agent.SetManualRotation(yaw, cruisePitch))
					ensureFireworkBoost(t, env.Agent.Agent)
					time.Sleep(steerInterval)
				}
				pos, _ := env.Agent.Agent.GetPositionSimple()
				t.Logf("waypoint %d target=(%.1f,%.1f,%.1f) reached=%v final pos=(%.2f,%.2f,%.2f)",
					i, wp.x, wp.y, wp.z, reached, pos.X, pos.Y, pos.Z)
				assert.True(t, reached, "should reach waypoint %d within %v", i, perLegTimeout)
			}

			// Land: pitch down and let the descent bring it down; keep
			// steering back toward the original takeoff spot so it doesn't
			// drift away while descending.
			landDeadline := time.Now().Add(15 * time.Second)
			var lastY, stableSince float64
			landed := false
			for time.Now().Before(landDeadline) {
				pos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok)

				yaw, _ := utils.GetYawAndPitch(pos, models.V3{X: centerX, Y: pos.Y, Z: centerZ})
				require.NoError(t, env.Agent.Agent.SetManualRotation(yaw, 15))

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

			finalPos, _ := env.Agent.Agent.GetPositionSimple()
			t.Logf("final position: (%.2f, %.2f, %.2f), landed=%v", finalPos.X, finalPos.Y, finalPos.Z, landed)
			assert.True(t, landed, "should settle to a stable altitude (landed) within the descent window")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("item replace entity %s armor.chest with air", env.BotName))
		})
	}
}
