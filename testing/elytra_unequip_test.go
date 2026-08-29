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

// TestElytraUnequippingStopsGliding verifies that removing the elytra
// mid-flight ends gliding immediately, matching vanilla's canGlide() gate
// (physics/elytra.go's CanGlide requires elytraEquipped every tick, not
// just at glide-start) - unlike landing, entering water, or gaining
// Levitation, this is a piece of behavior a player can trigger deliberately
// at any altitude, so it deserves its own direct check rather than relying
// on the other tests only ever un-equipping after landing.
//
// Removing the elytra via a real shift-click on the armor slot while
// airborne (the mirror image of the shift-click every elytra test already
// uses to equip it, not RCON's `item replace` - see the mid-flight
// shift-click's own comment below for why) exercises the exact
// server round-trip this depends on: the agent has no inventory access of
// its own inside physics.State, so CanGlide only sees the equipment change
// once movement/physics_executor.go's syncEquipment reads it back from the
// ClientboundSetSlot the server sends in response to the click - this test
// also confirms that path doesn't lag long enough to matter, not just that
// the flag flips in principle.
func TestElytraUnequippingStopsGliding(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "elytra_unequip_stops_gliding", "survival", false, tt.MCVersion, DifficultyEasy, true)
			defer env.Cancel()

			ctx := context.Background()

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")

			clearX := int(math.Floor(botPos.X))
			clearZ := int(math.Floor(botPos.Z))
			clearY := int(math.Floor(botPos.Y))
			require.NoError(t, ClearArea(ctx, env.Inst.RCON, clearX-20, clearY+1, clearZ-20, clearX+20, clearY+40, clearZ+20), "clear airspace")
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

			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("give %s minecraft:firework_rocket 5", env.BotName))
			require.NoError(t, err, "give firework rockets")
			_, _, ok = waitForInventorySlot(env.ScreenMgr, func(index int, s screen.Slot) bool {
				return index >= 9 && index <= 44 && s.Count > 0
			}, 10*time.Second)
			require.True(t, ok, "firework rockets never appeared in bot's main/hotbar inventory")

			require.NoError(t, env.Agent.Agent.EnterManualMode(), "enter manual movement mode")
			defer func() { _ = env.Agent.Agent.ExitManualMode() }()

			const climbPitch = -80.0
			const steerInterval = 100 * time.Millisecond
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))

			// Double-jump takeoff (see TestElytraDoubleJumpFireworkRocketTakeoff).
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(150 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(true))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.SetManualJump(false))
			time.Sleep(100 * time.Millisecond)
			require.NoError(t, env.Agent.Agent.UseFireworkRocket(), "use firework rocket to launch")

			// A brief climb - just enough altitude to have real room to fall
			// afterward, not a sustained ascent (this test isn't measuring
			// altitude gain).
			const climbDuration = 800 * time.Millisecond
			climbDeadline := time.Now().Add(climbDuration)
			for time.Now().Before(climbDeadline) {
				require.NoError(t, env.Agent.Agent.SetManualRotation(0, climbPitch))
				time.Sleep(steerInterval)
			}

			require.True(t, env.Agent.Agent.IsGliding(), "should be gliding after a double-jump takeoff with the elytra equipped")

			apexPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok)
			t.Logf("altitude before unequipping: (%.2f, %.2f, %.2f)", apexPos.X, apexPos.Y, apexPos.Z)

			// Level off before unequipping so the fall that follows is a
			// clean, mostly-vertical drop rather than tangled up with
			// whatever horizontal speed a continued dive would have built.
			require.NoError(t, env.Agent.Agent.SetManualRotation(0, 0))
			time.Sleep(300 * time.Millisecond)

			// Remove the elytra mid-flight via a real shift-click on the
			// armor slot itself - the mirror image of the shift-click used
			// to equip it above, and not RCON's `item replace`: that
			// command changes the entity's equipment server-side but, per
			// TestElytraGlideSlowsDescentAndAddsForwardMotion's doc comment,
			// isn't reliably broadcast to every observer already tracking
			// the entity (confirmed there via a real replay recording where
			// a companion agent never saw the elytra equipped despite the
			// bot gliding correctly). A shift-click goes through the same
			// server-authoritative container-click path a real player's
			// unequip action would.
			armorSlot := env.ScreenMgr.Inventory().GetSlots()[6]
			require.NoError(t, env.Agent.Agent.ShiftClickSlot(6, slotToItemStack(armorSlot)), "shift-click elytra out of the armor slot mid-flight")
			require.True(t, waitForSlotState(env.ScreenMgr, 6, func(s screen.Slot) bool {
				return s.Count == 0
			}, 10*time.Second), "chest armor slot never cleared after unequipping")

			// Verify against real server-side NBT inventory data (not just
			// the client's own predicted screen state, which only proves
			// what the client predicted, not what the server actually did)
			// that the elytra genuinely moved into the main inventory
			// rather than being duplicated or lost. waitForSlotState above
			// only confirms local prediction, which fires synchronously the
			// instant ShiftClickSlot is called - well before the real
			// server-side click processing and its RCON-visible NBT update
			// necessarily complete, so this polls rather than checking
			// once (confirmed live: a single immediate check raced ahead
			// of the server and saw neither the armor slot nor the
			// inventory holding the elytra for a brief window).
			elytraCount := 0
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				invItems, invErr := GetInventoryItems(ctx, env.Inst.RCON, env.BotName)
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
			// applyGlideStateTransition), so this should clear within a
			// tick or two of syncEquipment picking up the change - not
			// waiting for landing, water, or any other unrelated condition.
			glideStoppedDeadline := time.Now().Add(2 * time.Second)
			stoppedGliding := false
			for time.Now().Before(glideStoppedDeadline) {
				if !env.Agent.Agent.IsGliding() {
					stoppedGliding = true
					break
				}
				time.Sleep(steerInterval)
			}
			assert.True(t, stoppedGliding, "gliding should stop within 2 seconds of unequipping the elytra mid-flight")

			// Let it fall the rest of the way and confirm it actually lands
			// under normal (non-glide) physics rather than getting stuck.
			landDeadline := time.Now().Add(15 * time.Second)
			var lastY, stableSince float64
			landed := false
			for time.Now().Before(landDeadline) {
				pos, ok := env.Agent.Agent.GetPositionSimple()
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

			finalPos, _ := env.Agent.Agent.GetPositionSimple()
			t.Logf("final position: (%.2f, %.2f, %.2f), landed=%v", finalPos.X, finalPos.Y, finalPos.Z, landed)
			assert.True(t, landed, "should fall and settle on the ground within the descent window after unequipping")
			assert.False(t, env.Agent.Agent.IsGliding(), "should still not be gliding after landing")
		})
	}
}
