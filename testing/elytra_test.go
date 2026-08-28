package testing

import (
	"context"
	"fmt"
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
