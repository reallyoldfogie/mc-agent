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
// Removing the elytra via a real RCON `item replace` while airborne (the
// same command every other elytra test already uses at cleanup, just moved
// mid-flight here) exercises the exact server round-trip this depends on:
// the agent has no inventory access of its own inside physics.State, so
// CanGlide only sees the equipment change once movement/physics_executor.go's
// syncEquipment reads it back from the ClientboundSetSlot the server sends
// in response - this test also confirms that path doesn't lag long enough
// to matter, not just that the flag flips in principle.
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

			// Remove the elytra mid-flight - the same RCON command every
			// other elytra test already runs at cleanup, just while
			// airborne here instead of after landing.
			_, err = env.Inst.RCON.Exec(ctx, fmt.Sprintf("item replace entity %s armor.chest with air", env.BotName))
			require.NoError(t, err, "unequip elytra mid-flight")
			require.True(t, waitForSlotState(env.ScreenMgr, 6, func(s screen.Slot) bool {
				return s.Count == 0
			}, 10*time.Second), "chest armor slot never cleared after unequipping")

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
