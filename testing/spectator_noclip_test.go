package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpectatorAutoFliesAndClipsThroughWalls verifies PHASE 4.4 step 5
// (spectator noclip) end to end on a live server: spectator mode starts
// already flying with no toggle needed (GameMode.setAbilities() sets
// `abilities.flying = true` unconditionally for spectator, unlike
// creative's allowFlying-only default - see
// testing/flying_ability_test.go's creative case for the contrast), and a
// spectator can fly straight through a solid wall a normal player would
// collide with.
func TestSpectatorAutoFliesAndClipsThroughWalls(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			testSpectatorAutoFliesAndClipsThroughWalls(t, tt.MCVersion)
		})
	}
}

func testSpectatorAutoFliesAndClipsThroughWalls(t *testing.T, mcVersion string) {
	env := setupStandaloneTestWithModeAndBlockPlacement(t, "spectator_noclip", GameModeSpectator, false, mcVersion, DifficultyEasy, false)
	defer env.Cancel()

	ctx := context.Background()

	require.Eventually(t, func() bool {
		_, ok := env.Agent.Agent.GetPlayerAbilities()
		return ok
	}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

	gameMode, ok := env.Agent.Agent.GetGameMode()
	require.True(t, ok)
	assert.Equal(t, models.GameModeSpectator, gameMode)

	abilities, _ := env.Agent.Agent.GetPlayerAbilities()
	assert.True(t, abilities.AllowFlying)
	assert.False(t, abilities.CreativeMode, "spectator is not creative mode")
	assert.True(t, abilities.Flying, "spectator should start already flying, unlike creative which needs an explicit toggle")

	startPos, ok := env.Agent.Agent.GetPositionSimple()
	require.True(t, ok)

	// A solid wall of stone directly ahead (+Z), tall and wide enough that
	// going around or over it within the test window isn't plausible - the
	// only way through is noclip.
	wallZ := int(math.Floor(startPos.Z)) + 3
	baseX, baseY := int(math.Floor(startPos.X)), int(math.Floor(startPos.Y))
	_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf(
		"fill %d %d %d %d %d %d minecraft:stone",
		baseX-3, baseY-1, wallZ, baseX+3, baseY+5, wallZ))
	require.NoError(t, err, "place wall")

	require.NoError(t, env.Agent.Agent.EnterManualMode())
	defer func() { _ = env.Agent.Agent.ExitManualMode() }()

	require.NoError(t, env.Agent.Agent.SetManualThrottle(0, 1))
	time.Sleep(2 * time.Second)
	require.NoError(t, env.Agent.Agent.SetManualThrottle(0, 0))

	endPos, ok := env.Agent.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("start=(%.2f,%.2f,%.2f) end=(%.2f,%.2f,%.2f) wallZ=%d", startPos.X, startPos.Y, startPos.Z, endPos.X, endPos.Y, endPos.Z, wallZ)
	assert.Greater(t, endPos.Z, float64(wallZ)+1.0, "spectator should have flown straight through the wall")
}
