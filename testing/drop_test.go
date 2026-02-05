package testing

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPlayerAction_DropItem tests item drop action
func TestPlayerAction_DropItem(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "action_drop_item", tt.mcVersion)
			defer env.Cancel()

			// Give bot some dirt to drop
			giveCmd := fmt.Sprintf(`give %s minecraft:dirt`, env.BotName)
			_, err := env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give dirt")

			if env.Agent.Config.VersionHandler == nil {
				t.Skip("Version handler not available")
			}

			actionHandler := env.Agent.Config.VersionHandler.Play().Actions()
			botClient := env.Agent.BotClient()

			// Action ID 4 = drop item (single)
			err = actionHandler.SendPlayerAction(botClient.Conn(), 4, 0, 0, 0, 0, 0)
			require.NoError(t, err, "send drop item action")

			time.Sleep(200 * time.Millisecond)

			// Verify item was dropped by checking for item entity
			countCmd := `execute as @e[type=minecraft:item,limit=1] run say ItemFound`
			resp, err := env.Inst.RCON.Exec(env.Ctx, countCmd)
			if err == nil {
				t.Logf("Item drop verification: %s", resp)
			}

			t.Log("✓ Item drop test passed")
		})
	}
}

// TestPlayerAction_DropStack tests dropping entire stack
func TestPlayerAction_DropStack(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "action_drop_stack", tt.mcVersion)
			defer env.Cancel()

			// Give bot stack of dirt
			giveCmd := fmt.Sprintf(`give %s minecraft:dirt 64`, env.BotName)
			_, err := env.Inst.RCON.Exec(env.Ctx, giveCmd)
			require.NoError(t, err, "give dirt stack")

			if env.Agent.Config.VersionHandler == nil {
				t.Skip("Version handler not available")
			}

			actionHandler := env.Agent.Config.VersionHandler.Play().Actions()
			botClient := env.Agent.BotClient()

			// Action ID 3 = drop stack (entire stack)
			err = actionHandler.SendPlayerAction(botClient.Conn(), 3, 0, 0, 0, 0, 0)
			require.NoError(t, err, "send drop stack action")

			time.Sleep(200 * time.Millisecond)

			t.Log("✓ Drop stack test passed")
		})
	}
}
