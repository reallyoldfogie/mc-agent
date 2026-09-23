package testing

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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

// MineFlatSuite covers MineBlockAt end to end against a shared flat-world
// server - one stone block placed per method (via PlaceBlockAndWait, not the
// direct Origin-substitution pattern most other conversions have used, since
// this test needs a real solid block to mine, not just a spawn point to
// compute destinations from).
type MineFlatSuite struct {
	VersionWorldSuite
}

func TestMineFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &MineFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// placeMineTarget places a stone block 5 blocks over from leader's working
// area (matching setupStandaloneTestWithModeAndBlockPlacement's own
// containerX/Y/Z math) and returns its center position (matching
// StandaloneTestEnv.ContainerPos exactly, so the rest of this file's logic -
// teleport offset, mining distance - carries over unchanged).
func placeMineTarget(s *VersionWorldSuite, leader *WorkingAreaAgent) (models.V3, error) {
	blockX := int(math.Floor(leader.Origin.X)) + 5
	blockY := int(math.Floor(leader.Origin.Y))
	blockZ := int(math.Floor(leader.Origin.Z))
	blockPos := models.V3{X: float64(blockX), Y: float64(blockY), Z: float64(blockZ)}
	if _, err := PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, blockPos, "minecraft:stone", "stone", 30*time.Second); err != nil {
		return models.V3{}, err
	}
	return models.V3{X: float64(blockX) + 0.5, Y: float64(blockY), Z: float64(blockZ) + 0.5}, nil
}

// TestStoneWithPickaxe tests MineBlockAt end to end: give the bot a wooden
// pickaxe (anywhere in inventory — MineBlockAt's selectBestToolForBlock finds
// and equips the best matching tool itself, no manual hotbar selection
// needed), mine a stone block, and verify both that the block broke and that
// it dropped cobblestone. The drop specifically (rather than just the
// block-gone check) confirms the right tool was actually selected — mining
// stone bare-handed breaks the block but drops nothing (see
// TestBareHandedNoDrop), so a cobblestone drop proves the tool auto-selection
// path worked, not just that digging packets were sent. Verified via the
// dropped item *entity*'s existence, not via the bot's inventory: found live
// that the bot, teleported ~2 blocks from the block for mining reach, is
// outside vanilla's much shorter (~1.5 block) item pickup radius, so nothing
// gets auto-collected — checking the entity directly avoids depending on
// pickup mechanics, which aren't what this test is about.
//
// Stone with a wooden pickaxe has a real (non-instant) break time, so this
// also exercises MineBlockAt's wait-for-break-time loop and FinishDigging
// send, not just the breakTime<=0.05 "instant break" early return.
func (s *MineFlatSuite) TestStoneWithPickaxe() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("MineStoneBot", "mine_stone_pickaxe")
	require.NoError(t, err, "spawn agent")

	minePos, err := placeMineTarget(&s.VersionWorldSuite, leader)
	require.NoError(t, err, "place stone block")

	// Teleport near the stone block
	tpTarget := models.V3{X: minePos.X - 2, Y: minePos.Y, Z: minePos.Z}
	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, tpTarget.X, tpTarget.Y, tpTarget.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	require.NoError(t, waitForBotNear(leader.Agent, tpTarget, 1.0, 5*time.Second), "bot position sync after teleport")

	// Give the bot a wooden pickaxe in hotbar slot 1, deliberately not slot 0
	// (the bot's default held slot from spawn): placing it in slot 0 would
	// let selectBestToolForBlock's SelectHotbarSlot(0) take its "already
	// selected" fast path, which never exercises a real switch at all. Using
	// slot 1 forces a genuine one — this is also a regression test for a
	// real bug SelectHotbarSlot had (see agent/hotbar.go): it used to block
	// forever waiting for a ClientboundHeldItemSlot echo that vanilla never
	// sends back to the switching client, found live via this exact test
	// hanging until context timeout.
	giveCmd := fmt.Sprintf("item replace entity %s hotbar.1 with wooden_pickaxe 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give wooden pickaxe")
	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = leader.Agent.MineBlockAt(ctx, minePos, models.FaceDown)
	require.NoError(t, err, "mine stone block")

	// MineBlockAt returning only means the client-side dig sequence
	// finished; the server needs a moment to actually apply the break and,
	// afterward, spawn the dropped item entity — poll both rather than a
	// single fixed-delay check, which was observed to flake live under load
	// (see waitForBlockAir).
	blockX, blockY, blockZ := int(minePos.X), int(minePos.Y), int(minePos.Z)
	isAir, err := waitForBlockAir(s.Ctx, s.Inst.RCON, blockX, blockY, blockZ, 5*time.Second)
	require.NoError(t, err, "check block state via RCON")
	require.True(t, isAir, "stone block should have been broken")

	found, err := waitForItemEntityNear(s.Ctx, s.Inst.RCON, minePos, "minecraft:cobblestone", 3.0, 5*time.Second)
	require.NoError(t, err, "check for dropped item via RCON")
	require.True(t, found, "should have dropped a cobblestone item entity")
}

// TestBareHandedNoDrop tests that mining a block bare-handed (no matching
// tool in inventory) still breaks it — MineBlockAt's fallback to
// mining.HandTool() when no tool is found — without expecting an item drop,
// since stone specifically requires a pickaxe to drop cobblestone.
func (s *MineFlatSuite) TestBareHandedNoDrop() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("MineBareHandBot", "mine_stone_bare_handed")
	require.NoError(t, err, "spawn agent")

	minePos, err := placeMineTarget(&s.VersionWorldSuite, leader)
	require.NoError(t, err, "place stone block")

	tpTarget := models.V3{X: minePos.X - 2, Y: minePos.Y, Z: minePos.Z}
	cmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, tpTarget.X, tpTarget.Y, tpTarget.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, cmd)
	require.NoError(t, err)
	require.NoError(t, waitForBotNear(leader.Agent, tpTarget, 1.0, 5*time.Second), "bot position sync after teleport")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = leader.Agent.MineBlockAt(ctx, minePos, models.FaceDown)
	require.NoError(t, err, "mine stone block bare-handed")

	// No item-drop check here (bare-handed stone drops nothing); poll for
	// the break rather than a single fixed-delay check (see
	// waitForBlockAir).
	blockX, blockY, blockZ := int(minePos.X), int(minePos.Y), int(minePos.Z)
	isAir, err := waitForBlockAir(s.Ctx, s.Inst.RCON, blockX, blockY, blockZ, 5*time.Second)
	require.NoError(t, err, "check block state via RCON")
	require.True(t, isAir, "stone block should have been broken")
}
