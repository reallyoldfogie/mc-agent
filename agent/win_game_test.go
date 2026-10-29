package agent

import (
	"context"
	"testing"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"
	"github.com/stretchr/testify/require"
)

// buildGameEventPacket constructs a real ClientboundGameEvent (called
// GameStateChange pre-1.20.3) wire packet for 1.21.5.
func buildGameEventPacket(reason uint8, value float32) pk.Packet {
	ev := cb.NewGameStateChange()
	ev.Reason = pk.UnsignedByte(reason)
	ev.GameMode = pk.Float(value)
	return ev.Marshal()
}

// TestOnGameEvent_WinGame_FirstTime confirms the win-game (value!=0, first
// time seeing the end credits) branch pauses movement and eventually sends
// PERFORM_RESPAWN via the same models.Respawner death respawn uses.
func TestOnGameEvent_WinGame_FirstTime(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	mockConn := &MockConn{}
	mockPlayer := NewMockPlayer(mockConn)
	a.player = mockPlayer

	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)

	p := buildGameEventPacket(4, 1.0) // eventType 4 = win_game, value=1 (first time)
	require.NoError(t, a.onGameEvent(p))

	require.Equal(t, 1, fm.NotifyDeadCalls(), "win-game should pause outbound position updates immediately")
	require.Equal(t, 0, mockPlayer.RespawnCalls(), "Respawn() should not fire before the credits delay elapses")

	require.Eventually(t, func() bool {
		return mockPlayer.RespawnCalls() == 1
	}, winGameRespawnDelaySeenCredits+2*time.Second, 50*time.Millisecond,
		"Respawn() should fire once after the first-time credits delay")
}

// TestOnGameEvent_WinGame_AlreadySeenCredits confirms value==0 (every
// subsequent dragon kill/portal use) uses the short delay, not the
// first-time one.
func TestOnGameEvent_WinGame_AlreadySeenCredits(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	mockConn := &MockConn{}
	mockPlayer := NewMockPlayer(mockConn)
	a.player = mockPlayer

	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)

	p := buildGameEventPacket(4, 0.0) // eventType 4 = win_game, value=0 (already seen)
	require.NoError(t, a.onGameEvent(p))

	require.Eventually(t, func() bool {
		return mockPlayer.RespawnCalls() == 1
	}, winGameRespawnDelayAlreadySeenCredits+2*time.Second, 20*time.Millisecond,
		"Respawn() should fire quickly when credits have already been seen")
}
