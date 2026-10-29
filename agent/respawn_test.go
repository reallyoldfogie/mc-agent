package agent

import (
	"context"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"
	"github.com/stretchr/testify/require"
)

// buildRespawnPacket constructs a real ClientboundRespawn wire packet for
// 1.21.5, mirroring what a server actually sends on a dimension change.
func buildRespawnPacket(t *testing.T, dimensionTypeID int32, dimensionName string) pk.Packet {
	t.Helper()
	respawn := cb.NewRespawn()
	respawn.WorldState.Dimension = pk.VarInt(dimensionTypeID)
	respawn.WorldState.Name = pk.String(dimensionName)
	respawn.WorldState.Gamemode.Value = "survival"
	return respawn.Marshal()
}

// TestOnRespawn_ResetsDimensionAndWorldState exercises the full onRespawn
// dispatch added for dimension-change handling: dimension state update,
// world reset (chunks/time/difficulty/border), entity-tracking reset, the
// player-loaded gate re-arming, and the outbound-position pause.
func TestOnRespawn_ResetsDimensionAndWorldState(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)

	// Prime pre-transfer state as if the bot had been in the Overworld for
	// a while: world time/an entity/the player-loaded gate already fired.
	require.NotNil(t, a.worldMgr)
	a.worldMgr.SetWorldTime(12345, 6000)
	a.worldMgr.SetDifficulty(2, false)
	_, _, initializedBefore := a.GetDimension()
	require.False(t, initializedBefore, "dimension should be uninitialized before any Respawn")

	a.entitiesMu.Lock()
	a.entities[1] = &trackedEntity{EntityID: 1}
	a.entitiesMu.Unlock()

	a.playerLoadedSentMu.Lock()
	a.playerLoadedSent = true
	a.playerLoadedSentMu.Unlock()

	p := buildRespawnPacket(t, 5, "minecraft:the_nether")
	require.NoError(t, a.onRespawn(p))

	dimTypeID, dimName, initialized := a.GetDimension()
	require.True(t, initialized)
	require.Equal(t, int32(5), dimTypeID)
	require.Equal(t, "minecraft:the_nether", dimName)

	_, ok := a.worldMgr.GetWorldAge()
	require.False(t, ok, "world age should be cleared by Reset()")
	_, ok = a.worldMgr.GetTimeOfDay()
	require.False(t, ok, "time of day should be cleared by Reset()")
	_, _, ok = a.worldMgr.GetDifficulty()
	require.False(t, ok, "difficulty should be cleared by Reset()")

	a.entitiesMu.RLock()
	entityCount := len(a.entities)
	a.entitiesMu.RUnlock()
	require.Zero(t, entityCount, "entity tracking should be cleared on dimension change")

	a.playerLoadedSentMu.Lock()
	gateArmed := a.playerLoadedSent
	a.playerLoadedSentMu.Unlock()
	require.False(t, gateArmed, "player-loaded gate should be re-armed (not sent) for the new dimension")

	require.Equal(t, 1, fm.NotifyDeadCalls(), "onRespawn should pause outbound position updates")
}

// TestOnRespawn_DoesNotChangeEntityID confirms Respawn (unlike Login) never
// touches the bot's own entity ID — the connection and entity stay the same
// across a dimension transfer, per protocol.
func TestOnRespawn_DoesNotChangeEntityID(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	a.setEntityID(42)
	p := buildRespawnPacket(t, 2, "minecraft:the_end")
	require.NoError(t, a.onRespawn(p))

	require.Equal(t, int32(42), a.GetEntityID())
}
