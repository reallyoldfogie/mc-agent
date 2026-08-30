package testing

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlayerAbilities_TrackedFromServer verifies the foundational plumbing
// for PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4 (Creative/Spectator
// Flight): the clientbound Abilities packet and the player's own game mode
// (from the Login packet) are correctly parsed and tracked, and SetFlying
// respects the server-granted AllowFlying permission.
func TestPlayerAbilities_TrackedFromServer(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Run("creative mode grants AllowFlying", func(t *testing.T) {
				env := setupStandaloneTestWithModeAndBlockPlacement(t, "flying_ability_creative", GameModeCreative, false, tt.MCVersion, DifficultyEasy, false)
				defer env.Cancel()

				require.Eventually(t, func() bool {
					_, ok := env.Agent.Agent.GetPlayerAbilities()
					return ok
				}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

				gameMode, ok := env.Agent.Agent.GetGameMode()
				require.True(t, ok, "game mode should be tracked after login")
				assert.Equal(t, models.GameModeCreative, gameMode)

				abilities, _ := env.Agent.Agent.GetPlayerAbilities()
				assert.True(t, abilities.AllowFlying, "creative mode should grant AllowFlying")
				assert.True(t, abilities.CreativeMode, "creative mode should set the CreativeMode ability flag")
				assert.False(t, abilities.Flying, "joining should not start already flying - requires an explicit toggle")

				ctx := context.Background()
				require.NoError(t, env.Agent.Agent.SetFlying(ctx, true), "SetFlying(true) should succeed when AllowFlying is true")
				abilities, _ = env.Agent.Agent.GetPlayerAbilities()
				assert.True(t, abilities.Flying, "tracked state should reflect the toggle immediately (no server echo to wait for)")

				require.NoError(t, env.Agent.Agent.SetFlying(ctx, false))
				abilities, _ = env.Agent.Agent.GetPlayerAbilities()
				assert.False(t, abilities.Flying)
			})

			t.Run("survival mode denies flying", func(t *testing.T) {
				env := setupStandaloneTestWithModeAndBlockPlacement(t, "flying_ability_survival", GameModeSurvival, false, tt.MCVersion, DifficultyEasy, false)
				defer env.Cancel()

				require.Eventually(t, func() bool {
					_, ok := env.Agent.Agent.GetPlayerAbilities()
					return ok
				}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

				gameMode, ok := env.Agent.Agent.GetGameMode()
				require.True(t, ok)
				assert.Equal(t, models.GameModeSurvival, gameMode)

				abilities, _ := env.Agent.Agent.GetPlayerAbilities()
				assert.False(t, abilities.AllowFlying, "survival mode should not grant AllowFlying")

				err := env.Agent.Agent.SetFlying(context.Background(), true)
				assert.Error(t, err, "SetFlying(true) should be refused without server-granted AllowFlying")
			})
		})
	}
}
