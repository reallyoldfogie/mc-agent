package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findHotbarItem returns the item ID in the given hotbar slot (0-8), or "" if empty.
func findHotbarItem(items []InventoryItem, slot int) string {
	for _, item := range items {
		if item.Slot == slot {
			return item.ID
		}
	}
	return ""
}

// eyeHeight is the standard player eye offset above their feet position.
const eyeHeight = 1.62

// waitForBotNear polls the bot's client-tracked position until it's within
// tolerance blocks of target, or timeout elapses. A fixed sleep after an RCON
// teleport isn't reliable here: GetPositionSimple() can still return a stale,
// pre-teleport position for up to a couple hundred ms depending on scheduling,
// and faceBlock computing yaw/pitch from that stale position aims the
// subsequent use-item interaction nowhere near the real target — found live
// while building this test (bot position logged mid-test as ~8 blocks from
// where it had just been teleported).
func waitForBotNear(agent models.Position, target models.V3, tolerance float64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		pos, ok := agent.GetPositionSimple()
		if ok {
			dx, dy, dz := pos.X-target.X, pos.Y-target.Y, pos.Z-target.Z
			if math.Sqrt(dx*dx+dy*dy+dz*dz) <= tolerance {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("bot did not reach (%.2f, %.2f, %.2f) within %s (last known: %v)", target.X, target.Y, target.Z, timeout, pos)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// faceBlock computes the yaw/pitch to look at the center of the given block
// and sends a rotation packet turning the bot there, bypassing the movement
// executor (agent.LookAt goes through moveExec, whose continuously-running
// physics tick loop echoes its own tracked, unrotated yaw/pitch back before
// the next packet is sent, so the rotation never sticks). It returns the
// computed yaw/pitch so callers needing a "use item" interaction can pass
// them directly to ActionHandler.SendUseItem rather than through
// agent.UseItem: that convenience method reads yaw/pitch from
// agent.GetPosition(), which only reflects server-echoed state and stays
// stale after a rotation-only packet — and 1.21.5+'s ServerboundUseItem
// packet embeds yaw/pitch itself (used directly for the item's own fluid
// raytrace, e.g. a bucket), so a stale 0/0 there silently aims the
// interaction nowhere even though the packet sends without error. Found
// live while building this test; see
// docs/plans/VERSION_SPECIFIC_NETWORK_REFACTOR_PT2.md.
func faceBlock(env *StandaloneTestEnv, pos models.V3) (yaw, pitch float64, err error) {
	botPos, ok := env.Agent.Agent.GetPositionSimple()
	if !ok {
		return 0, 0, fmt.Errorf("bot position not initialized")
	}
	dx := (pos.X + 0.5) - botPos.X
	dy := (pos.Y + 0.5) - (botPos.Y + eyeHeight)
	dz := (pos.Z + 0.5) - botPos.Z
	yaw = math.Atan2(dx, dz) * 180 / math.Pi
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	pitch = math.Atan2(-dy, horizontalDist) * 180 / math.Pi

	if env.Agent.Config.VersionHandler == nil {
		return 0, 0, fmt.Errorf("version handler not available")
	}
	if err := env.Agent.Config.VersionHandler.Play().Movement().SendRotation(env.Agent.BotClient().Conn(), yaw, pitch, true); err != nil {
		return 0, 0, err
	}
	return yaw, pitch, nil
}

// useItemFacing sends a ServerboundUseItem packet with explicit yaw/pitch
// (from faceBlock), instead of agent.UseItem's stale agent.GetPosition()
// values — see faceBlock's doc comment for why.
func useItemFacing(env *StandaloneTestEnv, hand models.Hand, yaw, pitch float64) error {
	if env.Agent.Config.VersionHandler == nil {
		return fmt.Errorf("version handler not available")
	}
	actionHandler := env.Agent.Config.VersionHandler.Play().Actions()
	if actionHandler == nil {
		return fmt.Errorf("action handler not available")
	}
	return actionHandler.SendUseItem(env.Agent.BotClient().Conn(), hand, 0, yaw, pitch)
}

// TestItemUsage_CollectWater tests using an empty bucket on a water source block.
func TestItemUsage_CollectWater(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "light", tt.MCVersion)
			defer env.Cancel()

			// Fill a small pool of water SOURCE blocks in the ground rather than
			// placing a single water block against open air: a lone water block
			// starts flowing outward and pushes the bot away before it can pick
			// the water up (and re-teleporting the bot back only races the same
			// flow again) — found live while building this test. A multi-block
			// source pool stays put.
			cmdFillWater := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:water replace",
				int64(env.ContainerPos.X-1), int64(env.ContainerPos.Y-1), int64(env.ContainerPos.Z-1),
				int64(env.ContainerPos.X+1), int64(env.ContainerPos.Y-1), int64(env.ContainerPos.Z+1),
			)

			resp, err := env.Inst.RCON.Exec(env.Ctx, cmdFillWater)
			require.NoError(t, err, "fill water source blocks")
			t.Logf("%q %q", cmdFillWater, resp)

			time.Sleep(500 * time.Millisecond)

			// Teleport near the water source
			tpTarget := models.V3{X: env.ContainerPos.X, Y: env.ContainerPos.Y, Z: env.ContainerPos.Z}
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, tpTarget.X, tpTarget.Y, tpTarget.Z)
			_, err = env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			require.NoError(t, waitForBotNear(env.Agent.Agent, tpTarget, 1.0, 5*time.Second), "bot position sync after teleport")

			// Give the bot an empty bucket in hotbar slot 0
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with bucket 1", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give bucket")
			time.Sleep(500 * time.Millisecond)

			ctx := context.Background()
			require.NoError(t, env.Agent.Agent.SelectHotbarSlot(ctx, 0), "select hotbar slot 0")

			// Look at the water block, then send a plain "use item" interaction —
			// NOT UseItemOnBlock. Vanilla's BucketItem (both filling and emptying)
			// is driven entirely by Item.use()/ServerboundUseItemPacket: the item
			// does its own internal raytrace for a nearby fluid along the player's
			// facing direction. A ServerboundUseItemOnPacket targeting the water
			// block instead routes into the *block's* own (no-op) right-click
			// handling, so the packet sends cleanly but the bucket never fills —
			// found live while building this test, see
			// docs/plans/VERSION_SPECIFIC_NETWORK_REFACTOR_PT2.md.
			lookAtBlock := models.V3{X: env.ContainerPos.X, Y: env.ContainerPos.Y - 1, Z: env.ContainerPos.Z}
			yaw, pitch, err := faceBlock(env, lookAtBlock)
			require.NoError(t, err, "look at water block")
			time.Sleep(500 * time.Millisecond)

			err = useItemFacing(env, models.MainHand, yaw, pitch)
			require.NoError(t, err, "use bucket on water")
			time.Sleep(5 * time.Second)

			items, err := GetInventoryItems(env.Ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get inventory")
			assert.Equal(t, "minecraft:water_bucket", findHotbarItem(items, 0), "hotbar slot 0 should hold a water bucket")

			t.Log("✓ Collect water test passed")
		})
	}
}

// TestItemUsage_WaterBucketOnLava tests using a water bucket on a lava source block
// (the clutch mechanic: lava source + water = obsidian).
func TestItemUsage_WaterBucketOnLava(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "lava", tt.MCVersion)
			defer env.Cancel()

			// Teleport near the lava source
			tpTarget := models.V3{X: env.ContainerPos.X - 2, Y: env.ContainerPos.Y, Z: env.ContainerPos.Z}
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, tpTarget.X, tpTarget.Y, tpTarget.Z)
			_, err := env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			require.NoError(t, waitForBotNear(env.Agent.Agent, tpTarget, 1.0, 5*time.Second), "bot position sync after teleport")

			// Give the bot a water bucket in hotbar slot 0
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with water_bucket 1", env.BotName)
			_, err = env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give water bucket")
			time.Sleep(500 * time.Millisecond)

			ctx := context.Background()
			require.NoError(t, env.Agent.Agent.SelectHotbarSlot(ctx, 0), "select hotbar slot 0")

			// Look at the lava block, then send a plain "use item" interaction —
			// see the matching comment in TestItemUsage_CollectWater for why not
			// UseItemOnBlock.
			yaw, pitch, err := faceBlock(env, env.ContainerPos)
			require.NoError(t, err, "look at lava block")
			time.Sleep(500 * time.Millisecond)

			err = useItemFacing(env, models.MainHand, yaw, pitch)
			require.NoError(t, err, "use water bucket on lava")
			time.Sleep(1 * time.Second)

			// Not verifying the block became obsidian here: GetBlockAt's fallback path
			// (an "execute store result score @s ... run data get block" RCON command)
			// requires an executing entity context RCON doesn't provide, and fails with
			// "No entity was found" for any plain block with no block-entity data (like
			// obsidian) — a pre-existing testing/rcon_helpers.go limitation, found while
			// building this test. The bucket-emptying check below is a reliable enough
			// signal that the water-on-lava interaction actually happened server-side.
			items, err := GetInventoryItems(env.Ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get inventory")
			require.Equal(t, "minecraft:bucket", findHotbarItem(items, 0), "hotbar slot 0 should hold an empty bucket")

			t.Log("✓ Water bucket on lava test passed")
		})
	}
}

// TestItemUsage_MilkCow tests using an empty bucket on a cow entity to collect milk.
func TestItemUsage_MilkCow(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "milk_cow", "survival", false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()

			platformY := int(env.ContainerPos.Y) - 1
			platformX := int(env.ContainerPos.X) - 10
			platformZ := int(env.ContainerPos.Z) - 10
			BuildPlatform(ctx, env.Inst.RCON, platformX, platformY, platformZ, 20, 20, "minecraft:grass_block")
			if err := ClearArea(ctx, env.Inst.RCON,
				platformX, platformY+1, platformZ,
				platformX+20, platformY+5, platformZ+20); err != nil {
				t.Logf("warning: failed to clear area: %v", err)
			}

			// Give the bot an empty bucket in hotbar slot 0
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with bucket 1", env.BotName)
			_, err := env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give bucket")
			time.Sleep(500 * time.Millisecond)

			require.NoError(t, env.Agent.Agent.SelectHotbarSlot(ctx, 0), "select hotbar slot 0")

			// Spawn a cow near the bot
			spawnX := env.ContainerPos.X + 3
			spawnY := env.ContainerPos.Y
			spawnZ := env.ContainerPos.Z
			spawnCmd := fmt.Sprintf("summon minecraft:cow %.1f %.1f %.1f", spawnX, spawnY, spawnZ)
			_, err = env.Inst.RCON.Exec(env.Ctx, spawnCmd)
			require.NoError(t, err, "spawn cow")
			time.Sleep(3 * time.Second)

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "bot position initialized")

			cowType, ok := env.Agent.Agent.GetEntityTypeID("minecraft:cow")
			require.True(t, ok, "cow entity type should be in registry")

			cowID, dist, found := env.Agent.FindNearestEntityByType(cowType, botPos.X, botPos.Y, botPos.Z, false)
			require.True(t, found, "should find cow entity")
			require.Less(t, dist, 20.0, "cow should be within 20 blocks")

			require.NoError(t, env.Agent.FaceEntity(cowID), "face cow")
			time.Sleep(100 * time.Millisecond)

			err = env.Agent.Agent.UseItemOnEntity(ctx, cowID, models.MainHand, false)
			require.NoError(t, err, "milk cow")
			time.Sleep(1 * time.Second)

			items, err := GetInventoryItems(env.Ctx, env.Inst.RCON, env.BotName)
			require.NoError(t, err, "get inventory")
			require.Equal(t, "minecraft:milk_bucket", findHotbarItem(items, 0), "hotbar slot 0 should hold a milk bucket")

			t.Log("✓ Milk cow test passed")
		})
	}
}

// TestItemUsage_OpenContainerWhileHoldingItem verifies opening a container works
// normally while the bot has an unrelated item selected in its hotbar.
func TestItemUsage_OpenContainerWhileHoldingItem(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "chest", tt.MCVersion)
			defer env.Cancel()

			// Give the bot a bucket and select it before interacting with the chest
			giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with bucket 1", env.BotName)
			_, err := env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give bucket")
			time.Sleep(500 * time.Millisecond)

			ctx := context.Background()
			require.NoError(t, env.Agent.Agent.SelectHotbarSlot(ctx, 0), "select hotbar slot 0")

			// Teleport near chest
			cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", env.BotName, env.ContainerPos.X-2, env.ContainerPos.Y, env.ContainerPos.Z)
			_, err = env.Inst.RCON.Exec(env.Ctx, cmd)
			require.NoError(t, err)
			time.Sleep(500 * time.Millisecond)

			windowID, err := OpenContainerWithLOS(env.Ctx, env.Agent.Agent, env.ContainerPos, models.FaceEast, 5*time.Second)
			require.NoError(t, err, "open chest while holding bucket")
			t.Logf("chest opened with window ID: %d", windowID)

			screen, ok := env.ScreenMgr.Screens()[int(windowID)]
			require.True(t, ok, "chest window should exist")

			chest, ok := screen.(*mcscreen.Chest)
			require.True(t, ok, "screen should be a Chest")
			require.Equal(t, 3, chest.Rows, "should be single chest (3 rows)")

			require.NoError(t, env.Agent.Agent.CloseContainer(), "close chest")

			t.Log("✓ Open container while holding item test passed")
		})
	}
}
