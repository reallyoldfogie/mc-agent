package agent

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/stretchr/testify/require"
)

// findStateIDByName brute-force scans block IDs for a block whose name
// matches, returning a representative (minimum) state ID for it. There's
// no direct name->state-ID lookup on protocol_versions.BlockMgr - only the
// reverse (BlockIDByStateID + GetByID) - so this mirrors what a human
// reading the registry data by hand would do.
func findStateIDByName(t *testing.T, a *agent, name string) uint32 {
	t.Helper()
	for id := protocol_models.BlockID(0); id < 2048; id++ {
		block, ok := a.blockMgr.GetByID(id)
		if !ok {
			continue
		}
		if block.Name == name {
			return uint32(block.MinStateID)
		}
	}
	t.Fatalf("block %q not found in registry", name)
	return 0
}

// TestUsePortal_NoPortalFound confirms UsePortal fails closed (a clear
// error, not a hang or panic) when no portal block exists anywhere within
// range - the common case for a freshly-initialized agent with no chunks
// loaded.
func TestUsePortal_NoPortalFound(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	a.UpdatePosition(models.V3{}, 0, 0)

	err = a.UsePortal(context.Background(), 5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no portal block found")
}

// TestUsePortal_WalksInThenTimesOutWaitingForDimensionChange exercises the
// real findNearestPortalBlock -> MoveTo -> wait-for-dimension-change
// sequence end to end, with the agent starting exactly at the portal
// block's coordinates (so MoveTo's "already at target" shortcut applies
// and no real pathfinder run is needed) and nothing ever triggering a
// dimension change — confirming UsePortal correctly reports a timeout
// instead of hanging or silently returning success.
func TestUsePortal_WalksInThenTimesOutWaitingForDimensionChange(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	portalStateID := findStateIDByName(t, a, "minecraft:nether_portal")
	require.NotNil(t, a.mcAgentWorld)
	a.mcAgentWorld.HandleBlockUpdate(10, 64, 10, int32(portalStateID))

	a.UpdatePosition(models.V3{X: 10, Y: 64, Z: 10}, 0, 0)

	original := usePortalDimensionChangeTimeout
	usePortalDimensionChangeTimeout = 300 * time.Millisecond
	defer func() { usePortalDimensionChangeTimeout = original }()

	err = a.UsePortal(context.Background(), 5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out waiting for dimension change")
}

// TestFindNearestPortalBlock_PicksNearestAcrossNames seeds an end_portal
// far away and a nether_portal close by, confirming the nearer one wins
// even though it isn't the first name searched (portalBlockNames searches
// nether_portal first) - a bug that only compared within one name at a
// time, or kept whichever matched last, wouldn't show up with only one
// portal type present.
func TestFindNearestPortalBlock_PicksNearestAcrossNames(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	netherStateID := findStateIDByName(t, a, "minecraft:nether_portal")
	endStateID := findStateIDByName(t, a, "minecraft:end_portal")

	a.UpdatePosition(models.V3{X: 0, Y: 64, Z: 0}, 0, 0)

	// FindVisibleBlock requires line of sight, which errors on any
	// genuinely-unloaded cell along the ray rather than treating it as
	// "not visible" — fill the whole area the LOS check could reach with
	// air overrides (cheaper than constructing a real loaded chunk) so
	// only the two portal cells below are non-air.
	for x := int64(-2); x <= 2; x++ {
		for y := int64(62); y <= 67; y++ {
			for z := int64(-2); z <= 22; z++ {
				a.mcAgentWorld.HandleBlockUpdate(x, y, z, 0)
			}
		}
	}
	a.mcAgentWorld.HandleBlockUpdate(0, 64, 20, int32(netherStateID)) // far
	a.mcAgentWorld.HandleBlockUpdate(0, 64, 3, int32(endStateID))     // near

	x, y, z, found, err := a.findNearestPortalBlock(context.Background(), 30)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 0.0, x)
	require.Equal(t, 64.0, y)
	require.Equal(t, 3.0, z)
}
