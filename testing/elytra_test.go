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

// TestElytraGlideSlowsDescentAndAddsForwardMotion verifies that jumping
// while airborne with an elytra equipped triggers real elytra-gliding
// physics end-to-end against a real server: much slower descent than a
// plain fall, plus real forward motion in the look direction (which plain
// falling never produces, since there's no WASD thrust in the air) — see
// physics/elytra.go's GlidingVelocity and physics/state_elytra_test.go for
// the formula-level and state-level coverage this builds on.
//
// The elytra is equipped via a real client-driven shift-click (give it to
// the bot's inventory, then let the agent equip it itself), not RCON's
// `item replace entity ... armor.chest with ...`. That command does change
// the entity's equipment server-side (confirmed via its own RCON response),
// but observers who see the resulting ClientboundSetEquipment broadcast
// don't reliably include ones only just then starting to track the entity —
// verified against a real replay recording, where a companion observer
// agent showed the bot never wearing the elytra despite gliding correctly.
// A normal shift-click goes through the same server-authoritative
// container-click path a real player's equip action would, which is
// reliably broadcast to every current tracker.
func TestElytraGlideSlowsDescentAndAddsForwardMotion(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "elytra_glide", "survival", false, tt.MCVersion, DifficultyEasy, true)
			defer env.Cancel()

			ctx := context.Background()

			// Give the elytra to the bot's own inventory (no equipment
			// change yet - just an item, same as /give), then let the agent
			// equip it itself via a real shift-click so the resulting
			// equipment change is a normal, reliably-broadcast client
			// action rather than a server-side RCON mutation.
			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:elytra 1", env.BotName))
			require.NoError(t, err, "give elytra")

			fromSlot, fromSlotData, ok := waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "elytra never appeared in bot's main/hotbar inventory")
			t.Logf("elytra given in slot %d", fromSlot)

			require.NoError(t, env.Agent.Agent.ShiftClickSlot(int16(fromSlot), slotToItemStack(fromSlotData)), "shift-click elytra to equip it")

			// Player inventory chest armor slot is index 6.
			require.True(t, waitForSlotState(env.ScreenMgr, 6, func(s screen.Slot) bool {
				return s.Count > 0
			}, 10*time.Second), "elytra never landed in the chest armor slot")
			t.Log("elytra equipped via shift-click")

			// Teleport well clear of the ground, facing south (yaw 0) and
			// looking level (pitch 0), so gliding forward motion accumulates
			// in a known (+Z) direction.
			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("teleport %s 100 100 100 0 0", env.BotName))
			require.NoError(t, err, "teleport agent into open air")
			time.Sleep(300 * time.Millisecond)

			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, 0), "face south, level")

			startPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")

			// Jump while airborne to trigger the client-predicted
			// checkGliding() transition (see physics/state.go's
			// applyGlideStateTransition) - held briefly, not just one tick,
			// to absorb scheduling jitter between this goroutine and the
			// physics tick loop.
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(200 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))

			time.Sleep(3 * time.Second)

			endPos, _ := env.Agent.Agent.GetPositionSimple()
			drop := startPos.Y - endPos.Y
			forwardDist := endPos.Z - startPos.Z
			t.Logf("start=(%.2f,%.2f,%.2f) end=(%.2f,%.2f,%.2f) drop=%.3f forwardDist=%.3f",
				startPos.X, startPos.Y, startPos.Z, endPos.X, endPos.Y, endPos.Z, drop, forwardDist)

			// A 3-second plain fall from rest covers roughly 60 blocks under
			// this codebase's gravity/drag constants (steady state well
			// above 15 blocks/sec); gliding at level pitch should cover a
			// small fraction of that.
			assert.Less(t, drop, 20.0, "elytra gliding at level pitch should descend far slower than an ordinary fall")
			assert.Greater(t, forwardDist, 1.0, "gliding while facing south should accumulate real forward (+Z) motion, unlike a plain fall")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("item replace entity %s armor.chest with air", env.BotName))
		})
	}
}

// TestElytraDoubleJumpFireworkRocketTakeoff verifies the real "double jump"
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
// real players launch almost straight up.
func TestElytraDoubleJumpFireworkRocketTakeoff(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "elytra_firework_takeoff", "survival", false, tt.MCVersion, DifficultyEasy, true)
			defer env.Cancel()

			ctx := context.Background()

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")

			// Clear a tall column above spawn for the vertical launch - no
			// platform, no teleport, this starts right from natural ground
			// spawn.
			clearX := int(math.Floor(botPos.X))
			clearZ := int(math.Floor(botPos.Z))
			clearY := int(math.Floor(botPos.Y))
			require.NoError(t, ClearArea(ctx, env.Inst.RCON, clearX-15, clearY+1, clearZ-15, clearX+15, clearY+80, clearZ+15), "clear vertical launch airspace")
			time.Sleep(300 * time.Millisecond)

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

			// Give several firework rockets - with the elytra now moved out
			// of the hotbar into the chest slot, these land back in the
			// already-selected hotbar slot (0), ready to use immediately,
			// and there's enough to re-fire a few times as each one's boost
			// runs out.
			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:firework_rocket 5", env.BotName))
			require.NoError(t, err, "give firework rockets")
			_, _, ok = waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "firework rockets never appeared in bot's main/hotbar inventory")

			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()

			// Face nearly straight up. Not exactly -90: GlidingVelocity's
			// own horizontal-direction terms divide by the look vector's
			// horizontal magnitude, which is only exactly zero at a true
			// -90 - staying just short of it keeps those terms numerically
			// well-behaved (they remain well-defined near-zero, but there's
			// no reason to court the exact edge) while still being "almost
			// straight up" the way real players fly it.
			const climbPitch = -80.0
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))

			startPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")
			t.Logf("takeoff start position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

			// The real double-jump takeoff: a normal ground jump (single
			// press), a genuine release, then a fresh press while airborne.
			// JumpVelocity is small (the whole hop lasts under half a
			// second), so this must all happen well within that window.
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(150 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))

			// Fire the rocket immediately - real players don't glide first,
			// since every tick spent gliding-only from a cold, near-zero
			// speed start burns altitude for very little gain. A brief
			// settle (one or two physics ticks) just lets the glide-start
			// transition actually land before the use-item packet goes out.
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.UseFireworkRocket(), "use firework rocket to launch")

			// Climb for a few seconds, re-firing as soon as each boost
			// lapses (a default firework's boost lasts under a second) so
			// the climb doesn't stall out on gravity alone between them.
			climbDeadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(climbDeadline) {
				require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))
				if !env.Agent.Agent.HasActiveFireworkBoost() {
					_ = env.Agent.Agent.UseFireworkRocket()
				}
				time.Sleep(150 * time.Millisecond)
			}

			endPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok)

			climb := endPos.Y - startPos.Y
			t.Logf("after double-jump + repeated near-vertical firework boosts: start=(%.2f,%.2f,%.2f) end=(%.2f,%.2f,%.2f) climb=%.3f",
				startPos.X, startPos.Y, startPos.Z, endPos.X, endPos.Y, endPos.Z, climb)

			// A single boost alone reaches a steady-state ~1.7 blocks/tick
			// (physics/elytra_test.go's TestFireworkBoostVelocity); sustained
			// near-vertically for 3 seconds with re-firing, real altitude
			// gain should be large and unambiguous - a plain, unboosted
			// double-jump gains only a fraction of a block.
			assert.Greater(t, climb, 20.0, "repeated near-vertical firework boosts after a double-jump should gain a large amount of altitude")

			_, _ = env.Inst.RCON.Exec(ctx, fmt.Sprintf("item replace entity %s armor.chest with air", env.BotName))
		})
	}
}
