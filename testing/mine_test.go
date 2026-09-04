package testing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	"github.com/stretchr/testify/require"
)

// blockIsAir reports whether the block at the given position is air, using
// "setblock ... keep" (which only places if the target is currently air) as
// the success/failure signal — a direct check, unlike the shared GetBlockAt
// helper, whose fallback path is unreliable for plain (non-block-entity)
// blocks (see item_usage_test.go).
func blockIsAir(ctx context.Context, rcon testenv.RCONHelper, x, y, z int) (bool, error) {
	resp, err := rcon.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:barrier keep", x, y, z))
	if err != nil {
		return false, err
	}
	return !strings.Contains(resp, "Could not set the block"), nil
}

// waitForBlockAir polls blockIsAir until it succeeds or timeout elapses —
// mirrors waitForItemEntityNear below: MineBlockAt returning is a
// client-side signal only, and the server needs a moment to actually apply
// the break. Observed flaking with a single fixed-delay check in a full
// cross-version sweep (1/7 versions, once) even though isolated reruns were
// consistently fast enough — likely resource contention from running many
// Docker-backed test servers back to back, not a real MineBlockAt issue.
func waitForBlockAir(ctx context.Context, rcon testenv.RCONHelper, x, y, z int, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		isAir, err := blockIsAir(ctx, rcon, x, y, z)
		if err != nil {
			return false, err
		}
		if isAir {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// itemEntityExistsNear checks for a dropped item entity of the given item ID
// within radius blocks of pos, via "data get entity ... Item.id" (simpler
// and more reliable than combining position/distance/nbt selector filters
// in one command, which didn't match live despite the entity genuinely
// existing — found while building this test).
func itemEntityExistsNear(ctx context.Context, rcon testenv.RCONHelper, pos models.V3, itemID string, radius float64) (bool, error) {
	selector := fmt.Sprintf("@e[type=minecraft:item,x=%.2f,y=%.2f,z=%.2f,distance=..%.1f,limit=1,sort=nearest]",
		pos.X, pos.Y, pos.Z, radius)
	resp, err := rcon.Exec(ctx, fmt.Sprintf("data get entity %s Item.id", selector))
	if err != nil {
		return false, err
	}
	return strings.Contains(resp, itemID), nil
}

// waitForItemEntityNear polls itemEntityExistsNear until it finds a match or
// timeout elapses — the dropped item entity needs a moment to spawn and
// register server-side after a block breaks, and a single fixed-delay check
// was observed to flake live while building this test.
func waitForItemEntityNear(ctx context.Context, rcon testenv.RCONHelper, pos models.V3, itemID string, radius float64, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		found, err := itemEntityExistsNear(ctx, rcon, pos, itemID, radius)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// TestMineBlockAt_StoneWithPickaxe tests MineBlockAt end to end: give the
// bot a wooden pickaxe (anywhere in inventory — MineBlockAt's
// selectBestToolForBlock finds and equips the best matching tool itself, no
// manual hotbar selection needed), mine a stone block, and verify both that
// the block broke and that it dropped cobblestone. The drop specifically
// (rather than just the block-gone check) confirms the right tool was
// actually selected — mining stone bare-handed breaks the block but drops
// nothing (see TestMineBlockAt_BareHandedNoDrop), so a cobblestone drop
// proves the tool auto-selection path worked, not just that digging packets
// were sent. Verified via the dropped item *entity*'s existence, not via
// the bot's inventory: found live that the bot, teleported ~2 blocks from
// the block for mining reach, is outside vanilla's much shorter (~1.5
// block) item pickup radius, so nothing gets auto-collected — checking the
// entity directly avoids depending on pickup mechanics, which aren't what
// this test is about.
//
// Stone with a wooden pickaxe has a real (non-instant) break time, so this
// also exercises MineBlockAt's wait-for-break-time loop and FinishDigging
// send, not just the breakTime<=0.05 "instant break" early return.
func TestMineBlockAt_StoneWithPickaxe(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "stone", tt.MCVersion)
			defer env.Cancel()

			// Teleport near the stone block
			tpTarget := models.V3{X: env.ContainerPos.X - 2, Y: env.ContainerPos.Y, Z: env.ContainerPos.Z}
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, tpTarget.X, tpTarget.Y, tpTarget.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			require.NoError(t, waitForBotNear(env, tpTarget, 1.0, 5*time.Second), "bot position sync after teleport")

			// Give the bot a wooden pickaxe in hotbar slot 1, deliberately
			// not slot 0 (the bot's default held slot from spawn): placing
			// it in slot 0 would let selectBestToolForBlock's
			// SelectHotbarSlot(0) take its "already selected" fast path,
			// which never exercises a real switch at all. Using slot 1
			// forces a genuine one — this is also a regression test for a
			// real bug SelectHotbarSlot had (see agent/hotbar.go): it used
			// to block forever waiting for a ClientboundHeldItemSlot echo
			// that vanilla never sends back to the switching client,
			// found live via this exact test hanging until context
			// timeout.
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.1 with wooden_pickaxe 1", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give wooden pickaxe")
			time.Sleep(500 * time.Millisecond)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = env.Agent.Agent.MineBlockAt(ctx, env.ContainerPos, models.FaceDown)
			require.NoError(t, err, "mine stone block")

			// MineBlockAt returning only means the client-side dig sequence
			// finished; the server needs a moment to actually apply the
			// break and, afterward, spawn the dropped item entity — poll
			// both rather than a single fixed-delay check, which was
			// observed to flake live under load (see waitForBlockAir).
			blockX, blockY, blockZ := int(env.ContainerPos.X), int(env.ContainerPos.Y), int(env.ContainerPos.Z)
			isAir, err := waitForBlockAir(env.Ctx, env.Inst.RCON, blockX, blockY, blockZ, 5*time.Second)
			require.NoError(t, err, "check block state via RCON")
			require.True(t, isAir, "stone block should have been broken")

			found, err := waitForItemEntityNear(env.Ctx, env.Inst.RCON, env.ContainerPos, "minecraft:cobblestone", 3.0, 5*time.Second)
			require.NoError(t, err, "check for dropped item via RCON")
			require.True(t, found, "should have dropped a cobblestone item entity")

			t.Log("✓ Mine stone with pickaxe test passed")
		})
	}
}

// TestMineBlockAt_BareHandedNoDrop tests that mining a block bare-handed
// (no matching tool in inventory) still breaks it — MineBlockAt's fallback
// to mining.HandTool() when no tool is found — without expecting an item
// drop, since stone specifically requires a pickaxe to drop cobblestone.
func TestMineBlockAt_BareHandedNoDrop(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "stone", tt.MCVersion)
			defer env.Cancel()

			tpTarget := models.V3{X: env.ContainerPos.X - 2, Y: env.ContainerPos.Y, Z: env.ContainerPos.Z}
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, tpTarget.X, tpTarget.Y, tpTarget.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			require.NoError(t, waitForBotNear(env, tpTarget, 1.0, 5*time.Second), "bot position sync after teleport")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = env.Agent.Agent.MineBlockAt(ctx, env.ContainerPos, models.FaceDown)
			require.NoError(t, err, "mine stone block bare-handed")

			// No item-drop check here (bare-handed stone drops nothing);
			// poll for the break rather than a single fixed-delay check
			// (see waitForBlockAir).
			blockX, blockY, blockZ := int(env.ContainerPos.X), int(env.ContainerPos.Y), int(env.ContainerPos.Z)
			isAir, err := waitForBlockAir(env.Ctx, env.Inst.RCON, blockX, blockY, blockZ, 5*time.Second)
			require.NoError(t, err, "check block state via RCON")
			require.True(t, isAir, "stone block should have been broken")

			t.Log("✓ Mine stone bare-handed test passed")
		})
	}
}
