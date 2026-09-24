package agent

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

func TestBreakTimeScale_TracksServerTickRate(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)
	a := agentInt.(*agent)

	require.Equal(t, 1.0, a.breakTimeScale(), "no tick rate reported yet must behave like vanilla")

	a.serverTickRateBits.Store(math.Float32bits(80))
	require.InDelta(t, 0.25, a.breakTimeScale(), 1e-9, "4x server: break waits shrink to a quarter")

	a.serverTickRateBits.Store(math.Float32bits(10))
	require.InDelta(t, 2.0, a.breakTimeScale(), 1e-9, "slowed server: break waits grow")
}
