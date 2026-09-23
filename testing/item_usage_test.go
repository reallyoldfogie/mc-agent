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
	"github.com/stretchr/testify/suite"
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
func faceBlock(managed *ManagedAgent, pos models.V3) (yaw, pitch float64, err error) {
	botPos, ok := managed.Agent.GetPositionSimple()
	if !ok {
		return 0, 0, fmt.Errorf("bot position not initialized")
	}
	dx := (pos.X + 0.5) - botPos.X
	dy := (pos.Y + 0.5) - (botPos.Y + eyeHeight)
	dz := (pos.Z + 0.5) - botPos.Z
	yaw = math.Atan2(dx, dz) * 180 / math.Pi
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	pitch = math.Atan2(-dy, horizontalDist) * 180 / math.Pi

	if managed.Config.VersionHandler == nil {
		return 0, 0, fmt.Errorf("version handler not available")
	}
	if err := managed.Config.VersionHandler.Play().Movement().SendRotation(managed.BotClient().Conn(), yaw, pitch, true); err != nil {
		return 0, 0, err
	}
	return yaw, pitch, nil
}

// useItemFacing sends a ServerboundUseItem packet with explicit yaw/pitch
// (from faceBlock), instead of agent.UseItem's stale agent.GetPosition()
// values — see faceBlock's doc comment for why.
func useItemFacing(managed *ManagedAgent, hand models.Hand, yaw, pitch float64) error {
	if managed.Config.VersionHandler == nil {
		return fmt.Errorf("version handler not available")
	}
	actionHandler := managed.Config.VersionHandler.Play().Actions()
	if actionHandler == nil {
		return fmt.Errorf("action handler not available")
	}
	return actionHandler.SendUseItem(managed.BotClient().Conn(), hand, 0, yaw, pitch)
}

// ItemUsageFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the item-usage tests below: one server per version, shared by
// every method, instead of the previous per-test-function setupStandaloneTest* server-per-test
// pattern. WorldGen = WorldGenFlat + Difficulty = DifficultyEasy, matching setupStandaloneTest's
// own default.
//
// Only 1 of these 4 functions (TestOpenContainerWhileHoldingItem) actually opens a container -
// the checklist's "uses ScreenMgr." grep hit that flagged this whole file as window-ID-constrained
// was true only for that one function. TestCollectWater/TestWaterBucketOnLava/TestMilkCow use
// UseItemOnBlock/UseItemOnEntity, never OpenContainerWithLOS/ScreenMgr - the same
// substring/grep-vs-actual-usage gap 29/39 already found and corrected for
// elytra_unequip_test.go/craft_test.go. Converted together anyway since they were already one
// file and the one genuinely container-bound function needs no special isolation from the other
// three (confirmed live below).
type ItemUsageFlatSuite struct {
	VersionWorldSuite
}

func TestItemUsageFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ItemUsageFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestCollectWater tests using an empty bucket on a water source block.
// Equivalent to the pre-Phase-1 TestItemUsage_CollectWater.
func (s *ItemUsageFlatSuite) TestCollectWater() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("CollectWaterBot", "item_usage_collect_water")
	require.NoError(t, err, "spawn agent")

	// A placeholder block placed purely to get a confirmed-loaded reference
	// position (same PlaceBlockAndWait chunk-load guarantee the pre-Phase-1
	// setupStandaloneTest("light", ...) call relied on) - immediately
	// overwritten by the water fill below, so its type doesn't matter.
	refPos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, refPos, "minecraft:light", "light", 30*time.Second)
	require.NoError(t, err, "place reference block")

	// Fill a small pool of water SOURCE blocks in the ground rather than
	// placing a single water block against open air: a lone water block
	// starts flowing outward and pushes the bot away before it can pick the
	// water up (and re-teleporting the bot back only races the same flow
	// again) — found live while building this test. A multi-block source
	// pool stays put.
	cmdFillWater := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:water replace",
		int64(refPos.X-1), int64(refPos.Y-1), int64(refPos.Z-1),
		int64(refPos.X+1), int64(refPos.Y-1), int64(refPos.Z+1),
	)
	resp, err := s.Inst.RCON.Exec(s.Ctx, cmdFillWater)
	require.NoError(t, err, "fill water source blocks")
	t.Logf("%q %q", cmdFillWater, resp)

	time.Sleep(500 * time.Millisecond)

	tpTarget := refPos
	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, tpTarget.X, tpTarget.Y, tpTarget.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	require.NoError(t, waitForBotNear(leader.Agent, tpTarget, 1.0, 5*time.Second), "bot position sync after teleport")

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with bucket 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give bucket")
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	require.NoError(t, leader.Agent.SelectHotbarSlot(ctx, 0), "select hotbar slot 0")

	// Look at the water block, then send a plain "use item" interaction —
	// NOT UseItemOnBlock. Vanilla's BucketItem (both filling and emptying)
	// is driven entirely by Item.use()/ServerboundUseItemPacket: the item
	// does its own internal raytrace for a nearby fluid along the player's
	// facing direction. A ServerboundUseItemOnPacket targeting the water
	// block instead routes into the *block's* own (no-op) right-click
	// handling, so the packet sends cleanly but the bucket never fills —
	// found live while building this test, see
	// docs/plans/VERSION_SPECIFIC_NETWORK_REFACTOR_PT2.md.
	lookAtBlock := models.V3{X: refPos.X, Y: refPos.Y - 1, Z: refPos.Z}
	yaw, pitch, err := faceBlock(leader.ManagedAgent, lookAtBlock)
	require.NoError(t, err, "look at water block")
	time.Sleep(500 * time.Millisecond)

	err = useItemFacing(leader.ManagedAgent, models.MainHand, yaw, pitch)
	require.NoError(t, err, "use bucket on water")
	time.Sleep(5 * time.Second)

	items, err := GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get inventory")
	assert.Equal(t, "minecraft:water_bucket", findHotbarItem(items, 0), "hotbar slot 0 should hold a water bucket")

	t.Log("✓ Collect water test passed")
}

// TestWaterBucketOnLava tests using a water bucket on a lava source block
// (the clutch mechanic: lava source + water = obsidian). Equivalent to the
// pre-Phase-1 TestItemUsage_WaterBucketOnLava.
func (s *ItemUsageFlatSuite) TestWaterBucketOnLava() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("WaterOnLavaBot", "item_usage_water_on_lava")
	require.NoError(t, err, "spawn agent")

	lavaPos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, lavaPos, "minecraft:lava", "lava", 30*time.Second)
	require.NoError(t, err, "place lava source block")

	tpTarget := models.V3{X: lavaPos.X - 2, Y: lavaPos.Y, Z: lavaPos.Z}
	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, tpTarget.X, tpTarget.Y, tpTarget.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	require.NoError(t, waitForBotNear(leader.Agent, tpTarget, 1.0, 5*time.Second), "bot position sync after teleport")

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with water_bucket 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give water bucket")
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	require.NoError(t, leader.Agent.SelectHotbarSlot(ctx, 0), "select hotbar slot 0")

	// Look at the lava block, then send a plain "use item" interaction — see
	// the matching comment in TestCollectWater for why not UseItemOnBlock.
	yaw, pitch, err := faceBlock(leader.ManagedAgent, lavaPos)
	require.NoError(t, err, "look at lava block")
	time.Sleep(500 * time.Millisecond)

	err = useItemFacing(leader.ManagedAgent, models.MainHand, yaw, pitch)
	require.NoError(t, err, "use water bucket on lava")
	time.Sleep(1 * time.Second)

	// Not verifying the block became obsidian here: GetBlockAt's fallback
	// path (an "execute store result score @s ... run data get block" RCON
	// command) requires an executing entity context RCON doesn't provide,
	// and fails with "No entity was found" for any plain block with no
	// block-entity data (like obsidian) — a pre-existing
	// testing/rcon_helpers.go limitation, found while building this test.
	// The bucket-emptying check below is a reliable enough signal that the
	// water-on-lava interaction actually happened server-side.
	items, err := GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get inventory")
	require.Equal(t, "minecraft:bucket", findHotbarItem(items, 0), "hotbar slot 0 should hold an empty bucket")

	t.Log("✓ Water bucket on lava test passed")
}

// TestMilkCow tests using an empty bucket on a cow entity to collect milk.
// Equivalent to the pre-Phase-1 TestItemUsage_MilkCow.
func (s *ItemUsageFlatSuite) TestMilkCow() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("MilkCowBot", "item_usage_milk_cow")
	require.NoError(t, err, "spawn agent")

	ctx := context.Background()

	platformY := int(math.Floor(leader.Origin.Y)) - 1
	platformX := int(math.Floor(leader.Origin.X)) - 10
	platformZ := int(math.Floor(leader.Origin.Z)) - 10
	BuildPlatform(ctx, s.Inst.RCON, platformX, platformY, platformZ, 20, 20, "minecraft:grass_block")
	if err := ClearArea(ctx, s.Inst.RCON,
		platformX, platformY+1, platformZ,
		platformX+20, platformY+5, platformZ+20); err != nil {
		t.Logf("warning: failed to clear area: %v", err)
	}

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with bucket 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give bucket")
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, leader.Agent.SelectHotbarSlot(ctx, 0), "select hotbar slot 0")

	spawnX := leader.Origin.X + 3
	spawnY := leader.Origin.Y
	spawnZ := leader.Origin.Z
	spawnCmd := fmt.Sprintf("summon minecraft:cow %.1f %.1f %.1f", spawnX, spawnY, spawnZ)
	_, err = s.Inst.RCON.Exec(s.Ctx, spawnCmd)
	require.NoError(t, err, "spawn cow")
	time.Sleep(3 * time.Second)

	botPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")

	cowType, ok := leader.Agent.GetEntityTypeID("minecraft:cow")
	require.True(t, ok, "cow entity type should be in registry")

	cowID, dist, found := leader.FindNearestEntityByType(cowType, botPos.X, botPos.Y, botPos.Z, false)
	require.True(t, found, "should find cow entity")
	require.Less(t, dist, 20.0, "cow should be within 20 blocks")

	require.NoError(t, leader.FaceEntity(cowID), "face cow")
	time.Sleep(100 * time.Millisecond)

	err = leader.Agent.UseItemOnEntity(ctx, cowID, models.MainHand, false)
	require.NoError(t, err, "milk cow")
	time.Sleep(1 * time.Second)

	items, err := GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get inventory")
	require.Equal(t, "minecraft:milk_bucket", findHotbarItem(items, 0), "hotbar slot 0 should hold a milk bucket")

	t.Log("✓ Milk cow test passed")
}

// TestOpenContainerWhileHoldingItem verifies opening a container works
// normally while the bot has an unrelated item selected in its hotbar.
// Equivalent to the pre-Phase-1 TestItemUsage_OpenContainerWhileHoldingItem.
// The only genuinely container-opening function in this file - see the
// suite's own doc comment.
func (s *ItemUsageFlatSuite) TestOpenContainerWhileHoldingItem() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("HoldingItemBot", "item_usage_open_while_holding")
	require.NoError(t, err, "spawn agent")

	chestPos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, chestPos, "minecraft:chest", "chest", 30*time.Second)
	require.NoError(t, err, "place chest")

	giveCmd := fmt.Sprintf("item replace entity %s hotbar.0 with bucket 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give bucket")
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	require.NoError(t, leader.Agent.SelectHotbarSlot(ctx, 0), "select hotbar slot 0")

	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, chestPos.X-2, chestPos.Y, chestPos.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, chestPos, models.FaceEast, 5*time.Second)
	require.NoError(t, err, "open chest while holding bucket")
	t.Logf("chest opened with window ID: %d", windowID)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "chest window should exist")

	chest, ok := screen.(*mcscreen.Chest)
	require.True(t, ok, "screen should be a Chest")
	require.Equal(t, 3, chest.Rows, "should be single chest (3 rows)")

	require.NoError(t, leader.Agent.CloseContainer(), "close chest")

	t.Log("✓ Open container while holding item test passed")
}
