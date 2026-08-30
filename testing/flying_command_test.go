package testing

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestChatCommand_FlyAndLand verifies the "fly" and "land" chat commands
// end to end against a real server (PHASE 4.4 step 6): sent as real chat
// messages via RCON, not called directly, so this proves the commands are
// actually wired into the real dispatch pipeline, not just that
// agent.SetFlying itself works (already covered by
// testing/flying_ability_test.go).
func TestChatCommand_FlyAndLand(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "chat_fly_land", GameModeCreative, false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()

			require.Eventually(t, func() bool {
				_, ok := env.Agent.Agent.GetPlayerAbilities()
				return ok
			}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

			flying := sayCommandUntil(t, ctx, env, env.BotName, "fly", func() bool {
				abilities, _ := env.Agent.Agent.GetPlayerAbilities()
				return abilities.Flying
			})
			require.True(t, flying, "'fly' chat command should have enabled flying")

			landed := sayCommandUntil(t, ctx, env, env.BotName, "land", func() bool {
				abilities, _ := env.Agent.Agent.GetPlayerAbilities()
				return !abilities.Flying
			})
			require.True(t, landed, "'land' chat command should have disabled flying")
		})
	}
}
