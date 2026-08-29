package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sayCommand sends a real chat message addressed to the bot via RCON's
// `/say`, exercising the actual chat pipeline (agent.OnDisguisedChat ->
// handleChatCommand -> the real command registry) rather than calling the
// underlying agent method directly - this is what proves the commands are
// actually *wired*, not just that the underlying capability works.
func sayCommand(t *testing.T, ctx context.Context, env *StandaloneTestEnv, botName, command string) {
	t.Helper()
	_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("say >>>%s<<< %s", botName, command))
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
func sayCommandUntil(t *testing.T, ctx context.Context, env *StandaloneTestEnv, botName, command string, condition func() bool) bool {
	t.Helper()
	const settleDelay = 700 * time.Millisecond
	for attempt := 0; attempt < 4; attempt++ {
		sayCommand(t, ctx, env, botName, command)
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

// TestChatCommand_EquipAndUseItem verifies the "equip" and "useItem" chat
// commands end to end against a real server: "equip elytra" wears it (via
// a real shift-click - see TestElytraGlideSlowsDescentAndAddsForwardMotion's
// doc comment for why not RCON), "equip firework_rocket" selects it into
// the hand (the "select rockets" step), and "useItem" - sent as a real
// chat message, not called directly - fires the currently held firework
// while gliding and produces a real, measurable velocity boost.
func TestChatCommand_EquipAndUseItem(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "chat_equip_useitem", "survival", false, tt.MCVersion, DifficultyEasy, true)
			defer env.Cancel()

			ctx := context.Background()

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")
			clearX := int(math.Floor(botPos.X))
			clearZ := int(math.Floor(botPos.Z))
			clearY := int(math.Floor(botPos.Y))
			require.NoError(t, ClearArea(ctx, env.Inst.RCON, clearX-15, clearY+1, clearZ-15, clearX+15, clearY+40, clearZ+15), "clear airspace")
			time.Sleep(300 * time.Millisecond)

			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:elytra 1", env.BotName))
			require.NoError(t, err, "give elytra")
			_, _, ok = waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "elytra never appeared in inventory")

			equipped := sayCommandUntil(t, ctx, env, env.BotName, "equip elytra", func() bool {
				return waitForSlotState(env.ScreenMgr, 6, func(s screen.Slot) bool { return s.Count > 0 }, 500*time.Millisecond)
			})
			require.True(t, equipped, "'equip elytra' chat command never equipped it")
			t.Log("'equip elytra' chat command equipped the elytra")

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:firework_rocket 5", env.BotName))
			require.NoError(t, err, "give firework rockets")
			_, _, ok = waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "firework rockets never appeared in inventory")

			selected := sayCommandUntil(t, ctx, env, env.BotName, "equip firework_rocket", func() bool {
				return waitForSlotState(env.ScreenMgr, 36, func(s screen.Slot) bool { return s.Count > 0 }, 500*time.Millisecond)
			})
			require.True(t, selected, "'equip firework_rocket' chat command never selected it into hand")
			t.Log("'equip firework_rocket' chat command selected it into hand")

			// Get airborne and gliding directly (not via chat - there's no
			// "jump" command), so there's a real elytra flight in progress
			// for "useItem" to boost. Pitch steeply up (matching the other
			// elytra tests' takeoff, not a level 0 pitch) so the glide has
			// real airborne time to work with: an earlier version of this
			// test double-jumped level, which barely leaves the ground and
			// lands again in well under a second (confirmed live via
			// position/IsGliding polling) - not enough time for a real
			// server round trip plus the "useItem" chat command's own
			// processing to land before it's too late to observe a boost.
			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()
			const climbPitch = -80.0
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(150 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.True(t, env.Agent.Agent.IsGliding(), "double-jump should have started gliding")

			_, preVY, _, preOK := env.Agent.Agent.GetVelocity()
			require.True(t, preOK)
			t.Logf("velocity before 'useItem': vy=%.3f", preVY)

			boosted := sayCommandUntil(t, ctx, env, env.BotName, "useItem", func() bool {
				return env.Agent.Agent.HasActiveFireworkBoost()
			})
			assert.True(t, boosted, "'useItem' chat command should have used the firework rocket and started a boost")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("item replace entity %s armor.chest with air", env.BotName))
		})
	}
}

// TestChatCommand_FlyTo verifies the "flyTo" chat command end to end
// against a real server: from natural ground spawn (no platform/teleport,
// matching the other elytra tests), a single chat message takes off,
// cruises to a distant target, and lands - reusing the takeoff/cruise/land
// sequence validated by testing/elytra_test.go and
// testing/elytra_navigation_test.go, now driven entirely through the real
// chat command pipeline rather than direct Go calls.
func TestChatCommand_FlyTo(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "chat_flyto", "survival", false, tt.MCVersion, DifficultyEasy, true)
			defer env.Cancel()

			ctx := context.Background()

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")
			clearX := int(math.Floor(botPos.X))
			clearZ := int(math.Floor(botPos.Z))
			clearY := int(math.Floor(botPos.Y))
			require.NoError(t, ClearArea(ctx, env.Inst.RCON, clearX-40, clearY+1, clearZ-20, clearX+40, clearY+60, clearZ+220), "clear flight airspace")
			time.Sleep(300 * time.Millisecond)

			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:elytra 1", env.BotName))
			require.NoError(t, err, "give elytra")
			_, _, ok = waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "elytra never appeared in inventory")

			sayCommand(t, ctx, env, env.BotName, "equip elytra")
			require.True(t, waitForSlotState(env.ScreenMgr, 6, func(s screen.Slot) bool {
				return s.Count > 0
			}, 10*time.Second), "'equip elytra' chat command never equipped it")

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:firework_rocket 20", env.BotName))
			require.NoError(t, err, "give firework rockets")
			_, _, ok = waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "firework rockets never appeared in inventory")

			targetX := botPos.X
			targetY := botPos.Y
			targetZ := botPos.Z + 200

			sayCommand(t, ctx, env, env.BotName, fmt.Sprintf("flyTo %.0f %.0f %.0f", targetX, targetY, targetZ))

			var finalPos models.V3
			arrived := false
			deadline := time.Now().Add(60 * time.Second)
			for time.Now().Before(deadline) {
				pos, ok := env.Agent.Agent.GetPositionSimple()
				require.True(t, ok)
				finalPos = pos
				dx, dz := targetX-pos.X, targetZ-pos.Z
				if math.Sqrt(dx*dx+dz*dz) <= 15.0 && pos.Y <= 3.0 {
					arrived = true
					break
				}
				time.Sleep(200 * time.Millisecond)
			}

			t.Logf("final position: (%.2f, %.2f, %.2f), target: (%.0f, %.0f, %.0f)", finalPos.X, finalPos.Y, finalPos.Z, targetX, targetY, targetZ)
			assert.True(t, arrived, "'flyTo' chat command should have flown near the target and landed within the timeout")

			_ = env.Agent.Agent.ExitManualMode()
			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("item replace entity %s armor.chest with air", env.BotName))
		})
	}
}
