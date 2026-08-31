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

// TestCamFollow verifies StartCamFollow end to end against a real server:
// the companion Cam agent switches itself to spectator mode via RCON and
// then continuously teleports (also via RCON) to stay within maxDistance
// blocks of the main agent, including snapping instantly when the main
// agent is itself teleported a long distance rather than walking there.
//
// Called directly on the already-spawned Cam agent (env.Agent.Cam) rather
// than via testing.AgentConfig.EnableCamFollow, so this doesn't need any
// change to the widely-shared setupStandaloneTestWithModeAndBlockPlacement
// helper - StartCamFollow is a public agent capability, identical to what
// a real chat command or CLI flag would invoke.
func TestCamFollow(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "cam_follow", "survival", false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()
			require.NotNil(t, env.Agent.Cam, "cam agent should be spawned by default")

			const maxDistance = 6.0
			require.NoError(t, env.Agent.Cam.Agent.StartCamFollow(ctx, env.BotName, maxDistance), "start cam-follow")

			require.Eventually(t, func() bool {
				gameMode, ok := env.Agent.Cam.Agent.GetGameMode()
				return ok && gameMode == models.GameModeSpectator
			}, 10*time.Second, 200*time.Millisecond, "cam should switch to spectator mode via RCON")

			abilities, ok := env.Agent.Cam.Agent.GetPlayerAbilities()
			require.True(t, ok)
			assert.True(t, abilities.Flying, "spectator mode should start already flying (see PHASE 4.4)")

			assertWithinDistance := func(label string) {
				t.Helper()
				require.Eventually(t, func() bool {
					mainPos, ok := env.Agent.Agent.GetPositionSimple()
					if !ok {
						return false
					}
					camPos, ok := env.Agent.Cam.Agent.GetPositionSimple()
					if !ok {
						return false
					}
					dx := mainPos.X - camPos.X
					dy := mainPos.Y - camPos.Y
					dz := mainPos.Z - camPos.Z
					dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
					t.Logf("%s: main=(%.1f,%.1f,%.1f) cam=(%.1f,%.1f,%.1f) dist=%.2f", label, mainPos.X, mainPos.Y, mainPos.Z, camPos.X, camPos.Y, camPos.Z, dist)
					return dist <= maxDistance
				}, 5*time.Second, 200*time.Millisecond, "%s: cam should be within %.1f blocks of the main agent", label, maxDistance)
			}

			// Baseline: cam should already have closed in on the main agent
			// (its own spawn position could be far from the main agent's,
			// e.g. a different vanilla spawn scatter point).
			assertWithinDistance("initial")

			// Simulate an abrupt teleport (not a walk) - "including
			// teleportation" per the feature's own requirement.
			mainPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok)
			farX, farY, farZ := mainPos.X+150, mainPos.Y, mainPos.Z+150
			_, err := env.Inst.RCON.Exec(ctx, fmt.Sprintf("teleport %s %.2f %.2f %.2f", env.BotName, farX, farY, farZ))
			require.NoError(t, err, "teleport main agent far away")

			assertWithinDistance("after main agent teleported")
		})
	}
}
